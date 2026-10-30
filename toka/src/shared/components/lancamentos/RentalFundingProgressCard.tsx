import { CalendarDays, CheckCircle2, Clock3, WalletCards } from "lucide-react";

import { Progress } from "../../ui/progress";
import { formatMoneyBR, formatCompetencia } from "../../utils/format";

export interface RentalFundingEntryView {
  payment_id: string;
  charge_id: string;
  reference_month: string;
  amount_cents: number;
  funded_at: string;
}

interface RentalFundingProgressCardProps {
  requiredCents?: number;
  fundedCents?: number;
  entries?: RentalFundingEntryView[];
  compact?: boolean;
  /**
   * `false` quando nenhum aluguel foi recebido para esta competência. É o que
   * distingue "ainda não há o que liberar" de "o recebimento entrou e não
   * liberou nada" — sem isso, um R$ 0,00 de cobertura parece falta de valor.
   */
  hasReceivedRent?: boolean;
  title?: string;
  description?: string;
  emptyDescription?: string;
  emptyBadge?: string;
  emptyHint?: string;
  emptyOutstandingHint?: string;
  completedDescription?: string;
  ariaLabel?: string;
  valueLabel?: string;
  coveredLabel?: string;
  outstandingLabel?: string;
}

const competenceLabel = (referenceMonth: string) => {
  if (!referenceMonth) return "Competência não informada";
  return formatCompetencia(referenceMonth);
};

export function RentalFundingProgressCard({
  requiredCents = 0,
  fundedCents = 0,
  entries = [],
  compact = false,
  hasReceivedRent,
  title = "Cobertura pelos aluguéis recebidos",
  description = "A taxa de administração é apropriada primeiro. O saldo da comissão será coberto automaticamente pelos próximos aluguéis recebidos.",
  emptyDescription = "Nenhum aluguel recebido até agora. A comissão é liberada conforme os aluguéis entram, por isso a cobertura está em R$ 0,00 — não é ausência de valor contratado.",
  emptyBadge = "Nada a pagar ainda",
  emptyHint = "Nenhum aluguel recebido",
  emptyOutstandingHint = "Liberado a partir do 1º aluguel",
  completedDescription = "Cobertura concluída. A obrigação pode seguir para revisão e pagamento.",
  ariaLabel = "Cobertura da comissão pelos aluguéis",
  valueLabel = "Comissão contratual",
  coveredLabel = "Coberta por recebimentos",
  outstandingLabel,
}: RentalFundingProgressCardProps) {
  if (requiredCents <= 0) return null;

  const normalizedFunded = Math.min(requiredCents, Math.max(0, fundedCents));
  const outstandingCents = Math.max(0, requiredCents - normalizedFunded);
  const completed = outstandingCents === 0;
  const percentage = Math.min(100, Math.round((normalizedFunded / requiredCents) * 100));
  // Primeira parcela: nada recebido ainda, então nada pode ser liberado.
  const awaitingFirstRent = !completed && normalizedFunded === 0 && hasReceivedRent !== true;

  return (
    <section
      className={
        completed
          ? "rounded-xl border border-status-ok/30 bg-status-ok-soft p-4"
          : "rounded-xl border border-status-warning/30 bg-status-warning-soft p-4"
      }
      aria-label={ariaLabel}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <span className="mt-0.5 rounded-lg bg-background/80 p-2">
            {completed ? (
              <CheckCircle2 className="size-4 text-status-ok" />
            ) : (
              <WalletCards className="size-4 text-status-warning" />
            )}
          </span>
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-sm font-bold text-foreground">{title}</h3>
              {awaitingFirstRent && (
                <span className="rounded-full border border-border bg-background/80 px-2 py-0.5 text-[11px] font-bold text-muted-foreground">
                  {emptyBadge}
                </span>
              )}
            </div>
            <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
              {completed ? completedDescription : awaitingFirstRent ? emptyDescription : description}
            </p>
          </div>
        </div>
        <span className="shrink-0 text-xs font-bold text-foreground">{percentage}%</span>
      </div>

      <Progress value={percentage} className="mt-4 h-2 bg-background/70" />

      <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-3">
        <FundingMetric label={valueLabel} cents={requiredCents} hint="Valor devido pelo contrato" />
        <FundingMetric
          label={coveredLabel}
          cents={normalizedFunded}
          tone="ok"
          hint={awaitingFirstRent ? emptyHint : undefined}
        />
        <FundingMetric
          label={completed ? "Saldo" : outstandingLabel ?? "Saldo para próximos meses"}
          cents={outstandingCents}
          tone={completed ? "ok" : "warning"}
          hint={awaitingFirstRent ? emptyOutstandingHint : undefined}
        />
      </div>

      {!compact && entries.length > 0 && (
        <div className="mt-4 border-t border-border/60 pt-3">
          <p className="mb-2 text-[11px] font-bold uppercase tracking-wider text-muted-foreground">Recebimentos que deram cobertura</p>
          <ul className="space-y-2">
            {entries.slice().reverse().map((entry) => (
              <li key={`${entry.payment_id}:${entry.amount_cents}`} className="flex items-center justify-between gap-3 rounded-lg bg-background/70 px-3 py-2 text-xs">
                <span className="flex min-w-0 items-center gap-2 text-muted-foreground">
                  <CalendarDays className="size-3.5 shrink-0" />
                  <span className="truncate capitalize">{competenceLabel(entry.reference_month)}</span>
                </span>
                <span className="shrink-0 font-mono font-bold text-foreground">{formatMoneyBR(entry.amount_cents / 100)}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}

function FundingMetric({
  label,
  cents,
  tone,
  hint,
}: {
  label: string;
  cents: number;
  tone?: "ok" | "warning";
  hint?: string;
}) {
  const Icon = tone === "warning" ? Clock3 : tone === "ok" ? CheckCircle2 : WalletCards;
  return (
    <div className="rounded-lg border border-border/60 bg-background/75 p-3">
      <p className="flex items-center gap-1.5 text-[11px] font-semibold text-muted-foreground">
        <Icon className="size-3.5" /> {label}
      </p>
      <p className="mt-1 font-mono text-sm font-bold text-foreground">{formatMoneyBR(cents / 100)}</p>
      {hint && <p className="mt-0.5 text-caption font-semibold text-muted-foreground">{hint}</p>}
    </div>
  );
}
