import { usePortalContact } from "../hooks/usePortalContact";
import { PortalVisitActions } from "../components/PortalVisitActions";
import { supportErrorMessage } from "../../../shared/lib/errorDiagnostics";
import { isAxiosError } from "axios";
import { useQueryClient } from "@tanstack/react-query";
import { usePortalOverview } from "../hooks/usePortalOverview";
import { portalKeys } from "../hooks/queryKeys";
import { isVisitClosed, visitPickerDefaults } from "./portalVisitPresentation";
import { describeListingPrice, isRentListing } from "../../../shared/utils/listingPrice";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router";
import { usePortalAuth } from "../context/PortalAuthContext";

import { usePortalDeals } from "../context/PortalDealsContext";
import { portalService, PortalPropertySummary, type PortalOverviewLead } from "../services/portal.service";
import { DealStatus } from "../../../shared/services/deal.service";
import {
  Briefcase,
  Clock,
  FileText,
  ChevronRight,
  CalendarDays,
  Receipt,
  Wrench,
  MessageSquareText,
  Heart,
  Home,
  RefreshCw,
  Search,
  UserRound,
  Loader2,
  Eye,
  Sparkles,
} from "lucide-react";
import { toast } from "sonner";
import { NotificationCenter } from "../components/NotificationCenter";
import { SystemNotificationPermissionPrompt } from "../../../shared/components/SystemNotificationPermissionPrompt";
import { RelationshipSwitcher } from "../components/RelationshipSwitcher";
import { AgencyIdentity } from "../components/AgencyIdentity";
import { PropertyMediaCarousel } from "../components/PropertyMediaCarousel";
import { PortalError, PortalLoading } from "../components/PortalState";
import { DealStageStepper } from "../components/DealStageStepper";
import { AppointmentPickerDialog, type AppointmentPickerValues } from "../components/AppointmentPickerDialog";
import { Button } from "../../../shared/ui/button";
import { Skeleton } from "../../../shared/ui/skeleton";
import { usePortalDealFavorites, usePortalDealProperty, usePortalDealChecklist, usePortalDealRecommendations } from "../hooks/useDealResources";
import { useToggleDealFavorite } from "../hooks/useDealMutations";
import { getStageMeta } from "../../../shared/deals/dealStages";
import { usePortalPersona } from "../hooks/usePortalPersona";
import {
  dealForLead,
  effectiveLeadStatus,
  leadJourney,
  leadPropertyLabel,
  leadRelationDate,
  leadStatusLabel,
  nextLeadAction,
  relationshipDateLabel,
  roleLabelForLead,
} from "../lib/portalRelationship";
import { format } from "date-fns";
import { PortalPage } from "../shell/PortalPage";
import type { PortalQuickAction } from "../lib/portalPersona";

/** Ícone de cada atalho — o atalho em si vem do dicionário por persona. */
const QUICK_ACTION_ICONS: Record<PortalQuickAction["key"], typeof Search> = {
  search: Search,
  properties: Briefcase,
  finance: Receipt,
  rentals: Home,
  tickets: Wrench,
  contact: MessageSquareText,
};

const STAGE_LABELS: Record<DealStatus, string> = {
  NEGOTIATION: "Em Negociação",
  PROPOSAL: "Proposta",
  ANALYSIS: "Análise",
  CONTRACT: "Contrato",
  CONVERTED: "Concluído",
  CANCELLED: "Perdido",
};

const STAGE_ORDER: DealStatus[] = ["NEGOTIATION", "PROPOSAL", "ANALYSIS", "CONTRACT", "CONVERTED"];

function getStageProgress(status: DealStatus) {
  const idx = STAGE_ORDER.indexOf(status);
  if (idx < 0) return { current: 0, total: STAGE_ORDER.length, pct: 0 };
  return {
    current: idx + 1,
    total: STAGE_ORDER.length,
    pct: Math.round(((idx + 1) / STAGE_ORDER.length) * 100),
  };
}

function matchLabel(score?: number) {
  if (typeof score !== "number") return "Selecionado para você";
  if (score >= 80) return "Excelente combinação";
  if (score >= 60) return "Boa combinação";
  return "Combinação possível";
}

function matchReason(property: PortalPropertySummary) {
  const reasons = property.match_reasons;
  if (!reasons) return "Escolhido com base no seu interesse";
  const matches = [
    { value: reasons.business ?? 0, weight: reasons.used_weights.business ?? 0, label: "tipo de negócio" },
    { value: reasons.price, weight: reasons.used_weights.price, label: "orçamento" },
    { value: reasons.location, weight: reasons.used_weights.location, label: "região" },
    { value: reasons.bedrooms, weight: reasons.used_weights.bedrooms, label: "quartos" },
    { value: reasons.area, weight: reasons.used_weights.area, label: "área" },
  ]
    .filter((item) => item.weight > 0)
    .sort((a, b) => b.value / b.weight - a.value / a.weight);
  const best = matches[0];
  return best ? `Combina com seu ${best.label}` : "Escolhido com base no seu interesse";
}


function buildPropertyTitle(p: PortalPropertySummary) {
  const parts: string[] = [];
  if (p.bedrooms) parts.push(`${p.bedrooms} quartos`);
  if (p.total_area_m2) parts.push(`${Math.round(p.total_area_m2)}m²`);
  return parts.join(" • ") || p.title || "Imóvel";
}

