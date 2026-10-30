import { useState, useId, useMemo, useCallback, useEffect, useRef } from "react";
import { Link, useSearchParams } from "react-router";
import {
  ChevronLeft,
  ChevronRight,
  Plus,
  Calendar as CalendarIcon,
  Clock,
  MapPin,
  User,
  Phone,
  Video,
  Home,
  Search,
  Filter,
  LayoutGrid,
  List,
  Columns3,
  CalendarDays,
  CheckCircle2,
  XCircle,
  RotateCcw,
  Edit3,
  Trash2,
  Bell,
  FileText,
  ClipboardCheck,
  ChevronDown,
  Loader2,
  UserPlus,
  ChevronsUpDown,
  Check,
  X,
} from "lucide-react";
import {
  format,
  addDays,
  addMonths,
  subMonths,
  startOfWeek,
  endOfWeek,
  startOfMonth,
  endOfMonth,
  eachDayOfInterval,
  isSameDay,
  isSameMonth,
  isToday,
  parseISO,
} from "date-fns";
import { ptBR } from "date-fns/locale";
import { clsx } from "clsx";
import { motion, AnimatePresence } from "motion/react";
import { toast } from "sonner";
import { EmptyState } from "../ui/EmptyState";
import { Button } from "../ui/button";
import { PageHeader } from "../ui/page-header";
import { useAgentsContext } from "../context/AgentsContext";
import { useCalendarContext } from "../context/CalendarContext";
import { useEsteiraDeals } from "../hooks/useEsteiraDeals";
import { useLeadsContext } from "../context/LeadsContext";
import { usePropertiesContext } from "../context/PropertiesContext";
import { EventForm } from "../components/calendar/EventForm";
import {
  ALL_TYPES,
  DEFAULT_TYPE_CONFIG,
  PRIORITY_CONFIG,
  STATUS_CONFIG,
  TYPE_CONFIG,
} from "../components/calendar/appointmentOptions";
import {
  EMPTY_APPOINTMENT_FORM,
  appointmentFormFrom,
  validateAppointmentForm,
  type AppointmentFormValues,
} from "../components/calendar/appointmentForm";
import { useViewPreferences } from "../context/ViewPreferencesContext";
import { Appointment, AppointmentType, AppointmentStatus, AppointmentPriority } from "../data";
import { ExpandableSidePanel } from "../components/ExpandableSidePanel";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { AddToCalendarButton } from "../components/AddToCalendarButton";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "../ui/command";
import { PropertyPicker } from "../components/PropertyPicker";
import { dealOptionLabel } from "../lib/deals/dealOptionLabel";
import { leadService, ApiLead } from "../services/lead.service";
import { propertyService } from "../services/property.service";
import { dealService, DealDTO } from "../services/deal.service";
import { PropertyDTO } from "../services/property.service";
import { formatPhoneBR } from "../utils/masks";

type ViewMode = "mes" | "semana" | "dia" | "lista";

const HOURS = Array.from({ length: 14 }, (_, i) => i + 7);
const CALENDAR_DATE_STORAGE_KEY = "suitee-calendar-current-date";

function readStoredCalendarDate(): Date {
  if (typeof window === "undefined") return new Date();
  try {
    const stored = window.sessionStorage.getItem(CALENDAR_DATE_STORAGE_KEY);
    if (stored) {
      const parsed = parseISO(stored);
      if (!Number.isNaN(parsed.getTime())) return parsed;
    }
  } catch {
    // sessionStorage bloqueado: segue com o dia atual.
  }
  return new Date();
}


function CalendarSkeleton() {
  return (
    <div role="status" aria-live="polite" aria-label="Carregando agenda" data-testid="calendar-skeleton" className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <div className="h-4 w-40 animate-pulse rounded-md bg-muted" />
        <div className="h-8 w-48 animate-pulse rounded-lg bg-muted" />
      </div>
      <div className="overflow-hidden rounded-2xl border border-border bg-card">
        <div className="grid grid-cols-7 gap-px border-b border-border bg-border">
          {Array.from({ length: 7 }, (_, index) => (
            <div key={index} className="bg-card px-3 py-3">
              <div className="mx-auto h-3 w-14 animate-pulse rounded bg-muted" />
            </div>
          ))}
        </div>
        {Array.from({ length: 7 }, (_, row) => (
          <div key={row} className="grid grid-cols-7 gap-px border-b border-border bg-border last:border-b-0">
            {Array.from({ length: 7 }, (_, column) => (
              <div key={column} className="min-h-16 bg-card px-3 py-3">
                <div className={clsx(
                  "h-3 animate-pulse rounded bg-muted",
                  column % 3 === 0 ? "w-3/4" : column % 3 === 1 ? "w-1/2" : "w-2/3",
                )} />
                {row % 3 === 1 && column === 2 ? <div className="mt-3 h-7 animate-pulse rounded-lg bg-primary/10" /> : null}
              </div>
            ))}
          </div>
        ))}
      </div>
      <span className="sr-only">Carregando agenda…</span>
    </div>
  );
}

