import { AlertTriangle, RefreshCw } from "lucide-react";
import { clsx } from "clsx";

import { Button } from "./button";

/**
 * Falha de leitura de uma consulta. É distinto de vazio: quem consome precisa
 * renderizar este componente antes do estado vazio, para uma resposta 5xx ou
 * timeout nunca ser apresentada como "não há registros".
 *
 * O `role="alert"` anuncia a falha assim que ela aparece; o botão mantém o
 * mesmo texto em todas as telas e reexecuta a consulta que falhou.
 */
export function LoadError({
  title = "Não foi possível carregar",
  description,
  onRetry,
  retrying = false,
  className,
}: {
  title?: string;
  description: string;
  onRetry: () => unknown;
  retrying?: boolean;
  className?: string;
}) {
  return (
    <div
      role="alert"
      className={clsx(
        "flex flex-col items-center justify-center rounded-2xl border border-status-critical/30 bg-card p-8 text-center",
        className,
      )}
    >
      <div className="mb-4 flex size-14 items-center justify-center rounded-full border border-status-critical/20 bg-status-critical-soft">
        <AlertTriangle className="size-6 text-status-critical" strokeWidth={1.5} aria-hidden="true" />
      </div>
      <h3 className="text-lg font-semibold text-foreground">{title}</h3>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">{description}</p>
      <Button
        variant="outline"
        className="mt-5"
        onClick={() => {
          // A consulta continua no estado de erro se a nova tentativa falhar;
          // a rejeição não pode virar unhandledrejection na página.
          void Promise.resolve(onRetry()).catch(() => {});
        }}
        loading={retrying}
        loadingText="Tentando..."
      >
        <RefreshCw className="size-4" aria-hidden="true" />
        Tentar novamente
      </Button>
    </div>
  );
}
