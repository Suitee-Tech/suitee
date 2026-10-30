import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import PizZip from "pizzip";
import { ContractStatus, type Contract, type SignatureEnvelope } from "../../../../shared/contracts/types";
import type { DealDTO } from "../../../../shared/services/deal.service";
import { ContractTab } from "./ContractTab";
import { addMonthsClamped } from "../../../../shared/lib/finance/rentalProration";

const permissionMocks = vi.hoisted(() => ({
  can: vi.fn((_permission?: string) => true),
  isLoading: false,
}));

// Chamadas financeiras ficam observáveis: o teste de perfil sem acesso precisa
// provar que elas NEM SAEM (o endpoint devolveria 403).
const financialMocks = vi.hoisted(() => ({
  getSettings: vi.fn(async (): Promise<Record<string, number>> => ({ default_due_day: 7, admin_fee_pct: 8 })),
  getDraft: vi.fn(async () => ({ total_amount_cents: 100000, mode: "CASH", first_due_date: "2026-09-17", down_payment_cents: 0 })),
}));

const mocks = vi.hoisted(() => ({
  list: vi.fn(), regenerate: vi.fn(), create: vi.fn(), send: vi.fn(), envelope: vi.fn(),
  notify: vi.fn(), success: vi.fn(), configCreate: vi.fn(),
  useRealTemplatePipeline: false, templateContent: "template",
  templatesList: vi.fn(), templateCreate: vi.fn(),
}));
vi.mock("../../../../shared/context/DealContractMutationsContext", () => ({ useDealContractMutationsContext: () => ({ createContractFromDeal: mocks.create }) }));
vi.mock("../../../../shared/context/DealsContext", () => ({ useDealsContext: () => ({ inviteSigner: vi.fn() }) }));
vi.mock("../../../../shared/context/AuthUserContext", () => ({ useAuthUser: () => ({ user: null }) }));
vi.mock("../../../../shared/context/PermissionsContext", () => ({ usePermissionsContext: () => permissionMocks }));
vi.mock("../../../../shared/context/DealChecklistsContext", () => ({ useDealChecklistsContext: () => ({ dealChecklists: {} }) }));
vi.mock("../../../../shared/ws/useWsSubscription", () => ({ useWsSubscription: vi.fn() }));
vi.mock("../../../../shared/services/contract.service", () => ({ contractService: { listByDeal: mocks.list, regenerate: mocks.regenerate, sendForSignature: mocks.send } }));
vi.mock("../../../../shared/services/signature.service", () => ({ signatureService: { getByContract: mocks.envelope } }));
vi.mock("../../../../shared/services/deal.service", () => ({ dealService: {} }));
vi.mock("../../../../shared/services/lead.service", () => ({ leadService: {} }));
vi.mock("../../../../shared/services/property.service", () => ({ propertyService: {} }));
vi.mock("../../../../shared/services/broker.service", () => ({ brokerService: {} }));
vi.mock("../../../../shared/services/template.service", () => ({
  templateService: {
    list: (...args: unknown[]) => mocks.templatesList(...args),
    create: (...args: unknown[]) => mocks.templateCreate(...args),
  },
}));
// O editor de modelos é a tela inteira de DOCX: aqui ele é substituído pelo
// contrato do modo embutido (`embedded`/`onExit`/`onSaved`), que é o que esta
// suíte precisa provar — o editor em si é exercitado no beta.
vi.mock("../../../../shared/pages/ContractTemplates", () => ({
  ContractTemplates: ({
    embedded,
    onExit,
    onSaved,
  }: {
    embedded?: boolean;
    onExit?: () => void;
    onSaved?: (template: { id: string; name: string }) => void;
  }) =>
    embedded ? (
      <div>
        <span>Editor de modelos embutido</span>
        <button type="button" onClick={() => onExit?.()}>Fechar cadastro do modelo</button>
        <button
          type="button"
          onClick={() => onSaved?.({ id: "template-9", name: "Modelo novo" })}
        >
          Salvar modelo
        </button>
      </div>
    ) : (
      <div>Tela de modelos (rota)</div>
    ),
}));
vi.mock("../../../../shared/services/saleBillingPlan.service", () => ({ saleBillingPlanService: { getDraft: financialMocks.getDraft, upsertDraft: vi.fn() } }));
vi.mock("../../../../shared/services/rentalChargeConfig.service", () => ({ rentalChargeConfigService: { create: mocks.configCreate } }));
vi.mock("../../../../shared/services/tenantFinancialSettings.service", () => ({ tenantFinancialSettingsService: { get: financialMocks.getSettings, update: vi.fn() } }));
vi.mock("../../../../shared/services/errorToast", () => ({ notifyApiError: mocks.notify }));
vi.mock("../../../../shared/services/api", () => ({ unwrapApiData: (v: unknown) => v }));
const flagsState = vi.hoisted(() => ({ multiPartyUI: false }));
vi.mock("../../../../shared/config/flags", () => ({ flags: flagsState }));
vi.mock("sonner", () => ({ toast: { success: mocks.success, error: vi.fn(), warning: vi.fn() } }));
vi.mock("@superdoc-dev/react", () => ({ SuperDocEditor: () => <div>Editor da nova minuta</div> }));
vi.mock("../../../../shared/components/BillingPlanWizard", () => ({ BillingPlanWizard: () => null }));
vi.mock("./ContractSignatureSection", () => ({ ContractSignatureSection: () => null }));
vi.mock("../../../../shared/utils/contractVariableData", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../../../shared/utils/contractVariableData")>();
  return {
    ...actual,
    buildContractVariableData: (...args: Parameters<typeof actual.buildContractVariableData>) =>
      mocks.useRealTemplatePipeline ? actual.buildContractVariableData(...args) : { buyer: "Cliente Teste" },
  };
});
vi.mock("../../../../shared/components/signature/SignedDocumentsSection", () => ({ SignedDocumentsSection: () => null }));
vi.mock("../../../../shared/components/signature/SendForSignatureModal", () => ({
  SendForSignatureModal: ({ open, contract, onSubmit }: { open: boolean; contract: Contract; onSubmit: (a: object, b: object) => void }) => open ? (
    <div>
      <span>{contract.signatories[0]?.email}</span>
      <button onClick={() => onSubmit({}, {})}>Confirmar envio</button>
    </div>
  ) : null,
}));
vi.mock("../../../../shared/components/contract-editor/PreviewVariableForm", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../../../shared/components/contract-editor/PreviewVariableForm")>();
  return {
    PreviewVariableForm: (props: React.ComponentProps<typeof actual.PreviewVariableForm>) => mocks.useRealTemplatePipeline
      ? <actual.PreviewVariableForm {...props} />
      : <button onClick={() => props.onConfirm(props.autoFilledData)}>Confirmar dados</button>,
  };
});
vi.mock("../../../../shared/utils/docx", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../../../shared/utils/docx")>();
  return {
    ...actual,
    extractDocxTemplateVariables: (...args: Parameters<typeof actual.extractDocxTemplateVariables>) => mocks.useRealTemplatePipeline ? actual.extractDocxTemplateVariables(...args) : [],
    fillDocxTemplate: (...args: Parameters<typeof actual.fillDocxTemplate>) => mocks.useRealTemplatePipeline ? actual.fillDocxTemplate(...args) : "bmV3LWRvY3g=",
    base64ToFile: (...args: Parameters<typeof actual.base64ToFile>) => mocks.useRealTemplatePipeline ? actual.base64ToFile(...args) : new File(["new-docx"], "contrato.docx", { type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document" }),
  };
});

const draft: Contract = {
  id: "contract-1", deal_id: "deal-1", status: ContractStatus.PENDING_SIGNATURE,
  content: "old-docx", signatories: [{ id: "buyer-1", name: "Cliente Teste", email: "cliente@example.com", role: "comprador" }],
  type: "venda", start_date: "2026-09-09T00:00:00Z", created_at: "2026-09-09T00:00:00Z",
  updated_at: "2026-09-09T01:00:00Z", tenant_id: "tenant-1", signature_locked_at: "2026-09-09T01:00:00Z",
};
const failed: SignatureEnvelope = {
  id: "env-1", contract_id: draft.id, tenant_id: "tenant-1", environment: "production", status: "ERROR", signers: [],
  last_error: "E-mail do usuário da API não configurado.", created_at: "2026-09-09T00:00:00Z", updated_at: "2026-09-09T00:00:00Z",
};
const deal = {
  id: "deal-1", type: "SALE", status: "CONTRACT", value: 1000, parties: [], proposals: [],
  signers: [{ id: "buyer-1", name: "Cliente Teste", email: "cliente@example.com", role: "comprador", status: "PENDING" }],
} as unknown as DealDTO;

function mount(dealDetails: DealDTO = deal) {
  return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <MemoryRouter><ContractTab dealDetails={dealDetails} onRefresh={vi.fn()} onGoToTab={vi.fn()} /></MemoryRouter>
  </QueryClientProvider>);
}

