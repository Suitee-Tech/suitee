import type { ReactNode } from "react";
import { FinanceProvider } from "../../shared/context/FinanceContext";
import { MaintenanceProvider } from "../../shared/context/MaintenanceContext";
import { DocumentsProvider } from "../../shared/context/DocumentsContext";
import { AuditLogProvider } from "../../shared/context/AuditLogContext";
import { KeysProvider } from "../../shared/context/KeysContext";
import { InspectionsProvider } from "../../shared/context/InspectionsContext";
import { useAuth } from "../../shared/context/AuthContext";
import { settingsService } from "../../shared/services/settings.service";
import { AgentsProvider as SharedAgentsProvider } from "../../shared/context/AgentsContext";
import { CalendarProvider as SharedCalendarProvider } from "../../shared/context/CalendarContext";
import { ContractsProvider as SharedContractsProvider } from "../../shared/context/ContractsContext";
import { DealChecklistsProvider as SharedDealChecklistsProvider } from "../../shared/context/DealChecklistsContext";
import { useAgentsContext } from "../../shared/context/AgentsContext";
import { useDealsContext } from "../../shared/context/DealsContext";
import { useLeadsContext } from "../../shared/context/LeadsContext";
import { usePermissionsContext } from "../../shared/context/PermissionsContext";
import { usePropertiesContext } from "../../shared/context/PropertiesContext";

export function CRMProviders({ children }: { children: ReactNode }) {
  return (
    <AuditLogProvider>
      <CRMProvidersInner>{children}</CRMProvidersInner>
    </AuditLogProvider>
  );
}

function CRMProvidersInner({ children }: { children: ReactNode }) {
  const { isAuthenticated, user } = useAuth();
  const { can, isLoading: permissionsLoading } = usePermissionsContext();

  const companyId = user?.current_company_id || user?.companies?.[0]?.id;

  return (
    <SharedAgentsProvider
      isAuthenticated={isAuthenticated}
      companyId={companyId || ""}
      permissionsLoading={permissionsLoading}
      canReadBrokers={can("brokers:read")}
      listCompanyMembers={settingsService.listCompanyMembers}
      getPermissions={settingsService.getPermissions}
      me={user ? { id: user.id, name: user.fullname || user.email, avatar_url: user.avatar_url, role: user.role } : undefined}
    >
      <SharedContractsProvider
        isAuthenticated={isAuthenticated}
        companyId={companyId}
        permissionsLoading={permissionsLoading}
        canReadContracts={can("contracts:read")}
      >
        <SharedDealChecklistsProviderBridge isAuthenticated={isAuthenticated} companyId={companyId}>
          <FinanceProvider>
            <SharedCalendarProviderBridge isAuthenticated={isAuthenticated} companyId={companyId} currentUserId={user?.id}>
              <MaintenanceProvider>
                <InspectionsProvider>
                  <KeysProvider>
                    <DocumentsProvider>{children}</DocumentsProvider>
                  </KeysProvider>
                </InspectionsProvider>
              </MaintenanceProvider>
            </SharedCalendarProviderBridge>
          </FinanceProvider>
        </SharedDealChecklistsProviderBridge>
      </SharedContractsProvider>
    </SharedAgentsProvider>
  );
}

function SharedDealChecklistsProviderBridge({
  children,
  isAuthenticated,
  companyId,
}: {
  children: ReactNode;
  isAuthenticated: boolean;
  companyId?: string;
}) {
  const { can, isLoading: permissionsLoading } = usePermissionsContext();
  const { dealDTOs, fetchDealById, initChecklist, addChecklistItem, removeChecklistItem, updateChecklistItemStatus, solicitChecklistItem } =
    useDealsContext();

  return (
    <SharedDealChecklistsProvider
      isAuthenticated={isAuthenticated}
      companyId={companyId}
      canReadDeals={can("deals:read")}
      permissionsLoading={permissionsLoading}
      dealDTOs={dealDTOs}
      fetchDealById={fetchDealById}
      initChecklist={initChecklist}
      addChecklistItem={addChecklistItem}
      removeChecklistItem={removeChecklistItem}
      updateChecklistItemStatus={updateChecklistItemStatus}
      solicitChecklistItem={solicitChecklistItem}
    >
      {children}
    </SharedDealChecklistsProvider>
  );
}

function SharedCalendarProviderBridge({
  children,
  isAuthenticated,
  companyId,
  currentUserId,
}: {
  children: ReactNode;
  isAuthenticated: boolean;
  companyId?: string;
  currentUserId?: string;
}) {
  const { isLoading: permissionsLoading, hasTenantWideScope } = usePermissionsContext();
  const { agents } = useAgentsContext();
  const { properties } = usePropertiesContext();
  const { leads, setLeads } = useLeadsContext();

  return (
    <SharedCalendarProvider
      isAuthenticated={isAuthenticated}
      companyId={companyId}
      permissionsLoading={permissionsLoading}
      agents={agents}
      properties={properties}
      leads={leads}
      setLeads={setLeads}
      canViewTeam={hasTenantWideScope}
      currentUserId={currentUserId}
    >
      {children}
    </SharedCalendarProvider>
  );
}
