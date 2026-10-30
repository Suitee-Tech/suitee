import { describe, expect, it } from "vitest";

import { applyFilters, groupByKind } from "../lancamentos";
import { enrichLancamentos } from "../../../shared/lib/lancamentos/presentation";
import {
  chargeToLancamento,
  commissionToLancamento,
  transactionToLancamento,
  transferToLancamento,
} from "../../lib/lancamentos/mappers";
import type { Charge } from "../../services/charge.service";
import type { Commission } from "../../services/commission.service";
import type { OwnerTransfer } from "../../services/transfer.service";
import type { TransactionDTO } from "../../services/transaction.service";

function makeCharge(over: Partial<Charge> = {}): Charge {
  return {
    id: over.id ?? "ch-1",
    tenant_id: "t-1",
    contract_id: "co-1",
    tenant_party_id: "p-1",
    owner_id: "o-1",
    property_id: "pr-1",
    type: "RENT",
    description: over.description ?? "Aluguel",
    reference_month: "2026-05",
    amount: over.amount ?? 1000,
    late_fee: 0,
    interest: 0,
    discount: 0,
    total_amount: over.amount ?? 1000,
    due_date: over.due_date ?? "2026-05-10",
    status: over.status ?? "PENDING",
    created_at: over.created_at ?? "2026-05-01T00:00:00Z",
    updated_at: over.updated_at ?? "2026-05-02T00:00:00Z",
    ...over,
  };
}

function makeCommission(over: Partial<Commission> = {}): Commission {
  return {
    id: over.id ?? "co-1",
    deal_id: "d-1",
    broker_id: over.broker_id ?? "b-1",
    role: "fechamento",
    amount: over.amount ?? 500,
    currency: "BRL",
    status: over.status ?? "PENDING",
    tenant_id: "t-1",
    created_at: over.created_at ?? "2026-05-01T00:00:00Z",
    updated_at: "2026-05-02T00:00:00Z",
    ...over,
  };
}

function makeTransfer(over: Partial<OwnerTransfer> = {}): OwnerTransfer {
  return {
    id: over.id ?? "tr-1",
    tenant_id: "t-1",
    contract_id: "co-1",
    owner_id: "o-1",
    property_id: "pr-1",
    reference_month: "2026-05",
    gross_rent: over.gross_rent ?? 3000,
    admin_fee_percent: 10,
    admin_fee_amount: 300,
    maintenance_deductions: 0,
    other_deductions: 0,
    net_amount: 2700,
    status: over.status ?? "PENDING",
    created_at: over.created_at ?? "2026-05-01T00:00:00Z",
    updated_at: "2026-05-02T00:00:00Z",
    ...over,
  };
}

function makeTransaction(over: Partial<TransactionDTO> = {}): TransactionDTO {
  return {
    id: over.id ?? "tx-1",
    contract_id: over.contract_id ?? "",
    type: over.type ?? "receita",
    description: over.description ?? "Lançamento de caixa",
    value: over.value ?? 100,
    date: over.date ?? "2026-05-10",
    status: over.status ?? "PENDING",
    tenant_id: "t-1",
    created_at: over.created_at ?? "2026-05-01T00:00:00Z",
    updated_at: over.updated_at ?? "2026-05-02T00:00:00Z",
    ...over,
  };
}

