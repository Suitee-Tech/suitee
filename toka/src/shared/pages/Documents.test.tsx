import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  documents: [] as unknown[],
  viewPrefs: {
    viewMode: "list",
    pinnedFolders: [],
    groupBy: "origin",
    sort: "recent",
    showEmptyFolders: false,
  },
  getAll: vi.fn(),
  listKnownTags: vi.fn(),
  listCompanyMembers: vi.fn(),
  deleteDocument: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("../context/DocumentsContext", () => ({
  useDocumentsContext: () => ({ deleteDocument: mocks.deleteDocument }),
}));
vi.mock("../context/AuthContext", () => ({
  useAuth: () => ({ user: { current_company_id: "tenant-1" } }),
}));
vi.mock("../context/FocusModeContext", () => ({ useFocusMode: () => ({ focusMode: false }) }));
vi.mock("../context/PropertiesContext", () => ({
  usePropertiesContext: () => ({ properties: [{ id: "pr-1", title: "Casa Rua Itapetinga", displayId: "IM-88" }] }),
}));
vi.mock("../context/LeadsContext", () => ({
  useLeadsContext: () => ({ leads: [{ id: "ld-1", name: "Lucas Mendes Barros" }] }),
}));
vi.mock("../context/ContractsContext", () => ({
  useContractsContext: () => ({
    contracts: [
      { id: "ct-1", display_id: "CTR-1", client: "Vanessa Rodrigues Melo", property_title: "Apartamento dos Ipê" },
    ],
  }),
}));
vi.mock("../context/DealsContext", () => ({ useDealsContext: () => ({ dealDTOs: [] }) }));
vi.mock("../context/InspectionsContext", () => ({ useInspectionsContext: () => ({ inspections: [] }) }));
vi.mock("../context/MaintenanceContext", () => ({ useMaintenanceContext: () => ({ tickets: [] }) }));
vi.mock("../context/AgentsContext", () => ({ useAgentsContext: () => ({ agents: [] }) }));
vi.mock("../context/ViewPreferencesContext", () => ({
  useViewPreferences: () => [mocks.viewPrefs, vi.fn()],
}));
vi.mock("../services/settings.service", () => ({
  settingsService: { listCompanyMembers: mocks.listCompanyMembers },
}));
vi.mock("../services/document.service", () => ({
  documentService: {
    getAll: mocks.getAll,
    listKnownTags: mocks.listKnownTags,
    getById: vi.fn(),
    download: vi.fn(),
    getPresignedDownloadUrl: vi.fn(),
    update: vi.fn(),
  },
}));
vi.mock("sonner", () => ({ toast: { error: mocks.toastError, success: vi.fn() } }));
vi.mock("../components/AuditTrailPanel", () => ({ AuditTrailPanel: () => null }));
vi.mock("../components/DocumentViewerDialog", () => ({ DocumentViewerDialog: () => null }));
vi.mock("../components/documents/DocumentGridCard", () => ({
  DocumentGridCard: ({ doc, reference, originLabel }: { doc: { name: string }; reference?: string; originLabel?: string }) => (
    <div data-testid="grid-card">
      <span>{doc.name}</span>
      <span>{reference}</span>
      <span>{originLabel}</span>
    </div>
  ),
}));

import { Documents } from "./Documents";

interface DtoOverrides {
  [key: string]: unknown;
}

function makeDto(overrides: DtoOverrides = {}) {
  return {
    id: "doc-1",
    tenant_id: "tenant-1",
    name: "Contrato assinado — locação",
    file_type: "application/pdf",
    file_size: 524_000,
    category: "contrato",
    status: "assinado",
    source: "toca-deal",
    uploaded_by: "Bruno Exemplo",
    upload_date: "2026-09-17T12:00:00Z",
    signers: [],
    tags: [],
    version: 1,
    created_at: "2026-09-17T12:00:00Z",
    updated_at: "2026-09-17T12:00:00Z",
    ...overrides,
  };
}

