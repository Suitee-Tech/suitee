import { useCallback, useEffect, useState } from 'react';

/** Results belong to a specific selection. Superseded requests never publish
 * data or clear the loading/error state of a newer selection. */
export function useFinancialRead<T>(key: string, load: () => Promise<T>, enabled = true) {
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<{ key: string; attempt: number; data?: T; error?: string }>();
  useEffect(() => {
    if (!enabled) return;
    let active = true;
    void load().then(
      data => { if (active) setState({ key, attempt, data }); },
      () => { if (active) setState({ key, attempt, error: 'Não foi possível carregar os dados completos desta seleção.' }); },
    );
    return () => { active = false; };
  }, [key, load, enabled, attempt]);
  const current = state?.key === key && state.attempt === attempt ? state : undefined;
  return {
    data: enabled ? current?.data : undefined,
    error: enabled ? current?.error : undefined,
    loading: enabled && !current,
    retry: useCallback(() => setAttempt(value => value + 1), []),
  };
}