describe("applyFilters", () => {
  const lancamentos = [
    chargeToLancamento(makeCharge({ id: "ch-1", status: "PENDING", description: "Aluguel Centro" })),
    chargeToLancamento(makeCharge({ id: "ch-2", status: "PAID", description: "Aluguel Norte" })),
    commissionToLancamento(makeCommission({ id: "co-1", status: "APPROVED" })),
    transferToLancamento(makeTransfer({ id: "tr-1", status: "PROCESSED" })),
  ];

  it("returns everything when filters are empty", () => {
    expect(applyFilters(lancamentos, {})).toHaveLength(4);
  });

  it("filters by kind", () => {
    const out = applyFilters(lancamentos, { kinds: ["charge"] });
    expect(out).toHaveLength(2);
    expect(out.every((l) => l.kind === "charge")).toBe(true);
  });

  it("filters by status", () => {
    const out = applyFilters(lancamentos, { statuses: ["pago", "processado"] });
    expect(out.map((l) => l.id).sort()).toEqual(["ch-2", "tr-1"]);
  });

  it("filters by combined kind + status", () => {
    const out = applyFilters(lancamentos, {
      kinds: ["charge"],
      statuses: ["pendente"],
    });
    expect(out).toHaveLength(1);
    expect(out[0].id).toBe("ch-1");
  });

  it("filters by case-insensitive search against id and counterparty.label", () => {
    const out = applyFilters(lancamentos, { search: "CENTRO" });
    expect(out).toHaveLength(1);
    expect(out[0].id).toBe("ch-1");
  });

  it("filters by date range against due_at when available", () => {
    const out = applyFilters(lancamentos, {
      from: "2026-05-09",
      to: "2026-05-11",
    });
    // both charges have due_date 2026-05-10; commission/transfer have no due_at
    // and fall back to created_at 2026-05-01 (out of range).
    expect(out.map((l) => l.id).sort()).toEqual(["ch-1", "ch-2"]);
  });

  it("filters paid items by settlement date and includes the whole final day", () => {
    const paidAtEndOfDay = commissionToLancamento(makeCommission({
      id: "paid-in-august",
      status: "PAID",
      paid_at: "2026-08-31T22:45:00Z",
      created_at: "2026-07-10T10:00:00Z",
    }));
    const unpaid = commissionToLancamento(makeCommission({
      id: "not-settled",
      status: "APPROVED",
      created_at: "2026-08-15T10:00:00Z",
    }));

    const out = applyFilters([paidAtEndOfDay, unpaid], {
      from: "2026-08-01",
      to: "2026-08-31",
      dateBasis: "settled",
    });

    expect(out.map((item) => item.id)).toEqual(["paid-in-august"]);
  });

  it("filters by competence, including the bucket without competence", () => {
    // A competência vem do enriquecimento (reference_month da origem), como na
    // página: filtrar antes de enriquecer cairia na data-chave.
    const enriched = enrichLancamentos(
      [
        chargeToLancamento(makeCharge({ id: "ch-set", reference_month: "2026-09" })),
        chargeToLancamento(makeCharge({ id: "ch-ago", reference_month: "2026-08" })),
        commissionToLancamento(makeCommission({ id: "co-sem-data", created_at: "", updated_at: "" })),
      ],
      {},
    );

    expect(applyFilters(enriched, { competence: "2026-09" }).map((l) => l.id)).toEqual(["ch-set"]);
    expect(applyFilters(enriched, { competence: "none" }).map((l) => l.id)).toEqual(["co-sem-data"]);
  });

  it("filters by due month and separates what has no open due date", () => {
    const dueSeptember = chargeToLancamento(makeCharge({ id: "ch-due-set", due_date: "2026-09-10", status: "PENDING" }));
    const dueOctober = chargeToLancamento(makeCharge({ id: "ch-due-out", due_date: "2026-10-10", status: "PENDING" }));
    const withoutDue = commissionToLancamento(makeCommission({ id: "co-sem-vencimento", status: "APPROVED" }));

    expect(applyFilters([dueSeptember, dueOctober, withoutDue], { dueMonth: "2026-09" }).map((l) => l.id)).toEqual(["ch-due-set"]);
    expect(applyFilters([dueSeptember, dueOctober, withoutDue], { dueMonth: "none" }).map((l) => l.id)).toEqual(["co-sem-vencimento"]);
  });

  it("keeps only overdue items with the overdue shortcut", () => {
    const overdue = chargeToLancamento(makeCharge({ id: "ch-vencido", status: "OVERDUE" }));
    const pending = chargeToLancamento(makeCharge({ id: "ch-pendente", status: "PENDING" }));

    expect(applyFilters([overdue, pending], { overdueOnly: true }).map((l) => l.id)).toEqual(["ch-vencido"]);
  });

  it("returns empty when no item matches", () => {
    expect(
      applyFilters(lancamentos, { search: "string-improvavel-de-existir" }),
    ).toEqual([]);
  });

  it("filters by counterparty.id (deep link from CounterpartyLancamentosLink)", () => {
    const out = applyFilters(lancamentos, {
      counterparty: { id: "p-1" },
    });
    // both charges share tenant_party_id p-1; commission has broker_id b-1,
    // transfer has owner_id o-1
    expect(out.map((l) => l.id).sort()).toEqual(["ch-1", "ch-2"]);
  });

  it("filters by counterparty.id + kind narrows further", () => {
    const out = applyFilters(lancamentos, {
      counterparty: { id: "b-1", kind: "broker" },
    });
    expect(out).toHaveLength(1);
    expect(out[0].kind).toBe("commission");
  });

  it("returns empty when counterparty.id matches but kind mismatch", () => {
    const out = applyFilters(lancamentos, {
      counterparty: { id: "p-1", kind: "owner" },
    });
    expect(out).toEqual([]);
  });

  it("filters receiving flow across charges and cash inflows", () => {
    const input = [
      ...lancamentos,
      transactionToLancamento(makeTransaction({ id: "in-1", type: "receita" })),
      transactionToLancamento(makeTransaction({ id: "out-1", type: "despesa" })),
    ];

    const out = applyFilters(input, { flow: "receber" });

    expect(out.map((item) => item.id).sort()).toEqual(["ch-1", "ch-2", "in-1"]);
  });

  it("filters payment flow across payables and cash outflows", () => {
    const input = [
      ...lancamentos,
      transactionToLancamento(makeTransaction({ id: "in-1", type: "receita" })),
      transactionToLancamento(makeTransaction({ id: "out-1", type: "despesa" })),
    ];

    const out = applyFilters(input, { flow: "pagar" });

    expect(out.map((item) => item.id).sort()).toEqual(["co-1", "out-1", "tr-1"]);
  });
});

describe("groupByKind", () => {
  it("partitions lancamentos into the 4 kinds", () => {
    const lancamentos = [
      chargeToLancamento(makeCharge({ id: "ch-1" })),
      chargeToLancamento(makeCharge({ id: "ch-2" })),
      commissionToLancamento(makeCommission({ id: "co-1" })),
      transferToLancamento(makeTransfer({ id: "tr-1" })),
    ];
    const grouped = groupByKind(lancamentos);
    expect(grouped.charge).toHaveLength(2);
    expect(grouped.commission).toHaveLength(1);
    expect(grouped.transfer).toHaveLength(1);
    expect(grouped.royalty).toHaveLength(0);
  });

  it("returns all empty arrays when input is empty", () => {
    const grouped = groupByKind([]);
    expect(grouped.charge).toEqual([]);
    expect(grouped.commission).toEqual([]);
    expect(grouped.transfer).toEqual([]);
    expect(grouped.royalty).toEqual([]);
  });
});
