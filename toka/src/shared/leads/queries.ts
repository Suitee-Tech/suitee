import { leadService, type ApiLead } from "../services/lead.service";

export type LeadsListParams = {
  limit?: number;
  offset?: number;
  status?: string;
  search?: string;
  role?: string;
  is_client?: boolean;
  scope?: "broker" | "tenant";
};

export type LeadsListResponse = {
  data: ApiLead[];
  total: number;
  page: number;
  limit: number;
};

export const leadsListQueryKey = (params: LeadsListParams = {}, tenantId?: string | null) =>
  [
    "leads",
    tenantId ?? "no-tenant",
    params.limit ?? null,
    params.offset ?? null,
    params.status ?? null,
    params.search ?? null,
    params.role ?? null,
    params.is_client ?? null,
    params.scope ?? null,
  ] as const;

export const leadQueryKey = (id: string, tenantId?: string | null) =>
  ["lead", tenantId ?? "no-tenant", id] as const;

export const fetchLeadsList = async (params: LeadsListParams = {}): Promise<LeadsListResponse> => {
  return leadService.getAll(params);
};

export const fetchLead = async (id: string): Promise<ApiLead> => {
  return leadService.getById(id);
};
