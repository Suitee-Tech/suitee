import { ReactNode } from "react";
import { AlertCircle, Loader2, RotateCw } from "lucide-react";
import { clsx } from "clsx";
import { Skeleton } from "../../../../shared/ui/skeleton";
import { Button } from "../../../../shared/ui/button";
import { useSessionRecovery } from "../../../../shared/context/SessionRecoveryContext";
import type { StatusTone } from "../../../../shared/tokens/types";

/**
 * Shared primitives for the esteira side panels. Goals:
 *  - One source of truth for loading skeletons (mirrors the final layout).
 *  - Consistent error/empty/section semantics across deal-details, parties,
 *    audit, and proposal panels.
 *  - Single status-dot vocabulary so users learn it once: ok / pending /
 *    warning / critical / info / neutral. All colors live in design tokens
 *    (toka/src/app/tokens) — this file just composes Tailwind classes.
 */

export type { StatusTone };

const TONE_DOT: Record<StatusTone, string> = {
  ok: "bg-status-ok",
  pending: "bg-status-pending",
  warning: "bg-status-warning",
  critical: "bg-status-critical",
  info: "bg-status-info",
  neutral: "bg-status-neutral",
};

const TONE_TEXT: Record<StatusTone, string> = {
  ok: "text-status-ok",
  pending: "text-status-pending",
  warning: "text-status-warning",
  critical: "text-status-critical",
  info: "text-status-info",
  neutral: "text-muted-foreground",
};

const TONE_PILL: Record<StatusTone, string> = {
  ok: "bg-status-ok-soft text-status-ok border-status-ok/20",
  pending: "bg-status-pending-soft text-status-pending border-status-pending/20",
  warning: "bg-status-warning-soft text-status-warning border-status-warning/20",
  critical: "bg-status-critical-soft text-status-critical border-status-critical/20",
  info: "bg-status-info-soft text-status-info border-status-info/20",
  neutral: "bg-muted text-muted-foreground border-border",
};

export function StatusDot({ tone, className }: { tone: StatusTone; className?: string }) {
  return <span aria-hidden="true" className={clsx("w-2 h-2 rounded-full inline-block", TONE_DOT[tone], className)} />;
}

export function StatusText({ tone, children, className }: { tone: StatusTone; children: ReactNode; className?: string }) {
  return <span className={clsx("text-xs font-semibold", TONE_TEXT[tone], className)}>{children}</span>;
}

export function StatusPill({
  tone,
  children,
  icon,
  className,
}: {
  tone: StatusTone;
  children: ReactNode;
  icon?: ReactNode;
  className?: string;
}) {
  return (
    <span
      className={clsx(
        "inline-flex items-center gap-1 text-[11px] font-bold px-2 py-0.5 rounded-full border",
        TONE_PILL[tone],
        className
      )}
    >
      {icon}
      {children}
    </span>
  );
}

/**
 * Section header used inside panel content. Provides a consistent rhythm:
 * uppercase eyebrow → title row → optional summary metric on the right.
 */
export function PanelSectionHeader({
  eyebrow,
  title,
  summary,
  action,
}: {
  eyebrow?: string;
  title: ReactNode;
  summary?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <header className="flex items-end justify-between gap-3 mb-3">
      <div className="min-w-0">
        {eyebrow && (
          <p className="text-caption font-bold text-muted-foreground uppercase tracking-[0.08em] mb-1">{eyebrow}</p>
        )}
        <h3 className="text-base font-bold text-foreground leading-tight truncate">{title}</h3>
      </div>
      <div className="flex items-center gap-2 shrink-0">
        {summary}
        {action}
      </div>
    </header>
  );
}

/**
 * Generic error state that fits inside a panel content area. The retry
 * callback is required so we never leave the user in a dead-end.
 */
export function PanelError({
  title = "Não foi possível carregar",
  description,
  onRetry,
  retryLabel = "Tentar novamente",
}: {
  title?: string;
  description?: string;
  onRetry: () => void;
  retryLabel?: string;
}) {
  // Enquanto a sessão está sendo restaurada não existe "erro" para mostrar: as
  // consultas falharam com a credencial vencida e vão ser refeitas. Sem este
  // guard o painel pisca "não foi possível carregar" antes de os dados entrarem
  // (o caso da aba parada por muito tempo).
  const { isRecovering } = useSessionRecovery();
  if (isRecovering) {
    return (
      <div
        role="status"
        className="rounded-2xl border border-border bg-card p-6 flex flex-col items-center gap-3 text-center"
      >
        <Loader2 className="w-5 h-5 animate-spin text-primary" aria-hidden="true" />
        <p className="text-sm text-muted-foreground">Restaurando a sessão e recarregando os dados...</p>
      </div>
    );
  }

  return (
    <div
      role="alert"
      className="rounded-2xl border border-status-critical/20 bg-status-critical-soft p-6 flex flex-col items-center text-center gap-3"
    >
      <div className="w-10 h-10 rounded-full bg-status-critical/10 text-status-critical flex items-center justify-center">
        <AlertCircle className="w-5 h-5" aria-hidden="true" />
      </div>
      <div>
        <p className="text-sm font-bold text-foreground">{title}</p>
        {description && <p className="text-xs text-muted-foreground mt-1 max-w-sm">{description}</p>}
      </div>
      <Button size="sm" variant="outline" onClick={onRetry} className="gap-2">
        <RotateCw className="w-3.5 h-3.5" />
        {retryLabel}
      </Button>
    </div>
  );
}

/**
 * Skeleton that mirrors a deal-details panel: header strip + tabs + 3 cards.
 * The proportions match the real layout so the perceived layout shift is
 * minimized when content arrives.
 */
export function DealPanelSkeleton() {
  return (
    <div className="space-y-4" aria-busy="true" aria-live="polite">
      <div className="flex items-center gap-3">
        <Skeleton className="w-12 h-12 rounded-full" />
        <div className="flex-1 space-y-2">
          <Skeleton className="h-4 w-1/2" />
          <Skeleton className="h-3 w-1/3" />
        </div>
      </div>
      <div className="grid grid-cols-5 gap-1 p-1 rounded-xl bg-muted/40">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-9 rounded-lg" />
        ))}
      </div>
      <div className="space-y-3">
        <Skeleton className="h-32 rounded-2xl" />
        <Skeleton className="h-24 rounded-2xl" />
        <Skeleton className="h-24 rounded-2xl" />
      </div>
    </div>
  );
}

/**
 * Skeleton for the parties panel. Three role groups, each with two rows.
 */
export function PartiesPanelSkeleton() {
  return (
    <div className="space-y-4" aria-busy="true" aria-live="polite">
      {Array.from({ length: 3 }).map((_, group) => (
        <div key={group} className="rounded-2xl border border-border bg-card overflow-hidden">
          <div className="px-4 py-3 border-b border-border flex items-center justify-between">
            <Skeleton className="h-4 w-32" />
            <Skeleton className="h-3 w-8" />
          </div>
          <div className="divide-y divide-border">
            {Array.from({ length: 2 }).map((_, row) => (
              <div key={row} className="px-4 py-3 flex items-start justify-between gap-3">
                <div className="flex-1 space-y-2">
                  <Skeleton className="h-4 w-1/2" />
                  <Skeleton className="h-3 w-2/3" />
                  <div className="flex gap-2 pt-1">
                    <Skeleton className="h-5 w-16 rounded-full" />
                    <Skeleton className="h-5 w-20 rounded-full" />
                  </div>
                </div>
                <Skeleton className="w-24 h-8 rounded-md" />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
