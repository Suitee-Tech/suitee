import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { dealQueryKey, dealsScopeQueryKey } from "../../../../shared/deals/queries";
import { useWsSubscription } from "../../../../shared/ws/useWsSubscription";
import type { WSEvent } from "../../../../shared/ws/wsManager";
import { useCurrentUser } from "../../../../shared/context/CurrentUserContext";
import { useAuth } from "../../../../shared/context/AuthContext";
import { getActiveCompanyId } from "../../../../shared/utils/tenant";
import { useRequestScopeGuard } from "../../../../shared/hooks/useRequestScopeGuard";
import { getStageMeta } from "../../../../shared/deals/dealStages";
import type { DealStatus } from "../../../../shared/services/deal.service";

/**
 * Translates operator-side websocket events into React Query cache
 * invalidations for the Esteira Digital. Mount once at the top of the
 * Esteira route tree.
 *
 * Backend → broadcast → invalidation map
 * (toca-comm/internal/service/event_consumer.go uses
 *  Hub.BroadcastEventToTenantInternal so role="cliente" doesn't receive
 *  these events — they're scoped to admins/managers/brokers):
 *
 *   deal_created          ← deal.created
 *   deal_status_changed   ← deal.status_changed
 *   deal_contract_signed  ← deal.contract.signed
 *   proposal_update       ← proposal.sent / proposal.responded
 *   checklist_solicited   ← checklist.solicited
 *   signature_update      ← signature.* (tenant-wide, see hub:BroadcastEventToTenant)
 */
export function useEsteiraRealtimeSync() {
  const queryClient = useQueryClient();
  const { userId: currentUserId } = useCurrentUser();
  const { user } = useAuth();
  const tenantId = getActiveCompanyId(user);
  const beginScopedRequest = useRequestScopeGuard(tenantId ?? "");

  const extractDealId = (payload: unknown): string | undefined => {
    if (!payload || typeof payload !== "object") return undefined;
    const id = (payload as { deal_id?: unknown }).deal_id;
    return typeof id === "string" && id !== "" ? id : undefined;
  };

  const invalidateDealScope = useCallback(
    (dealId?: string) => {
      const isCurrent = beginScopedRequest();
      if (!isCurrent()) return;
      if (dealId) {
        queryClient.invalidateQueries({ queryKey: dealQueryKey(dealId, tenantId) });
      }
      queryClient.invalidateQueries({ queryKey: dealsScopeQueryKey(tenantId) });
    },
    [beginScopedRequest, queryClient, tenantId],
  );

  const handleDealCreated = useCallback(
    (_event: WSEvent) => {
      const isCurrent = beginScopedRequest();
      if (!isCurrent()) return;
      queryClient.invalidateQueries({ queryKey: dealsScopeQueryKey(tenantId) });
    },
    [beginScopedRequest, queryClient, tenantId],
  );

  const handleDealEvent = useCallback(
    (event: WSEvent) => {
      invalidateDealScope(extractDealId(event.payload));
    },
    [invalidateDealScope],
  );

  // Status changes are special — invalidate AND inform the operator when
  // a teammate triggered the move (suppressed on the originating browser).
  const handleDealStatusChanged = useCallback(
    (event: WSEvent) => {
      const payload = (event.payload ?? {}) as {
        deal_id?: string;
        actor_user_id?: string;
        new_status?: DealStatus;
      };
      // Skip invalidation when the current user initiated the change.
      // The optimistic update + PATCH response already keeps the cache
      // consistent; invalidating here would race with the backend's
      // eventual-consistency read path and cause a snap-back.
      const isOwnAction = payload.actor_user_id && currentUserId && payload.actor_user_id === currentUserId;
      if (!isOwnAction) {
        invalidateDealScope(payload.deal_id);
      }
      if (!payload.actor_user_id || !currentUserId) return;
      if (isOwnAction) return;
      if (!payload.new_status) return;
      const meta = getStageMeta(payload.new_status);
      if (!meta) return;
      toast.info(`Negócio movido para ${meta.label}`);
    },
    [currentUserId, invalidateDealScope],
  );

  useWsSubscription("deal_created", handleDealCreated);
  useWsSubscription("deal_status_changed", handleDealStatusChanged);
  useWsSubscription("deal_contract_signed", handleDealEvent);
  useWsSubscription("proposal_update", handleDealEvent);
  useWsSubscription("checklist_solicited", handleDealEvent);
  useWsSubscription("signature_update", handleDealEvent);
}
