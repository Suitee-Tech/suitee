import { useEffect, useMemo, useRef } from "react";
import { dataChangeMatchesResources, subscribeToDataChanges } from "../lib/dataSync";

export function useDataChangeRefresh(
  resources: readonly string[],
  refresh: () => void | Promise<void>,
  options: { enabled?: boolean; debounceMs?: number } = {},
) {
  const refreshRef = useRef(refresh);
  const enabledRef = useRef(options.enabled ?? true);
  const timerRef = useRef<number | null>(null);
  const runningRef = useRef(false);
  const queuedRef = useRef(false);
  const debounceMs = options.debounceMs ?? 80;
  const resourceKey = useMemo(() => [...resources].sort().join("|"), [resources]);

  refreshRef.current = refresh;
  enabledRef.current = options.enabled ?? true;

  useEffect(() => {
    const clearTimer = () => {
      if (timerRef.current != null) {
        window.clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };

    const run = async () => {
      timerRef.current = null;
      if (!enabledRef.current) {
        queuedRef.current = false;
        return;
      }
      if (runningRef.current) {
        queuedRef.current = true;
        return;
      }

      runningRef.current = true;
      queuedRef.current = false;
      try {
        await refreshRef.current();
      } catch {
        // Background reconciliation must not surface an unhandled rejection.
      } finally {
        runningRef.current = false;
        if (queuedRef.current && enabledRef.current) {
          clearTimer();
          timerRef.current = window.setTimeout(run, debounceMs);
        }
      }
    };

    const unsubscribe = subscribeToDataChanges((change) => {
      if (!enabledRef.current || !dataChangeMatchesResources(change, resourceKey.split("|"))) return;
      queuedRef.current = true;
      clearTimer();
      timerRef.current = window.setTimeout(run, debounceMs);
    });

    return () => {
      unsubscribe();
      clearTimer();
      queuedRef.current = false;
    };
  }, [debounceMs, resourceKey]);
}
