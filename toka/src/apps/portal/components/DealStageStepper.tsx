import { Check, X } from "lucide-react";
import { clsx } from "clsx";
import type { DealStatus } from "../../../shared/services/deal.service";
import { MAIN_STAGES, getStageIndex, getStageMeta, isCancelled, type StageMeta } from "../../../shared/deals/dealStages";

const ACCENT_BG: Record<StageMeta["accent"], string> = {
  slate: "bg-stage-negotiation",
  blue: "bg-stage-proposal",
  amber: "bg-stage-analysis",
  purple: "bg-stage-contract",
  emerald: "bg-stage-converted",
  rose: "bg-stage-cancelled",
};

const ACCENT_RING: Record<StageMeta["accent"], string> = {
  slate: "ring-stage-negotiation/15",
  blue: "ring-stage-proposal/15",
  amber: "ring-stage-analysis/15",
  purple: "ring-stage-contract/15",
  emerald: "ring-stage-converted/15",
  rose: "ring-stage-cancelled/15",
};

interface DealStageStepperProps {
  status: DealStatus;
  className?: string;
  onStageClick?: (status: DealStatus) => void;
}

export function DealStageStepper({ status, className, onStageClick }: DealStageStepperProps) {
  const cancelled = isCancelled(status);
  const currentIdx = cancelled ? -1 : getStageIndex(status);

  return (
    <nav aria-label="Estágios do negócio" className={clsx("bg-card border border-border rounded-2xl shadow-sm px-3 py-3", className)}>
      <ol className="flex items-center w-full gap-1">
        {MAIN_STAGES.map((stageKey, idx) => {
          const meta = getStageMeta(stageKey);
          const completed = !cancelled && idx < currentIdx;
          const current = !cancelled && idx === currentIdx;
          const isLast = idx === MAIN_STAGES.length - 1;
          const Wrapper = onStageClick ? "button" : "div";
          const accent = current ? ACCENT_BG[meta.accent] : completed ? "bg-foreground/90" : "bg-border";
          const ringClass = current ? ACCENT_RING[meta.accent] : "";

          return (
            <li key={stageKey} className="flex-1 flex items-center min-w-0">
              <Wrapper
                type={onStageClick ? "button" : undefined}
                onClick={onStageClick ? () => onStageClick(stageKey) : undefined}
                aria-current={current ? "step" : undefined}
                className={clsx("flex flex-col items-center gap-1 min-w-0 flex-1", onStageClick && "cursor-pointer")}
              >
                <span
                  className={clsx(
                    "w-7 h-7 rounded-full flex items-center justify-center text-white text-[11px] font-bold flex-shrink-0",
                    accent,
                    current && "ring-4 ring-offset-0",
                    current && ringClass,
                  )}
                >
                  {completed ? <Check className="w-3.5 h-3.5" strokeWidth={3} /> : idx + 1}
                </span>
                <span
                  className={clsx(
                    "text-caption font-semibold text-center leading-tight max-w-full truncate w-full px-0.5",
                    current ? "text-foreground" : completed ? "text-foreground/80" : "text-muted-foreground/70",
                  )}
                >
                  {meta.label}
                </span>
              </Wrapper>
              {!isLast && (
                <span
                  aria-hidden="true"
                  className={clsx("h-[2px] w-6 mx-0.5 self-center -mt-3 flex-shrink-0", completed ? "bg-foreground/90" : "bg-border")}
                />
              )}
            </li>
          );
        })}
      </ol>
      {cancelled && (
        <div className="mt-3 flex items-center gap-2 px-3 py-2 rounded-lg bg-status-critical-soft border border-status-critical/20 text-status-critical">
          <X className="w-4 h-4" aria-hidden="true" />
          <span className="text-[12px] font-bold">Negócio cancelado</span>
        </div>
      )}
    </nav>
  );
}

