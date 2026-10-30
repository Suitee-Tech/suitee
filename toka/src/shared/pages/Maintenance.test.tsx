import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  tickets: [] as unknown[],
  properties: [
    {
      id: "pr-1",
      displayId: "IM-101",
      title: "Apartamento dos Ipês",
      ownerId: "owner-1",
      owners: [{ id: "owner-1", percentage: 100 }],
      address: {},
      price: 0,
      type: "Apartamento",
      status: "Alugado",
      beds: 2,
      baths: 1,
      area: 60,
      image: "",
    },
    {
      id: "pr-2",
      displayId: "IM-102",
      title: "Casa Rua Itapetinga",
      ownerId: "owner-2",
      owners: [{ id: "owner-2", percentage: 100 }],
      address: {},
      price: 0,
      type: "Casa",
      status: "Alugado",
      beds: 3,
      baths: 2,
      area: 120,
      image: "",
    },
  ],
  contracts: [
    {
      id: "ct-1",
      display_id: "CTR-2026-014",
      type: "locacao",
      status: "SIGNED",
      property_id: "pr-1",
      client: "Vanessa Rodrigues Melo",
      customer_id: "tenant-1",
      content: "",
      signatories: [],
      created_at: "2026-09-01T10:00:00Z",
      updated_at: "2026-09-01T10:00:00Z",
      tenant_id: "company-1",
    },
  ],
  leads: [
    { id: "tenant-1", name: "Vanessa Rodrigues Melo", email: "v@x.com", isClient: true },
    { id: "owner-1", name: "João Proprietário", email: "j@x.com", isClient: false },
    { id: "owner-2", name: "Maria Proprietária", email: "m@x.com", isClient: false },
    { id: "lead-9", name: "Cliente Avulso", email: "a@x.com", isClient: true },
  ],
  addTicket: vi.fn(),
}));

vi.mock("../context/MaintenanceContext", () => ({
  useMaintenanceContext: () => ({
    tickets: mocks.tickets,
    isTicketsLoading: false,
    ticketsError: null,
    reloadTickets: vi.fn(),
    addTicket: mocks.addTicket,
    updateTicket: vi.fn(),
  }),
}));
vi.mock("../context/PropertiesContext", () => ({
  usePropertiesContext: () => ({ properties: mocks.properties }),
}));
vi.mock("../context/ContractsContext", () => ({
  useContractsContext: () => ({ contracts: mocks.contracts }),
}));
vi.mock("../context/LeadsContext", () => ({
  useLeadsContext: () => ({ leads: mocks.leads }),
}));
vi.mock("../context/FocusModeContext", () => ({ useFocusMode: () => ({ focusMode: false }) }));
vi.mock("../ui/use-mobile", () => ({ useIsMobile: () => false }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("../services/charge.service", () => ({
  chargeService: { getAll: vi.fn(async () => ({ data: billingMocks.charges, meta: {} })) },
}));
vi.mock("../services/transfer.service", () => ({
  transferService: { getAll: vi.fn(async () => ({ data: billingMocks.transfers, meta: {} })) },
}));
vi.mock("../services/maintenance.service", () => ({
  maintenanceService: { update: billingMocks.update, changeStatus: billingMocks.changeStatus },
}));

import { Maintenance } from "./Maintenance";

const billingMocks = vi.hoisted(() => ({
  charges: [] as unknown[],
  transfers: [] as unknown[],
  update: vi.fn().mockResolvedValue({ data: {} }),
  changeStatus: vi.fn().mockResolvedValue({ data: {} }),
}));

// cmdk (popover de busca) usa ResizeObserver, ausente no jsdom.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal("ResizeObserver", ResizeObserverStub);
// cmdk rola o item ativo para a viewport.
Element.prototype.scrollIntoView = vi.fn();
vi.stubGlobal("DOMRect", class {
  constructor() {
    return { x: 0, y: 0, width: 0, height: 0, top: 0, left: 0, right: 0, bottom: 0 };
  }
});

function openNewTicketModal() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <Maintenance />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: /Nova Solicitação/ }));
}

