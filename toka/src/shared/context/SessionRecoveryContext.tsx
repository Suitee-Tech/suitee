import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { getAccessToken, hasSessionHint, markAuthRestoreFailed } from "../services/api";
import { refreshTokenRequest } from "../services/auth.service";
import { isSessionRefreshActive, subscribeSessionRefresh } from "../services/sessionRefresh";
import { publishDataChange } from "../lib/dataSync";
import {
  isRecoverableStatus,
  needsSessionRecovery,
  recoveryRetryDelay,
  shouldCheckSessionOnWake,
} from "../lib/sessionRecovery";

/**
 * SessionRecoveryContext — restaura a sessão ANTES de a página carregar.
 *
 * Cenário que motivou: a aba fica horas parada, o access token vence e o
 * usuário volta navegando. As consultas da página saíam com o token vencido e a
 * tela mostrava "não foi possível carregar" por um instante — até o refresh
 * responder e os dados entrarem. Aqui a sessão é renovada de forma proativa (no
 * boot, ao voltar o foco depois de tempo parado e ao reconectar) e o
 * `ProtectedRoute` segura a renderização enquanto `isRecovering` for verdadeiro:
 * a página nem monta, então não existe erro para piscar.
 *
 * A renovação é single-flight: N consultas falhando com 401 ao mesmo tempo
 * compartilham a mesma troca de refresh token (mesma proteção do httpClient).
 */
export type SessionRecoveryState = {
  /** Renovação em andamento: a UI deve mostrar carregando, não erro. */
  isRecovering: boolean;
  /** Última recuperação bem-sucedida (ms epoch), 0 quando nunca houve. */
  lastRecoveredAt: number;
  /** Tenta renovar agora. `true` = sessão válida (renovada ou ainda vigente). */
  recover: (options?: { force?: boolean }) => Promise<boolean>;
  /** Registra interação do usuário — usado para medir inatividade. */
  touch: () => void;
};

const SessionRecoveryContext = createContext<SessionRecoveryState>({
  isRecovering: false,
  lastRecoveredAt: 0,
  recover: async () => false,
  touch: () => {},
});

const MAX_RECOVERY_ATTEMPTS = 3;

export function useSessionRecovery() {
  return useContext(SessionRecoveryContext);
}

export type SessionRecoveryProviderOptions = {
  /**
   * Troca de refresh token deste ciclo de sessão. O default é o app admin
   * (`refreshTokenRequest` → POST /auth/refresh); o portal passa
   * `refreshPortalAccessToken` (POST /auth/portal/refresh).
   */
  refreshAccessToken?: () => Promise<unknown>;
  /** Dica de sessão deste app. Default: hasSessionHint (app admin). */
  hasSessionHint?: () => boolean;
  /** Access token em memória deste app. Default: getAccessToken (app admin). */
  getAccessToken?: () => string | null;
  /** Sinal "renovação em andamento" deste ciclo. Default: hub do app admin. */
  isRefreshActive?: () => boolean;
  /** Observador do sinal acima. Default: hub do app admin. */
  subscribeRefresh?: (listener: (active: boolean) => void) => () => void;
};