const DOCUMENTS = [
  makeDto({ id: "1", contract_id: "ct-1", name: "Contrato assinado — locação" }),
  makeDto({
    id: "2",
    name: "extrato-a8f46eb1.pdf",
    category: "comprovante",
    status: "pendente_assinatura",
    entity_id: "pr-1",
    entity_type: "property",
  }),
  makeDto({
    id: "3",
    name: "Captura de Tela 2026-09-04.png",
    category: "fiscal",
    status: "pendente_assinatura",
    entity_id: "ld-1",
    entity_type: "lead",
    upload_date: "2026-09-16T12:00:00Z",
  }),
  makeDto({
    id: "4",
    name: "Documento avulso.pdf",
    category: "outros",
    status: "rascunho",
    source: "manual",
    uploaded_by: "9feafe7e-cbc4-94f4-9b8f-cc7a9c1c3bcc",
    upload_date: "2026-08-01T12:00:00Z",
  }),
  // Arquivo de apoio: o backend grava status "pendente" em todo upload, mas
  // uma foto de fachada não passa por fluxo de assinatura nenhum.
  makeDto({
    id: "5",
    name: "Fachada do imóvel.jpg",
    category: "foto",
    status: "pendente",
    entity_id: "pr-1",
    entity_type: "property",
    uploaded_by: "bruno@imob.com",
    upload_date: "2026-09-15T12:00:00Z",
  }),
];

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <MemoryRouter>{children}</MemoryRouter>
    </QueryClientProvider>
  );
  return render(<Documents />, { wrapper });
}

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(mocks.viewPrefs, {
    viewMode: "list",
    pinnedFolders: [],
    groupBy: "origin",
    sort: "recent",
    showEmptyFolders: false,
  });
  mocks.getAll.mockResolvedValue({ data: DOCUMENTS, meta: { total: DOCUMENTS.length } });
  mocks.listKnownTags.mockResolvedValue(["due-diligence"]);
  mocks.listCompanyMembers.mockResolvedValue([
    { account_id: "9feafe7e-cbc4-94f4-9b8f-cc7a9c1c3bcc", name: "Bruno Exemplo", email: "bruno@imob.com" },
  ]);
  mocks.deleteDocument.mockResolvedValue(true);
});

afterEach(cleanup);