/** Popover de contrato: campo é um botão com o texto "Opcional". */
function selectContract() {
  fireEvent.click(screen.getByRole("button", { name: /Opcional/ }));
  fireEvent.click(screen.getByText("CTR-2026-014 · Vanessa Rodrigues Melo · Apartamento dos Ipês"));
}

/** Combobox do imóvel (PropertyPicker). */
function propertyCombobox(): HTMLElement {
  const all = screen.getAllByRole("combobox");
  const element = all.find((node) => /imóvel/i.test(node.textContent ?? ""));
  if (!element) {
    console.log("COMBOBOXES:", all.map((n) => `${n.tagName}:${(n.textContent ?? "").slice(0, 30)}`));
    throw new Error("Combobox de imóvel não encontrado");
  }
  return element;
}

function openRequesterList() {
  const trigger = screen
    .getAllByRole("button")
    .find((element) => /solicitante|parte do contrato/i.test(element.textContent ?? ""));
  if (!trigger) {
    console.log("BOTÕES:", screen.getAllByRole("button").map((b) => b.textContent?.slice(0, 40)));
    throw new Error("Campo de solicitante não encontrado");
  }
  fireEvent.click(trigger);
}

afterEach(cleanup);

const OPEN_PANEL_KEY = "maintenance:open-panel";

beforeEach(() => {
  vi.clearAllMocks();
  mocks.tickets = [];
  window.sessionStorage.clear();
});

afterEach(() => {
  window.sessionStorage.clear();
});

