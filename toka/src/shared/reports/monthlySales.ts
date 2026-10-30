export type MonthlySalesPoint = {
  month: string;
  vendas: number;
  locacoes: number;
  valorVendas: number;
  valorLocacoes: number;
};

export type MonthlySalesDeal = {
  type?: string;
  value?: number;
  /**
   * Data de exibição vinda da esteira, no formato pt-BR ("30/09/2026").
   * É string para o usuário: NUNCA passe direto para `new Date()` — o parser do
   * JavaScript não entende dd/mm/aaaa e devolve `Invalid Date`, e
   * `getMonth()` vira `NaN` (foi o que zerou os gráficos de Vendas & Locações).
   */
  date?: string;
  /** Data em ISO 8601 (updated_at do negócio): fonte confiável para agrupar. */
  updated_at?: string;
};

const MONTHS = ["Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez"];

/**
 * parseDealDate interpreta as formas que aparecem no app:
 *
 * - ISO 8601 (o `updated_at`, preferido);
 * - `dd/mm/aaaa` (o `date` exibido na esteira);
 * - `aaaa-mm-dd`;
 * - qualquer outra forma aceita por `Date.parse`.
 *
 * Devolve `null` quando não dá para interpretar — melhor ignorar o ponto no
 * gráfico (e saber disso) do que empurrá-lo para um mês inexistente, que era o
 * comportamento antigo e escondia o negócio silenciosamente.
 */
export function parseDealDate(value: unknown): Date | null {
  if (value instanceof Date) return Number.isNaN(value.getTime()) ? null : value;
  if (typeof value !== "string") return null;
  const raw = value.trim();
  if (!raw) return null;

  const br = /^(\d{1,2})\/(\d{1,2})\/(\d{4})(?:[ T](\d{1,2}):(\d{2})(?::(\d{2}))?)?$/.exec(raw);
  if (br) {
    const [, day, month, year, hour = "0", minute = "0", second = "0"] = br;
    const parsed = new Date(
      Number(year),
      Number(month) - 1,
      Number(day),
      Number(hour),
      Number(minute),
      Number(second),
    );
    return Number.isNaN(parsed.getTime()) ? null : parsed;
  }

  const isoDateOnly = /^(\d{4})-(\d{2})-(\d{2})$/.exec(raw);
  if (isoDateOnly) {
    const [, year, month, day] = isoDateOnly;
    const parsed = new Date(Number(year), Number(month) - 1, Number(day));
    return Number.isNaN(parsed.getTime()) ? null : parsed;
  }

  const parsed = new Date(raw);
  return Number.isNaN(parsed.getTime()) ? null : parsed;
}

/** resolveDealDate usa a data confiável e só então a de exibição. */
export function resolveDealDate(deal: MonthlySalesDeal): Date | null {
  return parseDealDate(deal?.updated_at) ?? parseDealDate(deal?.date);
}

function isSale(type: unknown): boolean {
  const normalized = String(type ?? "").trim().toLowerCase();
  return normalized === "venda" || normalized === "sale" || normalized === "vendas";
}

/**
 * buildMonthlySalesData agrupa os negócios por mês para os gráficos de
 * "Vendas & Locações Mensais" (quantidade) e "Valor por Tipo de Transação"
 * (volume financeiro). Sempre devolve os 12 meses, para o eixo não mudar de
 * forma quando um mês não tem negócio.
 */
export function buildMonthlySalesData(deals: MonthlySalesDeal[] | null | undefined): MonthlySalesPoint[] {
  const grouped = new Map<string, MonthlySalesPoint>();

  for (const deal of deals ?? []) {
    if (!deal) continue;
    const date = resolveDealDate(deal);
    if (!date) continue;
    const month = MONTHS[date.getMonth()];
    if (!month) continue;

    const point =
      grouped.get(month) ?? { month, vendas: 0, locacoes: 0, valorVendas: 0, valorLocacoes: 0 };
    const value = Number(deal.value) || 0;
    if (isSale(deal.type)) {
      point.vendas += 1;
      point.valorVendas += value;
    } else {
      point.locacoes += 1;
      point.valorLocacoes += value;
    }
    grouped.set(month, point);
  }

  return MONTHS.map(
    (month) => grouped.get(month) ?? { month, vendas: 0, locacoes: 0, valorVendas: 0, valorLocacoes: 0 },
  );
}