export function PortalDashboard() {
  const { user } = usePortalAuth();
  const navigate = useNavigate();
  const { deals, activeDeal, activeDealId, isLoading, error, retry, setActiveDealId, updateDeal } = usePortalDeals();
  const propertyQuery = usePortalDealProperty(activeDeal?.id);
  const recommendationsQuery = usePortalDealRecommendations(activeDeal?.id);
  const checklistQuery = usePortalDealChecklist(activeDeal?.id);
  const activeProperty = propertyQuery.data ?? null;
  const recommendations = recommendationsQuery.data ?? [];
  const checklist = checklistQuery.data ?? [];
  const isChecklistLoading = checklistQuery.isPending;
  const isPropertyLoading = propertyQuery.isPending;
  const isRecommendationsLoading = recommendationsQuery.isPending;
  const propertyError = propertyQuery.isError && !propertyQuery.data;
  const recommendationsError = recommendationsQuery.isError && !recommendationsQuery.data;
  const queryClient = useQueryClient();
  const overviewQuery = usePortalOverview(!isLoading);
  const overview = overviewQuery.data ?? [];
  const overviewLoading = overviewQuery.isPending;
  const overviewError = overviewQuery.isError;
  const setOverview = (update: (current: PortalOverviewLead[]) => PortalOverviewLead[]) =>
    queryClient.setQueryData<PortalOverviewLead[]>(portalKeys.overview(), (current) => update(current ?? []));
  const [visitOpen, setVisitOpen] = useState(false);
  const [visitLeadId, setVisitLeadId] = useState<string | null>(null);
  const [activeOverviewLeadId, setActiveOverviewLeadId] = useState<string | null>(() => {
    if (typeof window === "undefined") return null;
    try {
      return window.localStorage.getItem("portal_active_lead_id");
    } catch {
      return null;
    }
  });
  const activeOverviewLead = useMemo(
    () => overview.find((lead) => lead.id === activeOverviewLeadId) ?? overview[0] ?? null,
    [activeOverviewLeadId, overview],
  );
  const selectOverviewLead = (leadId: string) => {
    setActiveOverviewLeadId(leadId);
    try {
      window.localStorage.setItem("portal_active_lead_id", leadId);
    } catch {}
  };
  const standaloneLeads = useMemo(
    () => overview.filter((lead) => !dealForLead(lead, deals)),
    [deals, overview],
  );
  const visitLead = useMemo(
    () => overview.find((lead) => lead.id === visitLeadId) ?? activeOverviewLead,
    [activeOverviewLead, overview, visitLeadId],
  );
  /**
   * Negócio de cada lead. Esta jornada (sem negócio ativo) é a única que mostra
   * a visita do lead, mas o `.ics` do portal é servido por NEGÓCIO — a agenda do
   * negócio também devolve as visitas anteriores do lead. Quando o cliente já
   * tem negócio para o lead, o botão "Adicionar ao calendário (.ics)" aparece;
   * sem negócio não existe rota do cliente que entregue o arquivo.
   */
  const dealIdByLead = useMemo(() => {
    const map = new Map<string, string>();
    for (const d of deals) {
      if (d.lead_id) map.set(d.lead_id, d.id);
      if (d.customer_id && !map.has(d.customer_id)) map.set(d.customer_id, d.id);
    }
    return map;
  }, [deals]);
  // Vocabulário do cliente: o papel vem do negócio ativo; antes do negócio,
  // vale o papel do lead no atendimento (comprador, locatário, proprietário...).
  const { persona, copy } = usePortalPersona({ leadRole: activeOverviewLead?.role ?? null });
  const openVisitPicker = (lead: PortalOverviewLead) => {
    setVisitLeadId(lead.id);
    setVisitOpen(true);
  };
  const handleRequestVisit = async (values: AppointmentPickerValues) => {
    if (!visitLead) return;
    const request = {
      date: format(values.date, "yyyy-MM-dd"),
      time: values.startTime,
      end_time: values.endTime,
      note: values.note?.trim(),
    };
    try {
      const updating = Boolean(visitLead.visit_request && !isVisitClosed(visitLead.visit_request));
      if (updating) {
        await portalService.updateLeadVisit(visitLead.id, request);
      } else {
        await portalService.requestLeadVisit(visitLead.id, {
          property_id: visitLead.recommended_property_id,
          ...request,
        });
      }
      toast.success(
        updating
          ? "Novo horário solicitado. Seu corretor fará a confirmação."
          : "Visita solicitada. Seu corretor fará a confirmação.",
      );
      setOverview((prev) =>
        prev.map((lead) =>
          lead.id === visitLead.id
            ? {
                ...lead,
                visit_request: {
                  property_id: updating ? lead.visit_request?.property_id : lead.recommended_property_id,
                  ...request,
                  status: "Aguardando",
                },
              }
            : lead,
        ),
      );
      void queryClient.invalidateQueries({ queryKey: portalKeys.overview() });
    } catch (err: unknown) {
      const message = isAxiosError<{ message?: string }>(err) && (err.response?.status ?? 500) < 500 ? err.response?.data?.message : undefined;
      toast.error(supportErrorMessage(err, message || "Não foi possível solicitar esse horário."));
      throw err;
    }
  };
  const { openContact, opening: chatOpening } = usePortalContact();
  const handleOpenChat = (le: PortalOverviewLead) => openContact({ leadId: le.id });
  const [cancelLoading, setCancelLoading] = useState<string | null>(null);
  const handleCancelVisit = async (le: PortalOverviewLead) => {
    setCancelLoading(le.id);
    try {
      await portalService.cancelLeadVisit(le.id);
      toast.success("Visita cancelada.");
      setOverview((prev) => prev.map((o) => o.id === le.id ? { ...o, visit_request: { ...(o.visit_request!), status: "Cancelado" } } : o));
      void queryClient.invalidateQueries({ queryKey: portalKeys.overview() });
    } catch (err: unknown) {
      toast.error(supportErrorMessage(err, "Não foi possível cancelar a visita."));
      throw err;
    } finally { setCancelLoading(null); }
  };
  const favoritesQuery = usePortalDealFavorites(activeDeal?.id);
  const toggleFavoriteMutation = useToggleDealFavorite(activeDeal?.id);
  const favoriteIds = useMemo(() => new Set(favoritesQuery.data ?? []), [favoritesQuery.data]);

  const refreshChecklist = useCallback(async (dealId: string) => {
    await queryClient.invalidateQueries({ queryKey: portalKeys.dealChecklist(dealId) });
  }, [queryClient]);

  useEffect(() => {
    if (activeDeal?.id && checklistQuery.data && checklistQuery.data !== activeDeal.checklist) {
      updateDeal(activeDeal.id, (previous) => ({ ...previous, checklist: checklistQuery.data }));
    }
  }, [activeDeal?.id, checklistQuery.data, updateDeal]);

  const firstName = user?.firstname || user?.name?.split(" ")[0] || "";
  const greeting = firstName ? `Olá ${firstName}` : "Olá";

  if (isLoading) {
    return <PortalLoading rows={4} />;
  }

  if (error && deals.length === 0) {
    return (
      <PortalPage width="xl" className="px-5 pt-8 overflow-x-hidden">
        <h1 className="text-[24px] font-bold text-foreground tracking-tight">{greeting}</h1>
        <div className="mt-6">
          <PortalError onRetry={retry} />
        </div>
      </PortalPage>
    );
  }

  if (!activeDeal) {
    // Jornada pré-negócio: cliente convidado ao portal ainda sem negócio na
    // esteira. Em vez de um estado vazio, apresentamos a jornada (imóveis +
    // contato) e deixamos claro que, quando um negócio for aberto, ele aparece
    // aqui — sem perder o contexto do convite.
    return (
      <PortalPage width="xl" className="px-5 pt-8 overflow-x-hidden">
        <header className="max-w-2xl">
          <p className="text-sm font-semibold text-primary">{copy.journeyEyebrow}</p>
          <h1 className="mt-1 text-2xl font-bold tracking-tight text-foreground sm:text-3xl">{greeting}</h1>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">{copy.journeyBody}</p>
        </header>
        <div className="mt-6 space-y-4">
          {activeOverviewLead ? (
            <div className="rounded-2xl border border-border bg-card p-4 shadow-sm">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <AgencyIdentity tenantId={activeOverviewLead.tenant_id} showSiteLink />
                <span className="rounded-full border border-border bg-muted px-2.5 py-1 text-[11px] font-bold text-muted-foreground">
                  {leadStatusLabel(effectiveLeadStatus(activeOverviewLead))}
                </span>
              </div>
              <p className="mt-3 text-sm font-semibold text-foreground">
                Próxima ação: {nextLeadAction(activeOverviewLead).title}
              </p>
              <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                {nextLeadAction(activeOverviewLead).description}
              </p>
              <div className="mt-3 flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
                <span>{roleLabelForLead(activeOverviewLead)}</span>
                {leadRelationDate(activeOverviewLead) ? <span>· Atualizado em {relationshipDateLabel(leadRelationDate(activeOverviewLead))}</span> : null}
                {activeOverviewLead.broker_name ? <span>· Corretor {activeOverviewLead.broker_name}</span> : null}
              </div>
              <div className="mt-3 flex flex-wrap items-center gap-3">
                {leadJourney(effectiveLeadStatus(activeOverviewLead)).map((step) => (
                  <span key={step.key} className={`inline-flex items-center gap-1 text-[10px] font-semibold ${
                    step.state === "done" ? "text-status-ok" : step.state === "current" ? "text-primary-700" : "text-muted-foreground"
                  }`}>
                    <span className={`size-1.5 rounded-full ${step.state === "todo" ? "bg-border" : step.state === "current" ? "bg-primary" : "bg-status-ok"}`} />
                    {step.label}
                  </span>
                ))}
              </div>
              <Link
                to={`/portal/atendimentos/${activeOverviewLead.id}`}
                className="mt-3 inline-flex min-h-9 items-center gap-1 text-xs font-bold text-primary-700 underline underline-offset-4"
              >
                Ver atendimento completo
                <ChevronRight className="size-3.5" aria-hidden="true" />
              </Link>
            </div>
          ) : null}
          <div className="rounded-2xl border border-border bg-card p-5">
            <h2 className="text-base font-bold text-foreground flex items-center gap-2">
              <Home className="w-4 h-4 text-primary-600" /> Bem-vindo(a) à sua jornada
            </h2>
            <p className="text-sm text-muted-foreground mt-2">
              {firstName ? `${firstName}, ` : ""}{copy.recommendationsIntro || copy.journeyBody}
            </p>
            <div className="mt-4 grid grid-cols-1 sm:grid-cols-2 gap-3">
              <Link
                to={copy.journeyPrimaryCta.to}
                className="rounded-xl border border-border bg-muted hover:bg-muted/60 px-4 py-3 text-sm font-semibold text-foreground flex items-center gap-2 transition-colors"
              >
                {copy.journeyPrimaryCta.to === "/portal/buscar" ? (
                  <Search className="w-4 h-4 text-muted-foreground" />
                ) : (
                  <MessageSquareText className="w-4 h-4 text-muted-foreground" />
                )}
                {copy.journeyPrimaryCta.label}
              </Link>
              <Link
                to={copy.journeySecondaryCta.to}
                className="rounded-xl border border-border bg-muted hover:bg-muted/60 px-4 py-3 text-sm font-semibold text-foreground flex items-center gap-2 transition-colors"
              >
                <MessageSquareText className="w-4 h-4 text-muted-foreground" /> {copy.journeySecondaryCta.label}
              </Link>
            </div>
          </div>
          {overview.length > 0 && (
            <RelationshipSwitcher
              deals={deals}
              leads={overview}
              activeLeadId={activeOverviewLead?.id}
              onSelectDeal={setActiveDealId}
              onSelectLead={selectOverviewLead}
              title="Meus atendimentos"
            />
          )}
          {overviewLoading && <PortalLoading rows={2} header={false} className="px-0 pb-0 pt-0" />}
          {overviewError && !overviewLoading && (
            <PortalError
              title="Não foi possível carregar sua busca"
              description="Você ainda pode explorar os imóveis ou tentar novamente."
              onRetry={() => void overviewQuery.refetch()}
              className="py-8"
            />
          )}
          {!overviewLoading &&
            (() => {
              const lead = activeOverviewLead;
              if (!lead) return null;
              const recId = lead.recommended_property_id;
              const recMatch = recId ? lead.matched_properties?.find((m) => m.property.id === recId) : undefined;
              const rec = lead.recommended_property || recMatch?.property;
              // Lead SEM imóvel recomendado (ou com recomendação que sumiu do
              // portfólio): a visita agendada pelo OPERADOR continua acessível
              // aqui — o visit_request é sincronizado pelo backend a partir do
              // evento appointment.created. Antes este card retornava null e o
              // agendamento ficava invisível até existir recomendação/contrato.
              if (!recId || !rec) {
                return (
                  <div className="rounded-2xl border border-border bg-card p-5">
                    <h2 className="text-base font-bold text-foreground flex items-center gap-2">
                      <CalendarDays className="w-4 h-4 text-primary-600" /> {copy.visitsTitle}
                    </h2>
                    <p className="text-sm text-muted-foreground mt-2">
                      {copy.propertyVisitCta
                        ? "Visitas agendadas pela imobiliária aparecem aqui. Você pode alterar o horário ou cancelar quando precisar."
                        : copy.visitsEmptyBody}
                    </p>
                    <div className="mt-4 flex flex-wrap items-center gap-2">
                      {lead.broker_name && (
                        <span className="inline-flex items-center gap-2 text-xs text-muted-foreground">
                          {lead.broker_avatar ? (
                            <img src={lead.broker_avatar} alt={lead.broker_name} className="w-6 h-6 rounded-full object-cover" />
                          ) : (
                            <div className="w-6 h-6 rounded-full bg-primary/15 text-primary text-micro font-bold flex items-center justify-center"><UserRound className="w-3 h-3" /></div>
                          )}
                          Corretor: <strong className="text-foreground">{lead.broker_name}</strong>
                        </span>
                      )}
                      <button
                        type="button"
                        onClick={() => handleOpenChat(lead)}
                        disabled={chatOpening}
                        className="inline-flex items-center gap-2 rounded-xl border border-border bg-muted hover:bg-muted/60 px-4 py-3 text-sm font-semibold text-foreground transition-colors disabled:opacity-60"
                      >
                        {chatOpening ? <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" /> : <MessageSquareText className="w-4 h-4 text-muted-foreground" />} Falar com meu corretor
                      </button>
                      {lead.visit_request?.status ? (
                        <PortalVisitActions lead={lead} dealId={dealIdByLead.get(lead.id)} loadCalendar={() => portalService.downloadLeadVisitCalendar(lead.id)} cancelLoading={cancelLoading === lead.id}
                          onReschedule={openVisitPicker} onCancel={handleCancelVisit} />
                      ) : (
                        <p className="text-sm text-muted-foreground">{copy.visitsEmptyBody}</p>
                      )}
                    </div>
                  </div>
                );
              }
              // O overview pode trazer o ID da recomendação SEM o objeto (imóvel
              // excluído depois, ou snapshot que não foi gravado). Sem esta
              // guarda o card lê `rec.title` de undefined e derruba a tela.
              const valor = describeListingPrice(
                rec ? (isRentListing((rec as any).listing_type) ? (rec as any).rent_price : (rec as any).sale_price) : 0,
                (rec as any)?.listing_type,
              );
              const recScore = recMatch?.score;
              return (
                <div className="rounded-2xl border-2 border-primary/40 bg-card overflow-hidden shadow-sm">
                  {/* Imagem em destaque — COVER full-width com carrossel */}
                  <div className="relative w-full h-48 bg-muted overflow-hidden">
                    {(rec as any)?.media_images?.length > 0 ? (
                      <PropertyMediaCarousel
                        images={(rec as any).media_images.map((u: string) => ({ url: u }))}
                        fallbackUrl={(rec as any).thumbnail_url}
                        alt={rec?.title || "Imóvel"}
                        className="w-full h-full"
                        imgClassName="w-full h-full object-cover"
                      />
                    ) : (rec as any)?.thumbnail_url ? (
                      <img src={(rec as any).thumbnail_url} alt={rec?.title || "Imóvel"} className="w-full h-full object-cover" />
                    ) : (
                      <div className="w-full h-full flex items-center justify-center text-muted-foreground"><Home className="w-8 h-8" /></div>
                    )}
                    {typeof recScore === "number" && (
                      <span className="absolute top-3 right-3 bg-primary/90 text-primary-foreground text-[11px] font-bold px-2.5 py-1 rounded-full backdrop-blur-sm">
                        {recScore.toFixed(0)}% compatível
                      </span>
                    )}
                  </div>
                  <div className="p-4">
                    <h2 className="text-base font-bold text-foreground flex items-center gap-2">
                      <Heart className="w-4 h-4 text-primary-600" /> {copy.recommendationsTitle ?? "Sugestão do seu corretor"}
                    </h2>
                    {rec ? (
                      <div className="mt-2">
                        <p className="text-lg font-bold text-foreground truncate">{rec.title}</p>
                        <p className="text-sm text-muted-foreground truncate">
                          {[rec.neighborhood, rec.city, rec.state].filter(Boolean).join(", ")}
                        </p>
                        <div className="flex items-center justify-between mt-2">
                          <p className="flex items-baseline gap-1.5 text-xl font-bold text-foreground tabular-nums">
                            {valor.label && (
                              <span className="text-caption font-bold uppercase tracking-wide text-muted-foreground">{valor.label}</span>
                            )}
                            <span>{valor.amount}</span>
                            {valor.period && <span className="text-xs font-normal text-muted-foreground">{valor.period}</span>}
                          </p>
                          {typeof recScore === "number" && (
                            <span className="text-xs font-bold text-status-ok">{matchLabel(recScore)}</span>
                          )}
                        </div>
                        {rec && (
                          <p className="text-xs text-muted-foreground mt-2 flex items-center gap-1.5">
                            <Sparkles className="w-3.5 h-3.5 text-primary-600" />
                            {matchReason(rec as any)}
                          </p>
                        )}
                      </div>
                    ) : (
                      <p className="text-sm text-muted-foreground mt-2">{copy.recommendationsIntro}</p>
                    )}
                  <div className="mt-3 flex flex-wrap items-center gap-2">
                    {lead?.broker_name && (
                      <span className="inline-flex items-center gap-2 text-xs text-muted-foreground">
                        {lead.broker_avatar ? (
                          <img src={lead.broker_avatar} alt={lead.broker_name} className="w-6 h-6 rounded-full object-cover" />
                        ) : (
                          <div className="w-6 h-6 rounded-full bg-primary/15 text-primary text-caption font-bold flex items-center justify-center"><UserRound className="w-3 h-3" /></div>
                        )}
                        Corretor: <strong className="text-foreground">{lead.broker_name}</strong>
                      </span>
                    )}
                    <button
                      type="button"
                      onClick={() => handleOpenChat(lead)}
                      disabled={chatOpening}
                      className="inline-flex items-center gap-2 rounded-xl border border-border bg-muted hover:bg-muted/60 px-4 py-3 text-sm font-semibold text-foreground transition-colors disabled:opacity-60"
                    >
                      {chatOpening ? <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" /> : <MessageSquareText className="w-4 h-4 text-muted-foreground" />} Falar com meu corretor
                    </button>
                    <Link
                      to={`/portal/imoveis/${recId}`}
                      state={{ property: lead?.recommended_property, leadId: lead?.id }}
                      className="inline-flex items-center gap-2 rounded-xl border border-border bg-muted hover:bg-muted/60 px-4 py-3 text-sm font-semibold text-foreground transition-colors"
                    >
                      <Eye className="w-4 h-4 text-muted-foreground" /> Ver imóvel
                    </Link>
                    {/* Quem anuncia o imóvel não agenda visita ao próprio imóvel:
                        as visitas são conduzidas pela imobiliária. */}
                    {copy.propertyVisitCta && (lead?.visit_request?.status ? (
                      <PortalVisitActions lead={lead} dealId={dealIdByLead.get(lead.id)} loadCalendar={() => portalService.downloadLeadVisitCalendar(lead.id)} cancelLoading={cancelLoading === lead.id}
                        onReschedule={openVisitPicker} onCancel={handleCancelVisit} />
                    ) : (
                      <button
                        type="button"
                        onClick={() => openVisitPicker(lead)}
                        className="inline-flex items-center gap-2 rounded-xl border border-border bg-muted hover:bg-muted/60 px-4 py-3 text-sm font-semibold text-foreground transition-colors"
                      >
                        <CalendarDays className="w-4 h-4 text-muted-foreground" /> {copy.propertyVisitCta}
                      </button>
                    ))}
                  </div>
                  </div>
                </div>
              );
            })()}
          <AppointmentPickerDialog
            open={visitOpen}
            onOpenChange={setVisitOpen}
            onSubmit={handleRequestVisit}
            propertyLabel={visitLead?.recommended_property?.title}
            mode={visitLead?.visit_request && !isVisitClosed(visitLead.visit_request) ? "reschedule" : "create"}
            defaultValues={visitPickerDefaults(visitLead?.visit_request)}
          />
          {!overviewLoading && activeOverviewLead?.matched_properties?.length > 0 && (
            <div className="rounded-2xl border border-border bg-card p-5">
              <h2 className="text-base font-bold text-foreground flex items-center gap-2">
                <Heart className="w-4 h-4 text-primary-600" /> Imóveis que combinam com você
              </h2>
              <div className="mt-3 space-y-2">
                {activeOverviewLead.matched_properties.slice(0, 3).map((mp, i) => {
                  const p = mp.property;
                  const valor = describeListingPrice(
                    isRentListing(p.listing_type) ? p.rent_price : p.sale_price,
                    p.listing_type,
                  );
                  return (
                    <Link
                      key={p.id || i}
                      to={`/portal/imoveis/${p.id}`}
                      state={{ property: p, leadId: activeOverviewLead.id }}
                      className="flex items-start justify-between gap-3 rounded-xl border border-border bg-muted/50 px-3 py-2.5"
                    >
                      <div className="min-w-0">
                        <p className="text-sm font-semibold text-foreground truncate">{p.title}</p>
                        <p className="text-xs text-muted-foreground truncate">
                          {[p.neighborhood, p.city, p.state].filter(Boolean).join(", ")}
                        </p>
                      </div>
                      <div className="text-right shrink-0">
                        {valor.hasPrice && (
                          <p className="text-sm font-bold text-foreground">
                            {valor.label && <span className="mr-1 text-caption font-bold uppercase tracking-wide text-muted-foreground">{valor.label}</span>}
                            {valor.amount}
                            {valor.period && <span className="text-caption font-normal text-muted-foreground">{valor.period}</span>}
                          </p>
                        )}
                        {mp.score != null && (
                          <p className="text-xs font-bold text-status-ok">{Math.round(mp.score)}% match</p>
                        )}
                      </div>
                    </Link>
                  );
                })}
              </div>
            </div>
          )}
          <p className="text-xs text-muted-foreground text-center">
            Assim que um negócio for aberto para você, ele aparece aqui — acompanhe propostas,
            imóveis, financeiro e documentos.
          </p>
        </div>
      </PortalPage>
    );
  }

  const progress = getStageProgress(activeDeal.status);
  const pendingDocs = checklist.filter((c) => c.status === "PENDING" || c.status === "REJECTED").length;
  const cityLine = activeProperty
    ? `${activeProperty.neighborhood || ""}${activeProperty.neighborhood ? ", " : ""}${activeProperty.city || ""}${activeProperty.city ? ", " : ""}${activeProperty.state || ""}`
    : "";

  const retryOverview = () => {
    if (activeDeal?.id) void queryClient.invalidateQueries({ queryKey: portalKeys.deal(activeDeal.id) });
  };

  const toggleFavorite = async (propertyId: string) => {
    if (!activeDeal?.id || toggleFavoriteMutation.isPending) return;
    const favorite = !favoriteIds.has(propertyId);
    try {
      await toggleFavoriteMutation.mutateAsync({ propertyId, favorite });
      toast.success(favorite ? "Imóvel salvo nos favoritos." : "Imóvel removido dos favoritos.");
    } catch {
      toast.error("Não foi possível atualizar favoritos.");
    }
  };

  const stageMeta = getStageMeta(activeDeal.status);

  return (
    <PortalPage width="xl" className="overflow-x-hidden">
      <header className="relative flex items-start justify-between gap-4 px-5 pb-6 pt-8 sm:px-8">
        <div className="max-w-2xl">
          <p className="text-sm font-semibold text-primary">{copy.overviewEyebrow}</p>
          <h1 className="mt-1 text-2xl font-bold leading-tight tracking-tight text-foreground sm:text-3xl">
            {greeting}
          </h1>
          <p className="mt-2 text-sm leading-6 text-muted-foreground sm:text-base">{copy.stageHeadline[activeDeal.status] ?? "Acompanhe aqui as etapas do seu negócio."}</p>
          <div className="mt-3">
            <AgencyIdentity tenantId={activeDeal.tenant_id} showSiteLink />
          </div>
        </div>
        <div className="relative mt-1">
          <NotificationCenter />
          <SystemNotificationPermissionPrompt />
        </div>
      </header>

      <div className="mb-4 px-5 sm:px-8">
        <RelationshipSwitcher
          deals={deals}
          leads={overview}
          activeDealId={activeDealId}
          activeLeadId={null}
          onSelectDeal={setActiveDealId}
          onSelectLead={(leadId) => navigate(`/portal/atendimentos/${leadId}`)}
          title="Minhas relações"
        />
      </div>

      {(propertyQuery.isError || recommendationsQuery.isError || checklistQuery.isError) && (
        <div role="status" className="mx-5 mb-4 flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-status-warning/25 bg-status-warning-soft/60 p-4 sm:mx-8">
          <p className="text-sm text-muted-foreground">Algumas informações não foram atualizadas. Tente novamente.</p>
          <Button type="button" variant="outline" size="sm" onClick={retryOverview}>Tentar novamente</Button>
        </div>
      )}

      <div className="mb-4 px-5 sm:px-8">
        <DealStageStepper status={activeDeal.status} />
      </div>

      <section className="px-5 sm:px-8">
        <div className="bg-card rounded-3xl border border-border overflow-hidden shadow-sm">
          <div className="relative h-52 bg-border sm:h-72">
            {isPropertyLoading ? (
              <Skeleton className="w-full h-full rounded-none" />
            ) : propertyError ? (
              <div className="w-full h-full flex flex-col items-center justify-center gap-2 bg-muted px-5 text-center">
                <p className="text-sm font-semibold text-foreground">Não foi possível carregar o imóvel.</p>
                <Button type="button" variant="outline" size="sm" onClick={retryOverview}>
                  Tentar novamente
                </Button>
              </div>
            ) : ((activeProperty as any)?.media?.images?.length || activeProperty?.image_url) ? (
              <PropertyMediaCarousel
                images={(activeProperty as any)?.media?.images}
                fallbackUrl={activeProperty?.image_url}
                alt={activeProperty?.title || "Imóvel"}
                className="w-full h-full"
                imgClassName="w-full h-full object-cover"
              />
            ) : (
              <div className="w-full h-full bg-gradient-to-br from-slate-200 to-slate-100" />
            )}
            <div className="absolute top-4 left-4">
              <span className="inline-flex items-center gap-1.5 rounded-full bg-card/95 px-3 py-1.5 text-caption font-semibold text-foreground/80 shadow-sm backdrop-blur-sm">
                <span className={`h-1.5 w-1.5 rounded-full ${activeDeal.status === "CANCELLED" ? "bg-status-critical" : activeDeal.status === "CONVERTED" ? "bg-status-ok" : "bg-primary"}`} />
                {stageMeta.label}
              </span>
            </div>
          </div>

          <div className="p-5">
            <div className="mb-4">
              <h3 className="text-lg font-bold text-foreground">{activeProperty?.address || activeProperty?.title || "Imóvel"}</h3>
              {cityLine && <p className="text-sm text-muted-foreground mt-0.5">{cityLine}</p>}
            </div>

            <div className="mb-5 rounded-2xl bg-muted/50 p-4">
              <p className="mb-1 text-caption font-medium text-muted-foreground">Etapa atual</p>
              <div className="flex items-center gap-2">
                <p className="text-sm font-bold text-foreground">{STAGE_LABELS[activeDeal.status] || activeDeal.status}</p>
                <span className="text-micro font-bold px-2 py-0.5 rounded-full bg-card text-foreground/80 border border-border">
                  {persona.label}
                </span>
              </div>
              <p className="mt-1 text-caption leading-5 text-muted-foreground">{stageMeta.description}</p>
            </div>

            <div className="mb-6">
              <div className="w-full h-1.5 bg-muted rounded-full overflow-hidden mb-2">
                <div className="h-full bg-status-ok rounded-full" style={{ width: `${progress.pct}%` }} />
              </div>
              <p className="text-caption text-muted-foreground font-medium">
                Etapa {progress.current} de {progress.total} — {progress.pct}% concluído
              </p>
            </div>

            <div className="grid gap-2 sm:grid-cols-2">
              <Link
                to={`/portal/deals/${activeDeal.id}`}
                className="flex min-h-12 w-full items-center justify-center gap-2 rounded-xl bg-primary py-3.5 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2"
              >
                <FileText className="h-4 w-4" />
                {copy.primaryAction[activeDeal.status] ?? "Acompanhar"}
              </Link>
              {activeProperty?.id && (
                <Link
                  to={`/portal/imoveis/${activeProperty.id}`}
                  className="flex min-h-12 w-full items-center justify-center gap-2 rounded-xl border border-border bg-card py-3.5 text-sm font-semibold text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2"
                >
                  <Eye className="h-4 w-4" />
                  Ver detalhes do imóvel
                </Link>
              )}
            </div>
          </div>
        </div>
      </section>

      {(pendingDocs > 0 || activeDeal.status === "CONTRACT") && (
        <section className="mt-8 px-5 sm:px-8">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-title-lg font-bold text-foreground">Lembretes</h2>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                if (!activeDeal?.id) return;
                refreshChecklist(activeDeal.id).catch(() => {});
              }}
              disabled={isChecklistLoading}
              className="text-xs font-semibold text-muted-foreground"
            >
              <RefreshCw className="w-3.5 h-3.5 mr-1" />
              {isChecklistLoading ? "Atualizando..." : "Atualizar"}
            </Button>
          </div>
          <div className="space-y-3">
            {pendingDocs > 0 && (
              <Link
                to={`/portal/deals/${activeDeal.id}`}
                className="flex items-center justify-between bg-card rounded-2xl border border-border p-4 shadow-sm"
              >
                <div className="flex items-center gap-3">
                  <FileText className="w-5 h-5 text-muted-foreground/70" />
                  <span className="text-sm font-medium text-foreground/80">
                    {pendingDocs} documento{pendingDocs > 1 ? "s" : ""} pendente{pendingDocs > 1 ? "s" : ""}
                  </span>
                </div>
                <ChevronRight className="w-4 h-4 text-muted-foreground/60" />
              </Link>
            )}

            {activeDeal.status === "CONTRACT" && (
              <Link
                to={`/portal/deals/${activeDeal.id}`}
                className="flex items-center justify-between bg-card rounded-2xl border border-border p-4 shadow-sm"
              >
                <div className="flex items-center gap-3">
                  <Clock className="w-5 h-5 text-muted-foreground/70" />
                  <span className="text-sm font-medium text-foreground/80">Contrato disponível para assinatura</span>
                </div>
                <ChevronRight className="w-4 h-4 text-muted-foreground/60" />
              </Link>
            )}
          </div>
        </section>
      )}

      <section className="mt-8 px-5 sm:px-8">
        <h2 className="text-title-lg font-bold text-foreground mb-4">Acesso rápido</h2>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
          {copy.quickActions.map((action) => {
            const Icon = QUICK_ACTION_ICONS[action.key];
            return (
              <Link key={action.key} to={action.to} className="bg-card border border-border rounded-2xl p-4 text-left shadow-sm">
                <Icon className="w-5 h-5 text-muted-foreground mb-3" />
                <span className="text-sm font-medium text-foreground">{action.label}</span>
              </Link>
            );
          })}
        </div>
      </section>

      {standaloneLeads.length > 0 && (
        <section className="mt-10 px-5 sm:px-8">
          <div className="mb-4 flex items-center justify-between">
            <div>
              <h2 className="text-title-lg font-bold text-foreground">Seus outros atendimentos</h2>
              <p className="mt-1 text-xs text-muted-foreground">Conversas que ainda não viraram negócio.</p>
            </div>
            <Button asChild variant="link" className="h-auto p-0 text-sm font-medium text-status-ok">
              <Link to="/portal/leads">Ver todos</Link>
            </Button>
          </div>
          <div className="space-y-2">
            {standaloneLeads.slice(0, 3).map((lead) => (
              <Link
                key={lead.id}
                to={`/portal/atendimentos/${lead.id}`}
                className="flex items-center justify-between gap-3 rounded-2xl border border-border bg-card p-4 shadow-sm transition-colors hover:bg-muted/50"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-bold text-foreground">{leadPropertyLabel(lead)}</p>
                  <p className="mt-0.5 truncate text-xs text-muted-foreground">{leadStatusLabel(lead.status)}</p>
                  <div className="mt-2"><AgencyIdentity tenantId={lead.tenant_id} compact /></div>
                </div>
                <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
              </Link>
            ))}
          </div>
        </section>
      )}

      {copy.recommendationsTitle && (isRecommendationsLoading || recommendationsError || recommendations.length > 0) && (
        <section className="mt-10 px-5 sm:px-8">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-title-lg font-bold text-foreground">{copy.recommendationsTitle}</h2>
            {!recommendationsError && !isRecommendationsLoading && (
              <Button asChild variant="link" className="h-auto p-0 text-sm font-medium text-status-ok hover:text-status-ok">
                <Link to="/portal/buscar">Ver todos</Link>
              </Button>
            )}
          </div>

          {isRecommendationsLoading ? (
            <div className="flex gap-4 overflow-hidden -mx-5 px-5" aria-busy="true">
              {[0, 1].map((index) => (
                <Skeleton key={index} className="min-w-[240px] h-64 rounded-2xl" />
              ))}
            </div>
          ) : recommendationsError ? (
            <PortalError
              title="Não foi possível carregar as recomendações"
              description="Você ainda pode falar com seu corretor e tentar novamente."
              onRetry={retryOverview}
              className="py-8"
            />
          ) : (
            <div className="flex gap-4 overflow-x-auto pb-4 -mx-5 px-5 snap-x">
            {recommendations.map((prop) => (
              <div
                key={prop.id}
                className="min-w-[240px] w-[240px] bg-card rounded-2xl border border-border shadow-sm overflow-hidden snap-start"
              >
                <div className="relative h-32 bg-border">
                  {(prop as any)?.media?.images?.length || prop.image_url ? (
                    <PropertyMediaCarousel
                      images={(prop as any)?.media?.images}
                      fallbackUrl={prop.image_url}
                      alt={prop.title}
                      className="w-full h-full"
                      imgClassName="w-full h-full object-cover"
                    />
                  ) : (
                    <div className="w-full h-full bg-muted" />
                  )}
                  <span className="absolute top-2 left-2 bg-card/95 backdrop-blur-sm px-2 py-1 rounded text-micro font-bold text-foreground/80">
                    {typeof prop.match_score === "number" ? `${Math.round(prop.match_score)}% match` : "Selecionado"}
                  </span>
                  <button
                    type="button"
                    aria-label={favoriteIds.has(prop.id) ? "Remover dos favoritos" : "Salvar nos favoritos"}
                    onClick={() => void toggleFavorite(prop.id)}
                    disabled={toggleFavoriteMutation.isPending}
                    className="absolute top-2 right-2 w-7 h-7 bg-card/90 backdrop-blur-sm rounded-full flex items-center justify-center disabled:opacity-60"
                  >
                    <Heart
                      className={
                        favoriteIds.has(prop.id)
                          ? "w-3.5 h-3.5 text-status-ok fill-emerald-600"
                          : "w-3.5 h-3.5 text-muted-foreground/70"
                      }
                    />
                  </button>
                </div>
                <div className="p-3">
                  <p className="text-micro text-muted-foreground mb-1">
                    {prop.neighborhood} · {prop.city}
                  </p>
                  <h4 className="text-sm font-bold text-foreground mb-1 truncate">{buildPropertyTitle(prop)}</h4>
                  <p className="text-micro text-status-ok font-semibold mb-2 truncate">{matchLabel(prop.match_score)} · {matchReason(prop)}</p>
                  {(() => {
                    const valor = describeListingPrice(prop.price, prop.listing_type);
                    return (
                      <p className="mb-3 flex items-baseline gap-1.5 text-sm font-bold text-foreground">
                        {valor.label && (
                          <span className="text-caption font-bold uppercase tracking-wide text-muted-foreground">{valor.label}</span>
                        )}
                        <span>{valor.amount}</span>
                        {valor.period && <span className="text-micro font-normal text-muted-foreground">{valor.period}</span>}
                      </p>
                    );
                  })()}
                  <div className="flex items-center gap-3 text-caption text-muted-foreground font-medium">
                    <span className="flex items-center gap-1">🛏 {prop.bedrooms ?? 0}</span>
                    <span className="flex items-center gap-1">🚿 {prop.bathrooms ?? 0}</span>
                    <span className="flex items-center gap-1">🚗 {prop.parking_spaces ?? 0}</span>
                  </div>
                  <Link
                    to={`/portal/imoveis/${prop.id}`}
                    className="mt-3 inline-flex min-h-10 w-full items-center justify-center rounded-xl border border-border bg-card px-3 text-xs font-semibold text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                  >
                    Ver detalhes
                  </Link>
                </div>
              </div>
            ))}
            </div>
          )}
        </section>
      )}
    </PortalPage>
  );
}
