export const formatCurrencyBRL = (value: number | null | undefined): string => {
  const n = typeof value === 'number' && Number.isFinite(value) ? value : 0;
  return new Intl.NumberFormat('pt-BR', { style: 'currency', currency: 'BRL' }).format(n);
};

// Meses abreviados em pt-BR para formatar competência ("2026-09" -> "set/2026").
const COMPETENCIA_MONTHS = ["Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez"];

/** Converte uma competência ISO "YYYY-MM" para o padrão brasileiro "mmm/aaaa". */
export const formatCompetencia = (referenceMonth: string | null | undefined): string => {
  if (!referenceMonth) return '—';
  const match = /^(\d{4})-(\d{1,2})$/.exec(referenceMonth.trim());
  if (!match) return referenceMonth;
  const [, year, m] = match;
  const label = COMPETENCIA_MONTHS[Number(m) - 1] ?? m;
  return `${label}/${year}`;
};

export const formatMoneyBR = (
  value: number | null | undefined,
  options?: { withSymbol?: boolean; fractionDigits?: 0 | 2 },
): string => {
  if (value === null || value === undefined) return "";
  if (typeof value !== "number" || !Number.isFinite(value)) return "";
  const withSymbol = options?.withSymbol !== false;
  const fractionDigits = options?.fractionDigits ?? 2;
  const nf = new Intl.NumberFormat("pt-BR", {
    style: withSymbol ? "currency" : "decimal",
    currency: withSymbol ? "BRL" : undefined,
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  });
  return nf.format(value);
};

export const formatCurrencyBRL0 = (value: number | null | undefined): string =>
  formatMoneyBR(value, { withSymbol: true, fractionDigits: 0 });

export const formatMoneyBR0 = (value: number | null | undefined): string =>
  formatMoneyBR(value, { withSymbol: false, fractionDigits: 0 });

export const parseMoneyBR = (value: string): number | null => {
  const s = (value || "").trim();
  if (!s) return null;
  const hasComma = s.includes(",");
  const cleaned = s.replace(/[^\d,.-]/g, "");
  const normalized = hasComma
    ? cleaned.replace(/\./g, "").replace(",", ".")
    : (/^-?\d+\.\d{1,2}$/.test(cleaned) ? cleaned : cleaned.replace(/\./g, ""));
  const n = Number.parseFloat(normalized);
  return Number.isFinite(n) ? n : null;
};

export const parseMoneyBR0 = (value: string): number | null => {
  const n = parseMoneyBR(value);
  if (n === null) return null;
  return Math.trunc(n);
};

export const coerceMoneyBR = (value: unknown): number => {
  if (value === null || value === undefined) return 0;
  if (typeof value === "number") return Number.isFinite(value) ? value : 0;
  if (typeof value === "string") return parseMoneyBR(value) ?? 0;
  const n = Number(value as any);
  return Number.isFinite(n) ? n : 0;
};

const toUTCDate = (date: string): Date | null => {
  if (!date) return null;
  const hasTime = date.includes('T');
  const d = new Date(hasTime ? date : `${date}T00:00:00Z`);
  if (Number.isNaN(d.getTime())) return null;
  return d;
};

export const formatISODateBR = (date: string | null | undefined): string => {
  const d = date ? toUTCDate(date) : null;
  if (!d) return '';
  return new Intl.DateTimeFormat('pt-BR', { timeZone: 'UTC' }).format(d);
};

/** Data + hora em pt-BR com timeZone UTC (evita deslocamento de dia no Brasil). */
export const formatDateTimeBR = (date: string | null | undefined): string => {
  const d = date ? toUTCDate(date) : null;
  if (!d) return '';
  return new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short', timeStyle: 'short', timeZone: 'UTC' }).format(d);
};

export const getDayFromISODate = (date: string | null | undefined): string => {
  const d = date ? toUTCDate(date) : null;
  if (!d) return '';
  return String(d.getUTCDate());
};

export const getYearFromISODate = (date: string | null | undefined): string => {
  const d = date ? toUTCDate(date) : null;
  if (!d) return '';
  return String(d.getUTCFullYear());
};

