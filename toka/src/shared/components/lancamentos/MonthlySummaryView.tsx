import { useMemo } from "react";
import { ChevronDown, ChevronRight, TrendingDown, TrendingUp, Wallet } from "lucide-react";
import { clsx } from "clsx";

import type { MonthlyBucket } from "../../lib/lancamentos/presentation";
import { formatCurrencyBRL0 } from "../../utils/format";

interface MonthlySummaryViewProps {
  buckets: MonthlyBucket[];
  selectedMonth?: string | null;
  onSelectMonth?: (month: string | null) => void;
}

const KIND_LABEL: Record<string, string> = {
  charge: "Cobranças",
  commission: "Comissões",
  transfer: "Repasses",
  royalty: "Parcerias",
  transaction: "Caixa",
};

function row(label: string, value: number, tone: "ok" | "warning" | "critical" | "muted" = "muted") {
  const color =
    tone === "ok"
      ? "text-status-ok"
      : tone === "warning"
        ? "text-status-warning"
        : tone === "critical"
          ? "text-status-critical"
          : "text-muted-foreground";
  return { label, value, color };
}

export function MonthlySummaryView({ buckets, selectedMonth, onSelectMonth }: MonthlySummaryViewProps) {
  const total = useMemo(() => {
    const entrada = buckets.reduce((s, b) => s + b.entrada, 0);
    const saida = buckets.reduce((s, b) => s + b.saida, 0);
    const receber = buckets.reduce((s, b) => s + b.receber, 0);
    const pagar = buckets.reduce((s, b) => s + b.pagar, 0);
    return { entrada, saida, saldo: entrada - saida, receber, pagar };
  }, [buckets]);

  const summaryRows = [
    row("Entradas do período", total.entrada, "ok"),
    row("Saídas do período", -total.saida, "critical"),
    row("Saldo", total.saldo, total.saldo >= 0 ? "ok" : "critical"),
    row("A receber em aberto", total.receber, "warning"),
    row("A pagar em aberto", -total.pagar, "warning"),
  ];

  return (
    <div className="space-y-3">
      <div className="rounded-2xl border border-border bg-card p-4 shadow-sm">
        <div className="flex items-center gap-2 mb-2">
          <Wallet className="h-4 w-4 text-primary-700" />
          <div>
            <p className="text-xs font-bold text-muted-foreground uppercase tracking-wide">Resumo do período</p>
            <p className="text-xs text-muted-foreground">
              Entradas (aluguel/recebimento) × saídas (comissão/repasse) por competência — {buckets.length}{" "}
              {buckets.length === 1 ? "mês" : "meses"} com lançamentos.
            </p>
          </div>
        </div>
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-2">
          {summaryRows.map((r) => (
            <div key={r.label} className="rounded-xl border border-border bg-muted/30 px-3 py-2">
              <p className="text-[11px] text-muted-foreground font-bold">{r.label}</p>
              <p className={clsx("text-sm font-mono font-bold tabular-nums", r.color)}>
                {r.value < 0 ? "− " : ""}
                {formatCurrencyBRL0(Math.abs(r.value))}
              </p>
            </div>
          ))}
        </div>
      </div>

      <div className="overflow-hidden rounded-2xl border border-border bg-card shadow-sm divide-y divide-border">
        {buckets.map((bucket) => {
          const open = selectedMonth === bucket.month;
          const kindRows = (["charge", "commission", "transfer", "royalty", "transaction"] as const)
            .filter((k) => Math.abs(bucket.byKind[k]) > 0.005)
            .map((k) => ({ label: KIND_LABEL[k], value: bucket.byKind[k], open: bucket.open[k] }));
          return (
            <div key={bucket.month}>
              <button
                type="button"
                onClick={() => onSelectMonth?.(open ? null : bucket.month)}
                className="w-full flex items-center justify-between gap-3 px-4 py-3 text-left hover:bg-muted/40 transition-colors"
              >
                <div className="flex items-center gap-3 min-w-0">
                  {open ? <ChevronDown className="h-4 w-4 text-muted-foreground" /> : <ChevronRight className="h-4 w-4 text-muted-foreground" />}
                  <div className="min-w-0">
                    <p className="text-sm font-bold text-foreground">{bucket.label}</p>
                    <p className="text-xs text-muted-foreground">
                      {bucket.count} {bucket.count === 1 ? "lançamento" : "lançamentos"}
                      {bucket.vencidos > 0 && (
                        <span className="ml-2 text-status-critical font-medium">{formatCurrencyBRL0(bucket.vencidos)} vencidos</span>
                      )}
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-3 shrink-0">
                  <span className="inline-flex items-center gap-1 text-xs font-bold text-status-ok">
                    <TrendingUp className="h-3.5 w-3.5" />
                    {formatCurrencyBRL0(bucket.entrada)}
                  </span>
                  <span className="inline-flex items-center gap-1 text-xs font-bold text-status-critical">
                    <TrendingDown className="h-3.5 w-3.5" />
                    {formatCurrencyBRL0(bucket.saida)}
                  </span>
                  <span
                    className={clsx(
                      "inline-flex items-center rounded-lg px-2 py-1 text-xs font-mono font-bold tabular-nums",
                      bucket.saldo >= 0 ? "bg-status-ok-soft text-status-ok" : "bg-status-critical-soft text-status-critical",
                    )}
                  >
                    {bucket.saldo < 0 ? "− " : ""}
                    {formatCurrencyBRL0(Math.abs(bucket.saldo))}
                  </span>
                </div>
              </button>

              {open && (
                <div className="px-4 pb-4 pt-1">
                  <div className="rounded-xl border border-border bg-muted/20 p-3">
                    <p className="text-[11px] font-bold text-muted-foreground uppercase tracking-wide mb-2">Composição do mês</p>
                    {kindRows.length === 0 ? (
                      <p className="text-xs text-muted-foreground">Sem valores neste mês.</p>
                    ) : (
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                        {kindRows.map((k) => (
                          <div key={k.label} className="flex items-center justify-between gap-3 rounded-lg bg-card border border-border px-3 py-2">
                            <span className="text-xs font-medium text-muted-foreground">{k.label}</span>
                            <span className="text-xs font-mono font-bold text-foreground tabular-nums">
                              {formatCurrencyBRL0(k.value)}
                              {k.open > 0.005 && (
                                <span className="ml-2 text-status-warning font-medium">({formatCurrencyBRL0(k.open)} aberto)</span>
                              )}
                            </span>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>
          );
        })}
        {buckets.length === 0 && (
          <div className="px-4 py-8 text-center text-xs text-muted-foreground">Nenhum lançamento no período filtrado.</div>
        )}
      </div>
    </div>
  );
}
