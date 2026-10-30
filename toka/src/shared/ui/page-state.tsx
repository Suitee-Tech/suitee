import type { ReactNode } from "react";
import { AlertTriangle, RefreshCw } from "lucide-react";

import { Button } from "./button";
import { cn } from "./utils";

export interface PageStateProps {
  loading?: boolean;
  /** `true` usa a mensagem genérica; string/ReactNode é a mensagem exibida. */
  error?: boolean | ReactNode;
  onRetry?: () => void;
  retrying?: boolean;
  empty?: boolean;
  emptyState?: ReactNode;
  /** Skeleton específico do layout; default é uma pilha de cards. */
  skeleton?: ReactNode;
  children: ReactNode;
  className?: string;
}

/**
 * Contrato único de estado de consulta: loading, erro com retry e vazio.
 *
 * Antes cada página escolhia entre `<p>Carregando…</p>`, spinner central,
 * skeleton e EmptyState — às vezes no mesmo fluxo. A regra aqui é: enquanto
 * carrega, o layout de destino é preservado por um skeleton; se falha, o erro
 * é anunciado e oferece retry; se não há dados, o vazio tem uma ação/descrição.
 */
export function PageState({
  loading,
  error,
  onRetry,
  retrying,
  empty,
  emptyState,
  skeleton,
  children,
  className,
}: PageStateProps) {
  if (loading) {
    return (
      <div className={cn("space-y-3", className)}>
        <span role="status" className="sr-only">Carregando…</span>
        {skeleton ?? <PageStateSkeleton />}
      </div>
    );
  }

  if (error) {
    const message = typeof error === "boolean" ? "Não foi possível carregar os dados." : error;
    return (
      <div
        role="alert"
        className={cn(
          "flex flex-col items-start gap-3 rounded-xl border border-status-critical/30 bg-status-critical-soft p-4 text-sm text-status-critical",
          className,
        )}
      >
        <div className="flex items-start gap-2">
          <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <p className="font-medium">{message}</p>
        </div>
        {onRetry ? (
          <Button variant="outline" size="sm" onClick={onRetry} loading={retrying} loadingText="Tentando…">
            <RefreshCw className="size-4" aria-hidden="true" /> Tentar novamente
          </Button>
        ) : null}
      </div>
    );
  }

  if (empty) {
    return (
      <div className={className}>
        {emptyState ?? (
          <div className="rounded-xl border border-dashed border-border bg-card p-6 text-center">
            <p className="text-sm font-bold text-foreground">Nada por aqui</p>
            <p className="mt-1 text-sm text-muted-foreground">Não há registros para os filtros atuais.</p>
          </div>
        )}
      </div>
    );
  }

  return <>{children}</>;
}

export function PageStateSkeleton({ rows = 4 }: { rows?: number }) {
  return (
    <div aria-hidden="true" className="space-y-3">
      {Array.from({ length: rows }).map((_, index) => (
        <div key={index} className="rounded-xl border border-border bg-card p-4">
          <div className="flex items-center gap-3">
            <div className="size-9 shrink-0 rounded-lg bg-muted" />
            <div className="flex-1 space-y-2">
              <div className="h-3 w-1/3 rounded bg-muted" />
              <div className="h-3 w-1/2 rounded bg-muted" />
            </div>
            <div className="h-7 w-20 rounded-md bg-muted" />
          </div>
        </div>
      ))}
    </div>
  );
}