describe("Central de Documentos — drive por origem", () => {
  it("organiza o acervo por origem com nomes reais em vez de UUIDs", async () => {
    renderPage();

    expect(await screen.findByText("Contrato assinado — locação")).toBeInTheDocument();
    expect(screen.getByText("Vanessa Rodrigues Melo · Apartamento dos Ipê")).toBeInTheDocument();
    // Dois documentos (extrato + foto) apontam para o mesmo imóvel.
    expect(screen.getAllByText("Casa Rua Itapetinga").length).toBe(2);
    expect(screen.getByText("Lucas Mendes Barros")).toBeInTheDocument();

    // Nunca expõe o id cru do responsável nem um separador vazio.
    expect(screen.queryByText(/9feafe7e/)).not.toBeInTheDocument();
    expect(screen.getAllByText(/Enviado por Bruno Exemplo/).length).toBeGreaterThan(0);
  });

  it("mostra a contagem por origem e por coleção na barra lateral", async () => {
    renderPage();

    await screen.findByText("Contrato assinado — locação");

    const sidebar = screen.getByRole("navigation");
    const contractsRow = within(sidebar).getByText("Contratos").closest("button") as HTMLElement;
    expect(within(contractsRow).getByText("1")).toBeInTheDocument();

    const unlinkedRow = within(sidebar).getByText("Sem vínculo").closest("button") as HTMLElement;
    expect(within(unlinkedRow).getByText("1")).toBeInTheDocument();

    const waitingRow = within(sidebar).getByText("Aguardando assinatura").closest("button") as HTMLElement;
    expect(within(waitingRow).getByText("2")).toBeInTheDocument();
  });

  it("filtra o acervo pela origem escolhida na barra lateral", async () => {
    renderPage();
    await screen.findByText("Contrato assinado — locação");

    fireEvent.click(within(screen.getByRole("navigation")).getByText("Imóveis"));

    await waitFor(() => {
      expect(screen.getByText("extrato-a8f46eb1.pdf")).toBeInTheDocument();
      expect(screen.queryByText("Contrato assinado — locação")).not.toBeInTheDocument();
    });
  });

  it("busca por cliente, imóvel e responsável, não só pelo nome do arquivo", async () => {
    renderPage();
    await screen.findByText("Contrato assinado — locação");

    const search = screen.getByLabelText("Buscar documentos");

    fireEvent.change(search, { target: { value: "vanessa" } });
    await waitFor(() => {
      expect(screen.getByText("Contrato assinado — locação")).toBeInTheDocument();
      expect(screen.queryByText("Captura de Tela 2026-09-04.png")).not.toBeInTheDocument();
    });

    fireEvent.change(search, { target: { value: "itapetinga" } });
    await waitFor(() => {
      expect(screen.getByText("extrato-a8f46eb1.pdf")).toBeInTheDocument();
      expect(screen.queryByText("Contrato assinado — locação")).not.toBeInTheDocument();
    });
  });

  it("agrupa o resultado em seções nomeadas pela origem", async () => {
    renderPage();

    expect(await screen.findByText("Contrato assinado — locação")).toBeInTheDocument();
    // Barra lateral + cabeçalho de seção + (quando ativo) chip do filtro.
    expect(screen.getAllByText("Contratos").length).toBeGreaterThanOrEqual(2);
    expect(screen.getAllByText("Imóveis").length).toBeGreaterThanOrEqual(2);
    expect(screen.getAllByText("Clientes").length).toBeGreaterThanOrEqual(2);
    // Cada seção declara quantos documentos entrega (4 origens; Imóveis com 2).
    expect(screen.getAllByText("1 documento").length).toBe(3);
    expect(screen.getByText("2 documentos")).toBeInTheDocument();
    expect(screen.getAllByText("Sem vínculo").length).toBeGreaterThanOrEqual(2);
  });

  it("avisa quando o resultado está vazio por causa do filtro e permite limpar", async () => {
    renderPage();
    await screen.findByText("Contrato assinado — locação");

    fireEvent.change(screen.getByLabelText("Buscar documentos"), { target: { value: "inexistente-xyz" } });

    expect(await screen.findByText("Nenhum documento neste recorte")).toBeInTheDocument();
    fireEvent.click(screen.getAllByRole("button", { name: /Limpar filtros/ })[0]);

    expect(await screen.findByText("Contrato assinado — locação")).toBeInTheDocument();
  });

  it("não rotula arquivo de apoio como aguardando assinatura", async () => {
    renderPage();
    await screen.findByText("Fachada do imóvel.jpg");

    // A foto aparece com o rótulo correto e NÃO entra em "Aguardando assinatura".
    const sidebar = screen.getByRole("navigation");
    const notRequiredRow = within(sidebar).getByText("Não exige assinatura").closest("button") as HTMLElement;
    expect(within(notRequiredRow).getByText("1")).toBeInTheDocument();

    const awaitingRow = within(sidebar).getByText("Aguardando assinatura").closest("button") as HTMLElement;
    expect(within(awaitingRow).getByText("2")).toBeInTheDocument();

    fireEvent.click(within(sidebar).getByText("Não exige assinatura"));

    await waitFor(() => {
      expect(screen.getByText("Fachada do imóvel.jpg")).toBeInTheDocument();
      expect(screen.queryByText("extrato-a8f46eb1.pdf")).not.toBeInTheDocument();
      expect(screen.queryByText("Contrato assinado — locação")).not.toBeInTheDocument();
    });
  });

  it("separa o contrato enviado para assinatura do contrato que ninguém enviou", async () => {
    mocks.getAll.mockResolvedValue({
      data: [
        ...DOCUMENTS,
        makeDto({ id: "6", name: "Contrato de venda (minuta).docx", category: "contrato", status: "pendente" }),
      ],
      meta: { total: DOCUMENTS.length + 1 },
    });

    renderPage();
    await screen.findByText("Contrato de venda (minuta).docx");

    const sidebar = screen.getByRole("navigation");
    const notStartedRow = within(sidebar).getByText("Assinatura não iniciada").closest("button") as HTMLElement;
    expect(within(notStartedRow).getByText("1")).toBeInTheDocument();

    fireEvent.click(within(sidebar).getByText("Assinatura não iniciada"));

    await waitFor(() => {
      expect(screen.getByText("Contrato de venda (minuta).docx")).toBeInTheDocument();
      expect(screen.queryByText("Contrato assinado — locação")).not.toBeInTheDocument();
    });
    expect(screen.getAllByText("Assinatura não iniciada").length).toBeGreaterThanOrEqual(2);
  });

  it("esconde pastas vazias e mantém a origem sempre com o vínculo explícito", async () => {
    renderPage();
    await screen.findByText("Contrato assinado — locação");

    const sidebar = screen.getByRole("navigation");
    expect(within(sidebar).queryByText("Manutenção")).not.toBeInTheDocument();
    expect(within(sidebar).queryByText("Vistorias")).not.toBeInTheDocument();
    expect(within(sidebar).getByText("Mostrar pastas vazias")).toBeInTheDocument();
  });
});

it("busca documentos além do antigo limite de mil registros", async () => {
  const docs = Array.from({ length: 1001 }, (_, index) => makeDto({ id: `doc-${index}`, name: index === 1000 ? 'Arquivo histórico localizado' : `Arquivo ${index}` }));
  mocks.getAll.mockImplementation(({ limit, offset }: { limit: number; offset: number }) => Promise.resolve({ data: docs.slice(offset, offset + limit), meta: { total: docs.length } }));
  renderPage();
  fireEvent.change(screen.getByPlaceholderText(/Buscar/), { target: { value: 'Arquivo histórico localizado' } });
  expect(await screen.findByText('Arquivo histórico localizado')).toBeInTheDocument();
  expect(mocks.getAll).toHaveBeenLastCalledWith({ limit: 100, offset: 1000 });
});

it("mostra falha de paginação e permite recarregar sem apresentar acervo parcial", async () => {
  mocks.getAll.mockRejectedValueOnce(new Error('offline'));
  renderPage();
  expect(await screen.findByRole('alert')).toHaveTextContent('acervo completo');
  expect(screen.queryByText('Nenhum documento na central')).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Tentar novamente' }));
  expect(await screen.findByText('Contrato assinado — locação')).toBeInTheDocument();
});
