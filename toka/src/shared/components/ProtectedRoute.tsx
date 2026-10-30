import { useEffect, useRef, useState } from "react";
import { Navigate, Outlet } from "react-router";
import { useAuth } from "../context/AuthContext";
import { useSessionRecovery } from "../context/SessionRecoveryContext";

/**
 * Portão das rotas autenticadas.
 *
 * Além do boot (`auth.isLoading`), segura a renderização enquanto a sessão está
 * sendo RESTAURADA (`isRecovering`): depois de muito tempo parada, a aba volta
 * com o access token vencido e as consultas da página falhariam antes de o
 * refresh responder — o usuário via "não foi possível carregar" por um instante.
 * Sem montar a página, não há erro para piscar.
 */
export const ProtectedRoute = () => {
  const { isAuthenticated, isLoading } = useAuth();
  const { isRecovering } = useSessionRecovery();
  // Segura a renderização apenas quando ainda NÃO há sessão montada (boot,
  // F5 depois de horas, troca de aba restaurando). Com o usuário já dentro do
  // app, uma renovação em segundo plano não pode desmontar a página — isso
  // apagaria estado local (formulário pela metade) e a navegação em curso; as
  // consultas em 401 se repetem sozinhas e mostram carregando no lugar do erro.
  const holdForRecovery = isRecovering && !isAuthenticated;
  const [shouldRedirect, setShouldRedirect] = useState(false);
  const redirectTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => {
    if (!isLoading && !holdForRecovery && !isAuthenticated) {
      // Delay the redirect slightly so that brief intermediate
      // renders during the initial auth transition (e.g. a batched
      // state update race) do not trigger a Navigate that starts
      // downloading the /login chunk only to cancel it immediately
      // afterwards, causing ERR_ABORTED.
      redirectTimer.current = setTimeout(() => {
        setShouldRedirect(true);
      }, 150);
    } else {
      setShouldRedirect(false);
    }

    return () => {
      if (redirectTimer.current) clearTimeout(redirectTimer.current);
    };
  }, [isAuthenticated, isLoading, holdForRecovery]);

  if (isLoading || holdForRecovery || (!isAuthenticated && !shouldRedirect)) {
    return (
      <div className="flex h-screen w-full flex-col items-center justify-center gap-3 bg-muted">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        {holdForRecovery ? (
          <p className="text-sm text-muted-foreground" role="status">
            Restaurando sua sessão...
          </p>
        ) : null}
      </div>
    );
  }

  if (shouldRedirect) {
    return <Navigate to="/login" replace />;
  }

  return <Outlet />;
};
