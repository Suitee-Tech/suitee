import type { ReactNode } from "react";
import { usePermissionsContext } from "../context/PermissionsContext";

export function RequirePermission({
  permission,
  children,
  loadingFallback,
  accessDeniedFallback,
}: {
  permission: string;
  children: ReactNode;
  loadingFallback?: ReactNode;
  accessDeniedFallback?: ReactNode;
}) {
  const { can, isLoading } = usePermissionsContext();

  if (isLoading) {
    return loadingFallback ? <>{loadingFallback}</> : <div className="p-8 text-center text-muted-foreground">Carregando acessos...</div>;
  }

  if (!can(permission)) {
    return accessDeniedFallback ? <>{accessDeniedFallback}</> : <div className="p-8 text-center text-muted-foreground">Acesso negado.</div>;
  }

  return <>{children}</>;
}
