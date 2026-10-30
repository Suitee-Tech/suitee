export interface RetryOptions {
  attempts?: number;
  baseDelayMs?: number;
  shouldRetry?: (err: unknown) => boolean;
  onRetry?: (attempt: number, err: unknown) => void;
}

export async function retry<T>(fn: () => Promise<T>, opts: RetryOptions = {}): Promise<T> {
  const attempts = opts.attempts ?? 3;
  const baseDelayMs = opts.baseDelayMs ?? 500;
  const shouldRetry = opts.shouldRetry ?? defaultShouldRetry;

  let lastError: unknown;
  for (let i = 0; i < attempts; i++) {
    try {
      return await fn();
    } catch (err) {
      lastError = err;
      const isLast = i === attempts - 1;
      if (isLast || !shouldRetry(err)) {
        throw err;
      }
      opts.onRetry?.(i + 1, err);
      const delay = baseDelayMs * Math.pow(2, i);
      await sleep(delay);
    }
  }
  throw lastError;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// True ONLY for requests the browser cancelled on purpose — navigation, unmount,
// tab close. Those are expected and must not surface as application errors.
//
// `ERR_NETWORK` and `ECONNABORTED` are deliberately NOT cancellations: they mean
// the request never reached a server (offline, DNS, connection refused) or that
// it timed out. Listing them here turned a real outage into total silence on
// three fronts at once — `errorToast` suppressed the toast, `defaultShouldRetry`
// refused to retry, and the operator was left with a button that did nothing.
// Covered by e2e/new-lead.spec.ts "falha de rede mostra toast de erro com o
// contexto".
export function isAbortedError(e: { code?: string; name?: string; message?: string }): boolean {
  return (
    e?.code === "ERR_CANCELED" ||
    e?.code === "ERR_ABORTED" ||
    e?.name === "CanceledError" ||
    e?.name === "AbortError" ||
    e?.message === "canceled"
  );
}

function defaultShouldRetry(err: unknown): boolean {
  const e = err as { response?: { status?: number }; code?: string; name?: string; message?: string };

  // Aborted / canceled requests (browser navigation, tab close) are not
  // transient — the request is dead and retrying just wastes time.
  if (!e?.response) {
    if (isAbortedError(e)) return false;
    return true;
  }

  const status = e.response.status;
  // 503/504 mean the backend is already overloaded or timing out — retrying
  // immediately just piles more load on a struggling service during an
  // incident, so surface the failure instead of hammering it.
  if (status === 503 || status === 504) return false;
  if (status === 429) return true;
  if (status && status >= 500 && status < 600) return true;
  return false;
}
