// Cálculo de pró-rata e residual do aluguel no plano pré-contrato.
//
// Espelha a regra canônica do backend — `toca-financial/internal/domain/
// rental_charge_config/rental_charge_config.go` (política `actual_actual`,
// padrão de toda config nova) — para o operador ver, ANTES de gerar a minuta,
// exatamente o que será cobrado no primeiro mês (pró-rata) e no último mês
// (residual do término). A geração real das cobranças continua sendo do
// backend (`buildCharges`), que usa as mesmas funções; aqui não há arredondamento
// diferente: centavos com divisão inteira truncada, como no Go.
//
// Regra (actual/actual):
//   - primeiro mês: dias do início até o fim daquele mês ÷ dias do mês;
//   - último mês: dias do início do mês até o término ÷ dias do mês — mês cheio
//     quando o término cai no último dia do mês.
//
// Todas as datas entram e saem como `YYYY-MM-DD` (data civil, sem fuso): o
// contrato é tratado no dia civil da imobiliária, e misturar ISO completo com
// `Date` local desloca o cálculo em um dia.

/** Prazo padrão do contrato de locação, em meses (espelha `DefaultLeaseTermMonths` do toca-deal). */
export const DEFAULT_LEASE_TERM_MONTHS = 30;

/** Limites aceitos para o prazo digitado no plano (1 mês a 10 anos). */
export const MIN_LEASE_TERM_MONTHS = 1;
export const MAX_LEASE_TERM_MONTHS = 120;

const ISO_DATE = /^(\d{4})-(\d{2})-(\d{2})$/;

type CivilDate = { year: number; month: number; day: number };

function parseCivilDate(value: string): CivilDate | null {
  const match = ISO_DATE.exec((value || "").trim());
  if (!match) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  if (month < 1 || month > 12 || day < 1 || day > 31) return null;
  // Rejeita datas que não existem no calendário (ex.: 2026-02-30).
  if (day > daysInMonth(year, month)) return null;
  return { year, month, day };
}

function toCivil(value: string): CivilDate | null {
  return parseCivilDate(value);
}

function formatCivil({ year, month, day }: CivilDate): string {
  const mm = String(month).padStart(2, "0");
  const dd = String(day).padStart(2, "0");
  return `${year}-${mm}-${dd}`;
}

/** Número de dias do mês (1-12). */
export function daysInMonth(year: number, month: number): number {
  return new Date(Date.UTC(year, month, 0)).getUTCDate();
}

function daysInMonthOf(date: CivilDate): number {
  return daysInMonth(date.year, date.month);
}

/**
 * Soma meses a uma data civil, ancorando no último dia do mês alvo.
 *
 * Sem a ancoragem, somar meses a partir do dia 31 "vaza" para o mês seguinte
 * (31/01 + 1 mês = 03/03) — e como o término determina o residual do último
 * aluguel, o vazamento cobraria dias que não existem no contrato. Com a
 * ancoragem: 31/01 + 1 mês = 28/02 (ou 29/02 em ano bissexto).
 */
export function addMonthsClamped(startISO: string, months: number): string {
  const start = toCivil(startISO);
  if (!start || !Number.isFinite(months)) return "";
  const totalMonths = start.year * 12 + (start.month - 1) + Math.trunc(months);
  const year = Math.floor(totalMonths / 12);
  const month = (totalMonths % 12) + 1;
  const day = Math.min(start.day, daysInMonth(year, month));
  return formatCivil({ year, month, day });
}

/**
 * Meses inteiros entre duas datas civis (piso), considerando que o término
 * conta como dia ocupado. Ex.: 02/10/2026 → 02/04/2029 = 30; 02/10/2026 →
 * 31/03/2029 = 29 (faltam 2 dias para fechar o 30º mês).
 */
export function wholeMonthsBetween(startISO: string, endISO: string): number {
  const start = toCivil(startISO);
  const end = toCivil(endISO);
  if (!start || !end) return 0;
  let months = (end.year - start.year) * 12 + (end.month - start.month);
  // O dia do término precisa alcançar o dia do início para contar o mês cheio;
  // quando o início é 31 e o mês alvo é mais curto, o "dia do início" efetivo é
  // o último dia do mês alvo (mesma ancoragem da soma).
  const anchor = Math.min(start.day, daysInMonth(end.year, end.month));
  if (end.day < anchor) months -= 1;
  return Math.max(0, months);
}

/** Dias de um mês que a data inicial ocupa (inclusive), até o fim daquele mês. */
function daysOccupiedInMonth(date: CivilDate): number {
  return daysInMonthOf(date) - date.day + 1;
}

