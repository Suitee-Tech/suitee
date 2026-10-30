import { replaceEqualDeep, useQuery } from "@tanstack/react-query";
import { portalService } from "../services/portal.service";
import type { DealDTO } from "../../../shared/services/deal.service";
import { usePortalAuth } from "../context/PortalAuthContext";
import { shareDealRevision, shareDealsRevisions } from "../../../shared/deals/revisions";
import { portalKeys } from "./queryKeys";

export const fetchPortalDeals = async (signal?: AbortSignal): Promise<DealDTO[]> => {
  const data = await portalService.getDeals({ signal });
  return Array.isArray(data) ? (data as DealDTO[]) : [];
};

export const fetchPortalDeal = async (dealId: string, signal?: AbortSignal): Promise<DealDTO> => {
  return (await portalService.getDealById(dealId, { signal })) as DealDTO;
};

export function usePortalDealsQuery() {
  const { isAuthenticated } = usePortalAuth();
  return useQuery({
    queryKey: portalKeys.deals(),
    queryFn: ({ signal }) => fetchPortalDeals(signal),
    structuralSharing: sharePortalDeals,
    enabled: isAuthenticated,
  });
}

export function usePortalDealQuery(dealId: string | undefined) {
  const { isAuthenticated } = usePortalAuth();
  return useQuery({
    queryKey: dealId ? portalKeys.deal(dealId) : ([...portalKeys.all(), "deal", ""] as const),
    queryFn: ({ signal }) => fetchPortalDeal(dealId ?? "", signal),
    structuralSharing: sharePortalDeal,
    refetchInterval: 30_000,
    refetchOnWindowFocus: "always",
    refetchOnReconnect: "always",
    enabled: isAuthenticated && Boolean(dealId),
  });
}


export const sharePortalDeal = (previous: unknown, next: unknown) => replaceEqualDeep(previous, shareDealRevision(previous, next));
export const sharePortalDeals = (previous: unknown, next: unknown) => replaceEqualDeep(previous, shareDealsRevisions(previous, next));
