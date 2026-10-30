import { reportClientError } from '../lib/errorDiagnostics';
import { getEnv } from '../../lib/runtime-env';
import { publishApiMutation } from '../lib/dataSync';
import { normalizeStorageUrl } from '../media';
import { isAbortedError } from '../lib/retry';
import { createHttpClient, unwrapData, type RetryableAxiosRequestConfig } from './httpClient';
import { runSessionRefresh } from './sessionRefresh';

export { normalizeStorageUrl } from '../media';

export const API_URL = getEnv('VITE_API_URL') || 'http://localhost:8080/api/v1';


// hasQueryAccessToken reconhece a URL que já carrega o token do app na query —
// o mesmo caminho que a tag <img> da prévia usa, e que dispensa header.
function hasQueryAccessToken(url: string): boolean {
  try {
    const parsed = new URL(url, typeof window !== "undefined" ? window.location.origin : "http://localhost");
    return parsed.searchParams.has("token");
  } catch {
    return false;
  }
}

// fetchPrivateContent baixa conteúdo privado da API (comprovante financeiro,
// arquivo da esteira digital, anexo do chat, download de documento) e devolve
// bytes para um blob URL.
//
// Por que não usar a URL direto no visualizador: react-pdf e <img> buscam a URL
// SEM o header Authorization, e as rotas privadas respondem 401 — o documento
// aparece na lista e não abre ("PDF load error: Unexpected server response (401)"),
// sem nenhum log no servidor.
//
// Por que um fetch cru em vez do cliente axios: várias dessas rotas respondem
// 302 para uma URL ASSINADA do Supabase Storage. O cliente axios manda
// `withCredentials`, e o navegador aplica o modo de credenciais também ao
// redirecionamento cross-origin — o Storage responde `Access-Control-Allow-Origin: *`
// e o CORS bloqueia ("must not be the wildcard '*' when the request's
// credentials mode is 'include'"), derrubando o anexo do chat. Com
// `credentials: "same-origin"` a perna cross-origin (a URL assinada) sai SEM
// credenciais — o wildcard é aceito e o token da própria URL assinada autoriza —
// e a perna da API continua autenticada pelo token/tenant enviados por header.
export async function fetchPrivateContent(
  url: string,
): Promise<{ blob: Blob; contentType: string | null }> {
  // URL já tokenizada (o padrão das telas: comGatewayToken) → requisição simples,
  // sem header nenhum. Sem token na URL, o Authorization autentica — e ele é
  // removido pelo navegador no redirecionamento cross-origin, mantendo a perna
  // assinada simples (sem preflight contra o Storage).
  const tokenizedUrl = hasQueryAccessToken(url);
  const headers: Record<string, string> = {};
  const token = getAccessToken();
  if (!tokenizedUrl && token) headers.Authorization = `Bearer ${token}`;

  const response = await fetch(url, { credentials: "same-origin", headers });
  if (!response.ok) {
    throw new Error(`private content request failed with status ${response.status}`);
  }
  const blob = await response.blob();
  return {
    blob,
    contentType: response.headers.get("content-type") || blob.type || null,
  };
}

export const getGatewayOrigin = (): string => {
 // Suporta API_URL absoluto (http://...) e relativo (/api/v1)
 try {
 return new URL(API_URL, window.location.origin).origin;
 } catch {
 return window.location.origin;
 }
};

export const getWsUrl = (): string => {
 const envWs = getEnv('VITE_WS_URL');
 if (envWs) return envWs;

 const apiUrl = new URL(API_URL, window.location.origin);
 const gatewayOrigin = apiUrl.origin;
 const wsOrigin = gatewayOrigin.replace(/^http/i, 'ws');
 const apiPath = apiUrl.pathname.replace(/\/+$/, '');
 return `${wsOrigin}${apiPath}/ws`;
};

