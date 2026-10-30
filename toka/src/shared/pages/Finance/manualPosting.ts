import type {
  CreateTransactionBatchItem,
  ManualCommissionBatchItem,
  ManualCommissionRetention,
} from "../../services/transaction.service";

const ATTEMPT_STORAGE_VERSION = 1 as const;
const ATTEMPT_STORAGE_PREFIX = "finance_manual_posting_attempts";

export type ManualPostingAttempt = {
  intentFingerprint: string;
  operationID: string;
  createdAt: number;
  items: CreateTransactionBatchItem[];
  commissions: ManualCommissionBatchItem[];
  commissionRetention?: ManualCommissionRetention;
};

type ManualPostingAttemptEnvelope = {
  version: typeof ATTEMPT_STORAGE_VERSION;
  attempts: ManualPostingAttempt[];
};

export function manualPostingAttemptStorageKey(tenantID: string): string {
  const tenant = tenantID.trim();
  if (!tenant) throw new Error("tenant is required to persist a financial posting attempt");
  return `${ATTEMPT_STORAGE_PREFIX}:v${ATTEMPT_STORAGE_VERSION}:${encodeURIComponent(tenant)}`;
}

export function localPostingDate(now = new Date()): string {
  const year = now.getFullYear();
  const month = String(now.getMonth() + 1).padStart(2, "0");
  const day = String(now.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

export function manualPostingIntentFingerprint(
  items: CreateTransactionBatchItem[],
  commissions: ManualCommissionBatchItem[],
  commissionRetention?: ManualCommissionRetention,
): string {
  return JSON.stringify({
    items: items.map(({ date: _date, ...economicFields }) => economicFields),
    commissions,
    ...(commissionRetention ? { commissionRetention } : {}),
  });
}

export function allocateCentsByPercentage(totalCents: number, percentages: number[]): number[] {
  if (!Number.isSafeInteger(totalCents) || totalCents < 0) {
    throw new Error("total cents must be a non-negative safe integer");
  }
  if (percentages.length === 0) return [];
  if (percentages.some((percentage) => !Number.isFinite(percentage) || percentage <= 0)) {
    throw new Error("distribution percentages must be positive and finite");
  }
  const totalPercentage = percentages.reduce((sum, percentage) => sum + percentage, 0);
  if (!Number.isFinite(totalPercentage) || totalPercentage <= 0) {
    throw new Error("distribution percentage total must be positive");
  }

  const exact = percentages.map((percentage) => (totalCents * percentage) / totalPercentage);
  const allocated = exact.map(Math.floor);
  let remainder = totalCents - allocated.reduce((sum, cents) => sum + cents, 0);
  const remainderOrder = exact
    .map((value, index) => ({ index, fraction: value - allocated[index] }))
    .sort((left, right) => right.fraction - left.fraction || left.index - right.index);
  for (let i = 0; i < remainder; i += 1) {
    allocated[remainderOrder[i % remainderOrder.length].index] += 1;
  }
  return allocated;
}

const COMMISSION_PERCENTAGE_TOLERANCE = 0.01;

const roundCommissionPercentage = (value: number): number =>
  Math.round(value * 10_000) / 10_000;

const PAYABLE_BROKER_COMMISSION_ROLES = new Set([
  "captador",
  "co_captador",
  "visitador",
  "atendente",
  "fechamento",
]);

const normalizeCommissionRole = (role: unknown): string => {
  if (typeof role !== "string") return "";
  return role
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .trim()
    .toLowerCase();
};

export type ManualCommissionDistribution = {
  brokerPercentage: number;
  agencyPercentage: number;
  totalPercentage: number;
  invalid: boolean;
  overallocated: boolean;
};

/**
 * The agency is the economic residual of a commission distribution, not a
 * broker counterparty. Old previews may still contain an explicit agency row;
 * callers use this helper to remove that row from broker selection/posting.
 */
export function isAgencyCommissionRole(role: unknown): boolean {
  return normalizeCommissionRole(role) === "imobiliaria";
}

export function isPayableBrokerCommissionRole(role: unknown): boolean {
  return PAYABLE_BROKER_COMMISSION_ROLES.has(normalizeCommissionRole(role));
}

export function payableBrokerCommissionEntries<
  T extends { role?: unknown; retained?: boolean },
>(entries: T[]): T[] {
  return entries.filter(
    (entry) => !entry.retained && isPayableBrokerCommissionRole(entry.role),
  );
}

export function calculateManualCommissionDistribution(
  brokerPercentages: number[],
): ManualCommissionDistribution {
  const invalid = brokerPercentages.some(
    (percentage) =>
      !Number.isFinite(percentage) || percentage < 0 || percentage > 100,
  );
  const calculatedBrokerPercentage = roundCommissionPercentage(
    brokerPercentages.reduce(
      (sum, percentage) => sum + (Number.isFinite(percentage) ? percentage : 0),
      0,
    ),
  );
  const overallocated =
    !invalid && calculatedBrokerPercentage - 100 > COMMISSION_PERCENTAGE_TOLERANCE;
  // Decimal input and API arithmetic may land immediately above 100. Clamp
  // only that tiny excess; any value below 100 is a real agency remainder and
  // must stay explicit even when it is within the validation tolerance.
  const brokerPercentage =
    !invalid &&
    !overallocated &&
    calculatedBrokerPercentage > 100 &&
    calculatedBrokerPercentage - 100 <= COMMISSION_PERCENTAGE_TOLERANCE
      ? 100
      : calculatedBrokerPercentage;
  const agencyPercentage =
    invalid || overallocated
      ? 0
      : roundCommissionPercentage(Math.max(0, 100 - brokerPercentage));

  return {
    brokerPercentage,
    agencyPercentage,
    totalPercentage:
      invalid || overallocated
        ? brokerPercentage
        : roundCommissionPercentage(brokerPercentage + agencyPercentage),
    invalid,
    overallocated,
  };
}

/**
 * Allocates against an implicit 100% agency remainder, then returns only the
 * broker portions as payable commissions. This avoids normalizing an 80%
 * broker split into 100% and accidentally paying away the agency's 20%.
 */
export function allocateManualCommissionCents(
  totalCents: number,
  brokerPercentages: number[],
): { brokerPoolCents: number; brokerCents: number[]; agencyCents: number } {
  if (!Number.isSafeInteger(totalCents) || totalCents < 0) {
    throw new Error("total cents must be a non-negative safe integer");
  }
  const distribution = calculateManualCommissionDistribution(brokerPercentages);
  if (distribution.invalid) {
    throw new Error("broker percentages must be finite values between 0 and 100");
  }
  if (distribution.overallocated) {
    throw new Error("broker percentages cannot exceed 100");
  }
  if (brokerPercentages.some((percentage) => percentage <= 0)) {
    throw new Error("broker percentages must be positive");
  }
  if (brokerPercentages.length === 0) {
    return { brokerPoolCents: 0, brokerCents: [], agencyCents: totalCents };
  }

  const brokerPoolCents = Math.min(
    totalCents,
    Math.round(totalCents * (distribution.brokerPercentage / 100)),
  );
  const brokerCents = allocateCentsByPercentage(
    brokerPoolCents,
    brokerPercentages,
  );
  return {
    brokerPoolCents,
    brokerCents,
    agencyCents: totalCents - brokerPoolCents,
  };
}

function isBatchItem(value: unknown): value is CreateTransactionBatchItem {
  if (!value || typeof value !== "object") return false;
  const item = value as Partial<CreateTransactionBatchItem>;
  return typeof item.contract_id === "string" &&
    typeof item.type === "string" &&
    typeof item.description === "string" &&
    typeof item.value === "number" && Number.isFinite(item.value) &&
    typeof item.date === "string";
}

function isCommission(value: unknown): value is ManualCommissionBatchItem {
  if (!value || typeof value !== "object") return false;
  const item = value as Partial<ManualCommissionBatchItem>;
  return typeof item.deal_id === "string" &&
    typeof item.broker_id === "string" &&
    typeof item.amount === "number" && Number.isFinite(item.amount);
}

function isCommissionRetention(value: unknown): value is ManualCommissionRetention {
  if (!value || typeof value !== "object") return false;
  const retention = value as Partial<ManualCommissionRetention>;
  return typeof retention.percentage === "number" &&
    Number.isFinite(retention.percentage) &&
    retention.percentage > 0 &&
    retention.percentage <= 100 &&
    typeof retention.amount === "number" &&
    Number.isFinite(retention.amount) &&
    retention.amount >= 0;
}

function isAttempt(value: unknown): value is ManualPostingAttempt {
  if (!value || typeof value !== "object") return false;
  const attempt = value as Partial<ManualPostingAttempt>;
  if (typeof attempt.intentFingerprint !== "string" ||
      typeof attempt.operationID !== "string" ||
      typeof attempt.createdAt !== "number" ||
      !Array.isArray(attempt.items) || !attempt.items.every(isBatchItem) ||
      !Array.isArray(attempt.commissions) || !attempt.commissions.every(isCommission) ||
      (attempt.commissionRetention !== undefined && !isCommissionRetention(attempt.commissionRetention))) {
    return false;
  }
  return manualPostingIntentFingerprint(
    attempt.items,
    attempt.commissions,
    attempt.commissionRetention,
  ) === attempt.intentFingerprint;
}

function readEnvelope(tenantID: string, storage: Storage): ManualPostingAttemptEnvelope {
  const key = manualPostingAttemptStorageKey(tenantID);
  const raw = storage.getItem(key);
  if (!raw) return { version: ATTEMPT_STORAGE_VERSION, attempts: [] };
  try {
    const parsed = JSON.parse(raw) as Partial<ManualPostingAttemptEnvelope>;
    if (parsed.version !== ATTEMPT_STORAGE_VERSION ||
        !Array.isArray(parsed.attempts) ||
        !parsed.attempts.every(isAttempt)) {
      throw new Error("invalid manual posting attempt envelope");
    }
    return parsed as ManualPostingAttemptEnvelope;
  } catch (error) {
    // An unreadable record may represent a request whose response was lost.
    // Deleting it and minting a new key could duplicate a committed posting,
    // so persistence corruption is deliberately fail-closed.
    const detail = error instanceof Error ? `: ${error.message}` : "";
    throw new Error(`stored manual posting attempts are invalid${detail}`);
  }
}

export function findManualPostingAttempt(
  tenantID: string,
  intentFingerprint: string,
  storage: Storage = globalThis.localStorage,
): ManualPostingAttempt | null {
  return readEnvelope(tenantID, storage).attempts.find(
    (attempt) => attempt.intentFingerprint === intentFingerprint,
  ) ?? null;
}

export function persistManualPostingAttempt(
  tenantID: string,
  attempt: ManualPostingAttempt,
  storage: Storage = globalThis.localStorage,
): void {
  if (!isAttempt(attempt)) throw new Error("invalid manual posting attempt");
  const envelope = readEnvelope(tenantID, storage);
  const attempts = envelope.attempts.filter(
    (existing) => existing.intentFingerprint !== attempt.intentFingerprint,
  );
  attempts.push(attempt);
  storage.setItem(
    manualPostingAttemptStorageKey(tenantID),
    JSON.stringify({ version: ATTEMPT_STORAGE_VERSION, attempts }),
  );
}

export function clearManualPostingAttempt(
  tenantID: string,
  intentFingerprint: string,
  storage: Storage = globalThis.localStorage,
): void {
  const envelope = readEnvelope(tenantID, storage);
  const attempts = envelope.attempts.filter(
    (attempt) => attempt.intentFingerprint !== intentFingerprint,
  );
  const key = manualPostingAttemptStorageKey(tenantID);
  if (attempts.length === 0) {
    storage.removeItem(key);
    return;
  }
  storage.setItem(key, JSON.stringify({ version: ATTEMPT_STORAGE_VERSION, attempts }));
}
