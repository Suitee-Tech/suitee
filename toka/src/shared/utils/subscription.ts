/**
 * Regras de data compartilhadas pela tela de assinatura e pelo indicador do
 * menu lateral: as duas calculam "dias restantes" do teste e exibem a mesma
 * data, então a conta vive aqui para não divergir.
 */

/**
 * Dias de calendário até uma data, nunca negativo. Um teste que termina no dia
 * 18 e começa hoje tem 15 dias, não 16: a conta é por data, não por fração de
 * 24h. Data inválida devolve 0 — melhor "hoje" do que "NaN dias".
 */
export function remainingDays(until: string, now: Date = new Date()): number {
  const end = new Date(until);
  if (Number.isNaN(end.getTime())) return 0;
  const target = new Date(end.getFullYear(), end.getMonth(), end.getDate()).getTime();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  return Math.max(0, Math.round((target - today) / 86_400_000));
}

/** Data em pt-BR (dd/mm/aaaa); vazio quando não há data. */
export const dateBR = (value?: string | null): string =>
  value ? new Date(value).toLocaleDateString("pt-BR") : "";

/** "3 dias" / "1 dia" — o rótulo curto usado no indicador do menu. */
export const daysLabel = (days: number): string => `${days} ${days === 1 ? "dia" : "dias"}`;
