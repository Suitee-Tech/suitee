import { type ReactNode } from "react";
import { SessionRecoveryProvider } from "../../shared/context/SessionRecoveryContext";
import { PortalAuthProvider } from "./context/PortalAuthContext";
import { PortalDealsProvider } from "./context/PortalDealsContext";
import { usePortalRealtimeSync } from "./hooks/usePortalRealtimeSync";
import {
  getPortalAccessToken,
  hasPortalSessionHint,
  isPortalRefreshActive,
  refreshPortalAccessToken,
  subscribePortalRefresh,
} from "./services/portal-api";

// T-03: o portal agora participa do fluxo de recuperação de sessão do
// SessionRecoveryProvider — mas com o PRÓPRIO ciclo (`/auth/portal/refresh`,
// token em memória, hint próprio). Ao voltar o foco depois de inatividade o
// refresh roda ANTES de as consultas do portal saírem com token vencido;
// durante qualquer renovação (inclusive a do interceptor 401) a UI fica em
// "carregando" em vez de piscar erro.
export function PortalProviders({ children }: { children: ReactNode }) {
  return (
    <SessionRecoveryProvider
      refreshAccessToken={refreshPortalAccessToken}
      hasSessionHint={hasPortalSessionHint}
      getAccessToken={getPortalAccessToken}
      isRefreshActive={isPortalRefreshActive}
      subscribeRefresh={subscribePortalRefresh}
    >
      <PortalAuthProvider>
        <PortalDealsProvider>
          <PortalRealtimeBridge />
          {children}
        </PortalDealsProvider>
      </PortalAuthProvider>
    </SessionRecoveryProvider>
  );
}

function PortalRealtimeBridge() {
  usePortalRealtimeSync();
  return null;
}
