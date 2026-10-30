import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";

import type { Charge } from "../services/charge.service";
import { ChargeDetailDrawer } from "./ChargeDetailDrawer";
import { PermissionsContext, type PermissionsContextValue } from "../context/PermissionsContext";

// O reparo usa React Query; o mock evita QueryClientProvider e permite afirmar
// o payload enviado ao backend (mesmo padrão dos outros modais financeiros).
const repairMock = vi.hoisted(() => ({ mutateAsync: vi.fn(async () => ({})) }));
const eligibilityMock = vi.hoisted(() => ({
  data: { eligible: true as boolean, reason: undefined as string | undefined },
  isLoading: false,
  calls: [] as Array<{ chargeId: string | null; enabled: boolean }>,
}));
vi.mock("../hooks/useContracts", () => ({
  useRepairRentalAllocationsMutation: () => repairMock,
  useRepairRentalAllocationsEligibilityQuery: (chargeId: string | null, enabled: boolean) => {
    eligibilityMock.calls.push({ chargeId, enabled });
    return { data: eligibilityMock.data, isLoading: eligibilityMock.isLoading };
  },
}));

const permissionsValue = (allowed: string[]): PermissionsContextValue => ({
  functionalRole: "operador",
  baseRole: "operador",
  hasTenantWideScope: true,
  profile: "operador",
  permissions: new Set(allowed),
  can: (permission: string) => allowed.includes(permission),
  isLoading: false,
  refresh: async () => {},
});

const caseGuideCharge: Charge = {
  id: "charge-aug-2026",
  tenant_id: "imobiliaria-estrela",
  contract_id: "ctr-plm245",
  tenant_party_id: "party-pedro",
  owner_id: "party-marisa",
  property_id: "property-palmeiras-245",
  type: "RENT",
  description: "Aluguel e encargos — agosto/2026",
  reference_month: "2026-08",
  amount: 6000,
  late_fee: 0,
  interest: 0,
  discount: 0,
  total_amount: 6000,
  due_date: "2026-08-10T12:00:00.000Z",
  paid_at: "2026-08-10T12:30:00.000Z",
  paid_amount: 6000,
  payment_method: "pix",
  status: "PAID",
  line_items: [
    {
      id: "item-rent",
      description: "Aluguel",
      amount_cents: 500000,
      source_type: "rental_base",
      source_id: "cfg-palmeiras",
      economic_owner_id: "party-marisa",
      settlement_mode: "arrecadacao",
      rule_version_id: "rule-rent-v1",
    },
    {
      id: "item-condo",
      description: "Condomínio",
      amount_cents: 70000,
      source_type: "rental_bundled_item",
      source_id: "condominio",
      economic_owner_id: "party-marisa",
      settlement_mode: "liquidacao_direta",
      rule_version_id: "rule-condo-v1",
    },
    {
      id: "item-iptu",
      description: "IPTU",
      amount_cents: 30000,
      source_type: "rental_bundled_item",
      source_id: "iptu",
      economic_owner_id: "party-marisa",
      settlement_mode: "liquidacao_direta",
      rule_version_id: "rule-iptu-v1",
    },
  ],
  payments: [
    {
      id: "payment-pix-1",
      amount_cents: 600000,
      method: "pix",
      paid_at: "2026-08-10T12:30:00.000Z",
      allocation_method: "proportional_v1",
      allocations: [
        {
          id: "allocation-owner-rent",
          payment_id: "payment-pix-1",
          charge_item_id: "item-rent",
          amount_cents: 460000,
          allocation_type: "owner_payable",
          beneficiary_party_id: "party-marisa",
          rule_version_id: "rule-rent-v1",
        },
        {
          id: "allocation-admin-fee",
          payment_id: "payment-pix-1",
          charge_item_id: "item-rent",
          amount_cents: 40000,
          allocation_type: "revenue",
          beneficiary_party_id: "imobiliaria-estrela",
          rule_version_id: "rule-admin-v1",
        },
        {
          id: "allocation-condo",
          payment_id: "payment-pix-1",
          charge_item_id: "item-condo",
          amount_cents: 70000,
          allocation_type: "owner_payable",
          beneficiary_party_id: "party-marisa",
          rule_version_id: "rule-condo-v1",
        },
        {
          id: "allocation-iptu",
          payment_id: "payment-pix-1",
          charge_item_id: "item-iptu",
          amount_cents: 30000,
          allocation_type: "owner_payable",
          beneficiary_party_id: "party-marisa",
          rule_version_id: "rule-iptu-v1",
        },
      ],
    },
  ],
  created_at: "2026-08-01T12:00:00.000Z",
  updated_at: "2026-08-10T12:30:00.000Z",
};

