import type { Commission } from "../../services/commission.service";
import type { TransactionDTO } from "../../services/transaction.service";

type EventLinked = {
  source_event_id?: unknown;
};

const UUID_V5_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const SHARE_LEDGER_DESCRIPTION_RE = /^Comissão - (.+) \((-?\d+(?:\.\d+)?)%\)$/;
const TOTAL_LEDGER_DESCRIPTION_RE = /^Comissão Total \((-?\d+(?:\.\d+)?)%\)$/;

interface LedgerDescription {
  role: string;
  percentageTenths: number;
}

function explicitLedgerTransactionID(commission: Commission): string | null {
  const value = commission.ledger_transaction_id?.trim();
  return value || null;
}

function generatedSourceEventID(value: unknown): string | null {
  if (!value || typeof value !== "object") return null;
  const sourceEventID = (value as EventLinked).source_event_id;
  if (typeof sourceEventID !== "string") return null;
  const normalized = sourceEventID.trim();
  return UUID_V5_RE.test(normalized) ? normalized : null;
}

function normalizeRole(value: string): string {
  return value.trim().toLocaleLowerCase("pt-BR");
}

function amountCents(value: number): number | null {
  return Number.isFinite(value) ? Math.round(value * 100) : null;
}

function percentageTenths(value: number | undefined): number | null {
  return typeof value === "number" && Number.isFinite(value) ? Math.round(value * 10) : null;
}

function parseGeneratedLedgerDescription(description: string): LedgerDescription | null {
  const share = SHARE_LEDGER_DESCRIPTION_RE.exec(description);
  if (share) {
    const percentage = Number(share[2]);
    if (!Number.isFinite(percentage)) return null;
    return {
      role: normalizeRole(share[1]),
      percentageTenths: Math.round(percentage * 10),
    };
  }

  const total = TOTAL_LEDGER_DESCRIPTION_RE.exec(description);
  if (!total) return null;
  const percentage = Number(total[1]);
  if (!Number.isFinite(percentage)) return null;
  return {
    role: "total",
    percentageTenths: Math.round(percentage * 10),
  };
}

function fingerprint(parts: {
  tenantID: string;
  contractID: string;
  role: string;
  percentageTenths: number;
  amountCents: number;
}): string {
  return [parts.tenantID, parts.contractID, parts.role, parts.percentageTenths, parts.amountCents].join("\u001f");
}

function commissionFingerprint(commission: Commission): string | null {
  if (!generatedSourceEventID(commission)) return null;

  const tenantID = commission.tenant_id?.trim();
  const contractID = commission.contract_id?.trim();
  const role = commission.role ? normalizeRole(commission.role) : "";
  const percentage = percentageTenths(commission.percentage);
  const amount = amountCents(commission.amount);

  if (!tenantID || !contractID || !role || percentage === null || amount === null) return null;
  return fingerprint({ tenantID, contractID, role, percentageTenths: percentage, amountCents: amount });
}

function transactionFingerprint(transaction: TransactionDTO): string | null {
  if (transaction.type !== "comissao" || transaction.category !== "Comissão") return null;
  if (!generatedSourceEventID(transaction)) return null;

  const tenantID = transaction.tenant_id?.trim();
  const contractID = transaction.contract_id?.trim();
  const parsed = parseGeneratedLedgerDescription(transaction.description);
  const amount = amountCents(transaction.value);

  if (!tenantID || !contractID || !parsed || amount === null) return null;
  return fingerprint({
    tenantID,
    contractID,
    role: parsed.role,
    percentageTenths: parsed.percentageTenths,
    amountCents: amount,
  });
}

function dimensionFingerprint(parts: {
  tenantID: string;
  contractID: string;
  brokerID: string;
  role: string;
  amountCents: number;
}): string {
  return [parts.tenantID, parts.contractID, parts.brokerID, parts.role, parts.amountCents].join("\u001f");
}

function commissionDimensionFingerprint(commission: Commission): string | null {
  if (!generatedSourceEventID(commission)) return null;
  const tenantID = commission.tenant_id?.trim();
  const contractID = commission.contract_id?.trim();
  const brokerID = commission.broker_id?.trim();
  const role = commission.role ? normalizeRole(commission.role) : "";
  const amount = amountCents(commission.amount);
  if (!tenantID || !contractID || !brokerID || !role || amount === null) return null;
  return dimensionFingerprint({ tenantID, contractID, brokerID, role, amountCents: amount });
}

function transactionDimensionFingerprint(transaction: TransactionDTO): string | null {
  if (transaction.type !== "comissao") return null;
  // Manual posting mirrors have a deterministic source/posting ID and a
  // stable operation. Requiring those server-owned dimensions keeps an
  // ordinary ad-hoc commission transaction visible in the ledger.
  const generated = generatedSourceEventID(transaction) || (UUID_V5_RE.test(transaction.id) && transaction.operation_id?.trim());
  if (!generated) return null;
  const tenantID = transaction.tenant_id?.trim();
  const contractID = transaction.contract_id?.trim();
  const brokerID = transaction.broker_id?.trim();
  const role = transaction.role ? normalizeRole(transaction.role) : "";
  const amount = amountCents(transaction.value);
  if (!tenantID || !contractID || !brokerID || !role || amount === null) return null;
  return dimensionFingerprint({ tenantID, contractID, brokerID, role, amountCents: amount });
}

/**
 * Removes only the Transaction ledger rows that can be identified as mirrors
 * of Commission entities generated by the same contract-signing pipeline.
 *
 * Matching is one-to-one. A manual transaction is preserved unless it carries
 * the complete backend-generated signature (UUID v5 source, generated label,
 * category, contract, tenant, role, percentage and amount).
 */
export function reconcileCommissionLedgerTransactions(
  commissions: Commission[] = [],
  transactions: TransactionDTO[] = [],
): TransactionDTO[] {
  const explicitLedgerIDs = new Map<string, number>();
  const unmatchedCommissions = new Map<string, number>();
  const unmatchedDimensions = new Map<string, number>();

  for (const commission of commissions) {
    const ledgerID = explicitLedgerTransactionID(commission);
    if (ledgerID) explicitLedgerIDs.set(ledgerID, (explicitLedgerIDs.get(ledgerID) ?? 0) + 1);

    const key = commissionFingerprint(commission);
    if (key) unmatchedCommissions.set(key, (unmatchedCommissions.get(key) ?? 0) + 1);

    const dimensions = commissionDimensionFingerprint(commission);
    if (dimensions) unmatchedDimensions.set(dimensions, (unmatchedDimensions.get(dimensions) ?? 0) + 1);
  }

  return transactions.filter((transaction) => {
    const explicit = explicitLedgerIDs.get(transaction.id) ?? 0;
    if (explicit > 0) {
      if (explicit === 1) explicitLedgerIDs.delete(transaction.id);
      else explicitLedgerIDs.set(transaction.id, explicit - 1);
      return false;
    }

    const key = transactionFingerprint(transaction);
    if (key) {
      const unmatched = unmatchedCommissions.get(key) ?? 0;
      if (unmatched > 0) {
        if (unmatched === 1) unmatchedCommissions.delete(key);
        else unmatchedCommissions.set(key, unmatched - 1);
        return false;
      }
    }

    const dimensions = transactionDimensionFingerprint(transaction);
    if (!dimensions) return true;
    const unmatched = unmatchedDimensions.get(dimensions) ?? 0;
    if (unmatched === 0) return true;
    if (unmatched === 1) unmatchedDimensions.delete(dimensions);
    else unmatchedDimensions.set(dimensions, unmatched - 1);
    return false;
  });
}