export const getMonthNameFromISODate = (date: string | null | undefined): string => {
  const d = date ? toUTCDate(date) : null;
  if (!d) return '';
  return new Intl.DateTimeFormat('pt-BR', { month: 'long', timeZone: 'UTC' }).format(d);
};

export const capitalizeFirst = (text: string): string => {
  const s = (text || '').trim();
  if (!s) return '';
  return s.charAt(0).toUpperCase() + s.slice(1);
};

const UNITS = [
  'zero', 'um', 'dois', 'três', 'quatro', 'cinco', 'seis', 'sete', 'oito', 'nove',
  'dez', 'onze', 'doze', 'treze', 'quatorze', 'quinze', 'dezesseis', 'dezessete', 'dezoito', 'dezenove',
];

const TENS = [
  '', '', 'vinte', 'trinta', 'quarenta', 'cinquenta', 'sessenta', 'setenta', 'oitenta', 'noventa',
];

const HUNDREDS = [
  '', 'cento', 'duzentos', 'trezentos', 'quatrocentos', 'quinhentos', 'seiscentos', 'setecentos', 'oitocentos', 'novecentos',
];

const tripletToWords = (n: number): string => {
  const x = Math.trunc(n);
  if (x === 0) return '';
  if (x === 100) return 'cem';
  if (x < 20) return UNITS[x];

  const c = Math.trunc(x / 100);
  const r = x % 100;
  const d = Math.trunc(r / 10);
  const u = r % 10;

  const parts: string[] = [];
  if (c > 0) parts.push(HUNDREDS[c]);
  if (r > 0) {
    if (r < 20) {
      parts.push(UNITS[r]);
    } else {
      if (d > 0) parts.push(TENS[d]);
      if (u > 0) parts.push(UNITS[u]);
    }
  }

  return parts.join(' e ');
};

const joinScaleParts = (parts: string[]): string => {
  const nonEmpty = parts.filter(Boolean);
  if (nonEmpty.length === 0) return '';
  if (nonEmpty.length === 1) return nonEmpty[0];

  const last = nonEmpty[nonEmpty.length - 1];
  const prev = nonEmpty.slice(0, -1).join(', ');
  return `${prev} e ${last}`;
};

export const numberToWordsPTBR = (value: number): string => {
  if (!Number.isFinite(value)) return '';
  const n = Math.trunc(Math.abs(value));
  if (n === 0) return 'zero';

  const billions = Math.trunc(n / 1_000_000_000);
  const millions = Math.trunc((n % 1_000_000_000) / 1_000_000);
  const thousands = Math.trunc((n % 1_000_000) / 1_000);
  const rest = n % 1_000;

  const parts: string[] = [];

  if (billions > 0) {
    const w = tripletToWords(billions);
    parts.push(billions === 1 ? 'um bilhão' : `${w} bilhões`);
  }

  if (millions > 0) {
    const w = tripletToWords(millions);
    parts.push(millions === 1 ? 'um milhão' : `${w} milhões`);
  }

  if (thousands > 0) {
    const w = tripletToWords(thousands);
    parts.push(thousands === 1 ? 'mil' : `${w} mil`);
  }

  if (rest > 0) {
    parts.push(tripletToWords(rest));
  }

  return joinScaleParts(parts);
};

export const currencyToWordsBRL = (value: number): string => {
  if (!Number.isFinite(value)) return '';
  const abs = Math.abs(value);
  const reais = Math.trunc(abs);
  const centavos = Math.round((abs - reais) * 100);

  const realWord = reais === 1 ? 'real' : 'reais';
  const centWord = centavos === 1 ? 'centavo' : 'centavos';

  const reaisText = reais > 0 ? `${numberToWordsPTBR(reais)} ${realWord}` : '';
  const centsText = centavos > 0 ? `${numberToWordsPTBR(centavos)} ${centWord}` : '';

  const joined = reaisText && centsText ? `${reaisText} e ${centsText}` : (reaisText || centsText || `zero ${realWord}`);
  const signed = value < 0 ? `menos ${joined}` : joined;
  return capitalizeFirst(signed);
};
