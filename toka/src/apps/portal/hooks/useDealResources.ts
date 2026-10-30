import { useQuery } from "@tanstack/react-query";
import {
  portalService,
  type PortalAppointment,
  type PortalCharge,
  type PortalFavoriteIDs,
  type PortalMaintenanceTicket,
  type PortalPropertySummary,
  type PortalTicket,
} from "../services/portal.service";
import type { ChecklistItemDTO } from "../../../shared/services/deal.service";
import { usePortalAuth } from "../context/PortalAuthContext";
import { portalKeys } from "./queryKeys";

const enabled = (dealId: string | undefined): dealId is string => Boolean(dealId);

export function usePortalDealProperty(dealId: string | undefined) {
  const { isAuthenticated } = usePortalAuth();
  return useQuery<PortalPropertySummary>({
    queryKey: dealId ? portalKeys.dealProperty(dealId) : ["portal", "deal", "", "property"],
    queryFn: ({ signal }) => portalService.getDealProperty(dealId!, { signal }),
    enabled: isAuthenticated && enabled(dealId),
    refetchOnWindowFocus: "always",
    refetchOnReconnect: "always",
  });
}

export function usePortalDealChecklist(dealId: string | undefined) {
  const { isAuthenticated } = usePortalAuth();
  return useQuery<ChecklistItemDTO[]>({
    queryKey: dealId ? portalKeys.dealChecklist(dealId) : ["portal", "deal", "", "checklist"],
    queryFn: async ({ signal }) => {
      const data = await portalService.getDealChecklist(dealId!, { signal });
      return Array.isArray(data) ? (data as ChecklistItemDTO[]) : [];
    },
    enabled: isAuthenticated && enabled(dealId),
    refetchOnWindowFocus: "always",
    refetchOnReconnect: "always",
    refetchInterval: 30_000,
  });
}

export function usePortalDealCharges(dealId: string | undefined) {
  return useQuery<PortalCharge[]>({
    queryKey: dealId ? portalKeys.dealCharges(dealId) : ["portal", "deal", "", "charges"],
    queryFn: () => portalService.getDealCharges(dealId!),
    enabled: enabled(dealId),
  });
}

export function usePortalDealTickets(dealId: string | undefined) {
  return useQuery<PortalTicket[]>({
    queryKey: dealId ? portalKeys.dealTickets(dealId) : ["portal", "deal", "", "tickets"],
    queryFn: () => portalService.getDealTickets(dealId!),
    enabled: enabled(dealId),
  });
}

export function usePortalDealMaintenance(dealId: string | undefined) {
  return useQuery<PortalMaintenanceTicket[]>({
    queryKey: dealId ? portalKeys.dealMaintenance(dealId) : ["portal", "deal", "", "maintenance"],
    queryFn: () => portalService.getDealMaintenance(dealId!),
    enabled: enabled(dealId),
  });
}

export function usePortalDealAppointments(dealId: string | undefined) {
  const { isAuthenticated } = usePortalAuth();
  return useQuery<PortalAppointment[]>({
    queryKey: dealId ? portalKeys.dealAppointments(dealId) : ["portal", "deal", "", "appointments"],
    queryFn: ({ signal }) => portalService.getDealAppointments(dealId!, { signal }),
    enabled: isAuthenticated && enabled(dealId),
    refetchOnWindowFocus: "always",
    refetchOnReconnect: "always",
    refetchInterval: 30_000,
  });
}

export function usePortalDealFavorites(dealId: string | undefined) {
  return useQuery<PortalFavoriteIDs>({
    queryKey: dealId ? portalKeys.dealFavorites(dealId) : ["portal", "deal", "", "favorites"],
    queryFn: () => portalService.getDealFavorites(dealId!),
    enabled: enabled(dealId),
  });
}


export function usePortalDealRecommendations(dealId: string | undefined) {
  const { isAuthenticated } = usePortalAuth();
  return useQuery<PortalPropertySummary[]>({
    queryKey: portalKeys.dealRecommendations(dealId ?? ""),
    queryFn: async ({ signal }) => {
      const data = await portalService.getDealRecommendations(dealId!, { signal });
      return Array.isArray(data) ? data : [];
    },
    enabled: isAuthenticated && enabled(dealId),
  });
}
