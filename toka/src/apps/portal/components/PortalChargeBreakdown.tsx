import { clsx } from "clsx";

import type { PortalCharge } from "../services/portal.service";

/**
 * Quebra de valores da cobrança vencida (G32 do plano de inadimplência).
 *
 * Antes o portal mostrava só `total_amount`: o cliente via o valor subir depois
 * do vencimento e não sabia se era multa, juros ou erro. Aqui a cobrança em
 * atraso passa a discriminar valor original, multa, juros, desconto e total,
 * com o atraso em dias. Cobrança em dia continua com o visual de antes — a
 * quebra só aparece quando há encargo E atraso.
 */

const CENTS_EPSILON = 0.005;

/** Estados em que a cobrança ainda é cobrável — mesma lista do domínio do financeiro. */
const OUTSTANDING_STATUSES = new Set(["PENDING", "OVERDUE", "PARTIALLY_PAID"]);

function money(value: number | undefined): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

/** Multa, juros ou desconto presentes — o total já não é o valor original. */
export function hasChargeFees(charge: PortalCharge): boolean {
  return (
    money(charge.late_fee) > CENTS_EPSILON ||
    money(charge.interest) > CENTS_EPSILON ||
    money(charge.discount) > CENTS_EPSILON
  );
}

/**
 * Cobrança em aberto e já vencida (payload antigo não traz `status: OVERDUE` garantido).
 *
 * A regra é a MESMA do backend: a cobrança vence NO DIA e só atrasa no dia
 * seguinte, comparando o DIA do vencimento (gravado em UTC, é o dia que a tela
 * mostra) com o dia civil de hoje no fuso de quem está vendo. Antes o portal
 * comparava instantes com `differenceInCalendarDays` sobre a data em UTC, então
 * a cobrança do próprio dia — e, em Brasília, desde as 21h da véspera — aparecia
 * como "Vencida" para o cliente.
 */
export function isOverdueCharge(charge: PortalCharge, now: Date = new Date()): boolean {
  if (!OUTSTANDING_STATUSES.has(charge.status)) return false;
  const due = new Date(charge.due_date);
  if (Number.isNaN(due.getTime())) return false;
  return civilDayDiff(now, due) > 0;
}

/**
 * Dias civis entre o vencimento (dia em UTC, que é o dia exibido) e hoje (dia no
 * fuso local). Exportado porque o cartão de próximo vencimento da tela usa a
 * MESMA conta — era outro lugar com a regra de instantes e mostrava "vencido" na
 * véspera em Brasília.
 */
export function civilDayDiff(now: Date, due: Date): number {
  const venc = Date.UTC(due.getUTCFullYear(), due.getUTCMonth(), due.getUTCDate());
  const hoje = Date.UTC(now.getFullYear(), now.getMonth(), now.getDate());
  return Math.round((hoje - venc) / 86_400_000);
}

/**
 * Dias de atraso exibidos. Usa `days_overdue` do servidor quando ele vem (é a
 * mesma régua da cobrança) e, em payload antigo, deriva do vencimento — assim a
 * tela não quebra nem some com a informação quando o backend é mais velho.
 */
export function chargeDaysOverdue(charge: PortalCharge, now: Date = new Date()): number {
  const fromServer = money(charge.days_overdue);
  if (fromServer > 0) return Math.floor(fromServer);
  if (!isOverdueCharge(charge, now)) return 0;
  return civilDayDiff(now, new Date(charge.due_date));
}

/** Mostra a quebra só quando há encargo e atraso: cobrança em dia fica como era. */
export function shouldShowChargeBreakdown(charge: PortalCharge, now: Date = new Date()): boolean {
  return hasChargeFees(charge) && isOverdueCharge(charge, now);
}

export function formatChargeDaysOverdue(days: number): string {
  if (days === 1) return "Venceu ontem";
  return `Venceu há ${days} dias`;
}

function formatCurrencyBRL(value: number): string {
  if (!Number.isFinite(value)) return "—";
  return value.toLocaleString("pt-BR", { style: "currency", currency: "BRL" });
}

type PortalChargeBreakdownProps = {
  charge: PortalCharge;
  /** `dark` no cartão de destaque (fundo escuro), `light` nos cartões da lista. */
  tone?: "light" | "dark";
  /** O rótulo de atraso já aparece fora da quebra? Então não repita. */
  showDaysOverdue?: boolean;
  className?: string;
};

export function PortalChargeBreakdown({
  charge,
  tone = "light",
  showDaysOverdue = false,
  className,
}: PortalChargeBreakdownProps) {
  const lateFee = money(charge.late_fee);
  const interest = money(charge.interest);
  const discount = money(charge.discount);
  const originalAmount = money(charge.amount);
  const total = money(charge.total_amount) || originalAmount;
  const days = chargeDaysOverdue(charge);

  const rows: Array<{ label: string; value: string }> = [
    { label: "Valor original", value: formatCurrencyBRL(originalAmount) },
  ];
  if (lateFee > CENTS_EPSILON) rows.push({ label: "Multa", value: formatCurrencyBRL(lateFee) });
  if (interest > CENTS_EPSILON) rows.push({ label: "Juros", value: formatCurrencyBRL(interest) });
  if (discount > CENTS_EPSILON) rows.push({ label: "Desconto", value: `- ${formatCurrencyBRL(discount)}` });

  const dark = tone === "dark";

  return (
    <div
      aria-label="Detalhamento da cobrança"
      className={clsx(
        "rounded-2xl border px-4 py-3",
        dark ? "border-white/15 bg-white/10" : "border-border bg-muted/50",
        className,
      )}
    >
      {showDaysOverdue && days > 0 && (
        <p className={clsx("text-caption font-semibold", dark ? "text-white" : "text-status-warning")}>
          {formatChargeDaysOverdue(days)}
        </p>
      )}

      <dl className={clsx("space-y-1", showDaysOverdue && days > 0 && "mt-2")}>
        {rows.map((row) => (
          <div key={row.label} className="flex items-baseline justify-between gap-4">
            <dt className={clsx("text-label", dark ? "text-white/70" : "text-muted-foreground")}>{row.label}</dt>
            <dd className={clsx("text-label tabular-nums", dark ? "text-white/80" : "text-foreground")}>{row.value}</dd>
          </div>
        ))}
        <div className={clsx("mt-2 flex items-baseline justify-between gap-4 border-t pt-2", dark ? "border-white/15" : "border-border")}>
          <dt className={clsx("text-label font-semibold", dark ? "text-white" : "text-foreground")}>Total</dt>
          <dd className={clsx("text-title font-bold tabular-nums", dark ? "text-white" : "text-foreground")}>
            {formatCurrencyBRL(total)}
          </dd>
        </div>
      </dl>
    </div>
  );
}
