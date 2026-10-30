import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { CommissionDeductionsEditor } from "./CommissionDeductionsEditor";
import { commissionService, type Commission, type Deduction } from "../services/commission.service";
import { brokerAdvanceService } from "../services/brokerAdvance.service";
import { tenantFinancialSettingsService } from "../services/tenantFinancialSettings.service";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }));
vi.mock("../services/commission.service", () => ({
  commissionService: { addDeduction: vi.fn(), removeDeduction: vi.fn() },
}));
vi.mock("../services/tenantFinancialSettings.service", () => ({
  tenantFinancialSettingsService: { get: vi.fn() },
}));
vi.mock("../services/brokerAdvance.service", () => ({
  brokerAdvanceService: { outstandingForBroker: vi.fn() },
}));

// Bruto de R$ 28.800,00 é o caso que o backend usa como exemplo: 20% de INSS
// dão R$ 5.760,00 exatos.
const commission: Commission = {
  id: "commission-1",
  tenant_id: "tenant-1",
  deal_id: "deal-1",
  broker_id: "broker-1",
  role: "captador",
  amount: 28800,
  gross_amount: 28800,
  net_amount: 28800,
  currency: "BRL",
  status: "PENDING",
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-02T10:00:00Z",
};

const addDeduction = vi.mocked(commissionService.addDeduction);
const getSettings = vi.mocked(tenantFinancialSettingsService.get);
const outstandingForBroker = vi.mocked(brokerAdvanceService.outstandingForBroker);

beforeEach(() => {
  addDeduction.mockReset().mockResolvedValue(commission);
  outstandingForBroker.mockReset().mockResolvedValue({ outstanding_cents: 150000, open_count: 1 });
  getSettings.mockReset().mockResolvedValue({
    tenant_id: "tenant-1",
    default_late_fee_pct: 2,
    default_monthly_interest_pct: 1,
    default_due_day: 10,
    default_commission_pct_rent: 100,
    default_commission_pct_sale: 6,
    default_currency: "BRL",
    payout_proof_required: true,
    admin_fee_pct: 10,
    default_lease_term_months: 30,
    default_inss_pct: 20,
    default_iss_pct: 2.5,
    updated_at: "",
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

function renderEditor(overrides: Partial<Commission> = {}, onChange = vi.fn()) {
  render(<CommissionDeductionsEditor commission={{ ...commission, ...overrides }} onChange={onChange} />);
  return { onChange };
}

const rateInput = () => screen.getByLabelText("Alíquota do desconto (%)");
const kindSelect = () => screen.getByLabelText("Tipo de desconto");
const addButton = () => screen.getByRole("button", { name: /Adicionar/ });

/** Entra no modo percentual e espera as alíquotas padrão do tenant chegarem. */
async function switchToRateMode(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "Percentual (%)" }));
  await waitFor(() => expect(getSettings).toHaveBeenCalled());
}

describe("CommissionDeductionsEditor · lançamento por alíquota", () => {
  it("pré-preenche a alíquota do tipo, mostra base e valor calculado e envia rate_pct/base_cents", async () => {
    const user = userEvent.setup();
    renderEditor();
    await switchToRateMode(user);
    await user.selectOptions(kindSelect(), "INSS");

    await waitFor(() => expect(rateInput()).toHaveValue("20"));

    // O valor NÃO é digitado: base e resultado aparecem antes de confirmar.
    expect(screen.getByText(/Base de cálculo \(bruto da comissão\):/)).toHaveTextContent("R$ 28.800,00");
    expect(screen.getByText(/Valor calculado/)).toHaveTextContent("R$ 5.760,00");
    expect(screen.getByText(/não digitado/)).toBeInTheDocument();
    expect(screen.queryByLabelText("Valor do desconto")).not.toBeInTheDocument();

    await user.click(addButton());

    await waitFor(() => expect(addDeduction).toHaveBeenCalledTimes(1));
    const [commissionId, payload] = addDeduction.mock.calls[0];
    expect(commissionId).toBe("commission-1");
    expect(payload).toMatchObject({
      kind: "INSS",
      amount: 5760,
      rate_pct: 20,
      base_cents: 2880000,
    });
    expect(typeof payload.deduction_id).toBe("string");
  });

  it("usa a alíquota padrão de ISS com decimal e arredonda o valor ao centavo", async () => {
    const user = userEvent.setup();
    renderEditor();
    await switchToRateMode(user);
    await user.selectOptions(kindSelect(), "ISS");

    await waitFor(() => expect(rateInput()).toHaveValue("2,5"));
    // 2,5% de R$ 28.800,00 = R$ 720,00.
    expect(screen.getByText(/Valor calculado/)).toHaveTextContent("R$ 720,00");

    await user.click(addButton());

    await waitFor(() => expect(addDeduction).toHaveBeenCalledTimes(1));
    expect(addDeduction.mock.calls[0][1]).toMatchObject({ amount: 720, rate_pct: 2.5, base_cents: 2880000 });
  });

  it("começa vazio em IRRF e OUTROS, que não têm alíquota padrão", async () => {
    const user = userEvent.setup();
    renderEditor();
    await switchToRateMode(user);

    expect(rateInput()).toHaveValue("");
    await user.selectOptions(kindSelect(), "OUTROS");
    expect(rateInput()).toHaveValue("");
  });

  it("aceita alíquota digitada quando os padrões do tenant não carregam", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    getSettings.mockRejectedValue(new Error("offline"));
    const user = userEvent.setup();
    renderEditor();
    await switchToRateMode(user);

    await user.type(rateInput(), "5");
    expect(screen.getByText(/Valor calculado/)).toHaveTextContent("R$ 1.440,00");

    await user.click(addButton());

    await waitFor(() => expect(addDeduction).toHaveBeenCalledTimes(1));
    expect(addDeduction.mock.calls[0][1]).toMatchObject({ amount: 1440, rate_pct: 5, base_cents: 2880000 });
    expect(warn).toHaveBeenCalled();
  });
});