describe("Nova Solicitação — cascata contrato → imóvel e partes", () => {
  it("trava o imóvel no imóvel do contrato escolhido", () => {
    openNewTicketModal();

    // Sem contrato: o imóvel é escolhido livremente.
    expect(screen.getByText("Selecione um imóvel...")).toBeInTheDocument();

    selectContract();

    const locked = screen.getByText("IM-101 - Apartamento dos Ipês");
    expect(locked).toBeInTheDocument();
    expect(screen.getByText("do contrato")).toBeInTheDocument();
    expect(screen.queryByText("Selecione um imóvel...")).not.toBeInTheDocument();
  });

  it("limita as partes às do contrato, sem oferecer clientes de fora", () => {
    openNewTicketModal();
    selectContract();

    openRequesterList();

    // Perfil "Inquilino": a parte é o locatário do contrato.
    expect(screen.getByText("Vanessa Rodrigues Melo")).toBeInTheDocument();
    // Clientes e proprietários de fora do contrato não aparecem.
    expect(screen.queryByText("Cliente Avulso")).not.toBeInTheDocument();
    expect(screen.queryByText("Maria Proprietária")).not.toBeInTheDocument();
    expect(screen.queryByText("João Proprietário")).not.toBeInTheDocument();
  });

  it("ao trocar o perfil para Proprietário, oferece o dono do imóvel", () => {
    openNewTicketModal();
    selectContract();

    fireEvent.change(screen.getByDisplayValue("Inquilino"), { target: { value: "Proprietário" } });
    openRequesterList();

    expect(screen.getByText("João Proprietário")).toBeInTheDocument();
    expect(screen.queryByText("Vanessa Rodrigues Melo")).not.toBeInTheDocument();
  });

  /** Abre a janela de lançamento do único chamado da lista. */
  function openBillingDrawer() {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
    mocks.tickets = [
      {
        id: "os-1",
        tenant_id: "company-1",
        type: "Manutenção",
        category: "Elétrica",
        property_id: "pr-1",
        property_title: "IM-101 - Apartamento dos Ipês",
        contract_id: "ct-1",
        requester_lead_id: "tenant-1",
        requester: "Vanessa Rodrigues Melo",
        profile: "Inquilino",
        status: "Em Análise",
        priority: "Média",
        created_at: "2026-09-18T10:00:00Z",
        updated_at: "2026-09-18T10:00:00Z",
      },
    ];
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <Maintenance />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    fireEvent.click(screen.getByTitle("Financeiro / Gerar Cobrança"));
  }

  it("permite escolher entre a próxima cobrança e a cobrança avulsa", async () => {
    billingMocks.charges = [
      {
        id: "ch-1",
        contract_id: "ct-1",
        reference_month: "2026-10",
        amount: 4128,
        total_amount: 4128,
        due_date: "2026-10-10",
        status: "PENDING",
        line_items: [{ id: "li-1", description: "Aluguel", amount_cents: 371520, source_type: "rental_base", source_id: "cfg-1" }],
      },
    ];
    billingMocks.transfers = [];
    openBillingDrawer();

    expect(await screen.findByText("Somar na próxima cobrança do inquilino")).toBeInTheDocument();
    expect(screen.getByText("Cobrança avulsa ao inquilino")).toBeInTheDocument();
    // Sem repasse pendente a opção de repasse nem é oferecida ao inquilino.
    expect(screen.queryByText("Descontar do próximo repasse")).not.toBeInTheDocument();
  });

  it("bloqueia a programação no repasse quando o contrato não tem repasse pendente", async () => {
    billingMocks.charges = [
      {
        id: "ch-1",
        contract_id: "ct-1",
        reference_month: "2026-10",
        amount: 4128,
        total_amount: 4128,
        due_date: "2026-10-10",
        status: "PENDING",
        line_items: [{ id: "li-1", description: "Aluguel", amount_cents: 371520, source_type: "rental_base", source_id: "cfg-1" }],
      },
    ];
    // Contrato encerrado: nenhum repasse pendente.
    billingMocks.transfers = [];
    openBillingDrawer();

    fireEvent.click(screen.getByText("Descontar do Proprietário"));

    const transferOption = await screen.findByText("Descontar do próximo repasse");
    const optionRow = transferOption.closest("label") as HTMLElement;
    expect(optionRow).toHaveAttribute("aria-disabled", "true");
    expect(within(optionRow).getByRole("radio")).toBeDisabled();
    expect(
      within(optionRow).getByText(/não tem repasse pendente/i),
    ).toBeInTheDocument();
    // A avulsa continua disponível e é o destino assumido assim que a
    // consulta do contrato termina.
    const standalone = screen.getByRole("radio", { name: /Cobrança avulsa ao proprietário/i });
    await waitFor(() => expect(standalone).toBeEnabled());
    await waitFor(() => expect(standalone).toBeChecked());
  });

  it("oferece o repasse pendente e mostra a competência antes de confirmar", async () => {
    billingMocks.charges = [];
    billingMocks.transfers = [{ id: "tr-1", contract_id: "ct-1", reference_month: "2026-10", status: "PENDING" }];
    openBillingDrawer();

    fireEvent.click(screen.getByText("Descontar do Proprietário"));

    const option = await screen.findByText("Descontar do próximo repasse");
    const optionRow = option.closest("label") as HTMLElement;
    await waitFor(() => expect(within(optionRow).getByRole("radio")).toBeEnabled());
    expect(within(optionRow).getByText(/Abate no repasse de/)).toBeInTheDocument();
    // Sem cobrança em aberto, a única alternativa do proprietário é a avulsa.
    expect(screen.getByText("Cobrança avulsa ao proprietário")).toBeInTheDocument();
    expect(screen.queryByText("Somar na próxima cobrança do inquilino")).not.toBeInTheDocument();
  });

  it("avisa que não há lançamento quando quem pagou o serviço é quem assume o custo", async () => {
    billingMocks.charges = [
      {
        id: "ch-1",
        contract_id: "ct-1",
        reference_month: "2026-10",
        amount: 4128,
        total_amount: 4128,
        due_date: "2026-10-10",
        status: "PENDING",
        line_items: [{ id: "li-1", description: "Aluguel", amount_cents: 371520, source_type: "rental_base", source_id: "cfg-1" }],
      },
    ];
    billingMocks.transfers = [];
    openBillingDrawer();

    // Inquilino assume o custo (padrão da responsabilidade financeira) e
    // também pagou o serviço: nada passa pelo financeiro da imobiliária.
    const paidBy = screen.getByRole("radio", { name: "Inquilino" });
    fireEvent.click(paidBy);
    expect(paidBy).toBeChecked();

    await waitFor(() =>
      expect(screen.getByText(/nada é cobrado nem descontado/i)).toBeInTheDocument(),
    );
  });

  it("envia quem pagou o serviço junto da política", async () => {
    billingMocks.charges = [
      {
        id: "ch-1",
        contract_id: "ct-1",
        reference_month: "2026-10",
        amount: 4128,
        total_amount: 4128,
        due_date: "2026-10-10",
        status: "PENDING",
        line_items: [{ id: "li-1", description: "Aluguel", amount_cents: 371520, source_type: "rental_base", source_id: "cfg-1" }],
      },
    ];
    billingMocks.transfers = [];
    openBillingDrawer();

    fireEvent.change(screen.getByPlaceholderText("0,00"), { target: { value: "13000" } });
    fireEvent.click(screen.getByRole("button", { name: /Lançar no Financeiro/i }));

    await waitFor(() => expect(billingMocks.update).toHaveBeenCalled());
    expect(billingMocks.update).toHaveBeenCalledWith(
      "os-1",
      expect.objectContaining({ advanced_by: "imobiliaria", billing_target: "next_charge", payer_role: "tenant" }),
    );
  });

  it("recusa o repasse quando o valor supera o que ele absorve", async () => {
    billingMocks.charges = [];
    billingMocks.transfers = [
      {
        id: "tr-1",
        contract_id: "ct-1",
        reference_month: "2026-10",
        status: "PENDING",
        net_amount: 200,
        owner_balance: { total_cents: 20000, reserved_cents: 0, blocked_cents: 0, available_cents: 20000 },
      },
    ];
    openBillingDrawer();

    fireEvent.click(screen.getByText("Descontar do Proprietário"));

    const option = await screen.findByText("Descontar do próximo repasse");
    const optionRow = option.closest("label") as HTMLElement;
    // Espera a consulta do contrato assentar antes de digitar o valor.
    await waitFor(() => expect(within(optionRow).getByText(/Abate no repasse/)).toBeInTheDocument());

    // MoneyInput lê os dígitos como centavos: 50000 = R$ 500,00 > R$ 200,00.
    fireEvent.change(screen.getByPlaceholderText("0,00"), { target: { value: "50000" } });

    await waitFor(() => expect(within(optionRow).getByRole("radio")).toBeDisabled());
    expect(within(optionRow).getByText(/absorve até R\$ 200,00/)).toBeInTheDocument();
    // A avulsa assume e continua habilitada.
    const standalone = screen.getByRole("radio", { name: /Cobrança avulsa ao proprietário/i });
    await waitFor(() => expect(standalone).toBeEnabled());
  });

  it("recusa a próxima cobrança quando ela não tem composição", async () => {
    billingMocks.charges = [
      { id: "ch-legacy", contract_id: "ct-1", reference_month: "2026-10", amount: 4128, total_amount: 4128, due_date: "2026-10-10", status: "PENDING" },
    ];
    billingMocks.transfers = [];
    openBillingDrawer();

    const option = await screen.findByText("Somar na próxima cobrança do inquilino");
    const optionRow = option.closest("label") as HTMLElement;
    await waitFor(() => expect(within(optionRow).getByRole("radio")).toBeDisabled());
    expect(within(optionRow).getByText(/não tem composição definida/i)).toBeInTheDocument();
  });

  it("descreve a cobrança avulsa no destino escolhido", async () => {
    billingMocks.charges = [];
    billingMocks.transfers = [];
    openBillingDrawer();

    fireEvent.change(screen.getByPlaceholderText("0,00"), { target: { value: "130" } });
    await waitFor(() => expect(screen.getByText(/Cobrança avulsa de/)).toBeInTheDocument());
  });

  it("bloqueia a próxima cobrança quando o contrato não tem cobrança em aberto", async () => {
    billingMocks.charges = [];
    billingMocks.transfers = [];
    openBillingDrawer();

    const chargeOption = await screen.findByText("Somar na próxima cobrança do inquilino");
    const optionRow = chargeOption.closest("label") as HTMLElement;
    expect(optionRow).toHaveAttribute("aria-disabled", "true");
    expect(within(optionRow).getByRole("radio")).toBeDisabled();
    expect(within(optionRow).getByText(/não tem cobrança em aberto/i)).toBeInTheDocument();
  });

  it("mostra os proprietários do imóvel escolhido quando não há contrato", () => {
    openNewTicketModal();

    fireEvent.click(propertyCombobox());
    fireEvent.click(screen.getByText("Casa Rua Itapetinga"));

    expect(
      screen.getByText(
        (_text, node) => node?.tagName === "P" && node.textContent?.trim() === "Proprietário: Maria Proprietária",
      ),
    ).toBeInTheDocument();
  });

  it("envia contrato, parte responsável e imóvel do contrato", async () => {
    mocks.addTicket.mockResolvedValue(true);
    openNewTicketModal();
    selectContract();

    fireEvent.change(screen.getByPlaceholderText(/Ex: Elétrica/), { target: { value: "Elétrica" } });
    openRequesterList();
    fireEvent.click(screen.getByText("Vanessa Rodrigues Melo"));

    fireEvent.click(screen.getByRole("button", { name: /Criar|Abrir|Salvar|Solicitar/ }));

    expect(mocks.addTicket).toHaveBeenCalledWith(
      expect.objectContaining({
        contract_id: "ct-1",
        requester_lead_id: "tenant-1",
        property_id: "pr-1",
      }),
    );
  });
});

