import { supportCode } from "../lib/errorDiagnostics";
import { AlertTriangle, RefreshCw, Home, Bug } from "lucide-react";
import { clsx } from "clsx";

export function ErrorFallback({
  error,
  resetError,
  variant = "page",
  className = "",
}: {
  error?: Error;
  resetError?: () => void;
  variant?: "page" | "inline";
  className?: string;
}) {
  if (variant === "inline") {
    return (
      <div
        className={clsx(
          "bg-card rounded-2xl border border-status-critical/30 p-8 flex flex-col items-center justify-center text-center",
          className,
        )}
      >
        <div className="w-12 h-12 rounded-full bg-status-critical-soft flex items-center justify-center mb-3 border border-status-critical/20">
          <AlertTriangle className="w-5 h-5 text-status-critical" strokeWidth={1.5} />
        </div>
        <h4 className="text-sm font-semibold text-foreground mb-1">Algo deu errado</h4>
        <p className="text-xs text-muted-foreground max-w-xs mb-4">Não foi possível carregar este conteúdo.</p>
        {supportCode(error) && <p className="text-xs text-muted-foreground mb-4">Código de suporte: {supportCode(error)}</p>}
        {resetError && (
          <button
            onClick={resetError}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-primary-600 bg-primary-50 hover:bg-primary-100 rounded-lg transition-colors"
          >
            <RefreshCw className="w-3 h-3" />
            Tentar novamente
          </button>
        )}
      </div>
    );
  }

  return (
    <div className={clsx("flex-1 flex items-center justify-center p-6 lg:p-8", className)}>
      <div className="bg-card rounded-2xl border border-border shadow-sm p-12 flex flex-col items-center justify-center text-center max-w-lg w-full">
        <div className="w-16 h-16 rounded-full bg-status-critical-soft flex items-center justify-center mb-4 border border-status-critical/20">
          <AlertTriangle className="w-8 h-8 text-status-critical" strokeWidth={1.5} />
        </div>
        <h3 className="text-lg font-semibold text-foreground mb-2">Algo deu errado</h3>
        <p className="text-sm text-muted-foreground max-w-sm mb-6">
          Ocorreu um erro inesperado ao carregar esta página. Tente recarregar ou voltar para o início.
        </p>

        {supportCode(error) && <p className="text-xs text-muted-foreground mb-4">Código de suporte: {supportCode(error)}</p>}
        {error && import.meta.env.DEV && (
          <details className="w-full mb-6 text-left">
            <summary className="text-xs font-medium text-muted-foreground cursor-pointer hover:text-muted-foreground transition-colors flex items-center gap-1.5">
              <Bug className="w-3 h-3" />
              Detalhes do erro
            </summary>
            <div className="mt-2 p-3 bg-muted border border-border rounded-xl">
              <p className="text-xs font-mono text-muted-foreground break-all whitespace-pre-wrap">{error.message}</p>
            </div>
          </details>
        )}

        <div className="flex items-center gap-3">
          {resetError && (
            <button
              onClick={resetError}
              className="inline-flex items-center gap-2 px-5 py-2.5 text-sm font-bold text-primary-foreground bg-primary hover:bg-primary/90 rounded-xl transition-colors shadow-sm"
            >
              <RefreshCw className="w-4 h-4" />
              Tentar novamente
            </button>
          )}
          <a
            href="/"
            className="inline-flex items-center gap-2 px-5 py-2.5 text-sm font-bold text-foreground bg-card border border-border hover:bg-muted rounded-xl transition-colors"
          >
            <Home className="w-4 h-4" />
            Voltar ao início
          </a>
        </div>
      </div>
    </div>
  );
}