async function prepare(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: "Regerar contrato" }));
  await user.click(screen.getByRole("button", { name: "Pré-visualizar Contrato" }));
  await user.click(screen.getByRole("button", { name: "Confirmar dados" }));
}

describe("ContractTab regeneration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Perfil com acesso financeiro: os fluxos de recebimento/plano desta suíte
    // são exercitados por quem pode configurá-los.
    permissionMocks.can.mockReturnValue(true);
    permissionMocks.isLoading = false;
    mocks.useRealTemplatePipeline = false;
    mocks.templateContent = "template";
    mocks.templatesList.mockImplementation(async () => [
      { id: "template-1", name: "Modelo de venda", content: mocks.templateContent, variables: [] },
    ]);
    mocks.list.mockResolvedValue([draft]);
    mocks.envelope.mockResolvedValue(failed);
    mocks.regenerate.mockResolvedValue({ ...draft, status: ContractStatus.DRAFT, signature_locked_at: undefined, updated_at: "2026-09-09T02:00:00Z" });
    mocks.send.mockResolvedValue({ ...draft, updated_at: "2026-09-09T03:00:00Z" });
  });

  it("reviews actual template variables and saves a sale DOCX with a recalculated deadline without sending", async () => {
    const templateZip = new PizZip();
    templateZip.file("[Content_Types].xml", '<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>');
    templateZip.file("_rels/.rels", '<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>');
    templateZip.file("word/document.xml", '<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>{{buyer_name}} — {{buyer.nationality}}. Vencimento: {{current_date_plus_30_days}}</w:t></w:r></w:p><w:sectPr/></w:body></w:document>');
    mocks.useRealTemplatePipeline = true;
    mocks.templateContent = templateZip.generate({ type: "base64" });
    mocks.list.mockResolvedValue([]);
    mocks.envelope.mockResolvedValue(null);
    mocks.create.mockResolvedValue({ ...draft, status: ContractStatus.DRAFT, signature_locked_at: undefined });
    const qualifiedDeal = {
      ...deal,
      parties: [{ id: "buyer-1", role: "buyer", is_primary: true, name: "Cliente Teste", document: "12345678900", email: "cliente@example.com", custom_fields: { nationality: "brasileira" } }],
    } as unknown as DealDTO;
    const user = userEvent.setup();
    mount(qualifiedDeal);

    await user.click(await screen.findByRole("button", { name: "Pré-visualizar Contrato" }));
    expect(screen.getByText("{{buyer.nationality}}")).toBeInTheDocument();
    expect(screen.getByText("{{current_date_plus_30_days}}")).toBeInTheDocument();
    expect(screen.getByDisplayValue("brasileira")).toBeInTheDocument();
    const sourceDate = screen.getByRole("textbox", { name: /^Data Atual \(DD\/MM\/AAAA\)/ });
    await user.clear(sourceDate);
    await user.type(sourceDate, "31/01/2026");
    expect(screen.getByDisplayValue("02/03/2026")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Gerar Preview" }));
    await user.click(screen.getByRole("button", { name: "Finalizar e Gerar Contrato" }));

    await waitFor(() => expect(mocks.create).toHaveBeenCalledOnce());
    const [dealId, request] = mocks.create.mock.calls[0];
    expect(dealId).toBe(qualifiedDeal.id);
    expect(request.type).toBe("venda");
    const savedZip = new PizZip(request.content.split(",").at(-1), { base64: true });
    const savedXml = savedZip.file("word/document.xml")!.asText();
    expect(savedXml).toContain("Cliente Teste");
    expect(savedXml).toContain("brasileira");
    expect(savedXml).toContain("02/03/2026");
    expect(savedXml).not.toContain("{{");
    expect(savedXml).not.toContain("}}");
    expect(mocks.regenerate).not.toHaveBeenCalled();
    expect(mocks.configCreate).not.toHaveBeenCalled();
    expect(mocks.send).not.toHaveBeenCalled();
  });

  it("saves Portuguese date formats selected for ISO tags through the actual review form", async () => {
    const templateZip = new PizZip();
    templateZip.file("[Content_Types].xml", '<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>');
    templateZip.file("_rels/.rels", '<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>');
    templateZip.file("word/document.xml", '<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>{{buyer_name}}. Término: {{contract_end_date_iso}}. Prazo: {{contract_end_date_plus_5_days_iso}}</w:t></w:r></w:p><w:sectPr/></w:body></w:document>');
    mocks.useRealTemplatePipeline = true;
    mocks.templateContent = templateZip.generate({ type: "base64" });
    mocks.list.mockResolvedValue([{ ...draft, end_date: "2029-03-14T00:00:00Z" }]);
    const user = userEvent.setup();
    mount();

    await user.click(await screen.findByRole("button", { name: "Regerar contrato" }));
    await user.click(screen.getByRole("button", { name: "Pré-visualizar Contrato" }));
    expect(screen.getByDisplayValue("2029-03-14")).toBeInTheDocument();
    await user.selectOptions(screen.getByRole("combobox", { name: "Formato da data no contrato: contract_end_date_iso" }), "br");
    await user.selectOptions(screen.getByRole("combobox", { name: "Formato da data no contrato: contract_end_date_plus_5_days_iso" }), "written");
    expect(screen.getByLabelText("Data no contrato: contract_end_date_iso")).toHaveTextContent("14/03/2029");
    expect(screen.getByLabelText("Data no contrato: contract_end_date_plus_5_days_iso")).toHaveTextContent("19 de março de 2029");

    const sourceDate = screen.getByRole("textbox", { name: /^Data de Término do Contrato/ });
    await user.clear(sourceDate);
    await user.type(sourceDate, "15/03/2029");
    expect(screen.getByDisplayValue("2029-03-15")).toBeInTheDocument();
    expect(screen.getByDisplayValue("2029-03-20")).toBeInTheDocument();
    expect(screen.getByLabelText("Data no contrato: contract_end_date_iso")).toHaveTextContent("15/03/2029");
    expect(screen.getByLabelText("Data no contrato: contract_end_date_plus_5_days_iso")).toHaveTextContent("20 de março de 2029");
    await user.click(screen.getByRole("button", { name: "Gerar Preview" }));
    await user.click(screen.getByRole("button", { name: "Salvar nova minuta" }));

    await waitFor(() => expect(mocks.regenerate).toHaveBeenCalledOnce());
    const [contractId, request] = mocks.regenerate.mock.calls[0];
    expect(contractId).toBe(draft.id);
    const savedZip = new PizZip(request.content.split(",").at(-1), { base64: true });
    const savedXml = savedZip.file("word/document.xml")!.asText();
    expect(savedXml).toContain("15/03/2029");
    expect(savedXml).toContain("20 de março de 2029");
    expect(savedXml).not.toContain("2029-03-15");
    expect(savedXml).not.toContain("2029-03-20");
    expect(savedXml).not.toContain("{{");
    expect(mocks.create).not.toHaveBeenCalled();
    expect(mocks.configCreate).not.toHaveBeenCalled();
    expect(mocks.send).not.toHaveBeenCalled();
  });

  it("revises the existing contract and requires an explicit new send", async () => {
    const user = userEvent.setup(); mount();
    await prepare(user);
    await user.click(screen.getByRole("button", { name: "Salvar nova minuta" }));
    await waitFor(() => expect(mocks.regenerate).toHaveBeenCalledWith(draft.id, {
      content: expect.stringContaining("bmV3LWRvY3g="), signatories: draft.signatories, expected_updated_at: draft.updated_at,
    }));
    expect(mocks.create).not.toHaveBeenCalled();
    expect(mocks.configCreate).not.toHaveBeenCalled();
    expect(mocks.send).not.toHaveBeenCalled();
    await user.click(await screen.findByRole("button", { name: "Enviar para Assinatura" }));
    await user.click(screen.getByRole("button", { name: "Confirmar envio" }));
    await waitFor(() => expect(mocks.send).toHaveBeenCalledWith(draft.id, {}));
  });

  it("permite regerar depois de cancelar o envelope antes da assinatura", async () => {
    mocks.envelope.mockResolvedValue({ ...failed, status: "CANCELED", external_envelope_id: "cs-old" });
    mocks.regenerate.mockResolvedValue({
      ...draft,
      status: ContractStatus.DRAFT,
      signature_locked_at: undefined,
      external_envelope_id: "",
      updated_at: "2026-09-09T02:00:00Z",
    });
    const user = userEvent.setup();
    mount();

    await user.click(await screen.findByRole("button", { name: "Regerar contrato" }));
    await user.click(screen.getByRole("button", { name: "Pré-visualizar Contrato" }));
    await user.click(screen.getByRole("button", { name: "Confirmar dados" }));
    await user.click(screen.getByRole("button", { name: "Salvar nova minuta" }));

    await waitFor(() => expect(mocks.regenerate).toHaveBeenCalledWith(
      draft.id,
      expect.objectContaining({ expected_updated_at: draft.updated_at }),
    ));
    expect(await screen.findByRole("button", { name: "Enviar para Assinatura" })).toBeInTheDocument();
  });

  it("reflete o e-mail corrigido na esteira antes do envio", async () => {
    mocks.list.mockResolvedValue([{ ...draft, status: ContractStatus.DRAFT, signature_locked_at: undefined, external_envelope_id: "" }]);
    mocks.envelope.mockResolvedValue(null);
    const correctedDeal = {
      ...deal,
      signers: [{ ...((deal.signers ?? [])[0] as unknown as Record<string, unknown>), email: "corrigido@example.com" }],
    } as unknown as DealDTO;
    const user = userEvent.setup();
    mount(correctedDeal);

    await user.click(await screen.findByRole("button", { name: "Enviar para Assinatura" }));
    expect(await screen.findByText("corrigido@example.com")).toBeInTheDocument();
  });

  it("cancel keeps the original document without writes", async () => {
    const user = userEvent.setup(); mount();
    await prepare(user);
    await user.click(screen.getByRole("button", { name: "Cancelar" }));
    expect(screen.getByText("Minuta Gerada")).toBeInTheDocument();
    expect(mocks.regenerate).not.toHaveBeenCalled();
  });

  it("keeps the editor and original contract when saving fails", async () => {
    mocks.regenerate.mockRejectedValueOnce(new Error("conflict"));
    const user = userEvent.setup(); mount(); await prepare(user);
    await user.click(screen.getByRole("button", { name: "Salvar nova minuta" }));
    await waitFor(() => expect(mocks.notify).toHaveBeenCalled());
    expect(screen.getByText("Editor da nova minuta")).toBeInTheDocument();
    expect(mocks.send).not.toHaveBeenCalled();
  });

  it.each(["external", "signed", "unknown"])("blocks regeneration for %s state", async (state) => {
    if (state === "external") mocks.envelope.mockResolvedValue({ ...failed, external_envelope_id: "cs-1" });
    if (state === "signed") mocks.list.mockResolvedValue([{ ...draft, status: ContractStatus.SIGNED }]);
    if (state === "unknown") mocks.envelope.mockRejectedValue(new Error("network"));
    mount(); await screen.findByText("Minuta Gerada");
    await waitFor(() => expect(mocks.envelope).toHaveBeenCalled());
    expect(screen.queryByRole("button", { name: "Regerar contrato" })).not.toBeInTheDocument();
  });
});