describe("Painel aberto resiste ao refresh global de dados", () => {
  it("restaura o agendamento em andamento quando a página remonta", async () => {
    // Qualquer mutação na aplicação remonta a árvore da rota. Sem o rascunho,
    // o operador perdia o painel e o que já tinha preenchido.
    mocks.tickets = [
      {
        id: "os-1",
        tenant_id: "company-1",
        type: "Manutenção",
        category: "Elétrica",
        property_id: "pr-1",
        property_title: "IM-101 - Apartamento dos Ipês",
        contract_id: "ct-1",
        requester: "Vanessa Rodrigues Melo",
        profile: "Inquilino",
        status: "Em Análise",
        priority: "Média",
        created_at: "2026-09-18T10:00:00Z",
        updated_at: "2026-09-18T10:00:00Z",
      },
    ];
    window.sessionStorage.setItem(
      OPEN_PANEL_KEY,
      JSON.stringify({
        activeModal: "agendamento",
        ticket: mocks.tickets[0],
        schedule: { supplierId: "sp-1", date: "2026-10-05", time: "14:00", instructions: "Levar escada" },
      }),
    );

    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <Maintenance />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    // O painel volta aberto, com data, horário e instruções preservados.
    expect(await screen.findByText("Gestão da Locação")).toBeInTheDocument();
    expect(screen.getByDisplayValue("2026-10-05")).toBeInTheDocument();
    expect(screen.getByDisplayValue("14:00")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Levar escada")).toBeInTheDocument();
  });

  it("não reabre painel nenhum quando não há rascunho", async () => {
    mocks.tickets = [];
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <Maintenance />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    await waitFor(() => expect(screen.queryByText("Gestão da Locação")).not.toBeInTheDocument());
  });
});
