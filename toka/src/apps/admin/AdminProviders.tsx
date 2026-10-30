import { useCallback, type ReactNode } from "react";
import { CRMProviders } from "./CRMProviders";
import { FocusModeProvider } from "../../shared/context/FocusModeContext";
import { AuthProvider, useAuth } from "../../shared/context/AuthContext";
import { SessionRecoveryProvider } from "../../shared/context/SessionRecoveryContext";
import { ThemeProvider } from "../../shared/context/ThemeContext";
import { getCompany } from "../../shared/services/auth.service";
import { settingsService } from "../../shared/services/settings.service";
import type { Property } from "../../shared/data";
import { AuthUserProvider as SharedAuthUserProvider } from "../../shared/context/AuthUserContext";
import { CurrentUserProvider as SharedCurrentUserProvider } from "../../shared/context/CurrentUserContext";
import { DealsProvider as SharedDealsProvider } from "../../shared/context/DealsContext";
import { LeadsProvider as SharedLeadsProvider, useLeadsContext } from "../../shared/context/LeadsContext";
import { PermissionsProvider as SharedPermissionsProvider, usePermissionsContext } from "../../shared/context/PermissionsContext";
import { PropertiesProvider as SharedPropertiesProvider } from "../../shared/context/PropertiesContext";
import { ViewPreferencesProvider as SharedViewPreferencesProvider } from "../../shared/context/ViewPreferencesContext";
import { SubscriptionAccessProvider } from "../../shared/context/SubscriptionAccessContext";

export function AdminProviders({ children }: { children: ReactNode }) {
  return (
    <AuthProvider>
      <ThemeProvider>
        {/* Recuperação de sessão por fora dos domínios: o ProtectedRoute usa o
            `isRecovering` daqui para segurar a página enquanto a sessão é
            renovada (aba parada por muito tempo). */}
        <SessionRecoveryProvider>
          <AdminProvidersInner>{children}</AdminProvidersInner>
        </SessionRecoveryProvider>
      </ThemeProvider>
    </AuthProvider>
  );
}

function AdminProvidersInner({ children }: { children: ReactNode }) {
  const { isAuthenticated, user } = useAuth();
  const companyId = user?.current_company_id || user?.companies?.[0]?.id;

  return (
    <SharedAuthUserProvider user={user}>
      <SharedCurrentUserProvider userId={user?.id}>
        <SharedPermissionsProvider user={user} getMyPermissions={settingsService.getMyPermissions}>
          <SubscriptionAccessProvider active={isAuthenticated} companyId={companyId}>
            <FocusModeProvider>
              <SharedViewPreferencesProvider
                userId={user?.id}
                remoteClient={{
                  getPreferences: settingsService.getPreferences,
                  updatePreferences: settingsService.updatePreferences,
                }}
              >
                <AdminDomainProviders isAuthenticated={isAuthenticated} companyId={companyId}>
                  <CRMProviders>{children}</CRMProviders>
                </AdminDomainProviders>
              </SharedViewPreferencesProvider>
            </FocusModeProvider>
          </SubscriptionAccessProvider>
        </SharedPermissionsProvider>
      </SharedCurrentUserProvider>
    </SharedAuthUserProvider>
  );
}

function AdminDomainProviders({
  children,
  isAuthenticated,
  companyId,
}: {
  children: ReactNode;
  isAuthenticated: boolean;
  companyId?: string;
}) {
  const { user } = useAuth();
  const { can, isLoading: permissionsLoading, hasTenantWideScope } = usePermissionsContext();

  // `tenantWideScope` substitui o antigo `role` (que era só o PERFIL): é o mesmo
  // critério do servidor — perfil, papel funcional e papel base do perfil de
  // acesso. Sem os três eixos, um admin com perfil `member` era tratado como
  // corretor e o contexto ignorava leads de outros corretores (incidente de
  // 2026-09-21).
  const tenantWideScope = !permissionsLoading && hasTenantWideScope;

  const resolveSharedCompanyName = useCallback(async (id: string) => {
    const company = await getCompany(id);
    return company.name || "Empresa parceira";
  }, []);

  const canReadSharedContent = can("shared_content:read");
  const shouldFetchProperties =
    can("properties:read") || can("deals:read") || can("deals:write") || canReadSharedContent;

  return (
    <SharedLeadsProvider
      isAuthenticated={isAuthenticated}
      companyId={companyId}
      userId={user?.id}
      tenantWideScope={tenantWideScope}
      permissionsLoading={permissionsLoading}
      canReadLeads={can("leads:read")}
      canReadSharedContent={canReadSharedContent}
      canConfigureRoulette={can("leads:configure_roulette")}
      resolveSharedCompanyName={resolveSharedCompanyName}
    >
      <PropertiesProviderBridge
        isAuthenticated={isAuthenticated}
        companyId={companyId}
        permissionsLoading={permissionsLoading}
        shouldFetch={shouldFetchProperties}
        canReadSharedContent={canReadSharedContent}
        resolveSharedCompanyName={resolveSharedCompanyName}
      >
        <SharedDealsProvider
          isAuthenticated={isAuthenticated}
          companyId={companyId}
          permissionsLoading={permissionsLoading}
          canReadDeals={can("deals:read")}
        >
          {children}
        </SharedDealsProvider>
      </PropertiesProviderBridge>
    </SharedLeadsProvider>
  );
}

function PropertiesProviderBridge({
  children,
  isAuthenticated,
  companyId,
  permissionsLoading,
  shouldFetch,
  canReadSharedContent,
  resolveSharedCompanyName,
}: {
  children: ReactNode;
  isAuthenticated: boolean;
  companyId?: string;
  permissionsLoading: boolean;
  shouldFetch: boolean;
  canReadSharedContent: boolean;
  resolveSharedCompanyName: (companyId: string) => Promise<string>;
}) {
  const { leads, updateLead } = useLeadsContext();

  const onPropertyCreated = useCallback(
    async (created: Property, draft: Omit<Property, "id">) => {
      const owners = draft.owners || [];
      if (owners.length === 0) return;

      for (const owner of owners) {
        const ownerLead = leads.find((l) => l.id === owner.id);
        if (ownerLead && ownerLead.property !== created.id) {
          await updateLead(owner.id, { property: created.id, leadType: "captacao" });
        }
      }
    },
    [leads, updateLead],
  );

  return (
    <SharedPropertiesProvider
      isAuthenticated={isAuthenticated}
      companyId={companyId}
      permissionsLoading={permissionsLoading}
      shouldFetch={shouldFetch}
      canReadSharedContent={canReadSharedContent}
      resolveSharedCompanyName={resolveSharedCompanyName}
      onPropertyCreated={onPropertyCreated}
    >
      {children}
    </SharedPropertiesProvider>
  );
}
