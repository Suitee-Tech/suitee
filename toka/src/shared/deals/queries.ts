import { dealService, type DealAuditLogEntry, type DealDTO } from "../services/deal.service";
import { unwrapApiData } from "../services/api";
import { collectPages } from "../services/collectPages";

export const dealsListQueryKey = (limit = 100, offset = 0, tenantId?: string | null) =>
  ["deals", tenantId ?? "no-tenant", limit, offset] as const;
export const allDealsQueryKey = (tenantId?: string | null) =>
  ["deals", tenantId ?? "no-tenant", "all"] as const;
export const dealsScopeQueryKey = (tenantId?: string | null) =>
  ["deals", tenantId ?? "no-tenant"] as const;
export const dealQueryKey = (id: string, tenantId?: string | null) =>
  ["deal", tenantId ?? "no-tenant", id] as const;
export const dealAuditLogQueryKey = (id: string, tenantId?: string | null) =>
  ["deal", tenantId ?? "no-tenant", id, "audit-log"] as const;

export const fetchDealsList = async (limit = 100, offset = 0): Promise<DealDTO[]> => {
  const res = await dealService.getAll(limit, offset);
  return unwrapApiData<DealDTO[]>(res);
};

export const fetchAllDeals = (signal?: AbortSignal): Promise<DealDTO[]> =>
  collectPages(async ({ limit, offset }) => {
    const response = await dealService.getAll(limit, offset, signal);
    return { data: unwrapApiData<DealDTO[]>(response), meta: response.data.meta };
  }, { signal });

export const fetchDeal = async (id: string): Promise<DealDTO> => {
  const res = await dealService.getById(id);
  return unwrapApiData<DealDTO>(res);
};

export const fetchDealAuditLog = async (id: string, limit = 100): Promise<DealAuditLogEntry[]> => {
  const res = await dealService.getAuditLog(id, limit);
  return unwrapApiData<DealAuditLogEntry[]>(res);
};
