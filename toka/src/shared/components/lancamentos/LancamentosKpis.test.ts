import { describe, expect, it } from "vitest";

import { mapLancamentos } from "../../lib/lancamentos/mappers";
import type { Commission } from "../../services/commission.service";
import type { TransactionDTO } from "../../services/transaction.service";
import { classifyLancamentos } from "./LancamentosKpis";

const commission: Commission & { source_event_id: string } = {
  id: "commission-1",
  deal_id: "deal-1",
  broker_id: "broker-1",
  role: "fechamento",
  percentage: 50,
  amount: 500,
  currency: "BRL",
  status: "PENDING",
  contract_id: "contract-1",
  source_event_id: "284a8ad8-5119-5e6d-bf88-829be78df99b",
  tenant_id: "tenant-1",
  created_at: "2026-08-06T10:00:00Z",
  updated_at: "2026-08-06T10:00:00Z",
};

const ledgerTransaction: TransactionDTO & { source_event_id: string } = {
  id: "transaction-1",
  contract_id: "contract-1",
  type: "comissao",
  description: "Comissão - fechamento (50.0%)",
  value: 500,
  date: "2026-08-06T10:00:00Z",
  status: "Pendente",
  category: "Comissão",
  source_event_id: "d670b8e8-c7b2-57d7-9b35-218216f23c29",
  tenant_id: "tenant-1",
  created_at: "2026-08-06T10:00:00Z",
  updated_at: "2026-08-06T10:00:00Z",
};

describe("classifyLancamentos", () => {
  it("classifies an open commission as A PAGAR, not A RECEBER", () => {
    const reconciled = mapLancamentos({
      commissions: [commission],
      transactions: [ledgerTransaction],
    });
    const result = classifyLancamentos(reconciled);

    expect(reconciled).toHaveLength(1);
    expect(result.aPagar).toEqual({ count: 1, amount: 500 });
    expect(result.aReceber).toEqual({ count: 0, amount: 0 });
  });

  it("includes open cash inflows in A RECEBER and outflows in A PAGAR", () => {
    const inflow: TransactionDTO = {
      ...ledgerTransaction,
      id: "inflow-1",
      type: "receita",
      description: "Entrada manual",
      value: 1200,
      contract_id: "",
      source_event_id: undefined,
    };
    const outflow: TransactionDTO = {
      ...ledgerTransaction,
      id: "outflow-1",
      type: "despesa",
      description: "Saída manual",
      value: 300,
      contract_id: "",
      source_event_id: undefined,
    };

    const result = classifyLancamentos(mapLancamentos({ transactions: [inflow, outflow] }));

    expect(result.aReceber).toEqual({ count: 1, amount: 1200 });
    expect(result.aPagar).toEqual({ count: 1, amount: 300 });
  });

  it("separates nominal value under review from the calculated difference", () => {
    const paidWithoutProof: Commission = {
      ...commission,
      id: "commission-without-proof",
      status: "PAID",
      gross_amount: 500,
      net_amount: 500,
      paid_at: "2026-08-07T10:00:00Z",
    };
    const amountMismatch: Commission = {
      ...commission,
      id: "commission-amount-mismatch",
      status: "APPROVED",
      gross_amount: 500,
      deductions: [{ kind: "OUTROS", amount: 100 }],
      net_amount: 380,
    };

    const result = classifyLancamentos(
      mapLancamentos({ commissions: [paidWithoutProof, amountMismatch] }),
    );

    expect(result.divergencias).toEqual({
      count: 2,
      amount: 1000,
      differenceAmount: 20,
    });
  });
});