/**
 * Acesso financeiro na aba Assinatura.
 *
 * O painel "Plano financeiro antes do contrato" era renderizado para qualquer
 * perfil que enxergasse a aba — um corretor via o plano-base, a taxa de
 * administração e os botões de recebimento, e as chamadas por trás voltavam 403
 * (um "Configuração pendente" que ele não tinha como resolver).
 */
describe("ContractTab — acesso financeiro na assinatura", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.useRealTemplatePipeline = false;
    mocks.list.mockResolvedValue([]);
    mocks.envelope.mockResolvedValue(null);
    mocks.create.mockResolvedValue({ ...draft, status: ContractStatus.DRAFT, signature_locked_at: undefined });
  });

  it("esconde o plano financeiro e explica a restrição sem acesso", async () => {
    permissionMocks.can.mockReturnValue(false);
    mount({ ...deal, type: "RENT" } as DealDTO);

    expect(await screen.findByText("Plano financeiro restrito")).toBeInTheDocument();
    expect(screen.getByText(/não acompanha a configuração financeira desta negociação/)).toBeInTheDocument();
    expect(screen.getByText(/solicite a permissão/)).toBeInTheDocument();
    // Nada de plano-base, mapa de contas ou valores estimados.
    expect(screen.queryByText("Plano financeiro antes do contrato")).not.toBeInTheDocument();
    expect(screen.queryByText(/Mapa de lançamentos e contas/)).not.toBeInTheDocument();
    expect(screen.getByText("Acompanhamento financeiro restrito ao seu perfil.")).toBeInTheDocument();
    // O gate é por permissão financeira (módulo) — não por cargo.
    expect(permissionMocks.can).toHaveBeenCalledWith("finance:read");
    expect(permissionMocks.can).toHaveBeenCalledWith("charges:write");
  });

  it("não busca configuração financeira nem recebimento sem permissão", async () => {
    permissionMocks.can.mockReturnValue(false);
    mount({ ...deal, type: "RENT" } as DealDTO);

    expect(await screen.findByText("Plano financeiro restrito")).toBeInTheDocument();
    expect(financialMocks.getSettings).not.toHaveBeenCalled();
    expect(financialMocks.getDraft).not.toHaveBeenCalled();
  });

  it("mostra o plano para quem tem acesso a cobranças (Operações)", async () => {
    permissionMocks.can.mockImplementation((permission?: string) => permission === "charges:write");
    mount({ ...deal, type: "RENT" } as DealDTO);

    expect(await screen.findByText("Plano financeiro antes do contrato")).toBeInTheDocument();
    expect(screen.queryByText("Plano financeiro restrito")).not.toBeInTheDocument();
  });
});

