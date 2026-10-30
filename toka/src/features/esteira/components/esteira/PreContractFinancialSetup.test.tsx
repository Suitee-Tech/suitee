import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { DealDTO } from "../../../../shared/services/deal.service";
import {
  FinancialPlanRestricted,
  isRentalPreContractPlanReady,
  PreContractFinancialSetup,
  type RentalPreContractPlan,
} from "./PreContractFinancialSetup";

const saleDeal: DealDTO = {
  id: "deal-sale",
  type: "SALE",
  status: "CONTRACT",
  property_id: "property-1",
  owner_id: "owner-1",
  customer_id: "buyer-1",
  broker_id: "broker-1",
  value: 980_000,
  commission: 6,
  created_at: "2026-08-29T00:00:00Z",
  updated_at: "2026-08-29T00:00:00Z",
  tenant_id: "tenant-1",
};

const rentalDeal: DealDTO = {
  ...saleDeal,
  id: "deal-rent",
  type: "RENT",
  customer_id: "tenant-party-1",
  value: 3_200,
  commission: 0,
};

const rentalPlan = (patch: Partial<RentalPreContractPlan> = {}): RentalPreContractPlan => ({
  startDate: "2026-09-15",
  endDate: "2029-03-15",
  suggestedEndDate: "2029-03-15",
  termMonths: 30,
  firstChargeType: "same_month_pro_rata",
  firstChargeDate: "2026-10-10",
  defaultDueDay: 10,
  adminFeePct: 10,
  onStartDateChange: vi.fn(),
  onEndDateChange: vi.fn(),
  onTermMonthsChange: vi.fn(),
  onFirstChargeTypeChange: vi.fn(),
  onFirstChargeDateChange: vi.fn(),
  onDefaultDueDayChange: vi.fn(),
  ...patch,
});

