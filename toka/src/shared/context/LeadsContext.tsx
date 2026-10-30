import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type { Lead, LeadStatus, SharedFrom } from "../data";
import { leadService, type ApiLead, type CreateLeadDTO, type UpdateLeadDTO } from "../services/lead.service";
import { partnershipService, type SharedLead } from "../services/partnership.service";
import { unwrapApiData } from "../services/api";
import { notifyApiError } from "../services/errorToast";
import { useWsSubscription } from "../ws/useWsSubscription";
import { fetchLeadsList, leadQueryKey, leadsListQueryKey } from "../leads/queries";
import { apiLeadStatusToLeadStatus, leadSourceToApiLeadSource, leadStatusToApiLeadStatus, mapApiLeadToLead } from "../lib/leads/mappers";
import { parsePhoneBR } from "../utils/masks";
import { useDataChangeRefresh } from "../hooks/useDataChangeRefresh";
import { useRequestScopeGuard } from "../hooks/useRequestScopeGuard";

type LeadsContextValue = {
  leads: Lead[];
  setLeads: React.Dispatch<React.SetStateAction<Lead[]>>;
  leadsLoading: boolean;
  leadsError: unknown | null;
  fetchLeads: (options?: { silent?: boolean }) => Promise<void>;
  addLead: (lead: Lead) => Promise<void>;
  updateLead: (id: string, updates: Partial<Lead>, apiDto?: UpdateLeadDTO) => Promise<void>;
  updateLeadStatus: (id: string, status: LeadStatus) => Promise<void>;
  inviteClient: (id: string) => Promise<void>;
  isRouletteEnabled: boolean;
  isRouletteMutationPending: boolean;
  setIsRouletteEnabled: (enabled: boolean, options?: { assignUnassignedLeads?: boolean }) => Promise<unknown>;
  activeRouletteAgents: string[];
  setActiveRouletteAgents: React.Dispatch<React.SetStateAction<string[]>>;
  togglingAgents: Set<string>;
  movingLeadIds: Set<string>;
};

const LeadsContext = createContext<LeadsContextValue | null>(null);

const companyNamePromiseCache = new Map<string, Promise<string>>();

async function resolveCompanyName(companyId: string, resolver?: (companyId: string) => Promise<string>): Promise<string> {
  if (!companyId || !resolver) return "Empresa parceira";
  const cached = companyNamePromiseCache.get(companyId);
  if (cached) return cached;

  const promise = resolver(companyId)
    .then((name) => name || "Empresa parceira")
    .catch(() => {
      companyNamePromiseCache.delete(companyId);
      return "Empresa parceira";
    });

  companyNamePromiseCache.set(companyId, promise);
  return promise;
}

async function mapSharedLeadsToLeads(shared: SharedLead[], resolver?: (companyId: string) => Promise<string>): Promise<Lead[]> {
  if (!Array.isArray(shared) || shared.length === 0) return [];

  const uniqueCompanyIds = Array.from(new Set(shared.filter((s) => !s.source_company_name?.trim()).map((s) => s.source_company_id).filter(Boolean)));
  const nameById = new Map<string, string>();
  await Promise.all(
    uniqueCompanyIds.map(async (id) => {
      nameById.set(id, await resolveCompanyName(id, resolver));
    }),
  );

  return shared.map((item) => {
    const companyName =
      item.source_company_name?.trim() || nameById.get(item.source_company_id) || "Empresa parceira";
    const sharedFrom: SharedFrom = {
      partnershipId: item.partnership_id,
      companyId: item.source_company_id,
      companyName,
    };
    return { ...mapApiLeadToLead(item.lead), sharedFrom };
  });
}

function mergeOwnedAndShared<T extends { id: string }>(owned: T[], shared: T[]): T[] {
  if (shared.length === 0) return owned;
  const ownedIds = new Set(owned.map((item) => item.id));
  const extras = shared.filter((item) => !ownedIds.has(item.id));
  return [...owned, ...extras];
}