/**
 * Prazo do contrato de locação no plano pré-contrato.
 *
 * O término era sempre "início + 30 meses" embutido no código, e a soma usava
 * `setUTCMonth` (31/08 + 30 meses virava 03/03 — um dia que não existe no
 * contrato, inflando o residual do último aluguel). Agora o prazo é campo, o
 * término é derivado com ancoragem no último dia do mês, e editar o término
 * devolve o prazo em meses inteiros: as três pontas ficam coerentes.
 */
describe("ContractTab — prazo do contrato de locação", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    permissionMocks.can.mockReturnValue(true);
    permissionMocks.isLoading = false;
    mocks.list.mockResolvedValue([]);
    mocks.envelope.mockResolvedValue(null);
  });

  const mountRental = () => mount({ ...deal, type: "RENT", value: 3_200 } as DealDTO);

  it("adapta o término ao mudar o início e ao mudar o prazo", async () => {
    mountRental();
    const start = await screen.findByLabelText(/Início do contrato/i);
    const term = screen.getByLabelText(/Prazo do contrato \(meses\)/i);
    const end = screen.getByLabelText(/Término do contrato/i);

    // Padrão de 30 meses a partir do início escolhido.
    fireEvent.change(start, { target: { value: "2027-01-15" } });
    expect(term).toHaveValue(30);
    expect(end).toHaveValue("2029-07-15");

    // Prazo manda no término.
    fireEvent.change(term, { target: { value: "12" } });
    expect(end).toHaveValue("2028-01-15");

    // Término manda no prazo (meses inteiros).
    fireEvent.change(end, { target: { value: "2028-07-15" } });
    expect(term).toHaveValue(18);
  });

  it("ancora no último dia do mês ao somar o prazo", async () => {
    mountRental();
    const start = await screen.findByLabelText(/Início do contrato/i);
    const end = screen.getByLabelText(/Término do contrato/i);

    fireEvent.change(start, { target: { value: "2026-08-31" } });
    // 31/08/2026 + 30 meses = 28/02/2029 (e não 03/03/2029).
    expect(end).toHaveValue("2029-02-28");
  });

  it("adota o prazo padrão configurado pela imobiliária", async () => {
    // Financeiro → Padrões pode definir outro prazo padrão (ex.: 12 meses).
    financialMocks.getSettings.mockResolvedValueOnce({ default_due_day: 7, admin_fee_pct: 8, default_lease_term_months: 12 });
    mountRental();

    const start = await screen.findByLabelText(/Início do contrato/i);
    const term = screen.getByLabelText(/Prazo do contrato \(meses\)/i);
    const end = screen.getByLabelText(/Término do contrato/i);

    await waitFor(() => expect(term).toHaveValue(12));
    const startValue = (start as HTMLInputElement).value;
    expect(end).toHaveValue(addMonthsClamped(startValue, 12));

    // Escolher um prazo à mão vence o padrão da imobiliária.
    fireEvent.change(term, { target: { value: "24" } });
    expect(end).toHaveValue(addMonthsClamped(startValue, 24));
    expect(term).toHaveValue(24);
  });

  it("mostra a prévia de pró-rata e residual do calendário", async () => {
    mountRental();
    const start = await screen.findByLabelText(/Início do contrato/i);
    const end = screen.getByLabelText(/Término do contrato/i);

    fireEvent.change(start, { target: { value: "2026-04-10" } });
    fireEvent.change(end, { target: { value: "2029-04-15" } });

    expect(screen.getByText("Calendário de cobranças do contrato")).toBeInTheDocument();
    expect(screen.getByText(/21 de 30 dia\(s\)/)).toBeInTheDocument();
    expect(screen.getByText(/15 de 30 dia\(s\)/)).toBeInTheDocument();
  });
});

