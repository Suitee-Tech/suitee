import { useParams, useNavigate, Link } from "react-router";
import {
  ArrowLeft,
  ArrowRight,
  Mail,
  Phone,
  Building2,
  Wallet,
  Calendar,
  CalendarPlus,
  Pencil,
  Trash2,
  Clock,
  MessageSquare,
  CheckCircle2,
  FileText,
  Edit,
  MoreVertical,
  X,
  Save,
  User,
  Users,
  Tag,
  Home,
  Handshake,
  BedDouble,
  Bath,
  Car,
  Maximize2,
  MapPin,
  DollarSign,
  TrendingUp,
  Briefcase,
  ClipboardList,
  ExternalLink,
  AlertCircle,
  Eye,
  Search,
  Key,
  UserCheck,
  Shield,
  ChevronRight,
  ChevronsUpDown,
  Check,
  Wrench,
  CalendarClock,
  CalendarDays,
  Crown,
} from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "../../shared/ui/popover";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "../../shared/ui/command";
import { leadService, ApiLead, LeadPropertyInterest, ClientRole, UpdateLeadDTO } from "../../shared/services/lead.service";
import { LeadStatus, Lead, LeadType, LeadSource, resolvePropertyDisplay, type Appointment } from "../../shared/data";
import { clsx } from "clsx";
import { useState, useEffect, useMemo, useCallback } from "react";
import { useAuthUser } from "../../shared/context/AuthUserContext";
import type { EsteiraDeal } from "../../shared/deals/types";
import { useLeadsContext } from "../../shared/context/LeadsContext";
import { useAgentsContext } from "../../shared/context/AgentsContext";
import { useEsteiraDeals } from "../../shared/hooks/useEsteiraDeals";
import { usePropertiesContext } from "../../shared/context/PropertiesContext";
import { useContractsContext } from "../../shared/context/ContractsContext";
import { useCalendarContext } from "../../shared/context/CalendarContext";
import { LeadAppointmentPanel } from "../../shared/components/calendar/LeadAppointmentPanel";
import { EsteiraDealModal } from "../../shared/components/EsteiraDealModal";
import { CaptacaoWizard } from "../../shared/components/CaptacaoWizard";
import { SharedBadge } from "../../shared/components/SharedBadge";
import { CounterpartyLancamentosLink } from "../../shared/components/lancamentos/CounterpartyLancamentosLink";
import { EmptyState } from "../../shared/ui/EmptyState";
import { toast } from "sonner";

function formatBRDate(iso?: string): string {
  if (!iso) return "";
  const [y, m, d] = iso.split("-");
  if (!y || !m || !d) return iso;
  return `${d}/${m}/${y}`;
}
import { ContractStatus } from "../../shared/contracts/types";
import { Avatar, AvatarImage, AvatarFallback } from "../../shared/ui/avatar";
import { Button } from "../../shared/ui/button";
import { ClientContextDrawer } from "../../shared/components/ClientContextDrawer";
import { MatchedPropertiesCard } from "../../shared/components/MatchedPropertiesCard";
import { MediaImage } from "../../shared/components/MediaImage";
import { usePermissionsContext } from "../../shared/context/PermissionsContext";
import { useSubscriptionAccess } from "../../shared/context/SubscriptionAccessContext";
import { formatPhoneBR, parsePhoneBR } from "../../shared/utils/masks";
import { formatCurrencyBRL0, parseMoneyBR0 } from "../../shared/utils/format";
import { getAvatarThumbUrl } from "../../shared/media";
import { useQuery } from "@tanstack/react-query";
import { fetchRentalsList, rentalsListQueryKey } from "../../shared/rentals/queries";
import { chargeService, type Charge } from "../../shared/services/charge.service";
import { commissionService, type Commission } from "../../shared/services/commission.service";
import { maintenanceService, type MaintenanceTicket } from "../../shared/services/maintenance.service";
import { unwrapApiData } from "../../shared/services/api";
import { collectPages } from "../../shared/services/collectPages";
import { formatBRL, formatLeadDate } from "../../shared/lib/leads/format";
import { leadStatusLabel } from "../../shared/lib/leads/labels";
import { resolveContractCommissionPct, resolveLeadChargeAmount } from "./leadFinancialReferences";

const PIPELINE_STAGES: LeadStatus[] = [
  "Novo",
  "Em Atendimento",
  "Qualificado",
  "Visita Agendada",
  "Em Negociação",
  "Proposta Enviada",
  "Convertido",
];

const LEAD_TYPE_CONFIG: Record<
  LeadType,
  { label: string; color: string; bg: string; icon: typeof Home; roleLabel: string }
> = {
  captacao: { label: "Captação", color: "text-status-warning", bg: "bg-status-warning-soft border-status-warning/30", icon: Key, roleLabel: "Proprietário" },
  compra_venda: { label: "Compra e Venda", color: "text-status-ok", bg: "bg-status-ok-soft border-status-ok/30", icon: Handshake, roleLabel: "Comprador" },
  locacao: { label: "Locação", color: "text-status-info", bg: "bg-status-info-soft border-status-info/30", icon: Home, roleLabel: "Inquilino" },
};

const SOURCE_LABELS: Record<string, string> = {
  Site: "Site",
  Indicacao: "Indicação",
  "Redes Sociais": "Redes Sociais",
  Manual: "Manual",
  Campanha: "Campanha",
  Outros: "Outros",
};

type Tab = "info" | "negocios" | "agenda" | "documentos" | "locacoes" | "cobrancas" | "tickets";

const fmt = formatBRL;
const fmtDate = (d: string) => formatLeadDate(d, "numeric");