export function LeadsProvider({
  children,
  isAuthenticated,
  companyId,
  userId,
  tenantWideScope,
  permissionsLoading,
  canReadLeads,
  canReadSharedContent,
  canConfigureRoulette,
  resolveSharedCompanyName,
}: {
  children: ReactNode;
  isAuthenticated: boolean;
  companyId?: string;
  userId?: string;
  /** Mesmo critério do servidor: perfil + papel + papel base do perfil. */
  tenantWideScope?: boolean;
  permissionsLoading: boolean;
  canReadLeads: boolean;
  canReadSharedContent: boolean;
  canConfigureRoulette: boolean;
  resolveSharedCompanyName?: (companyId: string) => Promise<string>;
}) {
  const queryClient = useQueryClient();

  const [leads, setLeads] = useState<Lead[]>([]);
  // Distinguir carga, falha e vazio é contrato da tela: a lista vazia só pode
  // aparecer depois de uma leitura bem-sucedida. Sem isso, um 500/timeout
  // pintava a base inteira como "vazia" e oferecia criar um lead duplicado.
  const [leadsLoading, setLeadsLoading] = useState(true);
  const [leadsError, setLeadsError] = useState<unknown | null>(null);
  const [isRouletteEnabled, setIsRouletteEnabledState] = useState(true);
  const [isRouletteMutationPending, setIsRouletteMutationPending] = useState(false);

  const movingLeadIdsRef = useRef<Set<string>>(new Set());
  const [movingLeadIds, setMovingLeadIds] = useState<Set<string>>(new Set());
  const leadsRef = useRef(leads);
  leadsRef.current = leads;
  const [activeRouletteAgents, setActiveRouletteAgentsState] = useState<string[]>([]);
  const activeRouletteAgentsRef = useRef(activeRouletteAgents);
  activeRouletteAgentsRef.current = activeRouletteAgents;
  const [togglingAgents, setTogglingAgents] = useState<Set<string>>(new Set());
  // Incremented before each mutation so that in-flight fetchRouletteConfig
  // calls can detect that a newer optimistic update was applied and skip
  // overwriting it with potentially stale server data.
  const rouletteVersionRef = useRef(0);
  // Tracks ANY inflight roulette mutation (setIsRouletteEnabled, setActiveRouletteAgents)
  // so fetchRouletteConfig (ws reconciliation) can wait for it before
  // overwriting the optimistic UI update with stale server data.
  const rouletteMutationRef = useRef<Promise<unknown> | null>(null);
  // After a successful mutation, block fetchRouletteConfig from applying
  // stale server state for a short cooldown. This handles replication lag
  // where the write hits the primary but a subsequent read hits a replica
  // that hasn't been updated yet.
  const rouletteCooldownUntilRef = useRef<number>(0);
  // Tracks ANY in-flight lead mutation so WS reconciliation (reconcileLeadData)
  // waits for it before overwriting optimistic state with stale server data.
  const leadMutationRef = useRef<Promise<unknown> | null>(null);
  const beginScopedRequest = useRequestScopeGuard(`${isAuthenticated}:${companyId ?? ""}`);
  const leadMutationGenerationRef = useRef<Map<string, number>>(new Map());
  const leadStatusMutationQueueRef = useRef<Map<string, Promise<void>>>(new Map());
  const rouletteEnabledRef = useRef(isRouletteEnabled);

  // Visão de tenant vem do critério compartilhado (AdminProviders), que espelha
  // `pkg/auth.HasTenantWideScope`. Antes era `role.includes("admin")` sobre o
  // PERFIL — `member` para um membro comum, mesmo com papel `admin`.
  const isAdmin = Boolean(tenantWideScope);

  const pendingLeadRefreshes = useRef<Map<string, number[]>>(new Map());

  useEffect(() => {
    rouletteEnabledRef.current = isRouletteEnabled;
  }, [isRouletteEnabled]);

  const beginLeadMutation = useCallback((leadId: string) => {
    const generation = (leadMutationGenerationRef.current.get(leadId) ?? 0) + 1;
    leadMutationGenerationRef.current.set(leadId, generation);
    return generation;
  }, []);

  const isLatestLeadMutation = useCallback(
    (leadId: string, generation: number) => leadMutationGenerationRef.current.get(leadId) === generation,
    [],
  );

  const clearPendingRefresh = useCallback((leadId: string) => {
    const timers = pendingLeadRefreshes.current.get(leadId);
    if (timers && timers.length) {
      timers.forEach((t) => window.clearTimeout(t));
    }
    pendingLeadRefreshes.current.delete(leadId);
  }, []);

  const fetchLeads = useCallback(async (options?: { silent?: boolean }) => {
    const isCurrent = beginScopedRequest();
    const silent = options?.silent === true;
    if (!silent) {
      setLeadsLoading(true);
      setLeadsError(null);
    }

    try {
      // Snapshot the moving IDs BEFORE the network request so the 300ms
      // delayed clear in updateLeadStatus doesn't race with fetchQuery.
      const movingIds = new Set(movingLeadIdsRef.current);

      const res = await queryClient.fetchQuery({
        queryKey: leadsListQueryKey({ limit: 1000, offset: 0 }, companyId),
        queryFn: () => fetchLeadsList({ limit: 1000, offset: 0 }),
        staleTime: 0,
      });
      const mapped = Array.isArray(res.data) ? res.data.map(mapApiLeadToLead) : [];

      // Preserve optimistic state for leads currently being moved via drag-and-drop
      if (movingIds.size > 0) {
        const optimisticLeads = leadsRef.current;
        for (let i = 0; i < mapped.length; i++) {
          if (movingIds.has(mapped[i].id)) {
            const optimistic = optimisticLeads.find((l) => l.id === mapped[i].id);
            if (optimistic) mapped[i] = optimistic;
          }
        }
      }

      if (canReadSharedContent) {
        try {
          const shared = await partnershipService.getAllSharedLeads();
          const sharedMapped = await mapSharedLeadsToLeads(shared, resolveSharedCompanyName);
          if (!isCurrent()) return;
          setLeads(mergeOwnedAndShared(mapped, sharedMapped));
          setLeadsError(null);
          if (!silent) setLeadsLoading(false);
          return;
        } catch (e) {
          if (!isCurrent()) return;
          notifyApiError(e, { context: "Carregar leads compartilhados", silent: true });
        }
      }
      if (!isCurrent()) return;
      setLeads(mapped);
      setLeadsError(null);
      if (!silent) setLeadsLoading(false);
    } catch (error) {
      if (isCurrent()) {
        setLeadsError(error);
        if (!silent) setLeadsLoading(false);
      }
      throw error;
    }
  }, [beginScopedRequest, canReadSharedContent, companyId, queryClient, resolveSharedCompanyName]);

  const upsertLeadFromApi = useCallback(
    (apiLead: ApiLead) => {
      const mapped = mapApiLeadToLead(apiLead);
      queryClient.setQueryData(leadQueryKey(apiLead.id, companyId), (prev: any) => {
        const current = prev && typeof prev === "object" ? prev : null;
        if (!current) return apiLead;
        const nextAssigned = (apiLead as any)?.assigned_broker_id;
        const prevAssigned = (current as any)?.assigned_broker_id;
        if ((nextAssigned === undefined || nextAssigned === null || String(nextAssigned) === "") && prevAssigned) {
          return { ...apiLead, assigned_broker_id: prevAssigned };
        }
        return apiLead;
      });
      queryClient.setQueryData(leadsListQueryKey({ limit: 1000, offset: 0 }, companyId), (prev: any) => {
        const current = prev && typeof prev === "object" ? prev : null;
        const data: ApiLead[] = Array.isArray(current?.data) ? current.data : [];
        const exists = data.some((l) => l.id === apiLead.id);
        const nextData = exists
          ? data.map((l) => {
            if (l.id !== apiLead.id) return l;
            const nextAssigned = (apiLead as any)?.assigned_broker_id;
            const prevAssigned = (l as any)?.assigned_broker_id;
            if ((nextAssigned === undefined || nextAssigned === null || String(nextAssigned) === "") && prevAssigned) {
              return { ...apiLead, assigned_broker_id: prevAssigned };
            }
            return apiLead;
          })
          : [apiLead, ...data];
        if (!current) {
          return { data: nextData, total: nextData.length, page: 1, limit: 1000 };
        }
        const total = typeof current.total === "number" ? (exists ? current.total : current.total + 1) : nextData.length;
        return { ...current, data: nextData, total };
      });

      setLeads((prev) => {
        const exists = prev.some((l) => l.id === mapped.id);
        if (!exists) return [mapped, ...prev];
        return prev.map((l) => {
          if (l.id !== mapped.id) return l;
          const next = { ...l, ...mapped };
          if ((!mapped.agentId || String(mapped.agentId).trim() === "") && l.agentId) {
            next.agentId = l.agentId;
          }
          return next;
        });
      });
    },
    [companyId, queryClient],
  );

  const fetchRouletteConfig = useCallback(async () => {
    const isCurrent = beginScopedRequest();

    // Wait for any in-flight roulette mutation to settle so we don't fetch
    // the server while it's still processing our own update.
    const pendingMutation = rouletteMutationRef.current;
    if (pendingMutation) {
      try {
        await pendingMutation;
      } catch {
        // Mutation failed — fall through and load the real server
        // state (which is the pre-mutation value, correct for rollback).
      }
    }

    // Capture the generation BEFORE fetching. If a new mutation starts
    // while we're waiting for the server response, the version will be
    // higher than when we started and we must skip applying the result.
    const fetchVersion = rouletteVersionRef.current;

    const config = await leadService.getRouletteConfig();
    if (!config || !isCurrent()) return;

    // Skip if a newer mutation started after we began fetching. Its
    // optimistic update is authoritatively ahead of whatever the server
    // returned for this generation.
    if (rouletteVersionRef.current !== fetchVersion) {
      return;
    }

    // Skip if we're within the post-mutation cooldown window. A recent
    // mutation already applied the correct API response — fetching now
    // would likely return stale data from a lagging read replica.
    const now = Date.now();
    if (now < rouletteCooldownUntilRef.current) {
      return;
    }

    setIsRouletteEnabledState(config.enabled);
    setActiveRouletteAgentsState(config.active_brokers || []);
  }, [beginScopedRequest]);

  const reconcileLeadData = useCallback(async () => {
    // Wait for any in-flight lead mutation to settle before refetching.
    const pendingMutation = leadMutationRef.current;
    if (pendingMutation) {
      try { await pendingMutation; } catch {}
    }

    // Skip full refetch while any lead is being moved via drag-and-drop.
    // upsertLeadFromApi already updated state + cache with the server response.
    if (movingLeadIdsRef.current.size > 0) {
      // Still refresh roulette (it doesn't affect card positions)
      if (canConfigureRoulette) fetchRouletteConfig().catch(() => {});
      return;
    }
    const requests: Promise<void>[] = [];
    if (canReadLeads || canReadSharedContent) requests.push(fetchLeads({ silent: true }));
    if (canConfigureRoulette) requests.push(fetchRouletteConfig());
    await Promise.all(requests);
  }, [canConfigureRoulette, canReadLeads, canReadSharedContent, fetchLeads, fetchRouletteConfig]);

  useDataChangeRefresh(["leads", "roulette", "partnerships"], reconcileLeadData, {
    enabled: isAuthenticated && !permissionsLoading && (canReadLeads || canReadSharedContent || canConfigureRoulette),
  });

  useEffect(() => {
    return () => {
      for (const [leadId] of pendingLeadRefreshes.current.entries()) {
        clearPendingRefresh(leadId);
      }
    };
  }, [clearPendingRefresh]);

  const previousCompanyId = useRef<string | undefined>(companyId);
  useEffect(() => {
    const prev = previousCompanyId.current;
    if (prev && companyId && prev !== companyId) {
      setLeads([]);
      setIsRouletteEnabledState(true);
      setIsRouletteMutationPending(false);
      setActiveRouletteAgentsState([]);
      setTogglingAgents(new Set());
      leadMutationGenerationRef.current.clear();
      rouletteMutationRef.current = null;
      rouletteCooldownUntilRef.current = 0;
      for (const [leadId] of pendingLeadRefreshes.current.entries()) {
        clearPendingRefresh(leadId);
      }
      queryClient.removeQueries({ queryKey: ["leads"] });
      queryClient.removeQueries({ queryKey: ["lead"] });
    }
    previousCompanyId.current = companyId;
  }, [clearPendingRefresh, companyId, queryClient]);

  useEffect(() => {
    if (isAuthenticated) return;
    setLeads([]);
    setIsRouletteEnabledState(true);
    setIsRouletteMutationPending(false);
    setActiveRouletteAgentsState([]);
    setTogglingAgents(new Set());
    leadMutationGenerationRef.current.clear();
    rouletteMutationRef.current = null;
    rouletteCooldownUntilRef.current = 0;
    for (const [leadId] of pendingLeadRefreshes.current.entries()) {
      clearPendingRefresh(leadId);
    }
    queryClient.removeQueries({ queryKey: ["leads"] });
    queryClient.removeQueries({ queryKey: ["lead"] });
  }, [clearPendingRefresh, isAuthenticated, queryClient]);

  useEffect(() => {
    if (!isAuthenticated || permissionsLoading) return;
    if (canReadLeads || canReadSharedContent) {
      fetchLeads().catch((e) => notifyApiError(e, { context: "Carregar leads" }));
      return;
    }
    // Sem permissão de leitura a tela não deve ficar em "carregando" para sempre.
    setLeadsLoading(false);
  }, [canReadLeads, canReadSharedContent, fetchLeads, isAuthenticated, permissionsLoading, companyId]);

  useEffect(() => {
    if (!isAuthenticated || permissionsLoading) return;
    if (canConfigureRoulette) {
      fetchRouletteConfig().catch((e) => notifyApiError(e, { context: "Carregar roleta", silent: true }));
    }
  }, [canConfigureRoulette, fetchRouletteConfig, isAuthenticated, permissionsLoading, companyId]);

  const handleLeadAssigned = useCallback(
    (event: unknown) => {
      const isCurrent = beginScopedRequest();
      if (!isCurrent()) return;
      if (!isAuthenticated) return;
      if (!event || typeof event !== "object") return;
      const t = (event as { type?: unknown }).type;
      if (t !== "lead_assigned") return;
      const payload = (event as { payload?: unknown }).payload;
      if (!payload || typeof payload !== "object") return;
      const leadId = (payload as { lead_id?: unknown }).lead_id;
      const brokerId = (payload as { broker_id?: unknown }).broker_id;
      const stage = (payload as { stage?: unknown; status?: unknown }).stage ?? (payload as { stage?: unknown; status?: unknown }).status;
      if (typeof leadId !== "string" || typeof brokerId !== "string") return;

      clearPendingRefresh(leadId);

      const nextStatus =
        typeof stage === "string" && apiLeadStatusToLeadStatus[stage] ? apiLeadStatusToLeadStatus[stage] : undefined;

      setLeads((prev) => {
        const exists = prev.some((l) => l.id === leadId);
        if (!exists) return prev;
        return prev.map((l) => (l.id === leadId ? { ...l, agentId: brokerId, status: nextStatus ?? l.status } : l));
      });

      queryClient.setQueryData(leadQueryKey(leadId, companyId), (prev: any) => {
        const current = prev && typeof prev === "object" ? prev : null;
        if (!current) return prev;
        return { ...current, assigned_broker_id: brokerId, status: typeof stage === "string" ? stage : current.status };
      });

      if (!isAdmin && brokerId !== userId) return;
      const hasLocally = leads.some((l) => l.id === leadId);
      if (!hasLocally) {
        leadService
          .getById(leadId)
          .then((apiLead) => {
            if (isCurrent()) upsertLeadFromApi(apiLead);
          })
          .catch(() => { });
      }
    },
    [beginScopedRequest, clearPendingRefresh, companyId, isAdmin, isAuthenticated, leads, queryClient, upsertLeadFromApi, userId],
  );

  useWsSubscription("lead_assigned", handleLeadAssigned);

  const handleLeadCreated = useCallback(
    (event: unknown) => {
      const isCurrent = beginScopedRequest();
      if (!isCurrent()) return;
      if (!isAuthenticated) return;
      if (!canReadLeads) return;
      if (!event || typeof event !== "object") return;
      const t = (event as { type?: unknown }).type;
      if (t !== "lead_created") return;
      const payload = (event as { payload?: unknown }).payload;
      if (!payload || typeof payload !== "object") return;

      const leadId = (payload as { lead_id?: unknown }).lead_id;
      const assignedBrokerID = (payload as { assigned_broker_id?: unknown }).assigned_broker_id;
      if (typeof leadId !== "string") return;

      if (!isAdmin && typeof assignedBrokerID === "string" && assignedBrokerID !== userId) return;
      if (leads.some((l) => l.id === leadId)) return;

      clearPendingRefresh(leadId);
      const timers: number[] = [];
      const schedule = (attempt: number) => {
        if (!isCurrent()) {
          clearPendingRefresh(leadId);
          return;
        }
        const delay = Math.min(10_000, 700 * 2 ** attempt);
        const t = window.setTimeout(() => {
          if (!isCurrent()) {
            clearPendingRefresh(leadId);
            return;
          }
          leadService
            .getById(leadId)
            .then((apiLead) => {
              if (!isCurrent()) {
                clearPendingRefresh(leadId);
                return;
              }
              upsertLeadFromApi(apiLead);
              const assigned = (apiLead as any)?.assigned_broker_id;
              if (assigned) {
                clearPendingRefresh(leadId);
                return;
              }
              if (attempt < 5) schedule(attempt + 1);
            })
            .catch(() => {
              if (!isCurrent()) {
                clearPendingRefresh(leadId);
              } else if (attempt < 5) {
                schedule(attempt + 1);
              }
            });
        }, delay);
        timers.push(t);
        pendingLeadRefreshes.current.set(leadId, timers);
      };

      schedule(0);
    },
    [beginScopedRequest, canReadLeads, clearPendingRefresh, isAdmin, isAuthenticated, leads, upsertLeadFromApi, userId],
  );

  useWsSubscription("lead_created", handleLeadCreated);

  const handleLeadStatusChanged = useCallback(
    (event: unknown) => {
      const isCurrent = beginScopedRequest();
      if (!isCurrent()) return;
      if (!isAuthenticated) return;
      if (!event || typeof event !== "object") return;
      const t = (event as { type?: unknown }).type;
      if (t !== "lead_status_changed") return;
      const payload = (event as { payload?: unknown }).payload;
      if (!payload || typeof payload !== "object") return;
      const leadId = (payload as { lead_id?: unknown }).lead_id;
      const newStatus = (payload as { new_status?: unknown }).new_status;
      if (typeof leadId !== "string" || typeof newStatus !== "string") return;
      const mapped = apiLeadStatusToLeadStatus[newStatus];
      if (!mapped) return;

      setLeads((prev) => prev.map((l) => (l.id === leadId ? { ...l, status: mapped } : l)));
      queryClient.setQueryData(leadQueryKey(leadId, companyId), (prev: any) => {
        const current = prev && typeof prev === "object" ? prev : null;
        if (!current) return prev;
        return { ...current, status: newStatus };
      });
    },
    [beginScopedRequest, companyId, isAuthenticated, queryClient],
  );

  useWsSubscription("lead_status_changed", handleLeadStatusChanged);

  const setIsRouletteEnabled = useCallback(
    (enabled: boolean, options?: { assignUnassignedLeads?: boolean }) => {
      // Serialize: if a roulette mutation is already in-flight, return the
      // existing promise so callers can await the full cycle.
      if (rouletteMutationRef.current) return rouletteMutationRef.current;

      const isCurrent = beginScopedRequest();

      // Placeholder: block WS reconciliation BEFORE any state changes.
      let releasePlaceholder: () => void = () => {};
      rouletteMutationRef.current = new Promise<void>((r) => { releasePlaceholder = r; });
      ++rouletteVersionRef.current;

      const previousEnabled = rouletteEnabledRef.current;
      setIsRouletteEnabledState(enabled);
      rouletteEnabledRef.current = enabled;
      setIsRouletteMutationPending(true);

      const mutationPromise = (async () => {
        try {
          const config = await leadService.updateRouletteConfig({
            enabled,
            assign_unassigned_leads: enabled ? options?.assignUnassignedLeads : undefined,
          });
          if (!isCurrent()) return;
          setIsRouletteEnabledState(config.enabled);
          rouletteEnabledRef.current = config.enabled;
          setActiveRouletteAgentsState(config.active_brokers || []);
          // Block fetchRouletteConfig for a short window after this mutation.
          rouletteCooldownUntilRef.current = Date.now() + 2000;
        } catch (err) {
          if (!isCurrent()) return;
          ++rouletteVersionRef.current;
          notifyApiError(err, { context: "Atualizar roleta" });
          // Fetch authoritative state from server for rollback.
          try {
            const config = await leadService.getRouletteConfig();
            if (config && isCurrent()) {
              setIsRouletteEnabledState(config.enabled);
              rouletteEnabledRef.current = config.enabled;
              setActiveRouletteAgentsState(config.active_brokers || []);
            }
          } catch {
            // Server fetch also failed — rollback to pre-mutation state.
            setIsRouletteEnabledState(previousEnabled);
            rouletteEnabledRef.current = previousEnabled;
          }
          throw err;
        }
      })();

      // Replace placeholder with real mutation promise.
      rouletteMutationRef.current = mutationPromise;
      releasePlaceholder();

      mutationPromise.finally(() => {
        if (rouletteMutationRef.current === mutationPromise) {
          rouletteMutationRef.current = null;
          if (isCurrent()) setIsRouletteMutationPending(false);
        }
      });

      return mutationPromise;
    },
    [beginScopedRequest],
  );

  const setActiveRouletteAgents: React.Dispatch<React.SetStateAction<string[]>> = useCallback(
    (action) => {
      // Serialize: skip if any roulette mutation is already in-flight.
      if (rouletteMutationRef.current) {
        return;
      }

      const isCurrent = beginScopedRequest();

      // Placeholder: block WS reconciliation BEFORE any state changes.
      // This prevents a race where fetchRouletteConfig fires between
      // the optimistic update and setting rouletteMutationRef.current.
      let releasePlaceholder: () => void = () => {};
      rouletteMutationRef.current = new Promise<void>((r) => { releasePlaceholder = r; });
      ++rouletteVersionRef.current;

      // 1. Compute everything synchronously from the ref.
      //    React 18 batches setState updaters — they run asynchronously.
      //    Reading computed values from inside the updater is unreliable.
      const prevFromRef = activeRouletteAgentsRef.current;
      const nextFromRef = typeof action === "function" ? action(prevFromRef) : (action as string[]);
      const changedAgentIds: string[] = [];
      for (const id of nextFromRef) {
        if (!prevFromRef.includes(id)) changedAgentIds.push(id);
      }
      for (const id of prevFromRef) {
        if (!nextFromRef.includes(id)) changedAgentIds.push(id);
      }

      // 2. Optimistic state update — use the synchronously computed value
      //    directly instead of an updater function. The ref always holds
      //    the latest rendered state at the start of each event handler,
      //    so nextFromRef is authoritative. Avoiding the updater eliminates
      //    any React 18 batching uncertainty.
      setActiveRouletteAgentsState(nextFromRef);

      // 3. Toggling indicators computed synchronously.
      if (changedAgentIds.length > 0) {
        setTogglingAgents((old) => {
          const merged = new Set(old);
          for (const id of changedAgentIds) merged.add(id);
          return merged;
        });
      }

      const next = nextFromRef;
      const touchedIds = changedAgentIds;

      // 4. Persist to the backend. updateRouletteConfig returns the full
      //    RouletteConfig — we apply it directly (no separate GET needed).
      const mutationPromise = (async () => {
        try {
          const config = await leadService.updateRouletteConfig({ active_brokers: next });
          if (!isCurrent()) return;
          setIsRouletteEnabledState(config.enabled);
          setActiveRouletteAgentsState(config.active_brokers || []);
          // Block fetchRouletteConfig for a short window after this mutation.
          // The API response is authoritative — any immediate server read would
          // likely return stale data from a lagging read replica.
          rouletteCooldownUntilRef.current = Date.now() + 2000;
        } catch (err) {
          if (!isCurrent()) return;
          ++rouletteVersionRef.current;
          notifyApiError(err, { context: "Atualizar corretores da roleta" });
          // Fetch authoritative state from server for rollback.
          try {
            const config = await leadService.getRouletteConfig();
            if (config && isCurrent()) {
              setIsRouletteEnabledState(config.enabled);
              setActiveRouletteAgentsState(config.active_brokers || []);
            }
          } catch {
            // Server fetch also failed — rollback to pre-mutation state.
            setActiveRouletteAgentsState(prevFromRef);
          }
        } finally {
          setTogglingAgents((old) => {
            const nextSet = new Set(old);
            for (const id of touchedIds) nextSet.delete(id);
            return nextSet.size === old.size ? old : nextSet;
          });
        }
      })();

      // Replace placeholder with real mutation promise.
      rouletteMutationRef.current = mutationPromise;
      releasePlaceholder();
      mutationPromise.finally(() => {
        if (rouletteMutationRef.current === mutationPromise) {
          rouletteMutationRef.current = null;
        }
      });
    },
    [beginScopedRequest],
  );

  const addLead = useCallback(
    async (lead: Lead) => {
      const isCurrent = beginScopedRequest();
      const leadTypeToRole: Record<string, "owner" | "buyer" | "tenant" | undefined> = {
        captacao: "owner",
        compra_venda: "buyer",
        locacao: "tenant",
      };

      const dto: CreateLeadDTO = {
        name: lead.name,
        email: lead.email,
        phone: parsePhoneBR(lead.phone),
        avatar: lead.avatar,
        status: leadStatusToApiLeadStatus[lead.status] as any,
        source: lead.source ? (leadSourceToApiLeadSource[lead.source] as any) : ("MANUAL" as any),
        assigned_broker_id: lead.agentId || undefined,
        role: lead.leadType ? leadTypeToRole[lead.leadType] : undefined,
        qualification: {},
        property_interest: {
          type: "OTHER",
          preferred_location: lead.property || "Interesse Geral",
          price_range: { min: 0, max: lead.budget },
        },
      };

      const newApiLead = await leadService.create(dto);
      if (!isCurrent()) return;
      const leadData = unwrapApiData<ApiLead>(newApiLead);
      const newLead = mapApiLeadToLead(leadData);
      // Use upsertLeadFromApi (dedup-by-id) instead of an unconditional prepend.
      // When the roleta auto-assigns the lead, the lead_assigned WS event can
      // upsert it into state during the `await leadService.create()` above, so a
      // raw [newLead, ...prev] would insert a second copy — the board would then
      // render two cards for the same id (conflicting framer-motion layoutId).
      // upsertLeadFromApi also preserves the broker the roleta just assigned.
      upsertLeadFromApi(leadData);
      toast.success(`Lead "${newLead.name}" criado com sucesso!`);
    },
    [beginScopedRequest, upsertLeadFromApi],
  );

  const updateLead = useCallback(
    async (id: string, updates: Partial<Lead>, apiDto?: UpdateLeadDTO) => {
      const isCurrent = beginScopedRequest();
      const generation = beginLeadMutation(id);
      const previousLead = leads.find((lead) => lead.id === id);

      // Block WS reconciliation before optimistic update
      let releasePlaceholder: () => void = () => {};
      leadMutationRef.current = new Promise<void>((r) => { releasePlaceholder = r; });

      setLeads((prev) => prev.map((lead) => (lead.id === id ? { ...lead, ...updates } : lead)));

      const mutationPromise = (async () => {
        try {
          let response: unknown;
          if (apiDto) {
            const normalized = apiDto.phone ? { ...apiDto, phone: parsePhoneBR(apiDto.phone) } : apiDto;
            response = await leadService.update(id, normalized);
          } else {
            const dto: UpdateLeadDTO = {};
            if (updates.name) dto.name = updates.name;
            if (updates.email) dto.email = updates.email;
            if (updates.phone) dto.phone = parsePhoneBR(updates.phone);
            if (updates.source) dto.source = (leadSourceToApiLeadSource[updates.source] || "MANUAL") as any;
            if (updates.status) dto.status = (leadStatusToApiLeadStatus[updates.status] || "NEW") as any;
            if (updates.agentId !== undefined) dto.assigned_broker_id = updates.agentId;

            if (updates.leadType) {
              const leadTypeToRole: Record<string, "owner" | "buyer" | "tenant"> = {
                captacao: "owner",
                compra_venda: "buyer",
                locacao: "tenant",
              };
              dto.role = leadTypeToRole[updates.leadType];
            }

            if (updates.budget !== undefined || updates.property !== undefined) {
              const currentLead = leads.find((l) => l.id === id);
              const isCaptacao = updates.leadType === "captacao" || currentLead?.leadType === "captacao";

              if (isCaptacao) {
                dto.captacao_property_id = updates.property || currentLead?.property;
                dto.role = "owner";
              } else {
                dto.property_interest = {
                  type: "OTHER",
                  preferred_location: updates.property || currentLead?.property,
                  price_range: { min: 0, max: updates.budget || currentLead?.budget || 0 },
                };
              }
            }

            response = await leadService.update(id, dto);
          }

          if (!isCurrent() || !isLatestLeadMutation(id, generation)) return;
          const updatedLead = unwrapApiData<ApiLead>(response);
          if (!updatedLead?.id) throw new Error("A API não retornou o lead atualizado.");
          upsertLeadFromApi(updatedLead);
        } catch (err) {
          if (!isCurrent()) return;
          if (isLatestLeadMutation(id, generation)) {
            setLeads((prev) =>
              prev.map((lead) => {
                if (lead.id !== id || !previousLead) return lead;
                const stillShowsOptimisticUpdate = Object.entries(updates).every(
                  ([key, value]) => lead[key as keyof Lead] === value,
                );
                return stillShowsOptimisticUpdate ? previousLead : lead;
              }),
            );
            notifyApiError(err, { context: "Atualizar lead" });
          }
          throw err;
        }
      })();

      leadMutationRef.current = mutationPromise;
      releasePlaceholder();
      mutationPromise.finally(() => {
        if (leadMutationRef.current === mutationPromise) {
          leadMutationRef.current = null;
        }
      });

      return mutationPromise;
    },
    [beginLeadMutation, beginScopedRequest, isLatestLeadMutation, leads, upsertLeadFromApi],
  );

  const updateLeadStatus = useCallback(
    async (id: string, status: LeadStatus) => {
      const currentLead = leadsRef.current.find((lead) => lead.id === id);
      if (!currentLead || currentLead.status === status) return;

      const isCurrent = beginScopedRequest();
      const generation = beginLeadMutation(id);
      const previousLead = currentLead;

      // Track this lead as moving — used by fetchLeads to preserve optimistic state
      movingLeadIdsRef.current.add(id);
      setMovingLeadIds((prev) => new Set(prev).add(id));

      // Block WS reconciliation before optimistic update
      let releasePlaceholder: () => void = () => {};
      leadMutationRef.current = new Promise<void>((r) => { releasePlaceholder = r; });

      setLeads((prev) => prev.map((lead) => (lead.id === id ? { ...lead, status } : lead)));
      const previousMutation = leadStatusMutationQueueRef.current.get(id) ?? Promise.resolve();
      const mutation = previousMutation.catch(() => undefined).then(async () => {
        const response = await leadService.updateStatus(id, (leadStatusToApiLeadStatus[status] || "NEW") as any);
        if (!isCurrent() || !isLatestLeadMutation(id, generation)) return;
        const updatedLead = unwrapApiData<ApiLead>(response);
        if (!updatedLead?.id) throw new Error("A API não retornou o lead atualizado.");
        upsertLeadFromApi(updatedLead);
      });
      leadStatusMutationQueueRef.current.set(id, mutation);

      leadMutationRef.current = mutation;
      releasePlaceholder();

      try {
        await mutation;
        if (!isLatestLeadMutation(id, generation)) return;
        // Success — remove from React state immediately (hides loading indicator)
        setMovingLeadIds((prev) => {
          const next = new Set(prev);
          next.delete(id);
          return next;
        });
        // Delay clearing the ref so the debounced fetchLeads (~80ms) can still
        // see the moving ID and preserve the optimistic state, avoiding a snap-back.
        setTimeout(() => {
          movingLeadIdsRef.current.delete(id);
        }, 300);
      } catch (err) {
        if (!isCurrent()) return;
        if (isLatestLeadMutation(id, generation)) {
          setLeads((prev) =>
            prev.map((lead) =>
              lead.id === id && lead.status === status && previousLead ? previousLead : lead,
            ),
          );
          notifyApiError(err, { context: "Atualizar status do lead" });
        }
        setMovingLeadIds((prev) => {
          const next = new Set(prev);
          next.delete(id);
          return next;
        });
        setTimeout(() => {
          movingLeadIdsRef.current.delete(id);
        }, 300);
        throw err;
      } finally {
        if (leadStatusMutationQueueRef.current.get(id) === mutation) {
          leadStatusMutationQueueRef.current.delete(id);
        }
        if (leadMutationRef.current === mutation) {
          leadMutationRef.current = null;
        }
      }
    },
    [beginLeadMutation, beginScopedRequest, isLatestLeadMutation, upsertLeadFromApi],
  );

  const inviteClient = useCallback(
    async (id: string) => {
      const isCurrent = beginScopedRequest();
      await leadService.inviteClient(id);
      if (isCurrent()) toast.success("Convite enviado com sucesso para o portal do cliente!");
    },
    [beginScopedRequest],
  );

  const value = useMemo<LeadsContextValue>(
    () => ({
      leads,
      setLeads,
      leadsLoading,
      leadsError,
      fetchLeads,
      addLead,
      updateLead,
      updateLeadStatus,
      inviteClient,
      isRouletteEnabled,
      isRouletteMutationPending,
      setIsRouletteEnabled,
      activeRouletteAgents,
      setActiveRouletteAgents,
      togglingAgents,
      movingLeadIds,
    }),
    [
      activeRouletteAgents,
      addLead,
      fetchLeads,
      inviteClient,
      isRouletteEnabled,
      isRouletteMutationPending,
      leads,
      leadsError,
      leadsLoading,
      movingLeadIds,
      setActiveRouletteAgents,
      setIsRouletteEnabled,
      togglingAgents,
      updateLead,
      updateLeadStatus,
    ],
  );

  return <LeadsContext.Provider value={value}>{children}</LeadsContext.Provider>;
}

export function useLeadsContext() {
  const ctx = useContext(LeadsContext);
  if (!ctx) {
    throw new Error("useLeadsContext must be used within LeadsProvider");
  }
  return ctx;
}
