import { reportClientError } from "../../../shared/lib/errorDiagnostics";
import axios from 'axios';
import { getEnv } from '../../../lib/runtime-env';
import { publishApiMutation } from '../../../shared/lib/dataSync';
import { createHttpClient, unwrapData } from '../../../shared/services/httpClient';
import { createSessionRefreshHub } from '../../../shared/services/sessionRefresh';

/**
 * Isolated axios instance for the client portal.
 *
 * Completely separate from the admin `api.ts`:
 *  - Own in-memory access token (never touches the admin token)
 *  - Own 401 interceptor (only redirects to /portal/login)
 *  - Own refresh flow (uses the same HTTP-only cookie, but the
 *    token lifecycle is independent on the frontend)
 */
export const API_URL = getEnv('VITE_API_URL') || 'http://localhost:8080/api/v1';

// P2/M18 — o access token do portal vivia em `localStorage` (chave
// `portal_access_token`): um bearer de `cliente` legível por qualquer XSS,
// extensão de navegador ou por quem compartilha o dispositivo, com validade até
// expirar. O refresh já é um cookie HttpOnly (o axios do portal manda
// `withCredentials`), então o access token passa a viver APENAS em memória e a
// continuidade de sessão usa o cookie + um hint booleano (nunca o token).
const PORTAL_SESSION_HINT_KEY = 'portal_has_session';

let portalAccessToken: string | null = null;

/** hasPortalSessionHint diz se este navegador já teve sessão de portal — é o
 *  que autoriza tentar restaurar via cookie em vez de bater em /auth/me às
 *  cegas. É um booleano, não um segredo. */
// hasPortalSessionHint é apenas um marcador de conveniência (sessão já vista
// neste navegador). Ele NÃO decide se a sessão pode ser restaurada: quem manda é
// o cookie HttpOnly. Gating por hint quebrava sessões criadas antes do hint
// existir (aba nova, sessão antiga, localStorage limpo) — o usuário ficava na
// tela de login com um refresh válido no cookie.
export const hasPortalSessionHint = (): boolean => {
  if (portalAccessToken) return true;
  if (typeof window === 'undefined') return false;
  try {
    return window.localStorage.getItem(PORTAL_SESSION_HINT_KEY) === '1';
  } catch {
    return false;
  }
};

export const setPortalAccessToken = (token: string | null) => {
  portalAccessToken = token;
  if (typeof window === 'undefined') return;
  try {
    if (token) {
      window.localStorage.setItem(PORTAL_SESSION_HINT_KEY, '1');
    } else {
      window.localStorage.removeItem(PORTAL_SESSION_HINT_KEY);
    }
  } catch {}
};

export const getPortalAccessToken = (): string | null => portalAccessToken;

const isOnPortalLogin = () => typeof window !== 'undefined' && window.location.pathname === '/portal/login';

// T-03: o portal ganha o próprio hub de single-flight — o ciclo de sessão do
// portal (`/auth/portal/refresh`) é independente do app admin e os dois NUNCA
// podem compartilhar a mesma promise (reuso de cookie entre refresh de admin
// e de portal derrubaria a sessão). O hub também alimenta o
// SessionRecoveryProvider do portal (isRecovering + recuperação ao voltar o
// foco depois de inatividade).
const portalRefreshHub = createSessionRefreshHub();

export const isPortalRefreshActive = portalRefreshHub.isActive;
export const subscribePortalRefresh = portalRefreshHub.subscribe;

/** Troca o access token do portal via cookie HttpOnly e grava o token em
 *  memória. Single-flight: N chamadas concorrentes compartilham a MESMA
 *  troca de refresh token. */
export const refreshPortalAccessToken = (): Promise<string> =>
  portalRefreshHub.run(async () => {
    const response = await axios.post(`${API_URL}/auth/portal/refresh`, {}, { withCredentials: true });
    const data = response.data.data || response.data;
    const newToken = data.access_token;
    if (!newToken) throw new Error('No access token');
    setPortalAccessToken(newToken);
    return newToken as string;
  });

/**
 * shouldRefreshPortalRequest decide quando vale tentar renovar o access token.
 *
 * O cookie HttpOnly é a fonte de verdade da sessão: um 401 em rota protegida
 * tenta o refresh UMA vez, com ou sem `hasPortalSessionHint()`. Gating pelo hint
 * quebrava sessões criadas antes dele existir (aba nova, sessão antiga,
 * localStorage limpo) — o cliente ficava na tela de login com um refresh válido.
 * Exportado para teste direto da regra.
 */
export const shouldRefreshPortalRequest = ({
  error,
  originalRequest,
}: {
  error: { response?: { status?: number } };
  originalRequest: { _retry?: boolean; url?: string };
}): boolean => {
  if (error.response?.status !== 401) return false;
  if (originalRequest._retry) return false;
  if (String(originalRequest.url ?? '').includes('/auth/otp/')) return false;
  if (String(originalRequest.url ?? '').includes('/auth/portal/refresh')) return false;
  return true;
};

const portalApi = createHttpClient({
  baseURL: API_URL,
  withCredentials: true,
  getAccessToken: getPortalAccessToken,
  setAccessToken: setPortalAccessToken,
  shouldRefresh: shouldRefreshPortalRequest,
  refreshAccessToken: async () => {
    return refreshPortalAccessToken();
  },
  onRefreshFailure: ({ originalRequest }) => {
    setPortalAccessToken(null);
    if (!originalRequest._silent && !isOnPortalLogin()) {
      window.location.href = '/portal/login';
    }
  },
});

portalApi.interceptors.response.use((response) => {
  publishApiMutation(response.config.method, response.config.url);
  return response;
}, (error) => {
  if (error.response?.status >= 400 && error.response?.status !== 401) reportClientError(error);
  return Promise.reject(error);
});

export const unwrapPortalData = unwrapData;

export default portalApi;