describe("CommissionDeductionsEditor · validação do modo percentual", () => {
  it("recusa alíquota zerada com mensagem no próprio campo e não chama a API", async () => {
    const user = userEvent.setup();
    renderEditor();
    await switchToRateMode(user);
    await user.selectOptions(kindSelect(), "INSS");
    await waitFor(() => expect(rateInput()).toHaveValue("20"));

    await user.clear(rateInput());
    await user.type(rateInput(), "0");
    await user.click(addButton());

    expect(await screen.findByRole("alert")).toHaveTextContent("Informe uma alíquota maior que zero.");
    expect(addDeduction).not.toHaveBeenCalled();
  });

  it("recusa alíquota acima de 100%", async () => {
    const user = userEvent.setup();
    renderEditor();
    await switchToRateMode(user);

    await user.type(rateInput(), "150");
    await user.click(addButton());

    expect(await screen.findByRole("alert")).toHaveTextContent("A alíquota não pode passar de 100%.");
    expect(addDeduction).not.toHaveBeenCalled();
  });

  it("recusa quando a comissão não tem bruto para servir de base", async () => {
    const user = userEvent.setup();
    renderEditor({ amount: 0, gross_amount: 0, net_amount: 0 });
    await switchToRateMode(user);

    await user.type(rateInput(), "20");
    await user.click(addButton());

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Esta comissão não tem valor bruto para servir de base de cálculo.",
    );
    expect(addDeduction).not.toHaveBeenCalled();
  });

  it("recusa alíquota pequena demais, que zeraria o valor calculado", async () => {
    const user = userEvent.setup();
    renderEditor({ amount: 4000, gross_amount: 4000, net_amount: 4000 });
    await switchToRateMode(user);

    await user.type(rateInput(), "0,0001");
    await user.click(addButton());

    expect(await screen.findByRole("alert")).toHaveTextContent("O valor calculado ficou zerado. Revise a alíquota.");
    expect(addDeduction).not.toHaveBeenCalled();
  });
});

describe("CommissionDeductionsEditor · valor fixo e lista", () => {
  it("mantém o lançamento por valor sem enviar alíquota nem base", async () => {
    const user = userEvent.setup();
    renderEditor();

    await user.type(screen.getByLabelText("Valor do desconto"), "576000");
    expect(screen.getByLabelText("Valor do desconto")).toHaveValue("5.760,00");

    await user.click(addButton());

    await waitFor(() => expect(addDeduction).toHaveBeenCalledTimes(1));
    const payload = addDeduction.mock.calls[0][1];
    expect(payload).toMatchObject({ kind: "IRRF", amount: 5760 });
    expect(payload).not.toHaveProperty("rate_pct");
    expect(payload).not.toHaveProperty("base_cents");
  });

  it("não busca as alíquotas padrão enquanto o modo é valor", async () => {
    const user = userEvent.setup();
    renderEditor();

    await user.type(screen.getByLabelText("Valor do desconto"), "10000");
    expect(getSettings).not.toHaveBeenCalled();
  });

  it("explica na lista como o valor retido foi calculado, sem esconder o valor", () => {
    const deductions: Deduction[] = [
      { id: "d1", kind: "INSS", amount: 5760, rate_pct: 20, base_cents: 2880000 },
      { id: "d2", kind: "OUTROS", amount: 100 },
    ];
    renderEditor({ deductions, net_amount: 22940 });

    expect(screen.getByText("20% de R$ 28.800,00 = R$ 5.760,00")).toBeInTheDocument();
    // O valor final continua sendo o dado principal da linha.
    expect(screen.getByText("-R$ 5.760,00")).toBeInTheDocument();
    // Lançamento por valor fixo não ganha a linha de cálculo.
    expect(screen.getAllByText(/ de R\$ /)).toHaveLength(1);
  });
});

