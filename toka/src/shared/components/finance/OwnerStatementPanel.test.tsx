import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import {
  ownerStatementService,
  type MonthlyStatement,
  type OwnerBalanceSummary,
} from "../../services/ownerStatement.service";
import { OwnerStatementPanel } from "./OwnerStatementPanel";

const statement: MonthlyStatement = {
  tenant_id: "imobiliaria-estrela",
  owner_lead_id: "lead-marisa",
  year: 2026,
  month: 8,
  reference_month: "2026-08",
  gross_rent: 5000,
  admin_fee_amount: 400,
  maintenance_deductions: 0,
  other_deductions: 0,
  net_amount: 4600,
  transfers: [
    {
      id: "transfer-aware",
      settlement_type: "rent",
      contract_id: "ctr-palmeiras",
      property_id: "property-palmeiras",
      reference_month: "2026-08",
      gross_rent: 5000,
      admin_fee_amount: 400,
      net_amount: 4600,
      status: "PENDING",
      owner_balance: {
        total_cents: 560000,
        reserved_cents: 100000,
        blocked_cents: 0,
      },
    },
    {
      id: "transfer-legacy",
      settlement_type: "rent",
      contract_id: "ctr-legacy",
      property_id: "property-legacy",
      reference_month: "2026-08",
      gross_rent: 0,
      admin_fee_amount: 0,
      net_amount: 0,
      status: "PROCESSED",
    },
  ],
  generated_at: "2026-08-20T12:00:00.000Z",
};

const emptyBalance: OwnerBalanceSummary = {
  tenant_id: "imobiliaria-estrela",
  owner_lead_id: "lead-marisa",
  transfer_id: "",
  reference_month: "",
  total_cents: 0,
  reserved_cents: 0,
  blocked_cents: 0,
  available_cents: 0,
  has_balance: false,
  updated_at: "",
};

