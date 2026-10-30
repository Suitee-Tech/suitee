import { describe, expect, it } from "vitest";

import {
  chargeAgencyCents,
  chargeOpenCents,
  chargeReceivableStatus,
  computeChargePortfolio,
  computeFinanceMonthly,
  computeFinanceTotals,
  monthLabelOf,
} from "./financeTotals";
import type { Charge } from "../../services/charge.service";
import type { Commission } from "../../services/commission.service";
import type { OwnerTransfer } from "../../services/transfer.service";

const window2026 = { from: new Date("2026-01-01T00:00:00"), to: new Date("2026-12-31T23:59:59") };

/** Venda de R$ 1.130.000: proprietário 1.062.200, corretor 40.680, agência 27.120. */
const saleCharge = (status: string, paidAt?: string): Charge =>
  ({
    id: `sale-${status}`,
    status,
    type: "SALE",
    description: "Venda",
    contract_id: "c1",
    tenant_party_id: "p1",
    owner_id: "o1",
    property_id: "pr1",
    reference_month: "2026-09",
    amount: 1130000,
    total_amount: 1130000,
    paid_amount: status === "PAID" ? 1130000 : 0,
    due_date: "2026-09-10",
    paid_at: paidAt,
    created_at: "2026-09-01",
    line_items: [
      { description: "Proprietário", amount_cents: 106220000, allocation_type: "owner_payable" },
      { description: "Corretor", amount_cents: 4068000, allocation_type: "commission_payable" },
      { description: "Agência", amount_cents: 2712000, allocation_type: "revenue" },
    ],
  }) as Charge;

const rentCharge = (status: string, paidAt?: string, due = "2026-09-10"): Charge =>
  ({
    id: `rent-${status}`,
    status,
    type: "RENT",
    description: "Aluguel",
    contract_id: "c2",
    tenant_party_id: "p2",
    owner_id: "o2",
    property_id: "pr2",
    reference_month: "2026-09",
    amount: 1000,
    total_amount: 1000,
    paid_amount: status === "PAID" ? 1000 : 0,
    due_date: due,
    paid_at: paidAt,
    created_at: "2026-09-01",
    line_items: [
      { description: "Proprietário", amount_cents: 90000, allocation_type: "owner_payable" },
      { description: "Agência", amount_cents: 10000, allocation_type: "revenue" },
    ],
  }) as Charge;

const paidCommission = (value: number, paidAt: string): Commission =>
  ({ id: `comm-${paidAt}`, status: "PAID", amount: value, net_amount: value, paid_at: paidAt, created_at: "2026-01-01" }) as Commission;

const processedTransfer = (net: number, processedAt: string): OwnerTransfer =>
  ({ id: `t-${processedAt}`, status: "PROCESSED", net_amount: net, processed_at: processedAt, created_at: "2026-01-01" }) as OwnerTransfer;