export const toGatewayUrl = (resourceUrl: string): string => {
 if (!resourceUrl) return '';
 try {
 const gatewayOrigin = getGatewayOrigin();
 const normalizedResourceUrl = normalizeStorageUrl(resourceUrl);
 if (!normalizedResourceUrl) return '';
 const u = new URL(normalizedResourceUrl, gatewayOrigin); // suporta URL relativa

 // Toda URL pública/assinada do Storage já está pronta para o navegador,
 // inclusive quando o Supabase público roda em localhost.
 if (u.pathname.startsWith('/storage/v1/')) {
 return u.toString();
 }

 // Se vier apontando para um host/porta interno (ex.: toca-core, localhost:8086), reescreve para o origin do gateway
 const isInternalHost = u.hostname.startsWith('toca-') || u.hostname === 'localhost' || u.hostname === '127.0.0.1';
 if (u.origin !== gatewayOrigin && isInternalHost) {
 const gw = new URL(gatewayOrigin);
 u.protocol = gw.protocol;
 u.host = gw.host;
 }
 return u.toString();
 } catch {
 return resourceUrl;
 }
};

export const withGatewayToken = (resourceUrl: string, token: string | null): string => {
 if (!resourceUrl || !token) return resourceUrl;
 try {
 const gatewayOrigin = getGatewayOrigin();
 const u = new URL(resourceUrl, gatewayOrigin);
 if (u.origin !== gatewayOrigin) return resourceUrl;
 if (u.searchParams.has('token')) return resourceUrl;
 u.searchParams.set('token', token);
 return u.toString();
 } catch {
 return resourceUrl;
 }
};

export const unwrapApiData = unwrapData;

let cachedAccessToken: string | null = null

// SESSION_HINT_KEY is a localStorage flag we set when login succeeds and
// clear on logout / failed refresh. It exists ONLY so the response
// interceptor can decide whether to attempt POST /auth/refresh on a 401.
//
// Why we need it: the refresh_token is HttpOnly (good — JS can't read it),
// but that means the frontend has no way to ask "do I plausibly have a
// session?". Without this hint, a freshly opened tab with no session would
// hit any 401 and immediately fire /auth/refresh, which the backend rejects
// with 400 "refresh token is required" because the cookie is missing.
// That's harmless but pollutes the console and burns one round-trip.
const SESSION_HINT_KEY = 'auth_has_session';

// AUTH_RESTORE_FAILED_KEY evita o CICLO DE RECARGA no /login.
//
// Incidente de 2026-09-21 ("depois de trocar o perfil de acesso de um usuário e
// sair, a página ficou em loop infinito"): o bootstrap do AuthContext SEMPRE
// tentava `POST /auth/refresh` quando não havia token em memória — ignorando a
// dica de sessão que existe exatamente para isso. Quando o refresh passava mas
// a primeira chamada autenticada era recusada (vínculo trocado), o app limpava o
// estado e fazia `window.location.href = '/login'`; o reload refazia o refresh
// (cookie ainda válido), falhava de novo e recarregava — indefinidamente, com o
// Suspense mostrando "Carregando...".
//
// A marca registra "acabamos de falhar ao restaurar" por alguns segundos. Nesse
// intervalo o boot NÃO tenta restaurar sozinho: mostra o formulário de login.
const AUTH_RESTORE_FAILED_KEY = 'auth_restore_failed_at';
const AUTH_RESTORE_BACKOFF_MS = 15_000;

/** markAuthRestoreFailed registra que a restauração de sessão falhou agora. */
export const markAuthRestoreFailed = () => {
  try {
    sessionStorage.setItem(AUTH_RESTORE_FAILED_KEY, String(Date.now()));
  } catch {
  }
};

/** hasRecentAuthRestoreFailure evita tentar de novo em loop. */
export const hasRecentAuthRestoreFailure = (): boolean => {
  try {
    const raw = sessionStorage.getItem(AUTH_RESTORE_FAILED_KEY);
    if (!raw) return false;
    const at = Number(raw);
    if (!Number.isFinite(at)) return false;
    return Date.now() - at < AUTH_RESTORE_BACKOFF_MS;
  } catch {
    return false;
  }
};

/** clearAuthRestoreFailure é chamado quando um login dá certo. */
export const clearAuthRestoreFailure = () => {
  try {
    sessionStorage.removeItem(AUTH_RESTORE_FAILED_KEY);
  } catch {
  }
};

export const setAccessToken = (token: string | null) => {
 cachedAccessToken = token
 try {
   if (token) {
     localStorage.setItem(SESSION_HINT_KEY, '1');
   } else {
     localStorage.removeItem(SESSION_HINT_KEY);
   }
 } catch {
   // localStorage can throw in private mode / SSR; the in-memory cache
   // still works for the current tab.
 }
};

