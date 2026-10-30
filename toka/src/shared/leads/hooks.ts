import { useQuery } from "@tanstack/react-query";
import { useAuth } from "../context/AuthContext";
import { fetchLead, fetchLeadsList, leadQueryKey, leadsListQueryKey, type LeadsListParams } from "./queries";

export function useLeadQuery(id: string | undefined) {
  const { user } = useAuth();
  const tenantId = user?.current_company_id;
  return useQuery({
    queryKey: leadQueryKey(id ?? "", tenantId),
    queryFn: () => fetchLead(id ?? ""),
    enabled: Boolean(id && tenantId),
  });
}

export function useLeadsQuery(params: LeadsListParams = {}) {
  const { user } = useAuth();
  const tenantId = user?.current_company_id;
  return useQuery({
    queryKey: leadsListQueryKey(params, tenantId),
    queryFn: () => fetchLeadsList(params),
    enabled: Boolean(tenantId),
  });
}
