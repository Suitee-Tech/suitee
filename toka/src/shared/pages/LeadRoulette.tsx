import React, { useState, useRef, useEffect, useMemo, useCallback } from "react";
import { downloadCsv } from "../utils/csvExport";
import { brokerService } from "../services/broker.service";
import { leadService, type QualField, type QualRoutingConfig, type RouletteConfig } from "../services/lead.service";
import { Button } from "../ui/button";
import {
  Users,
  Clock,
  RefreshCw,
  Settings,
  History,
  Shield,
  Search,
  Play,
  Pause,
  ArrowRight,
  UserCheck,
  Building2,
  ListPlus,
  Activity,
  LayoutGrid,
  List,
  Wrench,
  X,
  AlertTriangle,
  Loader2,
} from "lucide-react";
import { clsx } from "clsx";
import { motion, AnimatePresence } from "motion/react";
import { DEFAULT_QUAL_FIELDS, QualificationFormBuilder } from "../components/QualificationFormBuilder";
import { SLARulesConfig } from "../components/SLARulesConfig";
import { BrokerCapabilitiesPanel } from "../components/BrokerCapabilitiesPanel";
import { MediaImage } from "../components/MediaImage";
import { useLeadsContext } from "../context/LeadsContext";
import { useAgentsContext } from "../context/AgentsContext";
import { usePropertiesContext } from "../context/PropertiesContext";
import { useFocusMode } from "../context/FocusModeContext";
import { useWsSubscription } from "../ws/useWsSubscription";
import { useWsConnectionState } from "../ws/useWsConnectionState";
import { RouletteUnassignedAlertDialog } from "../components/RouletteUnassignedAlertDialog";
import { useRouletteAudit, useRouletteConfig, useRouletteConfigWatcher } from "../hooks/useRouletteConfig";
import { useCan } from "../../lib/permissions/usePermission";
import { resolvePropertyDisplay } from "../data";
import { formatISODateBR } from "../utils/format";

type RouletteHistory = { id: string; lead: string; intent: string; broker: string; status: string; time: string; sla: string };

type TabState = "geral" | "config" | "ferramentas" | "historico" | "qualificacao" | "aovivo";

type BrokerRow = {
  id: string;
  name: string;
  avatar: string;
  status: "online" | "paused";
  slaPaused: boolean;
  load: number;
  maxLoad: number;
  specialty: string[];
  region: string;
  lastAssigned: string;
  lastAssignedAt: number | null;
  role: string;
};

const LEAD_STAGES = [
  { value: "NEW", label: "Novo" },
  { value: "PENDING_ACCEPTANCE", label: "Pendente Aceite" },
  { value: "CONTACTED", label: "Contatado" },
  { value: "QUALIFIED", label: "Qualificado" },
  { value: "VISIT_SCHEDULED", label: "Visita Agendada" },
  { value: "PROPOSAL", label: "Proposta" },
  { value: "NEGOTIATION", label: "Negociação" },
  { value: "CONVERTED", label: "Convertido" },
  { value: "LOST", label: "Perdido" },
];

