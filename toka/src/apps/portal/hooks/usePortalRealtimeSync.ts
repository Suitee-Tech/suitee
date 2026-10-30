import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { portalKeys } from "./queryKeys";
import { usePortalWebSocket } from "./usePortalWebSocket";

export function usePortalRealtimeSync() {
  const { onMessage } = usePortalWebSocket();
  const queryClient = useQueryClient();

  useEffect(() => {
    return onMessage((type, payload) => {
      const dealId: string | undefined = payload?.deal_id;

      const invalidateDealScope = (id?: string) => {
        if (!id) {
          queryClient.invalidateQueries({ queryKey: portalKeys.deals() });
          return;
        }
        queryClient.invalidateQueries({ queryKey: portalKeys.deal(id) });
        queryClient.invalidateQueries({ queryKey: portalKeys.deals() });
      };

      switch (type) {
        case "deal_status_changed":
        case "proposal_update":
        case "deal_contract_signed":
          invalidateDealScope(dealId);
          break;
        case "checklist_solicited":
          if (dealId) {
            queryClient.invalidateQueries({ queryKey: portalKeys.dealChecklist(dealId) });
          }
          invalidateDealScope(dealId);
          if (payload?.item_name) {
            toast.info(`Documento solicitado: ${payload.item_name}`);
          }
          break;
        default:
          break;
      }
    });
  }, [onMessage, queryClient]);
}
