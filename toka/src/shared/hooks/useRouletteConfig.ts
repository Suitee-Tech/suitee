import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { toast } from "sonner";
import {
  leadService,
  type RouletteAuditResponse,
  type RouletteConfig,
  type RouletteHistoryResponse,
  type UpdateRouletteConfigDTO,
} from "../services/lead.service";
import { useWsSubscription } from "../ws/useWsSubscription";
import { useAuth } from "../context/AuthContext";

export const ROULETTE_QUERY_KEY = ["roulette-config"] as const;
export const ROULETTE_HISTORY_QUERY_KEY = (params?: { limit?: number; offset?: number }) =>
  ["roulette-history", params?.limit ?? null, params?.offset ?? null] as const;
export const ROULETTE_AUDIT_QUERY_KEY = (params?: {
  limit?: number;
  offset?: number;
  action?: string;
  target_id?: string;
  from?: string;
  to?: string;
}) =>
  [
    "roulette-audit",
    params?.limit ?? null,
    params?.offset ?? null,
    params?.action ?? null,
    params?.target_id ?? null,
    params?.from ?? null,
    params?.to ?? null,
  ] as const;

export function useRouletteConfig() {
  return useQuery<RouletteConfig>({
    queryKey: ROULETTE_QUERY_KEY,
    queryFn: () => leadService.getRouletteConfig(),
    staleTime: 60_000,
  });
}

export function useUpdateRouletteConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (dto: UpdateRouletteConfigDTO) => leadService.updateRouletteConfig(dto),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ROULETTE_QUERY_KEY });
      qc.invalidateQueries({ queryKey: ["roulette-history"] });
      qc.invalidateQueries({ queryKey: ["roulette-audit"] });
      toast.success("Configuração salva");
    },
    onError: () => {
      toast.error("Erro ao salvar configuração");
    },
  });
}

export function useRouletteHistory(params?: { limit?: number; offset?: number }) {
  return useQuery<RouletteHistoryResponse>({
    queryKey: ROULETTE_HISTORY_QUERY_KEY(params),
    queryFn: () => leadService.getRouletteHistory(params),
  });
}

export function useRouletteAudit(
  params?: { limit?: number; offset?: number; action?: string; target_id?: string; from?: string; to?: string },
  options?: { enabled?: boolean },
) {
  return useQuery<RouletteAuditResponse>({
    queryKey: ROULETTE_AUDIT_QUERY_KEY(params),
    queryFn: () => leadService.getRouletteAudit(params),
    enabled: options?.enabled,
  });
}

export function useRouletteConfigWatcher() {
  const qc = useQueryClient();
  const { user } = useAuth();
  const currentUserId = user?.id;

  const handler = useCallback(
    (event: { payload?: Record<string, unknown> }) => {
      const payload = event?.payload ?? {};
      const actorId = typeof payload.actor_id === "string" ? payload.actor_id : "";
      const changedFields = Array.isArray(payload.changed_fields) ? (payload.changed_fields as string[]) : [];
      qc.invalidateQueries({ queryKey: ROULETTE_QUERY_KEY });
      qc.invalidateQueries({ queryKey: ["roulette-history"] });
      qc.invalidateQueries({ queryKey: ["roulette-audit"] });
      if (actorId && currentUserId && actorId !== currentUserId) {
        const summary = changedFields.length > 0 ? changedFields.slice(0, 3).join(", ") : "configuração";
        toast.info(`Roleta atualizada por outro usuário (${summary}). A página foi recarregada.`);
      }
    },
    [qc, currentUserId],
  );

  useWsSubscription("roulette_config_updated", handler);
}

