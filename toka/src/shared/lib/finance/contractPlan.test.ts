import { describe, expect, it } from "vitest";

import type { Charge } from "../../services/charge.service";
import type { Invoice } from "../../finance/types";
import { buildContractPlanRows, summarizeContractPlan } from "./contractPlan";

function makeCharge(overrides: Partial<Charge> = {}): Charge {
  return {
    id: overrides.id ?? "charge-1",
    tenant_id: "tenant-1",
    contract_id: "contract-1",
    tenant_party_id: "party-1",
    owner_id: "owner-1",
    property_id: "property-1",
    type: "RENT",
    description: "Aluguel",
    reference_month: "2026-09",
    amount: 3200,
    late_fee: 0,
    interest: 0,
    discount: 0,
    total_amount: 3200,
    due_date: "2026-09-10",
    status: "PENDING",
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
    ...overrides,
  };
}

function makeInvoice(overrides: Partial<Invoice> = {}): Invoice {
  return {
    id: overrides.id ?? "invoice-1",
    reference_id: "deal-1",
    payer_id: "party-1",
    amount: 1000,
    currency: "BRL",
    due_date: "2026-09-20",
    status: "PENDING" as Invoice["status"],
    type: "SALE" as Invoice["type"],
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
    tenant_id: "tenant-1",
    ...overrides,
  };
}

describe("buildContractPlanRows", () => {
  it("monta o plano da locação a partir das cobranças do contrato", () => {
    const rows = buildContractPlanRows({
      source: "rent",
      charges: [
        makeCharge({ id: "c-2", reference_month: "2026-10", due_date: "2026-10-10" }),
        makeCharge({ id: "c-1", reference_month: "2026-09", due_date: "2026-09-10", status: "PAID" }),
      ],
    });

    // Ordem cronológica de vencimento, com competência legível no rótulo.
    expect(rows.map((row) => row.id)).toEqual(["c-1", "c-2"]);
    expect(rows[0].label).toBe("Set/2026");
    expect(rows[0].amount).toBe(3200);
    expect(rows[0].status).toBe("PAID");
    expect(rows[1].label).toBe("Out/2026");
  });

  it("não devolve plano de locação quando só existem faturas do negócio", () => {
    // O bug original: o painel lia `invoices` e mostrava "nenhuma cobrança".
    expect(buildContractPlanRows({ source: "rent", invoices: [makeInvoice()] })).toEqual([]);
  });

  it("usa o total da cobrança, incluindo os itens da composição", () => {
    const [row] = buildContractPlanRows({
      source: "rent",
      charges: [makeCharge({ amount: 3200, total_amount: 4128 })],
    });

    expect(row.amount).toBe(4128);
  });

  it("cai para a descrição quando a cobrança não tem competência", () => {
    const [row] = buildContractPlanRows({
      source: "rent",
      charges: [makeCharge({ reference_month: "", description: "Taxa de vistoria" })],
    });

    expect(row.label).toBe("Taxa de vistoria");
  });

  it("mantém as parcelas numeradas no plano de venda", () => {
    const rows = buildContractPlanRows({
      source: "sale",
      invoices: [makeInvoice({ id: "i-1" }), makeInvoice({ id: "i-2", status: "PAID" as Invoice["status"] })],
    });

    expect(rows.map((row) => row.label)).toEqual(["1/2", "2/2"]);
    expect(rows[1].status).toBe("PAID");
  });
});

describe("summarizeContractPlan", () => {
  it("separa emitido, recebido e em aberto sem perder baixa parcial", () => {
    const totals = summarizeContractPlan([
      makeCharge({ id: "pago", status: "PAID", total_amount: 3200, paid_amount: 3200 }),
      makeCharge({ id: "aberto", status: "PENDING", total_amount: 3200 }),
      makeCharge({ id: "parcial", status: "PARTIALLY_PAID", total_amount: 3200, paid_amount: 800 }),
    ]);

    expect(totals).toEqual({ count: 3, emitted: 9600, received: 4000, open: 5600, overdue: 0 });
  });

  it("destaca o que está em atraso", () => {
    const totals = summarizeContractPlan([
      makeCharge({ id: "atraso", status: "OVERDUE", total_amount: 1500 }),
      makeCharge({ id: "ok", status: "PAID", total_amount: 500, paid_amount: 500 }),
    ]);

    expect(totals.overdue).toBe(1500);
    expect(totals.emitted).toBe(2000);
  });

  it("ignora canceladas e reversas nos indicadores", () => {
    const totals = summarizeContractPlan([
      makeCharge({ id: "cancelada", status: "CANCELLED", total_amount: 3200 }),
      makeCharge({ id: "reversa", status: "REVERSED", total_amount: 3200 }),
      makeCharge({ id: "ativa", status: "PENDING", total_amount: 1000 }),
    ]);

    expect(totals).toEqual({ count: 1, emitted: 1000, received: 0, open: 1000, overdue: 0 });
  });

  it("trata plano vazio sem inventar valores", () => {
    expect(summarizeContractPlan([])).toEqual({ count: 0, emitted: 0, received: 0, open: 0, overdue: 0 });
  });
});
