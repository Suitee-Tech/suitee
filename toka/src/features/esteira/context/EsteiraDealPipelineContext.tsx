import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { dealQueryKey, allDealsQueryKey } from "../../../shared/deals/queries";
import { useDealsContext } from "../../../shared/context/DealsContext";
import { useAuth } from "../../../shared/context/AuthContext";
import { getActiveCompanyId } from "../../../shared/utils/tenant";
import { useRequestScopeGuard } from "../../../shared/hooks/useRequestScopeGuard";
import { blockInvalidationResource, unblockInvalidationResource } from "../../../lib/reactQuery";
import type { DealDTO, DealStatus } from "../../../shared/services/deal.service";
import type { DealStage } from "../../../shared/deals/types";
import { moveDealErrorMessage } from "../utils/moveDealError";

type EsteiraDealPipelineContextValue = {
  moveDeal: (id: string, toStage: DealStage) => Promise<void>;
  deleteDeal: (id: string) => Promise<void>;
  movingDealIds: Set<string>;
};

const EsteiraDealPipelineContext = createContext<EsteiraDealPipelineContextValue | null>(null);

const dealStatusMap: Record<string, DealStatus> = {
  "Em Negociação": "NEGOTIATION",
  Proposta: "PROPOSAL",
  "Análise": "ANALYSIS",
  Contrato: "CONTRACT",
  Concluído: "CONVERTED",
  Perdido: "CANCELLED",
};

export function EsteiraDealPipelineProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const { updateDealStatus: updateDealStatusDTO, deleteDeal: deleteDealDTO } = useDealsContext();
  const { user } = useAuth();
  const tenantId = getActiveCompanyId(user);
  const beginScopedRequest = useRequestScopeGuard(tenantId ?? "");

  const movingDealIdsRef = useRef<Set<string>>(new Set());
  const [movingDealIds, setMovingDealIds] = useState<Set<string>>(new Set());
  const moveGenerationRef = useRef<Map<string, number>>(new Map());
  const moveQueueRef = useRef<Map<string, Promise<DealDTO | null>>>(new Map());

  const moveDeal = useCallback(
    async (id: string, toStage: DealStage) => {
      const isCurrent = beginScopedRequest();
      const status = dealStatusMap[toStage];
      if (!status) return;

      const detailKey = dealQueryKey(id, tenantId);
      const listKey = allDealsQueryKey(tenantId);
      const currentDeal =
        queryClient.getQueryData<DealDTO>(detailKey) ??
        queryClient.getQueryData<DealDTO[]>(listKey)?.find((deal) => deal.id === id);
      if (!currentDeal || currentDeal.status === status) return;

      const generation = (moveGenerationRef.current.get(id) ?? 0) + 1;
      moveGenerationRef.current.set(id, generation);
      const isLatestMove = () => moveGenerationRef.current.get(id) === generation;

      // Block QueryDataSync from invalidating deals queries while this
      // optimistic drag-and-drop is in flight. The invalidation fires a
      // refetch that races with the PATCH response and can return stale data.
      blockInvalidationResource("deals");

      // Track this deal as moving
      movingDealIdsRef.current.add(id);
      setMovingDealIds((prev) => new Set(prev).add(id));

      const prevDeal = queryClient.getQueryData<DealDTO>(detailKey);
      const prevList = queryClient.getQueryData<DealDTO[]>(listKey);
      if (prevDeal) {
        queryClient.setQueryData<DealDTO>(detailKey, { ...prevDeal, status });
      }
      if (prevList) {
        queryClient.setQueryData<DealDTO[]>(
          listKey,
          prevList.map((d) => (d.id === id ? { ...d, status } : d)),
        );
      }

      const previousMutation = moveQueueRef.current.get(id) ?? Promise.resolve(null);
      const mutation = previousMutation
        .catch(() => null)
        .then(() => {
          // A previous queued response may have upserted its older status.
          // Re-assert this move immediately before persisting the latest intent.
          queryClient.setQueryData<DealDTO>(detailKey, (prev) => (prev ? { ...prev, status } : prev));
          queryClient.setQueryData<DealDTO[]>(listKey, (prev) =>
            Array.isArray(prev) ? prev.map((deal) => (deal.id === id ? { ...deal, status } : deal)) : prev,
          );
          return updateDealStatusDTO(id, { status });
        });
      moveQueueRef.current.set(id, mutation);

      try {
        await mutation;
        if (!isCurrent() || !isLatestMove()) return;
        // Cancel any refetch of the deals list triggered by QueryDataSync
        // (60ms debounce) while the PATCH was in flight. The refetch would
        // return stale data and overwrite our cache. Re-set with confirmed data.
        await queryClient.cancelQueries({ queryKey: listKey });
        const confirmed = queryClient.getQueryData<DealDTO>(detailKey);
        if (confirmed) {
          queryClient.setQueryData<DealDTO[]>(listKey, (prev) => {
            if (!Array.isArray(prev)) return prev;
            return prev.map((d) => (d.id === id ? confirmed : d));
          });
        }
        // Success — remove from React state immediately (hides loading indicator)
        setMovingDealIds((prev) => {
          const next = new Set(prev);
          next.delete(id);
          return next;
        });
      } catch (error: any) {
        if (!isCurrent() || !isLatestMove()) return;
        console.error("Erro ao mover negócio:", error);
        if (prevDeal !== undefined) queryClient.setQueryData(detailKey, prevDeal);
        if (prevList !== undefined) queryClient.setQueryData(listKey, prevList);
        const raw =
          error.response?.data?.message ||
          error.response?.data?.error ||
          error.message ||
          "";
        toast.error(moveDealErrorMessage(raw, prevDeal, status));
        setMovingDealIds((prev) => {
          const next = new Set(prev);
          next.delete(id);
          return next;
        });
      } finally {
        if (moveQueueRef.current.get(id) === mutation) {
          moveQueueRef.current.delete(id);
        }
        setTimeout(() => {
          if (isLatestMove()) movingDealIdsRef.current.delete(id);
          unblockInvalidationResource("deals");
        }, 300);
      }
    },
    [beginScopedRequest, queryClient, tenantId, updateDealStatusDTO],
  );

  const deleteDeal = useCallback(
    async (id: string) => {
      const isCurrent = beginScopedRequest();
      try {
        await deleteDealDTO(id);
        if (isCurrent()) toast.success("Negócio removido.");
      } catch (error) {
        if (isCurrent()) {
          console.error("Erro ao remover negócio:", error);
          toast.error("Erro ao remover negócio.");
        }
        // A confirmação só fecha quando a operação conclui; o diálogo usa esta
        // rejeição para permanecer aberto e permitir nova tentativa.
        throw error;
      }
    },
    [beginScopedRequest, deleteDealDTO],
  );

  const value = useMemo<EsteiraDealPipelineContextValue>(() => ({ moveDeal, deleteDeal, movingDealIds }), [deleteDeal, moveDeal, movingDealIds]);
  return <EsteiraDealPipelineContext.Provider value={value}>{children}</EsteiraDealPipelineContext.Provider>;
}

export function useEsteiraDealPipelineContext() {
  const ctx = useContext(EsteiraDealPipelineContext);
  if (!ctx) throw new Error("useEsteiraDealPipelineContext must be used within EsteiraDealPipelineProvider");
  return ctx;
}