beforeAll(() => {
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

afterEach(() => {
  vi.restoreAllMocks();
});

beforeEach(() => {
  vi.spyOn(ownerStatementService, "getBalance").mockResolvedValue(emptyBalance);
});

describe("OwnerStatementPanel", () => {
  it("separa venda e aluguel e identifica a modalidade nas linhas", async () => {
    vi.spyOn(ownerStatementService, "getAdmin").mockResolvedValue({
      ...statement,
      gross_rent: 1135000,
      net_amount: 1066800,
      flow_summaries: [
        { flow: "sale", transfer_count: 1, gross_amount: 1130000, admin_fee_amount: 0, brokerage_amount: 67800, maintenance_deductions: 0, other_deductions: 0, discount_amount: 0, net_amount: 1062200 },
        { flow: "rent", transfer_count: 1, gross_amount: 5000, admin_fee_amount: 400, brokerage_amount: null, maintenance_deductions: 0, other_deductions: 0, discount_amount: 0, net_amount: 4600 },
      ],
      transfers: [statement.transfers[0], { ...statement.transfers[1], statement_flow: "sale", settlement_type: "sale", gross_rent: 1130000, brokerage_amount: 67800, net_amount: 1062200 }],
    });
    const user = userEvent.setup();
    render(<OwnerStatementPanel />);
    await user.type(screen.getByLabelText(/ID do proprietário/i), "lead-marisa");
    await user.click(screen.getByRole("button", { name: /gerar extrato/i }));
    const sales = await screen.findByRole("region", { name: "Vendas" });
    const rentals = screen.getByRole("region", { name: "Aluguéis" });
    expect(within(sales).getByText("Venda bruta").parentElement).toHaveTextContent("R$ 1.130.000,00");
    expect(within(sales).getByText("Corretagem").parentElement).toHaveTextContent("R$ 67.800,00");
    expect(within(sales).getByText("Líquido do segmento").parentElement).toHaveTextContent("R$ 1.062.200,00");
    expect(within(sales).queryByText("Aluguel bruto")).not.toBeInTheDocument();
    expect(within(rentals).getByText("Aluguel bruto").parentElement).toHaveTextContent("R$ 5.000,00");
    expect(screen.getByRole("cell", { name: "Venda" })).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "Locação" })).toBeInTheDocument();
  });

  it("mantém modalidade e corretagem sem evidência como a apurar", async () => {
    vi.spyOn(ownerStatementService, "getAdmin").mockResolvedValue({ ...statement,
      transfers: [
        { ...statement.transfers[0], settlement_type: undefined },
        { ...statement.transfers[1], settlement_type: "sale", gross_rent: 1000, net_amount: 940 },
      ],
    });
    const user = userEvent.setup();
    render(<OwnerStatementPanel />);
    await user.type(screen.getByLabelText(/ID do proprietário/i), "lead-marisa");
    await user.click(screen.getByRole("button", { name: /gerar extrato/i }));
    const unknown = await screen.findByRole("region", { name: "Modalidade a apurar" });
    expect(within(unknown).getByText("Bruto a apurar").parentElement).toHaveTextContent("R$ 5.000,00");
    expect(within(screen.getByRole("region", { name: "Vendas" })).getByText("Corretagem").parentElement).toHaveTextContent("A apurar");
    expect(screen.queryByText("Aluguel bruto")).not.toBeInTheDocument();
  });

  it("seleciona o proprietário pelo cadastro sem exigir UUID manual", async () => {
    vi.spyOn(ownerStatementService, "getAdmin").mockResolvedValue(statement);
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    render(<OwnerStatementPanel owners={[{
      id: "lead-marisa",
      name: "Marisa Silva",
      email: "marisa@example.com",
      phone: "11999999999",
      property: "",
      budget: 0,
      status: "Convertido",
      date: "2026-01-01",
    }]} />);

    await user.click(screen.getByRole("combobox", { name: "Proprietário" }));
    await user.click(screen.getByRole("option", { name: "Marisa Silva" }));
    await user.click(screen.getByRole("button", { name: /gerar extrato/i }));

    await waitFor(() => expect(ownerStatementService.getAdmin).toHaveBeenCalledWith(
      "lead-marisa",
      expect.any(Number),
      expect.any(Number),
    ));
    expect(screen.queryByLabelText(/ID do proprietário/i)).not.toBeInTheDocument();
  });

  it("mostra somente os bolsões autoritativos e sinaliza repasses legados fora da cobertura", async () => {
    vi.spyOn(ownerStatementService, "getAdmin").mockResolvedValue(statement);
    const user = userEvent.setup();
    render(<OwnerStatementPanel />);

    await user.type(screen.getByLabelText(/ID do proprietário/i), "lead-marisa");
    await user.click(screen.getByRole("button", { name: /gerar extrato/i }));

    await waitFor(() => expect(screen.getByText("Composição econômica dos repasses")).toBeInTheDocument());
    expect(screen.getByText("Total do proprietário").parentElement).toHaveTextContent("R$ 5.600,00");
    expect(screen.getByText("Reservado para encargos").parentElement).toHaveTextContent("R$ 1.000,00");
    expect(screen.getByText("Bloqueado").parentElement).toHaveTextContent("R$ 0,00");
    expect(screen.getByText("Disponível para repasse").parentElement).toHaveTextContent("R$ 4.600,00");
    expect(screen.getByText(/Composição disponível em 1 de 2 repasses/)).toBeInTheDocument();
  });

  it("mostra a manutenção descontada do repasse, com a origem", async () => {
    vi.spyOn(ownerStatementService, "getAdmin").mockResolvedValue({
      ...statement,
      maintenance_deductions: 130,
      net_amount: 4470,
      transfers: [
        {
          ...statement.transfers[0],
          maintenance_deductions: 130,
          net_amount: 4470,
          deduction_details: [
            {
              description: "Manutenção: Elétrica (#os-1)",
              amount: 130,
              type: "MAINTENANCE",
              reference_id: "os-1",
            },
          ],
        },
        statement.transfers[1],
      ],
    });
    const user = userEvent.setup();
    render(<OwnerStatementPanel />);

    await user.type(screen.getByLabelText(/ID do proprietário/i), "lead-marisa");
    await user.click(screen.getByRole("button", { name: /gerar extrato/i }));

    const normalize = (value: string | null) => (value ?? "").replace(/\u00a0/g, " ");
    await screen.findByText("Deduções do período");

    const transferRow = screen
      .getAllByRole("row")
      .find((row) => normalize(row.textContent).includes("Locação") && normalize(row.textContent).includes("130,00"));
    expect(transferRow).toBeDefined();
    // Bruto − taxa − deduções = líquido, com a dedução visível na linha.
    const rowText = normalize(transferRow?.textContent ?? "");
    expect(rowText).toContain("R$ 130,00");
    expect(rowText).toContain("R$ 4.470,00");


    // Detalhe com serviço e OS de origem.
    expect(screen.getByText("Manutenção: Elétrica (#os-1)")).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "Manutenção" })).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "os-1" })).toBeInTheDocument();
    const deductionRow = screen
      .getAllByRole("row")
      .find((row) => normalize(row.textContent).includes("Manutenção: Elétrica (#os-1)"));
    expect(normalize(deductionRow?.textContent ?? "")).toContain("− R$ 130,00");
  });

  it("não renderiza o bloco de deduções quando o repasse não tem descontos", async () => {
    vi.spyOn(ownerStatementService, "getAdmin").mockResolvedValue(statement);
    const user = userEvent.setup();
    render(<OwnerStatementPanel />);

    await user.type(screen.getByLabelText(/ID do proprietário/i), "lead-marisa");
    await user.click(screen.getByRole("button", { name: /gerar extrato/i }));

    await waitFor(() => expect(screen.getByText("Aluguel bruto")).toBeInTheDocument());
    expect(screen.queryByText("Deduções do período")).not.toBeInTheDocument();
  });

  it("não fabrica bolsões para demonstrativos legados", async () => {
    vi.spyOn(ownerStatementService, "getAdmin").mockResolvedValue({
      ...statement,
      transfers: statement.transfers.map((transfer) => ({ ...transfer, owner_balance: undefined })),
    });
    const user = userEvent.setup();
    render(<OwnerStatementPanel />);

    await user.type(screen.getByLabelText(/ID do proprietário/i), "lead-marisa");
    await user.click(screen.getByRole("button", { name: /gerar extrato/i }));

    await waitFor(() => expect(screen.getByText("Aluguel bruto")).toBeInTheDocument());
    expect(screen.queryByText("Composição econômica dos repasses")).not.toBeInTheDocument();
    expect(screen.queryByText("Disponível para repasse")).not.toBeInTheDocument();
  });

  it("prefere os agregados autoritativos do demonstrativo quando presentes", async () => {
    vi.spyOn(ownerStatementService, "getAdmin").mockResolvedValue({
      ...statement,
      owner_balance_total: 5600,
      reserved_amount: 1000,
      blocked_amount: 0,
      available_amount: 4600,
      balance_covered_count: 1,
      transfers: statement.transfers.map((transfer) => ({ ...transfer, owner_balance: undefined })),
    });
    const user = userEvent.setup();
    render(<OwnerStatementPanel />);

    await user.type(screen.getByLabelText(/ID do proprietário/i), "lead-marisa");
    await user.click(screen.getByRole("button", { name: /gerar extrato/i }));

    await waitFor(() => expect(screen.getByText("Composição econômica dos repasses")).toBeInTheDocument());
    expect(screen.getByText("Total do proprietário").parentElement).toHaveTextContent("R$ 5.600,00");
    expect(screen.getByText("Disponível para repasse").parentElement).toHaveTextContent("R$ 4.600,00");
    expect(screen.getByText(/Composição disponível em 1 de 2 repasses/)).toBeInTheDocument();
  });

  it("exibe o widget de saldo corrente e as contas administradas + conferência contábil", async () => {
    vi.spyOn(ownerStatementService, "getAdmin").mockResolvedValue({
      ...statement,
      managed_bills: [
        {
          id: "mb-1",
          component: "IPTU",
          creditor_name: "Prefeitura",
          description: "IPTU 2026",
          amount: 250,
          status: "RESERVED",
          due_date: "2026-09-10T00:00:00.000Z",
        },
        {
          id: "mb-2",
          component: "condomínio",
          creditor_name: "Condomínio Jardins",
          description: "Condomínio setembro",
          amount: 150,
          status: "PAID",
          due_date: null,
        },
      ],
      managed_bills_total: 400,
      journal_totals: { revenue: 5000, owner_payable: -3600, accounts_receivable: 2000, cash: 5000 },
    });
    vi.spyOn(ownerStatementService, "getBalance").mockResolvedValue({
      tenant_id: "imobiliaria-estrela",
      owner_lead_id: "lead-marisa",
      transfer_id: "transfer-aware",
      reference_month: "2026-08",
      total_cents: 560000,
      reserved_cents: 100000,
      blocked_cents: 0,
      available_cents: 460000,
      has_balance: true,
      updated_at: "2026-08-20T12:00:00.000Z",
    });
    const user = userEvent.setup();
    render(<OwnerStatementPanel />);

    await user.type(screen.getByLabelText(/ID do proprietário/i), "lead-marisa");
    await user.click(screen.getByRole("button", { name: /gerar extrato/i }));

    const widget = await screen.findByTestId("owner-balance-widget");
    expect(within(widget).getByText("Saldo atual do proprietário")).toBeInTheDocument();
    expect(within(widget).getByText(/competência Ago\/2026/)).toBeInTheDocument();
    expect(within(widget).getByText("Total").parentElement).toHaveTextContent("R$ 5.600,00");
    expect(within(widget).getByText("Disponível para repasse").parentElement).toHaveTextContent("R$ 4.600,00");

    expect(screen.getByText("Contas administradas no período")).toBeInTheDocument();
    expect(screen.getByText("Prefeitura")).toBeInTheDocument();
    expect(screen.getByText(/Total: R\$ 400,00/)).toBeInTheDocument();
    expect(screen.getByText("Reservada")).toBeInTheDocument();
    expect(screen.getByText("Paga")).toBeInTheDocument();

    expect(screen.getByText("Conferência contábil (ledger)")).toBeInTheDocument();
    expect(screen.getByText("Receita (revenue)").parentElement).toHaveTextContent("R$ 5.000,00");
    expect(screen.getByText("A pagar ao proprietário").parentElement).toHaveTextContent(/[-−].*3\.600,00/);
  });
});

