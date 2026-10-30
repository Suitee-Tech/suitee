import { useEffect, useRef, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { Lead, Property } from "../data";
import { useAuth } from "./AuthContext";
import { useDealsContext } from "./DealsContext";
import { useLeadsContext } from "./LeadsContext";
import { usePropertiesContext } from "./PropertiesContext";
import { usePermissionsContext } from "../../lib/permissions/PermissionsContext";
import { notifyApiError } from "../services/errorToast";
import { fetchLead, leadQueryKey } from "../leads/queries";
import { fetchProperty, propertyQueryKey } from "../properties/queries";
import { mapApiLeadToLead } from "../lib/leads/mappers";
import { mapPropertyDtoToProperty } from "../lib/properties/mappers";

export function DealsBootstrapProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const { isAuthenticated, user } = useAuth();
  const { can, isLoading: permissionsLoading } = usePermissionsContext();
  const { dealDTOs } = useDealsContext();
  const { leads, setLeads } = useLeadsContext();
  const { properties, setProperties } = usePropertiesContext();
  const tenantId = user?.current_company_id;

  const lastProcessedDealIdsRef = useRef<string>("");

  useEffect(() => {
    if (!isAuthenticated || !tenantId || permissionsLoading) return;
    if (!can("deals:read")) return;

    const dealIds = Array.isArray(dealDTOs)
      ? dealDTOs
          .map((d) => d.id)
          .slice()
          .sort()
          .join(",")
      : "";
    if (dealIds === lastProcessedDealIdsRef.current) return;
    lastProcessedDealIdsRef.current = dealIds;

    const dealPropertyIds = Array.from(new Set((dealDTOs || []).map((d) => d?.property_id).filter(Boolean))) as string[];
    const dealLeadIds = Array.from(new Set((dealDTOs || []).map((d) => d?.customer_id).filter(Boolean))) as string[];

    const missingPropertyIds = dealPropertyIds.filter((id) => !properties.some((p) => p.id === id));
    const missingLeadIds = dealLeadIds.filter((id) => !leads.some((l) => l.id === id));

    if (missingPropertyIds.length === 0 && missingLeadIds.length === 0) return;

    let isMounted = true;

    const run = async () => {
      if (missingPropertyIds.length > 0) {
        const fetchedProperties: Property[] = [];
        for (const id of missingPropertyIds) {
          try {
            const dto = await queryClient.fetchQuery({
              queryKey: propertyQueryKey(id, tenantId),
              queryFn: () => fetchProperty(id),
            });
            if (dto?.id) fetchedProperties.push(mapPropertyDtoToProperty(dto));
          } catch (e) {
            notifyApiError(e, { context: "Carregar imóvel relacionado", silent: true });
          }
        }
        if (isMounted && fetchedProperties.length > 0) {
          setProperties((prev) => {
            const prevIds = new Set(prev.map((p) => p.id));
            const uniqueToAdd = fetchedProperties.filter((p) => !prevIds.has(p.id));
            return uniqueToAdd.length > 0 ? [...prev, ...uniqueToAdd] : prev;
          });
        }
      }

      if (missingLeadIds.length > 0) {
        const fetchedLeads: Lead[] = [];
        for (const id of missingLeadIds) {
          try {
            const apiLead = await queryClient.fetchQuery({
              queryKey: leadQueryKey(id, tenantId),
              queryFn: () => fetchLead(id),
            });
            if (apiLead?.id) fetchedLeads.push(mapApiLeadToLead(apiLead));
          } catch (e) {
            notifyApiError(e, { context: "Carregar lead relacionado", silent: true });
          }
        }
        if (isMounted && fetchedLeads.length > 0) {
          setLeads((prev) => {
            const prevIds = new Set(prev.map((l) => l.id));
            const uniqueToAdd = fetchedLeads.filter((l) => !prevIds.has(l.id));
            return uniqueToAdd.length > 0 ? [...prev, ...uniqueToAdd] : prev;
          });
        }
      }
    };

    void run();
    return () => {
      isMounted = false;
    };
  }, [
    can,
    dealDTOs,
    isAuthenticated,
    leads,
    permissionsLoading,
    properties,
    queryClient,
    setLeads,
    setProperties,
    tenantId,
  ]);

  return children;
}
