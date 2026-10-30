import { clsx } from "clsx";

import { Button } from "../../../../shared/ui/button";
import { StatusDot, type StatusTone } from "./PanelPrimitives";

export interface RequirementCardProps {
 eyebrow: string;
 status: string;
 hint?: string;
 blocking: boolean;
 ctaLabel?: string;
 onCta?: () => void;
}

export function RequirementCard({ eyebrow, status, hint, blocking, ctaLabel, onCta }: RequirementCardProps) {
 const tone: StatusTone = blocking ? "warning" : "ok";

 return (
  <div
   className={clsx(
    "p-4 rounded-2xl border shadow-sm flex items-start justify-between gap-3",
    blocking ? "bg-status-warning-soft border-status-warning/30" : "bg-card border-border",
   )}
  >
   <div className="min-w-0 flex items-start gap-2.5">
    <StatusDot tone={tone} className="mt-1.5" />
    <div>
     <p className="text-micro font-bold text-muted-foreground uppercase tracking-wide">{eyebrow}</p>
     <p className={clsx("mt-0.5 text-sm font-bold", blocking ? "text-status-warning" : "text-foreground")}>{status}</p>
     {hint && blocking && <p className="text-caption text-status-warning/85 mt-0.5">{hint}</p>}
    </div>
   </div>
   {blocking && ctaLabel && onCta && (
    <Button
     size="sm"
     variant="outline"
     className="h-8 border-status-warning/30 text-status-warning hover:bg-status-warning-soft shrink-0"
     onClick={onCta}
    >
     {ctaLabel}
    </Button>
   )}
  </div>
 );
}