export function Calendar() {
  const { appointments, isLoading, error, refreshAppointments, addAppointment, updateAppointment, deleteAppointment, scope = "mine", setScope = () => {}, canViewTeam = false, currentUserId } = useCalendarContext();
  const { agents } = useAgentsContext();
  const { properties } = usePropertiesContext();
  const { leads } = useLeadsContext();
  const { deals, dealsError, refreshDeals } = useEsteiraDeals();
  const [searchParams, setSearchParams] = useSearchParams();

  const [currentDate, setCurrentDateState] = useState<Date>(readStoredCalendarDate);
  const setCurrentDate = useCallback((date: Date) => {
    setCurrentDateState(date);
    try {
      window.sessionStorage.setItem(CALENDAR_DATE_STORAGE_KEY, format(date, "yyyy-MM-dd"));
    } catch {
      // Persistência é apenas conveniência de navegação.
    }
  }, []);
  const [selectedDate, setSelectedDate] = useState<Date>(currentDate);
  const [calendarView, setCalendarView] = useViewPreferences<{ viewMode: ViewMode }>("calendar", {
    viewMode: "semana",
  });
  const { viewMode } = calendarView;
  const setViewMode = (v: ViewMode) => setCalendarView({ viewMode: v });
  const [search, setSearch] = useState("");
  const [activeTypes, setActiveTypes] = useState<Set<AppointmentType>>(new Set(ALL_TYPES));
  const [activeStatuses, setActiveStatuses] = useState<Set<AppointmentStatus>>(
    new Set(Object.keys(STATUS_CONFIG) as AppointmentStatus[]),
  );
  const [showFilters, setShowFilters] = useState(false);

  const [detailApt, setDetailApt] = useState<Appointment | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [showCreatePanel, setShowCreatePanel] = useState(false);
  const [form, setForm] = useState(EMPTY_APPOINTMENT_FORM);
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [visitsOnly, setVisitsOnly] = useState(false);
  const assignableAgents = useMemo(
    () => scope === "team" && canViewTeam ? agents : agents.filter((agent) => agent.id === currentUserId),
    [agents, canViewTeam, currentUserId, scope],
  );

  useEffect(() => {
    // Sem polling periódico: a agenda já recebe eventos em tempo real e
    // revalida ao voltar o foco. O intervalo antigo gerava uma atualização
    // extra logo depois do carregamento, sem mudança real de dados.
    const refresh = () => { if (document.visibilityState !== "hidden") void refreshAppointments(); };
    window.addEventListener("focus", refresh);
    return () => { window.removeEventListener("focus", refresh); };
  }, [refreshAppointments]);

  useEffect(() => {
    if (!detailApt || editMode || saving) return;
    const current = appointments.find((appointment) => appointment.id === detailApt.id);
    if (current) setDetailApt(current);
  }, [appointments, editMode, saving]);

  const pendingVisits = useMemo(() => appointments.filter((appointment) =>
    appointment.requestedByClient && appointment.type === "Visita" &&
    ["Pendente", "Agendado", "Reagendado"].includes(appointment.status),
  ).sort((a, b) => `${a.date} ${a.time}`.localeCompare(`${b.date} ${b.time}`)), [appointments]);

  const filteredAppointments = useMemo(() => {
    return appointments.filter((a) => {
      if (visitsOnly && !pendingVisits.some((visit) => visit.id === a.id)) return false;
      if (ALL_TYPES.includes(a.type) && !activeTypes.has(a.type)) return false;
      if (a.status in STATUS_CONFIG && !activeStatuses.has(a.status)) return false;
      if (search) {
        const q = search.toLowerCase();
        return (
          a.title.toLowerCase().includes(q) ||
          a.client.toLowerCase().includes(q) ||
          (a.location || "").toLowerCase().includes(q)
        );
      }
      return true;
    });
  }, [appointments, activeTypes, activeStatuses, search, visitsOnly, pendingVisits]);

  const getAppointmentsForDate = useCallback(
    (date: Date) => {
      const ds = format(date, "yyyy-MM-dd");
      return filteredAppointments
        .filter((a) => a.date === ds)
        .sort((a, b) => a.time.localeCompare(b.time));
    },
    [filteredAppointments],
  );

  const navigateBack = () => {
    if (viewMode === "mes") setCurrentDate(subMonths(currentDate, 1));
    else if (viewMode === "semana") setCurrentDate(addDays(currentDate, -7));
    else setCurrentDate(addDays(currentDate, -1));
  };
  const navigateForward = () => {
    if (viewMode === "mes") setCurrentDate(addMonths(currentDate, 1));
    else if (viewMode === "semana") setCurrentDate(addDays(currentDate, 7));
    else setCurrentDate(addDays(currentDate, 1));
  };
  const goToday = () => {
    setCurrentDate(new Date());
    setSelectedDate(new Date());
  };

  const toggleType = (t: AppointmentType) => {
    setActiveTypes((prev) => {
      const n = new Set(prev);
      n.has(t) ? n.delete(t) : n.add(t);
      return n;
    });
  };

  const openCreate = (date?: Date) => {
    setForm({ ...EMPTY_APPOINTMENT_FORM, date: format(date || selectedDate, "yyyy-MM-dd"), agentId: currentUserId || "" });
    setEditMode(false);
    setShowCreatePanel(true);
  };

  const openEdit = (apt: Appointment) => {
    setDetailApt(apt);
    if (apt.inspectionId) return;
    setForm(appointmentFormFrom(apt));
    setEditMode(true);
    setShowCreatePanel(true);
  };

  useEffect(() => {
    const aptId = searchParams.get("compromisso") || searchParams.get("apt");
    if (!aptId || isLoading || error) return;
    const appointment = appointments.find((item) => item.id === aptId);
    if (appointment) {
      setCurrentDate(parseISO(appointment.date));
      setSelectedDate(parseISO(appointment.date));
      setDetailApt(appointment);
      if (searchParams.has("compromisso")) openEdit(appointment);
    } else {
      toast.error("Este compromisso não existe mais.");
    }
    const next = new URLSearchParams(searchParams);
    next.delete("compromisso"); next.delete("apt");
    setSearchParams(next, { replace: true });
  }, [appointments, isLoading, error, searchParams, setSearchParams]);

  const handleSave = async () => {
    if (saving) return;
    const errors = validateAppointmentForm(form, { editing: editMode ? detailApt : null });
    if (errors.length) {
      toast.error(`Campos obrigatórios: ${errors.join(", ")}`);
      return;
    }

    setSaving(true);
    try {
      const saved = editMode && detailApt
        ? await updateAppointment(detailApt.id, Object.fromEntries(Object.entries(form).filter(([key, value]) => value !== detailApt[key as keyof Appointment])) as Partial<Appointment>)
        : await addAppointment(form);
      if (!saved) return;
      if (editMode) toast.success("Compromisso atualizado");
      setShowCreatePanel(false);
      setEditMode(false);
      setDetailApt(saved);
    } finally {
      setSaving(false);
    }
  };

  const handleVisitStatus = async (appointment: Appointment, status: "Confirmado" | "Cancelado") => {
    if (saving) return;
    setSaving(true);
    try {
      const saved = await updateAppointment(appointment.id, { status });
      if (saved) {
        setDetailApt(saved);
        toast.success(status === "Confirmado" ? "Visita confirmada. O cliente acompanhará pelo portal." : "Visita cancelada. O cliente acompanhará pelo portal.");
      }
    } finally { setSaving(false); }
  };

  const handleDelete = (id: string) => {
    setConfirmDelete(id);
  };

  const confirmDeleteNow = async () => {
    if (!confirmDelete || saving) return;
    if (!(await deleteAppointment(confirmDelete))) return;
    if (detailApt?.id === confirmDelete) setDetailApt(null);
    setConfirmDelete(null);
  };

  const daysForMonth = useMemo(() => {
    const start = startOfWeek(startOfMonth(currentDate), { weekStartsOn: 0 });
    const end = endOfWeek(endOfMonth(currentDate), { weekStartsOn: 0 });
    return eachDayOfInterval({ start, end });
  }, [currentDate]);

  const weekRange = useMemo(() => {
    const start = startOfWeek(currentDate, { weekStartsOn: 0 });
    const end = endOfWeek(currentDate, { weekStartsOn: 0 });
    return { start, end };
  }, [currentDate]);

  const daysForWeek = useMemo(() => {
    return eachDayOfInterval({ start: weekRange.start, end: weekRange.end });
  }, [weekRange]);

  const dayLabel = useMemo(() => format(currentDate, "PPPP", { locale: ptBR }), [currentDate]);

  const headerLabel = useMemo(() => {
    if (viewMode === "mes") return format(currentDate, "MMMM yyyy", { locale: ptBR });
    if (viewMode === "semana")
      return `${format(weekRange.start, "dd MMM", { locale: ptBR })} - ${format(weekRange.end, "dd MMM yyyy", { locale: ptBR })}`;
    return format(currentDate, "dd MMM yyyy", { locale: ptBR });
  }, [currentDate, viewMode, weekRange]);

  const updateField = (field: string, value: any) => setForm((prev) => ({ ...prev, [field]: value }));

  const viewButtons = [
    { id: "mes" as const, label: "Mês", icon: CalendarDays },
    { id: "semana" as const, label: "Semana", icon: Columns3 },
    { id: "dia" as const, label: "Dia", icon: LayoutGrid },
    { id: "lista" as const, label: "Lista", icon: List },
  ];

  return (
    <div className="flex flex-col h-full bg-muted overflow-auto">
      {dealsError && <div role="alert" className="mx-6 mt-4 rounded-xl border border-destructive/30 bg-card p-4 flex items-center justify-between gap-4 text-sm">
        <p>{dealsError}</p><Button variant="outline" onClick={() => void refreshDeals()}>Tentar novamente</Button>
      </div>}
      {error && <div role="alert" className="mx-6 mt-4 rounded-xl border border-destructive/30 bg-card p-4 flex items-center justify-between gap-4 text-sm">
        <p>{error}</p><Button variant="outline" onClick={() => void refreshAppointments()}>Tentar novamente</Button>
      </div>}
      {pendingVisits.length > 0 && <div className="mx-6 mt-4 rounded-xl border border-primary/20 bg-card p-4">
        <div className="flex items-center justify-between gap-3 mb-3">
          <div><h2 className="text-sm font-bold">Solicitações de visita do portal ({pendingVisits.length})</h2><p className="text-xs text-muted-foreground">Confira o imóvel e o horário antes de confirmar com o cliente.</p></div>
          <Button variant="outline" onClick={() => { setVisitsOnly(!visitsOnly); setViewMode("lista"); }}>{visitsOnly ? "Ver toda a agenda" : "Ver solicitações"}</Button>
        </div>
        <div className="flex flex-wrap gap-2">{pendingVisits.slice(0, 6).map((visit) => <button
          key={visit.id} type="button" onClick={() => { setDetailApt(visit); setCurrentDate(parseISO(visit.date)); }}
          aria-label={`Ver solicitação de ${visit.client}`}
          className="rounded-lg border border-border p-3 text-left text-sm hover:bg-muted"
        ><span className="block font-semibold">{visit.client}</span><span className="text-xs text-muted-foreground">{format(parseISO(visit.date), "dd/MM/yyyy")} · {visit.time}–{visit.endTime}</span></button>)}</div>
      </div>}
      <div className="flex-none px-6 py-4 lg:px-8 border-b border-border bg-card sticky top-0 z-10">
        <PageHeader
          title="Agenda"
          description="Organize visitas, ligações, vistorias e lembretes com o time."
        >
          <div className="flex items-center gap-3 w-full sm:w-auto">
            {canViewTeam ? (
              <div className="flex rounded-lg border border-border bg-muted p-1" aria-label="Escopo da agenda">
                <button type="button" onClick={() => setScope("mine")} className={clsx("rounded-md px-3 py-1.5 text-xs font-semibold", scope === "mine" && "bg-card shadow-sm text-primary-700")}>Minha agenda</button>
                <button type="button" onClick={() => setScope("team")} className={clsx("rounded-md px-3 py-1.5 text-xs font-semibold", scope === "team" && "bg-card shadow-sm text-primary-700")}>Agenda da equipe</button>
              </div>
            ) : null}
            <div className="relative w-full sm:w-72">
              <Search className="w-4 h-4 text-muted-foreground absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Buscar compromissos..."
                className="w-full pl-9 pr-4 py-2 bg-muted border-transparent rounded-lg text-sm focus:bg-card focus:border-primary-500 focus:ring-2 focus:ring-primary-200 outline-none transition-all"
              />
            </div>
            <Button
              variant="outline"
              onClick={() => setShowFilters((v) => !v)}
              className={clsx(
                "whitespace-nowrap",
                showFilters &&
                  "bg-primary-50 border-primary-300 text-primary-700 hover:bg-primary-100 hover:text-primary-700",
              )}
            >
              <Filter className="w-4 h-4" />
              <span className="hidden sm:inline">Filtros</span>
            </Button>
            <Button onClick={() => openCreate()}>
              <Plus className="w-4 h-4" /> Novo
            </Button>
          </div>
        </PageHeader>

        <AnimatePresence>
          {showFilters && (
            <motion.div
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: "auto", opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={{ duration: 0.2 }}
              className="overflow-hidden"
            >
              <div className="pt-3 mt-3 border-t border-border">
                <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-2">
                  Tipos
                </p>
                <div className="flex flex-wrap gap-2">
                  {ALL_TYPES.map((t) => {
                    const cfg = TYPE_CONFIG[t] ?? DEFAULT_TYPE_CONFIG;
                    const active = activeTypes.has(t);
                    return (
                      <button
                        key={t}
                        type="button"
                        onClick={() => toggleType(t)}
                        className={clsx(
                          "px-2.5 py-1 rounded-lg text-xs font-medium border transition-all",
                          active
                            ? `${cfg.bg} ${cfg.border} ${cfg.color}`
                            : "bg-muted text-muted-foreground border-border line-through",
                        )}
                      >
                        {cfg.label}
                      </button>
                    );
                  })}
                </div>
              </div>
            </motion.div>
          )}
        </AnimatePresence>

        <div className="mt-4 flex items-center justify-between gap-3 flex-wrap">
          <div className="flex items-center gap-2">
            <Button variant="outline" size="icon" onClick={navigateBack}>
              <ChevronLeft className="w-4 h-4" />
            </Button>
            <Button variant="outline" size="icon" onClick={goToday}>
              <RotateCcw className="w-4 h-4" />
            </Button>
            <Button variant="outline" size="icon" onClick={navigateForward}>
              <ChevronRight className="w-4 h-4" />
            </Button>
            <div className="ml-1">
              <p className="text-sm font-bold text-foreground capitalize">{headerLabel}</p>
              {viewMode === "dia" ? (
                <p className="text-xs text-muted-foreground">{dayLabel}</p>
              ) : null}
            </div>
          </div>

          <div className="flex items-center gap-1.5 bg-muted rounded-lg border border-border p-1">
            {viewButtons.map((b) => (
              <button
                key={b.id}
                type="button"
                onClick={() => setViewMode(b.id)}
                className={clsx(
                  "px-3 py-2 rounded-md text-xs font-bold flex items-center gap-2 transition-colors",
                  viewMode === b.id ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground",
                )}
              >
                <b.icon className="w-4 h-4" />
                <span className="hidden sm:inline">{b.label}</span>
              </button>
            ))}
          </div>
        </div>
      </div>

      <div className="flex-1 p-6 lg:p-8">
        {isLoading && appointments.length === 0 ? (
          <CalendarSkeleton />
        ) : appointments.length === 0 ? (
          <EmptyState
            icon={CalendarIcon}
            title="Nenhum compromisso"
            message="Crie o primeiro compromisso para começar a organizar sua agenda."
            className="border-none bg-transparent"
            action={
              <Button onClick={() => openCreate()}>
                <Plus className="w-4 h-4" /> Criar compromisso
              </Button>
            }
          />
        ) : viewMode === "mes" ? (
          <MonthView
            days={daysForMonth}
            currentDate={currentDate}
            selectedDate={selectedDate}
            onSelectDate={(d) => {
              setSelectedDate(d);
              setCurrentDate(d);
            }}
            getAppointmentsForDate={getAppointmentsForDate}
            onOpenCreate={(d) => openCreate(d)}
            onOpenDetail={(apt) => setDetailApt(apt)}
            showAgent={scope === "team"}
          />
        ) : viewMode === "semana" ? (
          <WeekView
            days={daysForWeek}
            selectedDate={selectedDate}
            setSelectedDate={setSelectedDate}
            hours={HOURS}
            getAppointmentsForDate={getAppointmentsForDate}
            onOpenCreate={(d) => openCreate(d)}
            onOpenDetail={(apt) => setDetailApt(apt)}
            showAgent={scope === "team"}
          />
        ) : viewMode === "dia" ? (
          <DayView
            date={selectedDate}
            hours={HOURS}
            getAppointmentsForDate={getAppointmentsForDate}
            onOpenCreate={() => openCreate(selectedDate)}
            onOpenDetail={(apt) => setDetailApt(apt)}
            showAgent={scope === "team"}
          />
        ) : (
          <ListView
            appointments={filteredAppointments}
            onOpenDetail={(apt) => setDetailApt(apt)}
            onOpenCreate={() => openCreate()}
            showAgent={scope === "team"}
          />
        )}
      </div>

      <ExpandableSidePanel
        ariaLabel="Detalhes do compromisso"
        open={!!detailApt && !showCreatePanel}
        onClose={() => {
          setDetailApt(null);
          setEditMode(false);
        }}
        renderHeader={({ expandButton, closeButton }) => (
          <div className="px-6 py-4 border-b border-border bg-muted/50 flex items-center justify-between flex-shrink-0">
            <h3 className="text-base font-bold text-foreground tracking-tight">Detalhes</h3>
            <div className="flex items-center gap-2">
              {expandButton}
              {closeButton}
            </div>
          </div>
        )}
      >
        {detailApt ? (
          <EventDetail
            apt={detailApt}
            agents={assignableAgents}
            properties={properties}
            leads={leads}
            deals={deals}
            onEdit={() => openEdit(detailApt)}
            onDelete={() => handleDelete(detailApt.id)}
            saving={saving}
            onConfirm={() => void handleVisitStatus(detailApt, "Confirmado")}
            onCancel={() => void handleVisitStatus(detailApt, "Cancelado")}
          />
        ) : null}
      </ExpandableSidePanel>

      <ExpandableSidePanel
        ariaLabel={editMode ? "Editar compromisso" : "Novo compromisso"}
        open={showCreatePanel}
        onClose={() => {
          setShowCreatePanel(false);
          setEditMode(false);
        }}
        renderHeader={({ expandButton, closeButton }) => (
          <div className="px-6 py-4 border-b border-border bg-muted/50 flex items-center justify-between flex-shrink-0">
            <h3 className="text-base font-bold text-foreground tracking-tight">
              {editMode ? "Editar compromisso" : "Novo compromisso"}
            </h3>
            <div className="flex items-center gap-2">
              {expandButton}
              {closeButton}
            </div>
          </div>
        )}
      >
        {/* Corpo rolável — o rodapé com os botões de ação fica sempre visível */}
        <div className="flex-1 overflow-y-auto min-h-0 px-6 py-5 space-y-4">
          <EventForm
            form={form}
            updateField={updateField}
            agents={assignableAgents}
            properties={properties}
            leads={leads}
            editing={editMode ? detailApt : null}
          />
        </div>
        <div className="px-6 py-4 border-t border-border flex items-center justify-end gap-2 flex-shrink-0">
          <Button variant="outline" onClick={() => setShowCreatePanel(false)}>
            Cancelar
          </Button>
          <Button onClick={handleSave} disabled={saving}>
            <CheckCircle2 className="w-4 h-4" /> {saving ? "Salvando…" : "Salvar"}
          </Button>
        </div>
      </ExpandableSidePanel>

      <ConfirmDialog
        open={!!confirmDelete}
        title="Remover compromisso"
        description="Esta ação não pode ser desfeita."
        confirmLabel="Remover"
        variant="danger"
        onCancel={() => setConfirmDelete(null)}
        onConfirm={confirmDeleteNow}
      />
    </div>
  );
}