export function SessionRecoveryProvider({
  children,
  refreshAccessToken = refreshTokenRequest,
  hasSessionHint: checkHint = hasSessionHint,
  getAccessToken: readToken = getAccessToken,
  isRefreshActive = isSessionRefreshActive,
  subscribeRefresh = subscribeSessionRefresh,
}: SessionRecoveryProviderOptions & { children: ReactNode }) {
  const [isRecovering, setIsRecovering] = useState(false);
  const [lastRecoveredAt, setLastRecoveredAt] = useState(0);
  const inFlight = useRef<Promise<boolean> | null>(null);
  const lastActivityRef = useRef(Date.now());
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  // Qualquer renovação de sessão — inclusive a disparada pelo interceptor do
  // httpClient depois de um 401 — deixa a UI em "carregando" em vez de erro:
  // é o instante em que as consultas da página falharam e vão ser refeitas.
  useEffect(() => {
    if (isRefreshActive()) setIsRecovering(true);
    return subscribeRefresh((isActive) => {
      if (mountedRef.current) setIsRecovering(isActive);
    });
  }, [isRefreshActive, subscribeRefresh]);

  const touch = useCallback(() => {
    lastActivityRef.current = Date.now();
  }, []);

  const recover = useCallback(async (options?: { force?: boolean }): Promise<boolean> => {
    const force = options?.force ?? false;
    // Sem dica de sessão não há o que restaurar: visita nova ou logout. Nesse
    // caso o fluxo normal (login) assume — não marcamos falha de restauração.
    if (!checkHint()) return false;

    if (!force && !needsSessionRecovery(readToken())) return true;

    if (inFlight.current) return inFlight.current;

    const run = (async () => {
      setIsRecovering(true);
      try {
        for (let attempt = 0; attempt < MAX_RECOVERY_ATTEMPTS; attempt += 1) {
          try {
            await refreshAccessToken();
            // O token novo é gravado pelo próprio refresh; avisar o
            // app faz usuário/permissões recarregarem — sem isso a tela seguiria
            // com o contexto antigo (e um `can()` possivelmente vazio).
            // "manual": a mudança não veio de uma requisição do app, veio da
            // renovação da sessão — os contextos que dependem de usuário e
            // permissões escutam esse evento e se recarregam.
            publishDataChange(
              { source: "manual", resources: ["permissions", "users", "profile", "companies", "settings"] },
              { broadcast: false },
            );
            if (mountedRef.current) {
              setLastRecoveredAt(Date.now());
            }
            return true;
          } catch (error) {
            const status = (error as { response?: { status?: number } })?.response?.status;
            const canRetry = attempt < MAX_RECOVERY_ATTEMPTS - 1 && isRecoverableStatus(status);
            if (!canRetry) {
              // Refresh recusado de verdade (cookie expirado/revogado): o
              // usuário precisa logar de novo. `onRefreshFailure` do httpClient
              // cuida do redirecionamento quando a falha vem de uma requisição.
              markAuthRestoreFailed();
              return false;
            }
            await new Promise((resolve) => setTimeout(resolve, recoveryRetryDelay(attempt)));
          }
        }
        return false;
      } finally {
        if (mountedRef.current) setIsRecovering(false);
      }
    })();

    inFlight.current = run;
    try {
      return await run;
    } finally {
      inFlight.current = null;
    }
  }, [refreshAccessToken, checkHint, readToken]);

  // Volta do foco/reconexão depois de tempo parado: renova antes de a página
  // disparar a primeira consulta.
  useEffect(() => {
    const onWake = () => {
      if (typeof document !== "undefined" && document.visibilityState === "hidden") return;
      if (!shouldCheckSessionOnWake(lastActivityRef.current)) return;
      void recover();
    };

    const onActivity = () => touch();

    const listeners: Array<[EventTarget, string, EventListener]> = [
      [document, "visibilitychange", onWake as EventListener],
      [window, "focus", onWake as EventListener],
      [window, "online", onWake as EventListener],
      [window, "pointerdown", onActivity as EventListener],
      [window, "keydown", onActivity as EventListener],
    ];
    listeners.forEach(([target, event, handler]) => target?.addEventListener(event, handler));

    return () => {
      listeners.forEach(([target, event, handler]) => target?.removeEventListener(event, handler));
    };
  }, [recover, touch]);

  const value = useMemo<SessionRecoveryState>(
    () => ({ isRecovering, lastRecoveredAt, recover, touch }),
    [isRecovering, lastRecoveredAt, recover, touch],
  );

  return <SessionRecoveryContext.Provider value={value}>{children}</SessionRecoveryContext.Provider>;
}
