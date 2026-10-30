import { useQuery } from "@tanstack/react-query";
import { useAuth } from "../context/AuthContext";
import { getActiveCompanyId } from "../utils/tenant";
import { dealQueryKey, dealsListQueryKey, fetchDeal, fetchDealsList } from "./queries";

export function useDealQuery(id: string | undefined) {
  const { user } = useAuth();
  const tenantId = getActiveCompanyId(user);
  return useQuery({
    queryKey: dealQueryKey(id ?? "", tenantId),
    queryFn: () => fetchDeal(id ?? ""),
    enabled: Boolean(id && tenantId),
  });
}

export function useDealsQuery(limit = 100, offset = 0) {
  const { user } = useAuth();
  const tenantId = getActiveCompanyId(user);
  return useQuery({
    queryKey: dealsListQueryKey(limit, offset, tenantId),
    queryFn: () => fetchDealsList(limit, offset),
    enabled: Boolean(tenantId),
  });
}
