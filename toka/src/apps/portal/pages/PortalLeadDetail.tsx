import { useMemo, useState } from "react";
import { Link, useParams } from "react-router";
import { format } from "date-fns";
import { CalendarDays, CheckCircle2, Home, Loader2, MessageSquareText, Sparkles } from "lucide-react";
import { toast } from "sonner";
import { isAxiosError } from "axios";
import { Button } from "../../../shared/ui/button";
import { AppointmentPickerDialog, type AppointmentPickerValues } from "../components/AppointmentPickerDialog";
import { AgencyIdentity } from "../components/AgencyIdentity";
import { PortalSiteListingLink } from "../components/PortalSiteListingLink";
import { PortalVisitActions } from "../components/PortalVisitActions";
import { PortalError, PortalLoading } from "../components/PortalState";
import { usePortalContact } from "../hooks/usePortalContact";
import { usePortalOverview } from "../hooks/usePortalOverview";
import { usePortalDeals } from "../context/PortalDealsContext";
import { portalService } from "../services/portal.service";
import { portalCopy, resolvePortalPersona } from "../lib/portalPersona";
import {
  dealForLead,
  effectiveLeadStatus,
  leadJourney,
  leadLocationLabel,
  leadPropertyLabel,
  leadStatusLabel,
  leadStatusTone,
  nextLeadAction,
  roleLabelForLead,
} from "../lib/portalRelationship";
import { describeListingPrice, isRentListing } from "../../../shared/utils/listingPrice";
import { isVisitClosed, visitPickerDefaults } from "./portalVisitPresentation";
import { PortalPage } from "../shell/PortalPage";
import { PortalPageHeader } from "../shell/PortalPageHeader";
import { supportErrorMessage } from "../../../shared/lib/errorDiagnostics";