describe("CommissionDeductionsEditor · abatimento de adiantamento (ADVANCE)", () => {
  /** Seleciona o tipo e espera o saldo do corretor chegar. */
  async function selectAdvance(user: ReturnType<typeof userEvent.setup>) {
    await user.selectOptions(kindSelect(), "ADVANCE");
    await waitFor(() => expect(outstandingForBroker).toHaveBeenCalledWith("broker-1"));
    await screen.findByText(/Saldo de adiantamentos do corretor:/);
  }

  it("busca e mostra o saldo de adiantamentos do corretor só quando o tipo é Adiantamento", async () => {
    const user = userEvent.setup();
    renderEditor();

    // Nenhuma consulta de saldo enquanto o tipo é tributo.
    expect(outstandingForBroker).not.toHaveBeenCalled();

    await selectAdvance(user);

    expect(screen.getByText(/Saldo de adiantamentos do corretor:/)).toHaveTextContent("R$ 1.500,00");
    expect(screen.getByLabelText("Valor do desconto")).toBeInTheDocument();
  });

  it("trata o adiantamento como valor fixo: o modo percentual fica indisponível", async () => {
    const user = userEvent.setup();
    renderEditor();
    await selectAdvance(user);

    expect(screen.getByRole("button", { name: "Valor (R$)" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Percentual (%)" })).toBeDisabled();
    // Sem alíquota não há campo de alíquota nem base de cálculo.
    expect(screen.queryByLabelText("Alíquota do desconto (%)")).not.toBeInTheDocument();
    expect(getSettings).not.toHaveBeenCalled();
  });

  it("avisa e bloqueia quando o valor passa do saldo do corretor", async () => {
    const user = userEvent.setup();
    renderEditor();
    await selectAdvance(user);

    // R$ 2.000,00 contra um saldo de R$ 1.500,00 — o backend recusaria com 400.
    await user.type(screen.getByLabelText("Valor do desconto"), "200000");

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "O valor excede o saldo de adiantamentos do corretor (R$ 1.500,00)",
    );

    await user.click(addButton());

    expect(addDeduction).not.toHaveBeenCalled();
    expect(await screen.findByRole("alert")).toHaveTextContent(/excede o saldo de adiantamentos do corretor/i);
  });

  it("envia kind ADVANCE com valor fixo, sem rate_pct nem base_cents", async () => {
    const user = userEvent.setup();
    renderEditor();
    await selectAdvance(user);

    // Exatamente o saldo: dentro do limite, o lançamento passa.
    await user.type(screen.getByLabelText("Valor do desconto"), "150000");
    await user.click(addButton());

    await waitFor(() => expect(addDeduction).toHaveBeenCalledTimes(1));
    const [commissionId, payload] = addDeduction.mock.calls[0];
    expect(commissionId).toBe("commission-1");
    expect(payload).toMatchObject({ kind: "ADVANCE", amount: 1500 });
    expect(payload).not.toHaveProperty("rate_pct");
    expect(payload).not.toHaveProperty("base_cents");
  });

  it("libera o lançamento quando a consulta do saldo falha — quem valida é o servidor", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    outstandingForBroker.mockRejectedValue(new Error("offline"));
    const user = userEvent.setup();
    renderEditor();
    await user.selectOptions(kindSelect(), "ADVANCE");

    expect(await screen.findByText(/Não foi possível consultar o saldo de adiantamentos/i)).toBeInTheDocument();

    await user.type(screen.getByLabelText("Valor do desconto"), "999900");
    await user.click(addButton());

    await waitFor(() => expect(addDeduction).toHaveBeenCalledTimes(1));
    expect(addDeduction.mock.calls[0][1]).toMatchObject({ kind: "ADVANCE", amount: 9999 });
    expect(warn).toHaveBeenCalled();
  });

  it("nomeia o desconto de adiantamento na lista de descontos já lançados", () => {
    renderEditor({ deductions: [{ id: "d1", kind: "ADVANCE", amount: 1500 }], net_amount: 27300 });

    expect(screen.getByText("Adiantamento")).toBeInTheDocument();
    expect(screen.getByText("-R$ 1.500,00")).toBeInTheDocument();
  });
});
