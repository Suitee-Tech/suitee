import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";
import { type PropsWithChildren, useEffect, useState } from "react";
import { queryKeyMatchesResources, subscribeToDataChanges } from "../shared/lib/dataSync";
import { isRecoverableStatus, recoveryRetryDelay } from "../shared/lib/sessionRecovery";

export function resetQueriesForTenant(client: QueryClient) {
  return client.resetQueries(undefined, { cancelRefetch: true });
}

// Resources that should be skipped during QueryDataSync invalidation.
// Used to prevent stale-data refetches during optimistic drag-and-drop.
const invalidationResourceLocks = new Map<string, number>();

export function blockInvalidationResource(resource: string) {
  invalidationResourceLocks.set(resource, (invalidationResourceLocks.get(resource) ?? 0) + 1);
}

export function unblockInvalidationResource(resource: string) {
  const remaining = (invalidationResourceLocks.get(resource) ?? 0) - 1;
  if (remaining > 0) {
    invalidationResourceLocks.set(resource, remaining);
  } else {
    invalidationResourceLocks.delete(resource);
  }
}

export function isInvalidationResourceBlocked(resource: string) {
  return invalidationResourceLocks.has(resource);
}

/** Quantas vezes uma consulta recuperável é repetida antes de virar erro. */
export const QUERY_MAX_RETRIES = 3;

/**
 * Política de retry das consultas.
 *
 * O caso que motivou a revisão: aba parada por horas volta ao foco e a primeira
 * leva de consultas sai com o access token vencido (401) ou bate no gateway
 * recém-reiniciado (502/503/504). A versão anterior não repetia NENHUM desses e
 * a tela mostrava "não foi possível carregar" por um instante — até o refresh
 * da sessão terminar e a consulta ser refeita por outro gatilho.
 *
 * Agora 401/rede/timeout/429/5xx são repetidos com backoff (o cliente HTTP já
 * renova a sessão e repete a requisição); 403/404/409/422 continuam sem retry
 * porque insistir só atrasa o erro real para o usuário.
 */
export function shouldRetryQuery(failureCount: number, error: unknown) {
  if (failureCount >= QUERY_MAX_RETRIES) return false;
  const status = (error as { response?: { status?: number } } | undefined)?.response?.status;
  return isRecoverableStatus(status);
}

/** Backoff exponencial (400ms → 800ms → 1.6s, teto) entre as tentativas. */
export function queryRetryDelay(failureCount: number) {
  return recoveryRetryDelay(failureCount, 400);
}

function QueryDataSync({ client }: { client: QueryClient }) {
  useEffect(() => {
    let timer: number | null = null;
    const resources = new Set<string>();

    const flush = () => {
      timer = null;
      const changedResources = [...resources].filter((r) => !isInvalidationResourceBlocked(r));
      resources.clear();
      if (changedResources.length === 0) return;

      void client.invalidateQueries({
        predicate: (query) => queryKeyMatchesResources(query.queryKey, changedResources),
        refetchType: "active",
      });
    };

    const unsubscribe = subscribeToDataChanges((change) => {
      if (change.resetCache) {
        if (timer != null) {
          window.clearTimeout(timer);
          timer = null;
        }
        resources.clear();
        void resetQueriesForTenant(client);
        return;
      }

      change.resources.forEach((resource) => resources.add(resource));
      if (timer != null) window.clearTimeout(timer);
      timer = window.setTimeout(flush, 60);
    });

    return () => {
      unsubscribe();
      if (timer != null) window.clearTimeout(timer);
    };
  }, [client]);

  return null;
}

export function ReactQueryProvider({ children }: PropsWithChildren) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 30_000,
            gcTime: 10 * 60_000,
            retry: shouldRetryQuery,
            retryDelay: queryRetryDelay,
            refetchOnWindowFocus: true,
            refetchOnReconnect: "always",
          },
        },
      }),
  );

  return (
    <QueryClientProvider client={client}>
      <QueryDataSync client={client} />
      {children}
      {import.meta.env.DEV ? <ReactQueryDevtools initialIsOpen={false} /> : null}
    </QueryClientProvider>
  );
}
