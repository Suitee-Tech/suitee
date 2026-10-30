import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { PortalFinance } from "./PortalFinance";
import type { PortalCharge } from "../services/portal.service";

// Item 2.7 da Fase 2 do plano de inadimplência (G32): a tela Financeiro do
// portal mostrava só `total_amount` e o cliente não entendia por que o valor
// tinha subido. Agora a cobrança vencida discrimina valor original, multa,
// juros, desconto e total, com os dias de atraso — e a cobrança em dia continua
// exatamente como era.
//
// O terceiro caso trava a compatibilidade: payload antigo (sem os campos novos)
// não pode quebrar a tela.

const mocks = vi.hoisted(() => ({
  charges: [] as PortalCharge[],
  isPending: false,
  error: null as Error | null,
  refetch: vi.fn(),
}));

vi.mock("../context/PortalDealsContext", () => ({
  usePortalDeals: () => ({
    deals: [{ id: "deal-1" }],
    activeDeal: { id: "deal-1" },
    activeDealId: "deal-1",
    isLoading: false,
    error: null,
    retry: vi.fn(),
    setActiveDealId: vi.fn(),
  }),
}));

vi.mock("../hooks/usePortalPersona", () => ({
  usePortalPersona: () => ({
    persona: { isOwnerSide: false },
    copy: { financeTitle: "Meus pagamentos" },
  }),
}));

vi.mock("../hooks/useDealResources", () => ({
  usePortalDealCharges: () => ({
    data: mocks.charges,
    isPending: mocks.isPending,
    error: mocks.error,
    refetch: mocks.refetch,
  }),
}));

/**
 * Mesmo formato da tela, já normalizado como o testing-library normaliza o DOM:
 * o `Intl` usa espaço não separável (NBSP) depois de "R$" e o matcher por texto
 * compara com o valor colapsado.
 */
const brl = (value: number) =>
  value
    .toLocaleString("pt-BR", { style: "currency", currency: "BRL" })
    .replace(/[\u00a0\u202f]/g, " ");

/** Meio-dia local para o dia relativo a hoje: o cálculo por dia de calendário não oscila com a hora do teste. */
function isoDaysFromNow(days: number): string {
  const date = new Date();
  date.setHours(12, 0, 0, 0);
  date.setDate(date.getDate() + days);
  return date.toISOString();
}

function charge(overrides: Partial<PortalCharge>): PortalCharge {
  return {
    id: "charge-1",
    description: "Aluguel",
    due_date: isoDaysFromNow(-23),
    amount: 3200,
    total_amount: 3200,
    status: "OVERDUE",
    ...overrides,
  };
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/portal/finance"]}>
      <PortalFinance />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  mocks.charges = [];
  mocks.isPending = false;
  mocks.error = null;
  mocks.refetch.mockReset();
});

describe("PortalFinance — encargos discriminados (G32)", () => {
  it("mostra valor original, multa, juros, total e os dias de atraso na cobrança vencida", () => {
    mocks.charges = [
      charge({
        id: "charge-open-early",
        description: "Taxa de lixo",
        due_date: isoDaysFromNow(-30),
        amount: 100,
        total_amount: 100,
        status: "PENDING",
      }),
      charge({
        id: "charge-overdue",
        description: "Aluguel de outubro",
        due_date: isoDaysFromNow(-23),
        amount: 3200,
        late_fee: 64,
        interest: 12.8,
        total_amount: 3276.8,
        days_overdue: 23,
        status: "OVERDUE",
      }),
    ];

    renderPage();

    expect(screen.getByText("Aluguel de outubro")).toBeInTheDocument();
    expect(screen.getByText("Valor original")).toBeInTheDocument();
    expect(screen.getByText("Multa")).toBeInTheDocument();
    expect(screen.getByText("Juros")).toBeInTheDocument();
    expect(screen.getByText("Total")).toBeInTheDocument();
    expect(screen.getByText(brl(3200))).toBeInTheDocument();
    expect(screen.getByText(brl(64))).toBeInTheDocument();
    expect(screen.getByText(brl(12.8))).toBeInTheDocument();
    // Total aparece no valor grande do cartão e na última linha da quebra.
    expect(screen.getAllByText(brl(3276.8))).toHaveLength(2);
    // "Vencida há N dias" só existe na quebra de valores: o destaque usa "Vencido há N dias".
    expect(screen.getByText("Venceu há 23 dias")).toBeInTheDocument();
    // A cobrança sem encargo (a mais antiga, ainda PENDING) não ganha quebra.
    expect(screen.getByText("Taxa de lixo")).toBeInTheDocument();
    expect(screen.getAllByLabelText("Detalhamento da cobrança")).toHaveLength(1);
  });

  it("não mostra quebra nenhuma quando a cobrança está em dia", () => {
    mocks.charges = [
      charge({
        id: "charge-open",
        description: "Aluguel de novembro",
        due_date: isoDaysFromNow(12),
        amount: 2800,
        total_amount: 2800,
        status: "PENDING",
      }),
    ];

    renderPage();

    expect(screen.getByText("Aluguel de novembro")).toBeInTheDocument();
    expect(screen.getAllByText(brl(2800)).length).toBeGreaterThan(0);
    expect(screen.queryByText("Valor original")).toBeNull();
    expect(screen.queryByText("Multa")).toBeNull();
    expect(screen.queryByText("Juros")).toBeNull();
    expect(screen.queryByText("Total")).toBeNull();
    expect(screen.queryByLabelText("Detalhamento da cobrança")).toBeNull();
  });

  it("não quebra com payload antigo, sem nenhum campo novo", () => {
    mocks.charges = [
      charge({
        id: "charge-legacy",
        description: "Aluguel vencido",
        due_date: isoDaysFromNow(-12),
        amount: 1500,
        total_amount: 1500,
        status: "OVERDUE",
      }),
    ];

    renderPage();

    expect(screen.getByText("Aluguel vencido")).toBeInTheDocument();
    expect(screen.getAllByText(brl(1500)).length).toBeGreaterThan(0);
    // Sem encargo não há quebra — mas o atraso continua dito no destaque.
    expect(screen.queryByLabelText("Detalhamento da cobrança")).toBeNull();
    expect(screen.getByText(/Venceu há 12 dias/)).toBeInTheDocument();
  });

  it("deriva os dias de atraso do vencimento quando o payload não traz days_overdue", () => {
    mocks.charges = [
      charge({
        id: "charge-legacy-fees",
        description: "Aluguel vencido com encargo",
        due_date: isoDaysFromNow(-12),
        amount: 1500,
        late_fee: 30,
        interest: 5.36,
        total_amount: 1535.36,
        status: "OVERDUE",
      }),
    ];

    renderPage();

    // O destaque e o cartão da lista mostram a quebra: o valor exibido subiu.
    expect(screen.getAllByLabelText("Detalhamento da cobrança")).toHaveLength(2);
    expect(screen.getAllByText("Multa").length).toBeGreaterThan(0);
    expect(screen.getAllByText(brl(30)).length).toBeGreaterThan(0);
    expect(screen.getAllByText(brl(5.36)).length).toBeGreaterThan(0);
    expect(screen.getByText("Venceu há 12 dias")).toBeInTheDocument();
    expect(screen.getAllByText(brl(1535.36)).length).toBeGreaterThan(0);
  });
});