describe("PreContractFinancialSetup", () => {
  it("exige uma escolha explícita para a venda e explica o plano de contas", () => {
    const onConfigureSale = vi.fn();

    render(
      <PreContractFinancialSetup
        deal={saleDeal}
        saleDraft={null}
        saleDraftCurrent={false}
        ready={false}
        onConfigureSale={onConfigureSale}
        onOpenFinancialPreview={vi.fn()}
      />,
    );

    expect(screen.getByText("Plano financeiro antes do contrato")).toBeInTheDocument();
    expect(screen.getByText("A proposta não substitui o plano operacional")).toBeInTheDocument();
    expect(screen.getByText("Contas a receber")).toBeInTheDocument();
    expect(screen.getByText("Repasse ao proprietário")).toBeInTheDocument();
    expect(screen.queryByText(/será gerada 1 cobrança à vista/i)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /definir recebimento/i }));
    expect(onConfigureSale).toHaveBeenCalledTimes(1);
  });

  it("organiza vigência, primeiro aluguel e separação econômica da locação", () => {
    const plan = rentalPlan();

    render(
      <PreContractFinancialSetup
        deal={rentalDeal}
        rentalPlan={plan}
        saleDraftCurrent={false}
        ready
        onConfigureSale={vi.fn()}
        onOpenFinancialPreview={vi.fn()}
      />,
    );

    expect(screen.getByText("Calendário e primeiro aluguel")).toBeInTheDocument();
    expect(screen.getByText("Receita de administração")).toBeInTheDocument();
    expect(screen.getAllByText("Passivo de terceiros", { selector: "span" }).length).toBeGreaterThan(0);
    expect(screen.getByText(/320,00/)).toBeInTheDocument();
    expect(screen.getByText(/2\.880,00/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /acumular no próximo mês/i }));
    expect(plan.onFirstChargeTypeChange).toHaveBeenCalledWith("next_month_accumulated");
  });

  it("bloqueia datas incoerentes e dias de vencimento que não existem em todos os meses", () => {
    expect(isRentalPreContractPlanReady(rentalPlan())).toBe(true);
    expect(isRentalPreContractPlanReady(rentalPlan({ defaultDueDay: 31 }))).toBe(false);
    expect(isRentalPreContractPlanReady(rentalPlan({ firstChargeDate: "2026-09-01" }))).toBe(false);
    expect(isRentalPreContractPlanReady(rentalPlan({ endDate: "2026-09-01" }))).toBe(false);
  });

  it("mostra o prazo do contrato com padrão de 30 meses e informa o término calculado", () => {
    const plan = rentalPlan();
    render(
      <PreContractFinancialSetup
        deal={rentalDeal}
        rentalPlan={plan}
        saleDraftCurrent={false}
        ready
        onConfigureSale={vi.fn()}
        onOpenFinancialPreview={vi.fn()}
      />,
    );

    const termField = screen.getByLabelText(/Prazo do contrato \(meses\)/i);
    expect(termField).toHaveValue(30);
    // Término coerente com (início + prazo): 15/09/2026 + 30 meses.
    expect(screen.getByLabelText(/Término do contrato/i)).toHaveValue("2029-03-15");
    expect(screen.getByText(/30 meses a partir do início/i)).toBeInTheDocument();

    fireEvent.change(termField, { target: { value: "12" } });
    expect(plan.onTermMonthsChange).toHaveBeenCalledWith(12);
  });

  it("mostra pró-rata do primeiro mês e residual do término com a base em dias", () => {
    render(
      <PreContractFinancialSetup
        deal={{ ...rentalDeal, value: 3_200 }}
        rentalPlan={rentalPlan({ startDate: "2026-04-10", endDate: "2029-04-15", suggestedEndDate: "2029-04-10" })}
        saleDraftCurrent={false}
        ready
        onConfigureSale={vi.fn()}
        onOpenFinancialPreview={vi.fn()}
      />,
    );

    expect(screen.getByText("Calendário de cobranças do contrato")).toBeInTheDocument();
    // 10/04 → 21 de 30 dias = R$ 2.240,00; término em 15/04/2029 → 15 de 30 dias = R$ 1.600,00.
    expect(screen.getByText(/2\.240,00/)).toBeInTheDocument();
    expect(screen.getByText(/1\.600,00/)).toBeInTheDocument();
    expect(screen.getByText(/21 de 30 dia\(s\)/)).toBeInTheDocument();
    expect(screen.getByText(/15 de 30 dia\(s\)/)).toBeInTheDocument();
    expect(screen.getByText(/Término 5 dia\(s\) depois do prazo de 30 meses/)).toBeInTheDocument();
  });

  it("explica o mês cheio quando o término cai no último dia do mês", () => {
    render(
      <PreContractFinancialSetup
        deal={{ ...rentalDeal, value: 3_200 }}
        rentalPlan={rentalPlan({ startDate: "2026-10-02", endDate: "2029-02-28", suggestedEndDate: "2029-02-28" })}
        saleDraftCurrent={false}
        ready
        onConfigureSale={vi.fn()}
        onOpenFinancialPreview={vi.fn()}
      />,
    );

    expect(screen.getByText(/28 de 28 dia\(s\)/)).toBeInTheDocument();
    expect(screen.getByText(/mês cheio — o término cai no último dia/)).toBeInTheDocument();
  });

  it("prazo fora da faixa reprova o plano", () => {
    expect(isRentalPreContractPlanReady(rentalPlan({ termMonths: 0 }))).toBe(false);
    expect(isRentalPreContractPlanReady(rentalPlan({ termMonths: 121 }))).toBe(false);
    expect(isRentalPreContractPlanReady(rentalPlan({ termMonths: 1 }))).toBe(true);
  });
});

/**
 * Estado mostrado na aba Assinatura para quem não tem acesso financeiro: diz o
 * que está restrito, por que a minuta depende disso e a quem pedir acesso —
 * sem plano-base, valores, mapa de contas ou botões de configuração.
 */
describe("FinancialPlanRestricted", () => {
  it("explica a restrição, o efeito na minuta e como pedir acesso", () => {
    render(<FinancialPlanRestricted />);

    expect(screen.getByRole("heading", { name: "Plano financeiro restrito" })).toBeInTheDocument();
    expect(screen.getByText(/não acompanha a configuração financeira desta negociação/)).toBeInTheDocument();
    expect(screen.getByText(/A minuta é liberada quando o plano-base é concluído/)).toBeInTheDocument();
    expect(screen.getByText(/solicite a permissão/)).toBeInTheDocument();
    // Nada do plano em si.
    expect(screen.queryByText("Plano financeiro antes do contrato")).not.toBeInTheDocument();
    expect(screen.queryByText(/Mapa de lançamentos e contas/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Ver projeção estimada/)).not.toBeInTheDocument();
  });
});
