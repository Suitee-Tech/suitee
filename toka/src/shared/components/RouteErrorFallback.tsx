import { useEffect } from "react";
import { useRouteError, isRouteErrorResponse } from "react-router";
import { ErrorFallback } from "../ui/ErrorFallback";
import { isStaleChunkError, reloadForStaleChunk } from "../lib/staleChunk";

export function RouteErrorFallback() {
  const error = useRouteError();

  let errorObj: Error;
  if (isRouteErrorResponse(error)) {
    errorObj = new Error(`${error.status} — ${error.statusText || error.data}`);
  } else if (error instanceof Error) {
    errorObj = error;
  } else {
    errorObj = new Error(String(error));
  }

  // Rota lazy que falhou por chunk obsoleto (deploy novo): recarregar aqui pega
  // o `index.html` atual e os hashes certos. Sem isso a rota fica quebrada até
  // um hard refresh manual — foi o 404 em /assets/*.js que o usuário viu.
  const staleChunk = isStaleChunkError(errorObj);
  useEffect(() => {
    if (staleChunk) reloadForStaleChunk("route error boundary", errorObj);
  }, [staleChunk, errorObj]);

  const handleReset = () => {
    window.location.reload();
  };

  return <ErrorFallback error={errorObj} resetError={handleReset} variant="page" />;
}