function proRataCents(monthlyCents: number, days: number, denominator: number): number {
  if (!Number.isFinite(monthlyCents) || monthlyCents <= 0) return 0;
  if (denominator <= 0 || days <= 0) return 0;
  return Math.floor((monthlyCents * days) / denominator);
}

/**
 * Valor do primeiro mês (pró-rata): dias ocupados do mês inicial ÷ dias do mês.
 * Devolve também a base em dias, para a UI explicar o número.
 */
export function firstMonthProRata(startISO: string, monthlyCents: number): { cents: number; days: number; daysInMonth: number } {
  const start = toCivil(startISO);
  if (!start) return { cents: 0, days: 0, daysInMonth: 0 };
  const denominator = daysInMonthOf(start);
  const days = daysOccupiedInMonth(start);
  return { cents: proRataCents(monthlyCents, days, denominator), days, daysInMonth: denominator };
}

/**
 * Valor do último mês (residual): dias do mês final ocupados até o término.
 * Mês cheio quando o término cai no último dia do mês; mês vazio quando o
 * término é anterior ao início do mês final (contrato de dias, ex.: término no
 * próprio mês do início).
 */
export function lastMonthResidual(endISO: string, monthlyCents: number): { cents: number; days: number; daysInMonth: number } {
  const end = toCivil(endISO);
  if (!end) return { cents: 0, days: 0, daysInMonth: 0 };
  const denominator = daysInMonthOf(end);
  if (end.day >= denominator) {
    return { cents: monthlyCents > 0 ? monthlyCents : 0, days: denominator, daysInMonth: denominator };
  }
  return { cents: proRataCents(monthlyCents, end.day, denominator), days: end.day, daysInMonth: denominator };
}

export type FirstChargeType = "same_month_pro_rata" | "next_month_accumulated";

/**
 * Valor da PRIMEIRA cobrança conforme o tratamento escolhido:
 *   - `same_month_pro_rata`: só o pró-rata do mês inicial;
 *   - `next_month_accumulated`: pró-rata do mês inicial + mês seguinte (e, se o
 *     contrato terminar dentro desse mês seguinte, o residual dele).
 *
 * Espelha `CalculateFirstChargeAmount` do backend.
 */
export function firstChargeCents(
  startISO: string,
  endISO: string,
  monthlyCents: number,
  chargeType: FirstChargeType,
): number {
  const prorata = firstMonthProRata(startISO, monthlyCents).cents;
  if (chargeType !== "next_month_accumulated") return prorata;

  const start = toCivil(startISO);
  if (!start) return prorata;
  const nextMonthStart = addMonthsClamped(formatCivil({ ...start, day: 1 }), 1);
  const nextMonthEnd = endOfMonthISO(nextMonthStart);
  const end = toCivil(endISO);
  if (end && nextMonthEnd) {
    const endOfNext = toCivil(nextMonthEnd);
    if (endOfNext && compareCivil(end, endOfNext) <= 0) {
      return prorata + lastMonthResidual(endISO, monthlyCents).cents;
    }
  }
  return prorata + monthlyCents;
}

/** Último dia do mês da data informada, como `YYYY-MM-DD`. */
export function endOfMonthISO(dateISO: string): string {
  const date = toCivil(dateISO);
  if (!date) return "";
  return formatCivil({ year: date.year, month: date.month, day: daysInMonthOf(date) });
}

function compareCivil(a: CivilDate, b: CivilDate): number {
  if (a.year !== b.year) return a.year - b.year;
  if (a.month !== b.month) return a.month - b.month;
  return a.day - b.day;
}

/** Dias corridos entre duas datas civis (término inclusive), para exibição. */
export function daysBetweenInclusive(startISO: string, endISO: string): number {
  const start = toCivil(startISO);
  const end = toCivil(endISO);
  if (!start || !end) return 0;
  const startMs = Date.UTC(start.year, start.month - 1, start.day);
  const endMs = Date.UTC(end.year, end.month - 1, end.day);
  if (endMs < startMs) return 0;
  return Math.floor((endMs - startMs) / 86_400_000) + 1;
}

/**
 * Meses de cobrança do calendário: do mês inicial ao mês do término
 * (inclusive) — é o número de aluguéis que o contrato gera.
 */
export function billingMonths(startISO: string, endISO: string): number {
  const start = toCivil(startISO);
  const end = toCivil(endISO);
  if (!start || !end) return 0;
  const months = (end.year - start.year) * 12 + (end.month - start.month) + 1;
  return Math.max(0, months);
}
