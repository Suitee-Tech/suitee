import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { DealDTO } from "../../../shared/services/deal.service";
import { notifyApiError } from "../../../shared/services/errorToast";
import { usePortalAuth } from "./PortalAuthContext";
import { portalKeys } from "../hooks/queryKeys";
import { newestDeal } from "../../../shared/deals/revisions";
import { usePortalWebSocket } from "../hooks/usePortalWebSocket";
import { fetchPortalDeal, fetchPortalDeals, sharePortalDeal, sharePortalDeals } from "../hooks/useDeals";

interface PortalDealsContextType {
  deals: DealDTO[];
  activeDeal: DealDTO | null;
  activeDealId: string | null;
  isLoading: boolean;
  error: Error | null;
  setActiveDealId: (id: string) => void;
  refreshDeals: () => Promise<void>;
  retry: () => void;
  getDeal: (id: string) => DealDTO | null;
  fetchDeal: (id: string, options?: { force?: boolean }) => Promise<DealDTO>;
  invalidateDeal: (id: string) => Promise<DealDTO>;
  updateDeal: (id: string, updater: (prev: DealDTO) => DealDTO) => void;
}

const PortalDealsContext = createContext<PortalDealsContextType | undefined>(undefined);

const STORAGE_KEY = "portal_active_deal_id";