// Relato: com o aceite de todas as partes marcado como OPCIONAL nos parâmetros da
// proposta, a geração do contrato continuava bloqueada ("É necessário o aceite das
// partes primárias para gerar o contrato"). Sem parte obrigatória não há aceite a
// exigir — é o que a própria tela de parâmetros anuncia.
function dealWithAcceptance(signatureRequired: boolean[], proposalStatus = "SENT") {
  return {
    ...deal,
    parties: signatureRequired.map((required, index) => ({
      id: `p${index + 1}`,
      name: `Parte ${index + 1}`,
      email: `p${index + 1}@example.com`,
      role: index === 0 ? "comprador" : "vendedor",
      is_primary: true,
      signature_required: required,
    })),
    proposals: [{ id: "prop-1", status: proposalStatus, accepted_by: [] }],
  } as unknown as DealDTO;
}

// O caso complementar ("com parte obrigatória o gate continua fechado") é provado
// nos testes de serviço do toca-deal, onde a regra vive.
it("libera a geração do contrato quando nenhuma parte precisa aceitar", async () => {
  flagsState.multiPartyUI = true;
  try {
    mount(dealWithAcceptance([false, false]));
    expect(await screen.findByText(/Aceite não exigido/)).toBeInTheDocument();
    expect(screen.queryByText(/É necessário o aceite das partes obrigatórias para gerar o contrato/)).toBeNull();
  } finally {
    flagsState.multiPartyUI = false;
  }
});


