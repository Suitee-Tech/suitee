import { ReactNode } from "react";
import { AlertCircle, RefreshCw, LucideIcon } from "lucide-react";
import { Skeleton } from "../../../shared/ui/skeleton";

interface PortalLoadingProps {
  rows?: number;
  header?: boolean;
  className?: string;
}

export function PortalLoading({ rows = 3, header = true, className = "" }: PortalLoadingProps) {
  return (
    <div className={`space-y-3 px-5 pb-24 pt-8 ${className}`} aria-busy="true" aria-live="polite" role="status">
      <span className="sr-only">Carregando conteúdo</span>
      {header && (
        <div className="space-y-2">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-4 w-64" />
        </div>
      )}
      {Array.from({ length: rows }).map((_, idx) => (
        <Skeleton key={idx} className="h-24 rounded-2xl w-full" />
      ))}
    </div>
  );
}

interface PortalEmptyProps {
  icon: LucideIcon;
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
}

export function PortalEmpty({ icon: Icon, title, description, action, className = "" }: PortalEmptyProps) {
  return (
    <div
      className={`flex flex-col items-center justify-center text-center px-6 py-10 bg-card rounded-2xl border border-border shadow-token-sm ${className}`}
    >
      <div className="w-14 h-14 rounded-full bg-muted border border-border flex items-center justify-center mb-4">
        <Icon className="w-6 h-6 text-muted-foreground" strokeWidth={1.5} />
      </div>
      <h3 className="text-title font-semibold text-foreground">{title}</h3>
      {description && <p className="text-label text-muted-foreground mt-1 max-w-xs">{description}</p>}
      {action && <div className="mt-5">{action}</div>}
    </div>
  );
}

interface PortalErrorProps {
  title?: string;
  description?: string;
  onRetry?: () => void;
  retryLabel?: string;
  className?: string;
}

export function PortalError({
  title = "Não foi possível carregar",
  description = "Houve um erro ao buscar suas informações. Tente novamente em instantes.",
  onRetry,
  retryLabel = "Tentar novamente",
  className = "",
}: PortalErrorProps) {
  return (
    <div
      role="alert"
      className={`flex flex-col items-center justify-center text-center px-6 py-10 bg-card rounded-2xl border border-status-critical/20 shadow-token-sm ${className}`}
    >
      <div className="w-14 h-14 rounded-full bg-status-critical-soft border border-status-critical/20 flex items-center justify-center mb-4">
        <AlertCircle className="w-6 h-6 text-status-critical" strokeWidth={1.5} />
      </div>
      <h3 className="text-title font-semibold text-foreground">{title}</h3>
      <p className="text-label text-muted-foreground mt-1 max-w-xs">{description}</p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="mt-5 inline-flex min-h-11 items-center gap-2 rounded-xl bg-foreground px-4 py-2 text-label font-semibold text-background transition-colors hover:bg-foreground/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2"
        >
          <RefreshCw className="h-3.5 w-3.5" aria-hidden="true" />
          {retryLabel}
        </button>
      )}
    </div>
  );
}

interface PortalSectionProps {
  title?: string;
  subtitle?: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}

export function PortalSection({ title, subtitle, action, children, className = "" }: PortalSectionProps) {
  return (
    <section className={`bg-card rounded-2xl border border-border shadow-token-sm ${className}`}>
      {(title || action) && (
        <header className="flex items-start justify-between gap-3 px-4 py-3 border-b border-border">
          <div className="min-w-0">
            {title && <h2 className="text-sm font-bold text-foreground">{title}</h2>}
            {subtitle && <p className="text-caption text-muted-foreground mt-0.5">{subtitle}</p>}
          </div>
          {action && <div className="shrink-0">{action}</div>}
        </header>
      )}
      <div className="p-4">{children}</div>
    </section>
  );
}
