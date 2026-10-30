import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type ReactNode,
  type SetStateAction,
} from "react";
import { supportErrorMessage } from "../lib/errorDiagnostics";
import { toast } from "sonner";
import type {
  Appointment,
  AppointmentPriority,
  AppointmentStatus,
  AppointmentType,
  Lead,
  LeadStatus,
  Property,
} from "../data";
import { formatPropertyAddress } from "../data";
import {
  calendarService,
  type ApiAppointment,
  type CreateAppointmentDTO,
  type UpdateAppointmentDTO,
} from "../services/calendar.service";
import { useDataChangeRefresh } from "../hooks/useDataChangeRefresh";
import { useRequestScopeGuard } from "../hooks/useRequestScopeGuard";
import { unwrapApiData } from "../services/api";
import type { Agent } from "../types/agent";

type CalendarContextValue = {
  appointments: Appointment[];
  isLoading: boolean;
  error: string | null;
  refreshAppointments: () => Promise<void>;
  addAppointment: (apt: Omit<Appointment, "id" | "createdAt">) => Promise<Appointment | null>;
  updateAppointment: (id: string, updates: Partial<Appointment>) => Promise<Appointment | null>;
  deleteAppointment: (id: string) => Promise<boolean>;
  scope: "mine" | "team";
  setScope: (scope: "mine" | "team") => void;
  canViewTeam: boolean;
  currentUserId?: string;
};

const CalendarContext = createContext<CalendarContextValue | null>(null);

// Cache em memória por tenant/escopo. O provider pode ser remontado ao navegar
// entre áreas; sem isso a agenda nascia vazia e piscava "Carregando agenda…"
// mesmo já tendo os dados. A cópia é revalidada em segundo plano.
type CalendarCacheEntry = { appointments: Appointment[]; updatedAt: number };
const calendarCache = new Map<string, CalendarCacheEntry>();

