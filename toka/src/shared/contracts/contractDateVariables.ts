/** Calendar date operations supported by contract templates, without evaluating expressions. */
export const CONTRACT_DATE_BASE_KEYS = [
  'current_date',
  'contract_start_date',
  'contract_end_date',
  'rent_first_charge_date',
  'rent_standard_due_date',
  'contract_next_adjustment',
  'due_diligence_updated_at',
] as const;

export type ContractDateBaseKey = (typeof CONTRACT_DATE_BASE_KEYS)[number];
export type ContractDateUnit = 'days' | 'business_days' | 'months' | 'years';
export type ContractDateFormat = 'br' | 'iso' | 'written';

/** A maximum of roughly one century also bounds the work needed for business days. */
export const CONTRACT_DATE_OFFSET_LIMITS = {
  days: 36_600,
  business_days: 26_000,
  months: 1_200,
  years: 100,
} as const satisfies Record<ContractDateUnit, number>;

export interface ContractDateVariable {
  baseKey: ContractDateBaseKey;
  direction: 'plus' | 'minus' | null;
  amount: number;
  unit: ContractDateUnit | null;
  format: ContractDateFormat;
}

const DATE_VARIABLE_PATTERN = new RegExp(
  `^(${CONTRACT_DATE_BASE_KEYS.join('|')})(?:_(plus|minus)_(0|[1-9]\\d*)_(business_days|days|months|years))?(?:_(iso|written))?$`,
);

/** Accepts English identifier tokens such as current_date_plus_30_days_iso. */
export function parseContractDateVariable(key: string): ContractDateVariable | undefined {
  const match = DATE_VARIABLE_PATTERN.exec(key);
  if (!match) return undefined;
  const amount = match[3] === undefined ? 0 : Number(match[3]);
  const unit = (match[4] || null) as ContractDateUnit | null;
  if (!Number.isSafeInteger(amount) || (unit && amount > CONTRACT_DATE_OFFSET_LIMITS[unit])) {
    return undefined;
  }
  return {
    baseKey: match[1] as ContractDateBaseKey,
    direction: (match[2] || null) as ContractDateVariable['direction'],
    amount,
    unit,
    format: (match[5] || 'br') as ContractDateFormat,
  };
}

export function isContractDateVariable(key: string): boolean {
  return parseContractDateVariable(key) !== undefined;
}

function makeCalendarDate(year: number, month: number, day: number): Date | undefined {
  if (year < 1 || year > 9999 || month < 1 || month > 12 || day < 1 || day > 31) return undefined;
  // UTC is only an arithmetic container: no conversion of the original calendar day occurs.
  const date = new Date(0);
  date.setUTCFullYear(year, month - 1, day);
  date.setUTCHours(0, 0, 0, 0);
  return date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day
    ? date
    : undefined;
}

function parseCalendarDate(value: unknown): Date | undefined {
  if (value instanceof Date) {
    if (Number.isNaN(value.getTime())) return undefined;
    return makeCalendarDate(value.getUTCFullYear(), value.getUTCMonth() + 1, value.getUTCDate());
  }
  if (typeof value !== 'string') return undefined;
  const normalized = value.trim();
  const brazilian = /^(\d{2})\/(\d{2})\/(\d{4})$/.exec(normalized);
  if (brazilian) {
    return makeCalendarDate(Number(brazilian[3]), Number(brazilian[2]), Number(brazilian[1]));
  }
  const iso = /^(\d{4})-(\d{2})-(\d{2})(?:T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?)?$/.exec(normalized);
  if (!iso || (normalized.includes('T') && Number.isNaN(Date.parse(normalized)))) return undefined;
  return makeCalendarDate(Number(iso[1]), Number(iso[2]), Number(iso[3]));
}

function addCalendarMonths(date: Date, months: number): void {
  const originalDay = date.getUTCDate();
  date.setUTCDate(1);
  date.setUTCMonth(date.getUTCMonth() + months);
  const lastDay = new Date(date.getTime());
  lastDay.setUTCMonth(lastDay.getUTCMonth() + 1, 0);
  date.setUTCDate(Math.min(originalDay, lastDay.getUTCDate()));
}

function formatCalendarDate(date: Date, format: ContractDateFormat): string {
  const day = String(date.getUTCDate()).padStart(2, '0');
  const month = String(date.getUTCMonth() + 1).padStart(2, '0');
  const year = String(date.getUTCFullYear()).padStart(4, '0');
  if (format === 'iso') return `${year}-${month}-${day}`;
  if (format === 'written') {
    return new Intl.DateTimeFormat('pt-BR', {
      day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC',
    }).format(date);
  }
  return `${day}/${month}/${year}`;
}

/** Formats an already resolved date without applying an offset a second time. */
export function formatContractDateValue(value: unknown, format: ContractDateFormat): string | undefined {
  const date = parseCalendarDate(value);
  return date ? formatCalendarDate(date, format) : undefined;
}

/**
 * Resolves from an explicit source base, preferring its human date over an ISO alias.
 * Missing/invalid dates stay unresolved for review. Business days exclude weekends only.
 * Literal template overrides are handled first by resolveTemplateValue in docx.ts.
 */
export function resolveContractDateVariable(source: unknown, key: string): string | undefined {
  const variable = parseContractDateVariable(key);
  if (!variable || source === null || typeof source !== 'object') return undefined;
  const data = source as Record<string, unknown>;
  const { baseKey, direction, amount, unit, format } = variable;
  const isoKey = `${baseKey}_iso`;
  const baseValue = Object.prototype.hasOwnProperty.call(data, baseKey)
    ? data[baseKey]
    : Object.prototype.hasOwnProperty.call(data, isoKey) ? data[isoKey] : undefined;
  const date = parseCalendarDate(baseValue);
  if (!date) return undefined;
  const step = direction === 'minus' ? -1 : 1;
  if (unit === 'days') date.setUTCDate(date.getUTCDate() + step * amount);
  if (unit === 'months') addCalendarMonths(date, step * amount);
  if (unit === 'years') addCalendarMonths(date, step * amount * 12);
  if (unit === 'business_days') {
    let remaining = amount;
    while (remaining > 0) {
      date.setUTCDate(date.getUTCDate() + step);
      if (date.getUTCDay() !== 0 && date.getUTCDay() !== 6) remaining--;
    }
  }
  if (date.getUTCFullYear() < 1 || date.getUTCFullYear() > 9999) return undefined;
  return formatCalendarDate(date, format);
}
