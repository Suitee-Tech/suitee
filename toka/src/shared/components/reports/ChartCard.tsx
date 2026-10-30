import { useState, type ReactNode } from "react";
import { clsx } from "clsx";
import { motion } from "motion/react";
import { Maximize2, Minimize2, Download } from "lucide-react";
import { Button } from "../../ui/button";

interface ChartCardProps {
  title: string;
  subtitle?: string;
  children: ReactNode;
  actions?: ReactNode;
  className?: string;
  delay?: number;
  fullHeight?: boolean;
  onExport?: () => void;
}

export function ChartCard({
  title,
  subtitle,
  children,
  actions,
  className,
  delay = 0,
  fullHeight,
  onExport,
}: ChartCardProps) {
  const [expanded, setExpanded] = useState(false);

  return (
    <motion.div
      initial={{ opacity: 0, y: 16 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.35, delay }}
      className={clsx(
        "rounded-2xl border border-border bg-card shadow-sm flex flex-col overflow-hidden min-w-0",
        expanded && "col-span-full",
        fullHeight && "h-full",
        className,
      )}
    >
      <div className={clsx("px-5 py-4 border-b flex items-center justify-between gap-3", "border-border")}>
        <div className="min-w-0">
          <h3 className={clsx("text-sm font-bold truncate", "text-foreground")}>{title}</h3>
          {subtitle && <p className={clsx("text-[11px] mt-0.5 truncate", "text-muted-foreground")}>{subtitle}</p>}
        </div>
        <div className="flex items-center gap-1 flex-shrink-0">
          {actions}
          {onExport && (
            <Button
              variant="ghost"
              size="icon"
              onClick={onExport}
              className="size-8 text-muted-foreground hover:text-primary"
              title="Exportar dados"
            >
              <Download className="w-3.5 h-3.5" />
            </Button>
          )}
          <Button
            variant="ghost"
            size="icon"
            onClick={() => setExpanded((e) => !e)}
            className="size-8 text-muted-foreground"
            title={expanded ? "Reduzir" : "Expandir"}
          >
            {expanded ? <Minimize2 className="w-3.5 h-3.5" /> : <Maximize2 className="w-3.5 h-3.5" />}
          </Button>
        </div>
      </div>

      <div
        className={clsx("p-5 min-w-0 flex-1", expanded ? "min-h-[400px]" : "min-h-[300px]")}
        style={{ minWidth: 1, overflow: "hidden" }}
      >
        {children}
      </div>
    </motion.div>
  );
}