describe("financeTotals · fronteira econômica da imobiliária", () => {
  it("conta apenas a parcela própria da agência como receita", () => {
    const charges = [saleCharge("PAID", "2026-09-15T12:00:00Z"), rentCharge("PAID", "2026-09-15T12:00:00Z")];

    // Receita = 27.120 (venda) + 100 (locação). Nunca 1.131.000.
    expect(chargeAgencyCents(saleCharge("PAID"))).toBe(2712000);
    expect(chargeAgencyCents(rentCharge("PAID"))).toBe(10000);

    const totals = computeFinanceTotals(charges, [], [], window2026);
    expect(totals.revenue).toBe(27120 + 100);
    // O valor arrecadado (que pertence a terceiros) fica separado.
    expect(totals.grossReceived).toBe(1131000);
    expect(totals.received).toBe(totals.revenue);
  });

  it("a receber e em atraso usam o saldo aberto, não o valor cheio", () => {
    const charges = [
      rentCharge("PENDING", undefined, "2026-09-10"),
      rentCharge("OVERDUE", undefined, "2026-01-10"),
    ];
    const totals = computeFinanceTotals(charges, [], [], window2026);
    expect(totals.pending).toBe(1000);
    expect(totals.overdue).toBe(1000);
    expect(totals.revenue).toBe(0);
  });

  it("repasse é o líquido processado; comissão paga é despesa", () => {
    const totals = computeFinanceTotals(
      [],
      [processedTransfer(1406.3, "2026-09-20T10:00:00Z")],
      [paidCommission(2980, "2026-09-18T10:00:00Z"), paidCommission(13560, "2026-09-18T10:00:00Z")],
      window2026,
    );
    expect(totals.transfers).toBe(1406.3);
    expect(totals.commissionExpense).toBe(16540);
  });

  it("classifica o status da cobrança para o relatório", () => {
    expect(chargeReceivableStatus(rentCharge("PAID"))).toBe("pago");
    expect(chargeReceivableStatus(rentCharge("PENDING"))).toBe("pendente");
    expect(chargeReceivableStatus(rentCharge("OVERDUE"))).toBe("vencido");
    expect(chargeReceivableStatus(rentCharge("PARTIALLY_PAID"))).toBe("pago");
  });

  it("agrupa a série mensal por mês econômico", () => {
    const monthly = computeFinanceMonthly(
      [rentCharge("PAID", "2026-09-05T10:00:00Z"), saleCharge("PAID", "2026-10-02T10:00:00Z")],
      [paidCommission(1000, "2026-09-06T10:00:00Z")],
      window2026,
    );
    // A série cobre o ano inteiro; o que importa é o valor cair no mês certo.
    const setembro = monthly.find((b) => b.month === "2026-09")!;
    const outubro = monthly.find((b) => b.month === "2026-10")!;
    expect(setembro.revenue).toBe(100);
    expect(setembro.expense).toBe(1000);
    expect(outubro.revenue).toBe(27120);
    expect(monthly.find((b) => b.month === "2026-08")!.revenue).toBe(0);
    expect(monthLabelOf("2026-09")).toBe("Set");
  });

  it("respeita a janela de datas", () => {
    const september = { from: new Date("2026-09-01"), to: new Date("2026-09-30T23:59:59") };
    const totals = computeFinanceTotals(
      [rentCharge("PAID", "2026-08-20T10:00:00Z"), rentCharge("PAID", "2026-09-15T10:00:00Z")],
      [],
      [],
      september,
    );
    expect(totals.revenue).toBe(100);
  });

  // A carteira aberta é fotografia: amarrá-la à janela escondia 29 cobranças
  // em aberto porque venciam fora do período selecionado.
  it("a carteira aberta não depende da janela de datas", () => {
    const future = rentCharge("PENDING", undefined, "2029-03-10");
    const pastDue = rentCharge("OVERDUE", undefined, "2025-01-10");

    const estreita = { from: new Date("2026-09-01"), to: new Date("2026-09-30T23:59:59") };
    const totals = computeFinanceTotals([future, pastDue], [], [], estreita);

    // A janela é só de setembro, mas a carteira inteira aparece.
    expect(totals.pending).toBe(1000);
    expect(totals.overdue).toBe(1000);
    expect(totals.revenue).toBe(0);

    const portfolio = computeChargePortfolio([future, pastDue, rentCharge("PAID", "2026-09-05T10:00:00Z")]);
    expect(portfolio.pending).toBe(1000);
    expect(portfolio.overdue).toBe(1000);
    expect(portfolio.grossReceived).toBe(1000);
  });

  it("a receita recebida continua restrita ao período", () => {
    const dentro = rentCharge("PAID", "2026-09-05T10:00:00Z");
    const fora = rentCharge("PAID", "2026-08-05T10:00:00Z");
    const setembro = { from: new Date("2026-09-01"), to: new Date("2026-09-30T23:59:59") };
    const totals = computeFinanceTotals([dentro, fora], [], [], setembro);
    expect(totals.revenue).toBe(100);
    expect(totals.received).toBe(100);
  });

  // O gráfico mensal precisa da linha do tempo completa: uma barra isolada não
  // situa o mês no ano e parece dado faltando.
  it("a série mensal cobre os doze meses do ano, com zero onde não há movimento", () => {
    const monthly = computeFinanceMonthly(
      [rentCharge("PAID", "2026-09-05T10:00:00Z")],
      [],
      { from: new Date("2026-01-01"), to: new Date("2026-12-31T23:59:59") },
    );
    expect(monthly).toHaveLength(12);
    expect(monthly[0].month).toBe("2026-01");
    expect(monthly[11].month).toBe("2026-12");
    expect(monthly[8].month).toBe("2026-09");
    expect(monthly[8].revenue).toBe(100);
    expect(monthly[0].revenue).toBe(0);
  });
});
