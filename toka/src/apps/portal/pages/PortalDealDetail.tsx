import { usePortalContact } from "../hooks/usePortalContact";
import { useQueryClient } from "@tanstack/react-query";
import { portalKeys } from "../hooks/queryKeys";
import { newestDeal } from "../../../shared/deals/revisions";
import { usePortalDealQuery } from "../hooks/useDeals";
import { usePortalDealProperty, usePortalDealChecklist, usePortalDealAppointments } from "../hooks/useDealResources";
import { isAxiosError } from "axios";
import { ConfirmDialog } from "../../../shared/components/ConfirmDialog";
import { AddToCalendarButton, canAddToCalendar } from "../../../shared/components/AddToCalendarButton";
import { useEffect, useState, useCallback, useMemo } from "react";
import { useParams, useNavigate } from "react-router";
import {
  portalService,
  PortalAppointment,
  PortalPartyFieldsUpdateRequest,
  PortalPropertySummary,
  PortalSignatureView,
} from "../services/portal.service";
import { DealDTO, DealStatus, DealHistory, ChecklistItemDTO } from "../../../shared/services/deal.service";
import { ClicksignWidget } from "../../../shared/components/signature/ClicksignWidget";
import { SignatureStatusBadge } from "../../../shared/components/signature/SignatureStatusBadge";
import {
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  Bath,
  BedDouble,
  CalendarDays,
  CarFront,
  Check,
  CheckCircle2,
  ChevronDown,
  Clock,
  Download,
  Eye,
  FileText,
  Home,
  Loader2,
  MapPin,
  MessageSquareText,
  RefreshCw,
  Ruler,
  XCircle,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { downloadUrlAsFile } from "../../../shared/components/signature/useSignedContractDocument";
import { format as formatDateFns } from "date-fns";
import { CHECKLIST_CATEGORY_LABELS } from "../../../shared/data";
import { PropertyMediaCarousel } from "../components/PropertyMediaCarousel";
import { getPortalAccessToken } from "../services/portal-api";
import { usePortalWebSocket } from "../hooks/usePortalWebSocket";
import { toGatewayUrl, withGatewayToken } from "../../../shared/services/api";
import { fetchPortalContent, toPortalOwnedFileUrl } from "../services/portal-content";
import { usePortalAuth } from "../context/PortalAuthContext";
import { usePortalDeals } from "../context/PortalDealsContext";
import { AppointmentPickerDialog } from "../components/AppointmentPickerDialog";
import { PortalAgencySiteCard } from "../components/PortalAgencySiteCard";
import { PortalEmpty, PortalError, PortalLoading, PortalSection } from "../components/PortalState";
import { DealPartiesSection, DealSignersList } from "../components/DealPartiesSection";
import { DealStageStepper } from "../components/DealStageStepper";
import { Search as SearchIcon } from "lucide-react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "../../../shared/ui/dialog";
import { Textarea } from "../../../shared/ui/textarea";
import { Button } from "../../../shared/ui/button";
import { usePortalBack } from "../shell/usePortalBack";
import { FileDropzone } from "../components/FileDropzone";
import { DocumentViewerDialog } from "../../../shared/components/DocumentViewerDialog";
import { MoneyInput } from "../../../shared/ui/money-input";
import { parseCpfCnpj, isValidCpfCnpj } from "../../../shared/utils/masks";
import { DocumentInput } from "../../../shared/ui/document-input";
import { describeListingPrice, isRentListing } from "../../../shared/utils/listingPrice";
import {
  PORTAL_DEAL_STAGES,
  formatAppointmentDate,
  formatPortalDateTime,
  getAppointmentStatusMeta,
  getChecklistProgress,
  getPortalDealNextAction,
  getPortalDealStatus,
  sortAppointments,
  sortProposalsNewestFirst,
  type PortalDealTone,
} from "./PortalDealDetail.presentation";
import { PortalPage } from "../shell/PortalPage";
import { portalCopy, resolvePortalPersona, type PortalVisitCopy } from "../lib/portalPersona";

function formatDate(date: string | Date | undefined): string {
  if (!date) return "";
  try { return formatDateFns(new Date(date), "dd/MM/yyyy"); } catch { return String(date); }
}


function stageTimestamp(stage: DealStatus, history: DealHistory[] | undefined, createdAt: string): string | null {
  if (history && history.length > 0) {
    const entry = [...history].reverse().find((h) => h.status === stage);
    if (entry) return formatPortalDateTime(entry.timestamp);
  }
  if (stage === "NEGOTIATION") return formatPortalDateTime(createdAt);
  return null;
}

/** Mensagem do servidor quando existe; senão o texto genérico do fluxo. */
function uploadErrorMessage(error: unknown): string {
  const message = (error as { response?: { data?: { message?: string } } })?.response?.data?.message;
  return typeof message === "string" && message.trim() ? message : "Erro ao enviar documento.";
}

const TONE_STYLES: Record<PortalDealTone, { card: string; icon: string; badge: string }> = {
  neutral: {
    card: "border-border bg-card",
    icon: "bg-muted text-muted-foreground",
    badge: "bg-muted text-muted-foreground",
  },
  info: {
    card: "border-primary/20 bg-primary-50/60",
    icon: "bg-primary-100 text-primary-700",
    badge: "bg-primary-100 text-primary-700",
  },
  warning: {
    card: "border-status-warning/25 bg-status-warning-soft/60",
    icon: "bg-status-warning-soft text-status-warning",
    badge: "bg-status-warning-soft text-status-warning",
  },
  success: {
    card: "border-status-ok/25 bg-status-ok-soft/60",
    icon: "bg-status-ok-soft text-status-ok",
    badge: "bg-status-ok-soft text-status-ok",
  },
  critical: {
    card: "border-status-critical/25 bg-status-critical-soft/60",
    icon: "bg-status-critical-soft text-status-critical",
    badge: "bg-status-critical-soft text-status-critical",
  },
};

interface PropertyJourneyCardProps {
  property: PortalPropertySummary | null;
  error?: unknown;
  displayTitle: string;
  cityLine: string;
  onRetry?: () => void;
  onViewProperty?: () => void;
}

function PropertyJourneyCard({ property, error, displayTitle, cityLine, onRetry, onViewProperty }: PropertyJourneyCardProps) {
  const valor = describeListingPrice(
    property ? (isRentListing((property as any).listing_type) ? (property as any).rent_price : (property as any).sale_price) : 0,
    (property as any)?.listing_type,
  );
  return (
    <div className="rounded-2xl border border-border bg-card overflow-hidden shadow-sm">
      <div className="relative w-full h-44 bg-muted overflow-hidden">
        {((property as any)?.media?.images?.length || property?.image_url) ? (
          <PropertyMediaCarousel
            images={(property as any)?.media?.images}
            fallbackUrl={property?.image_url}
            alt={property?.title || "Imóvel"}
            className="w-full h-full"
            imgClassName="w-full h-full object-cover"
          />
        ) : (
          <div className="w-full h-full flex items-center justify-center text-muted-foreground"><Home className="w-8 h-8" /></div>
        )}
      </div>
      <div className="p-4">
        {error ? (
          <div className="flex items-center justify-between gap-2">
            <p className="text-sm text-muted-foreground">Não foi possível carregar o imóvel.</p>
            {onRetry && <Button size="sm" variant="outline" onClick={onRetry}>Tentar novamente</Button>}
          </div>
        ) : property ? (
          <>
            <div className="flex items-start justify-between gap-2">
              <div className="min-w-0">
                <p className="text-base font-bold text-foreground truncate">{displayTitle}</p>
                {cityLine && <p className="text-xs text-muted-foreground mt-0.5 truncate">{cityLine}</p>}
              </div>
              {valor.hasPrice && (
                <p className="flex shrink-0 items-baseline gap-1.5 text-base font-bold text-foreground tabular-nums">
                  {valor.label && (
                    <span className="text-caption font-bold uppercase tracking-wide text-muted-foreground">{valor.label}</span>
                  )}
                  <span>{valor.amount}</span>
                  {valor.period && <span className="text-xs font-normal text-muted-foreground">{valor.period}</span>}
                </p>
              )}
            </div>
            {onViewProperty && (
              <Button size="sm" variant="ghost" className="mt-3 w-full" onClick={onViewProperty}>
                <Eye className="w-4 h-4" /> Ver imóvel
              </Button>
            )}
          </>
        ) : (
          <p className="text-sm text-muted-foreground">Carregando informações do imóvel…</p>
        )}
      </div>
    </div>
  );
}

export function PortalDealDetail() {
  const goBack = usePortalBack("/portal");
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const queryClient = useQueryClient();
  const dealQuery = usePortalDealQuery(id);
  const propertyQuery = usePortalDealProperty(id);
  const checklistQuery = usePortalDealChecklist(id);
  const appointmentsQuery = usePortalDealAppointments(id);
  const property = propertyQuery.data ?? null;
  const checklist = checklistQuery.data ?? [];
  const appointments = appointmentsQuery.data ?? [];
  const setChecklist = useCallback((update: ChecklistItemDTO[] | ((current: ChecklistItemDTO[]) => ChecklistItemDTO[])) => {
    if (id) queryClient.setQueryData<ChecklistItemDTO[]>(portalKeys.dealChecklist(id), (current) => typeof update === "function" ? update(current ?? []) : update);
  }, [id, queryClient]);
  const setAppointments = useCallback((update: PortalAppointment[] | ((current: PortalAppointment[]) => PortalAppointment[])) => {
    if (id) queryClient.setQueryData<PortalAppointment[]>(portalKeys.dealAppointments(id), (current) => typeof update === "function" ? update(current ?? []) : update);
  }, [id, queryClient]);
  const loading = Boolean(id) && [dealQuery, propertyQuery, checklistQuery, appointmentsQuery].some((q) => q.isPending);
  const resourceErrors = { deal: dealQuery.isError, property: propertyQuery.isError,
    checklist: checklistQuery.isError, appointments: appointmentsQuery.isError };
  const [editingAppointment, setEditingAppointment] = useState<PortalAppointment | null>(null);
  const [cancellingAppointment, setCancellingAppointment] = useState<PortalAppointment | null>(null);
  const [busyAppointmentId, setBusyAppointmentId] = useState<string | null>(null);
  const editingAppointmentValues = useMemo(() => editingAppointment ? {
    date: new Date(`${editingAppointment.date}T12:00:00`),
    startTime: editingAppointment.time, endTime: editingAppointment.end_time,
    note: editingAppointment.description || "",
  } : undefined, [editingAppointment]);
  const [expandedStage, setExpandedStage] = useState<number | null>(null);
  const { user } = usePortalAuth();
  const { getDeal, invalidateDeal, updateDeal } = usePortalDeals();
  const cachedDeal = id ? getDeal(id) : null;
  const deal = cachedDeal && dealQuery.data ? newestDeal(dealQuery.data, cachedDeal) : dealQuery.data ?? cachedDeal;
  const { onMessage } = usePortalWebSocket();
  const [signatureRealtimeNonce, setSignatureRealtimeNonce] = useState(0);
  const [signatureView, setSignatureView] = useState<PortalSignatureView | null>(null);


  useEffect(() => {
    if (!id) return;
    const unsubscribe = onMessage((type, payload) => {
      if (type === "signature_update") {
        if (!payload?.deal_id || payload.deal_id === id) {
          setSignatureRealtimeNonce((n) => n + 1);
        }
      }
      if (type === "appointment_update" && (
        payload?.deal_id === id ||
        (user?.id && deal?.tenant_id && !payload?.deal_id && payload?.tenant_id === deal.tenant_id && payload?.customer_account_id === user?.id) ||
        appointments.some((appointment) => appointment.id === payload?.appointment_id)
      )) {
        void queryClient.invalidateQueries({ queryKey: portalKeys.dealAppointments(id) });
      }
    });
    return unsubscribe;
  }, [onMessage, id, queryClient, deal?.tenant_id, user?.id, appointments]);

  useEffect(() => {
    setEditingAppointment(null);
    setCancellingAppointment(null);
    setScheduleModalOpen(false);
  }, [id]);

  useEffect(() => {
    if (deal) {
      const idx = PORTAL_DEAL_STAGES.findIndex((s) => s.key === deal.status);
      if (idx >= 0) setExpandedStage(idx);
    }
  }, [deal?.status]);

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    portalService
      .getDealSignature(id)
      .then((v) => {
        if (!cancelled) setSignatureView(v);
      })
      .catch(() => {
        if (!cancelled) setSignatureView(null);
      });
    return () => {
      cancelled = true;
    };
  }, [id, signatureRealtimeNonce]);

  const updateChecklistPartyFields = useCallback(
    async (itemId: string, req: PortalPartyFieldsUpdateRequest) => {
      if (!id) return;
      const updatedDeal = await portalService.updateChecklistPartyFields(id, itemId, req);
      updateDeal(id, (prev) => ({ ...prev, parties: updatedDeal.parties, checklist: updatedDeal.checklist }));
      if (Array.isArray(updatedDeal.checklist)) {
        setChecklist(updatedDeal.checklist);
      }
    },
    [id, updateDeal],
  );

  const handleUpload = useCallback(
    async (itemId: string, file: File) => {
      if (!id) return;
      const previousItem = checklist.find((item) => item.id === itemId);
      try {
        const optimisticUploadedAt = new Date().toISOString();
        setChecklist((prev) =>
          prev.map((it) =>
            it.id === itemId
              ? {
                  ...it,
                  status: "UPLOADED",
                  file_name: file.name,
                  file_size: file.size,
                  content_type: file.type || it.content_type,
                  uploaded_at: optimisticUploadedAt,
                }
              : it,
          ),
        );
        updateDeal(id, (prev) => ({
          ...prev,
          checklist: (prev.checklist || []).map((it) =>
            it.id === itemId
              ? {
                  ...it,
                  status: "UPLOADED",
                  file_name: file.name,
                  file_size: file.size,
                  content_type: file.type || it.content_type,
                  uploaded_at: optimisticUploadedAt,
                }
              : it,
          ),
        }));
        await portalService.uploadChecklistItem(id, itemId, file);
        toast.success("Documento enviado com sucesso!");
        const c = await portalService.getDealChecklist(id);
        const checklistList = Array.isArray(c) ? c : [];
        setChecklist(checklistList);
        updateDeal(id, (prev) => ({ ...prev, checklist: checklistList }));
        await invalidateDeal(id);
      } catch (error) {
        if (previousItem) {
          const rollbackItem = previousItem;
          setChecklist((prev) => prev.map((item) => (item.id === itemId ? rollbackItem : item)));
          updateDeal(id, (prev) => ({
            ...prev,
            checklist: (prev.checklist || []).map((item) => (item.id === itemId ? rollbackItem : item)),
          }));
        }
        // O backend distingue tamanho (413), verificação de segurança (422) e
        // indisponibilidade do antivírus (503): a mensagem dele evita o "erro ao
        // enviar documento" sem causa.
        toast.error(uploadErrorMessage(error));
      }
    },
    [checklist, id, invalidateDeal, updateDeal],
  );

  const [scheduleModalOpen, setScheduleModalOpen] = useState(false);
  const [rejectModalOpen, setRejectModalOpen] = useState(false);
  const [rejectReason, setRejectReason] = useState("");
  const [rejectCounterValue, setRejectCounterValue] = useState<number | null>(null);
  const [selectedProposalId, setSelectedProposalId] = useState<string | null>(null);
  const [isSubmittingProposal, setIsSubmittingProposal] = useState(false);

  const handleAcceptProposal = async (proposalId: string) => {
    if (!id) return;
    try {
      setIsSubmittingProposal(true);
      await portalService.acceptProposal(id, proposalId);
      toast.success("Proposta aceita! A imobiliária será notificada.");
      updateDeal(id, (prev) => ({
        ...prev,
        proposals: (prev.proposals || []).map((p) => (p.id === proposalId ? { ...p, status: "ACCEPTED" } : p)),
      }));
      await invalidateDeal(id);
    } catch {
      toast.error("Erro ao aceitar proposta.");
    } finally {
      setIsSubmittingProposal(false);
    }
  };

  const handleRejectClick = (proposalId: string) => {
    setSelectedProposalId(proposalId);
    setRejectReason("");
    setRejectCounterValue(null);
    setRejectModalOpen(true);
  };

  const handleRejectConfirm = async () => {
    if (!id || !selectedProposalId) return;
    if (!rejectReason.trim()) {
      toast.error("O motivo é obrigatório.");
      return;
    }
    const counterValue = rejectCounterValue === null ? undefined : rejectCounterValue;
    try {
      setIsSubmittingProposal(true);
      await portalService.rejectProposal(id, selectedProposalId, rejectReason, counterValue);
      toast.success("Contraproposta enviada com sucesso.");
      const nextStatus = counterValue != null ? "COUNTER_OFFER" : "REJECTED";
      updateDeal(id, (prev) => ({
        ...prev,
        proposals: (prev.proposals || []).map((p) =>
          p.id === selectedProposalId ? { ...p, status: nextStatus, notes: rejectReason, counter_value: counterValue } : p,
        ),
      }));
      await invalidateDeal(id);
      setRejectModalOpen(false);
    } catch {
      toast.error("Erro ao recusar proposta.");
    } finally {
      setIsSubmittingProposal(false);
    }
  };

  const refreshAppointmentsAfterAction = useCallback(async () => {
    if (id) await queryClient.invalidateQueries({ queryKey: portalKeys.dealAppointments(id) });
  }, [id, queryClient]);

  const visitActionError = (err: unknown, fallback: string) => {
    const message = isAxiosError<{ message?: string }>(err) && (err.response?.status ?? 500) < 500
      ? err.response?.data?.message : undefined;
    return message || fallback;
  };

  const handleScheduleSubmit = async (data: { date: Date; startTime: string; endTime: string; note?: string }) => {
    if (!id) return;
    const editing = editingAppointment;
    setBusyAppointmentId(editing?.id || null);
    try {
      const payload = { date: formatDateFns(data.date, "yyyy-MM-dd"), time: data.startTime,
        end_time: data.endTime, description: data.note || "" };
      const updated = editing
        ? await portalService.updateDealAppointment(id, editing.id, { ...payload, expected_updated_at: editing.updated_at! })
        : await portalService.createDealAppointment(id, { ...payload, title: "Visita", type: "Visita" });
      await queryClient.cancelQueries({ queryKey: portalKeys.dealAppointments(id), exact: true });
      setAppointments((current) => editing ? current.map((a) => a.id === updated.id ? updated : a) : [...current, updated]);
      toast.success(editing ? "Horário da visita alterado!" : "Visita solicitada!");
      setScheduleModalOpen(false);
      setEditingAppointment(null);
      void refreshAppointmentsAfterAction();
    } catch (err) {
      const message = visitActionError(err, editing ? "Não foi possível alterar o horário da visita." : "Não foi possível agendar a visita.");
      toast.error(message);
      throw new Error(message);
    } finally { setBusyAppointmentId(null); }
  };

  const handleCancelAppointment = async () => {
    if (!id || !cancellingAppointment) return;
    setBusyAppointmentId(cancellingAppointment.id);
    try {
      const updated = await portalService.cancelDealAppointment(id, cancellingAppointment.id, cancellingAppointment.updated_at!);
      await queryClient.cancelQueries({ queryKey: portalKeys.dealAppointments(id), exact: true });
      setAppointments((current) => current.map((a) => a.id === updated.id ? updated : a));
      toast.success("Visita cancelada.");
      setCancellingAppointment(null);
      void refreshAppointmentsAfterAction();
    } catch (err) {
      toast.error(visitActionError(err, "Não foi possível cancelar a visita."));
      throw err;
    } finally { setBusyAppointmentId(null); }
  };

  const { openContact, opening: chatLoadingDeal } = usePortalContact();

  // Only an explicitly identified party can select a lead. Otherwise the
  // deal endpoint resolves access and the responsible broker server-side.
  const myLeadId = useMemo(() => {
    const parties = deal?.parties ?? [];
    const mine = parties.find((party) => party.id === deal?.my_party_id);
    return mine?.lead_id ?? "";
  }, [deal?.my_party_id, deal?.parties]);

  const handleOpenChatDeal = () => openContact({ leadId: myLeadId, dealId: deal?.id });

  const currentIdx = useMemo(() => {
    if (!deal) return -1;
    return PORTAL_DEAL_STAGES.findIndex((s) => s.key === deal.status);
  }, [deal]);

  const scrollToStage = useCallback((status: DealStatus) => {
    const stageIndex = PORTAL_DEAL_STAGES.findIndex((stage) => stage.key === status);
    if (stageIndex < 0) return;
    setExpandedStage(stageIndex);
    if (typeof window !== "undefined") {
      window.requestAnimationFrame(() => {
        document.getElementById(`portal-stage-${status.toLowerCase()}`)?.scrollIntoView({
          behavior: "smooth",
          block: "start",
        });
      });
    }
  }, []);

  const retryLoad = useCallback(() => {
    if (id) void queryClient.invalidateQueries({ queryKey: portalKeys.deal(id) });
  }, [id, queryClient]);

  if (loading) {
    return <PortalLoading rows={5} className="mx-auto max-w-6xl sm:px-6 lg:px-8" />;
  }

  if (!deal) {
    return (
      <PortalPage width="xl" className="px-5 pt-8 sm:px-6 overflow-x-hidden">
        <Button variant="ghost" onClick={() => navigate("/portal")} className="mb-6 -ml-3 rounded-xl">
          <ArrowLeft className="h-4 w-4" aria-hidden="true" />
          Voltar ao início
        </Button>
        {resourceErrors.deal ? (
          <PortalError
            title="Não foi possível carregar esta jornada"
            description="Sua página continua disponível. Tente carregar novamente ou volte ao início do portal."
            onRetry={retryLoad}
          />
        ) : (
          <PortalEmpty
            icon={SearchIcon}
            title="Negócio não encontrado"
            description="Esse negócio pode ter sido encerrado ou você não tem mais acesso a ele."
            action={
              <Button variant="outline" onClick={() => navigate("/portal")}>
                Ir para o início
              </Button>
            }
          />
        )}
      </PortalPage>
    );
  }


  const displayTitle = property?.address || property?.title || "Minha transação";
  const cityLine = property ? [property.neighborhood, property.city, property.state].filter(Boolean).join(", ") : "";
  const dealStatus = getPortalDealStatus(deal.status);
  // Vocabulário deste negócio: a persona sai do papel do cliente neste negócio
  // (`my_role`) cruzado com o tipo (SALE/RENT) — vendedor e locador mudam de
  // texto, comprador e locatário mudam de fluxo.
  const persona = resolvePortalPersona({ role: deal.my_role, dealType: deal.type });
  const copy = portalCopy(persona);
  const nextAction = getPortalDealNextAction({
    status: deal.status,
    checklist,
    proposals: deal.proposals,
    appointments,
    signature: signatureView,
    visits: copy.visits,
  });
  const checklistProgress = getChecklistProgress(checklist);
  const partialErrorCount = [resourceErrors.deal, resourceErrors.property, resourceErrors.checklist, resourceErrors.appointments].filter(Boolean).length;
  const handleNextAction = () => {
    if (nextAction.action === "schedule") {
      setScheduleModalOpen(true);
      return;
    }
    if (nextAction.stage) scrollToStage(nextAction.stage);
  };
  const appointmentPropertyLabel = property
    ? [property.title, property.address, [property.neighborhood, property.city, property.state].filter(Boolean).join(", ")]
        .filter(Boolean)
        .join(" · ")
    : "";

  return (
    <PortalPage width="wide" className="overflow-x-hidden">
      <header className="sticky top-0 z-20 border-b border-border/80 bg-background/95 backdrop-blur">
        <div className="mx-auto flex max-w-6xl items-center gap-3 px-4 py-3 sm:px-6 lg:px-8">
          <Button
            variant="ghost"
            size="icon"
            onClick={goBack}
            aria-label="Voltar"
            title="Voltar"
            className="size-10 shrink-0 rounded-full text-muted-foreground hover:bg-muted"
          >
            <ArrowLeft className="h-5 w-5" aria-hidden="true" />
          </Button>
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-center gap-2">
              <h1 className="truncate text-lg font-bold leading-tight tracking-tight text-foreground sm:text-xl">Sua jornada</h1>
              {deal.my_role && (
                <span className="hidden whitespace-nowrap rounded-full border border-border bg-muted px-2 py-1 text-micro font-bold text-foreground/80 sm:inline-flex">
                  {persona.label}
                </span>
              )}
            </div>
            <p className="mt-0.5 truncate text-xs text-muted-foreground sm:text-sm">{displayTitle}</p>
          </div>
          <span className={clsx("inline-flex shrink-0 items-center gap-1.5 rounded-full px-2.5 py-1 text-caption font-bold", TONE_STYLES[dealStatus.tone].badge)}>
            <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
            {dealStatus.label}
          </span>
        </div>
      </header>

      <div className="mx-auto max-w-6xl px-4 py-5 sm:px-6 sm:py-7 lg:px-8">
        {partialErrorCount > 0 && (
          <div role="status" className="mb-5 flex flex-col gap-3 rounded-2xl border border-status-warning/25 bg-status-warning-soft/60 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex items-start gap-3">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-status-warning" aria-hidden="true" />
              <div>
                <p className="text-sm font-semibold text-foreground">Algumas informações não foram atualizadas</p>
                <p className="mt-0.5 text-xs text-muted-foreground">Você pode continuar navegando e tentar carregar os dados novamente.</p>
              </div>
            </div>
            <Button variant="outline" size="sm" onClick={retryLoad} className="self-start bg-card sm:self-auto">
              <RefreshCw className="h-3.5 w-3.5" aria-hidden="true" />
              Tentar novamente
            </Button>
          </div>
        )}

        <div className="grid items-start gap-6 overflow-x-hidden lg:grid-cols-[minmax(0,1fr)_320px]">
          <aside className="space-y-4 min-w-0 lg:sticky lg:top-24 lg:col-start-2 lg:row-start-1">
            <div className={clsx("rounded-3xl border p-5 shadow-sm", TONE_STYLES[nextAction.tone].card)}>
              <div className={clsx("mb-4 flex h-10 w-10 items-center justify-center rounded-2xl", TONE_STYLES[nextAction.tone].icon)}>
                {nextAction.tone === "warning" || nextAction.tone === "critical" ? (
                  <AlertTriangle className="h-5 w-5" aria-hidden="true" />
                ) : nextAction.tone === "success" ? (
                  <CheckCircle2 className="h-5 w-5" aria-hidden="true" />
                ) : (
                  <ArrowRight className="h-5 w-5" aria-hidden="true" />
                )}
              </div>
              <p className="text-caption font-bold uppercase tracking-wide text-muted-foreground">Próxima ação</p>
              <h2 className="mt-1 text-lg font-bold text-foreground">{nextAction.title}</h2>
              <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{nextAction.description}</p>
              {nextAction.label && (
                <Button type="button" onClick={handleNextAction} className="mt-5 h-11 w-full rounded-xl font-bold">
                  {nextAction.label}
                  <ArrowRight className="h-4 w-4" aria-hidden="true" />
                </Button>
              )}
            </div>

            <PortalAgencySiteCard tenantId={deal.tenant_id} />

            <PortalSection title="Acesso rápido" subtitle="Resolva tudo sem sair da sua jornada">
              <div className="grid gap-2">
                <Button variant="outline" onClick={handleOpenChatDeal} disabled={chatLoadingDeal} className="h-11 justify-start rounded-xl">
                  {chatLoadingDeal ? <Loader2 className="h-4 w-4 animate-spin" /> : <MessageSquareText className="h-4 w-4" />}
                  Falar com a imobiliária
                </Button>
                <Button variant="outline" onClick={() => scrollToStage("NEGOTIATION")} className="h-11 justify-start rounded-xl">
                  <CalendarDays className="h-4 w-4" aria-hidden="true" />
                  Ver agendamentos{appointments.length > 0 ? ` (${appointments.length})` : ""}
                </Button>
                {deal.status !== "CANCELLED" && deal.status !== "CONVERTED" && (
                  <Button variant="outline" onClick={() => setScheduleModalOpen(true)} className="h-11 justify-start rounded-xl">
                    <CalendarDays className="h-4 w-4" aria-hidden="true" />
                    Solicitar visita
                  </Button>
                )}
              </div>
            </PortalSection>

            <PortalSection title="Resumo do processo">
              <dl className="space-y-3 text-sm">
                <div className="flex items-start justify-between gap-4">
                  <dt className="text-muted-foreground flex-shrink-0">Etapa atual</dt>
                  <dd className="text-right font-semibold text-foreground min-w-0 truncate max-w-[60%]">
                    {PORTAL_DEAL_STAGES.find((stage) => stage.key === deal.status)?.label ?? dealStatus.label}
                  </dd>
                </div>
                <div className="flex items-start justify-between gap-4">
                  <dt className="text-muted-foreground flex-shrink-0">Início</dt>
                  <dd className="text-right font-semibold text-foreground min-w-0 truncate max-w-[60%]">{formatPortalDateTime(deal.created_at) ?? "Não informado"}</dd>
                </div>
                {checklistProgress.total > 0 && (
                  <div className="flex items-start justify-between gap-4">
                    <dt className="text-muted-foreground">Documentos</dt>
                    <dd className="text-right font-semibold text-foreground">{checklistProgress.percent}% enviados</dd>
                  </div>
                )}
                <div className="flex items-start justify-between gap-4">
                  <dt className="text-muted-foreground">Identificação</dt>
                  <dd className="text-right font-semibold text-foreground">{deal.display_id || `#${deal.id.slice(0, 8)}`}</dd>
                </div>
              </dl>
            </PortalSection>
          </aside>

          <main className="space-y-6 min-w-0 lg:col-start-1 lg:row-start-1">
            <PropertyJourneyCard
              property={property}
              error={resourceErrors.property && !property}
              displayTitle={displayTitle}
              cityLine={cityLine}
              onRetry={retryLoad}
              onViewProperty={property?.id ? () => navigate(`/portal/imoveis/${property.id}`) : undefined}
            />

            <PortalSection title="Etapas da jornada" subtitle="Abra cada etapa para ver detalhes, pendências e histórico">
              <DealStageStepper status={deal.status} onStageClick={scrollToStage} className="border-0 p-0 shadow-none" />

              <div className="relative mt-6 border-t border-border pt-5">
                {PORTAL_DEAL_STAGES.map((stage, i) => {
                  const isCompleted = currentIdx >= 0 && i < currentIdx;
                  const isCurrent = i === currentIdx;
                  const isFuture = currentIdx >= 0 && i > currentIdx;
                  const isExpanded = expandedStage === i;
                  const subtitle = stageTimestamp(stage.key, deal.history, deal.created_at);
                  const stageState = isCurrent ? "Etapa atual" : isCompleted ? "Concluída" : isFuture ? "Próxima etapa" : "Não iniciada";
                  const panelId = `portal-stage-panel-${stage.key.toLowerCase()}`;

                  return (
                    <div id={`portal-stage-${stage.key.toLowerCase()}`} key={stage.key} className="relative flex scroll-mt-28 gap-3 sm:gap-4">
                      {i < PORTAL_DEAL_STAGES.length - 1 && (
                        <div className={clsx("absolute bottom-[-4px] left-[13px] top-9 w-px", isCompleted ? "bg-foreground/40" : "bg-border")} aria-hidden="true" />
                      )}

                      <div className="relative z-10 mt-1 shrink-0 bg-card py-1" aria-hidden="true">
                        {isCompleted ? (
                          <div className="flex h-7 w-7 items-center justify-center rounded-full bg-foreground">
                            <Check className="h-4 w-4 text-background" strokeWidth={3} />
                          </div>
                        ) : isCurrent ? (
                          <div className="flex h-7 w-7 items-center justify-center rounded-full border-2 border-status-ok bg-card ring-4 ring-status-ok/10">
                            <div className="h-2.5 w-2.5 rounded-full bg-status-ok" />
                          </div>
                        ) : (
                          <div className="h-7 w-7 rounded-full border-2 border-border bg-card" />
                        )}
                      </div>

                      <div className="min-w-0 flex-1 pb-5">
                        <button
                          type="button"
                          onClick={() => setExpandedStage(isExpanded ? null : i)}
                          aria-expanded={isExpanded}
                          aria-controls={panelId}
                          className="flex w-full items-start justify-between gap-3 rounded-xl px-2 py-1.5 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                        >
                          <div className="min-w-0">
                            <div className="flex flex-wrap items-center gap-2">
                              <p className={clsx("text-title font-bold tracking-tight", isFuture ? "text-muted-foreground" : "text-foreground")}>{stage.label}</p>
                              <span className={clsx("rounded-full px-2 py-0.5 text-micro font-bold", isCurrent ? "bg-status-ok-soft text-status-ok" : "bg-muted text-muted-foreground")}>{stageState}</span>
                            </div>
                            {subtitle && <p className="mt-0.5 text-xs font-medium text-muted-foreground">Desde {subtitle}</p>}
                          </div>
                          <ChevronDown className={clsx("mt-1 h-4 w-4 shrink-0 text-muted-foreground transition-transform duration-200", isExpanded && "rotate-180")} aria-hidden="true" />
                        </button>

                        {isExpanded && (
                          <div id={panelId}>
                            <StageDetails
                              stageKey={stage.key}
                              deal={deal}
                              checklist={checklist}
                              appointments={appointments}
                              checklistError={resourceErrors.checklist && !checklistQuery.data}
                              appointmentsError={resourceErrors.appointments && !appointmentsQuery.data}
                              onRetry={retryLoad}
                              onUpload={handleUpload}
                              onUpdatePartyFields={updateChecklistPartyFields}
                              onCreateAppointment={() => { setEditingAppointment(null); setScheduleModalOpen(true); }}
                              onRescheduleAppointment={(a) => { setEditingAppointment(a); setScheduleModalOpen(true); }}
                              onCancelAppointment={setCancellingAppointment}
                              busyAppointmentId={busyAppointmentId}
                              onOpenChat={handleOpenChatDeal}
                              isChatLoading={chatLoadingDeal}
                              onAcceptProposal={handleAcceptProposal}
                              onRejectProposal={handleRejectClick}
                              userEmail={user?.email}
                              isSubmittingProposal={isSubmittingProposal}
                              signatureRealtimeNonce={signatureRealtimeNonce}
                              signatureView={signatureView}
                              appointmentPropertyLabel={appointmentPropertyLabel}
                              visits={copy.visits}
                            />
                          </div>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            </PortalSection>

            <DealPartiesSection
              parties={deal.parties}
              signers={deal.signers}
              myPartyId={deal.my_party_id}
              myEmail={user?.email}
              signatureSnapshot={signatureView ?? undefined}
            />
          </main>
        </div>
      </div>

      <AppointmentPickerDialog
        open={scheduleModalOpen}
        onOpenChange={(open) => { setScheduleModalOpen(open); if (!open) setEditingAppointment(null); }}
        mode={editingAppointment ? "reschedule" : "create"}
        defaultValues={editingAppointmentValues}
        onSubmit={handleScheduleSubmit}
        propertyLabel={editingAppointment?.property || property?.title || property?.address}
      />

      <ConfirmDialog open={Boolean(cancellingAppointment)} title="Cancelar visita?" variant="warning"
        description={<>{cancellingAppointment?.property || property?.title}<br />
          {cancellingAppointment && formatAppointmentDate(cancellingAppointment.date)} · {cancellingAppointment?.time}
          <br />A visita será cancelada também na agenda da imobiliária. Para mudar o horário, use “Alterar horário”.</>}
        confirmLabel="Cancelar visita" cancelLabel="Manter visita"
        onCancel={() => setCancellingAppointment(null)} onConfirm={handleCancelAppointment} />

      <Dialog open={rejectModalOpen} onOpenChange={setRejectModalOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Recusar / Contraproposta</DialogTitle>
          </DialogHeader>
          <div className="py-4">
            <div className="mb-3">
              <label className="text-xs font-bold text-foreground/80">Valor da contraproposta (opcional)</label>
              <MoneyInput
                withSymbol
                value={rejectCounterValue ?? undefined}
                onChange={(v) => setRejectCounterValue(v ?? null)}
                placeholder="R$ 0,00"
                className="mt-2 w-full h-11 rounded-xl border border-border px-3 text-sm outline-none bg-card"
              />
            </div>
            <Textarea
              placeholder="Digite o motivo ou sua contraproposta..."
              value={rejectReason}
              onChange={(e) => setRejectReason(e.target.value)}
              className="min-h-[100px]"
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRejectModalOpen(false)} disabled={isSubmittingProposal}>
              Cancelar
            </Button>
            <Button onClick={handleRejectConfirm} disabled={isSubmittingProposal}>
              {isSubmittingProposal ? "Enviando..." : "Enviar"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <div className="h-4" />
    </PortalPage>
  );
}

/** Status em que a visita já não aceita alteração nem cancelamento pelo cliente. */
const CLOSED_APPOINTMENT_STATUSES = ["Cancelado", "Concluido", "Concluído", "Em Andamento"];

/** A visita ainda pode ser alterada/cancelada pelo próprio cliente. */
function canManageAppointment(appt: PortalAppointment): boolean {
  return Boolean(appt.can_manage && appt.updated_at && !CLOSED_APPOINTMENT_STATUSES.includes(appt.status));
}

/**
 * O cartão mostra a linha de ações quando o cliente pode gerir a visita OU
 * quando o compromisso marcado já pode virar arquivo de calendário — baixar o
 * `.ics` é leitura, não depende de `can_manage` (compromisso de outro tipo, como
 * vistoria, também entra).
 */
function showsAppointmentActions(appt: PortalAppointment): boolean {
  return canManageAppointment(appt)
    || canAddToCalendar({ status: appt.status, date: appt.date, time: appt.time, endTime: appt.end_time });
}

function StageDetails({
  stageKey,
  deal,
  checklist,
  appointments,
  checklistError,
  appointmentsError,
  onRetry,
  onUpload,
  onUpdatePartyFields,
  onCreateAppointment,
  onRescheduleAppointment,
  onCancelAppointment,
  busyAppointmentId,
  onOpenChat,
  isChatLoading,
  onAcceptProposal,
  onRejectProposal,
  userEmail,
  isSubmittingProposal,
  signatureRealtimeNonce,
  signatureView,
  appointmentPropertyLabel,
  visits,
}: {
  stageKey: DealStatus;
  deal: DealDTO;
  checklist: ChecklistItemDTO[];
  appointments: PortalAppointment[];
  checklistError: boolean;
  appointmentsError: boolean;
  onRetry: () => void;
  onUpload: (id: string, file: File) => void;
  onUpdatePartyFields: (id: string, req: PortalPartyFieldsUpdateRequest) => Promise<void>;
  onCreateAppointment: () => void;
  onRescheduleAppointment: (a: PortalAppointment) => void;
  onCancelAppointment: (a: PortalAppointment) => void;
  busyAppointmentId: string | null;
  onOpenChat: () => void;
  isChatLoading: boolean;
  onAcceptProposal: (id: string) => void;
  onRejectProposal: (id: string) => void;
  userEmail?: string;
  isSubmittingProposal: boolean;
  signatureRealtimeNonce?: number;
  signatureView?: PortalSignatureView | null;
  appointmentPropertyLabel?: string;
  /** Quem pode agendar visita neste negócio (a fonte é o dicionário por persona). */
  visits: PortalVisitCopy;
}) {
  if (stageKey === "NEGOTIATION") {
    const statusConfig: Record<string, string> = {
      Agendado: "bg-muted text-muted-foreground",
      Confirmado: "bg-status-ok-soft text-status-ok",
      Concluido: "bg-status-info-soft text-status-info",
      "Concluído": "bg-status-info-soft text-status-info",
      Cancelado: "bg-status-critical-soft text-status-critical",
      Reagendado: "bg-status-warning-soft text-status-warning",
      Pendente: "bg-muted text-muted-foreground",
    };
    return (
      <div className="mt-3 p-4 bg-muted/50 border border-border rounded-2xl">
        <p className="text-label text-muted-foreground mb-3">{visits.sectionIntro}</p>
        <div className={clsx("grid grid-cols-1 gap-2 mb-3", visits.canSchedule && "sm:grid-cols-2")}>
          {visits.canSchedule && (
            <Button
              onClick={onCreateAppointment}
              className="h-10 rounded-xl bg-status-ok hover:bg-status-ok/90 text-white font-semibold text-sm"
            >
              <CalendarDays className="w-4 h-4" /> Agendar visita
            </Button>
          )}
          <Button
            onClick={onOpenChat}
            disabled={isChatLoading}
            className="h-10 rounded-xl bg-primary hover:bg-primary-600 text-primary-foreground font-semibold text-sm"
          >
            {isChatLoading ? <Loader2 className="w-4 h-4 animate-spin" /> : <MessageSquareText className="w-4 h-4" />} Falar com corretor
          </Button>
        </div>
        {appointmentsError ? (
          <div className="flex items-start justify-between gap-3 rounded-xl border border-status-warning/25 bg-status-warning-soft/60 p-3">
            <div className="flex min-w-0 items-start gap-2">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-status-warning" aria-hidden="true" />
              <p className="text-label text-muted-foreground">Não foi possível carregar seus agendamentos.</p>
            </div>
            <Button type="button" size="sm" variant="outline" onClick={onRetry} className="shrink-0 bg-card">
              Tentar novamente
            </Button>
          </div>
        ) : appointments && appointments.length > 0 ? (
          <div className="space-y-2">
            {sortAppointments(appointments).map((appt) => (
              <div key={appt.id} className="grid grid-cols-[2.5rem_minmax(0,1fr)] items-start gap-3 p-3 bg-card border border-border rounded-xl shadow-sm sm:grid-cols-[2.5rem_minmax(0,1fr)_auto]">
                <div className="w-10 h-10 rounded-full bg-status-ok-soft flex items-center justify-center text-status-ok flex-shrink-0">
                  <Clock className="w-5 h-5" />
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-bold text-foreground">{appt.title || "Visita Agendada"}</p>
                  <p className="text-xs text-muted-foreground">
                    {formatAppointmentDate(appt.date)} · {appt.time}{appt.end_time ? `–${appt.end_time}` : ""}
                  </p>
                  {(appt.property || appointmentPropertyLabel) && (
                    <p className="text-xs text-muted-foreground mt-1 truncate">{appt.property || appointmentPropertyLabel}</p>
                  )}

                </div>
                <span
                  className={clsx(
                    "col-start-2 justify-self-start px-2 py-1 rounded-lg text-[11px] font-bold whitespace-nowrap sm:col-start-3 sm:row-start-1",
                    statusConfig[appt.status] || "bg-muted text-muted-foreground",
                  )}
                >
                  {appt.status === "Pendente" ? "Aguardando confirmação" : appt.status}
                </span>
                  {showsAppointmentActions(appt) && (
                    <div className="col-span-2 flex flex-wrap gap-2 sm:col-span-1 sm:col-start-2">
                      {canManageAppointment(appt) && (
                        <>
                          <Button type="button" size="sm" variant="outline" disabled={Boolean(busyAppointmentId)} onClick={() => onRescheduleAppointment(appt)}>Alterar horário</Button>
                          <Button type="button" size="sm" variant="outline" className="text-status-critical" disabled={Boolean(busyAppointmentId)} onClick={() => onCancelAppointment(appt)}>
                            {busyAppointmentId === appt.id ? "Cancelando…" : "Cancelar visita"}
                          </Button>
                        </>
                      )}
                      <AddToCalendarButton
                        appointmentId={appt.id}
                        status={appt.status}
                        date={appt.date}
                        time={appt.time}
                        endTime={appt.end_time}
                        load={() => portalService.downloadDealAppointmentCalendar(deal.id, appt.id)}
                      />
                    </div>
                  )}
              </div>
            ))}
          </div>
        ) : (
          <p className="text-label text-muted-foreground italic">Nenhuma visita agendada.</p>
        )}
      </div>
    );
  }

  if (stageKey === "PROPOSAL") {
    const latestProposal = deal.proposals && deal.proposals.length > 0 ? deal.proposals[deal.proposals.length - 1] : null;
    const proposals = deal.proposals || [];
    const proposalStatusConfig = {
      DRAFT: { label: "Rascunho", className: "bg-muted text-foreground/80" },
      SENT: { label: "Enviada", className: "bg-status-info-soft text-status-info" },
      ACCEPTED: { label: "Aceita", className: "bg-status-ok-soft text-status-ok" },
      REJECTED: { label: "Rejeitada", className: "bg-status-critical-soft text-status-critical" },
      COUNTER_OFFER: { label: "Contraproposta", className: "bg-status-warning-soft text-status-warning" },
    } as const;

    return (
      <div className="mt-3 p-4 bg-muted/50 border border-border rounded-2xl">
        {latestProposal ? (
          <div className="bg-card border border-border rounded-xl p-4 shadow-sm">
            <h4 className="text-sm font-bold text-foreground mb-2">Proposta Atual</h4>
            <div className="space-y-1 mb-4">
              <p className="text-label text-muted-foreground flex justify-between">
                <span>Valor:</span>
                <span className="font-semibold text-foreground">
                  {latestProposal.value.toLocaleString("pt-BR", { style: "currency", currency: "BRL" })}
                </span>
              </p>
              <p className="text-label text-muted-foreground flex justify-between">
                <span>Pagamento:</span>
                <span className="font-semibold text-foreground">{latestProposal.payment_method}</span>
              </p>
              <p className="text-label text-muted-foreground flex justify-between">
                <span>Status:</span>
                <span
                  className={clsx(
                    "text-caption font-bold px-2 py-0.5 rounded",
                    proposalStatusConfig[latestProposal.status]?.className ?? "bg-muted text-foreground/80",
                  )}
                >
                  {proposalStatusConfig[latestProposal.status]?.label ?? latestProposal.status}
                </span>
              </p>
              {latestProposal.status === "COUNTER_OFFER" && latestProposal.counter_value != null && (
                <p className="text-label text-muted-foreground flex justify-between">
                  <span>Contraproposta:</span>
                  <span className="font-semibold text-status-warning">
                    {latestProposal.counter_value.toLocaleString("pt-BR", { style: "currency", currency: "BRL" })}
                  </span>
                </p>
              )}
              {latestProposal.notes && (
                <p className="text-xs text-muted-foreground mt-2 bg-muted/50 p-2 rounded-lg italic">"{latestProposal.notes}"</p>
              )}
            </div>

            {latestProposal.status === "ACCEPTED" ? (
              <div className="p-3 rounded-xl bg-status-ok-soft text-status-ok text-label font-semibold">
                Proposta aceita. A equipe foi notificada.
              </div>
            ) : latestProposal.status === "REJECTED" ? (
              <div className="p-3 rounded-xl bg-status-critical-soft text-status-critical text-label font-semibold">
                Proposta rejeitada. A equipe foi notificada.
              </div>
            ) : latestProposal.status === "SENT" || latestProposal.status === "COUNTER_OFFER" ? (
              <div className="flex gap-2">
                <Button
                  onClick={() => onAcceptProposal(latestProposal.id)}
                  disabled={isSubmittingProposal}
                  className="flex-1 h-10 text-label font-bold text-white bg-status-ok rounded-lg hover:bg-status-ok/90 transition-colors"
                >
                  {isSubmittingProposal ? "Aguarde..." : "Aceitar"}
                </Button>
                <Button
                  variant="outline"
                  onClick={() => onRejectProposal(latestProposal.id)}
                  disabled={isSubmittingProposal}
                  className="flex-1 h-10 text-label font-bold text-foreground/80 rounded-lg border-border hover:bg-muted/50 transition-colors"
                >
                  Contraproposta
                </Button>
              </div>
            ) : null}
          </div>
        ) : (
          <p className="text-label text-muted-foreground">Aguardando o envio da primeira proposta.</p>
        )}

        {proposals.length > 0 && (
          <div className="mt-3 space-y-2">
            <p className="text-xs font-bold text-foreground/80">Histórico</p>
            <div className="space-y-2">
              {proposals
                .slice()
                .reverse()
                .map((p, idx) => (
                  <div key={p.id ?? idx} className="bg-card border border-border rounded-xl p-3 shadow-sm">
                    <div className="flex items-center justify-between gap-3">
                      <p className="text-label font-bold text-foreground">
                        {p.value.toLocaleString("pt-BR", { style: "currency", currency: "BRL" })}
                      </p>
                      <span
                        className={clsx(
                          "text-caption font-bold px-2 py-0.5 rounded",
                          proposalStatusConfig[p.status]?.className ?? "bg-muted text-foreground/80",
                        )}
                      >
                        {proposalStatusConfig[p.status]?.label ?? p.status}
                      </span>
                    </div>
                    <p className="text-xs text-muted-foreground mt-0.5">
                      {p.payment_method} · {formatDate(p.created_at)}
                    </p>
                    {p.status === "COUNTER_OFFER" && p.counter_value != null && (
                      <p className="text-xs font-semibold text-status-warning mt-1">
                        Contraproposta: {p.counter_value.toLocaleString("pt-BR", { style: "currency", currency: "BRL" })}
                      </p>
                    )}
                    {p.notes && <p className="text-xs text-muted-foreground mt-1 italic">"{p.notes}"</p>}
                  </div>
                ))}
            </div>
          </div>
        )}
      </div>
    );
  }

  if (stageKey === "ANALYSIS") {
    if (checklistError) {
      return (
        <div className="mt-3 flex items-start justify-between gap-3 rounded-2xl border border-status-warning/25 bg-status-warning-soft/60 p-4">
          <div className="flex min-w-0 items-start gap-2">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-status-warning" aria-hidden="true" />
            <div>
              <p className="text-sm font-semibold text-foreground">Documentos indisponíveis</p>
              <p className="mt-0.5 text-label text-muted-foreground">Não foi possível carregar as pendências desta análise.</p>
            </div>
          </div>
          <Button type="button" size="sm" variant="outline" onClick={onRetry} className="shrink-0 bg-card">
            Tentar novamente
          </Button>
        </div>
      );
    }
    const myPartyId = deal.my_party_id;
    const relevantItems = checklist
      .filter((c) => c.category === "financeiro" || c.category === "pessoal" || c.category === "imovel" || c.category === "juridico" || c.category === "outros")
      .filter((c) => !c.party_id || !myPartyId || c.party_id === myPartyId);
    if (relevantItems.length === 0) {
      return (
        <div className="mt-3 p-3 bg-muted/50 rounded-2xl">
          <p className="text-label text-muted-foreground">
            {deal.due_diligence?.status === "APPROVED"
              ? "Análise aprovada com sucesso."
              : deal.due_diligence?.score_serasa
                ? `Score Serasa: ${deal.due_diligence.score_serasa}`
                : "Aguardando documentos para iniciar a análise."}
          </p>
        </div>
      );
    }
    return (
      <div className="mt-3 space-y-2">
        {relevantItems.map((item) => (
          <ChecklistRow key={item.id} item={item} onUpload={onUpload} onUpdatePartyFields={onUpdatePartyFields} dealParties={deal.parties} />
        ))}
      </div>
    );
  }

  if (stageKey === "CONTRACT") {
    if (deal.status !== "CONTRACT" && deal.status !== "CONVERTED") {
      return (
        <div className="mt-3 p-3 bg-muted/50 rounded-2xl">
          <p className="text-label text-muted-foreground">O contrato será gerado após a aprovação da análise.</p>
        </div>
      );
    }

    const mySignerInfo = deal.signers?.find((s) => s.email === userEmail);

    return (
      <div className="mt-3 p-4 bg-muted/50 border border-border rounded-2xl space-y-4">
        <p className="text-label text-muted-foreground">O contrato está pronto para assinatura digital.</p>

        {mySignerInfo && (
          <ClientSignaturePanel dealId={deal.id} userEmail={userEmail} mySignerInfo={mySignerInfo} realtimeNonce={signatureRealtimeNonce} />
        )}

        <DealSignersList signers={deal.signers} myEmail={userEmail} signatureSnapshot={signatureView ?? undefined} />
      </div>
    );
  }

  const historyEntry = deal.history?.slice().reverse().find((h) => h.status === stageKey);
  if (historyEntry?.notes) {
    return (
      <div className="mt-3 p-3 bg-muted/50 rounded-2xl">
        <p className="text-label text-muted-foreground">{historyEntry.notes}</p>
      </div>
    );
  }

  return null;
}

function RequiredMark({ on }: { on?: boolean }) {
  return on ? <span className="text-status-critical"> *</span> : null;
}

function ChecklistRow({
  item,
  onUpload,
  onUpdatePartyFields,
  dealParties,
}: {
  item: ChecklistItemDTO;
  onUpload: (id: string, file: File) => void;
  onUpdatePartyFields: (id: string, req: PortalPartyFieldsUpdateRequest) => Promise<void>;
  dealParties?: DealDTO["parties"];
}) {
  const [viewerOpen, setViewerOpen] = useState(false);
  const [isSavingFields, setIsSavingFields] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const viewerUrl = useMemo(() => {
    if (!item.file_url) return "";
    // O arquivo do checklist é gravado com a rota de OPERADOR
    // (`/api/v1/deals/:id/checklist/:itemId/download`, gate `deals:read`), que o
    // cliente não atravessa. A rota do portal autoriza pela identidade dele.
    const portalUrl = toPortalOwnedFileUrl(item.file_url);
    return withGatewayToken(toGatewayUrl(portalUrl), getPortalAccessToken());
  }, [item.file_url]);

  const partyId = item.party_id;
  const myParty = useMemo(() => (dealParties || []).find((p) => p.id === partyId), [dealParties, partyId]);
  // Memoizado: `item.requested_fields ?? []` cria um array NOVO a cada render
  // e, como `customFields` e o effect de seed dependem dele, o formulário
  // re-seedava os campos a cada tecla — apagando o que o cliente digitava.
  const requestedFields = useMemo(() => item.requested_fields ?? [], [item.requested_fields]);
  const baseField = (key: string) => requestedFields.find((f) => (f.key ?? "").trim() === key);
  const fullNameField = baseField("full_name");
  const cpfField = baseField("cpf");
  const birthField = baseField("birth_date");
  const phoneField = baseField("phone");
  const isFormOnly = item.request_mode === "form";
  const customFields = useMemo(
    () =>
      requestedFields.filter((f) => {
        const k = (f.key ?? "").trim();
        return k !== "full_name" && k !== "cpf" && k !== "birth_date" && k !== "phone";
      }),
    [requestedFields],
  );

  const [fullName, setFullName] = useState("");
  const [cpf, setCpf] = useState("");
  const [birthDate, setBirthDate] = useState("");
  const [phone, setPhone] = useState("");
  const [customValues, setCustomValues] = useState<Record<string, string>>({});

  useEffect(() => {
    setFullName(myParty?.name ?? "");
    setCpf(parseCpfCnpj(myParty?.document ?? ""));
    setBirthDate(myParty?.birth_date ?? "");
    setPhone(myParty?.phone ?? "");
    const seed: Record<string, string> = {};
    for (const f of customFields) {
      const key = (f.key ?? "").trim();
      if (!key) continue;
      seed[key] = myParty?.custom_fields?.[key] ?? "";
    }
    setCustomValues(seed);
    setFieldErrors({});
  }, [myParty?.name, myParty?.document, myParty?.birth_date, myParty?.phone, myParty?.custom_fields, customFields]);

  const requiresField = (f?: { required?: boolean }) => !!f?.required;
  const isFilled = (v: string) => v.trim() !== "";

  // Labels dos campos obrigatórios ainda NÃO salvos na parte. Baseia-se na
  // parte PERSISTIDA (myParty), não no formulário — preserva a semântica
  // "salvar antes de enviar" e evita 422 no backend.
  const missingRequiredPersistedLabels = (): string[] => {
    const labels: string[] = [];
    if (fullNameField && requiresField(fullNameField) && !isFilled(myParty?.name ?? "")) labels.push("Nome completo");
    if (cpfField && requiresField(cpfField) && !isFilled(myParty?.document ?? "")) labels.push(cpfField.label || "CPF");
    if (birthField && requiresField(birthField) && !isFilled(myParty?.birth_date ?? "")) labels.push("Data de nascimento");
    if (phoneField && requiresField(phoneField) && !isFilled(myParty?.phone ?? "")) labels.push(phoneField.label || "Telefone/WhatsApp");
    for (const f of customFields) {
      const key = (f.key ?? "").trim();
      if (!key) continue;
      if (requiresField(f) && !isFilled(myParty?.custom_fields?.[key] ?? "")) labels.push(f.label || key);
    }
    return labels;
  };

  // Valida o formulário e devolve erros por campo (chave do campo → mensagem).
  const validateForm = (): Record<string, string> => {
    const errs: Record<string, string> = {};
    if (fullNameField && requiresField(fullNameField) && !isFilled(fullName)) errs.full_name = "Campo obrigatório";
    if (cpfField) {
      if (requiresField(cpfField) && !isFilled(cpf)) errs.cpf = "Campo obrigatório";
      else if (isFilled(cpf) && !isValidCpfCnpj(cpf)) errs.cpf = "CPF/CNPJ inválido";
    }
    if (birthField && requiresField(birthField) && !isFilled(birthDate)) errs.birth_date = "Campo obrigatório";
    if (phoneField && requiresField(phoneField) && !isFilled(phone)) errs.phone = "Campo obrigatório";
    for (const f of customFields) {
      const key = (f.key ?? "").trim();
      if (!key) continue;
      if (requiresField(f) && !isFilled(customValues[key] ?? "")) errs[key] = "Campo obrigatório";
    }
    return errs;
  };

  const cpfInvalid = !!cpfField && isFilled(cpf) && !isValidCpfCnpj(cpf);
  const missingLabels = missingRequiredPersistedLabels();
  const canUpload = requestedFields.length === 0 || missingLabels.length === 0;
  const showFields = requestedFields.length > 0 && item.party_id;
  const hasRequired = requestedFields.some((f) => requiresField(f));

  const inputClass = (hasError?: boolean) =>
    clsx(
      "w-full px-3 py-2 bg-card border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary",
      hasError ? "border-status-critical" : "border-border",
    );

  const handleSaveFields = async () => {
    if (!item.party_id) return;
    const errs = validateForm();
    setFieldErrors(errs);
    if (Object.keys(errs).length > 0) return;
    setIsSavingFields(true);
    try {
      await onUpdatePartyFields(item.id, {
        name: fullName.trim() || undefined,
        document: parseCpfCnpj(cpf) || undefined,
        birth_date: birthDate.trim() || undefined,
        phone: phone.trim() || undefined,
        custom_fields: customValues,
      });
      toast.success(isFormOnly ? "Dados enviados — não é necessário anexar arquivo." : "Dados atualizados.");
    } catch {
      toast.error("Não foi possível salvar os dados.");
    } finally {
      setIsSavingFields(false);
    }
  };

  return (
    <div className="bg-card border border-border rounded-2xl p-4 flex flex-col gap-3">
      <div className="flex items-start gap-3 min-w-0">
        <div className="mt-0.5">
          {item.status === "APPROVED" && <CheckCircle2 className="w-5 h-5 text-status-ok flex-shrink-0" />}
          {item.status === "UPLOADED" && <Clock className="w-5 h-5 text-status-warning flex-shrink-0" />}
          {item.status === "PENDING" && <FileText className="w-5 h-5 text-muted-foreground/70 flex-shrink-0" />}
          {item.status === "REJECTED" && <XCircle className="w-5 h-5 text-status-critical flex-shrink-0" />}
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-bold text-foreground leading-snug">{item.name}</p>
          <p className="text-xs text-muted-foreground mt-0.5">
            {CHECKLIST_CATEGORY_LABELS[item.category as keyof typeof CHECKLIST_CATEGORY_LABELS] || item.category}
          </p>
          {item.status === "REJECTED" && item.rejection_reason && (
            <p className="text-xs text-status-critical mt-1 font-medium">Recusado: {item.rejection_reason}</p>
          )}
          {(item.status === "APPROVED" || item.status === "UPLOADED") && item.file_url && (
            <>
              <Button
                type="button"
                variant="link"
                size="sm"
                onClick={() => setViewerOpen(true)}
                className="mt-2 h-auto p-0 text-xs font-bold text-status-ok hover:text-status-ok"
              >
                Visualizar documento enviado
              </Button>
              {viewerUrl && (
                <DocumentViewerDialog
                  open={viewerOpen}
                  onOpenChange={setViewerOpen}
                  url={viewerUrl}
                  title={item.name}
                  contentType={item.content_type || null}
                  fetchContent={fetchPortalContent}
                />
              )}
            </>
          )}
        </div>
      </div>

      {showFields && (
        <div className="space-y-3 rounded-xl border border-border bg-muted/40 p-3">
          <div>
            <p className="text-xs font-bold text-foreground/80">
              {isFormOnly ? "Preencha os dados para enviar (sem anexo)" : "Preencha os dados antes de enviar"}
            </p>
            {hasRequired && (
              <p className="text-[11px] text-muted-foreground mt-0.5">
                Campos com <span className="text-status-critical">*</span> são obrigatórios.
              </p>
            )}
          </div>
          {fullNameField && (
            <div className="space-y-1">
              <label className="text-xs font-bold text-foreground">
                Nome completo<RequiredMark on={requiresField(fullNameField)} />
              </label>
              <input
                value={fullName}
                onChange={(e) => setFullName(e.target.value)}
                className={inputClass(!!fieldErrors.full_name)}
                placeholder="Digite o nome completo"
              />
              {fieldErrors.full_name && (
                <p className="text-xs text-status-critical font-medium">{fieldErrors.full_name}</p>
              )}
            </div>
          )}
          {cpfField && (
            <div className="space-y-1">
              <label className="text-xs font-bold text-foreground">
                {cpfField.label || "CPF"}<RequiredMark on={requiresField(cpfField)} />
              </label>
              <DocumentInput
                value={cpf}
                onChange={setCpf}
                className={inputClass(!!fieldErrors.cpf || cpfInvalid)}
                placeholder="Digite o CPF ou CNPJ"
              />
              {(fieldErrors.cpf || cpfInvalid) && (
                <p className="text-xs text-status-critical font-medium">{fieldErrors.cpf || "CPF/CNPJ inválido"}</p>
              )}
            </div>
          )}
          {birthField && (
            <div className="space-y-1">
              <label className="text-xs font-bold text-foreground">
                Data de nascimento<RequiredMark on={requiresField(birthField)} />
              </label>
              <input
                type="date"
                value={birthDate}
                onChange={(e) => setBirthDate(e.target.value)}
                className={inputClass(!!fieldErrors.birth_date)}
              />
              {fieldErrors.birth_date && (
                <p className="text-xs text-status-critical font-medium">{fieldErrors.birth_date}</p>
              )}
            </div>
          )}
          {phoneField && (
            <div className="space-y-1">
              <label className="text-xs font-bold text-foreground">
                {phoneField.label || "Telefone/WhatsApp"}<RequiredMark on={requiresField(phoneField)} />
              </label>
              <input
                value={phone}
                onChange={(e) => setPhone(e.target.value)}
                className={inputClass(!!fieldErrors.phone)}
                placeholder="Digite o telefone/WhatsApp"
              />
              {fieldErrors.phone && (
                <p className="text-xs text-status-critical font-medium">{fieldErrors.phone}</p>
              )}
            </div>
          )}
          {customFields.map((f) => {
            const key = (f.key ?? "").trim();
            if (!key) return null;
            const inputType =
              (f.type ?? "text").trim() === "date"
                ? "date"
                : (f.type ?? "text").trim() === "number"
                  ? "number"
                  : "text";
            return (
              <div key={key} className="space-y-1">
                <label className="text-xs font-bold text-foreground">
                  {f.label || key}<RequiredMark on={requiresField(f)} />
                </label>
                <input
                  type={inputType}
                  value={customValues[key] ?? ""}
                  onChange={(e) => setCustomValues((prev) => ({ ...prev, [key]: e.target.value }))}
                  className={inputClass(!!fieldErrors[key])}
                  placeholder={`Digite ${f.label || key}`}
                />
                {fieldErrors[key] && <p className="text-xs text-status-critical font-medium">{fieldErrors[key]}</p>}
              </div>
            );
          })}
          <div className="flex justify-end">
            <Button type="button" size="sm" onClick={handleSaveFields} disabled={isSavingFields || !item.party_id}>
              {isSavingFields ? "Salvando..." : isFormOnly ? "Enviar dados (sem anexo)" : "Salvar dados"}
            </Button>
          </div>
        </div>
      )}

      {isFormOnly ? (
        (item.status === "UPLOADED" && (
          <p className="text-xs text-status-ok font-medium">
            ✓ Dados enviados sem anexo — aguardando verificação da imobiliária.
          </p>
        ))
      ) : (
        (item.status === "PENDING" || item.status === "REJECTED" || item.status === "UPLOADED") && (
          <div className="space-y-2">
            {!canUpload && (
              <div className="rounded-lg border border-status-warning/30 bg-status-warning-soft px-3 py-2">
                <p className="text-xs text-status-warning font-medium">
                  Para enviar o arquivo, preencha e salve {missingLabels.length === 1 ? "o campo" : "os campos"}: {missingLabels.join(", ")}.
                </p>
              </div>
            )}
            <FileDropzone
              className="mt-1"
              currentFile={
                item.status === "UPLOADED" && item.file_url
                  ? { name: item.name, url: withGatewayToken(toGatewayUrl(toPortalOwnedFileUrl(item.file_url)), getPortalAccessToken()) }
                  : null
              }
              disabled={!canUpload}
              maxSize={portalService.maxUploadBytes}
              onUpload={async (file) => onUpload(item.id, file)}
            />
          </div>
        )
      )}
    </div>
  );
}

function ClientSignaturePanel({
  dealId,
  mySignerInfo,
  realtimeNonce,
}: {
  dealId: string;
  userEmail?: string;
  mySignerInfo: { role?: string; status?: string };
  realtimeNonce?: number;
}) {
  const [view, setView] = useState<PortalSignatureView | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshNonce, setRefreshNonce] = useState(0);
  const [contractViewerOpen, setContractViewerOpen] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      setLoading(true);
      try {
        const v = await portalService.getDealSignature(dealId);
        if (!cancelled) setView(v);
      } catch {
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    load();
    return () => {
      cancelled = true;
    };
  }, [dealId, refreshNonce, realtimeNonce]);

  // A URL do contrato assinado é pré-assinada e vale ~5 minutos: uma aba aberta
  // por mais tempo baixava o XML de "Request has expired" do storage. Antes de
  // abrir ou baixar, o painel relê o view — a consulta gera um link NOVO.
  const loadFreshSignature = useCallback(async (): Promise<PortalSignatureView | null> => {
    try {
      const v = await portalService.getDealSignature(dealId);
      setView(v);
      return v;
    } catch {
      return null;
    }
  }, [dealId]);

  const role = view?.my_signer?.role ?? mySignerInfo.role ?? "";
  const myStatus = view?.my_signer?.status ?? mySignerInfo.status ?? "PENDING";
  const signerKey = view?.my_signer?.request_signature_key;
  const alreadySigned = myStatus === "SIGNED";
  const otherSigners = view?.other_signers ?? [];

  return (
    <div className="bg-card border border-border rounded-xl p-4 shadow-sm">
      <h4 className="text-sm font-bold text-foreground mb-1">Sua Assinatura</h4>
      <div className="flex items-center justify-between mb-3">
        <span className="text-xs text-muted-foreground capitalize">Papel: {role}</span>
        <SignatureStatusBadge kind="signer" size="sm" status={myStatus} />
      </div>

      {loading ? (
        <p className="text-xs text-muted-foreground">Carregando informações da assinatura…</p>
      ) : !view?.has_envelope ? (
        <p className="text-xs text-muted-foreground">
          O contrato ainda não foi enviado para assinatura. Você será notificado assim que estiver disponível.
        </p>
      ) : alreadySigned ? (
        (() => {
          const pendingOthers = otherSigners.filter((s) => s.status !== "SIGNED");
          const allDone = pendingOthers.length === 0;
          const summary =
            otherSigners.length === 0
              ? "Sua assinatura foi registrada. O contrato foi finalizado."
              : allDone
                ? `Sua assinatura foi registrada. Contrato finalizado (${view.signed_count}/${view.total_count}).`
                : `Sua assinatura foi registrada. Aguardando ${pendingOthers.length} de ${view.total_count} assinatura(s):`;
          return (
            <div className="space-y-2">
              <p className="text-xs text-muted-foreground">{summary}</p>
              {otherSigners.length > 0 && (
                <ul className="space-y-1">
                  {otherSigners.map((s) => (
                    <li key={s.signer_id} className="flex items-center justify-between text-xs">
                      <span className="truncate">
                        <span className="font-medium text-foreground">{s.name}</span>
                        {s.role && <span className="text-muted-foreground"> · {s.role}</span>}
                      </span>
                      <SignatureStatusBadge kind="signer" size="sm" status={s.status} />
                    </li>
                  ))}
                </ul>
              )}
              {view.signed_document_url && (
                <>
                  <div className="flex flex-wrap items-center gap-2 mt-2">
                    <button
                      type="button"
                      onClick={async () => {
                        const fresca = await loadFreshSignature();
                        if (fresca?.signed_document_url) setContractViewerOpen(true);
                      }}
                      className="inline-flex items-center gap-2 px-3 py-2 bg-card border border-border rounded-lg text-xs font-bold text-foreground hover:bg-muted transition-colors"
                    >
                      <Eye className="w-4 h-4" /> Visualizar contrato assinado
                    </button>
                    <button
                      type="button"
                      onClick={async () => {
                        const fresca = await loadFreshSignature();
                        const url = fresca?.signed_document_url;
                        if (!url) return;
                        const ok = await downloadUrlAsFile(url, "Contrato assinado.pdf");
                        if (!ok) toast.error("Não foi possível baixar o contrato assinado.");
                      }}
                      className="inline-flex items-center gap-2 px-3 py-2 bg-card border border-border rounded-lg text-xs font-bold text-foreground hover:bg-muted transition-colors"
                    >
                      <Download className="w-4 h-4" /> Baixar
                    </button>
                  </div>
                  <DocumentViewerDialog
                    open={contractViewerOpen}
                    onOpenChange={setContractViewerOpen}
                    url={view.signed_document_url}
                    title="Contrato assinado"
                    contentType="application/pdf"
                  />
                </>
              )}
            </div>
          );
        })()
      ) : !signerKey ? (
        <p className="text-xs text-muted-foreground">
          Estamos finalizando a preparação do envelope. Atualize esta página em alguns segundos.
          <Button
            variant="link"
            onClick={() => setRefreshNonce((n) => n + 1)}
            className="ml-2 h-auto p-0 align-baseline text-foreground underline"
          >
            Atualizar agora
          </Button>
        </p>
      ) : (
        <div>
          {otherSigners.length > 0 && (
            <p className="mb-2 text-xs text-muted-foreground">
              Progresso: {view.signed_count} de {view.total_count} assinatura(s).
            </p>
          )}
          <ClicksignWidget
            signerKey={signerKey}
            environment={view.environment === "production" ? "production" : "sandbox"}
            onSigned={() => {
              toast.success("Assinatura registrada! Aguardando os demais signatários.");
              setRefreshNonce((n) => n + 1);
            }}
            onClose={() => setRefreshNonce((n) => n + 1)}
          />
        </div>
      )}
    </div>
  );
}
