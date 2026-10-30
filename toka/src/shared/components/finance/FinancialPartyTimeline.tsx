import { BadgeCheck, CalendarClock, UserRound } from "lucide-react";

import { clsx } from "clsx";

interface FinancialPartyTimelineProps {
  beneficiaryLabel?: string | null;
  beneficiaryKind?: string | null;
  beneficiaryId?: string | null;
  expectedDueAt?: string | null;
  settledAt?: string | null;
  className?: string;
}

const formatDate = (value?: string | null, includeTime = false): string => {
  if (!value) return "";

  // Vencimento é uma data civil: meia-noite UTC não pode recuar um dia no
  // fuso do navegador. Liquidação é um instante e preserva hora/fuso.
  const dateOnly = !includeTime && /^\d{4}-\d{2}-\d{2}/.test(value);
  const date = new Date(dateOnly ? `${value.slice(0, 10)}T12:00:00` : value);
  if (Number.isNaN(date.getTime())) return value;

  return date.toLocaleString("pt-BR", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    ...(includeTime && !dateOnly ? { hour: "2-digit", minute: "2-digit" } : {}),
  });
};

export function FinancialPartyTimeline({
  beneficiaryLabel,
  beneficiaryKind,
  beneficiaryId,
  expectedDueAt,
  settledAt,
  className,
}: FinancialPartyTimelineProps) {
  return (
    <div className={clsx("grid grid-cols-1 gap-3 rounded-xl border border-border bg-muted/20 p-4 sm:grid-cols-3", className)}>
      <FinancialFact
        icon={UserRound}
        label="Beneficiário"
        value={beneficiaryLabel || "Não identificado"}
        detail={beneficiaryKind || beneficiaryId || undefined}
      />
      <FinancialFact
        icon={CalendarClock}
        label="Vencimento esperado"
        value={expectedDueAt ? formatDate(expectedDueAt) : "Não definido"}
        detail={expectedDueAt ? undefined : "Sem prazo registrado"}
      />
      <FinancialFact
        icon={BadgeCheck}
        label="Liquidação"
        value={settledAt ? formatDate(settledAt, true) : "Pendente"}
        detail={settledAt ? "Pagamento registrado" : "Sem baixa registrada"}
        accent={settledAt ? "ok" : undefined}
      />
    </div>
  );
}

function FinancialFact({
  icon: Icon,
  label,
  value,
  detail,
  accent,
}: {
  icon: typeof UserRound;
  label: string;
  value: string;
  detail?: string;
  accent?: "ok";
}) {
  return (
    <div className="min-w-0">
      <p className="flex items-center gap-1.5 text-xs font-bold text-muted-foreground">
        <Icon className={clsx("h-4 w-4 shrink-0", accent === "ok" ? "text-status-ok" : "text-primary-600")} />
        {label}
      </p>
      <p className={clsx("mt-1 break-words text-sm font-bold leading-snug", accent === "ok" ? "text-status-ok" : "text-foreground")} title={value}>
        {value}
      </p>
      {detail && <p className="mt-0.5 break-words text-xs text-muted-foreground" title={detail}>{detail}</p>}
    </div>
  );
}
