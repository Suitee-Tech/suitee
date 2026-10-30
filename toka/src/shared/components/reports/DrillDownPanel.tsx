import { useState, useCallback } from "react";
import { clsx } from "clsx";
import { motion, AnimatePresence } from "motion/react";
import { X, ArrowUpRight, TrendingUp, TrendingDown, Maximize2, Minimize2 } from "lucide-react";
import { useTheme } from "../../context/ThemeContext";
import { Button } from "../../ui/button";

export interface DrillDownData {
  title: string;
  subtitle?: string;
  records: DrillDownRecord[];
  summary?: { label: string; value: string; trend?: number }[];
}

export interface DrillDownRecord {
  id: string;
  label: string;
  sublabel?: string;
  value?: string;
  status?: string;
  statusColor?: string;
}

interface DrillDownPanelProps {
  data: DrillDownData | null;
  onClose: () => void;
}

export function DrillDownPanel({ data, onClose }: DrillDownPanelProps) {
  const { resolvedMode } = useTheme();
  const isDark = resolvedMode === "dark";
  const [isExpanded, setIsExpanded] = useState(false);

  const toggleExpand = useCallback(() => {
    setIsExpanded((prev) => !prev);
  }, []);

  return (
    <AnimatePresence>
      {data && (
        <>
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            onClick={onClose}
            className="fixed inset-0 bg-black/20 backdrop-blur-[2px] z-[60]"
          />
          <motion.div
            initial={{ x: "100%", opacity: 0.5 }}
            animate={{ x: 0, opacity: 1 }}
            exit={{ x: "100%", opacity: 0.5 }}
            transition={{ type: "spring", damping: 28, stiffness: 300 }}
            className={clsx(
              "fixed right-0 top-0 bottom-0 w-full shadow-2xl border-l z-[61] flex flex-col transition-[max-width] duration-300 ease-in-out",
              isExpanded ? "max-w-full" : "max-w-md",
              "bg-card border-border",
            )}
          >
            <div className={clsx("flex items-start justify-between px-6 py-5 border-b", "border-border")}>
              <div>
                <h2 className={clsx("text-lg font-bold", "text-foreground")}>{data.title}</h2>
                {data.subtitle && <p className={clsx("text-sm mt-0.5", "text-muted-foreground")}>{data.subtitle}</p>}
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={toggleExpand}
                  className={clsx(isExpanded && "bg-primary/10 text-primary border-primary/30 hover:bg-primary/15 hover:text-primary")}
                  title={isExpanded ? "Recolher painel" : "Expandir painel"}
                >
                  {isExpanded ? <Minimize2 className="w-4 h-4" /> : <Maximize2 className="w-4 h-4" />}
                  <span className="hidden sm:inline">{isExpanded ? "Recolher" : "Expandir"}</span>
                </Button>
                <Button variant="ghost" size="icon" onClick={onClose} className="size-9 text-muted-foreground">
                  <X className="w-5 h-5" />
                </Button>
              </div>
            </div>

            {data.summary && data.summary.length > 0 && (
              <div className={clsx("px-6 py-4 border-b grid grid-cols-2 gap-3", "border-border")}>
                {data.summary.map((s) => (
                  <div key={s.label} className={clsx("rounded-xl p-3", isDark ? "bg-foreground" : "bg-muted")}>
                    <p className={clsx("text-caption uppercase tracking-wider font-bold", "text-muted-foreground")}>{s.label}</p>
                    <p className={clsx("text-lg font-bold mt-1", "text-foreground")}>{s.value}</p>
                    {s.trend !== undefined && (
                      <div
                        className={clsx(
                          "flex items-center gap-1 text-caption font-bold mt-1",
                          s.trend >= 0 ? "text-status-ok" : "text-status-critical",
                        )}
                      >
                        {s.trend >= 0 ? <TrendingUp className="w-3 h-3" /> : <TrendingDown className="w-3 h-3" />}
                        {s.trend > 0 ? "+" : ""}
                        {s.trend.toFixed(1)}%
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}

            <div className="flex-1 overflow-y-auto px-6 py-4">
              <p className={clsx("text-caption font-bold uppercase tracking-wider mb-3", "text-muted-foreground")}>
                {data.records.length} registro{data.records.length !== 1 ? "s" : ""}
              </p>
              <div className="space-y-2">
                {data.records.map((rec) => (
                  <div
                    key={rec.id}
                    className={clsx(
                      "border rounded-xl p-3.5 transition-all group",
                      isDark
                        ? "bg-foreground border-foreground/30 hover:border-foreground/25 hover:shadow-sm"
                        : "bg-card border-border hover:border-border hover:shadow-sm",
                    )}
                  >
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <p className={clsx("text-sm font-bold truncate", "text-foreground")}>{rec.label}</p>
                        {rec.sublabel && <p className={clsx("text-xs mt-0.5 truncate", "text-muted-foreground")}>{rec.sublabel}</p>}
                      </div>
                      <div className="flex items-center gap-2 flex-shrink-0">
                        {rec.value && (
                          <span className={clsx("text-sm font-bold", isDark ? "text-muted-foreground/60" : "text-foreground")}>{rec.value}</span>
                        )}
                        <ArrowUpRight className="w-3.5 h-3.5 text-muted-foreground/60 group-hover:text-primary-500 transition-colors" />
                      </div>
                    </div>
                    {rec.status && (
                      <span
                        className={clsx(
                          "inline-block mt-2 px-2 py-0.5 text-micro font-bold rounded-full border",
                          rec.statusColor || "bg-muted text-muted-foreground border-border",
                        )}
                      >
                        {rec.status}
                      </span>
                    )}
                  </div>
                ))}
                {data.records.length === 0 && (
                  <div className={clsx("text-center py-12 text-sm", "text-muted-foreground")}>Nenhum registro encontrado</div>
                )}
              </div>
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  );
}
