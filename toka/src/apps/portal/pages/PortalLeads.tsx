import { useMemo } from "react";
import { Link } from "react-router";
import { CalendarDays, ChevronRight, Heart, Home, MessageSquareText } from "lucide-react";
import { usePortalOverview } from "../hooks/usePortalOverview";
import { usePortalDeals } from "../context/PortalDealsContext";
import { PortalEmpty, PortalError, PortalLoading } from "../components/PortalState";
import { AgencyIdentity } from "../components/AgencyIdentity";
import { PortalPage } from "../shell/PortalPage";
import { PortalPageHeader } from "../shell/PortalPageHeader";
import { Button } from "../../../shared/ui/button";
import {
  dealForLead,
  effectiveLeadStatus,
  leadStatusTone,
  leadLocationLabel,
  leadPropertyLabel,
  leadRelationDate,
  leadStatusLabel,
  roleLabelForLead,
  relationshipDateLabel,
} from "../lib/portalRelationship";

export function PortalLeads() {
  const overview = usePortalOverview(true);
  const { deals } = usePortalDeals();
  const leads = useMemo(() => overview.data ?? [], [overview.data]);

  if (overview.isPending) return <PortalLoading rows={3} />;
  if (overview.isError) {
    return (
      <PortalPage className="px-5">
        <PortalError onRetry={() => overview.refetch()} />
      </PortalPage>
    );
  }

  return (
    <PortalPage className="overflow-x-hidden">
      <PortalPageHeader title="Meus atendimentos" subtitle="Cada conversa, imóvel e próxima ação em um só lugar." />

      <div className="space-y-3 px-5 pt-4">
        {leads.length === 0 ? (
          <PortalEmpty
            icon={Heart}
            title="Nenhum atendimento ainda"
            description="Quando uma imobiliária iniciar um atendimento com você, ele aparece aqui com o corretor, o imóvel e a etapa."
            action={
              <Button asChild>
                <Link to="/portal/buscar">Explorar imóveis</Link>
              </Button>
            }
          />
        ) : (
          leads.map((lead) => {
            const deal = dealForLead(lead, deals);
            const property = lead.recommended_property;
            const image = property?.media_images?.[0] || property?.thumbnail_url;
            return (
              <article key={lead.id} className="rounded-2xl border border-border bg-card p-4 shadow-sm">
                <div className="flex items-start gap-3">
                  <div className="flex size-16 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-muted">
                    {image ? (
                      <img src={image} alt={property?.title || "Imóvel do atendimento"} className="size-full object-cover" />
                    ) : (
                      <Home className="size-5 text-muted-foreground" aria-hidden="true" />
                    )}
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div className="min-w-0">
                        <h2 className="truncate text-sm font-bold text-foreground">{leadPropertyLabel(lead)}</h2>
                        <p className="truncate text-xs text-muted-foreground">
                          {leadLocationLabel(lead) || "Atendimento imobiliário"}
                        </p>
                      </div>
                      <span className={`shrink-0 rounded-full border px-2 py-0.5 text-[10px] font-bold ${
                        leadStatusTone(effectiveLeadStatus(lead)) === "critical"
                          ? "border-status-critical/30 bg-status-critical-soft text-status-critical"
                          : leadStatusTone(effectiveLeadStatus(lead)) === "positive"
                            ? "border-status-ok/30 bg-status-ok-soft text-status-ok"
                            : "border-border bg-muted text-muted-foreground"
                      }`}>
                        {leadStatusLabel(effectiveLeadStatus(lead))}
                      </span>
                    </div>
                    <div className="mt-2 flex flex-wrap items-center gap-2">
                      <AgencyIdentity tenantId={lead.tenant_id} compact />
                      <span className="text-[11px] text-muted-foreground">{roleLabelForLead(lead)}</span>
                      {leadRelationDate(lead) ? (
                        <span className="text-[11px] text-muted-foreground">· {relationshipDateLabel(leadRelationDate(lead))}</span>
                      ) : null}
                    </div>
                  </div>
                </div>

                <div className="mt-3 flex flex-wrap items-center justify-between gap-2 border-t border-border pt-3">
                  <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                    {lead.broker_name ? (
                      <span className="inline-flex items-center gap-1"><MessageSquareText className="size-3.5" aria-hidden="true" /> {lead.broker_name}</span>
                    ) : null}
                    {lead.visit_request?.status ? (
                      <span className="inline-flex items-center gap-1 text-status-ok"><CalendarDays className="size-3.5" aria-hidden="true" /> Visita {lead.visit_request.status.toLowerCase()}</span>
                    ) : null}
                    {deal ? <span className="text-primary-700">Negócio em andamento</span> : null}
                  </div>
                  <Button asChild variant="outline" className="h-9 rounded-xl px-3">
                    <Link to={deal ? `/portal/deals/${deal.id}` : `/portal/atendimentos/${lead.id}`}>
                      {deal ? "Abrir negócio" : "Abrir atendimento"}
                      <ChevronRight className="size-4" aria-hidden="true" />
                    </Link>
                  </Button>
                </div>
              </article>
            );
          })
        )}
        {leads.length > 0 ? (
          <Button asChild variant="outline" className="w-full">
            <Link to="/portal/buscar">Explorar mais imóveis</Link>
          </Button>
        ) : null}
      </div>
    </PortalPage>
  );
}