export function PortalLeadDetail() {
  const { id = "" } = useParams();
  const overview = usePortalOverview(true);
  const { deals, setActiveDealId } = usePortalDeals();
  const lead = useMemo(
    () => overview.data?.find((item) => item.id === id) ?? null,
    [id, overview.data],
  );
  const deal = lead ? dealForLead(lead, deals) : null;
  const persona = useMemo(
    () => resolvePortalPersona({ role: lead?.role ?? null, dealType: null }),
    [lead?.role],
  );
  const copy = useMemo(() => portalCopy(persona), [persona]);
  const { openContact, opening } = usePortalContact();
  const [visitOpen, setVisitOpen] = useState(false);
  const [cancelLoading, setCancelLoading] = useState(false);

  if (overview.isPending) return <PortalLoading rows={4} />;
  if (overview.isError) {
    return (
      <PortalPage className="px-5">
        <PortalError onRetry={() => overview.refetch()} />
      </PortalPage>
    );
  }
  if (!lead) {
    return (
      <PortalPage className="px-5">
        <PortalPageHeader title="Atendimento" />
        <div className="mt-6 rounded-2xl border border-border bg-card p-6 text-center">
          <p className="text-sm text-muted-foreground">Este atendimento não está mais disponível.</p>
          <Button asChild className="mt-4"><Link to="/portal/leads">Ver meus atendimentos</Link></Button>
        </div>
      </PortalPage>
    );
  }

  const action = nextLeadAction(lead);
  const journey = leadJourney(effectiveLeadStatus(lead));
  const recommendation = lead.recommended_property;
  const price = recommendation
    ? describeListingPrice(
        isRentListing(recommendation.listing_type) ? recommendation.rent_price : recommendation.sale_price,
        recommendation.listing_type,
      )
    : null;
  const visit = lead.visit_request;

  const requestVisit = async (values: AppointmentPickerValues) => {
    const request = {
      date: format(values.date, "yyyy-MM-dd"),
      time: values.startTime,
      end_time: values.endTime,
      note: values.note?.trim(),
    };
    try {
      if (visit && !isVisitClosed(visit)) {
        await portalService.updateLeadVisit(lead.id, request);
      } else {
        await portalService.requestLeadVisit(lead.id, {
          property_id: lead.recommended_property_id,
          ...request,
        });
      }
      toast.success("Solicitação de visita enviada. A imobiliária fará a confirmação.");
      setVisitOpen(false);
      void overview.refetch();
    } catch (error: unknown) {
      if (isAxiosError<{ message?: string }>(error) && (error.response?.status ?? 500) < 500) {
        toast.error(error.response?.data?.message || "Não foi possível solicitar a visita.");
      } else {
        toast.error(supportErrorMessage(error, "Não foi possível solicitar a visita."));
      }
      throw error;
    }
  };

  const cancelVisit = async () => {
    setCancelLoading(true);
    try {
      await portalService.cancelLeadVisit(lead.id);
      toast.success("Visita cancelada.");
      void overview.refetch();
    } catch (error) {
      toast.error(supportErrorMessage(error, "Não foi possível cancelar a visita."));
    } finally {
      setCancelLoading(false);
    }
  };

  return (
    <PortalPage className="overflow-x-hidden">
      <PortalPageHeader
        title="Atendimento"
        subtitle={lead.broker_name ? `Com ${lead.broker_name}` : undefined}
        backTo="/portal/leads"
      />

      <div className="space-y-4 px-5 pt-4">
        <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <AgencyIdentity tenantId={lead.tenant_id} showSiteLink />
            <span className={`rounded-full border px-2.5 py-1 text-[11px] font-bold ${
              leadStatusTone(effectiveLeadStatus(lead)) === "positive"
                ? "border-status-ok/30 bg-status-ok-soft text-status-ok"
                : leadStatusTone(effectiveLeadStatus(lead)) === "warning"
                  ? "border-status-warning/30 bg-status-warning-soft text-status-warning"
                  : "border-border bg-muted text-muted-foreground"
            }`}>
              {leadStatusLabel(effectiveLeadStatus(lead))}
            </span>
          </div>
          <h1 className="mt-4 text-2xl font-bold tracking-tight text-foreground">{leadPropertyLabel(lead)}</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {leadLocationLabel(lead) || "Atendimento com a imobiliária"} · {roleLabelForLead(lead)}
          </p>
          <div className="mt-5 rounded-2xl border border-primary/20 bg-primary-50/50 p-4">
            <p className="text-caption font-bold uppercase tracking-wide text-primary-700">Próxima ação</p>
            <p className="mt-1 text-sm font-bold text-foreground">{action.title}</p>
            <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{action.description}</p>
          </div>
        </section>

        <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
          <h2 className="text-sm font-bold text-foreground">Etapas do atendimento</h2>
          <ol className="mt-4 grid grid-cols-5 gap-1" aria-label="Etapas do atendimento">
            {journey.map((step) => (
              <li key={step.key} className="text-center">
                <span
                  className={`mx-auto flex size-7 items-center justify-center rounded-full border text-[11px] font-bold ${
                    step.state === "done"
                      ? "border-primary bg-primary text-primary-foreground"
                      : step.state === "current"
                        ? "border-primary bg-primary-50 text-primary-700"
                        : "border-border bg-muted text-muted-foreground"
                  }`}
                  aria-current={step.state === "current" ? "step" : undefined}
                >
                  {step.state === "done" ? <CheckCircle2 className="size-4" aria-hidden="true" /> : null}
                </span>
                <span className={`mt-1 block text-[10px] font-semibold ${step.state === "todo" ? "text-muted-foreground" : "text-foreground"}`}>
                  {step.label}
                </span>
              </li>
            ))}
          </ol>
        </section>

        {recommendation ? (
          <section className="overflow-hidden rounded-3xl border border-border bg-card shadow-sm">
            {recommendation.media_images?.[0] || recommendation.thumbnail_url ? (
              <img
                src={recommendation.media_images?.[0] || recommendation.thumbnail_url}
                alt={recommendation.title || "Imóvel recomendado"}
                className="h-48 w-full object-cover"
              />
            ) : null}
            <div className="p-5">
              <p className="flex items-center gap-1.5 text-caption font-bold uppercase tracking-wide text-primary-700">
                <Sparkles className="size-3.5" aria-hidden="true" /> Sugestão do seu corretor
              </p>
              <h2 className="mt-2 text-lg font-bold text-foreground">{recommendation.title}</h2>
              <p className="text-sm text-muted-foreground">
                {[recommendation.neighborhood, recommendation.city, recommendation.state].filter(Boolean).join(", ")}
              </p>
              {price ? (
                <p className="mt-2 text-xl font-bold text-foreground">
                  {price.label ? <span className="mr-1 text-caption font-bold uppercase tracking-wide text-muted-foreground">{price.label}</span> : null}
                  {price.amount}{price.period ? <span className="text-xs font-normal text-muted-foreground">{price.period}</span> : null}
                </p>
              ) : null}
              <div className="mt-4 flex flex-wrap gap-2">
                <Button asChild><Link to={`/portal/imoveis/${recommendation.id}`} state={{ property: recommendation, tenantId: lead.tenant_id, leadId: lead.id }}>Ver imóvel</Link></Button>
                <PortalSiteListingLink propertyId={recommendation.id} tenantId={lead.tenant_id} />
              </div>
            </div>
          </section>
        ) : (
          <section className="rounded-3xl border border-dashed border-border bg-card p-5">
            <h2 className="text-sm font-bold text-foreground">Imóveis do atendimento</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              Seu corretor ainda não separou um imóvel específico. Fale com ele para receber opções.
            </p>
            <Button asChild variant="outline" className="mt-4"><Link to="/portal/buscar">Explorar imóveis</Link></Button>
          </section>
        )}

        <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
          <h2 className="flex items-center gap-2 text-sm font-bold text-foreground">
            <CalendarDays className="size-4 text-primary" aria-hidden="true" /> Sua visita
          </h2>
          {visit?.status ? (
            <div className="mt-3">
              <PortalVisitActions
                lead={lead}
                dealId={deal?.id}
                loadCalendar={() => portalService.downloadLeadVisitCalendar(lead.id)}
                cancelLoading={cancelLoading}
                onReschedule={() => setVisitOpen(true)}
                onCancel={cancelVisit}
              />
            </div>
          ) : (
            <div className="mt-3">
              <p className="text-sm text-muted-foreground">{copy.visitsEmptyBody}</p>
              {copy.propertyVisitCta ? (
                <Button className="mt-3" onClick={() => setVisitOpen(true)}>
                  <CalendarDays className="size-4" aria-hidden="true" /> {copy.propertyVisitCta}
                </Button>
              ) : null}
            </div>
          )}
        </section>

        <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
          <h2 className="text-sm font-bold text-foreground">Fale com a imobiliária</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {lead.broker_name ? `Seu corretor é ${lead.broker_name}.` : "Tire dúvidas e acompanhe o atendimento pelo chat."}
          </p>
          <div className="mt-3 flex flex-wrap gap-2">
            <Button onClick={() => void openContact({ leadId: lead.id })} disabled={opening}>
              {opening ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <MessageSquareText className="size-4" aria-hidden="true" />}
              Falar com meu corretor
            </Button>
            <Button asChild variant="outline"><Link to="/portal/imoveis">Imóveis do atendimento</Link></Button>
            {deal ? (
              <Button asChild variant="outline">
                <Link to={`/portal/deals/${deal.id}`} onClick={() => setActiveDealId(deal.id)}>
                  <Home className="size-4" aria-hidden="true" /> Ver negócio
                </Link>
              </Button>
            ) : null}
          </div>
        </section>

        {lead.matched_properties.length > 0 ? (
          <section className="space-y-2">
            <h2 className="text-sm font-bold text-foreground">Também combinam com você</h2>
            {lead.matched_properties.slice(0, 4).map(({ property, score }) => (
              <Link
                key={property.id}
                to={`/portal/imoveis/${property.id}`}
                state={{ property, tenantId: lead.tenant_id, leadId: lead.id }}
                className="flex items-center justify-between gap-3 rounded-2xl border border-border bg-card p-3 hover:bg-muted/50"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-semibold text-foreground">{property.title}</p>
                  <p className="truncate text-xs text-muted-foreground">
                    {[property.neighborhood, property.city].filter(Boolean).join(", ")}
                  </p>
                </div>
                {typeof score === "number" ? <span className="shrink-0 text-xs font-bold text-status-ok">{Math.round(score)}%</span> : null}
              </Link>
            ))}
          </section>
        ) : null}
      </div>

      <AppointmentPickerDialog
        open={visitOpen}
        onOpenChange={setVisitOpen}
        onSubmit={requestVisit}
        propertyLabel={recommendation?.title}
        mode={visit && !isVisitClosed(visit) ? "reschedule" : "create"}
        defaultValues={visitPickerDefaults(visit)}
      />
    </PortalPage>
  );
}