export const getAccessToken = (): string | null => {
 return cachedAccessToken
};

/** hasSessionHint returns true when a previous login was recorded in this
 *  browser. False on a fresh visit, after explicit logout, or after the
 *  refresh-token cookie is known to have failed. */
export const hasSessionHint = (): boolean => {
 if (cachedAccessToken) return true;
 try {
   return localStorage.getItem(SESSION_HINT_KEY) === '1';
 } catch {
   return false;
 }
};

const isAuthEndpoint = (url: string | undefined): boolean => {
  const u = String(url ?? '')
  return /\/auth\/(login|register|refresh|otp|forgot-password|reset-password|invitations)\b/i.test(u)
}

// switch-context / switch-company are authenticated calls that carry a
// Bearer token — but a 401 from them means the user's context is truly
// invalid (e.g. company deleted), not just an expired access token.
// Do NOT attempt token refresh on these; let the caller handle the
// error gracefully instead of triggering a hard redirect to /login.
const isContextSwitchEndpoint = (url: string | undefined): boolean =>
  /\/auth\/(switch-context|switch-company)\b/i.test(String(url ?? ''))

const api = createHttpClient({
  baseURL: API_URL,
  withCredentials: true,
  timeout: 45000,
  getAccessToken,
  setAccessToken,
  shouldAttachToken: (config) => !isAuthEndpoint(String(config.url ?? "")),
  getExtraHeaders: () => {
    const out: Record<string, string | undefined> = {};
    try {
      const tenantId = localStorage.getItem("current_company_id");
      if (tenantId) out["X-Tenant-Id"] = tenantId;
    } catch {
    }
    if (import.meta.env.DEV && getEnv("VITE_API_DEBUG") === "true") {
      out["X-Debug"] = "true";
    }
    return out;
  },
  shouldRefresh: ({ error, originalRequest }) => {
    if (error.response?.status !== 401) return false;
    if (originalRequest._retry) return false;
    if (isAuthEndpoint(originalRequest.url)) return false;
    if (isContextSwitchEndpoint(originalRequest.url)) return false;

    const isPortalRoute = typeof window !== "undefined" && window.location.pathname.startsWith("/portal");
    if (isPortalRoute) return false;

    if (!hasSessionHint()) return false;

    return true;
  },
  // A renovação é single-flight no módulo sessionRefresh: o interceptor, o boot
  // do AuthProvider e o SessionRecoveryProvider compartilham a MESMA troca de
  // refresh token (o Supabase invalida a sessão ao ver reuso de refresh token).
  refreshAccessToken: ({ client }) =>
    runSessionRefresh(async () => {
      const refreshResponse = await client.post<any>("/auth/refresh", {});
      const refreshData = refreshResponse.data?.data || refreshResponse.data;
      const newToken = refreshData?.access_token;
      if (!newToken) {
        throw new Error("No access token");
      }
      return newToken as string;
    }),
  onRefreshFailure: () => {
    setAccessToken(null);
    markAuthRestoreFailed();
    try {
      localStorage.removeItem("current_company_id");
    } catch {
    }
    // Sem reload quando já estamos no /login: recarregar ali refaz o refresh,
    // falha e recarrega de novo (o ciclo que travou a tela em "Carregando...").
    if (typeof window !== "undefined" && window.location?.pathname !== "/login") {
      window.location.href = "/login";
    }
  },
});

api.interceptors.response.use(
  (response) => {
    publishApiMutation(response.config.method, response.config.url);
    return response;
  },
  async (error) => {
    const originalRequest = error.config as RetryableAxiosRequestConfig | undefined;
    if (!originalRequest) return Promise.reject(error);

    // Requests aborted by the browser (navigation, tab close) are
    // expected and should not be treated as application errors.
    if (isAbortedError(error as any)) {
      // Still reject so callers can handle it, but do NOT log.
      return Promise.reject(error);
    }

    const status = error?.response?.status;
    const url = String(error?.response?.config?.url ?? "");
    const shouldLog = !(status === 400 && /\/auth\/refresh\b/i.test(url));
    if (shouldLog && typeof status === "number" && status >= 400 && status !== 401) {
      reportClientError(error);
    }

    return Promise.reject(error);
  }
);

export default api;