export function LeadDetails() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { user } = useAuthUser();
  const { leads, updateLead, updateLeadStatus } = useLeadsContext();
  const { agents } = useAgentsContext();
  const { deals } = useEsteiraDeals();
  const { properties } = usePropertiesContext();
  const { contracts } = useContractsContext();
  const { appointments, updateAppointment, deleteAppointment } = useCalendarContext();
  // Agenda no contexto do lead: criar/editar sem sair da página (a aba Agenda
  // era só leitura e o caminho para marcar uma visita passava pela agenda).
  const [appointmentPanel, setAppointmentPanel] = useState<{ open: boolean; appointment: Appointment | null }>({
    open: false,
    appointment: null,
  });
  const [appointmentBusy, setAppointmentBusy] = useState<string | null>(null);

  // Ações rápidas da agenda ficam na própria lista do lead (o painel cobre
  // criar/editar; confirmar, cancelar e remover não precisam abrir nada).
  const handleAppointmentStatus = useCallback(
    async (apt: Appointment, status: "Confirmado" | "Cancelado") => {
      setAppointmentBusy(apt.id);
      try {
        const saved = await updateAppointment(apt.id, { status });
        if (!saved) return;
        toast.success(status === "Confirmado" ? "Visita confirmada. O cliente foi notificado." : "Visita cancelada");
      } finally {
        setAppointmentBusy(null);
      }
    },
    [updateAppointment],
  );

  const handleAppointmentRemove = useCallback(
    async (apt: Appointment) => {
      setAppointmentBusy(apt.id);
      try {
        const ok = await deleteAppointment(apt.id);
        if (ok) toast.success("Compromisso removido");
      } finally {
        setAppointmentBusy(null);
      }
    },
    [deleteAppointment],
  );
  const { can } = usePermissionsContext();
  const { hasFeature } = useSubscriptionAccess();
  const canUseClientPortal = hasFeature("client_portal");

  const originalLead = leads.find((l) => l.id === id);

  const [activeTab, setActiveTab] = useState<Tab>("info");
  const [isEditing, setIsEditing] = useState(false);
  const [isCreatingDeal, setIsCreatingDeal] = useState(false);
  const [isCaptacaoWizardOpen, setIsCaptacaoWizardOpen] = useState(false);
  const [lead, setLead] = useState<Lead | null>(originalLead || null);
  const [noteText, setNoteText] = useState("");
  const [isSubmittingNote, setIsSubmittingNote] = useState(false);
  const [history, setHistory] = useState<any[]>([]);
  const [apiLead, setApiLead] = useState<ApiLead | null>(null);
  const [isConverting, setIsConverting] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [brokerComboOpen, setBrokerComboOpen] = useState(false);
  const [isClientDrawerOpen, setIsClientDrawerOpen] = useState(false);
  const [isDisqualifyOpen, setIsDisqualifyOpen] = useState(false);
  const [disqualifyReason, setDisqualifyReason] = useState("Sem renda comprovada");
  const [disqualifyNote, setDisqualifyNote] = useState("");
  const [isDisqualifying, setIsDisqualifying] = useState(false);
  const [isInvitingClient, setIsInvitingClient] = useState(false);

  const [editInterest, setEditInterest] = useState<LeadPropertyInterest>({});

  useEffect(() => {
    if (originalLead) setLead(originalLead);
  }, [originalLead]);

  const fetchLeadDetails = useCallback(async () => {
    if (!id) return;
    try {
      const data = await leadService.getById(id);
      setApiLead(data);
      if (data.property_interest) setEditInterest(data.property_interest);

      if (data.history) {
        const mappedHistory = data.history
          .map((h) => ({
            id: h.id,
            type: h.action === "NOTE" ? "note" : h.action === "STATUS_CHANGED" ? "status" : "system",
            badge:
              h.action === "NOTE"
                ? { label: "Nota", bg: "bg-primary-50", border: "border-primary-200", color: "text-primary-700" }
                : h.action === "STATUS_CHANGED"
                  ? { label: "Status", bg: "bg-status-warning-soft", border: "border-status-warning/30", color: "text-status-warning" }
                  : { label: "Sistema", bg: "bg-muted", border: "border-border", color: "text-muted-foreground" },
            title:
              h.action === "NOTE"
                ? "Nota adicionada"
                : h.action === "STATUS_CHANGED"
                  ? (
                      <span className="inline-flex items-center gap-2 flex-wrap">
                        <span>Status atualizado</span>
                        {h.previous_status ? (
                          <>
                            <span className="px-2 py-0.5 text-micro font-bold rounded-full bg-muted border border-border text-muted-foreground">
                              {leadStatusLabel(h.previous_status)}
                            </span>
                            <ArrowRight className="w-3 h-3 text-muted-foreground" />
                          </>
                        ) : null}
                        <span className="px-2 py-0.5 text-micro font-bold rounded-full bg-primary-50 border border-primary-200 text-primary-700">
                          {leadStatusLabel(h.new_status)}
                        </span>
                      </span>
                    )
                  : h.details || h.action,
            desc: h.details || "",
            date: new Date(h.timestamp).toLocaleString("pt-BR"),
            icon: h.action === "NOTE" ? FileText : h.action === "STATUS_CHANGED" ? TrendingUp : CheckCircle2,
            color: h.action === "NOTE" ? "text-primary-600" : h.action === "STATUS_CHANGED" ? "text-status-warning" : "text-muted-foreground",
            bg: h.action === "NOTE" ? "bg-primary-100" : h.action === "STATUS_CHANGED" ? "bg-status-warning-soft" : "bg-muted",
          }))
          .reverse();
        setHistory(mappedHistory);
      }
    } catch (error) {
      console.error("Error fetching lead details:", error);
    } finally {
      setIsLoading(false);
    }
  }, [id]);

  useEffect(() => {
    fetchLeadDetails();
  }, [fetchLeadDetails]);

  const relatedDeals = useMemo(() => deals.filter((d) => d.clientId === id || d.ownerId === id), [deals, id]);

  const ACTIVE_DEAL_STAGES = useMemo(() => new Set(["Em Negociação", "Proposta", "Análise", "Contrato"]), []);

  const { activeDeals, closedDeals } = useMemo(() => {
    const active: EsteiraDeal[] = [];
    const closed: EsteiraDeal[] = [];
    relatedDeals.forEach((d) => {
      (ACTIVE_DEAL_STAGES.has(d.stage) ? active : closed).push(d);
    });
    return { activeDeals: active, closedDeals: closed };
  }, [relatedDeals, ACTIVE_DEAL_STAGES]);

  const dealRoleForLead = useCallback(
    (deal: EsteiraDeal): "owner" | "customer" | "both" => {
      const isOwner = deal.ownerId === id;
      const isCustomer = deal.clientId === id;
      if (isOwner && isCustomer) return "both";
      return isOwner ? "owner" : "customer";
    },
    [id],
  );

  const duplicatePropertyIds = useMemo(() => {
    const counts = new Map<string, number>();
    activeDeals.forEach((d) => {
      if (!d.propertyId) return;
      counts.set(d.propertyId, (counts.get(d.propertyId) ?? 0) + 1);
    });
    return new Set([...counts.entries()].filter(([, c]) => c > 1).map(([pid]) => pid));
  }, [activeDeals]);

  const relatedProperties = useMemo(() => {
    const propIds = new Set<string>();
    relatedDeals.forEach((d) => {
      if (d.propertyId) propIds.add(d.propertyId);
    });
    if (apiLead?.captacao_property_id) propIds.add(apiLead.captacao_property_id);
    return properties.filter((p) => propIds.has(p.id));
  }, [relatedDeals, properties, apiLead]);

  const relatedContracts = useMemo(() => {
    const dealIds = new Set(relatedDeals.map((d) => d.id));
    const fromDeals = contracts.filter((c) => c.deal_id && dealIds.has(c.deal_id));
    if (apiLead?.contract_id) {
      const legacy = contracts.find((c) => c.id === apiLead.contract_id);
      if (legacy && !fromDeals.some((c) => c.id === legacy.id)) {
        return [legacy, ...fromDeals];
      }
    }
    return fromDeals;
  }, [contracts, apiLead, relatedDeals]);

  const relatedAppointments = useMemo(
    () =>
      appointments
        .filter((a) => a.leadId === id)
        .sort((a, b) => `${a.date}T${a.time}`.localeCompare(`${b.date}T${b.time}`)),
    [appointments, id],
  );
  // O que o operador precisa ver primeiro na visão geral: os próximos
  // compromissos que ainda vão acontecer (cancelado/concluído fica na agenda).
  const nextAppointments = useMemo(
    () => relatedAppointments.filter((a) => a.status !== "Cancelado" && a.status !== "Concluido").slice(0, 3),
    [relatedAppointments],
  );

  const relatedContractDealIds = useMemo(
    () => [...new Set(relatedContracts.map((contract) => contract.deal_id).filter((dealId): dealId is string => Boolean(dealId)))].sort(),
    [relatedContracts],
  );
  const relatedContractIds = useMemo(
    () => [...new Set(relatedContracts.map((contract) => contract.id))].sort(),
    [relatedContracts],
  );

  const contractCommissionsQuery = useQuery({
    queryKey: ["lead-contract-commissions", id, relatedContractDealIds.join(",")],
    queryFn: async () => {
      const groups = await Promise.all(
        relatedContractDealIds.map((dealId) => commissionService.listByDeal(dealId)),
      );
      return groups.flat();
    },
    enabled: Boolean(id && relatedContractDealIds.length > 0),
    staleTime: 60_000,
  });

  const leadRole = (apiLead?.role ?? "").toLowerCase();
  const rentalRole: "tenant" | "owner" | undefined = leadRole === "tenant" ? "tenant" : leadRole === "owner" ? "owner" : undefined;

  const rentalsQuery = useQuery({
    queryKey: rentalsListQueryKey({ lead_id: id, role: rentalRole }),
    queryFn: () => fetchRentalsList({ lead_id: id, role: rentalRole, limit: 50 }),
    enabled: Boolean(id && rentalRole),
    staleTime: 60_000,
  });

  const chargesQuery = useQuery({
    queryKey: ["lead-charges", id, relatedContractIds.join(",")],
    queryFn: async () => {
      const groups = await Promise.all(
        relatedContractIds.map((contractId) => collectPages<Charge>(
          ({ limit, offset }) => chargeService.getAll({ contract_id: contractId, limit, offset }),
        )),
      );
      return groups.flat();
    },
    enabled: Boolean(id && relatedContractIds.length > 0),
    staleTime: 60_000,
  });

  const maintenanceQuery = useQuery({
    queryKey: ["lead-maintenance", id],
    queryFn: async () => {
      const res = await maintenanceService.getAll({ requester_lead_id: id });
      return unwrapApiData<MaintenanceTicket[]>(res);
    },
    enabled: Boolean(id),
    staleTime: 60_000,
  });

  const rentals = rentalsQuery.data ?? [];
  const charges = chargesQuery.data ?? [];
  const contractCommissions: Commission[] = contractCommissionsQuery.data ?? [];
  const maintenanceTickets = maintenanceQuery.data ?? [];

  const handleAddNote = async () => {
    if (!id || !noteText.trim()) return;
    setIsSubmittingNote(true);
    try {
      await leadService.addNote(id, noteText);
      setNoteText("");
      fetchLeadDetails();
      toast.success("Nota adicionada!");
    } catch (error) {
      console.error("Error adding note:", error);
      toast.error("Erro ao adicionar nota.");
    } finally {
      setIsSubmittingNote(false);
    }
  };

  const sourceMap: Record<string, string> = {
    Site: "WEBSITE",
    Indicacao: "REFERRAL",
    "Redes Sociais": "SOCIAL",
    Manual: "MANUAL",
    Campanha: "CAMPAIGN",
    Outros: "OTHER",
  };
  const leadTypeToRole: Record<string, ClientRole> = {
    captacao: "owner",
    compra_venda: "buyer",
    locacao: "tenant",
  };

  const handleSave = async () => {
    if (!lead || !id) return;
    setIsSaving(true);
    try {
      const apiDto: UpdateLeadDTO = {
        name: lead.name,
        email: lead.email,
        phone: parsePhoneBR(lead.phone),
        source: lead.source ? ((sourceMap[lead.source] || "MANUAL") as any) : undefined,
        assigned_broker_id: lead.agentId ?? "",
        property_interest: editInterest,
        role: lead.leadType ? leadTypeToRole[lead.leadType] : undefined,
      };

      if (lead.leadType === "captacao" && apiLead) {
        apiDto.captacao_property_id = apiLead.captacao_property_id;
        apiDto.owner_lead_id = apiLead.owner_lead_id;
      }

      const localUpdates: Partial<Lead> = {
        name: lead.name,
        email: lead.email,
        phone: parsePhoneBR(lead.phone),
        agentId: lead.agentId,
        source: lead.source,
        leadType: lead.leadType,
        role: lead.leadType ? leadTypeToRole[lead.leadType] : lead.role,
        budget: editInterest.price_range?.max || lead.budget,
        property: editInterest.preferred_location || lead.property,
      };
      await updateLead(id, localUpdates, apiDto);

      await fetchLeadDetails();
      toast.success("Lead atualizado!");
      setIsEditing(false);
    } catch {
      // LeadsContext already restored the optimistic snapshot and surfaced the error.
    } finally {
      setIsSaving(false);
    }
  };

  const handleCancel = () => {
    setLead(originalLead!);
    if (apiLead?.property_interest) setEditInterest(apiLead.property_interest);
    setIsEditing(false);
  };

  const handleConvertToClient = async () => {
    if (!id || !lead) return;
    if (lead.leadType === "captacao") {
      setIsCaptacaoWizardOpen(true);
      return;
    }
    setIsCreatingDeal(true);
  };

  const handleDisqualify = async () => {
    if (!id) return;
    const reason = disqualifyReason === "Outro" ? disqualifyNote.trim() : disqualifyReason;
    if (!reason) {
      toast.error("Informe o motivo da desqualificação.");
      return;
    }
    setIsDisqualifying(true);
    try {
      await leadService.disqualifyLead(id, reason);
      setIsDisqualifyOpen(false);
      setDisqualifyNote("");
      await fetchLeadDetails();
      toast.success("Lead desqualificado.");
    } catch {
      toast.error("Erro ao desqualificar o lead.");
    } finally {
      setIsDisqualifying(false);
    }
  };

  const handleInviteClient = async () => {
    if (!id || !lead) return;
    if (!canUseClientPortal) {
      navigate("/assinatura");
      return;
    }
    if (!can("leads:write")) {
      toast.error("Sem permissão para enviar convites.");
      return;
    }
    if (!lead.email?.trim()) {
      toast.error("Preencha o e-mail do lead para enviar o convite.");
      return;
    }
    setIsInvitingClient(true);
    try {
      await leadService.inviteClient(id);
      const localMail = typeof window !== "undefined"
        && (window.location.hostname === "localhost" || window.location.hostname === "127.0.0.1");
      toast.success("Convite enviado com sucesso para o portal do cliente!", localMail ? {
        description: "No ambiente local, o e-mail fica no Mailpit (não vai para a caixa real).",
        action: {
          label: "Abrir Mailpit",
          onClick: () => window.open("http://localhost:54324", "_blank", "noopener,noreferrer"),
        },
      } : undefined);
    } catch (error: any) {
      const message =
        error.response?.data?.message ||
        error.response?.data?.error ||
        error.response?.data?.details ||
        error.message ||
        "Erro ao enviar convite.";
      toast.error(message);
    } finally {
      setIsInvitingClient(false);
    }
  };

  if (!lead) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center bg-muted p-8">
        <AlertCircle className="w-12 h-12 text-muted-foreground/60 mb-4" />
        <h2 className="text-xl font-bold text-foreground">Lead não encontrado</h2>
        <Button onClick={() => navigate(-1)} className="mt-4">
          Voltar
        </Button>
      </div>
    );
  }

  const leadTypeConf = LEAD_TYPE_CONFIG[lead.leadType || "compra_venda"];
  const isClient = lead.isClient || lead.status === "Convertido";
  const budgetLabel = lead.leadType === "locacao" ? "Orçamento Mensal" : "Orçamento";

  if (isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center bg-muted p-8">
        <div className="w-8 h-8 border-4 border-primary-500 border-t-transparent rounded-full animate-spin"></div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full bg-muted overflow-x-hidden overflow-y-auto">
      <div className="flex-none px-6 py-4 lg:px-8 border-b border-border bg-card sticky top-0 z-20">
        <div className="flex items-center gap-4 mb-4">
          <Button variant="ghost" size="icon" onClick={() => navigate(-1)}>
            <ArrowLeft className="w-5 h-5" />
          </Button>
          <div className="flex-1 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 min-w-0">
            <div className="flex items-center gap-4 min-w-0">
              <Avatar className="w-12 h-12 border border-border">
                {lead.avatar ? <AvatarImage src={getAvatarThumbUrl(lead.avatar, 96)} alt={lead.name} /> : null}
                <AvatarFallback>{lead.name.charAt(0).toUpperCase()}</AvatarFallback>
              </Avatar>
              <div className="min-w-0">
                {isEditing ? (
                  <input
                    type="text"
                    value={lead.name}
                    onChange={(e) => setLead({ ...lead, name: e.target.value })}
                    className="text-2xl font-bold text-foreground leading-tight bg-muted border border-border rounded px-2 py-0.5 outline-none focus:ring-2 focus:ring-primary-500"
                  />
                ) : (
                  <h1 className="text-2xl font-bold text-foreground leading-tight truncate">{lead.name}</h1>
                )}
                <div className="flex items-center gap-2 mt-1 flex-wrap">
                  <span className="text-xs text-muted-foreground">Ref: {lead.displayId || lead.id.substring(0, 8)}</span>
                  <span className="text-muted-foreground/60">|</span>
                  <span className="text-xs text-muted-foreground">{fmtDate(lead.date)}</span>
                  {lead.leadType && (
                    <>
                      <span className="text-muted-foreground/60">|</span>
                      <span
                        className={clsx(
                          "inline-flex items-center gap-1 px-2 py-0.5 text-micro font-bold uppercase tracking-wider rounded-full border",
                          leadTypeConf.bg,
                          leadTypeConf.color,
                        )}
                      >
                        <leadTypeConf.icon className="w-3 h-3" />
                        {leadTypeConf.label}
                      </span>
                    </>
                  )}
                  {isClient && (
                    <span className="inline-flex items-center gap-1 px-2 py-0.5 text-micro font-bold uppercase tracking-wider rounded-full border bg-status-ok-soft border-status-ok/30 text-status-ok">
                      <UserCheck className="w-3 h-3" /> Cliente
                    </span>
                  )}
                  {lead.sharedFrom && <SharedBadge companyName={lead.sharedFrom.companyName} />}
                  {lead.source && (
                    <>
                      <span className="text-muted-foreground/60">|</span>
                      <span className="text-xs text-muted-foreground flex items-center gap-1">
                        <Tag className="w-3 h-3" /> {SOURCE_LABELS[lead.source] || lead.source}
                      </span>
                    </>
                  )}
                  {isClient && (
                    <CounterpartyLancamentosLink
                      counterpartyKind={lead.leadType === "captacao" ? "owner" : "tenant"}
                      searchTerm={lead.name}
                      label="Ver lançamentos"
                    />
                  )}
                </div>
              </div>
            </div>

            <div className="flex items-center gap-2 flex-wrap justify-end">
              {isEditing ? (
                <>
                  <Button variant="outline" onClick={handleCancel}>
                    <X className="w-4 h-4" /> Cancelar
                  </Button>
                  <Button onClick={handleSave} loading={isSaving} loadingText="Salvando...">
                    <Save className="w-4 h-4" /> Salvar
                  </Button>
                </>
              ) : (
                <>
                  <Button variant="outline" onClick={() => setIsEditing(true)}>
                    <Edit className="w-4 h-4" /> Editar
                  </Button>
                  <div className="flex flex-wrap items-center justify-end gap-2">
                    {isClient && can("customers:read") && (
                      <Button
                        variant="outline"
                        onClick={() => setIsClientDrawerOpen(true)}
                        className="bg-stage-contract/10 text-stage-contract border-stage-contract/30 hover:bg-stage-contract/15 hover:text-stage-contract"
                      >
                        <User className="w-4 h-4" /> Perfil 360°
                      </Button>
                    )}
                    <Button
                      variant="outline"
                      onClick={canUseClientPortal ? handleInviteClient : () => navigate("/assinatura")}
                      loading={isInvitingClient}
                      loadingText="Enviando..."
                      disabled={!can("leads:write") || (canUseClientPortal && !lead.email?.trim())}
                      title={canUseClientPortal ? undefined : "Recurso do plano Pro"}
                      className={canUseClientPortal ? "bg-status-info-soft text-status-info border-status-info/30 hover:bg-status-info-soft hover:text-status-info" : "border-primary/30 text-primary hover:bg-primary/5"}
                    >
                      {canUseClientPortal ? <Mail className="w-4 h-4" /> : <Crown className="w-4 h-4" />}
                      {canUseClientPortal ? "Convidar para Portal" : "Conheça o Portal Pro"}
                    </Button>
                    <Button
                      variant="outline"
                      onClick={() => setAppointmentPanel({ open: true, appointment: null })}
                      disabled={!can("leads:write")}
                      title={can("leads:write") ? "Agendar visita, ligação ou reunião com este lead" : "Sem permissão para agendar"}
                    >
                      <CalendarPlus className="w-4 h-4" /> Agendar
                    </Button>
                    <Button
                      variant="success"
                      onClick={() => setIsCreatingDeal(true)}
                      disabled={!!lead.sharedFrom && !can("shared_content:operate")}
                      title={
                        !!lead.sharedFrom && !can("shared_content:operate") ? "Sem permissão para operar conteúdo compartilhado" : undefined
                      }
                    >
                      <Briefcase className="w-4 h-4" /> {lead.sharedFrom ? "Iniciar negócio" : "Novo Negócio"}
                    </Button>
                    {!isClient && (
                      <Popover>
                        <PopoverTrigger asChild>
                          <Button variant="outline" size="icon" aria-label="Mais ações">
                            <MoreVertical className="w-4 h-4" />
                          </Button>
                        </PopoverTrigger>
                        <PopoverContent align="end" className="w-64 p-1">
                          <Button
                            variant="ghost"
                            onClick={handleConvertToClient}
                            loading={isConverting}
                            loadingText="Convertendo..."
                            className="w-full justify-start"
                          >
                            <CheckCircle2 className="w-4 h-4 text-status-ok" />
                            <span>Marcar como cliente</span>
                          </Button>
                          {can("leads:write") && String(lead.status) !== "DISQUALIFIED" && (
                            <Button
                              variant="ghost"
                              onClick={() => setIsDisqualifyOpen(true)}
                              className="w-full justify-start text-status-critical hover:text-status-critical"
                            >
                              <X className="w-4 h-4" />
                              <span>Desqualificar lead</span>
                            </Button>
                          )}
                        </PopoverContent>
                      </Popover>
                    )}
                  </div>
                </>
              )}
            </div>
          </div>
        </div>

        <div className="mt-6 mb-2 hidden sm:block">
          <div className="flex items-center justify-between relative max-w-5xl mx-auto px-4">
            <div className="absolute left-4 right-4 top-1/2 -translate-y-1/2 h-1 bg-muted rounded-full z-0"></div>
            {PIPELINE_STAGES.map((stage, idx) => {
              const currentIdx = PIPELINE_STAGES.indexOf(lead.status);
              const isCompleted = idx <= currentIdx;
              const isCurrent = idx === currentIdx;
              return (
                <div
                  key={stage}
                  className={clsx("relative z-10 flex flex-col items-center gap-2 cursor-pointer group", !isEditing && "hover:opacity-80")}
                  onClick={async () => {
                    if (!isEditing) {
                      const updated = { ...lead, status: stage };
                      setLead(updated);
                      try {
                        await updateLeadStatus(lead.id, stage);
                        await fetchLeadDetails();
                      } catch {
                        setLead(lead);
                      }
                    }
                  }}
                >
                  <div
                    className={clsx(
                      "w-8 h-8 rounded-full flex items-center justify-center text-sm font-bold border-4 transition-colors",
                      isCompleted ? "bg-primary border-primary text-primary-foreground" : "bg-card border-border text-muted-foreground",
                      isCurrent && "ring-4 ring-primary-100 border-primary-600 bg-card text-primary-600",
                    )}
                  >
                    {isCompleted && !isCurrent ? <CheckCircle2 className="w-4 h-4" /> : idx + 1}
                  </div>
                  <span
                    className={clsx(
                      "text-micro font-bold uppercase tracking-wider absolute -bottom-6 whitespace-nowrap",
                      isCurrent ? "text-primary-700" : isCompleted ? "text-foreground" : "text-muted-foreground",
                    )}
                  >
                    {stage}
                  </span>
                </div>
              );
            })}
          </div>
        </div>

        <div className="mt-10 flex gap-1 border-b border-border -mb-[1px] overflow-x-auto">
          {([
            { key: "info" as Tab, label: "Informações", icon: User },
            { key: "negocios" as Tab, label: `Negócios (${relatedDeals.length})`, icon: Briefcase },
            { key: "agenda" as Tab, label: `Agenda (${relatedAppointments.length})`, icon: Calendar },
            { key: "documentos" as Tab, label: `Contratos (${relatedContracts.length})`, icon: FileText },
            ...(rentals.length > 0 ? [{ key: "locacoes" as Tab, label: `Locações (${rentals.length})`, icon: Home }] : []),
            ...(charges.length > 0 ? [{ key: "cobrancas" as Tab, label: `Cobranças (${charges.length})`, icon: Wallet }] : []),
            ...(maintenanceTickets.length > 0 ? [{ key: "tickets" as Tab, label: `Manutenção (${maintenanceTickets.length})`, icon: Wrench }] : []),
          ]).map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={clsx(
                "flex items-center gap-2 px-4 py-2.5 text-sm font-bold transition-colors border-b-2 -mb-[2px] whitespace-nowrap",
                activeTab === tab.key ? "text-primary-700 border-primary-600" : "text-muted-foreground border-transparent hover:text-foreground hover:border-border",
              )}
            >
              <tab.icon className="w-4 h-4" /> {tab.label}
            </button>
          ))}
        </div>
      </div>

      <div className="flex-1 p-6 lg:p-8">
        <div className="max-w-7xl mx-auto">
          {activeTab === "info" && (
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
              <div className="lg:col-span-1 space-y-6">
                <Card title="Corretor Responsável" icon={<Users className="w-5 h-5 text-primary-500" />}>
                  <div>
                    <Label>Atribuído para</Label>
                    {isEditing ? (
                      <Popover open={brokerComboOpen} onOpenChange={setBrokerComboOpen}>
                        <PopoverTrigger asChild>
                          <button
                            type="button"
                            className="w-full flex items-center justify-between px-3 py-2 bg-muted border border-border rounded-md text-sm text-foreground hover:bg-muted focus:border-primary-500 focus:ring-1 focus:ring-primary-500 outline-none transition-all"
                          >
                            {lead.agentId ? (
                              <div className="flex items-center gap-2">
                                {(() => {
                                  const agent = agents.find((a) => a.id === lead.agentId);
                                  const agentToDisplay =
                                    agent ||
                                    (lead.agentId === user?.id && user
                                      ? { id: user.id, name: user.fullname || `${user.firstname || ""} ${user.lastname || ""}`.trim() || "Você", role: user.role || "Corretor", avatar: user.avatar_url || "" }
                                      : undefined);
                                  return agentToDisplay ? (
                                    <Avatar className="w-5 h-5 border border-border">
                                      {agentToDisplay.avatar ? <AvatarImage src={getAvatarThumbUrl(agentToDisplay.avatar, 40)} alt={agentToDisplay.name} /> : null}
                                      <AvatarFallback className="text-micro font-bold">{agentToDisplay.name.charAt(0).toUpperCase()}</AvatarFallback>
                                    </Avatar>
                                  ) : (
                                    <Avatar className="w-5 h-5 border border-border">
                                      <AvatarFallback className="text-micro font-bold bg-primary-100 text-primary-700">U</AvatarFallback>
                                    </Avatar>
                                  );
                                })()}
                                <span className="truncate">
                                  {(() => {
                                    const agent = agents.find((a) => a.id === lead.agentId);
                                    const agentToDisplay =
                                      agent ||
                                      (lead.agentId === user?.id && user
                                        ? { id: user.id, name: user.fullname || `${user.firstname || ""} ${user.lastname || ""}`.trim() || "Você", role: user.role || "Corretor", avatar: user.avatar_url || "" }
                                        : undefined);
                                    return agentToDisplay?.name || "Corretor desconhecido";
                                  })()}
                                </span>
                              </div>
                            ) : (
                              <span className="text-muted-foreground">Não atribuído</span>
                            )}
                            <ChevronsUpDown className="w-4 h-4 text-muted-foreground opacity-50" />
                          </button>
                        </PopoverTrigger>
                        <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0 bg-card border border-border shadow-xl rounded-lg z-50" align="start">
                          <Command>
                            <CommandInput placeholder="Buscar corretor..." />
                            <CommandList>
                              <CommandEmpty>Nenhum corretor encontrado.</CommandEmpty>
                              <CommandGroup>
                                <CommandItem
                                  value="unassigned"
                                  onSelect={() => {
                                    setLead({ ...lead, agentId: "" });
                                    setBrokerComboOpen(false);
                                  }}
                                  className="cursor-pointer"
                                >
                                  <Check className={clsx("w-4 h-4 mr-2", !lead.agentId ? "opacity-100" : "opacity-0")} />
                                  <span className="italic text-muted-foreground">Não atribuído</span>
                                </CommandItem>
                                {agents.map((agent) => (
                                  <CommandItem
                                    key={agent.id}
                                    value={agent.name}
                                    onSelect={() => {
                                      setLead({ ...lead, agentId: agent.id });
                                      setBrokerComboOpen(false);
                                    }}
                                    className="cursor-pointer"
                                  >
                                    <Check className={clsx("w-4 h-4 mr-2", lead.agentId === agent.id ? "opacity-100" : "opacity-0")} />
                                    <div className="flex items-center gap-2">
                                      <Avatar className="w-5 h-5 border border-border">
                                        {agent.avatar ? <AvatarImage src={getAvatarThumbUrl(agent.avatar, 40)} alt={agent.name} /> : null}
                                        <AvatarFallback className="text-micro font-bold">{agent.name.charAt(0).toUpperCase()}</AvatarFallback>
                                      </Avatar>
                                      <span>{agent.name}</span>
                                    </div>
                                  </CommandItem>
                                ))}
                              </CommandGroup>
                            </CommandList>
                          </Command>
                        </PopoverContent>
                      </Popover>
                    ) : (
                      <AgentDisplay agentId={lead.agentId} agents={agents} user={user} />
                    )}
                  </div>
                </Card>

                <Card title="Informações de Contato" icon={<Mail className="w-5 h-5 text-primary-500" />}>
                  {lead.sharedFrom && !lead.email && !lead.phone ? (
                    <div className="rounded-xl border border-stage-contract/30 bg-stage-contract/5 p-4">
                      <p className="text-sm font-bold text-foreground">Contato não compartilhado</p>
                      <p className="text-xs text-muted-foreground mt-1">
                        {lead.sharedFrom.companyName} não compartilhou os dados de contato deste lead. Inicie um negócio de co-corretagem para avançar — a imobiliária dona será notificada.
                      </p>
                    </div>
                  ) : (
                    <div className="space-y-4">
                      <div>
                        <Label>E-mail Principal</Label>
                        {isEditing ? (
                          <input
                            type="email"
                            value={lead.email}
                            onChange={(e) => setLead({ ...lead, email: e.target.value })}
                            className="w-full text-sm font-medium text-foreground bg-muted border border-border rounded-lg px-3 py-2 outline-none focus:ring-2 focus:ring-primary-500"
                          />
                        ) : (
                          <p className="text-sm font-bold text-foreground">{lead.email || "Não compartilhado"}</p>
                        )}
                      </div>
                      <div>
                        <Label>Telefone / WhatsApp</Label>
                        {isEditing ? (
                          <input
                            type="tel"
                            value={formatPhoneBR(lead.phone)}
                            onChange={(e) => setLead({ ...lead, phone: parsePhoneBR(e.target.value) })}
                            className="w-full text-sm font-medium text-foreground bg-muted border border-border rounded-lg px-3 py-2 outline-none focus:ring-2 focus:ring-primary-500"
                          />
                        ) : (
                          <p className="text-sm font-bold text-foreground">{lead.phone ? formatPhoneBR(lead.phone) : "Não compartilhado"}</p>
                        )}
                      </div>
                      <div>
                        <Label>Fonte / Origem</Label>
                        {isEditing ? (
                          <select
                            value={lead.source || ""}
                            onChange={(e) => setLead({ ...lead, source: (e.target.value || undefined) as LeadSource })}
                            className="w-full text-sm font-medium text-foreground bg-muted border border-border rounded px-3 py-2 outline-none focus:ring-2 focus:ring-primary-500"
                          >
                            <option value="">Selecione</option>
                            {Object.entries(SOURCE_LABELS).map(([k, v]) => (
                              <option key={k} value={k}>
                                {v}
                              </option>
                            ))}
                          </select>
                        ) : (
                          <p className="text-sm font-bold text-foreground">{SOURCE_LABELS[lead.source || ""] || "Não informado"}</p>
                        )}
                      </div>
                    </div>
                  )}
                  {!isEditing && !(lead.sharedFrom && !lead.email && !lead.phone) && (
                    <div className="mt-6 flex gap-2">
                      <Button variant="outline" disabled={!lead.phone} className="flex-1 bg-[#25D366]/10 text-[#075E54] border-transparent hover:bg-[#25D366]/20">
                        <MessageSquare className="w-4 h-4" /> WhatsApp
                      </Button>
                      <Button variant="secondary" disabled={!lead.phone} className="flex-1">
                        <Phone className="w-4 h-4" /> Ligar
                      </Button>
                    </div>
                  )}
                </Card>

                {/* Caminho de VISUALIZAÇÃO: o próximo compromisso aparece na
                    própria visão geral, com atalho para a agenda do lead. */}
                <div className="p-4 rounded-2xl border border-border bg-card shadow-sm">
                  <div className="flex items-center justify-between gap-2 mb-2">
                    <div className="flex items-center gap-2">
                      <Calendar className="w-4 h-4 text-primary" />
                      <h4 className="text-label font-bold uppercase tracking-wide">Agenda do lead</h4>
                    </div>
                    <Button variant="ghost" size="sm" onClick={() => setActiveTab("agenda")}>
                      Ver agenda ({relatedAppointments.length})
                    </Button>
                  </div>
                  {nextAppointments.length === 0 ? (
                    <p className="text-sm text-muted-foreground">
                      Nenhum compromisso agendado.{" "}
                      <button
                        type="button"
                        className="font-bold text-primary underline-offset-2 hover:underline disabled:opacity-60"
                        disabled={!can("leads:write")}
                        onClick={() => setAppointmentPanel({ open: true, appointment: null })}
                      >
                        Agendar agora
                      </button>
                    </p>
                  ) : (
                    <ul className="space-y-2">
                      {nextAppointments.map((apt) => (
                        <li key={apt.id}>
                          <button
                            type="button"
                            onClick={() => setAppointmentPanel({ open: true, appointment: apt })}
                            className="w-full text-left text-sm flex items-center gap-2 rounded-lg px-2 py-1.5 hover:bg-muted"
                          >
                            <CalendarDays className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
                            <span className="font-medium text-foreground truncate">{apt.title}</span>
                            <span className="text-xs text-muted-foreground shrink-0">
                              {fmtDate(apt.date)} às {apt.time} · {apt.status}
                            </span>
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
                {apiLead?.id &&
                  (apiLead.role === "buyer" || apiLead.role === "tenant" || lead.leadType === "compra_venda" || lead.leadType === "locacao") && (
                    <MatchedPropertiesCard leadId={apiLead.id} refreshKey={apiLead.updated_at} compact />
                  )}
                {apiLead?.id && (apiLead as any).visit_request?.status && (
                  <div className="p-4 rounded-2xl border border-border bg-card shadow-sm">
                    <div className="flex items-center gap-2 mb-2">
                      <CalendarClock className="w-4 h-4 text-primary" />
                      <h4 className="text-label font-bold uppercase tracking-wide">Visita do cliente</h4>
                    </div>
                    <p className="text-sm text-muted-foreground flex items-center gap-1.5">
                      <CalendarDays className="w-3.5 h-3.5" /> {formatBRDate((apiLead as any).visit_request.date)} às {(apiLead as any).visit_request.time}
                      <span className={clsx("text-xs font-bold", (apiLead as any).visit_request.status === "Confirmado" ? "text-status-ok" : (apiLead as any).visit_request.status === "Cancelado" ? "text-status-critical" : "text-status-warning")}>
                        {(() => { const s=(apiLead as any).visit_request.status; return s === "Confirmado" ? "· Confirmada" : s === "Cancelado" ? "· Cancelada" : "· Aguardando"; })()}
                      </span>
                    </p>
                    {(apiLead as any).visit_request.status === "Aguardando" && (
                      <Button
                        size="sm"
                        className="mt-3"
                        variant="default"
                        onClick={async () => {
                          await leadService.confirmLeadVisit(apiLead.id);
                          updateLead(apiLead.id, { visit_request: { ...(apiLead as any).visit_request, status: "Confirmado" } } as any);
                          toast.success("Visita confirmada. O cliente foi notificado.");
                        }}
                      >
                        <Check className="w-4 h-4" /> Confirmar visita
                      </Button>
                    )}
                  </div>
                )}
              </div>

              <div className="lg:col-span-2 space-y-6">
                <div className="bg-card rounded-3xl border border-border shadow-sm flex flex-col overflow-hidden">
                  <div className="p-6 border-b border-border bg-muted/50">
                    <div className="flex items-center justify-between mb-4">
                      <h3 className="text-lg font-bold text-foreground">Registro de Atividades</h3>
                    </div>
                    <div className="relative">
                      <textarea
                        value={noteText}
                        onChange={(e) => setNoteText(e.target.value)}
                        placeholder="Registre uma anotacao, ligacao ou visita..."
                        className="w-full h-24 text-sm text-foreground bg-card border border-border rounded-xl p-4 pr-32 focus:ring-2 focus:ring-primary-500 outline-none resize-none shadow-inner"
                      />
                      <Button
                        size="sm"
                        disabled={!noteText.trim()}
                        loading={isSubmittingNote}
                        loadingText="Salvando..."
                        onClick={handleAddNote}
                        className="absolute bottom-3 right-3"
                      >
                        Salvar Nota
                      </Button>
                    </div>
                  </div>

                  <div className="flex-1 p-6 relative bg-card min-h-[300px]">
                    <div className="absolute left-8 top-6 bottom-6 w-0.5 bg-gradient-to-b from-primary-100 via-muted to-transparent z-0"></div>
                    <div className="space-y-8 relative z-10">
                      {history.length > 0 ? (
                        history.map((event) => (
                          <div key={event.id} className="flex gap-4 items-start group">
                            <div className={clsx("w-10 h-10 rounded-full border-4 border-white flex items-center justify-center shrink-0 shadow-sm transition-transform group-hover:scale-110", event.bg, event.color)}>
                              <event.icon className="w-4 h-4" />
                            </div>
                            <div className="flex-1 pt-1.5">
                              <div className="flex items-baseline justify-between gap-4 mb-1">
                                <h4 className="text-sm font-bold text-foreground flex items-center gap-2 flex-wrap">
                                  {event.badge ? (
                                    <span className={clsx("px-2 py-0.5 text-micro font-bold uppercase tracking-wider rounded-md border", event.badge.bg, event.badge.border, event.badge.color)}>
                                      {event.badge.label}
                                    </span>
                                  ) : null}
                                  {event.title}
                                </h4>
                                <span className="text-micro font-bold text-muted-foreground uppercase tracking-wider flex items-center gap-1">
                                  <Clock className="w-3 h-3" /> {event.date}
                                </span>
                              </div>
                              {event.desc && <div className="mt-2 text-sm p-4 rounded-2xl shadow-sm border bg-muted border-border text-foreground">{event.desc}</div>}
                            </div>
                          </div>
                        ))
                      ) : (
                        <div className="text-center text-muted-foreground py-8 text-sm">Nenhuma atividade registrada ainda.</div>
                      )}
                    </div>
                  </div>
                </div>
              </div>
            </div>
          )}

          {activeTab === "negocios" && (
            <div className="space-y-6">
              <div className="flex items-center justify-between">
                <h3 className="text-lg font-bold text-foreground">Negócios do Lead</h3>
                <Button onClick={() => setIsCreatingDeal(true)}>
                  <Briefcase className="w-4 h-4" /> Novo Negócio
                </Button>
              </div>

              {relatedDeals.length === 0 && <EmptyState icon={Briefcase} message="Nenhum negócio vinculado a este lead." />}

              {activeDeals.length > 0 && (
                <div>
                  <h4 className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-3">Ativos ({activeDeals.length})</h4>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    {activeDeals.map((deal) => (
                      <DealCard key={deal.id} deal={deal} properties={properties} roleOfLead={dealRoleForLead(deal)} isDuplicate={!!deal.propertyId && duplicatePropertyIds.has(deal.propertyId)} />
                    ))}
                  </div>
                </div>
              )}

              {closedDeals.length > 0 && (
                <details className="group">
                  <summary className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-3 cursor-pointer list-none flex items-center gap-2">
                    <ChevronRight className="w-3 h-3 transition-transform group-open:rotate-90" />
                    Encerrados ({closedDeals.length})
                  </summary>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mt-3">
                    {closedDeals.map((deal) => (
                      <DealCard key={deal.id} deal={deal} properties={properties} roleOfLead={dealRoleForLead(deal)} />
                    ))}
                  </div>
                </details>
              )}

              {relatedContracts.length > 0 && (
                <div className="mt-8">
                  <h4 className="text-base font-bold text-foreground mb-4 flex items-center gap-2">
                    <FileText className="w-5 h-5 text-primary-500" /> Contratos
                  </h4>
                  <div className="space-y-3">
                    {relatedContracts.map((c) => (
                      <div key={c.id} className="bg-card rounded-xl border border-border p-4 flex items-center justify-between">
                        <div>
                          <p className="text-sm font-bold text-foreground">
                            {c.property_title || "Contrato"} - {c.type === "SALE" || c.type === "venda" ? "Venda" : "Locação"}
                          </p>
                          <p className="text-xs text-muted-foreground mt-1">
                            {c.status === ContractStatus.ACTIVE ? "Ativo" : c.status} | {fmt(c.value || 0)} | Início: {fmtDate(c.start_date)}
                          </p>
                        </div>
                        <span className={clsx("px-3 py-1 text-xs font-bold rounded-full", c.status === ContractStatus.ACTIVE ? "bg-status-ok-soft text-status-ok" : "bg-muted text-muted-foreground")}>
                          {c.status === ContractStatus.ACTIVE ? "Ativo" : c.status}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}

          {activeTab === "agenda" && (
            <div className="space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <h3 className="text-lg font-bold text-foreground">Agenda do lead</h3>
                  <p className="text-xs text-muted-foreground">
                    Visitas, ligações e reuniões deste cliente — agende, reagende ou mude o status sem sair da página.
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <Button variant="outline" onClick={() => navigate("/agenda")}>
                    <CalendarDays className="w-4 h-4" /> Ver agenda completa
                  </Button>
                  <Button
                    onClick={() => setAppointmentPanel({ open: true, appointment: null })}
                    disabled={!can("leads:write")}
                  >
                    <CalendarPlus className="w-4 h-4" /> Novo compromisso
                  </Button>
                </div>
              </div>
              {relatedAppointments.length === 0 ? (
                <div className="rounded-xl border border-dashed border-border bg-card p-8 text-center">
                  <Calendar className="w-8 h-8 mx-auto text-muted-foreground" />
                  <p className="mt-3 text-sm font-medium text-foreground">Nenhum compromisso agendado para este lead.</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    Marque a próxima visita, ligação ou reunião para o cliente acompanhar.
                  </p>
                  <Button
                    className="mt-4"
                    onClick={() => setAppointmentPanel({ open: true, appointment: null })}
                    disabled={!can("leads:write")}
                  >
                    <CalendarPlus className="w-4 h-4" /> Agendar compromisso
                  </Button>
                </div>
              ) : (
                <div className="space-y-3">
                  {relatedAppointments.map((apt) => (
                    <div key={apt.id} className="bg-card rounded-xl border border-border p-4 flex flex-wrap items-center gap-4">
                      <button
                        type="button"
                        onClick={() => setAppointmentPanel({ open: true, appointment: apt })}
                        aria-label={`Abrir compromisso ${apt.title}`}
                        className={clsx("w-12 h-12 rounded-xl flex items-center justify-center shrink-0 transition-colors hover:ring-2 hover:ring-primary/30", apt.status === "Concluido" ? "bg-status-ok-soft text-status-ok" : apt.status === "Cancelado" ? "bg-status-critical-soft text-status-critical" : "bg-primary-100 text-primary-600")}
                      >
                        <Calendar className="w-5 h-5" />
                      </button>
                      <button
                        type="button"
                        onClick={() => setAppointmentPanel({ open: true, appointment: apt })}
                        className="flex-1 min-w-0 text-left"
                      >
                        <p className="text-sm font-bold text-foreground truncate">{apt.title}</p>
                        <p className="text-xs text-muted-foreground mt-0.5">
                          {fmtDate(apt.date)} as {apt.time} - {apt.type} | {apt.status}
                        </p>
                        {apt.location && (
                          <p className="text-xs text-muted-foreground mt-0.5 flex items-center gap-1">
                            <MapPin className="w-3 h-3" /> {apt.location}
                          </p>
                        )}
                      </button>
                      <span className={clsx("px-3 py-1 text-xs font-bold rounded-full shrink-0", apt.status === "Concluido" ? "bg-status-ok-soft text-status-ok" : apt.status === "Cancelado" ? "bg-status-critical-soft text-status-critical" : "bg-primary-50 text-primary-700")}>
                        {apt.status}
                      </span>
                      <div className="flex items-center gap-1.5 shrink-0">
                        <Button
                          variant="outline"
                          size="sm"
                          aria-label={`Editar compromisso ${apt.title}`}
                          onClick={() => setAppointmentPanel({ open: true, appointment: apt })}
                          disabled={!can("leads:write")}
                        >
                          <Pencil className="w-3.5 h-3.5" /> Editar
                        </Button>
                        {apt.status !== "Confirmado" && apt.status !== "Concluido" && (
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={!can("leads:write") || appointmentBusy === apt.id}
                            onClick={() => void handleAppointmentStatus(apt, "Confirmado")}
                          >
                            <Check className="w-3.5 h-3.5" /> Confirmar
                          </Button>
                        )}
                        {apt.status !== "Cancelado" && apt.status !== "Concluido" && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive hover:text-destructive"
                            disabled={!can("leads:write") || appointmentBusy === apt.id}
                            onClick={() => void handleAppointmentStatus(apt, "Cancelado")}
                          >
                            <X className="w-3.5 h-3.5" /> Cancelar
                          </Button>
                        )}
                        <Button
                          variant="ghost"
                          size="icon"
                          aria-label={`Remover compromisso ${apt.title}`}
                          className="text-muted-foreground hover:text-destructive"
                          disabled={!can("leads:write") || appointmentBusy === apt.id}
                          onClick={() => void handleAppointmentRemove(apt)}
                        >
                          <Trash2 className="w-4 h-4" />
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}

            </div>
          )}

          {activeTab === "documentos" && (
            <div className="space-y-4">
              <h3 className="text-lg font-bold text-foreground">Contratos e Documentos</h3>
              {relatedContracts.length === 0 ? (
                <EmptyState icon={FileText} message="Nenhum contrato vinculado a este lead." />
              ) : (
                <div className="space-y-3">
                  {relatedContracts.map((c) => {
                    const commissionPct = resolveContractCommissionPct(c, relatedDeals, contractCommissions);
                    return (
                      <div key={c.id} className="bg-card rounded-xl border border-border p-5">
                        <div className="flex items-center justify-between mb-3">
                          <div>
                            <p className="text-base font-bold text-foreground">{c.property_title || "Contrato"}</p>
                            <p className="text-xs text-muted-foreground mt-1">Ref: {c.display_id || c.id.substring(0, 8)}</p>
                          </div>
                          <span className={clsx("px-3 py-1 text-xs font-bold rounded-full", c.status === ContractStatus.ACTIVE ? "bg-status-ok-soft text-status-ok" : "bg-muted text-muted-foreground")}>
                            {c.status === ContractStatus.ACTIVE ? "Ativo" : c.status}
                          </span>
                        </div>
                        <div className="grid grid-cols-3 gap-4 text-sm">
                          <div>
                            <Label>Tipo</Label>
                            <p className="font-bold text-foreground">{c.type === "SALE" || c.type === "venda" ? "Venda" : "Locação"}</p>
                          </div>
                          <div>
                            <Label>Valor</Label>
                            <p className="font-bold text-foreground">{fmt(c.value || 0)}</p>
                          </div>
                          <div>
                            <Label>Comissão</Label>
                            <p className="font-bold text-foreground">
                              {commissionPct === null ? "A apurar" : `${commissionPct}%`}
                            </p>
                          </div>
                        </div>
                        {c.signatories && c.signatories.length > 0 && (
                          <div className="mt-4 pt-3 border-t border-border">
                            <Label>Signatarios</Label>
                            <div className="flex flex-wrap gap-2 mt-1">
                              {c.signatories.map((s) => (
                                <span key={s.id} className={clsx("px-2.5 py-1 text-xs font-bold rounded-full border", s.signed_at ? "bg-status-ok-soft border-status-ok/30 text-status-ok" : "bg-muted border-border text-muted-foreground")}>
                                  {s.name} ({s.role}) {s.signed_at ? "✓" : "⏳"}
                                </span>
                              ))}
                            </div>
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          )}

          {activeTab === "locacoes" && (
            <div className="space-y-4">
              <h3 className="text-lg font-bold text-foreground">{rentalRole === "owner" ? "Imóveis locados" : "Locações"}</h3>
              {rentals.length === 0 ? (
                <EmptyState icon={Home} message="Nenhuma locação encontrada." />
              ) : (
                <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                  {rentals.map((r) => (
                    <Link key={r.id} to={`/locacao/${r.id}`} className="bg-card rounded-xl border border-border p-5 hover:border-primary-300 hover:shadow-md transition-all group">
                      <div className="flex items-center justify-between mb-3">
                        <div className="min-w-0">
                          <p className="text-base font-bold text-foreground group-hover:text-primary-700 transition-colors truncate">Locação #{r.id.slice(0, 8)}</p>
                          <p className="text-xs text-muted-foreground mt-0.5">Imóvel: {r.property_id.slice(0, 8)}</p>
                        </div>
                        <span className={clsx("px-2.5 py-1 text-xs font-bold rounded-full shrink-0", r.status === "ACTIVE" ? "bg-status-ok-soft text-status-ok" : r.status === "ENDED" ? "bg-muted text-muted-foreground" : "bg-status-critical-soft text-status-critical")}>
                          {r.status === "ACTIVE" ? "Ativa" : r.status === "ENDED" ? "Encerrada" : "Rescindida"}
                        </span>
                      </div>
                      <div className="grid grid-cols-2 gap-4 text-sm">
                        <div>
                          <Label>Aluguel mensal</Label>
                          <p className="font-bold text-foreground">{fmt((r.monthly_value_cents || 0) / 100)}</p>
                        </div>
                        <div>
                          <Label>Início</Label>
                          <p className="font-bold text-foreground">{r.start_date ? new Date(r.start_date).toLocaleDateString("pt-BR") : "—"}</p>
                        </div>
                      </div>
                    </Link>
                  ))}
                </div>
              )}
            </div>
          )}

          {activeTab === "cobrancas" && (
            <div className="space-y-4">
              <h3 className="text-lg font-bold text-foreground">Cobranças</h3>
              {charges.length === 0 ? (
                <EmptyState icon={Wallet} message="Nenhuma cobrança encontrada para este lead." />
              ) : (
                <div className="space-y-2">
                  {charges.map((c) => {
                    const chargeAmount = resolveLeadChargeAmount(c);
                    return (
                      <div key={c.id} className="bg-card rounded-xl border border-border p-4 flex items-center justify-between">
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-bold text-foreground truncate">{c.description || c.type}</p>
                          <p className="text-xs text-muted-foreground mt-0.5">
                            Vencimento: {c.due_date ? new Date(c.due_date).toLocaleDateString("pt-BR") : "—"}
                            {c.reference_month && ` · Ref: ${c.reference_month}`}
                          </p>
                        </div>
                        <div className="flex items-center gap-3 shrink-0">
                          <span className="font-bold text-foreground">
                            {chargeAmount === null ? "A apurar" : fmt(chargeAmount)}
                          </span>
                          <span className={clsx("px-2.5 py-1 text-xs font-bold rounded-full", c.status === "PAID" ? "bg-status-ok-soft text-status-ok" : c.status === "OVERDUE" ? "bg-status-critical-soft text-status-critical" : c.status === "CANCELLED" ? "bg-muted text-muted-foreground" : "bg-status-warning-soft text-status-warning")}>
                            {c.status === "PAID" ? "Pago" : c.status === "OVERDUE" ? "Vencido" : c.status === "CANCELLED" ? "Cancelado" : c.status === "PARTIALLY_PAID" ? "Parcial" : "Pendente"}
                          </span>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          )}

          {activeTab === "tickets" && (
            <div className="space-y-4">
              <h3 className="text-lg font-bold text-foreground">Tickets de manutenção</h3>
              {maintenanceTickets.length === 0 ? (
                <EmptyState icon={Wrench} message="Nenhum ticket de manutenção." />
              ) : (
                <div className="space-y-2">
                  {maintenanceTickets.map((t) => (
                    <Link key={t.id} to={`/manutencao?ticket=${t.id}`} className="block bg-card rounded-xl border border-border p-4 hover:border-primary-300 hover:shadow-md transition-all group">
                      <div className="flex items-center justify-between gap-3">
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-bold text-foreground truncate group-hover:text-primary-700 transition-colors">{t.category || "Manutenção"} · {t.priority || "Normal"}</p>
                          <p className="text-xs text-muted-foreground mt-0.5 line-clamp-1">{t.description || t.property_title || "—"}</p>
                        </div>
                        <span className={clsx("px-2.5 py-1 text-xs font-bold rounded-full shrink-0", t.status === "Concluído" ? "bg-status-ok-soft text-status-ok" : t.status === "Em Andamento" ? "bg-status-info-soft text-status-info" : t.status === "Cancelado" ? "bg-muted text-muted-foreground" : "bg-status-warning-soft text-status-warning")}>
                          {t.status}
                        </span>
                      </div>
                    </Link>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      <CaptacaoWizard open={isCaptacaoWizardOpen} onClose={() => setIsCaptacaoWizardOpen(false)} initialOwnerLeadId={id ?? undefined} />

      {isDisqualifyOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" onClick={() => { if (!isDisqualifying) setIsDisqualifyOpen(false); }}>
          <div className="bg-card rounded-2xl shadow-xl w-full max-w-md p-6" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-lg font-bold text-foreground mb-1">Desqualificar lead</h3>
            <p className="text-sm text-muted-foreground mb-4">
              Informe o motivo. O lead será marcado como desqualificado e o motivo fica registrado no histórico.
            </p>
            <label className="block text-xs font-semibold text-muted-foreground mb-1">Motivo</label>
            <select value={disqualifyReason} onChange={(e) => setDisqualifyReason(e.target.value)} className="w-full px-3 py-2 mb-3 bg-card border border-border rounded-lg text-sm text-foreground">
              {["Sem renda comprovada", "Sem interesse", "Lead duplicado", "Dados inválidos", "Outro"].map((r) => (
                <option key={r} value={r}>
                  {r}
                </option>
              ))}
            </select>
            {disqualifyReason === "Outro" && (
              <input type="text" value={disqualifyNote} onChange={(e) => setDisqualifyNote(e.target.value)} placeholder="Descreva o motivo" className="w-full px-3 py-2 mb-3 bg-card border border-border rounded-lg text-sm text-foreground" />
            )}
            <div className="flex justify-end gap-2 mt-2">
              <button onClick={() => setIsDisqualifyOpen(false)} disabled={isDisqualifying} className="px-4 py-2 bg-card border border-border text-foreground text-sm font-bold rounded-lg hover:bg-muted disabled:opacity-50 transition-colors">
                Cancelar
              </button>
              <button onClick={handleDisqualify} disabled={isDisqualifying} className="px-4 py-2 bg-status-critical text-white text-sm font-bold rounded-lg hover:bg-status-critical/90 disabled:opacity-50 transition-colors">
                {isDisqualifying ? "Desqualificando..." : "Desqualificar"}
              </button>
            </div>
          </div>
        </div>
      )}

      {isCreatingDeal && (
        <EsteiraDealModal
          open={isCreatingDeal}
          onClose={() => {
            setIsCreatingDeal(false);
          }}
          onSuccess={async () => {
            setIsCreatingDeal(false);
            await fetchLeadDetails();
            toast.success("Negócio criado com sucesso!");
          }}
          initialLeadId={lead.id}
          initialPropertyTitle={lead.property}
        />
      )}

      {isClientDrawerOpen && id && (
        <ClientContextDrawer open={isClientDrawerOpen} onClose={() => setIsClientDrawerOpen(false)} clientId={lead?.accountId || id} />
      )}

      {/* O painel vive no nível da página: o botão "Agendar" do cabeçalho abre a
          agenda mesmo com o operador na aba Informações (montado dentro da aba,
          o clique não tinha o que abrir). */}
      <LeadAppointmentPanel
        open={appointmentPanel.open}
        lead={{ id: lead.id, name: lead.name, propertyId: lead.property || null }}
        appointment={appointmentPanel.appointment}
        properties={properties}
        leads={leads}
        onClose={() => setAppointmentPanel({ open: false, appointment: null })}
      />
    </div>
  );
}

function Card({ title, icon, children }: { title: string; icon: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="bg-card rounded-3xl border border-border shadow-sm p-6 relative overflow-hidden">
      <div className="absolute top-0 right-0 w-24 h-24 bg-gradient-to-br from-primary-50 to-transparent rounded-bl-full -z-0"></div>
      <h3 className="text-base font-bold text-foreground mb-6 relative z-10 flex items-center gap-2">
        {icon} {title}
      </h3>
      <div className="relative z-10">{children}</div>
    </div>
  );
}

function Label({ children }: { children: React.ReactNode }) {
  return <p className="text-micro font-bold text-muted-foreground uppercase tracking-wider mb-1">{children}</p>;
}

function AgentDisplay({ agentId, agents, user }: { agentId?: string; agents: { id: string; name: string; role: string; avatar?: string }[]; user?: any }) {
  const agent = agents.find((a) => a.id === agentId);
  const agentToDisplay =
    agent ||
    (agentId === user?.id && user
      ? { id: user.id, name: user.fullname || `${user.firstname || ""} ${user.lastname || ""}`.trim() || "Você", role: user.role || "Corretor", avatar: user.avatar_url || "" }
      : undefined);
  if (!agentToDisplay) return <div className="text-sm font-medium text-muted-foreground italic">Não atribuído</div>;
  return (
    <div className="flex items-center gap-3">
      <Avatar className="w-8 h-8 border border-border">
        {agentToDisplay.avatar ? <AvatarImage src={getAvatarThumbUrl(agentToDisplay.avatar, 64)} alt={agentToDisplay.name} /> : null}
        <AvatarFallback className="text-sm font-bold">{agentToDisplay.name.charAt(0).toUpperCase()}</AvatarFallback>
      </Avatar>
      <div>
        <div className="text-sm font-bold text-foreground">{agentToDisplay.name}</div>
        <div className="text-xs text-muted-foreground">{agentToDisplay.role}</div>
      </div>
    </div>
  );
}

function DealCard({ deal, properties, roleOfLead, isDuplicate }: { deal: EsteiraDeal; properties: { id: string; title: string; image: string }[]; roleOfLead?: "owner" | "customer" | "both"; isDuplicate?: boolean }) {
  const property = properties.find((p) => p.id === deal.propertyId);
  const stageColors: Record<string, string> = {
    "Em Negociação": "bg-status-info-soft text-status-info",
    Proposta: "bg-status-warning-soft text-status-warning",
    "Análise": "bg-stage-contract/10 text-stage-contract",
    Contrato: "bg-primary-50 text-primary-700",
    "Concluído": "bg-status-ok-soft text-status-ok",
    Perdido: "bg-status-critical-soft text-status-critical",
  };
  const roleLabel = (() => {
    if (!roleOfLead) return null;
    if (roleOfLead === "both") return "Prop. & Cliente";
    if (roleOfLead === "owner") return "Proprietário";
    return deal.type === "Venda" ? "Comprador" : "Inquilino";
  })();
  return (
    <Link to="/esteira" className="bg-card rounded-xl border border-border p-4 hover:border-primary-300 hover:shadow-md transition-all group block">
      <div className="flex items-start gap-3">
        {property && (
          <div className="w-14 h-14 rounded-lg bg-muted overflow-hidden shrink-0">
            <MediaImage src={property.image} alt={property.title || "Imóvel"} className="w-full h-full object-cover" />
          </div>
        )}
        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between mb-1 gap-2">
            <p className="text-sm font-bold text-foreground truncate group-hover:text-primary-700">{deal.property}</p>
            <span className={clsx("px-2 py-0.5 text-micro font-bold rounded-full uppercase shrink-0", stageColors[deal.stage] || "bg-muted text-muted-foreground")}>
              {deal.stage}
            </span>
          </div>
          <p className="text-xs text-muted-foreground">
            {deal.type} | {fmt(deal.value)}
          </p>
          <div className="flex items-center justify-between gap-2 mt-1">
            <p className="text-xs text-muted-foreground">{deal.date}</p>
            <div className="flex items-center gap-1.5 shrink-0">
              {isDuplicate && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 text-micro font-bold rounded-full bg-status-warning-soft text-status-warning border border-status-warning/30">
                  <AlertCircle className="w-3 h-3" /> Duplicado
                </span>
              )}
              {roleLabel && <span className="px-2 py-0.5 text-micro font-bold rounded-full bg-primary-50 text-primary-700 border border-primary-200">{roleLabel}</span>}
            </div>
          </div>
        </div>
      </div>
    </Link>
  );
}