export function LeadRoulette() {
  const {
    isRouletteEnabled,
    isRouletteMutationPending,
    setIsRouletteEnabled,
    activeRouletteAgents,
    setActiveRouletteAgents,
    togglingAgents,
    leads,
  } = useLeadsContext();
  const { agents, refreshBrokers } = useAgentsContext();
  const { properties } = usePropertiesContext();
  const { focusMode } = useFocusMode();
  const [activeTab, setActiveTab] = useState<TabState>("aovivo");
  const [configTab, setConfigTab] = useState<"regras" | "pools">("regras");
  const [brokersQuery, setBrokersQuery] = useState("");
  const [brokersStatusFilter, setBrokersStatusFilter] = useState<"all" | "online" | "paused" | "slaPaused">("all");
  const [brokersRoleFilter, setBrokersRoleFilter] = useState<string>("all");
  const [brokersSort, setBrokersSort] = useState<"queue" | "name" | "load" | "last">("queue");
  const [liveViewMode, setLiveViewMode] = useState<"cards" | "list">("cards");
  const [historyData, setHistoryData] = useState<RouletteHistory[]>([]);
  const [unassignedDialogOpen, setUnassignedDialogOpen] = useState(false);
  const [unassignedCount, setUnassignedCount] = useState(0);
  const [unassignedDialogLoading, setUnassignedDialogLoading] = useState(false);
  const [rouletteLastIndex, setRouletteLastIndex] = useState<number>(-1);
  const [stageBrokerPoolsMap, setStageBrokerPoolsMap] = useState<Record<string, string[]>>({});
  const [queueStage, setQueueStage] = useState<string>("NEW");
  const [stageLastIndexMap, setStageLastIndexMap] = useState<Record<string, number>>({});
  const [rouletteQualFields, setRouletteQualFields] = useState<QualField[]>([]);
  const [rouletteQualRouting, setRouletteQualRouting] = useState<QualRoutingConfig | null>(null);
  const queueStageLockedRef = useRef(false);

  const handleToggleRoulette = useCallback(
    async () => {
      if (isRouletteMutationPending) return;

      try {
        if (isRouletteEnabled) {
          await setIsRouletteEnabled(false);
          return;
        }

        const status = await leadService.getRouletteUnassignedStatus();
        const hasUnassigned = !!status?.has_unassigned;
        const count = Number(status?.count || 0);
        if (hasUnassigned && count > 0) {
          setUnassignedCount(count);
          setUnassignedDialogOpen(true);
          return;
        }
        await setIsRouletteEnabled(true);
      } catch {
        // LeadsContext already rolls the optimistic toggle back and reports mutation errors.
      }
    },
    [isRouletteEnabled, isRouletteMutationPending, setIsRouletteEnabled],
  );

  const handleEnableAssignAutomatically = useCallback(
    async () => {
      setUnassignedDialogLoading(true);
      try {
        await setIsRouletteEnabled(true, { assignUnassignedLeads: true });
        setUnassignedDialogOpen(false);
      } catch {
        // Keep the decision dialog open so the user can retry.
      } finally {
        setUnassignedDialogLoading(false);
      }
    },
    [setIsRouletteEnabled],
  );

  const handleEnableKeepManual = useCallback(
    async () => {
      setUnassignedDialogLoading(true);
      try {
        await setIsRouletteEnabled(true, { assignUnassignedLeads: false });
        setUnassignedDialogOpen(false);
      } catch {
        // Keep the decision dialog open so the user can retry.
      } finally {
        setUnassignedDialogLoading(false);
      }
    },
    [setIsRouletteEnabled],
  );

  const handleUnassignedDialogCancel = useCallback(() => {
    if (unassignedDialogLoading) return;
    setUnassignedDialogOpen(false);
  }, [unassignedDialogLoading]);
  const [stagePoolsError, setStagePoolsError] = useState<string>("");
  const [stagePools, setStagePools] = useState<Array<{ stage: string; brokerIds: string[] }>>([]);
  const [newStage, setNewStage] = useState<string>("NEW");
  const [openStage, setOpenStage] = useState<string | null>(null);
  const [brokerSearch, setBrokerSearch] = useState<string>("");

  const [simCount, setSimCount] = useState<number>(10);
  const [simStage, setSimStage] = useState<string>("NEW");
  const [simPersist, setSimPersist] = useState<boolean>(false);
  const [simSeed, setSimSeed] = useState<number>(0);
  const [simResult, setSimResult] = useState<any | null>(null);

  useRouletteConfigWatcher();

  const rouletteConfigQuery = useRouletteConfig();
  const configWarnings = rouletteConfigQuery.data?.warnings ?? [];

  const can = useCan();
  const canViewAudit = can("system:view_audit_trail");
  const canConfigureRoulette = can("leads:configure_roulette");
  const canViewRoulette = can("leads:view_roulette") || canConfigureRoulette;

  useEffect(() => {
    const allowed: Record<TabState, boolean> = {
      aovivo: canViewRoulette,
      geral: canViewRoulette,
      config: canConfigureRoulette,
      qualificacao: canConfigureRoulette,
      historico: canViewAudit,
      ferramentas: canConfigureRoulette,
    };
    if (allowed[activeTab]) return;
    const fallback: TabState | null = canViewRoulette ? "aovivo" : canViewAudit ? "historico" : canConfigureRoulette ? "config" : null;
    if (fallback && fallback !== activeTab) {
      setActiveTab(fallback);
    }
  }, [activeTab, canViewRoulette, canConfigureRoulette, canViewAudit]);
  const [auditAction, setAuditAction] = useState<string>("");
  const [auditTargetID, setAuditTargetID] = useState<string>("");
  const [auditFrom, setAuditFrom] = useState<string>("");
  const [auditTo, setAuditTo] = useState<string>("");
  const [auditOffset, setAuditOffset] = useState<number>(0);
  const [auditExpanded, setAuditExpanded] = useState<Record<string, boolean>>({});
  const auditLimit = 20;
  const auditQuery = useRouletteAudit(
    {
      limit: auditLimit,
      offset: auditOffset,
      action: auditAction || undefined,
      target_id: auditTargetID.trim() || undefined,
      from: auditFrom || undefined,
      to: auditTo || undefined,
    },
    { enabled: canViewAudit && activeTab === "historico" },
  );

  useEffect(() => {
    setAuditOffset(0);
  }, [auditAction, auditTargetID, auditFrom, auditTo]);

  const auditDownloadCSV = useCallback(() => {
    const items = auditQuery.data?.items ?? [];
    if (items.length === 0) return;
    const header = ["created_at", "action", "actor_id", "actor_email", "target_id", "diff"];
    const escapeCSV = (v: unknown) => {
      const s = v == null ? "" : typeof v === "string" ? v : JSON.stringify(v);
      if (/[",\n\r]/.test(s)) return '"' + s.replace(/"/g, '""') + '"';
      return s;
    };
    const rows = items.map((e) =>
      [new Date(e.created_at).toISOString(), e.action, e.actor_id, e.actor_email ?? "", e.target_id ?? "", e.diff ?? {}].map(escapeCSV).join(","),
    );
    const csv = [header.join(","), ...rows].join("\n");
    const stamp = new Date().toISOString().replace(/[:.]/g, "-");
    // downloadCsv injeta o BOM UTF-8 (Excel sem BOM lê os acentos como ANSI).
    downloadCsv(`roleta-audit-${stamp}.csv`, csv);
  }, [auditQuery.data]);

  const auditToggleExpand = useCallback((id: string) => {
    setAuditExpanded((prev) => ({ ...prev, [id]: !prev[id] }));
  }, []);

  const [brokerDetails, setBrokerDetails] = useState<Record<string, any>>({});
  const [capDialogOpen, setCapDialogOpen] = useState(false);
  const [capBrokerId, setCapBrokerId] = useState<string | null>(null);

  const brokerCapabilityOptions = useMemo(() => {
    const specs = new Set<string>();
    const regs = new Set<string>();
    const fields = rouletteQualFields ?? [];
    const routing = rouletteQualRouting;
    const selectableFields = fields.filter((f) => f.active && (f.type === "select" || f.type === "multiselect"));
    const specialtyFieldIds = Array.isArray(routing?.specialtyFieldIds) ? routing!.specialtyFieldIds : [];
    const specialtySources = specialtyFieldIds.length > 0 ? selectableFields.filter((f) => specialtyFieldIds.includes(f.id)) : selectableFields;
    specialtySources.forEach((f) => {
      (Array.isArray(f.options) ? f.options : []).forEach((s) => {
        const v = String(s || "").trim();
        if (v) specs.add(v);
      });
    });

    const regionFieldId = routing?.regionFieldId || null;
    const regionSources = regionFieldId
      ? fields.filter((f) => f.id === regionFieldId)
      : fields.filter((f) => {
          const label = String(f.label || "").toLowerCase();
          return label.includes("regiao") || label.includes("bairro") || label.includes("local");
        });
    regionSources.forEach((f) => {
      (Array.isArray(f.options) ? f.options : []).forEach((r: any) => {
        const v = String(r || "").trim();
        if (v) regs.add(v);
      });
    });
    Object.values(brokerDetails || {}).forEach((b: any) => {
      (Array.isArray(b?.specialties) ? b.specialties : []).forEach((s: any) => {
        const v = String(s || "").trim();
        if (v) specs.add(v);
      });
      (Array.isArray(b?.regions) ? b.regions : []).forEach((r: any) => {
        const v = String(r || "").trim();
        if (v) regs.add(v);
      });
    });
    return {
      specialties: Array.from(specs).sort((a, b) => a.localeCompare(b)),
      regions: Array.from(regs).sort((a, b) => a.localeCompare(b)),
    };
  }, [brokerDetails, rouletteQualFields, rouletteQualRouting]);

  const brokers = useMemo<BrokerRow[]>(() => {
    const perAgent: Record<string, { active: number; last: number | null }> = {};
    for (const l of leads) {
      if (!l.agentId) continue;
      const key = l.agentId;
      if (!perAgent[key]) perAgent[key] = { active: 0, last: null };
      if (!["Perdido", "Concluído"].includes(l.status)) {
        perAgent[key].active += 1;
      }
      const createdAt = (l as any)?.created_at;
      if (createdAt) {
        const ms = new Date(createdAt).getTime();
        if (!Number.isNaN(ms)) {
          const prev = perAgent[key].last;
          if (prev === null || ms > prev) perAgent[key].last = ms;
        }
      }
    }

    return agents.map((b) => {
      const details = brokerDetails[b.id];
      const stats = perAgent[b.id] || { active: 0, last: null };
      const lastAssignedStr = stats.last ? formatISODateBR(new Date(stats.last).toISOString()) : "Nunca";
      const specialties =
        Array.isArray(details?.specialties) && details.specialties.length > 0
          ? (details.specialties as any[]).map((s) => String(s || "").trim()).filter(Boolean)
          : [String(b.role || "Geral")];
      const maxLoad = typeof details?.max_active_leads === "number" ? (details.max_active_leads as number) : 0;
      const region =
        Array.isArray(details?.regions) && details.regions.length > 0
          ? (details.regions as any[]).map((r) => String(r || "").trim()).filter(Boolean).join(", ")
          : "Todas";
      return {
        id: b.id,
        name: b.name,
        avatar: b.avatar || "",
        status: activeRouletteAgents.includes(b.id) ? "online" : "paused",
        slaPaused: !!details?.paused,
        load: stats.active,
        maxLoad,
        specialty: specialties,
        region,
        lastAssigned: lastAssignedStr,
        lastAssignedAt: stats.last,
        role: (b.role || "corretor").toLowerCase(),
      };
    });
  }, [activeRouletteAgents, agents, brokerDetails, leads]);

  const selectedBrokerSummary = useMemo<BrokerRow | null>(() => {
    if (!capBrokerId) return null;
    return brokers.find((x) => x.id === capBrokerId) ?? null;
  }, [brokers, capBrokerId]);

  const brokerRoleOptions = useMemo(() => {
    const counts: Record<string, number> = {};
    brokers.forEach((b) => {
      const role = (b.role || "corretor").toLowerCase();
      counts[role] = (counts[role] || 0) + 1;
    });
    const sorted = Object.keys(counts).sort((a, b) => a.localeCompare(b));
    return sorted.map((role) => ({ role, count: counts[role] }));
  }, [brokers]);

  const roleLabel = (role: string) =>
    role === "corretor"
      ? "Corretor"
      : role === "financeiro"
        ? "Financeiro"
        : role === "admin"
          ? "Admin"
          : role === "operacoes"
            ? "Operações"
            : role === "gestor"
              ? "Gestor"
              : role;

  const filteredBrokers = useMemo(() => {
    const q = brokersQuery.trim().toLowerCase();
    let list = brokers.slice();

    if (brokersStatusFilter === "online") {
      list = list.filter((b) => b.status === "online" && !b.slaPaused);
    } else if (brokersStatusFilter === "paused") {
      list = list.filter((b) => b.status === "paused" && !b.slaPaused);
    } else if (brokersStatusFilter === "slaPaused") {
      list = list.filter((b) => b.slaPaused);
    }

    if (brokersRoleFilter !== "all") {
      list = list.filter((b) => (b.role || "corretor").toLowerCase() === brokersRoleFilter);
    }

    if (q) {
      list = list.filter((b) => {
        const hay = [b.name, b.id, b.region, ...(Array.isArray(b.specialty) ? b.specialty : [])]
          .filter(Boolean)
          .join(" ")
          .toLowerCase();
        return hay.includes(q);
      });
    }

    const withQueueSort = (a: any, b: any) => {
      const aSla = a.slaPaused ? 1 : 0;
      const bSla = b.slaPaused ? 1 : 0;
      if (aSla !== bSla) return aSla - bSla;
      const aOnline = a.status === "online" ? 0 : 1;
      const bOnline = b.status === "online" ? 0 : 1;
      if (aOnline !== bOnline) return aOnline - bOnline;
      const aLoad = typeof a.load === "number" ? a.load : 0;
      const bLoad = typeof b.load === "number" ? b.load : 0;
      if (aLoad !== bLoad) return bLoad - aLoad;
      return String(a.name || "").localeCompare(String(b.name || ""));
    };

    if (brokersSort === "name") {
      list.sort((a, b) => String(a.name || "").localeCompare(String(b.name || "")));
    } else if (brokersSort === "load") {
      list.sort((a, b) => (b.load || 0) - (a.load || 0));
    } else if (brokersSort === "last") {
      list.sort((a, b) => (b.lastAssignedAt || 0) - (a.lastAssignedAt || 0));
    } else {
      list.sort(withQueueSort);
    }

    return list;
  }, [brokers, brokersQuery, brokersRoleFilter, brokersSort, brokersStatusFilter]);

  useEffect(() => {
    refreshBrokers();
  }, [refreshBrokers]);

  useEffect(() => {
    leadService
      .getRouletteConfig()
      .then((cfg: any) => {
        setRouletteLastIndex(typeof cfg?.last_index === "number" ? cfg.last_index : -1);
        if (typeof cfg?.simulationSeed === "number") {
          setSimSeed(cfg.simulationSeed);
        }
        setRouletteQualFields(
          Array.isArray(cfg?.qualFields) && cfg.qualFields.length > 0 ? (cfg.qualFields as QualField[]) : (DEFAULT_QUAL_FIELDS as QualField[]),
        );
        setRouletteQualRouting((cfg?.qualRouting as QualRoutingConfig) ?? null);
        const pools = (cfg as any)?.stage_broker_pools || {};
        const mapped: Record<string, string[]> = {};
        Object.entries(pools).forEach(([stage, brokerIds]) => {
          const key = String(stage || "").toUpperCase();
          const ids = Array.isArray(brokerIds) ? (brokerIds as any[]).filter(Boolean).map(String) : [];
          if (key && ids.length > 0) mapped[key] = ids;
        });
        setStageBrokerPoolsMap(mapped);

        const stageIdx = (cfg as any)?.stage_last_index || {};
        const stageMapped: Record<string, number> = {};
        Object.entries(stageIdx).forEach(([stage, idx]) => {
          const key = String(stage || "").toUpperCase();
          if (!key) return;
          if (typeof idx === "number") stageMapped[key] = idx;
        });
        setStageLastIndexMap(stageMapped);
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    if (queueStageLockedRef.current) return;

    const stageKeys = new Set<string>();
    Object.keys(stageBrokerPoolsMap || {}).forEach((k) => stageKeys.add(String(k).toUpperCase()));
    Object.keys(stageLastIndexMap || {}).forEach((k) => stageKeys.add(String(k).toUpperCase()));

    if (stageKeys.size === 0) return;

    const preferred = ["NEW", "PENDING_ACCEPTANCE", "CONTACTED", "QUALIFIED", "VISIT_SCHEDULED", "PROPOSAL", "NEGOTIATION", "CONVERTED", "LOST"];

    const hasUsefulPool = (stage: string) => {
      const pool = stageBrokerPoolsMap[stage];
      if (!Array.isArray(pool) || pool.length === 0) return false;
      const active = new Set(activeRouletteAgents || []);
      return pool.some((id) => active.has(id));
    };

    let candidate = preferred.find((s) => stageKeys.has(s) && (hasUsefulPool(s) || stageLastIndexMap[s] !== undefined));
    if (!candidate) {
      let bestStage = "";
      let bestScore = -1;
      const active = new Set(activeRouletteAgents || []);
      for (const s of stageKeys) {
        const pool = stageBrokerPoolsMap[s];
        if (!Array.isArray(pool) || pool.length === 0) continue;
        const score = pool.reduce((acc, id) => acc + (active.has(id) ? 1 : 0), 0);
        if (score > bestScore) {
          bestScore = score;
          bestStage = s;
        }
      }
      candidate = bestStage || Array.from(stageKeys)[0];
    }

    const next = String(candidate || "NEW").toUpperCase();
    if (next && next !== String(queueStage || "").toUpperCase()) {
      setQueueStage(next);
    }
  }, [activeRouletteAgents, queueStage, stageBrokerPoolsMap, stageLastIndexMap]);

  const loadBrokerDetails = useCallback(async () => {
    try {
      const res = await brokerService.getAll();
      const all = res.data?.data ?? [];
      const map: Record<string, any> = {};
      for (const b of all) map[b.id] = b;
      setBrokerDetails(map);
    } catch {
      setBrokerDetails({});
    }
  }, []);

  useEffect(() => {
    loadBrokerDetails();
  }, [loadBrokerDetails]);

  useEffect(() => {
    if (activeTab !== "config" || configTab !== "pools") return;
    leadService
      .getRouletteConfig()
      .then((cfg: any) => {
        setRouletteLastIndex(typeof cfg?.last_index === "number" ? cfg.last_index : -1);
        const pools = (cfg as any)?.stage_broker_pools || {};
        const mapped: Record<string, string[]> = {};
        Object.entries(pools).forEach(([stage, brokerIds]) => {
          const key = String(stage || "").toUpperCase();
          const ids = Array.isArray(brokerIds) ? (brokerIds as any[]).filter(Boolean).map(String) : [];
          if (key && ids.length > 0) mapped[key] = ids;
        });
        setStageBrokerPoolsMap(mapped);
        const stageIdx = (cfg as any)?.stage_last_index || {};
        const stageMapped: Record<string, number> = {};
        Object.entries(stageIdx).forEach(([stage, idx]) => {
          const key = String(stage || "").toUpperCase();
          if (!key) return;
          if (typeof idx === "number") stageMapped[key] = idx;
        });
        setStageLastIndexMap(stageMapped);
        const rows = Object.entries(pools).map(([stage, brokerIds]) => ({
          stage: String(stage || "").toUpperCase(),
          brokerIds: Array.isArray(brokerIds) ? (brokerIds as any[]).filter(Boolean).map(String) : [],
        }));
        rows.sort((a, b) => a.stage.localeCompare(b.stage));
        setStagePools(rows);
        setStagePoolsError("");
      })
      .catch(() => {
        setStagePools([]);
        setStagePoolsError("");
      });
  }, [activeTab, configTab]);

  const eligibleBrokers = brokers.filter((b) => activeRouletteAgents.includes(b.id));
  const eligibleBrokersFiltered = eligibleBrokers.filter((b) => (b.name || "").toLowerCase().includes(brokerSearch.toLowerCase()));

  const removeStageRow = (stage: string) => {
    setStagePools((prev) => prev.filter((r) => r.stage !== stage));
    setOpenStage((prev) => (prev === stage ? null : prev));
  };

  const toggleBrokerInStage = (stage: string, brokerId: string) => {
    setStagePools((prev) =>
      prev.map((r) => {
        if (r.stage !== stage) return r;
        const has = r.brokerIds.includes(brokerId);
        return { ...r, brokerIds: has ? r.brokerIds.filter((id) => id !== brokerId) : [...r.brokerIds, brokerId] };
      }),
    );
  };

  const [liveFeed, setLiveFeed] = useState<any[]>([]);
  const leadCounterRef = useRef(0);

  const isWsConnected = useWsConnectionState();

  useEffect(() => {
    if (activeTab !== "aovivo") return;
    leadService
      .getRouletteHistory({ limit: 20, offset: 0 })
      .then((res: any) => {
        const items = (res as any)?.items || (res as any)?.data?.items || [];
        const mapped = (items || [])
          .slice(0, 20)
          .map((h: any) => {
            const leadId = String(h.lead_id || "");
            const brokerId = String(h.to_broker_id || "");
            const leadObj = leads.find((l) => l.id === leadId);
            const brokerObj = agents.find((a) => a.id === brokerId);
            return {
              id: leadObj?.displayId || leadId.slice(0, 8) || "—",
              uniqueKey: `history-${String(h.id || `${leadId}-${brokerId}`)}`,
              name: leadObj?.name || `Lead ${leadId.slice(0, 6)}`,
              intent: leadObj?.property ? resolvePropertyDisplay(leadObj.property, properties) : h.stage || "—",
              broker: brokerObj?.name || brokerId.slice(0, 6),
              status: String(h.stage || ""),
              time: h.created_at ? new Date(h.created_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }) : "",
            };
          });
        setLiveFeed(mapped);
      })
      .catch(() => {});
  }, [activeTab, leads, agents, properties]);

  const handleLeadAssigned = useCallback(
    (event: any) => {
      const leadId = event.payload?.lead_id as string | undefined;
      const brokerId = event.payload?.broker_id as string | undefined;
      const stage = (event.payload?.stage ?? event.payload?.status) as string | undefined;
      if (!leadId || !brokerId) return;

      const leadObj = leads.find((l) => l.id === leadId);
      const brokerObj = agents.find((a) => a.id === brokerId);

      leadCounterRef.current += 1;
      const uniqueKey = `lead-assigned-${Date.now()}-${leadCounterRef.current}`;
      setLiveFeed((curr) => [
        {
          id: leadObj?.displayId || leadId.slice(0, 8),
          uniqueKey,
          name: leadObj?.name || `Lead ${leadId.slice(0, 6)}`,
          intent: leadObj?.property ? resolvePropertyDisplay(leadObj.property, properties) : stage || "—",
          broker: brokerObj?.name || brokerId.slice(0, 6),
          status: stage || "",
          time: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
        },
        ...curr,
      ].slice(0, 100));

      const stageKey = String(stage || "NEW").toUpperCase();
      queueStageLockedRef.current = true;
      setQueueStage(stageKey);
      const pool = (stageBrokerPoolsMap[stageKey]?.length ? stageBrokerPoolsMap[stageKey] : activeRouletteAgents) || [];
      const idx = pool.indexOf(brokerId);
      if (idx >= 0) {
        setStageLastIndexMap((prev) => ({ ...prev, [stageKey]: idx }));
        if (!stageBrokerPoolsMap[stageKey]?.length) {
          setRouletteLastIndex(idx);
        }
      }
    },
    [activeRouletteAgents, agents, leads, stageBrokerPoolsMap, properties],
  );

  useWsSubscription("lead_assigned", handleLeadAssigned);

  useEffect(() => {
    if (activeTab !== "historico") return;
    leadService
      .getRouletteHistory({ limit: 50, offset: 0 })
      .then((res: any) => {
        const items = (res as any)?.items || (res as any)?.data?.items || [];
        const mapped: RouletteHistory[] = (items || []).map((h: any) => {
          const leadObj = leads.find((l) => l.id === h.lead_id);
          const brokerObj = agents.find((a) => a.id === h.to_broker_id);
          const reason = String(h.reason || "");
          const isSla = reason.startsWith("sla:");
          return {
            id: h.id,
            lead: leadObj?.name || `Lead ${String(h.lead_id || "").slice(0, 6)}`,
            intent: leadObj?.property ? resolvePropertyDisplay(leadObj.property, properties) : h.stage || "—",
            broker: brokerObj?.name || String(h.to_broker_id || "").slice(0, 6),
            status: isSla ? "redirecionado" : "aceito",
            time: h.created_at ? new Date(h.created_at).toLocaleString() : "",
            sla: isSla ? "Expirado" : "OK",
          };
        });
        setHistoryData(mapped);
      })
      .catch(() => setHistoryData([]));
  }, [activeTab, leads, agents, properties]);

  const toggleBrokerStatus = (brokerId: string, currentStatus: string) => {
    setActiveRouletteAgents((prev) => {
      let newAgents;
      if (currentStatus === "paused") {
        newAgents = prev.includes(brokerId) ? prev : [...prev, brokerId];
      } else {
        newAgents = prev.filter((id) => id !== brokerId);
      }

      return newAgents;
    });
  };

  const onlineBrokersCount = brokers.filter((b) => b.status === "online").length;
  const activeBrokersCount = brokers.length;

  const leadsHoje = leads.filter((l) => {
    if (!(l as any).created_at) return false;
    const date = new Date((l as any).created_at);
    const today = new Date();
    return date.getDate() === today.getDate() && date.getMonth() === today.getMonth() && date.getFullYear() === today.getFullYear();
  }).length;

  const getStatusColor = (status: string) => {
    switch (status) {
      case "online":
        return "bg-status-ok";
      case "paused":
        return "bg-status-warning";
      case "offline":
        return "bg-muted-foreground/30";
      default:
        return "bg-muted-foreground/30";
    }
  };

  const getHistoryStatusStyle = (status: string) => {
    switch (status) {
      case "aceito":
        return "bg-status-ok-soft text-status-ok border-status-ok/30";
      case "aguardando":
        return "bg-status-warning-soft text-status-warning border-status-warning/30";
      case "redirecionado":
        return "bg-status-critical-soft text-status-critical border-status-critical/30";
      default:
        return "bg-muted text-foreground border-border";
    }
  };

  const todayStart = new Date();
  todayStart.setHours(0, 0, 0, 0);

  const historyHoje = historyData.filter((h) => {
    const d = new Date(h.time.replace(/(\d{2})\/(\d{2})\/(\d{4})/, "$2/$1/$3"));
    return d >= todayStart || h.time.includes(new Intl.DateTimeFormat('pt-BR').format(todayStart));
  });

  const redirecionadosHojeCount = historyHoje.filter((h) => h.status === "redirecionado").length;
  const leadsHojeReal = historyHoje.length > 0 ? historyHoje.length : leadsHoje;
  const pctRedirecionados = leadsHojeReal > 0 ? Math.round((redirecionadosHojeCount / leadsHojeReal) * 100) : 0;
  const slaMedioHoje = leadsHojeReal > 0 ? "1m 15s" : "0s";

  return (
    <div className="flex flex-col h-full bg-muted/50 font-sans relative overflow-hidden">
      <div className="flex-none px-6 lg:px-8 py-6 border-b border-border bg-card sticky top-0 z-10">
        <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-3xl font-bold text-foreground tracking-tight">Roleta de Leads</h1>
              <span
                className={clsx(
                  "px-2.5 py-1 text-xs font-bold rounded-full border flex items-center gap-1.5",
                  isRouletteEnabled ? "bg-status-ok-soft text-status-ok border-status-ok/30" : "bg-muted text-muted-foreground border-border",
                )}
              >
                <span className={clsx("w-2 h-2 rounded-full", isRouletteEnabled ? "bg-status-ok" : "bg-muted-foreground/30")}></span>
                {isRouletteEnabled ? "Roleta Ativa" : "Roleta Pausada"}
              </span>
            </div>
            <p className="text-sm text-muted-foreground mt-1">
              Distribuição automática, qualificação inteligente e balanceamento de carga entre corretores
            </p>
          </div>
          <Button
            variant={isRouletteEnabled ? "outline" : "default"}
            onClick={handleToggleRoulette}
            loading={isRouletteMutationPending}
            loadingText="Salvando..."
          >
            {isRouletteEnabled ? <Pause className="w-4 h-4" /> : <Play className="w-4 h-4" />}
            {isRouletteEnabled ? "Pausar Distribuição" : "Ativar Distribuição"}
          </Button>
          <RouletteUnassignedAlertDialog
            open={unassignedDialogOpen}
            count={unassignedCount}
            isLoading={unassignedDialogLoading}
            onAssignAutomatically={handleEnableAssignAutomatically}
            onKeepManual={handleEnableKeepManual}
            onCancel={handleUnassignedDialogCancel}
          />
          <BrokerCapabilitiesPanel
            open={capDialogOpen}
            onClose={() => {
              setCapDialogOpen(false);
              setCapBrokerId(null);
            }}
            broker={capBrokerId ? brokerDetails[capBrokerId] ?? null : null}
            brokerSummary={
              selectedBrokerSummary
                ? {
                    status: selectedBrokerSummary.status,
                    slaPaused: selectedBrokerSummary.slaPaused,
                    load: selectedBrokerSummary.load,
                    maxLoad: selectedBrokerSummary.maxLoad,
                    lastAssigned: selectedBrokerSummary.lastAssigned,
                  }
                : null
            }
            stagePoolsMap={stageBrokerPoolsMap}
            onToggleRouletteStatus={selectedBrokerSummary ? () => toggleBrokerStatus(selectedBrokerSummary.id, selectedBrokerSummary.status) : undefined}
            toggling={selectedBrokerSummary ? togglingAgents.has(selectedBrokerSummary.id) : false}
            onOpenPoolsConfig={() => {
              setActiveTab("config");
              setConfigTab("pools");
              setCapDialogOpen(false);
            }}
            specialtyOptions={brokerCapabilityOptions.specialties}
            regionOptions={brokerCapabilityOptions.regions}
            onSaved={() => {
              loadBrokerDetails();
              refreshBrokers();
            }}
          />
        </div>

        <div role="tablist" aria-label="Seções da roleta" className="flex items-center gap-6 mt-8 border-b border-border overflow-x-auto no-scrollbar">
          {canViewRoulette && (
            <button
              role="tab"
              aria-selected={activeTab === "aovivo"}
              onClick={() => setActiveTab("aovivo")}
              className={clsx(
                "pb-3 text-sm font-bold border-b-2 transition-colors flex items-center gap-2 whitespace-nowrap",
                activeTab === "aovivo" ? "border-status-critical text-status-critical" : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              <Activity className={clsx("w-4 h-4", isRouletteEnabled && "animate-pulse")} aria-hidden="true" /> Monitor Ao Vivo
            </button>
          )}
          {canViewRoulette && (
            <button
              role="tab"
              aria-selected={activeTab === "geral"}
              onClick={() => setActiveTab("geral")}
              className={clsx(
                "pb-3 text-sm font-bold border-b-2 transition-colors flex items-center gap-2",
                activeTab === "geral" ? "border-primary-600 text-primary-600" : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              <Users className="w-4 h-4" aria-hidden="true" /> Corretores & Fila
            </button>
          )}
          {canConfigureRoulette && (
            <button
              role="tab"
              aria-selected={activeTab === "config"}
              onClick={() => setActiveTab("config")}
              className={clsx(
                "pb-3 text-sm font-bold border-b-2 transition-colors flex items-center gap-2",
                activeTab === "config" ? "border-primary-600 text-primary-600" : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              <Settings className="w-4 h-4" aria-hidden="true" /> Configuração
            </button>
          )}
          {canConfigureRoulette && (
            <button
              role="tab"
              aria-selected={activeTab === "qualificacao"}
              onClick={() => setActiveTab("qualificacao")}
              className={clsx(
                "pb-3 text-sm font-bold border-b-2 transition-colors flex items-center gap-2",
                activeTab === "qualificacao" ? "border-primary-600 text-primary-600" : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              <ListPlus className="w-4 h-4" aria-hidden="true" /> Qualificação (Forms)
            </button>
          )}
          {canViewAudit && (
            <button
              role="tab"
              aria-selected={activeTab === "historico"}
              onClick={() => setActiveTab("historico")}
              className={clsx(
                "pb-3 text-sm font-bold border-b-2 transition-colors flex items-center gap-2",
                activeTab === "historico" ? "border-primary-600 text-primary-600" : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              <History className="w-4 h-4" aria-hidden="true" /> Histórico de Envios
            </button>
          )}
          {canConfigureRoulette && (
            <button
              role="tab"
              aria-selected={activeTab === "ferramentas"}
              onClick={() => setActiveTab("ferramentas")}
              className={clsx(
                "pb-3 text-sm font-bold border-b-2 transition-colors flex items-center gap-2 whitespace-nowrap",
                activeTab === "ferramentas" ? "border-primary-600 text-primary-600" : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              <Wrench className="w-4 h-4" aria-hidden="true" /> Ferramentas
            </button>
          )}
        </div>
      </div>

      {configWarnings.length > 0 && (
        <div role="alert" aria-live="polite" className="mx-6 lg:mx-8 mt-4 rounded-xl border border-status-warning/30 bg-status-warning-soft p-4">
          <h4 className="text-sm font-bold text-status-warning mb-2 flex items-center gap-2">
            <AlertTriangle className="w-4 h-4" aria-hidden="true" />
            Atenção na configuração da roleta
          </h4>
          <ul className="space-y-1.5 text-sm text-status-warning/90 list-disc list-inside">
            {configWarnings.map((msg, i) => (
              <li key={i}>{msg}</li>
            ))}
          </ul>
        </div>
      )}

      <div className={clsx("flex-1 p-6 lg:p-8", activeTab === "aovivo" ? "flex flex-col overflow-hidden" : "overflow-y-auto")}>
        <AnimatePresence mode="wait">
          {activeTab === "aovivo" && (
            <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -10 }} className="h-full flex flex-col flex-1 min-h-0">
              <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 h-full min-h-0">
                <div className="lg:col-span-2 flex flex-col h-full bg-muted/50 overflow-hidden">
                  <div className="flex items-center justify-between mb-4 flex-none">
                    <h3 className="text-lg font-bold text-foreground flex items-center gap-2">
                      <RefreshCw className={clsx("w-5 h-5 text-primary-600", isRouletteEnabled && "animate-spin")} /> Roleta de Distribuição
                    </h3>
                    <div className="flex items-center gap-3">
                      <div className="flex items-center bg-card border border-border rounded-lg p-0.5 shadow-sm">
                        <button
                          type="button"
                          onClick={() => setLiveViewMode("cards")}
                          className={clsx(
                            "p-1.5 rounded-md transition-colors",
                            liveViewMode === "cards" ? "bg-muted text-primary-600" : "text-muted-foreground hover:text-muted-foreground",
                          )}
                          title="Visualização em Cards"
                        >
                          <LayoutGrid className="w-4 h-4" />
                        </button>
                        <button
                          type="button"
                          onClick={() => setLiveViewMode("list")}
                          className={clsx(
                            "p-1.5 rounded-md transition-colors",
                            liveViewMode === "list" ? "bg-muted text-primary-600" : "text-muted-foreground hover:text-muted-foreground",
                          )}
                          title="Visualização em Lista"
                        >
                          <List className="w-4 h-4" />
                        </button>
                      </div>
                      <span className="hidden sm:inline-block text-xs font-medium text-muted-foreground bg-card px-3 py-1.5 border border-border rounded-lg shadow-sm">
                        Apenas corretores online
                      </span>
                    </div>
                  </div>

                  <div className={clsx("flex flex-col relative w-full overflow-y-auto pr-2 flex-1 pb-4", liveViewMode === "cards" ? "gap-4" : "gap-2")}>
                    {(() => {
                      const online = brokers.filter((b) => b.status === "online");
                      const byId = new Map(online.map((b) => [b.id, b]));
                      const stageKey = String(queueStage || "NEW").toUpperCase();
                      const pool = (stageBrokerPoolsMap[stageKey]?.length ? stageBrokerPoolsMap[stageKey] : activeRouletteAgents) || [];
                      const start = stageLastIndexMap[stageKey] ?? rouletteLastIndex;
                      const len = pool.length;
                      const offset = len > 0 ? (start >= 0 ? (start + 1) % len : 0) : 0;
                      const rotated = len > 0 ? [...pool.slice(offset), ...pool.slice(0, offset)] : [];
                      const ordered: any[] = [];
                      const used = new Set<string>();
                      for (const id of rotated) {
                        const b = byId.get(id);
                        if (!b) continue;
                        ordered.push(b);
                        used.add(id);
                      }
                      for (const b of online) {
                        if (!used.has(b.id)) ordered.push(b);
                      }
                      return ordered;
                    })().map((broker, index) => (
                      <motion.div
                        key={broker.id}
                        layout
                        initial={{ opacity: 0, scale: 0.9 }}
                        animate={{ opacity: 1, scale: 1 }}
                        transition={{ type: "spring", stiffness: 300, damping: 25, mass: 1 }}
                        className={clsx(
                          "bg-card flex z-10",
                          liveViewMode === "cards"
                            ? "p-4 rounded-2xl flex-col sm:flex-row items-start sm:items-center justify-between gap-4"
                            : "p-3 rounded-xl items-center justify-between gap-3",
                          index === 0
                            ? liveViewMode === "cards"
                              ? "border-2 border-primary-500 shadow-md ring-4 ring-primary-500/10"
                              : "border-l-4 border-primary-500 shadow-sm border-t border-r border-b border-y-slate-200 border-r-slate-200"
                            : "border border-border shadow-sm opacity-90",
                        )}
                      >
                        <div className="flex items-center gap-4 flex-1 w-full min-w-0">
                          <div
                            className={clsx(
                              "rounded-full flex items-center justify-center font-bold text-sm flex-shrink-0",
                              liveViewMode === "cards" ? "w-10 h-10" : "w-8 h-8 text-xs",
                              index === 0 ? "bg-primary-600 text-white" : "bg-muted text-muted-foreground",
                            )}
                          >
                            {index + 1}º
                          </div>

                          <div className="flex items-center gap-3 min-w-0">
                            {broker.avatar ? (
                              <div className={clsx("rounded-full overflow-hidden flex-shrink-0 border border-border shadow-sm", liveViewMode === "cards" ? "w-10 h-10" : "w-8 h-8")}>
                                <MediaImage src={broker.avatar} alt={broker.name} className="w-full h-full object-cover shrink-0" />
                              </div>
                            ) : (
                              <div
                                className={clsx(
                                  "rounded-full bg-primary-100 flex items-center justify-center text-primary-700 font-bold flex-shrink-0",
                                  liveViewMode === "cards" ? "w-10 h-10" : "w-8 h-8 text-xs",
                                )}
                              >
                                {broker.name
                                  .split(" ")
                                  .map((n: string) => n[0])
                                  .join("")}
                              </div>
                            )}
                            <div className="min-w-0">
                              <div className={clsx("font-bold text-foreground flex items-center gap-2 truncate", liveViewMode === "list" && "text-sm")}>
                                {broker.name}
                                {index === 0 && (
                                  <span
                                    className={clsx(
                                      "bg-primary-100 text-primary-700 rounded-full uppercase tracking-wider font-bold",
                                      liveViewMode === "cards" ? "px-2 py-0.5 text-micro" : "px-1.5 py-0.5 text-caption",
                                    )}
                                  >
                                    Próximo
                                  </span>
                                )}
                              </div>
                              <div className="text-xs text-muted-foreground mt-0.5 flex flex-wrap gap-1">
                                {broker.specialty
                                  .slice(0, liveViewMode === "list" ? 1 : undefined)
                                  .map((s: string) => (
                                    <span key={s} className="bg-muted px-1.5 py-0.5 rounded text-micro truncate">
                                      {s}
                                    </span>
                                  ))}
                                {liveViewMode === "list" && broker.specialty.length > 1 && <span className="text-micro text-muted-foreground">+{broker.specialty.length - 1}</span>}
                              </div>
                            </div>
                          </div>
                        </div>

                        <div className={clsx("bg-muted border border-border flex-shrink-0", liveViewMode === "cards" ? "w-full sm:w-48 p-3 rounded-xl" : "w-32 p-2 rounded-lg ml-auto")}>
                          <div className={clsx("flex items-center justify-between mb-1.5", liveViewMode === "cards" ? "text-xs" : "text-micro")}>
                            <span className="font-bold text-foreground">{liveViewMode === "cards" ? "Carga Atual" : "Carga"}</span>
                            <span className="text-muted-foreground font-medium">
                              {broker.load}/{broker.maxLoad}
                            </span>
                          </div>
                          <div className="w-full bg-muted rounded-full h-1.5 overflow-hidden">
                            <motion.div
                              className={clsx("h-full rounded-full", broker.maxLoad > 0 && broker.load / broker.maxLoad > 0.8 ? "bg-status-warning" : "bg-primary-500")}
                              initial={{ width: 0 }}
                              animate={{ width: `${broker.maxLoad > 0 ? (broker.load / broker.maxLoad) * 100 : 0}%` }}
                              transition={{ duration: 0.5 }}
                            />
                          </div>
                        </div>
                      </motion.div>
                    ))}
                  </div>
                </div>

                <div className="flex flex-col h-full bg-card rounded-2xl border border-border shadow-sm p-5 relative overflow-hidden min-h-0">
                  <h3 className="text-sm font-bold text-foreground mb-4 flex items-center gap-2 uppercase tracking-wider flex-none">
                    <Activity className="w-4 h-4 text-status-ok" /> Últimos Leads
                    <span
                      className={clsx(
                        "text-xs px-2 py-0.5 rounded-full ml-2 border",
                        isWsConnected ? "bg-status-ok-soft text-status-ok border-status-ok/30" : "bg-muted text-muted-foreground border-border",
                      )}
                    >
                      {isWsConnected ? "Ao vivo" : "Reconectando"}
                    </span>
                  </h3>

                  <div className="flex flex-col gap-3 relative flex-1 overflow-y-auto pr-1 pb-4">
                    <AnimatePresence>
                      {liveFeed.length === 0 ? (
                        <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} className="flex flex-col items-center justify-center h-40 text-center">
                          <div className="w-12 h-12 bg-muted rounded-full flex items-center justify-center mb-3">
                            <Activity className="w-6 h-6 text-muted-foreground animate-pulse" />
                          </div>
                          <p className="text-sm font-medium text-muted-foreground">Aguardando novos leads entrarem na roleta...</p>
                        </motion.div>
                      ) : (
                        liveFeed.map((lead, i) => (
                          <motion.div
                            key={lead.uniqueKey}
                            layout
                            initial={{ opacity: 0, x: 20, y: -20, scale: 0.95 }}
                            animate={{ opacity: 1, x: 0, y: 0, scale: 1 }}
                            exit={{ opacity: 0, scale: 0.9, transition: { duration: 0.2 } }}
                            transition={{ type: "spring", stiffness: 400, damping: 25 }}
                            className={clsx("bg-card border shadow-sm relative overflow-hidden rounded-xl flex-shrink-0", i === 0 ? "border-status-ok/40 ring-1 ring-emerald-100" : "border-border")}
                          >
                            {i === 0 && <div className="absolute top-0 left-0 w-1 h-full bg-status-ok rounded-l-xl" />}

                            <div className="p-4 pl-5">
                              <div className="flex justify-between items-center mb-2">
                                <span className="text-caption font-bold text-muted-foreground font-mono">{lead.id}</span>
                                <span className="text-caption font-medium text-muted-foreground bg-muted px-2 py-0.5 rounded-md">{lead.time}</span>
                              </div>
                              <p className="text-sm font-bold text-foreground truncate">{lead.name}</p>
                              <p className="text-xs text-muted-foreground mt-0.5">{lead.intent}</p>
                              <div className="mt-3 pt-2.5 border-t border-border flex items-center gap-2">
                                <ArrowRight className="w-3.5 h-3.5 text-status-ok flex-shrink-0" />
                                <span className="text-caption font-bold text-status-ok truncate">Enviado p/ {lead.broker}</span>
                              </div>
                            </div>
                          </motion.div>
                        ))
                      )}
                    </AnimatePresence>
                  </div>
                </div>
              </div>
            </motion.div>
          )}

          {activeTab === "geral" && (
            <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -10 }} className="space-y-6">
              {!focusMode && (
                <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
                  <div className="bg-card p-6 rounded-2xl border border-border shadow-sm flex flex-col gap-4">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-full bg-status-ok-soft flex items-center justify-center">
                        <UserCheck className="w-4 h-4 text-status-ok" />
                      </div>
                      <span className="text-sm font-medium text-muted-foreground">Corretores Online</span>
                    </div>
                    <h2 className="text-3xl font-bold text-foreground">
                      {onlineBrokersCount} <span className="text-sm font-medium text-muted-foreground">/ {activeBrokersCount} ativos</span>
                    </h2>
                  </div>

                  <div className="bg-card p-6 rounded-2xl border border-border shadow-sm flex flex-col gap-4">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-full bg-primary-50 flex items-center justify-center">
                        <Activity className="w-4 h-4 text-primary-600" />
                      </div>
                      <span className="text-sm font-medium text-muted-foreground">Leads Hoje</span>
                    </div>
                    <h2 className="text-3xl font-bold text-foreground">{leadsHojeReal}</h2>
                  </div>

                  <div className="bg-card p-6 rounded-2xl border border-border shadow-sm flex flex-col gap-4">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-full bg-status-warning-soft flex items-center justify-center">
                        <Clock className="w-4 h-4 text-status-warning" />
                      </div>
                      <span className="text-sm font-medium text-muted-foreground">Tempo Médio (SLA)</span>
                    </div>
                    <h2 className="text-3xl font-bold text-foreground">{slaMedioHoje}</h2>
                  </div>

                  <div className="bg-card p-6 rounded-2xl border border-border shadow-sm flex flex-col gap-4">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-full bg-status-critical-soft flex items-center justify-center">
                        <RefreshCw className="w-4 h-4 text-status-critical" />
                      </div>
                      <span className="text-sm font-medium text-muted-foreground">Redirecionados (Hoje)</span>
                    </div>
                    <h2 className="text-3xl font-bold text-foreground">
                      {redirecionadosHojeCount} <span className="text-sm font-medium text-muted-foreground">({pctRedirecionados}%)</span>
                    </h2>
                  </div>
                </div>
              )}

              <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
                <div className="p-5 border-b border-border bg-muted/50 flex items-center justify-between">
                  <h3 className="text-base font-bold text-foreground flex items-center gap-2">
                    <Users className="w-5 h-5 text-primary-600" /> Fila e Balanceamento de Carga
                  </h3>
                  <Button variant="link" size="sm" className="h-auto p-0">
                    Adicionar Corretor à Roleta
                  </Button>
                </div>
                <div className="p-4 border-b border-border bg-card">
                  <div className="flex flex-col lg:flex-row lg:items-center gap-3">
                    <div className="relative flex-1">
                      <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
                      <input
                        value={brokersQuery}
                        onChange={(e) => setBrokersQuery(e.target.value)}
                        placeholder="Buscar por nome, região, especialidade ou ID..."
                        className="w-full pl-9 pr-3 py-2 rounded-xl border border-border text-sm bg-card outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
                      />
                    </div>
                    <div className="flex gap-2">
                      <select
                        value={brokersStatusFilter}
                        onChange={(e) => setBrokersStatusFilter(e.target.value as any)}
                        className="px-3 py-2 rounded-xl border border-border text-sm bg-card outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
                      >
                        <option value="all">Todos</option>
                        <option value="online">Online</option>
                        <option value="paused">Em pausa</option>
                        <option value="slaPaused">Pausado (SLA)</option>
                      </select>
                      <select
                        value={brokersRoleFilter}
                        onChange={(e) => setBrokersRoleFilter(e.target.value)}
                        className="px-3 py-2 rounded-xl border border-border text-sm bg-card outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
                      >
                        <option value="all">Papel: Todos</option>
                        {brokerRoleOptions.map((o) => (
                          <option key={o.role} value={o.role}>
                            {roleLabel(o.role)} ({o.count})
                          </option>
                        ))}
                      </select>
                      <select
                        value={brokersSort}
                        onChange={(e) => setBrokersSort(e.target.value as any)}
                        className="px-3 py-2 rounded-xl border border-border text-sm bg-card outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
                      >
                        <option value="queue">Ordenar: Fila</option>
                        <option value="name">Ordenar: Nome</option>
                        <option value="load">Ordenar: Carga</option>
                        <option value="last">Ordenar: Último lead</option>
                      </select>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => {
                          setBrokersQuery("");
                          setBrokersStatusFilter("all");
                          setBrokersRoleFilter("all");
                          setBrokersSort("queue");
                        }}
                      >
                        Limpar
                      </Button>
                    </div>
                  </div>
                  <div className="mt-3 text-xs text-muted-foreground">
                    Mostrando <span className="font-bold text-foreground">{filteredBrokers.length}</span> de{" "}
                    <span className="font-bold text-foreground">{brokers.length}</span>
                  </div>
                </div>
                <div className="overflow-x-auto">
                  <table className="w-full text-left border-collapse">
                    <thead>
                      <tr className="border-b border-border text-muted-foreground text-xs font-medium tracking-wide">
                        <th className="px-6 py-4">Corretor</th>
                        <th className="px-6 py-4">Especialidade / Região</th>
                        <th className="px-6 py-4">Carga de Atendimento</th>
                        <th className="px-6 py-4">Último Lead</th>
                        <th className="px-6 py-4 text-right">Ação</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-border">
                      {filteredBrokers.map((broker) => (
                        <tr key={broker.id} className="hover:bg-muted/80 transition-colors">
                          <td className="px-6 py-4">
                            <div className="flex items-center gap-3">
                              <div className="relative">
                                {broker.avatar ? (
                                  <div className="w-10 h-10 rounded-full overflow-hidden border border-border shadow-sm">
                                    <MediaImage src={broker.avatar} alt={broker.name} className="w-full h-full object-cover" />
                                  </div>
                                ) : (
                                  <div className="w-10 h-10 rounded-full bg-primary-100 flex items-center justify-center text-primary-700 font-bold text-sm">
                                    {broker.name
                                      .split(" ")
                                      .map((n) => n[0])
                                      .join("")}
                                  </div>
                                )}
                                <span className={clsx("absolute bottom-0 right-0 w-3 h-3 rounded-full border-2 border-card", getStatusColor(broker.status))}></span>
                              </div>
                              <div>
                                <div className="text-sm font-bold text-foreground">{broker.name}</div>
                                <div className="text-xs text-muted-foreground capitalize">
                                  {broker.slaPaused ? "Pausado (SLA)" : broker.status === "paused" ? "Em Pausa" : broker.status}
                                </div>
                              </div>
                            </div>
                          </td>
                          <td className="px-6 py-4">
                            <div className="flex flex-wrap gap-1 mb-1">
                              {broker.specialty.map((spec, i) => (
                                <span key={i} className="px-2 py-0.5 bg-muted text-muted-foreground text-micro font-bold rounded">
                                  {spec}
                                </span>
                              ))}
                            </div>
                            <div className="text-micro text-muted-foreground font-medium flex items-center gap-1">
                              <Building2 className="w-3 h-3" /> {broker.region}
                            </div>
                          </td>
                          <td className="px-6 py-4">
                            {(() => {
                              const max = broker.maxLoad > 0 ? broker.maxLoad : 0;
                              const pct = max > 0 ? Math.min(100, (broker.load / max) * 100) : 0;
                              return (
                                <>
                                  <div className="flex items-center justify-between text-xs mb-1">
                                    {!focusMode ? (
                                      <span className="font-bold text-foreground">{broker.load} leads ativos</span>
                                    ) : (
                                      <span className="font-bold text-muted-foreground select-none">Oculto</span>
                                    )}
                                    <span className="text-muted-foreground">Máx {max > 0 ? max : "∞"}</span>
                                  </div>
                                  <div className="w-full bg-muted rounded-full h-1.5 overflow-hidden">
                                    <div
                                      className={clsx("h-full rounded-full transition-all", max > 0 && broker.load / max > 0.8 ? "bg-status-warning" : "bg-primary-500")}
                                      style={{ width: `${pct}%` }}
                                    ></div>
                                  </div>
                                </>
                              );
                            })()}
                          </td>
                          <td className="px-6 py-4 text-sm text-muted-foreground">{broker.lastAssigned}</td>
                          <td className="px-6 py-4 text-right">
                            <div className="flex items-center justify-end gap-2">
                              <Button
                                variant="outline"
                                size="icon"
                                className="size-9"
                                onClick={() => {
                                  setCapBrokerId(broker.id);
                                  setCapDialogOpen(true);
                                }}
                                aria-label="Editar capacidade"
                              >
                                <Settings className="w-4 h-4" />
                              </Button>
                              <Button
                                variant="outline"
                                size="sm"
                                disabled={togglingAgents.has(broker.id)}
                                onClick={() => toggleBrokerStatus(broker.id, broker.status)}
                                className={clsx(
                                  broker.status === "paused" &&
                                    "bg-primary-50 border-primary-200 text-primary-700 hover:bg-primary-100 hover:text-primary-700",
                                )}
                              >
                                {togglingAgents.has(broker.id) ? (
                                  <Loader2 className="w-4 h-4 animate-spin" />
                                ) : broker.status === "paused" ? (
                                  "Retomar"
                                ) : (
                                  "Pausar"
                                )}
                              </Button>
                            </div>
                          </td>
                        </tr>
                      ))}
                      {filteredBrokers.length === 0 && (
                        <tr>
                          <td colSpan={5} className="px-6 py-10 text-sm text-muted-foreground text-center">
                            Nenhum corretor encontrado com os filtros atuais.
                          </td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                </div>
              </div>
            </motion.div>
          )}

          {activeTab === "config" && (
            <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -10 }} className="space-y-6 max-w-6xl">
              <div className="flex items-center gap-2 bg-card border border-border rounded-xl p-1 w-fit">
                <button
                  type="button"
                  onClick={() => setConfigTab("regras")}
                  className={clsx(
                    "px-4 py-2 rounded-lg text-sm font-bold transition-colors",
                    configTab === "regras" ? "bg-primary-600 text-white" : "text-muted-foreground hover:text-foreground hover:bg-muted",
                  )}
                >
                  Regras & SLA
                </button>
                <button
                  type="button"
                  onClick={() => setConfigTab("pools")}
                  className={clsx(
                    "px-4 py-2 rounded-lg text-sm font-bold transition-colors",
                    configTab === "pools" ? "bg-primary-600 text-white" : "text-muted-foreground hover:text-foreground hover:bg-muted",
                  )}
                >
                  Pools por etapa
                </button>
              </div>

              {configTab === "regras" && <SLARulesConfig />}

              {configTab === "pools" && (
                <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
                  <div className="p-4 border-b border-border bg-muted/50">
                    <h3 className="text-base font-bold text-foreground">Pools por etapa (esteira)</h3>
                    <p className="text-xs text-muted-foreground mt-1">
                      Defina quais corretores concorrem em cada etapa (por status do lead). Se uma etapa não estiver configurada, cai no pool padrão (active_brokers).
                    </p>
                  </div>
                  <div className="p-4 space-y-3">
                    <div className="flex flex-col lg:flex-row gap-3 lg:items-end">
                      <div className="flex-1">
                        <label className="block text-xs font-bold text-muted-foreground mb-1">Etapa (status do lead)</label>
                        <select
                          value={newStage}
                          onChange={(e) => setNewStage(e.target.value)}
                          className="w-full px-3 py-2 rounded-xl border border-border text-sm bg-card outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
                        >
                          {LEAD_STAGES.map((stage) => (
                            <option key={stage.value} value={stage.value}>
                              {stage.label} ({stage.value})
                            </option>
                          ))}
                        </select>
                      </div>
                      <div className="flex gap-2">
                        <Button
                          variant="secondary"
                          onClick={() => {
                            const stage = newStage.trim().toUpperCase();
                            if (!stage) return;
                            if (stagePools.some((r) => r.stage === stage)) {
                              setOpenStage(stage);
                              return;
                            }
                            setStagePools((prev) => [...prev, { stage, brokerIds: [] }].sort((a, b) => a.stage.localeCompare(b.stage)));
                            setOpenStage(stage);
                          }}
                        >
                          Adicionar etapa
                        </Button>
                        <Button
                          onClick={async () => {
                            try {
                              setStagePoolsError("");
                              const obj: Record<string, string[]> = {};
                              stagePools.forEach((r) => {
                                const key = r.stage.trim().toUpperCase();
                                const ids = (r.brokerIds || []).filter(Boolean);
                                if (key && ids.length > 0) obj[key] = ids;
                              });
                              await leadService.updateRouletteConfig({ stage_broker_pools: obj });
                              setStageBrokerPoolsMap(obj);
                            } catch {
                              setStagePoolsError("Não foi possível salvar os pools. Verifique sua conexão e tente novamente.");
                            }
                          }}
                        >
                          Salvar
                        </Button>
                        <Button
                          variant="outline"
                          onClick={async () => {
                            try {
                              const cfg = await leadService.getRouletteConfig();
                              setRouletteLastIndex(typeof (cfg as any)?.last_index === "number" ? (cfg as any).last_index : -1);
                              const pools = (cfg as any)?.stage_broker_pools || {};
                              const mappedObj: Record<string, string[]> = {};
                              Object.entries(pools).forEach(([stage, brokerIds]) => {
                                const key = String(stage || "").toUpperCase();
                                const ids = Array.isArray(brokerIds) ? (brokerIds as any[]).filter(Boolean).map(String) : [];
                                if (key && ids.length > 0) mappedObj[key] = ids;
                              });
                              setStageBrokerPoolsMap(mappedObj);
                              const stageIdx = (cfg as any)?.stage_last_index || {};
                              const stageMapped: Record<string, number> = {};
                              Object.entries(stageIdx).forEach(([stage, idx]) => {
                                const key = String(stage || "").toUpperCase();
                                if (!key) return;
                                if (typeof idx === "number") stageMapped[key] = idx;
                              });
                              setStageLastIndexMap(stageMapped);
                              const rows = Object.entries(pools).map(([stage, brokerIds]) => ({
                                stage: String(stage || "").toUpperCase(),
                                brokerIds: Array.isArray(brokerIds) ? (brokerIds as any[]).filter(Boolean).map(String) : [],
                              }));
                              rows.sort((a, b) => a.stage.localeCompare(b.stage));
                              setStagePools(rows);
                              setStagePoolsError("");
                            } catch {
                            }
                          }}
                        >
                          Recarregar
                        </Button>
                      </div>
                    </div>

                    {stagePoolsError && <div className="text-xs font-bold text-status-critical">{stagePoolsError}</div>}

                    <div className="rounded-2xl border border-border overflow-hidden">
                      <div className="px-4 py-3 bg-card border-b border-border flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2">
                        <div className="text-xs font-bold text-muted-foreground">
                          Corretores disponíveis: <span className="text-foreground">{eligibleBrokers.length}</span>
                          <span className="text-muted-foreground font-medium"> (apenas os ativos na roleta)</span>
                        </div>
                      </div>

                      {stagePools.length === 0 ? (
                        <div className="p-6 text-sm text-muted-foreground">
                          Nenhuma etapa configurada. Adicione uma etapa acima para definir o pool específico.
                        </div>
                      ) : (
                        <div className="divide-y divide-border">
                          {stagePools.map((row) => {
                            const selected = row.brokerIds || [];
                            const selectedNames = selected
                              .map((id) => eligibleBrokers.find((b) => b.id === id)?.name || id)
                              .filter(Boolean);

                            return (
                              <div key={row.stage} className="p-4 flex flex-col lg:flex-row lg:items-center gap-3">
                                <div className="w-full lg:w-44">
                                  <div className="text-xs font-bold text-muted-foreground">Etapa</div>
                                  <div className="mt-1 inline-flex items-center gap-2">
                                    <span className="px-2.5 py-1 rounded-lg bg-secondary text-secondary-foreground text-xs font-bold">{row.stage}</span>
                                    <Button variant="outline" size="icon" className="size-8" onClick={() => removeStageRow(row.stage)} title="Remover etapa">
                                      <X className="w-4 h-4" />
                                    </Button>
                                  </div>
                                </div>

                                <div className="flex-1">
                                  <div className="text-xs font-bold text-muted-foreground">Corretores</div>
                                  <div className="mt-1 flex flex-wrap gap-2">
                                    {selectedNames.length === 0 ? (
                                      <span className="text-sm text-muted-foreground">Nenhum (vai cair no pool padrão)</span>
                                    ) : (
                                      selectedNames.slice(0, 6).map((name, idx) => (
                                        <span
                                          key={idx}
                                          className="px-2.5 py-1 rounded-full bg-primary-50 text-primary-700 text-xs font-bold border border-primary-100"
                                        >
                                          {name}
                                        </span>
                                      ))
                                    )}
                                    {selectedNames.length > 6 && (
                                      <span className="px-2.5 py-1 rounded-full bg-muted text-foreground text-xs font-bold border border-border">
                                        +{selectedNames.length - 6}
                                      </span>
                                    )}
                                  </div>
                                </div>

                                <div className="w-full lg:w-72">
                                  <Button
                                    variant="outline"
                                    onClick={() => {
                                      const isClosing = openStage === row.stage;
                                      if (!isClosing) setBrokerSearch("");
                                      setOpenStage(isClosing ? null : row.stage);
                                    }}
                                    className="w-full"
                                  >
                                    {openStage === row.stage ? "Fechar seleção" : "Selecionar corretores"}
                                  </Button>

                                  {openStage === row.stage && (
                                    <div className="mt-2 rounded-2xl border border-border bg-card shadow-sm overflow-hidden">
                                      <div className="max-h-56 overflow-auto p-2">
                                        <div className="px-2 pb-2 mb-2 border-b border-border sticky top-0 bg-card z-10">
                                          <input
                                            type="text"
                                            value={brokerSearch}
                                            onChange={(e) => setBrokerSearch(e.target.value)}
                                            placeholder="Buscar corretor para adicionar..."
                                            className="w-full px-3 py-1.5 rounded-lg border border-border text-xs focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                                            onClick={(e) => e.stopPropagation()}
                                          />
                                        </div>
                                        {(eligibleBrokersFiltered.length === 0 && brokerSearch === "" ? eligibleBrokers : eligibleBrokersFiltered).map((b) => (
                                          <label key={b.id} className="flex items-center gap-2 px-2 py-2 rounded-xl hover:bg-muted cursor-pointer">
                                            <input
                                              type="checkbox"
                                              checked={selected.includes(b.id)}
                                              onChange={() => toggleBrokerInStage(row.stage, b.id)}
                                              className="rounded border-border text-primary-600 focus:ring-primary-600"
                                            />
                                            <span className="text-sm font-bold text-foreground">{b.name}</span>
                                          </label>
                                        ))}
                                      </div>
                                    </div>
                                  )}
                                </div>
                              </div>
                            );
                          })}
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              )}
            </motion.div>
          )}

          {activeTab === "qualificacao" && (
            <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -10 }} className="max-w-5xl">
              <QualificationFormBuilder
                onSaved={(cfg: RouletteConfig) => {
                  setRouletteQualFields(Array.isArray(cfg?.qualFields) && cfg.qualFields.length > 0 ? (cfg.qualFields as QualField[]) : (DEFAULT_QUAL_FIELDS as QualField[]));
                  setRouletteQualRouting((cfg?.qualRouting as QualRoutingConfig) ?? null);
                }}
              />
            </motion.div>
          )}

          {activeTab === "ferramentas" && (
            <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -10 }} className="space-y-6 max-w-5xl">
              <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
                <div className="p-4 border-b border-border bg-muted/50">
                  <h3 className="text-base font-bold text-foreground">Simular distribuição (N leads)</h3>
                  <p className="text-xs text-muted-foreground mt-1">
                    Útil para validar pools e regras. Com <span className="font-mono">persist=true</span>, o ponteiro avança como se leads reais tivessem entrado.
                  </p>
                </div>
                <div className="p-4 space-y-3">
                  <div className="flex flex-col sm:flex-row gap-3">
                    <div className="flex-1">
                      <label className="block text-xs font-bold text-muted-foreground mb-1">Stage</label>
                      <select
                        value={simStage}
                        onChange={(e) => setSimStage(e.target.value)}
                        className="w-full px-3 py-2 rounded-xl border border-border text-sm bg-card outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
                      >
                        {LEAD_STAGES.map((stage) => (
                          <option key={stage.value} value={stage.value}>
                            {stage.label} ({stage.value})
                          </option>
                        ))}
                      </select>
                    </div>
                    <div className="w-full sm:w-40">
                      <label className="block text-xs font-bold text-muted-foreground mb-1">Quantidade</label>
                      <input
                        type="number"
                        min={1}
                        value={simCount}
                        onChange={(e) => setSimCount(parseInt(e.target.value || "1", 10))}
                        className="w-full px-3 py-2 rounded-xl border border-border text-sm"
                      />
                    </div>
                    <div className="w-full sm:w-40">
                      <label className="block text-xs font-bold text-muted-foreground mb-1">Seed (0 = aleatório)</label>
                      <input
                        type="number"
                        min={0}
                        value={simSeed}
                        onChange={(e) => setSimSeed(parseInt(e.target.value || "0", 10))}
                        className="w-full px-3 py-2 rounded-xl border border-border text-sm"
                      />
                    </div>
                    <div className="w-full sm:w-40 flex items-end">
                      <label className="flex items-center gap-2 text-sm font-bold text-foreground">
                        <input type="checkbox" checked={simPersist} onChange={(e) => setSimPersist(e.target.checked)} />
                        persist
                      </label>
                    </div>
                  </div>
                  <Button
                    variant="secondary"
                    onClick={async () => {
                      try {
                        const r = await leadService.simulateRoulette({
                          count: simCount,
                          stage: simStage,
                          persist: simPersist,
                          seed: simSeed > 0 ? simSeed : undefined,
                        });
                        setSimResult(r);
                      } catch {
                        setSimResult(null);
                      }
                    }}
                  >
                    Rodar simulação
                  </Button>
                  {simResult && (
                    <div className="mt-4 p-5 border border-primary-200 bg-primary-50/30 rounded-2xl">
                      <h4 className="text-sm font-bold text-primary-900 mb-4 flex items-center gap-2">
                        <Activity className="w-4 h-4" /> Resultado da Simulação ({simResult.count} leads)
                      </h4>
                      <div className="space-y-4">
                        {simResult.assignments?.map((item: any, idx: number) => {
                          const brokerName = eligibleBrokers.find((b) => b.id === item.broker_id)?.name || item.broker_id;
                          return (
                            <motion.div initial={{ opacity: 0, x: -10 }} animate={{ opacity: 1, x: 0 }} transition={{ delay: idx * 0.1 }} key={idx} className="flex items-start gap-4">
                              <div className="flex flex-col items-center mt-1">
                                <div className="w-6 h-6 rounded-full bg-primary-100 text-primary-700 flex items-center justify-center text-xs font-bold border border-primary-200">
                                  {item.n}
                                </div>
                                {idx < simResult.assignments.length - 1 && <div className="w-px h-10 bg-primary-200 my-1"></div>}
                              </div>
                              <div className="flex-1 bg-card border border-border p-3 rounded-xl shadow-sm">
                                <div className="flex items-center justify-between">
                                  <div>
                                    <span className="text-xs text-muted-foreground font-medium block">Atribuído para</span>
                                    <span className="text-sm font-bold text-foreground flex items-center gap-2 mt-0.5">
                                      <Users className="w-4 h-4 text-primary-600" /> {brokerName}
                                    </span>
                                  </div>
                                  <div className="text-right">
                                    <span className="text-micro uppercase tracking-wider font-bold text-status-ok bg-status-ok-soft px-2 py-1 rounded-full border border-status-ok/30">
                                      {simResult.stage || "GERAL"}
                                    </span>
                                  </div>
                                </div>
                              </div>
                            </motion.div>
                          );
                        })}
                        {(!simResult.assignments || simResult.assignments.length === 0) && (
                          <div className="text-sm text-muted-foreground p-4 bg-card rounded-xl border border-border text-center">
                            Nenhum corretor elegível encontrado para esta configuração.
                          </div>
                        )}
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </motion.div>
          )}

          {activeTab === "historico" && (
            <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -10 }} className="space-y-6">
              <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden flex flex-col">
                <div className="p-4 border-b border-border bg-muted/50 flex items-center justify-between">
                  <h3 className="text-base font-bold text-foreground flex items-center gap-2">
                    <History className="w-5 h-5 text-primary-600" /> Log de Distribuições Recentes
                  </h3>
                  <Button variant="outline" size="sm">
                    Filtrar
                  </Button>
                </div>

                <div className="overflow-x-auto">
                  <table className="w-full text-left border-collapse min-w-[800px]">
                    <thead>
                      <tr className="border-b border-border text-muted-foreground text-xs font-medium tracking-wide">
                        <th className="px-6 py-4">ID Lead</th>
                        <th className="px-6 py-4">Intenção / Qualificação</th>
                        <th className="px-6 py-4">Corretor Atribuído</th>
                        <th className="px-6 py-4">Horário / SLA</th>
                        <th className="px-6 py-4">Status</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-border">
                      {historyData.map((item, index) => (
                        <tr key={index} className="hover:bg-muted/80 transition-colors">
                          <td className="px-6 py-4">
                            <span className="text-sm font-bold text-foreground">{item.lead}</span>
                            <div className="text-xs text-muted-foreground font-mono mt-0.5">{item.id}</div>
                          </td>
                          <td className="px-6 py-4">
                            <span className="text-sm text-foreground">{item.intent}</span>
                          </td>
                          <td className="px-6 py-4">
                            <div className="flex items-center gap-2">
                              <span className="text-sm font-bold text-foreground">{item.broker}</span>
                              {item.status === "redirecionado" && (
                                <span title="Foi redirecionado para outro corretor">
                                  <ArrowRight className="w-4 h-4 text-status-critical" />
                                </span>
                              )}
                            </div>
                          </td>
                          <td className="px-6 py-4">
                            <span className="block text-sm font-medium text-foreground">{item.time}</span>
                            <span className={clsx("block text-micro font-bold mt-0.5", item.sla.includes("Expirado") ? "text-status-critical" : "text-muted-foreground")}>
                              SLA: {item.sla}
                            </span>
                          </td>
                          <td className="px-6 py-4">
                            <span className={clsx("px-2.5 py-1 text-caption font-bold rounded-full uppercase tracking-wider", getHistoryStatusStyle(item.status))}>
                              {item.status}
                            </span>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>

              {canViewAudit && (
                <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden flex flex-col">
                  <div className="p-4 border-b border-border bg-muted/50 space-y-3">
                    <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-3">
                      <div>
                        <h3 className="text-base font-bold text-foreground flex items-center gap-2">
                          <Shield className="w-5 h-5 text-primary-600" aria-hidden="true" /> Auditoria
                        </h3>
                        <p className="text-xs text-muted-foreground mt-1">
                          Mudanças de configuração e eventos relevantes (atribuições, pausas e escalonamentos).
                        </p>
                      </div>
                      <div className="flex flex-wrap items-center gap-2">
                        <Button type="button" variant="outline" size="sm" onClick={() => auditQuery.refetch()} aria-label="Recarregar auditoria">
                          Recarregar
                        </Button>
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={auditDownloadCSV}
                          disabled={(auditQuery.data?.items ?? []).length === 0}
                          aria-label="Exportar auditoria para CSV"
                          className="border-primary-200 bg-primary-50 text-primary-700 hover:bg-primary-100 hover:text-primary-700"
                        >
                          Exportar CSV
                        </Button>
                      </div>
                    </div>
                    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-2">
                      <label className="text-xs font-bold text-muted-foreground flex flex-col gap-1">
                        Ação
                        <select
                          value={auditAction}
                          onChange={(e) => setAuditAction(e.target.value)}
                          className="px-3 py-2 rounded-lg border border-border bg-card text-xs font-medium text-foreground hover:bg-muted transition-colors"
                        >
                          <option value="">Todas</option>
                          <option value="config.update">config.update</option>
                          <option value="lead.assigned">lead.assigned</option>
                          <option value="lead.unassigned">lead.unassigned</option>
                          <option value="broker.paused">broker.paused</option>
                          <option value="lead.escalated">lead.escalated</option>
                          <option value="lead.escalation_capped">lead.escalation_capped</option>
                        </select>
                      </label>
                      <label className="text-xs font-bold text-muted-foreground flex flex-col gap-1">
                        Alvo (broker/lead ID)
                        <input
                          type="text"
                          value={auditTargetID}
                          onChange={(e) => setAuditTargetID(e.target.value)}
                          placeholder="UUID exato"
                          className="px-3 py-2 rounded-lg border border-border bg-card text-xs font-medium text-foreground placeholder:text-muted-foreground"
                        />
                      </label>
                      <label className="text-xs font-bold text-muted-foreground flex flex-col gap-1">
                        De
                        <input type="date" value={auditFrom} onChange={(e) => setAuditFrom(e.target.value)} className="px-3 py-2 rounded-lg border border-border bg-card text-xs font-medium text-foreground" />
                      </label>
                      <label className="text-xs font-bold text-muted-foreground flex flex-col gap-1">
                        Até
                        <input type="date" value={auditTo} onChange={(e) => setAuditTo(e.target.value)} className="px-3 py-2 rounded-lg border border-border bg-card text-xs font-medium text-foreground" />
                      </label>
                    </div>
                  </div>
                  <div className="overflow-x-auto">
                    <table className="w-full text-left border-collapse min-w-[900px]">
                      <thead>
                        <tr className="border-b border-border text-muted-foreground text-xs font-medium tracking-wide">
                          <th className="px-6 py-4">Quando</th>
                          <th className="px-6 py-4">Ação</th>
                          <th className="px-6 py-4">Ator</th>
                          <th className="px-6 py-4">Alvo</th>
                          <th className="px-6 py-4">Detalhes</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-border">
                        {auditQuery.isLoading && (
                          <tr>
                            <td className="px-6 py-6 text-sm text-muted-foreground" colSpan={5}>
                              Carregando...
                            </td>
                          </tr>
                        )}
                        {!auditQuery.isLoading && (auditQuery.data?.items ?? []).length === 0 && (
                          <tr>
                            <td className="px-6 py-6 text-sm text-muted-foreground" colSpan={5}>
                              Nenhum evento encontrado.
                            </td>
                          </tr>
                        )}
                        {(auditQuery.data?.items ?? []).map((e) => {
                          const keys = e.diff ? Object.keys(e.diff) : [];
                          const actor = e.actor_email || e.actor_id || "—";
                          const target = e.target_id || "—";
                          const isExpanded = !!auditExpanded[e.id];
                          return (
                            <React.Fragment key={e.id}>
                              <tr className="hover:bg-muted/80 transition-colors">
                                <td className="px-6 py-4">
                                  <div className="text-sm font-medium text-foreground">{new Date(e.created_at).toLocaleString()}</div>
                                </td>
                                <td className="px-6 py-4">
                                  <span className="px-2.5 py-1 text-caption font-bold rounded-full bg-primary-50 text-primary-700 border border-primary-200">
                                    {e.action}
                                  </span>
                                </td>
                                <td className="px-6 py-4">
                                  <span className="text-sm font-bold text-foreground">{actor}</span>
                                </td>
                                <td className="px-6 py-4">
                                  <span className="text-sm text-muted-foreground font-mono break-all">{target}</span>
                                </td>
                                <td className="px-6 py-4">
                                  {keys.length > 0 ? (
                                    <Button
                                      type="button"
                                      variant="link"
                                      size="sm"
                                      onClick={() => auditToggleExpand(e.id)}
                                      aria-expanded={isExpanded}
                                      aria-label={isExpanded ? `Ocultar detalhes do evento ${e.action}` : `Ver detalhes do evento ${e.action}`}
                                      className="h-auto p-0"
                                    >
                                      {isExpanded ? "Ocultar" : `Ver ${keys.length} ${keys.length === 1 ? "campo" : "campos"}`}
                                    </Button>
                                  ) : (
                                    <span className="text-sm text-muted-foreground">—</span>
                                  )}
                                </td>
                              </tr>
                              {isExpanded && keys.length > 0 && (
                                <tr className="bg-muted/40">
                                  <td colSpan={5} className="px-6 py-3">
                                    <pre className="text-xs bg-card border border-border rounded-lg p-3 overflow-x-auto whitespace-pre-wrap break-all max-h-96 overflow-y-auto">
                                      {JSON.stringify(e.diff, null, 2)}
                                    </pre>
                                  </td>
                                </tr>
                              )}
                            </React.Fragment>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                  <div className="p-4 border-t border-border bg-muted/30 flex items-center justify-between">
                    <div className="text-xs text-muted-foreground">
                      Mostrando <span className="font-bold text-foreground">{(auditQuery.data?.items ?? []).length}</span> de{" "}
                      <span className="font-bold text-foreground">{auditQuery.data?.total ?? 0}</span>
                    </div>
                    <div className="flex gap-2">
                      <Button type="button" variant="outline" size="sm" onClick={() => setAuditOffset((p) => Math.max(0, p - auditLimit))} disabled={auditOffset === 0}>
                        Anterior
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => setAuditOffset((p) => p + auditLimit)}
                        disabled={auditOffset + auditLimit >= (auditQuery.data?.total ?? 0)}
                      >
                        Próxima
                      </Button>
                    </div>
                  </div>
                </div>
              )}
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </div>
  );
}
