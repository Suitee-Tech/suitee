import { useMemo } from "react";
import { AlertOctagon, ArrowDownCircle, ArrowUpCircle, Clock } from "lucide-react";
import { clsx } from "clsx";

import { formatCurrencyBRL0 } from "../../utils/format";
import { computeDivergence } from "../../lib/lancamentos/divergence";
import type { Lancamento } from "../../lib/lancamentos/types";

interface LancamentosKpisProps {
  lancamentos: Lancamento[];
  onSelect?: (view: LancamentosKpiView) => void;
}

export type LancamentosKpiView = "receber" | "pagar" | "vencidos" | "divergencias";

interface KpiBucket {
  count: number;
  amount: number;
}

interface DivergenceBucket extends KpiBucket {
  differenceAmount: number;
}

function emptyBucket(): KpiBucket {
  return { count: 0, amount: 0 };
}

function emptyDivergenceBucket(): DivergenceBucket {
  return { count: 0, amount: 0, differenceAmount: 0 };
}

export function classifyLancamentos(lancamentos: Lancamento[]) {
  const aReceber = emptyBucket();
  const aPagar = emptyBucket();
  const vencidos = emptyBucket();
  const divergencias = emptyDivergenceBucket();

  for (const l of lancamentos) {
    const openish = l.status === "pendente" || l.status === "aprovado" || l.status === "parcial" || l.status === "vencido";

    if (openish) {
      if (l.kind === "charge") {
        aReceber.count += 1;
        aReceber.amount += l.amount || 0;
      } else if (l.kind === "commission" || l.kind === "transfer" || l.kind === "royalty") {
        aPagar.count += 1;
        aPagar.amount += l.amount || 0;
      } else if (l.kind === "transaction" && l.kind_data.direction === "entrada") {
        aReceber.count += 1;
        aReceber.amount += l.amount || 0;
      } else if (l.kind === "transaction" && l.kind_data.direction === "saida") {
        aPagar.count += 1;
        aPagar.amount += l.amount || 0;
      }
    }

    if (l.status === "vencido") {
      vencidos.count += 1;
      vencidos.amount += l.amount || 0;
    }

    const divergence = computeDivergence(l);
    if (divergence) {
      divergencias.count += 1;
      divergencias.amount += l.amount || 0;
      if (divergence.reason === "amount_mismatch") {
        divergencias.differenceAmount += Math.abs(divergence.delta ?? 0);
      }
    }
  }

  return { aReceber, aPagar, vencidos, divergencias };
}

export function LancamentosKpis({ lancamentos, onSelect }: LancamentosKpisProps) {
  const { aReceber, aPagar, vencidos, divergencias } = useMemo(() => classifyLancamentos(lancamentos), [lancamentos]);

  return (
    <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
      <KpiCard icon={ArrowDownCircle} label="A receber em aberto" bucket={aReceber} tone="ok" onClick={() => onSelect?.("receber")} />
      <KpiCard icon={ArrowUpCircle} label="A pagar em aberto" bucket={aPagar} tone="info" onClick={() => onSelect?.("pagar")} />
      <KpiCard icon={Clock} label="Vencidos" bucket={vencidos} tone="critical" onClick={() => onSelect?.("vencidos")} />
      <KpiCard
        icon={AlertOctagon}
        label="Diferenças calculadas"
        bucket={{ count: divergencias.count, amount: divergencias.differenceAmount }}
        tone="warning"
        hint="Soma somente das diferenças de valor que puderam ser calculadas. Outras sinalizações aparecem na contagem."
        amountCaption="Não soma o valor nominal dos lançamentos"
        detail={`${formatCurrencyBRL0(divergencias.amount)} em lançamentos sinalizados`}
        onClick={() => onSelect?.("divergencias")}
      />
    </div>
  );
}

interface KpiCardProps {
  icon: typeof ArrowDownCircle;
  label: string;
  bucket: KpiBucket;
  tone: "ok" | "info" | "warning" | "critical";
  hint?: string;
  amountCaption?: string;
  detail?: string;
  onClick?: () => void;
}

function KpiCard({ icon: Icon, label, bucket, tone, hint, amountCaption, detail, onClick }: KpiCardProps) {
  const toneClasses = {
    ok: "text-status-ok bg-status-ok-soft border-status-ok/20",
    info: "text-status-info bg-status-info-soft border-status-info/20",
    warning: "text-status-warning bg-status-warning-soft border-status-warning/20",
    critical: "text-status-critical bg-status-critical-soft border-status-critical/20",
  } as const;

  const iconColor = {
    ok: "text-status-ok",
    info: "text-status-info",
    warning: "text-status-warning",
    critical: "text-status-critical",
  } as const;

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={!onClick}
      className={clsx(
        "rounded-xl border p-3 flex items-start gap-3 text-left transition-all",
        toneClasses[tone],
        onClick && "hover:-translate-y-0.5 hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary",
      )}
      title={hint}
      aria-label={onClick ? `Filtrar por ${label}` : undefined}
    >
      <div className={clsx("shrink-0 mt-0.5", iconColor[tone])}>
        <Icon className="w-5 h-5" aria-hidden="true" />
      </div>
      <div className="min-w-0">
        <div className="text-xs font-bold uppercase tracking-wide opacity-80">{label}</div>
        <div className="text-lg font-bold tabular-nums mt-0.5 text-foreground">{formatCurrencyBRL0(bucket.amount)}</div>
        {amountCaption && <div className="text-[11px] text-muted-foreground">{amountCaption}</div>}
        <div className="text-xs text-muted-foreground mt-0.5">
          {bucket.count} {bucket.count === 1 ? "lançamento" : "lançamentos"}
        </div>
        {detail && <div className="mt-1 text-[11px] font-semibold text-foreground/75">{detail}</div>}
      </div>
    </button>
  );
}