function MonthView({
  days,
  currentDate,
  selectedDate,
  onSelectDate,
  getAppointmentsForDate,
  onOpenCreate,
  onOpenDetail,
  showAgent,
}: {
  days: Date[];
  currentDate: Date;
  selectedDate: Date;
  onSelectDate: (d: Date) => void;
  getAppointmentsForDate: (d: Date) => Appointment[];
  onOpenCreate: (d: Date) => void;
  onOpenDetail: (apt: Appointment) => void;
  showAgent: boolean;
}) {
  return (
    <div className="grid grid-cols-7 gap-px bg-border rounded-2xl overflow-hidden border border-border">
      {["Dom", "Seg", "Ter", "Qua", "Qui", "Sex", "Sáb"].map((d) => (
        <div key={d} className="bg-card px-3 py-2 text-xs font-bold text-muted-foreground">
          {d}
        </div>
      ))}
      {days.map((day) => {
        const dayApts = getAppointmentsForDate(day);
        const inMonth = isSameMonth(day, currentDate);
        const selected = isSameDay(day, selectedDate);
        const today = isToday(day);
        return (
          <button
            key={day.toISOString()}
            type="button"
            onClick={() => onSelectDate(day)}
            onDoubleClick={() => onOpenCreate(day)}
            className={clsx(
              "bg-card p-2 text-left min-h-[96px] hover:bg-muted/40 transition-colors relative",
              !inMonth && "opacity-40",
              selected && "ring-2 ring-primary-600 ring-inset",
            )}
          >
            <div className="flex items-center justify-between">
              <span
                className={clsx(
                  "text-xs font-bold",
                  today ? "text-primary-700" : "text-foreground",
                )}
              >
                {format(day, "d")}
              </span>
              <span className="text-caption text-muted-foreground">{dayApts.length || ""}</span>
            </div>
            <div className="mt-1 space-y-1">
              {dayApts.slice(0, 3).map((apt) => {
                const cfg = TYPE_CONFIG[apt.type] ?? { ...DEFAULT_TYPE_CONFIG, label: apt.type || "Tipo desconhecido" };
                return (
                  <button
                    key={apt.id}
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation();
                      onOpenDetail(apt);
                    }}
                    className={clsx(
                      "w-full text-left px-2 py-1 rounded-md text-caption font-bold truncate border",
                      cfg.bg,
                      cfg.border,
                      cfg.color,
                    )}
                  >
                    {apt.time} {apt.title}{showAgent && apt.agentName ? ` · ${apt.agentName}` : ""}
                  </button>
                );
              })}
              {dayApts.length > 3 ? (
                <div className="text-caption text-muted-foreground font-medium">
                  +{dayApts.length - 3} mais
                </div>
              ) : null}
            </div>
          </button>
        );
      })}
    </div>
  );
}

