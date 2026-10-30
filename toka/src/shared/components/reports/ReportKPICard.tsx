import { clsx } from "clsx";
import { motion } from "motion/react";
import { TrendingUp, TrendingDown, Minus } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { useTheme } from "../../context/ThemeContext";

interface ReportKPICardProps {
  label: string;
  value: string;
  change?: number;
  changeLabel?: string;
  icon: LucideIcon;
  iconBg: string;
  iconColor: string;
  delay?: number;
  sparkData?: number[];
}

export function ReportKPICard({
  label,
  value,
  change,
  changeLabel,
  icon: Icon,
  iconBg,
  iconColor,
  delay = 0,
  sparkData,
}: ReportKPICardProps) {
  const trend = change === undefined ? "neutral" : change > 0 ? "up" : change < 0 ? "down" : "neutral";
  const { resolvedMode } = useTheme();
  const isDark = resolvedMode === "dark";

  return (
    <motion.div
      initial={{ opacity: 0, y: 16 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.35, delay }}
      className={clsx(
        // Superfície do tema nos DOIS modos. O `bg-foreground/90` do noturno
        // pintava o card com 90% da cor do texto (quase branco) enquanto valor e
        // rótulo usam `text-foreground`/`text-muted-foreground` — texto claro
        // sobre card claro, ilegível (era o "R$ 0" apagado dos Relatórios).
        "rounded-2xl border shadow-sm p-5 flex flex-col gap-3 transition-colors",
        "bg-card border-border hover:border-border",
      )}
    >
      <div className="flex items-start justify-between">
        <div className={clsx("w-10 h-10 rounded-xl flex items-center justify-center", iconBg)}>
          <Icon className={clsx("w-5 h-5", iconColor)} />
        </div>
        {change !== undefined && (
          <div
            className={clsx(
              "flex items-center gap-1 px-2 py-1 rounded-lg text-xs font-bold",
              trend === "up" && "bg-status-ok-soft text-status-ok",
              trend === "down" && "bg-status-critical-soft text-status-critical",
              trend === "neutral" && "bg-muted text-muted-foreground",
            )}
          >
            {trend === "up" && <TrendingUp className="w-3 h-3" />}
            {trend === "down" && <TrendingDown className="w-3 h-3" />}
            {trend === "neutral" && <Minus className="w-3 h-3" />}
            {change > 0 ? "+" : ""}
            {change.toFixed(1)}%
          </div>
        )}
      </div>

      <div>
        <p className={clsx("text-2xl font-bold tracking-tight", "text-foreground")}>{value}</p>
        <p className={clsx("text-xs mt-0.5", "text-muted-foreground")}>{label}</p>
      </div>

      {sparkData && sparkData.length > 1 && (
        <div className="h-8 flex items-end gap-[3px]">
          {sparkData.map((v, i) => {
            const max = Math.max(...sparkData);
            const h = max > 0 ? (v / max) * 100 : 0;
            return (
              <div
                key={i}
                className={clsx(
                  "flex-1 rounded-sm min-h-[2px] transition-all",
                  i === sparkData.length - 1
                    ? trend === "up"
                      ? "bg-status-ok/80"
                      : trend === "down"
                        ? "bg-status-critical/80"
                        : "bg-primary-400"
                    : isDark
                      // Barra neutra: um cinza claro translúcido sobre o card
                      // escuro. `bg-foreground/75` era quase branco sólido.
                      ? "bg-foreground/25"
                      : "bg-muted",
                )}
                style={{ height: `${Math.max(h, 5)}%` }}
              />
            );
          })}
        </div>
      )}

      {changeLabel && <p className={clsx("text-[11px]", "text-muted-foreground")}>{changeLabel}</p>}
    </motion.div>
  );
}
