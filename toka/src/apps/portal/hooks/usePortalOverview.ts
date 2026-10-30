import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useDataChangeRefresh } from "../../../shared/hooks/useDataChangeRefresh";
import { portalService } from "../services/portal.service";
import { portalKeys } from "./queryKeys";
import { usePortalWebSocket } from "./usePortalWebSocket";

export function usePortalOverview(enabled = true) {
  const client = useQueryClient();
  const { onMessage } = usePortalWebSocket();
  const query = useQuery({
    queryKey: portalKeys.overview(),
    queryFn: () => portalService.getOverview(),
    enabled,
    refetchInterval: 30_000,
    refetchOnWindowFocus: "always",
  });
  useDataChangeRefresh(["calendar", "leads"], () => client.invalidateQueries({ queryKey: portalKeys.overview() }), { enabled });
  useEffect(() => {
    if (!enabled) return;
    return onMessage((type) => {
      if (type === "appointment_update") void client.invalidateQueries({ queryKey: portalKeys.overview() });
    });
  }, [client, enabled, onMessage]);
  return query;
}