function WeekView({
  days,
  selectedDate,
  setSelectedDate,
  hours,
  getAppointmentsForDate,
  onOpenCreate,
  onOpenDetail,
  showAgent,
}: {
  days: Date[];
  selectedDate: Date;
  setSelectedDate: (d: Date) => void;
  hours: number[];
  getAppointmentsForDate: (d: Date) => Appointment[];
  onOpenCreate: (d: Date) => void;
  onOpenDetail: (apt: Appointment) => void;
  showAgent: boolean;
}) {
  return (
    <div className="bg-card rounded-2xl border border-border overflow-hidden">
      <div className="grid grid-cols-8 border-b border-border">
        <div className="p-3 text-xs font-bold text-muted-foreground">Hora</div>
        {days.map((d) => (
          <button
            key={d.toISOString()}
            type="button"
            onClick={() => setSelectedDate(d)}
            className={clsx(
              "p-3 text-left border-l border-border hover:bg-muted/40 transition-colors",
              isSameDay(d, selectedDate) && "bg-primary-50",
            )}
          >
            <div className="text-xs font-bold text-foreground capitalize">
              {format(d, "EEE", { locale: ptBR })}
            </div>
            <div className="text-xs text-muted-foreground">{format(d, "d MMM", { locale: ptBR })}</div>
          </button>
        ))}
      </div>
      <div className="max-h-[680px] overflow-auto">
        <div className="grid grid-cols-8">
          {hours.map((h) => (
            <div key={h} className="contents">
              <div className="p-3 text-xs font-bold text-muted-foreground border-b border-border">
                {String(h).padStart(2, "0")}:00
              </div>
              {days.map((d) => {
                const ds = d.toISOString().split("T")[0];
                const slots = getAppointmentsForDate(d).filter((a) => a.time.startsWith(String(h).padStart(2, "0")));
                return (
                  <div
                    key={`${ds}-${h}`}
                    className="p-2 border-b border-l border-border hover:bg-muted/30 transition-colors relative"
                    onDoubleClick={() => onOpenCreate(d)}
                  >
                    {slots.map((apt) => {
                      const cfg = TYPE_CONFIG[apt.type] ?? { ...DEFAULT_TYPE_CONFIG, label: apt.type || "Tipo desconhecido" };
                      return (
                        <button
                          key={apt.id}
                          type="button"
                          onClick={() => onOpenDetail(apt)}
                          className={clsx(
                            "w-full text-left px-2 py-1 rounded-lg text-xs font-bold truncate border",
                            cfg.bg,
                            cfg.border,
                            cfg.color,
                          )}
                        >
                          {apt.time} {apt.title}{showAgent && apt.agentName ? ` · ${apt.agentName}` : ""}
                        </button>
                      );
                    })}
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function DayView({
  date,
  hours,
  getAppointmentsForDate,
  onOpenCreate,
  onOpenDetail,
  showAgent,
}: {
  date: Date;
  hours: number[];
  getAppointmentsForDate: (d: Date) => Appointment[];
  onOpenCreate: () => void;
  onOpenDetail: (apt: Appointment) => void;
  showAgent: boolean;
}) {
  const dayApts = getAppointmentsForDate(date);
  return (
    <div className="bg-card rounded-2xl border border-border overflow-hidden">
      <div className="p-4 border-b border-border flex items-center justify-between">
        <div className="text-sm font-bold text-foreground capitalize">
          {format(date, "PPPP", { locale: ptBR })}
        </div>
        <Button variant="outline" size="sm" onClick={onOpenCreate}>
          <Plus className="w-4 h-4" /> Novo
        </Button>
      </div>
      <div className="max-h-[720px] overflow-auto">
        {hours.map((h) => {
          const hh = String(h).padStart(2, "0");
          const slots = dayApts.filter((a) => a.time.startsWith(hh));
          return (
            <div key={h} className="border-b border-border grid grid-cols-12">
              <div className="col-span-2 p-3 text-xs font-bold text-muted-foreground">
                {hh}:00
              </div>
              <div className="col-span-10 p-2">
                {slots.length === 0 ? (
                  <div className="h-10 rounded-lg bg-muted/30" />
                ) : (
                  <div className="space-y-2">
                    {slots.map((apt) => {
                      const cfg = TYPE_CONFIG[apt.type] ?? { ...DEFAULT_TYPE_CONFIG, label: apt.type || "Tipo desconhecido" };
                      return (
                        <button
                          key={apt.id}
                          type="button"
                          onClick={() => onOpenDetail(apt)}
                          className={clsx(
                            "w-full text-left px-3 py-2 rounded-xl text-sm font-bold truncate border",
                            cfg.bg,
                            cfg.border,
                            cfg.color,
                          )}
                        >
                          {apt.time} {apt.title}{apt.client ? ` • ${apt.client}` : ""}{showAgent && apt.agentName ? ` · ${apt.agentName}` : ""}
                        </button>
                      );
                    })}
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function ListView({
  appointments,
  onOpenDetail,
  onOpenCreate,
  showAgent,
}: {
  appointments: Appointment[];
  onOpenDetail: (apt: Appointment) => void;
  onOpenCreate: () => void;
  showAgent: boolean;
}) {
  return (
    <div className="bg-card rounded-2xl border border-border overflow-hidden">
      <div className="p-4 border-b border-border flex items-center justify-between">
        <div className="text-sm font-bold text-foreground">Todos os compromissos</div>
        <Button variant="outline" size="sm" onClick={onOpenCreate}>
          <Plus className="w-4 h-4" /> Novo
        </Button>
      </div>
      <div className="divide-y divide-border">
        {appointments
          .slice()
          .sort((a, b) => `${a.date}T${a.time}`.localeCompare(`${b.date}T${b.time}`))
          .map((apt) => {
            const cfg = TYPE_CONFIG[apt.type] ?? { ...DEFAULT_TYPE_CONFIG, label: apt.type || "Tipo desconhecido" };
            const statusCfg = STATUS_CONFIG[apt.status] ?? { ...STATUS_CONFIG.Pendente, label: apt.status || "Status desconhecido" };
            return (
              <button
                key={apt.id}
                type="button"
                onClick={() => onOpenDetail(apt)}
                className="w-full text-left p-4 hover:bg-muted/40 transition-colors"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className={clsx("px-2 py-0.5 rounded-md border text-caption font-bold", cfg.bg, cfg.border, cfg.color)}>
                        {cfg.label}
                      </span>
                      <span className={clsx("px-2 py-0.5 rounded-md text-caption font-bold", statusCfg.bg, statusCfg.color)}>
                        {statusCfg.label}
                      </span>
                    </div>
                    <p className="mt-2 text-sm font-bold text-foreground truncate">{apt.title}</p>
                    <p className="text-xs text-muted-foreground truncate">{apt.client}</p>
                    {showAgent && apt.agentName ? <p className="text-xs font-medium text-primary-700 truncate">Corretor: {apt.agentName}</p> : null}
                  </div>
                  <div className="text-right shrink-0">
                    <p className="text-xs font-bold text-foreground">
                      {format(parseISO(`${apt.date}T${apt.time}`), "dd/MM/yyyy HH:mm")}
                    </p>
                    {apt.location ? (
                      <p className="text-xs text-muted-foreground truncate max-w-[160px]">
                        {apt.location}
                      </p>
                    ) : null}
                  </div>
                </div>
              </button>
            );
          })}
        {appointments.length === 0 ? (
          <div className="p-6">
            <EmptyState
              icon={CalendarIcon}
              message="Nenhum compromisso encontrado."
              className="border-none bg-transparent"
            />
          </div>
        ) : null}
      </div>
    </div>
  );
}

function EventDetail({
  apt,
  agents,
  properties,
  leads,
  deals,
  onEdit,
  onDelete,
  onConfirm,
  onCancel,
  saving,
}: {
  apt: Appointment;
  agents: any[];
  properties: any[];
  leads: any[];
  deals: any[];
  onEdit: () => void;
  onDelete: () => void;
  onConfirm: () => void;
  onCancel: () => void;
  saving: boolean;
}) {
  const typeCfg = TYPE_CONFIG[apt.type] ?? { ...DEFAULT_TYPE_CONFIG, label: apt.type || "Tipo desconhecido" };
  const statusCfg = STATUS_CONFIG[apt.status] ?? { ...STATUS_CONFIG.Pendente, label: apt.status || "Status desconhecido" };
  const prioCfg = PRIORITY_CONFIG[apt.priority];

  const agent = agents?.find((a: any) => a.id === apt.agentId);
  const lead = leads?.find((l: any) => l.id === apt.leadId);
  const property = properties?.find((p: any) => p.id === apt.propertyId);
  const deal = deals?.find((d: any) => d.id === apt.contractId);

  return (
    <div className="flex flex-col h-full">
      <div className="px-6 py-5 space-y-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-sm font-bold text-foreground">{apt.title}</p>
            <p className="text-xs text-muted-foreground">{apt.client}</p>
          </div>
          <button type="button" disabled={Boolean(apt.inspectionId)} aria-label="Editar compromisso" onClick={onEdit} className="text-primary-600 hover:text-primary-700 disabled:hidden">
            <Edit3 className="w-4 h-4" />
          </button>
        </div>

        <div className="flex flex-wrap gap-2">
          <span className={clsx("px-2.5 py-1 rounded-md border text-caption font-bold uppercase tracking-wider", typeCfg.bg, typeCfg.border, typeCfg.color)}>
            {typeCfg.label}
          </span>
          <span className={clsx("px-2.5 py-1 rounded-md text-caption font-bold uppercase tracking-wider", statusCfg.bg, statusCfg.color)}>
            {statusCfg.label}
          </span>
          <span className={clsx("px-2.5 py-1 rounded-md text-caption font-bold uppercase tracking-wider", prioCfg.bg, prioCfg.color)}>
            <span className={clsx("inline-block w-2 h-2 rounded-full mr-1", prioCfg.dot)} />
            {apt.priority === "Media" ? "Média" : apt.priority}
          </span>
          {apt.visibility === "private" ? (
            <span className="px-2.5 py-1 rounded-md border border-border bg-muted text-caption font-bold uppercase tracking-wider text-muted-foreground">Particular · somente responsável</span>
          ) : null}
        </div>

        <div className="space-y-2">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Clock className="w-4 h-4" />
            <span>
              {apt.isAllDay
                ? `${format(parseISO(`${apt.date}T00:00:00`), "dd/MM/yyyy")} (dia inteiro)`
                : `${format(parseISO(`${apt.date}T${apt.time}`), "dd/MM/yyyy HH:mm")} - ${apt.endTime}`}
            </span>
          </div>
          {apt.location ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <MapPin className="w-4 h-4" />
              <span>{apt.location}</span>
            </div>
          ) : null}
          {agent ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <User className="w-4 h-4" />
              <span>{agent.name || agent.fullname || agent.email}</span>
            </div>
          ) : null}
          {lead?.phone ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Phone className="w-4 h-4" />
              <span>{formatPhoneBR(lead.phone)}</span>
            </div>
          ) : null}
        </div>

        {property ? (
          <div className="p-3 rounded-xl border border-border bg-muted/30">
            <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
              Imóvel
            </p>
            <p className="text-sm font-bold text-foreground truncate">
              {property.displayId ? `${property.displayId} - ${property.title}` : property.title}
            </p>
          </div>
        ) : null}

        {deal ? (
          <div className="p-3 rounded-xl border border-border bg-muted/30">
            <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
              Negócio
            </p>
            <p className="text-sm font-bold text-foreground truncate">
              {dealOptionLabel({
                deal,
                propertyTitle: property?.title,
                customerName: lead?.name,
              }).full}
            </p>
          </div>
        ) : null}

        {apt.description ? (
          <div className="p-3 rounded-xl border border-border bg-card">
            <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">
              Descrição
            </p>
            <p className="text-sm text-muted-foreground whitespace-pre-wrap">{apt.description}</p>
          </div>
        ) : null}
      </div>

      {apt.requestedByClient && apt.type === "Visita" && !["Cancelado", "Concluido"].includes(apt.status) && (
        <div className="px-6 py-4 border-t border-border space-y-2">
          <p className="text-xs text-muted-foreground">Solicitada pelo cliente no portal. Mudanças de horário e status aparecem na conta do cliente.</p>
          <div className="flex flex-wrap gap-2">
            {apt.status !== "Confirmado" && <Button onClick={onConfirm} disabled={saving}><CheckCircle2 className="w-4 h-4" /> Confirmar visita</Button>}
            <Button variant="outline" onClick={onEdit} disabled={saving}>Ajustar horário</Button>
            <Button variant="outline" onClick={onCancel} disabled={saving}>Cancelar visita</Button>
          </div>
        </div>
      )}
      {apt.inspectionId ? <div className="mt-auto border-t border-border px-6 py-4 space-y-2">
        <p className="text-label text-muted-foreground">Este compromisso acompanha a vistoria. Alterações devem ser feitas no contrato de origem.</p>
        <p className="text-label">Vistoriador: {apt.inspectorName || "A definir"}</p>
        {apt.inspectionContractId && <Button asChild><Link to={`/contratos/${apt.inspectionContractId}?aba=locacao&secao=vistorias`}>Abrir contrato da vistoria</Link></Button>}
      </div> : <div className="mt-auto px-6 py-4 border-t border-border flex items-center justify-end gap-2 flex-shrink-0">
        {/* Cópia pontual do compromisso no calendário de quem consulta. O próprio
            botão se esconde quando o compromisso não é elegível (pendente, cancelado
            ou sem horário de término). */}
        <AddToCalendarButton
          appointmentId={apt.id}
          status={apt.status}
          date={apt.date}
          time={apt.time}
          endTime={apt.endTime}
        />
        <Button
          variant="outline"
          size="icon"
          onClick={onDelete}
          className="border-status-critical/30 text-status-critical hover:bg-status-critical-soft hover:text-status-critical"
        >
          <Trash2 className="w-4 h-4" />
        </Button>
        <Button onClick={onEdit}>
          <Edit3 className="w-4 h-4" /> Editar
        </Button>
      </div>}
    </div>
  );
}
