import { clsx } from "clsx";
import { TrendingUp, TrendingDown, Minus, ArrowRight } from "lucide-react";
import { useTheme } from "../../context/ThemeContext";

interface ComparisonIndicatorProps {
  currentValue: number;
  previousValue: number;
  format?: (v: number) => string;
  label?: string;
  previousLabel?: string;
  compact?: boolean;
}

export function ComparisonIndicator({
  currentValue,
  previousValue,
  format = (v) => String(v),
  label = "Atual",
  previousLabel = "Anterior",
  compact = false,
}: ComparisonIndicatorProps) {
  // Sem base de comparação não há variação a exibir. Antes o cálculo devolvia 0
  // e a interface anunciava "0.0%" — que parece "ficou igual" quando na verdade
  // não houve o que comparar (período anterior zerado).
  const hasBaseline = previousValue !== 0;
  const diff = hasBaseline ? ((currentValue - previousValue) / previousValue) * 100 : null;
  const trend = diff === null ? "none" : diff > 0 ? "up" : diff < 0 ? "down" : "neutral";
  const { resolvedMode } = useTheme();
  const isDark = resolvedMode === "dark";

  void isDark;

  if (compact) {
    return (
      <div className="inline-flex items-center gap-1.5">
        <span
          className={clsx(
            "inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-micro font-bold",
            trend === "up" && "bg-status-ok-soft text-status-ok",
            trend === "down" && "bg-status-critical-soft text-status-critical",
            trend === "neutral" && "bg-muted text-muted-foreground",
          )}
        >
          {trend === "up" && <TrendingUp className="w-2.5 h-2.5" />}
          {trend === "down" && <TrendingDown className="w-2.5 h-2.5" />}
          {(trend === "neutral" || trend === "none") && <Minus className="w-2.5 h-2.5" />}
          {diff === null ? "—" : `${diff > 0 ? "+" : ""}${diff.toFixed(1)}%`}
        </span>
      </div>
    );
  }

  return (
    <div className={clsx("rounded-xl border shadow-sm p-4", "bg-card border-border")}>
      {/*
        Empilhado por padrão e lado a lado a partir de sm. Os dois valores com o
        selo no meio não cabiam em coluna estreita: valores longos como
        "R$ 1.063.606,30" estouravam o cartão e cortavam o texto.
      */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
        <div className="min-w-0 text-center sm:flex-1">
          <p className={clsx("text-micro font-bold uppercase tracking-wider mb-1", "text-muted-foreground")}>{previousLabel}</p>
          <p className={clsx("truncate text-lg font-bold", "text-muted-foreground")} title={format(previousValue)}>{format(previousValue)}</p>
        </div>

        <div className="flex flex-col items-center gap-1">
          <ArrowRight className="w-4 h-4 rotate-90 text-muted-foreground/60 sm:rotate-0" />
          <span
            className={clsx(
              "inline-flex items-center gap-1 px-2 py-1 rounded-lg text-xs font-bold",
              trend === "up" && "bg-status-ok-soft text-status-ok border border-status-ok/20",
              trend === "down" && "bg-status-critical-soft text-status-critical border border-status-critical/20",
              (trend === "neutral" || trend === "none") && "bg-muted text-muted-foreground border border-border",
            )}
          >
            {trend === "up" && <TrendingUp className="w-3 h-3" />}
            {trend === "down" && <TrendingDown className="w-3 h-3" />}
            {(trend === "neutral" || trend === "none") && <Minus className="w-3 h-3" />}
            {diff === null ? "—" : `${diff > 0 ? "+" : ""}${diff.toFixed(1)}%`}
          </span>
        </div>

        <div className="min-w-0 text-center sm:flex-1">
          <p className={clsx("text-micro font-bold uppercase tracking-wider mb-1", "text-muted-foreground")}>{label}</p>
          <p className={clsx("truncate text-lg font-bold", "text-foreground")} title={format(currentValue)}>{format(currentValue)}</p>
        </div>
      </div>
      {label && hasBaseline && (
        <div className={clsx("mt-3 h-1.5 rounded-full overflow-hidden relative", "bg-muted")}>
          <div
            className={clsx(
              "absolute inset-y-0 left-0 rounded-full transition-all",
              trend === "up"
                ? "bg-gradient-to-r from-emerald-400 to-emerald-500"
                : trend === "down"
                  ? "bg-gradient-to-r from-rose-400 to-rose-500"
                  : "bg-muted-foreground/30",
            )}
            style={{
              width: `${Math.min((currentValue / previousValue) * 50, 100)}%`,
            }}
          />
        </div>
      )}
    </div>
  );
}
