import { Navigate, Outlet, useLocation } from 'react-router';
import { useSessionRecovery } from '../../../shared/context/SessionRecoveryContext';
import { usePortalAuth } from '../context/PortalAuthContext';

export function PortalProtectedRoute() {
  const { isAuthenticated, isLoading } = usePortalAuth();
  const { isRecovering } = useSessionRecovery();
  const location = useLocation();

  // T-03: durante a renovação proativa da sessão (volta do foco depois de
  // inatividade) a rota segura a renderização — a página não monta, então
  // não existe erro para piscar enquanto o refresh termina.
  if (isLoading || isRecovering) {
    return (
      <div className="flex h-screen items-center justify-center bg-muted">
        <div className="flex flex-col items-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary-200 border-t-primary-600" />
          <p className="mt-4 text-sm text-muted-foreground">Carregando...</p>
        </div>
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/portal/login" state={{ from: location }} replace />;
  }

  return <Outlet />;
}
