import { useQuery } from "@tanstack/react-query";
import { useAuth } from "../context/AuthContext";
import { fetchPropertiesList, fetchProperty, propertiesListQueryKey, propertyQueryKey, type PropertyFilter } from "./queries";

export function usePropertyQuery(id: string | undefined) {
  const { user } = useAuth();
  const tenantId = user?.current_company_id;
  return useQuery({
    queryKey: propertyQueryKey(id ?? "", tenantId),
    queryFn: () => fetchProperty(id ?? ""),
    enabled: Boolean(id && tenantId),
  });
}

export function usePropertiesQuery(filter: PropertyFilter = {}) {
  const { user } = useAuth();
  const tenantId = user?.current_company_id;
  return useQuery({
    queryKey: propertiesListQueryKey(filter, tenantId),
    queryFn: () => fetchPropertiesList(filter),
    enabled: Boolean(tenantId),
  });
}