/**
 * O extrato mostrava apenas a taxa de administração: o proprietário via
 * "aluguel bruto − taxa" e não fechava com o líquido, porque a comissão do
 * primeiro aluguel (intermediação) também sai do aluguel nos primeiros meses.
 */
describe("OwnerStatementPanel — intermediação", () => {
  const withIntermediation: MonthlyStatement = {
    ...statement,
    gross_rent: 2800,
    admin_fee_amount: 280,
    net_amount: 2061.28,
    intermediation_amount: 457.44,
    intermediation_commission_funded_amount: 361.28,
    intermediation_outstanding_amount: 2077.44,
    flow_summaries: [
      {
        flow: "rent",
        transfer_count: 1,
        gross_amount: 2800,
        admin_fee_amount: 280,
        brokerage_amount: null,
        maintenance_deductions: 0,
        other_deductions: 0,
        discount_amount: 0,
        intermediation_amount: 457.44,
        intermediation_commission_funded_amount: 361.28,
        net_amount: 2061.28,
      },
    ],
    transfers: [
      {
        id: "transfer-intermediation",
        settlement_type: "rent",
        contract_id: "ctr-palmeiras",
        property_id: "property-palmeiras",
        reference_month: "2026-10",
        gross_rent: 2800,
        admin_fee_amount: 280,
        net_amount: 2061.28,
        status: "PENDING",
        intermediation_amount: 457.44,
        intermediation_commission_funded_amount: 361.28,
        intermediation_outstanding_amount: 2077.44,
        owner_balance: { total_cents: 206128, reserved_cents: 0, blocked_cents: 0 },
      },
    ],
  };

  beforeEach(() => {
    vi.spyOn(ownerStatementService, "getAdmin").mockResolvedValue(withIntermediation);
    vi.spyOn(ownerStatementService, "getBalance").mockResolvedValue(emptyBalance);
  });

  async function gerarExtrato(user: ReturnType<typeof userEvent.setup>) {
    render(<OwnerStatementPanel />);
    await user.clear(screen.getByLabelText(/ID do proprietário/i));
    await user.type(screen.getByLabelText(/ID do proprietário/i), "lead-marisa");
    await user.click(screen.getByRole("button", { name: /gerar extrato/i }));
  }

  it("explica o desconto da intermediação e o efeito no repasse", async () => {
    const user = userEvent.setup();
    await gerarExtrato(user);

    const card = await screen.findByRole("region", { name: /Intermediação descontada do repasse/i });
    expect(within(card).getByText(/Intermediação \(comissão do primeiro aluguel\)/i)).toBeInTheDocument();
    // Valor do desconto no período (o "−" é um nó separado do valor).
    expect(card).toHaveTextContent(/−\s*R\$\s*457,44/);
    // Reflexos: comissões pagas e o que ainda será descontado.
    expect(within(card).getByText(/361,28 foram para comissões de corretores/)).toBeInTheDocument();
    expect(within(card).getByText(/Ainda serão descontados/)).toBeInTheDocument();
    expect(within(card).getByText(/R\$\s*2\.077,44/)).toBeInTheDocument();
    // Deixa explícito que a taxa de administração continua sendo cobrada todo mês.
    expect(within(card).getByText(/taxa de administração continua sendo cobrada todo mês/)).toBeInTheDocument();
  });

  it("mostra a intermediação também no resumo da locação e na linha do repasse", async () => {
    const user = userEvent.setup();
    await gerarExtrato(user);

    const rentSection = await screen.findByRole("region", { name: "Aluguéis" });
    expect(within(rentSection).getByText("Intermediação")).toBeInTheDocument();
    expect(within(rentSection).getByText(/457,44/)).toBeInTheDocument();

    const row = screen.getByRole("cell", { name: "Locação" }).closest("tr");
    expect(row).not.toBeNull();
    expect(row!).toHaveTextContent(/−\s*R\$\s*457,44/);
  });

  it("sem intermediação, não inventa a seção", async () => {
    vi.spyOn(ownerStatementService, "getAdmin").mockResolvedValue({ ...statement, intermediation_amount: 0, intermediation_outstanding_amount: 0 });
    const user = userEvent.setup();
    await gerarExtrato(user);

    await screen.findByText(/Total líquido dos repasses/i);
    expect(screen.queryByRole("region", { name: /Intermediação descontada do repasse/i })).not.toBeInTheDocument();
  });
});