// Sem nenhum modelo cadastrado não há minuta a gerar: o estado vazio do
// seletor precisa oferecer o cadastro (o texto antigo mandava "criar em
// Configurações" e deixava o operador sem caminho).
describe("ContractTab — primeiro modelo de contrato", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    permissionMocks.can.mockReturnValue(true);
    permissionMocks.isLoading = false;
    mocks.useRealTemplatePipeline = false;
    mocks.templatesList.mockResolvedValue([]);
    mocks.list.mockResolvedValue([]);
    mocks.envelope.mockResolvedValue(null);
  });

  it("oferece criar o primeiro modelo quando não há nenhum cadastrado", async () => {
    mount();

    expect(await screen.findByText("Nenhum modelo de contrato cadastrado.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Criar modelo de contrato/ })).toBeInTheDocument();
    expect(screen.queryByText(/Crie um em Configurações/)).not.toBeInTheDocument();
  });

  it("abre o editor de modelos no próprio painel e mantém o negócio atrás", async () => {
    const user = userEvent.setup();
    mount();

    await user.click(await screen.findByRole("button", { name: /Criar modelo de contrato/ }));

    const painel = await screen.findByRole("dialog", { name: "Novo modelo de contrato" });
    // O editor entra por `lazy`: o conteúdo aparece depois do fallback.
    expect(await within(painel).findByText("Editor de modelos embutido")).toBeInTheDocument();
    // A tela de modelos como ROTA não é montada: o painel usa o modo embutido.
    expect(screen.queryByText("Tela de modelos (rota)")).not.toBeInTheDocument();

    await user.click(within(painel).getByRole("button", { name: "Fechar cadastro do modelo" }));
    expect(screen.queryByRole("dialog", { name: "Novo modelo de contrato" })).not.toBeInTheDocument();
    expect(screen.getByText("Nenhum modelo de contrato cadastrado.")).toBeInTheDocument();
  });

  it("seleciona o modelo recém-criado e recarrega a lista", async () => {
    const user = userEvent.setup();
    mount();
    await user.click(await screen.findByRole("button", { name: /Criar modelo de contrato/ }));
    const painel = await screen.findByRole("dialog", { name: "Novo modelo de contrato" });

    mocks.templatesList.mockResolvedValue([
      { id: "template-9", name: "Modelo novo", content: "template", variables: [] },
    ]);
    await user.click(within(painel).getByRole("button", { name: "Salvar modelo" }));

    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Novo modelo de contrato" })).not.toBeInTheDocument());
    const seletor = await screen.findByRole("button", { name: /Modelo novo/ });
    expect(seletor.className).toContain("border-primary");
    expect(mocks.success).toHaveBeenCalledWith(expect.stringContaining("Modelo novo"));
  });

  it("sem permissão de editar modelos orienta quem pode criar, em vez de oferecer o CTA", async () => {
    permissionMocks.can.mockImplementation((permission?: string) => permission !== "templates:write");
    mount();

    expect(await screen.findByText(/Peça a um administrador ou gestor/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Criar modelo de contrato/ })).not.toBeInTheDocument();
  });
});