beforeAll(() => {
  // jsdom não implementa as APIs de ponteiro/scroll que o Radix Dialog usa.
  const proto = Element.prototype as unknown as Record<string, unknown>;
  proto.scrollIntoView ??= vi.fn();
  proto.hasPointerCapture ??= vi.fn(() => false);
  proto.setPointerCapture ??= vi.fn();
  proto.releasePointerCapture ??= vi.fn();
  (globalThis as Record<string, unknown>).ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

function renderDrawer(onClose = vi.fn()) {
  render(
    <MemoryRouter>
      <ChargeDetailDrawer
        charge={caseGuideCharge}
        ownerLabel="Marisa Andrade Ferreira"
        tenantLabel="Pedro Henrique Lima Costa"
        propertyLabel="Rua das Palmeiras, 245, apto 62"
        onClose={onClose}
        onAction={vi.fn()}
      />
    </MemoryRouter>,
  );
  return { onClose };
}

describe("ChargeDetailDrawer", () => {
  beforeEach(() => {
    eligibilityMock.data = { eligible: true, reason: undefined };
    eligibilityMock.calls = [];
    repairMock.mutateAsync.mockClear();
  });

  it("distingue aluguel bruto cobrado e líquido econômico quando a taxa está incluída", () => {
    const charge: Charge = {
      ...caseGuideCharge, amount: 4128, total_amount: 4128, payments: [], paid_amount: undefined, status: "PENDING",
      source_event_id: "rental.cfg.month.2026-10.r1",
      line_items: [
        { ...caseGuideCharge.line_items![0], amount_cents: 288000, rule_version_id: "same-rule" },
        { id: "admin", description: "Taxa de administração", amount_cents: 32000, source_type: "rental_bundled_item", source_id: "admin_fee", economic_owner_id: caseGuideCharge.tenant_id, settlement_mode: "arrecadacao", allocation_type: "revenue", rule_version_id: "same-rule" },
        { ...caseGuideCharge.line_items![1], amount_cents: 65000 },
        { ...caseGuideCharge.line_items![2], amount_cents: 18000 },
        { description: "Seguro Fiança", amount_cents: 9800 },
      ],
    };
    render(<MemoryRouter><ChargeDetailDrawer charge={charge} onClose={vi.fn()} onAction={vi.fn()} /></MemoryRouter>);
    expect(screen.getByText("Composição da cobrança")).toBeInTheDocument();
    expect(screen.getByText("R$ 3.200,00")).toBeInTheDocument();
    expect(screen.getByText("Aluguel líquido do proprietário")).toBeInTheDocument();
    expect(screen.getByText(/A taxa de administração de R\$ 320,00 está incluída no aluguel/)).toBeInTheDocument();
  });

  it("expõe titularidade, modo de liquidação e composição alocada do caso-guia", () => {
    render(
      <MemoryRouter>
        <ChargeDetailDrawer
          charge={caseGuideCharge}
          ownerLabel="Marisa Andrade Ferreira"
          tenantLabel="Pedro Henrique Lima Costa"
          propertyLabel="Rua das Palmeiras, 245, apto 62"
          onClose={vi.fn()}
          onAction={vi.fn()}
        />
      </MemoryRouter>,
    );

    expect(screen.getByText("Composição econômica")).toBeInTheDocument();
    expect(screen.getByText("Arrecadação")).toBeInTheDocument();
    expect(screen.getAllByText("Liquidação direta")).toHaveLength(2);
    expect(screen.getAllByText(/Titular econômico:/)).toHaveLength(3);
    expect(screen.getAllByText("Marisa Andrade Ferreira").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Pagamentos e composição alocada (1)")).toBeInTheDocument();
    expect(screen.getByText(/proportional_v1/)).toBeInTheDocument();
    expect(screen.getAllByText(/Repasse ao proprietário/).length).toBeGreaterThanOrEqual(3);
    expect(screen.getByText(/Receita da imobiliária/)).toBeInTheDocument();
    expect(screen.getByText("Alocado: R$ 6.000,00 de R$ 6.000,00")).toBeInTheDocument();
    expect(screen.getAllByText(/Regra rule-/).length).toBeGreaterThanOrEqual(4);
  });

  // FIN-07: o drawer montava overlay artesanal com motion.div, sem papel de
  // diálogo, sem Escape e sem gestão de foco — o teclado não conseguia fechar.
  it("expõe contrato de diálogo com nome acessível", async () => {
    renderDrawer();
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveAttribute("aria-labelledby");
    expect(screen.getByRole("heading", { name: /Detalhes da Cobrança/i })).toBeInTheDocument();
  });

  it("fecha com Escape", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const { onClose } = renderDrawer();
    await screen.findByRole("dialog");

    await user.keyboard("{Escape}");
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it("não renderiza diálogo sem cobrança selecionada", async () => {
    render(
      <MemoryRouter>
        <ChargeDetailDrawer charge={null} onClose={vi.fn()} onAction={vi.fn()} />
      </MemoryRouter>,
    );
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("não oferece o reparo sem a permissão de aprovação de repasse", async () => {
    render(
      <MemoryRouter>
        <PermissionsContext.Provider value={permissionsValue(["finance:write"])}>
          <ChargeDetailDrawer charge={caseGuideCharge} onClose={vi.fn()} onAction={vi.fn()} />
        </PermissionsContext.Provider>
      </MemoryRouter>,
    );
    await screen.findByRole("dialog");
    expect(screen.queryByRole("button", { name: /Reparar alocações/i })).not.toBeInTheDocument();
  });

  it("oferece o reparo em cobrança de locação quitada com permissão", async () => {
    render(
      <MemoryRouter>
        <PermissionsContext.Provider value={permissionsValue(["finance:approve_transfer"])}>
          <ChargeDetailDrawer charge={caseGuideCharge} onClose={vi.fn()} onAction={vi.fn()} />
        </PermissionsContext.Provider>
      </MemoryRouter>,
    );
    expect(await screen.findByRole("button", { name: /Reparar alocações/i })).toBeInTheDocument();
  });

  it("exige motivo e envia o reparo com a justificativa", async () => {
    repairMock.mutateAsync.mockClear();
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <PermissionsContext.Provider value={permissionsValue(["finance:approve_transfer"])}>
          <ChargeDetailDrawer charge={caseGuideCharge} onClose={vi.fn()} onAction={vi.fn()} />
        </PermissionsContext.Provider>
      </MemoryRouter>,
    );
    await user.click(await screen.findByRole("button", { name: /Reparar alocações/i }));

    const confirm = await screen.findByRole("button", { name: /^Reparar alocações$/i });
    // Curto demais: bloqueado (mínimo de 10 caracteres) depois que o campo é
    // tocado — o botão só desabilita com `touched`, para não abrir o diálogo
    // com um CTA desabilitado sem explicação.
    await user.type(screen.getByLabelText(/Motivo/i), "curto");
    await user.tab();
    expect(confirm).toBeDisabled();

    await user.clear(screen.getByLabelText(/Motivo/i));
    await user.type(screen.getByLabelText(/Motivo/i), "baixa anterior ao plano de intermediação");
    await user.click(confirm);

    await waitFor(() => {
      expect(repairMock.mutateAsync).toHaveBeenCalledWith({
        chargeId: "charge-aug-2026",
        reason: "baixa anterior ao plano de intermediação",
      });
    });
  });

  it("não oferece o reparo quando o backend diz que não há o que reparar", async () => {
    eligibilityMock.data = { eligible: false, reason: "intermediação já integralmente financiada" };
    render(
      <MemoryRouter>
        <PermissionsContext.Provider value={permissionsValue(["finance:approve_transfer"])}>
          <ChargeDetailDrawer charge={caseGuideCharge} onClose={vi.fn()} onAction={vi.fn()} />
        </PermissionsContext.Provider>
      </MemoryRouter>,
    );
    await screen.findByRole("dialog");
    expect(screen.queryByRole("button", { name: /Reparar alocações/i })).not.toBeInTheDocument();
  });

  it("não oferece o reparo para cobrança que não é de locação quitada", async () => {
    const pendingSale: Charge = { ...caseGuideCharge, type: "SALE", status: "PENDING" };
    render(
      <MemoryRouter>
        <PermissionsContext.Provider value={permissionsValue(["finance:approve_transfer"])}>
          <ChargeDetailDrawer charge={pendingSale} onClose={vi.fn()} onAction={vi.fn()} />
        </PermissionsContext.Provider>
      </MemoryRouter>,
    );
    await screen.findByRole("dialog");
    expect(screen.queryByRole("button", { name: /Reparar alocações/i })).not.toBeInTheDocument();
    // Cobrança fora das pré-condições nem consulta a elegibilidade.
    expect(eligibilityMock.calls.every((call) => call.enabled === false)).toBe(true);
  });

  it("rotula a taxa da imobiliária como intermediação em venda", async () => {
    const sale: Charge = { ...caseGuideCharge, type: "SALE" };
    render(
      <MemoryRouter>
        <ChargeDetailDrawer charge={sale} onClose={vi.fn()} onAction={vi.fn()} />
      </MemoryRouter>,
    );
    await screen.findByRole("dialog");
    expect(screen.getByText("Total de Intermediação da Venda")).toBeInTheDocument();
    expect(screen.getByText("Comissão do Corretor (Venda)")).toBeInTheDocument();
    expect(screen.getByText("Repasse ao Vendedor")).toBeInTheDocument();
    expect(screen.getByText("Comprador")).toBeInTheDocument();
    expect(screen.getByText("Vendedor")).toBeInTheDocument();
    expect(screen.queryByText("Para a imobiliária (taxa de admin.)")).not.toBeInTheDocument();
    expect(screen.queryByText("Para o corretor (intermediação)")).not.toBeInTheDocument();
  });

  it("mantém a taxonomia de administração e repasse ao proprietário na locação", async () => {
    render(
      <MemoryRouter>
        <ChargeDetailDrawer charge={caseGuideCharge} onClose={vi.fn()} onAction={vi.fn()} />
      </MemoryRouter>,
    );
    await screen.findByRole("dialog");
    expect(screen.getByText("Taxa de Administração (Locação)")).toBeInTheDocument();
    expect(screen.getByText("Comissão do Corretor (Locação)")).toBeInTheDocument();
    expect(screen.getByText("Repasse ao Proprietário")).toBeInTheDocument();
    expect(screen.getByText("Inquilino")).toBeInTheDocument();
  });
});
