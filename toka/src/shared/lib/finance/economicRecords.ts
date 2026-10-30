import type { Charge } from "../../services/charge.service";
import type { OwnerTransfer } from "../../services/transfer.service";
import type { Transaction } from "../../types/transaction";
import { normalizeStatus } from "../lancamentos/status";

const cleanID = (value: string | null | undefined): string | null => {
  const normalized = value?.trim();
  return normalized ? normalized : null;
};

export interface ReceivableReconciliation {
  /** Recebíveis independentes que continuam visíveis ao lado das cobranças. */
  transactions: Transaction[];
  /** IDs de projeções/espelhos suprimidos porque o vínculo canônico é exato. */
  consolidatedTransactionIDs: string[];
  /** Projeções de fatura cujo charge_id não veio no lote de cobranças. */
  unresolvedProjectionIDs: string[];
  /** Despesas, comissões e repasses não pertencem ao fluxo de recebimentos. */
  excludedNonReceivableIDs: string[];
}

export interface ReceivableAmountBuckets {
  paid: number;
  pending: number;
  overdue: number;
}

const emptyAmountBuckets = (): ReceivableAmountBuckets => ({ paid: 0, pending: 0, overdue: 0 });
const positiveAmount = (value: number | null | undefined): number =>
  typeof value === "number" && Number.isFinite(value) ? Math.max(0, value) : 0;

/** Divide uma cobrança nos valores já pagos e ainda em aberto sem perder o
 * trecho pago de uma baixa parcial. Cancelamentos e reversões não compõem os
 * indicadores do fluxo ativo. */
export function chargeReceivableAmounts(charge: Charge): ReceivableAmountBuckets {
  const status = String(charge.status).trim().toUpperCase();
  if (status === "CANCELLED" || status === "REVERSED") return emptyAmountBuckets();

  const total = positiveAmount(charge.total_amount);
  const reportedPaid = positiveAmount(charge.paid_amount);
  const paid = Math.min(total, status === "PAID" && reportedPaid === 0 ? total : reportedPaid);
  const remaining = Math.max(0, total - paid);

  if (status === "OVERDUE") return { paid, pending: 0, overdue: remaining };
  if (status === "PAID") return { paid, pending: 0, overdue: 0 };
  return { paid, pending: remaining, overdue: 0 };
}

/** Classifica um recebível independente nos mesmos bolsões usados pelas
 * cobranças. Status desconhecido fica fora dos totais em vez de ser presumido. */
export function transactionReceivableAmounts(transaction: Transaction): ReceivableAmountBuckets {
  const amount = Math.abs(Number.isFinite(transaction.value) ? transaction.value : 0);
  const status = normalizeStatus("transaction", transaction.status);
  if (status === "pago") return { paid: amount, pending: 0, overdue: 0 };
  if (status === "pendente" || status === "parcial") return { paid: 0, pending: amount, overdue: 0 };
  if (status === "vencido") return { paid: 0, pending: 0, overdue: amount };
  return emptyAmountBuckets();
}

/**
 * Concilia a apresentação de recebíveis sem heurísticas financeiras.
 *
 * Uma Charge é o agregado canônico. Invoice e Transaction podem ser apenas
 * projeções do mesmo fato econômico; elas só são ocultadas quando charge_id ou
 * source_event_id provam esse vínculo. Ausência de identidade mantém o registro
 * visível e explicitamente "a apurar" para não apagar histórico legítimo.
 */
export function reconcileReceivableTransactions(
  charges: Charge[],
  transactions: Transaction[],
): ReceivableReconciliation {
  const chargeIDs = new Set(charges.map((charge) => cleanID(charge.id)).filter((id): id is string => id !== null));
  const chargeSourceEventIDs = new Set(
    charges.map((charge) => cleanID(charge.source_event_id)).filter((id): id is string => id !== null),
  );

  const visible: Transaction[] = [];
  const consolidatedTransactionIDs: string[] = [];
  const unresolvedProjectionIDs: string[] = [];
  const excludedNonReceivableIDs: string[] = [];

  for (const transaction of transactions) {
    if (transaction.type !== "receita") {
      excludedNonReceivableIDs.push(transaction.id);
      continue;
    }

    const linkedChargeID = cleanID(transaction.linkedChargeID);
    if (transaction.recordSource === "invoice") {
      if (linkedChargeID && chargeIDs.has(linkedChargeID)) {
        consolidatedTransactionIDs.push(transaction.id);
        continue;
      }
      unresolvedProjectionIDs.push(transaction.id);
      visible.push(transaction);
      continue;
    }

    const sourceEventID = cleanID(transaction.sourceEventID);
    if (sourceEventID && chargeSourceEventIDs.has(sourceEventID)) {
      consolidatedTransactionIDs.push(transaction.id);
      continue;
    }

    visible.push(transaction);
  }

  return {
    transactions: visible,
    consolidatedTransactionIDs,
    unresolvedProjectionIDs,
    excludedNonReceivableIDs,
  };
}

export interface TransferReconciliation {
  transactions: Transaction[];
  consolidatedTransactionIDs: string[];
  legacyTransactionIDs: string[];
}

/**
 * Separa repasses canônicos de lançamentos legados. Só elimina uma linha
 * legada quando o OwnerTransfer aponta para aquele transaction_id exato.
 * Demais linhas permanecem visíveis como "A apurar" e fora dos totais
 * canônicos, evitando uma soma que não pode ser provada.
 */
export function reconcileTransferTransactions(
  transfers: OwnerTransfer[],
  transactions: Transaction[],
): TransferReconciliation {
  const sourceTransactionIDs = new Set(
    transfers.map((transfer) => cleanID(transfer.source_transaction_id)).filter((id): id is string => id !== null),
  );
  const candidates = transactions.filter(
    (transaction) =>
      transaction.type === "repasse" ||
      (transaction.type === "despesa" && transaction.description.toLowerCase().includes("repasse")),
  );

  const consolidatedTransactionIDs: string[] = [];
  const legacyTransactionIDs: string[] = [];
  const visible = candidates.filter((transaction) => {
    if (sourceTransactionIDs.has(transaction.id)) {
      consolidatedTransactionIDs.push(transaction.id);
      return false;
    }
    legacyTransactionIDs.push(transaction.id);
    return true;
  });

  return {
    transactions: visible,
    consolidatedTransactionIDs,
    legacyTransactionIDs,
  };
}
