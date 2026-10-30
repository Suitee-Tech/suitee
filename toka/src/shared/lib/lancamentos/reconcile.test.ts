import { describe, expect, it } from "vitest";

import type { Commission } from "../../services/commission.service";
import type { TransactionDTO } from "../../services/transaction.service";
import { mapLancamentos } from "./mappers";
import { reconcileCommissionLedgerTransactions } from "./reconcile";

type EventCommission = Commission & { source_event_id: string };
type EventTransaction = TransactionDTO & { source_event_id: string };

const commission: EventCommission = {
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

const ledgerTransaction: EventTransaction = {
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

describe("reconcileCommissionLedgerTransactions", () => {
  it("keeps one canonical obligation and total when Commission has a matching ledger mirror", () => {
    const lancamentos = mapLancamentos({
      commissions: [commission],
      transactions: [ledgerTransaction],
    });

    expect(lancamentos).toHaveLength(1);
    expect(lancamentos[0]).toMatchObject({ id: "commission-1", kind: "commission", amount: 500 });
    expect(lancamentos.reduce((total, item) => total + item.amount, 0)).toBe(500);
  });

  it("reconciles mirrors one-to-one without hiding an extra transaction", () => {
    const extra = { ...ledgerTransaction, id: "transaction-2" };

    const transactions = reconcileCommissionLedgerTransactions([commission], [ledgerTransaction, extra]);

    expect(transactions.map((transaction) => transaction.id)).toEqual(["transaction-2"]);
  });

  it("preserves a manual transaction even when contract and amount match", () => {
    const manual: EventTransaction = {
      ...ledgerTransaction,
      id: "manual-transaction",
      // crypto.randomUUID() used by the manual flow produces UUID v4.
      source_event_id: "b225c24d-22ad-48cf-b702-218c2094a998",
    };

    expect(reconcileCommissionLedgerTransactions([commission], [manual])).toEqual([manual]);
  });

  it("preserves a generated ledger transaction when no Commission entity exists", () => {
    expect(reconcileCommissionLedgerTransactions([], [ledgerTransaction])).toEqual([ledgerTransaction]);
  });

  it("reconciles the explicit ledger mirror created by an atomic manual posting", () => {
    const manualMirror: TransactionDTO = {
      ...ledgerTransaction,
      id: "742d87cf-85db-5d5f-a165-9e77afbda88a",
      description: "Comissão - atendente (Tiago Santini)",
      category: undefined,
      broker_id: "broker-1",
      role: "fechamento",
      operation_id: "operation-1",
    };
    const linkedCommission: Commission = {
      ...commission,
      ledger_transaction_id: manualMirror.id,
    };

    expect(reconcileCommissionLedgerTransactions([linkedCommission], [manualMirror])).toEqual([]);
  });

  it("reconciles a legacy manual mirror by immutable broker dimensions", () => {
    const manualMirror: TransactionDTO = {
      ...ledgerTransaction,
      id: "742d87cf-85db-5d5f-a165-9e77afbda88a",
      description: "Comissão - fechamento (Tiago Santini)",
      category: undefined,
      broker_id: commission.broker_id,
      role: commission.role,
      operation_id: "operation-1",
    };

    expect(reconcileCommissionLedgerTransactions([commission], [manualMirror])).toEqual([]);
  });
});
