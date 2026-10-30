import { useCallback, useEffect, useRef } from "react";

/**
 * Creates a guard for async work tied to a tenant/session scope.
 *
 * Capture `isCurrent` before starting a request and check it immediately
 * before committing its result. When the scope changes, every older guard
 * becomes invalid synchronously, including requests that cannot be aborted.
 */
export function useRequestScopeGuard(scope: unknown): () => () => boolean {
  const stateRef = useRef({ scope, generation: 0 });
  const lifecycleRef = useRef({ mounted: false, epoch: 0 });

  if (!Object.is(stateRef.current.scope, scope)) {
    stateRef.current = {
      scope,
      generation: stateRef.current.generation + 1,
    };
  }

  useEffect(() => {
    lifecycleRef.current.mounted = true;
    lifecycleRef.current.epoch += 1;

    return () => {
      lifecycleRef.current.mounted = false;
      lifecycleRef.current.epoch += 1;
    };
  }, []);

  const scopeGeneration = stateRef.current.generation;

  return useCallback(() => {
    const lifecycleEpoch = lifecycleRef.current.epoch;
    return () =>
      lifecycleRef.current.mounted &&
      lifecycleEpoch === lifecycleRef.current.epoch &&
      scopeGeneration === stateRef.current.generation;
  }, [scopeGeneration]);
}
