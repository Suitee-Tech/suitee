import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { brokerService, type BrokerDTO } from "../services/broker.service";
import { useDataChangeRefresh } from "../hooks/useDataChangeRefresh";
import { useRequestScopeGuard } from "../hooks/useRequestScopeGuard";
import { getAvatarThumbUrl } from "../media";
import { retry, isAbortedError } from "../lib/retry";
import { readAgentsCache, writeAgentsCache } from "../lib/agentsCache";
import type { Agent } from "../types/agent";

type CompanyMemberLike = {
  account_id: string;
  email: string;
  name?: string;
  functional_role?: string;
  profile?: string;
  avatar_url?: string;
};

type AgentsContextValue = {
  agents: Agent[];
  refreshBrokers: () => Promise<void>;
};

const AgentsContext = createContext<AgentsContextValue | null>(null);

export function AgentsProvider({
  children,
  isAuthenticated,
  companyId,
  permissionsLoading,
  canReadBrokers,
  listCompanyMembers,
  getPermissions,
  me,
}: {
  children: ReactNode;
  isAuthenticated: boolean;
  companyId?: string;
  permissionsLoading: boolean;
  canReadBrokers: boolean;
  listCompanyMembers?: (companyId: string) => Promise<CompanyMemberLike[]>;
  getPermissions?: () => Promise<unknown>;
  me?: { id?: string; name?: string; avatar_url?: string; role?: string };
}) {
  const [agents, setAgents] = useState<Agent[]>([]);
  const beginScopedRequest = useRequestScopeGuard(`${isAuthenticated}:${companyId ?? ""}`);
  const previousScopeRef = useRef(`${isAuthenticated}:${companyId ?? ""}`);
  const refreshPromiseRef = useRef<{ scope: string; promise: Promise<void> } | null>(null);
  const agentsMutationRef = useRef<Promise<unknown> | null>(null);
  const agentsVersionRef = useRef(0);
  const agentsCooldownUntilRef = useRef<number>(0);

  const mapBrokersToAgents = useCallback((brokers: BrokerDTO[]): Agent[] => {
    const filtered = brokers.filter((b) => b.active !== false);
    const mapped: Agent[] = filtered.map((b) => ({
      id: b.id,
      name: b.name,
      role: b.role,
      avatar: b.avatar ? getAvatarThumbUrl(b.avatar, 64) : undefined,
    }));
    return mapped.filter(
      (agent, index, self) =>
        index === self.findIndex((t) => t.id === agent.id || (t.name === agent.name && !!t.name)),
    );
  }, []);

  const syncBrokersWithMembers = useCallback(
    async (resolvedCompanyId: string, initialBrokers: BrokerDTO[]) => {
      const isCurrent = beginScopedRequest();
      if (!listCompanyMembers) return;

      // Bloqueia fetch de reconciliação WS durante a sincronização
      let releasePlaceholder: () => void;
      agentsMutationRef.current = new Promise<void>((r) => { releasePlaceholder = r; });
      ++agentsVersionRef.current;

      try {
        const members = await listCompanyMembers(resolvedCompanyId);
        if (!isCurrent()) return;

        if (getPermissions) {
          try {
            await getPermissions();
            if (!isCurrent()) return;
          } catch (e) {
            if (!isCurrent()) return;
            console.warn("[brokers] permissões não disponíveis para sync de roleta", e);
          }
        }

        let needsRefetch = false;
        for (const m of members) {
          if (!isCurrent()) return;
          const existingBroker = initialBrokers.find((b) => b.id === m.account_id || b.email === m.email);
          try {
            if (existingBroker) {
              const incomingRole = m.functional_role || m.profile;
              const expectedActive = m.profile !== "removed";
              if (
                existingBroker.role !== incomingRole ||
                existingBroker.name !== (m.name || m.email) ||
                existingBroker.active !== expectedActive ||
                existingBroker.avatar !== m.avatar_url
              ) {
                await brokerService.update(existingBroker.id, {
                  name: m.name || m.email,
                  role: incomingRole,
                  active: expectedActive,
                  avatar: m.avatar_url,
                });
                needsRefetch = true;
              }
            } else if (m.profile !== "removed") {
              await brokerService.create({
                id: m.account_id,
                name: m.name || m.email,
                role: m.functional_role || m.profile || "corretor",
                email: m.email,
                avatar: m.avatar_url,
                active: true,
              });
              needsRefetch = true;
            }
          } catch (memberErr) {
            console.warn(`[brokers] falha ao sincronizar membro ${m.account_id}`, memberErr);
          }
        }

        for (const b of initialBrokers) {
          if (!isCurrent()) return;
          const stillInCompany = members.find((m) => m.account_id === b.id || m.email === b.email);
          if (!stillInCompany && b.active !== false && b.role !== "removed") {
            try {
              await brokerService.update(b.id, { role: "removed", active: false });
              needsRefetch = true;
            } catch {
            }
          }
        }

        if (needsRefetch) {
          try {
            const newRes = await retry(() => brokerService.getAll(), {
              attempts: 2,
              baseDelayMs: 500,
            });
            if (!isCurrent()) return;
            const newBrokersData: BrokerDTO[] = newRes.data?.data || [];
            const mapped = mapBrokersToAgents(newBrokersData);
            setAgents(mapped);
            writeAgentsCache(resolvedCompanyId, mapped);
          } catch (e) {
            console.warn("[brokers] refetch pós-sync falhou; mantendo agents anteriores", e);
          }
        }
      } catch (e) {
        if (!isCurrent()) return;
        console.warn("[brokers] sync com membros da empresa falhou (silencioso)", e);
      } finally {
        releasePlaceholder!();
        agentsMutationRef.current = null;
        agentsCooldownUntilRef.current = Date.now() + 2000;
      }
    },
    [beginScopedRequest, getPermissions, listCompanyMembers, mapBrokersToAgents],
  );

  const refreshBrokers = useCallback(() => {
    const scope = `${isAuthenticated}:${companyId ?? ""}`;
    const existing = refreshPromiseRef.current;
    if (existing?.scope === scope) return existing.promise;

    let promise: Promise<void>;
    promise = (async () => {
      const pendingMutation = agentsMutationRef.current;
      if (pendingMutation) {
        try { await pendingMutation; } catch {}
      }
      const isCurrent = beginScopedRequest();
      const resolvedCompanyId = companyId || "";
      const fetchVersion = agentsVersionRef.current;

      if (resolvedCompanyId) {
        const cached = readAgentsCache(resolvedCompanyId);
        if (cached && cached.length > 0 && isCurrent()) {
          setAgents(cached);
        }
      }

      let brokersData: BrokerDTO[] = [];
      try {
        const response = await retry(() => brokerService.getAll(), {
          attempts: 3,
          baseDelayMs: 500,
          onRetry: (attempt, err) => {
            console.warn(`[brokers] tentativa ${attempt} falhou, retentando...`, err);
          },
        });
        if (!isCurrent()) return;
        if (agentsVersionRef.current !== fetchVersion) return;
        if (Date.now() < agentsCooldownUntilRef.current) return;
        brokersData = response.data?.data || [];

        const mappedAgents = mapBrokersToAgents(brokersData);
        setAgents(mappedAgents);
        if (resolvedCompanyId) writeAgentsCache(resolvedCompanyId, mappedAgents);
      } catch (error) {
        if (!isCurrent()) return;
        if (isAbortedError(error as any)) return;
        console.error("Erro ao buscar corretores após retries:", error);
      }

      if (!resolvedCompanyId || !isCurrent()) return;
      await syncBrokersWithMembers(resolvedCompanyId, brokersData);
    })().finally(() => {
      if (refreshPromiseRef.current?.promise === promise) {
        refreshPromiseRef.current = null;
      }
    });

    refreshPromiseRef.current = { scope, promise };
    return promise;
  }, [beginScopedRequest, companyId, isAuthenticated, mapBrokersToAgents, syncBrokersWithMembers]);

  useEffect(() => {
    const scope = `${isAuthenticated}:${companyId ?? ""}`;
    const scopeChanged = previousScopeRef.current !== scope;
    previousScopeRef.current = scope;

    if (!isAuthenticated) {
      setAgents([]);
      return;
    }

    if (scopeChanged) setAgents([]);
    if (!companyId || permissionsLoading) return;

    if (!canReadBrokers) {
      setAgents([]);
      return;
    }
    void refreshBrokers();
  }, [canReadBrokers, companyId, isAuthenticated, permissionsLoading, refreshBrokers]);

  useEffect(() => {
    if (!isAuthenticated || !companyId || permissionsLoading) return;
    if (!canReadBrokers) return;

    let lastRefetchAt = Date.now();
    const MIN_INTERVAL_MS = 30 * 1000;

    const onVisibilityChange = () => {
      if (typeof document === "undefined" || document.visibilityState !== "visible") return;
      const now = Date.now();
      if (now - lastRefetchAt < MIN_INTERVAL_MS) return;
      lastRefetchAt = now;
      refreshBrokers().catch(() => {});
    };

    document.addEventListener("visibilitychange", onVisibilityChange);
    window.addEventListener("focus", onVisibilityChange);
    return () => {
      document.removeEventListener("visibilitychange", onVisibilityChange);
      window.removeEventListener("focus", onVisibilityChange);
    };
  }, [canReadBrokers, companyId, isAuthenticated, permissionsLoading, refreshBrokers]);

  useDataChangeRefresh(["agents"], refreshBrokers, {
    enabled: isAuthenticated && Boolean(companyId) && !permissionsLoading && canReadBrokers,
  });

  // Garante que o próprio usuário logado esteja sempre resolvível como
  // corretor/captador, mesmo quando o usuário não tem a permissão
  // brokers:read (ex. perfil corretor), em que agents vem vazio — assim o
  // nome/foto do próprio corretor aparece em vez de um id cru.
  const agentsWithMe = useMemo<Agent[]>(() => {
    if (!me?.id) return agents;
    if (agents.some((a) => a.id === me.id)) return agents;
    return [
      {
        id: me.id,
        name: me.name || "Você",
        role: me.role || "corretor",
        avatar: me.avatar_url ? getAvatarThumbUrl(me.avatar_url, 64) : undefined,
      },
      ...agents,
    ];
  }, [agents, me]);

  const value = useMemo<AgentsContextValue>(
    () => ({
      agents: agentsWithMe,
      refreshBrokers,
    }),
    [agentsWithMe, refreshBrokers],
  );

  return <AgentsContext.Provider value={value}>{children}</AgentsContext.Provider>;
}

export function useAgentsContext() {
  const ctx = useContext(AgentsContext);
  if (!ctx) throw new Error("useAgentsContext must be used within AgentsProvider");
  return ctx;
}