function safeStorageGet(key: string): string | null {
  try {
    if (typeof window === "undefined") return null;
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

function safeStorageSet(key: string, value: string) {
  try {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(key, value);
  } catch {}
}

function safeStorageRemove(key: string) {
  try {
    if (typeof window === "undefined") return;
    window.localStorage.removeItem(key);
  } catch {}
}

function pickDefaultDeal(deals: DealDTO[]): DealDTO | null {
  const open = deals.find((d) => d.status !== "CONVERTED" && d.status !== "CANCELLED");
  return open || deals[0] || null;
}

export function PortalDealsProvider({ children }: { children: React.ReactNode }) {
  const { isAuthenticated } = usePortalAuth();
  const queryClient = useQueryClient();
  const { onMessage, isConnected } = usePortalWebSocket();
  const [activeDealId, setActiveDealIdState] = useState<string | null>(() => safeStorageGet(STORAGE_KEY));

  const dealsQuery = useQuery({
    queryKey: portalKeys.deals(),
    queryFn: ({ signal }) => fetchPortalDeals(signal),
    structuralSharing: sharePortalDeals,
    refetchOnWindowFocus: "always",
    refetchOnReconnect: "always",
    enabled: isAuthenticated,
    // Refresca periodicamente para o cliente acompanhar a transição de
    // "pré-negociação" para o negócio assim que ele for aberto na Esteira
    // (sem depender de recarregar a página).
    refetchInterval: 60000,
    refetchIntervalInBackground: false,
  });

  const deals = useMemo(() => dealsQuery.data ?? [], [dealsQuery.data]);

  // Detail mutations/refetches must also update the dashboard and switcher.
  useEffect(() => {
    if (!isAuthenticated) return;
    return queryClient.getQueryCache().subscribe((event) => {
      if (event.type !== "updated" || event.action.type !== "success") return;
      const key = event.query.queryKey;
      if (key.length !== 3 || key[0] !== "portal" || key[1] !== "deal") return;
      const next = event.query.state.data as DealDTO | undefined;
      if (!next?.id) return;
      queryClient.setQueryData<DealDTO[]>(portalKeys.deals(), (previous) => {
        if (!previous) return previous;
        const index = previous.findIndex((d) => d.id === next.id);
        if (index < 0) return previous;
        const merged = newestDeal(previous[index], next);
        if (merged === previous[index]) return previous;
        return previous.map((d, i) => i === index ? merged : d);
      });
    });
  }, [isAuthenticated, queryClient]);

  useEffect(() => {
    if (!isAuthenticated) return;
    const pending = new Set<string>();
    let allDeals = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const flush = () => {
      timer = undefined;
      void queryClient.invalidateQueries({ queryKey: portalKeys.deals() });
      if (allDeals) {
        void queryClient.invalidateQueries({ predicate: (q) => q.queryKey[0] === "portal" && q.queryKey[1] === "deal" });
      } else {
        for (const id of pending) void queryClient.invalidateQueries({ queryKey: portalKeys.deal(id) });
      }
      pending.clear();
      allDeals = false;
    };
    const unsubscribe = onMessage((type, payload) => {
      if (!["deal_status_changed", "proposal_update", "checklist_solicited", "signature_update"].includes(type)) return;
      if (typeof payload?.deal_id === "string" && payload.deal_id) pending.add(payload.deal_id);
      else allDeals = true;
      if (timer === undefined) timer = setTimeout(flush, 80);
    });
    return () => { unsubscribe(); if (timer !== undefined) clearTimeout(timer); };
  }, [isAuthenticated, onMessage, queryClient]);

  // Events missed while offline or with a suspended tab are recovered by reads.
  useEffect(() => {
    if (isAuthenticated && isConnected) void queryClient.invalidateQueries({ queryKey: portalKeys.all() });
  }, [isAuthenticated, isConnected, queryClient]);
  useEffect(() => {
    if (!isAuthenticated) return;
    const refresh = () => {
      if (document.visibilityState !== "hidden") void queryClient.invalidateQueries({ queryKey: portalKeys.all() });
    };
    document.addEventListener("visibilitychange", refresh);
    return () => document.removeEventListener("visibilitychange", refresh);
  }, [isAuthenticated, queryClient]);

  useEffect(() => {
    if (dealsQuery.isError && dealsQuery.error) {
      notifyApiError(dealsQuery.error, { context: "Carregar negócios do portal" });
    }
  }, [dealsQuery.isError, dealsQuery.error]);

  useEffect(() => {
    if (!isAuthenticated) return;
    if (!dealsQuery.isSuccess) return;
    const stored = safeStorageGet(STORAGE_KEY);
    const current = stored ? deals.find((d) => d.id === stored) : null;
    if (current) {
      if (stored !== activeDealId) setActiveDealIdState(stored);
      return;
    }
    const fallback = pickDefaultDeal(deals);
    if (fallback?.id) {
      setActiveDealIdState(fallback.id);
      safeStorageSet(STORAGE_KEY, fallback.id);
    } else {
      setActiveDealIdState(null);
      safeStorageRemove(STORAGE_KEY);
    }
  }, [activeDealId, deals, dealsQuery.isSuccess, isAuthenticated]);

  useEffect(() => {
    if (isAuthenticated) return;
    queryClient.removeQueries({ queryKey: portalKeys.all() });
    setActiveDealIdState(null);
    safeStorageRemove(STORAGE_KEY);
  }, [isAuthenticated, queryClient]);

  const refreshDeals = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: portalKeys.deals() });
  }, [queryClient]);

  const retry = useCallback(() => {
    refreshDeals().catch(() => {});
  }, [refreshDeals]);

  const setActiveDealId = useCallback((id: string) => {
    setActiveDealIdState(id);
    safeStorageSet(STORAGE_KEY, id);
  }, []);

  const getDeal = useCallback(
    (id: string): DealDTO | null => {
      if (!id) return null;
      const cached = queryClient.getQueryData<DealDTO>(portalKeys.deal(id));
      const listed = deals.find((d) => d.id === id);
      return cached ? newestDeal(listed, cached) : listed || null;
    },
    [deals, queryClient],
  );

  const fetchDeal = useCallback(
    async (id: string, options?: { force?: boolean }) => {
      if (!id) throw new Error("missing deal id");
      if (options?.force) {
        await queryClient.invalidateQueries({ queryKey: portalKeys.deal(id), exact: true, refetchType: "none" });
      }
      const data = await queryClient.fetchQuery({
        queryKey: portalKeys.deal(id),
        queryFn: ({ signal }) => fetchPortalDeal(id, signal),
        structuralSharing: sharePortalDeal,
      });
      queryClient.setQueryData<DealDTO[] | undefined>(portalKeys.deals(), (prev) => {
        if (!prev) return prev;
        const idx = prev.findIndex((d) => d.id === id);
        if (idx < 0) return prev;
        const next = prev.slice();
        next[idx] = newestDeal(next[idx], data);
        return next;
      });
      return data;
    },
    [queryClient],
  );

  const invalidateDeal = useCallback(async (id: string) => {
    return fetchDeal(id, { force: true });
  }, [fetchDeal]);

  const updateDeal = useCallback(
    (id: string, updater: (prev: DealDTO) => DealDTO) => {
      if (!id) return;
      const cached = queryClient.getQueryData<DealDTO>(portalKeys.deal(id));
      const listed = (queryClient.getQueryData<DealDTO[]>(portalKeys.deals()) ?? []).find((d) => d.id === id);
      const current = cached ? newestDeal(listed, cached) : listed;
      if (!current) return;
      const next = updater(current);
      queryClient.setQueryData<DealDTO>(portalKeys.deal(id), next);
      queryClient.setQueryData<DealDTO[] | undefined>(portalKeys.deals(), (prev) => {
        if (!prev) return prev;
        const idx = prev.findIndex((d) => d.id === id);
        if (idx < 0) return prev;
        const arr = prev.slice();
        arr[idx] = next;
        return arr;
      });
    },
    [queryClient],
  );

  const activeDeal = useMemo(() => {
    if (!activeDealId) return pickDefaultDeal(deals);
    return getDeal(activeDealId) || pickDefaultDeal(deals);
  }, [activeDealId, deals, getDeal]);

  const error =
    dealsQuery.error instanceof Error ? dealsQuery.error : dealsQuery.error ? new Error(String(dealsQuery.error)) : null;

  return (
    <PortalDealsContext.Provider
      value={{
        deals,
        activeDeal,
        activeDealId,
        isLoading: dealsQuery.isPending,
        error,
        setActiveDealId,
        refreshDeals,
        retry,
        getDeal,
        fetchDeal,
        invalidateDeal,
        updateDeal,
      }}
    >
      {children}
    </PortalDealsContext.Provider>
  );
}

export function usePortalDeals(): PortalDealsContextType {
  const ctx = useContext(PortalDealsContext);
  if (!ctx) throw new Error("usePortalDeals must be used within a PortalDealsProvider");
  return ctx;
}