export function CalendarProvider({
  children,
  isAuthenticated,
  companyId,
  permissionsLoading,
  agents,
  properties,
  leads,
  setLeads,
  canViewTeam = false,
  currentUserId,
}: {
  children: ReactNode;
  isAuthenticated: boolean;
  companyId?: string;
  permissionsLoading: boolean;
  agents: Agent[];
  properties: Property[];
  leads: Lead[];
  setLeads: Dispatch<SetStateAction<Lead[]>>;
  canViewTeam?: boolean;
  currentUserId?: string;
}) {
  const [scope, setScopeState] = useState<"mine" | "team">("mine");
  const effectiveScope = canViewTeam ? scope : "mine";
  const cacheKey = `${companyId ?? ""}:${effectiveScope}:${currentUserId ?? ""}`;
  const initialCache = calendarCache.get(cacheKey);
  // Qual chave realmente hidratou o state. Evita gravar a agenda antiga sob
  // a chave nova no primeiro render após trocar de empresa/escopo.
  const activeCacheKeyRef = useRef<string | null>(initialCache ? cacheKey : null);
  const [appointments, setAppointments] = useState<Appointment[]>(() => initialCache?.appointments ?? []);
  const [isLoading, setIsLoading] = useState(() => !initialCache);
  const [error, setError] = useState<string | null>(null);
  const beginScopedRequest = useRequestScopeGuard(`${isAuthenticated}:${companyId ?? ""}:${effectiveScope}`);
  const setScope = useCallback((next: "mine" | "team") => setScopeState(canViewTeam ? next : "mine"), [canViewTeam]);

  // WS reconciliation protection for optimistic updates
  const calendarMutationRef = useRef<Promise<unknown> | null>(null);
  const calendarVersionRef = useRef(0);
  const fetchAbortRef = useRef<AbortController | null>(null);
  const fetchInFlightRef = useRef(false);
  const lastFetchAtRef = useRef(0);
  const calendarCooldownUntilRef = useRef<number>(0);

  const defaultPriority: AppointmentPriority = "Media";

  const calcEndTime = useCallback((start: string) => {
    const parts = start.split(":");
    if (parts.length < 2) return "10:00";
    const h = Number(parts[0]);
    const m = Number(parts[1]);
    const endH = (Number.isFinite(h) ? h : 9) + 1;
    const endM = Number.isFinite(m) ? m : 0;
    return `${String(endH).padStart(2, "0")}:${String(endM).padStart(2, "0")}`;
  }, []);

  const normalizeAppointment = useCallback(
    (apiApt: ApiAppointment, fallback?: Partial<Appointment>): Appointment => {
      const type: AppointmentType = apiApt.type || fallback?.type || "Outro";

      const rawStatus = apiApt.status === "Concluído" ? "Concluido" : apiApt.status;
      const status: AppointmentStatus = rawStatus || fallback?.status || "Agendado";

      const time = apiApt.time || fallback?.time || "09:00";
      const endTime = apiApt.endTime || fallback?.endTime || calcEndTime(time);

      return {
        id: apiApt.id,
        inspectionId: apiApt.inspectionId,
        inspectionContractId: apiApt.inspectionContractId,
        inspectorId: apiApt.inspectorId,
        inspectorName: apiApt.inspectorName,
        title: apiApt.title || fallback?.title || "",
        client: apiApt.client || fallback?.client || "",
        type,
        date: apiApt.date || fallback?.date || new Date().toISOString().split("T")[0],
        time,
        endTime,
        location: apiApt.property ?? "",
        status,
        priority: (apiApt.priority as AppointmentPriority) || fallback?.priority || defaultPriority,
        description: apiApt.description ?? fallback?.description,
        notes: fallback?.notes,
        agentId: apiApt.agentId || fallback?.agentId,
        agentName: apiApt.agentName || fallback?.agentName,
        visibility: apiApt.visibility || fallback?.visibility || "team",
        propertyId: apiApt.propertyId,
        contractId: apiApt.dealId,
        leadId: apiApt.leadId || fallback?.leadId,
        requestedByClient: apiApt.requestedByClient ?? fallback?.requestedByClient,
        customerAccountId: apiApt.customerAccountId || fallback?.customerAccountId,
        isAllDay: fallback?.isAllDay,
        reminder: fallback?.reminder,
        updatedAt: apiApt.updatedAt,
        createdAt: apiApt.createdAt || fallback?.createdAt || new Date().toISOString(),
      };
    },
    [calcEndTime],
  );

  const toUpdateAppointmentDTO = useCallback(
    (updates: Partial<Appointment>): UpdateAppointmentDTO => {
      const dto: UpdateAppointmentDTO = {};
      if (updates.title !== undefined) dto.title = updates.title;
      if (updates.date !== undefined) dto.date = updates.date;
      if (updates.time !== undefined) dto.time = updates.time;
      if (updates.endTime !== undefined) dto.endTime = updates.endTime;
      if (updates.type !== undefined) dto.type = updates.type;
      if (updates.client !== undefined) dto.client = updates.client;
      if (updates.propertyId !== undefined) {
        dto.propertyId = updates.propertyId;
        const prop = properties.find((p) => p.id === updates.propertyId);
        if (prop) dto.property = `${prop.title} - ${formatPropertyAddress(prop.address)}`;
      }
      if (updates.agentId !== undefined) {
        dto.agentId = updates.agentId;
        const agent = agents.find((a) => a.id === updates.agentId);
        if (agent) dto.agent = agent.name;
      }
      if (updates.contractId !== undefined) dto.dealId = updates.contractId;
      if (updates.leadId !== undefined) dto.leadId = updates.leadId;
      if (updates.priority !== undefined) dto.priority = updates.priority;
      if (updates.status !== undefined) dto.status = updates.status;
      if (updates.description !== undefined) dto.description = updates.description;
      if (updates.visibility !== undefined) dto.visibility = updates.visibility;
      return dto;
    },
    [agents, properties],
  );

  const fetchAppointments = useCallback(async (force = false) => {
    const isCurrent = beginScopedRequest();

    // Wait for any in-flight calendar mutation to settle.
    const pendingMutation = calendarMutationRef.current;
    if (pendingMutation) {
      try { await pendingMutation; } catch {}
    }

    if (!isCurrent()) return;
    // Uma montagem + um evento de dados não podem gerar duas consultas em
    // sequência. Rajadas de refresh também não podem virar um loop de rede.
    if (!force && (fetchInFlightRef.current || Date.now() - lastFetchAtRef.current < 2_000)) return;
    fetchInFlightRef.current = true;
    lastFetchAtRef.current = Date.now();
    fetchAbortRef.current?.abort();
    const controller = new AbortController();
    fetchAbortRef.current = controller;
    const fetchVersion = calendarVersionRef.current;

    try {
      const response = await calendarService.getAll({ scope: effectiveScope }, { signal: controller.signal });
      const appointmentsData = unwrapApiData<ApiAppointment[]>(response);
      if (!isCurrent()) return;

      // Skip if a newer mutation started or we're within post-mutation cooldown.
      if (calendarVersionRef.current !== fetchVersion) return;
      if (Date.now() < calendarCooldownUntilRef.current) return;

      const nextAppointments = Array.isArray(appointmentsData)
        ? appointmentsData.map((a) => normalizeAppointment(a))
        : [];
      setAppointments(nextAppointments);
      activeCacheKeyRef.current = cacheKey;
      calendarCache.set(cacheKey, { appointments: nextAppointments, updatedAt: Date.now() });
      setError(null);
    } catch (error: any) {
      if (!isCurrent() || controller.signal.aborted) return;
      setError("Não foi possível atualizar a agenda. Os compromissos podem estar desatualizados.");
    } finally {
      fetchInFlightRef.current = false;
      if (isCurrent()) setIsLoading(false);
    }
  }, [beginScopedRequest, cacheKey, effectiveScope, normalizeAppointment]);

  useDataChangeRefresh(["calendar", "inspections"], fetchAppointments, {
    enabled: isAuthenticated && Boolean(companyId) && !permissionsLoading,
  });

  useEffect(() => {
    fetchAbortRef.current?.abort();
    // `permissionsLoading` não pode derrubar a agenda já carregada: uma
    // revalidação de permissões (ou de preferências) não muda o escopo.
    if (!isAuthenticated || !companyId) {
      activeCacheKeyRef.current = null;
      setAppointments([]);
      setIsLoading(false);
      setError(null);
      return;
    }
    const cached = calendarCache.get(cacheKey);
    if (cached) {
      activeCacheKeyRef.current = cacheKey;
      // Stale-while-revalidate: mantém a agenda na tela enquanto a nova
      // consulta roda; nunca troca conteúdo por um estado vazio/loading.
      setAppointments(cached.appointments);
      setIsLoading(false);
    } else {
      activeCacheKeyRef.current = null;
      setAppointments([]);
      setIsLoading(true);
    }
    fetchAppointments(true).catch(() => {});
    return () => { fetchAbortRef.current?.abort(); };
  }, [cacheKey, companyId, fetchAppointments, isAuthenticated]);

  // Mantém o cache coerente com mutações otimistas e com o escopo atual.
  useEffect(() => {
    if (isLoading || activeCacheKeyRef.current !== cacheKey) return;
    calendarCache.set(cacheKey, { appointments, updatedAt: Date.now() });
  }, [appointments, cacheKey, isLoading]);

  useEffect(() => {
    if (appointments.length === 0 || leads.length === 0) return;
    setLeads((prev) =>
      prev.map((lead) => {
        if (lead.status === "Qualificado") {
          const hasVisit = appointments.some(
            (a) => a.leadId === lead.id && a.type === "Visita" && (a.status === "Confirmado" || (!a.requestedByClient && a.status === "Agendado")),
          );
          if (hasVisit) return { ...lead, status: "Visita Agendada" as LeadStatus };
        }
        return lead;
      }),
    );
  }, [appointments, leads.length, setLeads]);

  const addAppointment = useCallback(
    async (apt: Omit<Appointment, "id" | "createdAt">) => {
      const isCurrent = beginScopedRequest();
      try {
        const agent = agents.find((a) => a.id === apt.agentId);
        const agentName = agent?.name || apt.client || "";

        const prop = apt.propertyId ? properties.find((p) => p.id === apt.propertyId) : undefined;
        const propertyName = prop ? `${prop.title} - ${formatPropertyAddress(prop.address)}` : undefined;

        const dto: CreateAppointmentDTO = {
          title: apt.title,
          date: apt.date,
          time: apt.time,
          endTime: apt.endTime,
          type: apt.type,
          client: apt.client,
          agent: agentName,
          agentId: apt.agentId,
          property: propertyName,
          propertyId: apt.propertyId,
          leadId: apt.leadId,
          dealId: apt.contractId,
          priority: apt.priority,
          description: apt.description,
          status: apt.status,
          visibility: apt.visibility,
        };

        const response = await calendarService.create(dto);
        const apiApt = unwrapApiData<ApiAppointment>(response);
        if (!isCurrent()) return null;
        const newApt = normalizeAppointment(apiApt, apt);

        setAppointments((prev) => [newApt, ...prev]);
        toast.success("Agendamento criado!");
        return newApt;
      } catch (error: any) {
        if (!isCurrent()) return null;
        if (error.response?.status === 409) {
          toast.error("Conflito de horário: já existe um agendamento nesse período para este corretor.");
        } else {
          toast.error(supportErrorMessage(error, "Erro ao criar agendamento. Seus dados continuam no formulário."));
        }
        return null;
      }
    },
    [agents, beginScopedRequest, normalizeAppointment, properties],
  );

  const updateAppointment = useCallback(
    async (id: string, updates: Partial<Appointment>) => {
      const isCurrent = beginScopedRequest();
      const previous = appointments.find((appointment) => appointment.id === id);

      // Block WS reconciliation before optimistic update
      let releasePlaceholder: () => void = () => {};
      calendarMutationRef.current = new Promise<void>((r) => { releasePlaceholder = r; });
      ++calendarVersionRef.current;

      setAppointments((prev) => prev.map((a) => (a.id === id ? { ...a, ...updates } : a)));

      const mutationPromise = (async () => {
        try {
          const dto: UpdateAppointmentDTO = toUpdateAppointmentDTO(updates);
          dto.expectedUpdatedAt = previous?.updatedAt;
          const response = await calendarService.update(id, dto);
          const apiApt = unwrapApiData<ApiAppointment>(response);
          if (apiApt?.id && isCurrent()) {
            const saved = normalizeAppointment(apiApt, previous);
            setAppointments((prev) => prev.map((a) => (a.id === id ? saved : a)));
            calendarCooldownUntilRef.current = Date.now() + 2000;
            return saved;
          }
          return null;
        } catch (error: any) {
          if (!isCurrent()) return null;
          ++calendarVersionRef.current;
          const status = error?.response?.status;
          if (status === 404) {
            // O compromisso foi removido/cancelado (ex. cliente cancelou a visita
            // pelo portal) — não existe mais para editar.
            setAppointments((prev) => prev.filter((a) => a.id !== id));
            toast.error("Este compromisso não existe mais (foi cancelado/removido).");
          } else {
            if (previous) setAppointments((prev) => prev.map((a) => a.id === id ? previous : a));
            // Fetch authoritative state on failure
            try {
              fetchAbortRef.current?.abort();
              const controller = new AbortController();
              fetchAbortRef.current = controller;
              const response = await calendarService.getAll(undefined, { signal: controller.signal });
              const data = unwrapApiData<ApiAppointment[]>(response);
              if (data && isCurrent()) {
                setAppointments(data.map((a) => normalizeAppointment(a)));
              }
            } catch {
              if (isCurrent()) setError("Não foi possível atualizar a agenda. Tente novamente.");
            }
            if (!isCurrent()) return null;
            toast.error(supportErrorMessage(error, status === 409 ? "Não foi possível salvar por conflito de horário ou alteração recente. Confira a agenda atualizada e tente novamente." : "Erro ao atualizar agendamento. Seus dados continuam no formulário."));
          }
          return null;
        }
      })();

      calendarMutationRef.current = mutationPromise;
      releasePlaceholder();
      mutationPromise.finally(() => {
        if (calendarMutationRef.current === mutationPromise) {
          calendarMutationRef.current = null;
        }
      });
      return mutationPromise;
    },
    [appointments, beginScopedRequest, normalizeAppointment, toUpdateAppointmentDTO],
  );

  const deleteAppointment = useCallback(
    async (id: string) => {
      const isCurrent = beginScopedRequest();
      try {
        await calendarService.delete(id);
        if (!isCurrent()) return false;
        setAppointments((prev) => prev.filter((a) => a.id !== id));
        toast.success("Agendamento removido.");
        return true;
      } catch (error) {
        if (!isCurrent()) return false;
        toast.error(supportErrorMessage(error, "Erro ao remover agendamento."));
        return false;
      }
    },
    [beginScopedRequest],
  );

  const value = useMemo<CalendarContextValue>(
    () => ({
      appointments,
      isLoading,
      error,
      refreshAppointments: fetchAppointments,
      addAppointment,
      updateAppointment,
      deleteAppointment,
      scope: effectiveScope,
      setScope,
      canViewTeam,
      currentUserId,
    }),
    [addAppointment, appointments, canViewTeam, currentUserId, deleteAppointment, effectiveScope, updateAppointment, error, isLoading, fetchAppointments, setScope],
  );

  return <CalendarContext.Provider value={value}>{children}</CalendarContext.Provider>;
}

export function useCalendarContext() {
  const ctx = useContext(CalendarContext);
  if (!ctx) throw new Error("useCalendarContext must be used within CalendarProvider");
  return ctx;
}
