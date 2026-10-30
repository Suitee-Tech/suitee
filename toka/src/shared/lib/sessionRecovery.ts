// Recuperação de sessão após inatividade no navegador.
//
// O navegador congela timers e a aba pode passar horas parada: quando o usuário
// volta e navega, o access token em memória já venceu e as requisições da página
// saem com credencial inválida. O cliente HTTP já troca o refresh token em um
// 401, mas a primeira leva de respostas chega antes disso — e a tela mostrava
// "erro ao carregar" por um instante, antes de os dados entrarem.
//
// Este módulo concentra as decisões puras dessa recuperação (testáveis sem
// React):
//   - o token está vencido/vencendo? (decodifica o `exp` do JWT, sem lib);
//   - esse status HTTP é recuperável (vale tentar de novo depois de renovar a
//     sessão) ou é erro real (403/404/409/422 = não insista)?

/** Margem antes do vencimento para renovar proativamente (segundos). */
export const TOKEN_RENEW_SKEW_SECONDS = 90;

/** Volta a checar a sessão depois desse tempo sem interação (ms). */
export const WAKE_RECOVERY_MIN_IDLE_MS = 60_000;

/** Teto de espera entre tentativas de renovação (ms). */
export const RECOVERY_RETRY_MAX_DELAY_MS = 4_000;

/** Status em que insistir faz sentido: a sessão pode ser renovada ou o serviço
 *  estava subindo/instável. 403/404/409/422 são erros reais — insistir só
 *  atrasa a mensagem para o usuário. */
export function isRecoverableStatus(status: number | undefined | null): boolean {
  if (status == null || status === 0) return true; // rede/timeout/abort do navegador
  if (status === 401) return true; // access token vencido — renovável
  if (status === 408 || status === 429) return true;
  if (status === 502 || status === 503 || status === 504) return true;
  return status >= 500;
}

function base64UrlToJson(segment: string): unknown {
  const normalized = segment.replace(/-/g, "+").replace(/_/g, "/");
  const padded = normalized + "=".repeat((4 - (normalized.length % 4)) % 4);
  const decoded =
    typeof atob === "function"
      ? atob(padded)
      : typeof Buffer !== "undefined"
        ? Buffer.from(padded, "base64").toString("binary")
        : "";
  if (!decoded) return null;
  try {
    return JSON.parse(decodeURIComponent(escape(decoded)));
  } catch {
    return null;
  }
}

/** Instante de expiração (ms) do JWT, ou null quando não dá para ler. */
export function tokenExpiryMs(token: string | null | undefined): number | null {
  if (!token) return null;
  const parts = token.split(".");
  if (parts.length < 2) return null;
  const payload = base64UrlToJson(parts[1]) as { exp?: unknown } | null;
  const exp = payload?.exp;
  if (typeof exp !== "number" || !Number.isFinite(exp) || exp <= 0) return null;
  return exp * 1000;
}

/**
 * A sessão precisa ser renovada antes de a página carregar?
 *
 * Verdadeiro quando não há token em memória (aba nova, F5 depois de horas) ou
 * quando ele vence dentro da margem. Sem `exp` legível devolve true só quando
 * não há token — um JWT opaco não justifica renovação a cada foco de aba.
 */
export function needsSessionRecovery(token: string | null | undefined, now: number = Date.now()): boolean {
  if (!token) return true;
  const expiry = tokenExpiryMs(token);
  if (expiry == null) return false;
  return expiry - now <= TOKEN_RENEW_SKEW_SECONDS * 1000;
}

/**
 * A aba voltou ao foco depois de tempo suficiente para valer uma checagem?
 * Evita renovar a sessão a cada alt-tab: só depois de `minIdleMs` sem interação
 * — e ainda assim só se o token estiver vencendo.
 */
export function shouldCheckSessionOnWake(
  lastActivityAt: number,
  now: number = Date.now(),
  minIdleMs: number = WAKE_RECOVERY_MIN_IDLE_MS,
): boolean {
  if (!Number.isFinite(lastActivityAt) || lastActivityAt <= 0) return true;
  return now - lastActivityAt >= minIdleMs;
}

/** Backoff exponencial com teto, para as tentativas de recuperação/refetch. */
export function recoveryRetryDelay(attempt: number, baseMs = 400): number {
  const safeAttempt = Math.max(0, Math.trunc(attempt));
  const delay = baseMs * Math.pow(2, safeAttempt);
  return Math.min(delay, RECOVERY_RETRY_MAX_DELAY_MS);
}
