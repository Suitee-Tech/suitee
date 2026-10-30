import { describe, expect, it } from "vitest";

import type { Charge } from "../../services/charge.service";
import type { OwnerTransfer } from "../../services/transfer.service";
import type { Transaction } from "../../types/transaction";
import {
  chargeReceivableAmounts,
  reconcileReceivableTransactions,
  reconcileTransferTransactions,
  transactionReceivableAmounts,
} from "./economicRecords";

const charge = (overrides: Partial<Charge> = {}): Charge => ({
  id: "charge-1",
  tenant_id: "tenant-1",
  source_event_id: "event-1",
  contract_id: "contract-1",
  tenant_party_id: "payer-1",
  owner_id: "owner-1",
  property_id: "property-1",
  type: "RENT",
  description: "Aluguel",
  reference_month: "2026-09",
  amount: 1_000,
  late_fee: 0,
  interest: 0,
  discount: 0,
  total_amount: 1_000,
  due_date: "2026-09-10",
  status: "PENDING",
  created_at: "2026-09-01T00:00:00Z",
  updated_at: "2026-09-01T00:00:00Z",
  ...overrides,
});

const transaction = (overrides: Partial<Transaction> = {}): Transaction => ({
  id: "transaction-1",
  contractId: "contract-1",
  type: "receita",
  description: "Recebível",
  value: 1_000,
  date: "2026-09-01",
  status: "Pendente",
  ...overrides,
});

const transfer = (overrides: Partial<OwnerTransfer> = {}): OwnerTransfer => ({
  id: "transfer-1",
  tenant_id: "tenant-1",
  contract_id: "contract-1",
  owner_id: "owner-1",
  property_id: "property-1",
  reference_month: "2026-09",
  gross_rent: 1_000,
  admin_fee_percent: 10,
  admin_fee_amount: 100,
  maintenance_deductions: 0,
  other_deductions: 0,
  net_amount: 900,
  status: "PENDING",
  created_at: "2026-09-01T00:00:00Z",
  updated_at: "2026-09-01T00:00:00Z",
  ...overrides,
});

describe("reconcileReceivableTransactions", () => {
  it("consolida fatura e espelho de ledger somente por vínculos exatos", () => {
    const result = reconcileReceivableTransactions([charge()], [
      transaction({ id: "invoice-1", recordSource: "invoice", linkedChargeID: "charge-1" }),
      transaction({ id: "ledger-1", recordSource: "transaction", sourceEventID: "event-1" }),
      transaction({ id: "manual-1", recordSource: "transaction" }),
    ]);

    expect(result.transactions.map((item) => item.id)).toEqual(["manual-1"]);
    expect(result.consolidatedTransactionIDs).toEqual(["invoice-1", "ledger-1"]);
    expect(result.unresolvedProjectionIDs).toEqual([]);
  });

  it("mantém projeção sem cobrança carregada como vínculo a apurar", () => {
    const result = reconcileReceivableTransactions([charge()], [
      transaction({ id: "invoice-2", recordSource: "invoice", linkedChargeID: "charge-ausente" }),
    ]);

    expect(result.transactions.map((item) => item.id)).toEqual(["invoice-2"]);
    expect(result.unresolvedProjectionIDs).toEqual(["invoice-2"]);
    expect(result.consolidatedTransactionIDs).toEqual([]);
  });

  it("exclui obrigações e despesas da experiência de recebimentos", () => {
    const result = reconcileReceivableTransactions([], [
      transaction({ id: "expense-1", type: "despesa" }),
      transaction({ id: "commission-1", type: "comissao" }),
      transaction({ id: "transfer-1", type: "repasse" }),
    ]);

    expect(result.transactions).toEqual([]);
    expect(result.excludedNonReceivableIDs).toEqual(["expense-1", "commission-1", "transfer-1"]);
  });
});

describe("receivable amount buckets", () => {
  it("preserva as parcelas paga e pendente de uma cobrança parcial", () => {
    expect(chargeReceivableAmounts(charge({ status: "PARTIALLY_PAID", paid_amount: 250 }))).toEqual({
      paid: 250,
      pending: 750,
      overdue: 0,
    });
  });

  it("separa saldo vencido e exclui fatos encerrados dos totais ativos", () => {
    expect(chargeReceivableAmounts(charge({ status: "OVERDUE", paid_amount: 100 }))).toEqual({
      paid: 100,
      pending: 0,
      overdue: 900,
    });
    expect(chargeReceivableAmounts(charge({ status: "CANCELLED" }))).toEqual({ paid: 0, pending: 0, overdue: 0 });
    expect(transactionReceivableAmounts(transaction({ status: "Estornado" }))).toEqual({ paid: 0, pending: 0, overdue: 0 });
  });
});

describe("reconcileTransferTransactions", () => {
  it("suprime somente o lançamento ligado por source_transaction_id", () => {
    const result = reconcileTransferTransactions(
      [transfer({ source_transaction_id: "legacy-linked" })],
      [
        transaction({ id: "legacy-linked", type: "repasse" }),
        transaction({ id: "legacy-unlinked", type: "repasse" }),
        transaction({ id: "expense", type: "despesa", description: "Outra despesa" }),
      ],
    );

    expect(result.transactions.map((item) => item.id)).toEqual(["legacy-unlinked"]);
    expect(result.consolidatedTransactionIDs).toEqual(["legacy-linked"]);
    expect(result.legacyTransactionIDs).toEqual(["legacy-unlinked"]);
  });
});
