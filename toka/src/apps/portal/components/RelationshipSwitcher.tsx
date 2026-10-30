import { useMemo, useState } from "react";
import { Check, ChevronDown, Home, MessagesSquare } from "lucide-react";
import type { DealDTO } from "../../../shared/services/deal.service";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "../../../shared/ui/dialog";
import { Badge } from "../../../shared/ui/badge";
import { usePortalDealProperty } from "../hooks/useDealResources";
import type { PortalOverviewLead } from "../services/portal.service";
import {
  dealForLead,
  effectiveLeadStatus,
  leadJourney,
  leadLocationLabel,
  leadPropertyLabel,
  leadStatusLabel,
  leadStatusTone,
} from "../lib/portalRelationship";
import { AgencyIdentity } from "./AgencyIdentity";
import { portalRoleLabel } from "../lib/portalPersona";

const DEAL_STAGE_LABELS: Record<string, string> = {
  NEGOTIATION: "Em negociação",
  PROPOSAL: "Proposta",
  ANALYSIS: "Análise",
  CONTRACT: "Contrato",
  CONVERTED: "Concluído",
  CANCELLED: "Encerrado",
};

function toneClass(tone: ReturnType<typeof leadStatusTone>): string {
  if (tone === "positive") return "border-status-ok/30 bg-status-ok-soft text-status-ok";
  if (tone === "warning") return "border-status-warning/30 bg-status-warning-soft text-status-warning";
  if (tone === "critical") return "border-status-critical/30 bg-status-critical-soft text-status-critical";
  return "border-border bg-muted text-muted-foreground";
}

function DealRow({
  deal,
  selected,
  onSelect,
}: {
  deal: DealDTO;
  selected: boolean;
  onSelect: () => void;
}) {
  const property = usePortalDealProperty(deal.id);
  const title = property.data?.title || `Negócio #${deal.display_id || deal.id.slice(0, 6)}`;
  const location = property.data
    ? [property.data.neighborhood, property.data.city].filter(Boolean).join(", ")
    : "";
  const stage = DEAL_STAGE_LABELS[deal.status] || deal.status;

  return (
    <button
      type="button"
      onClick={onSelect}
      className={`w-full rounded-2xl border px-4 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary ${
        selected ? "border-primary/40 bg-primary-50/50" : "border-border bg-card hover:bg-muted/50"
      }`}
      aria-pressed={selected}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-bold text-foreground">{title}</p>
          {location ? <p className="mt-0.5 truncate text-xs text-muted-foreground">{location}</p> : null}
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <AgencyIdentity tenantId={deal.tenant_id} compact />
            <Badge variant="outline" className="text-[10px]">{stage}</Badge>
            <span className="text-[11px] text-muted-foreground">{portalRoleLabel(deal.my_role)}</span>
          </div>
        </div>
        {selected ? <Check className="mt-1 size-4 shrink-0 text-primary" aria-hidden="true" /> : null}
      </div>
    </button>
  );
}

function LeadRow({
  lead,
  selected,
  onSelect,
}: {
  lead: PortalOverviewLead;
  selected: boolean;
  onSelect: () => void;
}) {
  const journey = leadJourney(effectiveLeadStatus(lead));
  const current = journey.find((step) => step.state === "current");
  return (
    <button
      type="button"
      onClick={onSelect}
      className={`w-full rounded-2xl border px-4 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary ${
        selected ? "border-primary/40 bg-primary-50/50" : "border-border bg-card hover:bg-muted/50"
      }`}
      aria-pressed={selected}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-bold text-foreground">{leadPropertyLabel(lead)}</p>
          <p className="mt-0.5 truncate text-xs text-muted-foreground">
            {leadLocationLabel(lead) || "Atendimento com a imobiliária"}
          </p>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <AgencyIdentity tenantId={lead.tenant_id} compact />
            <span className={`rounded-full border px-2 py-0.5 text-[10px] font-bold ${toneClass(leadStatusTone(effectiveLeadStatus(lead)))}`}>
              {leadStatusLabel(effectiveLeadStatus(lead))}
            </span>
            {current ? (
              <span className="text-[11px] text-muted-foreground">{current.label}</span>
            ) : null}
          </div>
        </div>
        {selected ? <Check className="mt-1 size-4 shrink-0 text-primary" aria-hidden="true" /> : null}
      </div>
    </button>
  );
}

interface RelationshipSwitcherProps {
  deals: DealDTO[];
  leads: PortalOverviewLead[];
  activeDealId?: string | null;
  activeLeadId?: string | null;
  onSelectDeal: (id: string) => void;
  onSelectLead: (id: string) => void;
  title?: string;
}

/**
 * Seletor único de relações: negócios e atendimentos, com imobiliária,
 * imóvel, etapa e papel. Substitui o antigo seletor que mostrava apenas
 * "Transação #..." e não dava acesso aos leads quando havia um negócio ativo.
 */
export function RelationshipSwitcher({
  deals,
  leads,
  activeDealId,
  activeLeadId,
  onSelectDeal,
  onSelectLead,
  title = "Minhas relações",
}: RelationshipSwitcherProps) {
  const [open, setOpen] = useState(false);
  const leadIdsWithDeal = useMemo(
    () => new Set(leads.filter((lead) => dealForLead(lead, deals)).map((lead) => lead.id)),
    [deals, leads],
  );
  const standaloneLeads = useMemo(
    () => leads.filter((lead) => !leadIdsWithDeal.has(lead.id)),
    [leadIdsWithDeal, leads],
  );
  const activeDeal = deals.find((deal) => deal.id === activeDealId);
  const activeLead = leads.find((lead) => lead.id === activeLeadId);
  const count = deals.length + standaloneLeads.length;
  const label = activeDeal
    ? `Negócio · ${DEAL_STAGE_LABELS[activeDeal.status] || activeDeal.status}`
    : activeLead
      ? `Atendimento · ${leadStatusLabel(effectiveLeadStatus(activeLead))}`
      : count > 0
        ? `${count} ${count === 1 ? "relação" : "relações"}`
        : "Minhas relações";

  if (count === 0) return null;

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="inline-flex min-h-10 max-w-full items-center gap-2 rounded-full border border-border bg-card px-4 text-sm font-semibold text-foreground shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
        aria-label={`${title}: ${label}`}
      >
        <MessagesSquare className="size-4 shrink-0 text-primary" aria-hidden="true" />
        <span className="truncate">{label}</span>
        <ChevronDown className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
      </button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
          </DialogHeader>
          <div className="space-y-6 py-2">
            {deals.length > 0 ? (
              <section>
                <h3 className="mb-2 text-caption font-bold uppercase tracking-wide text-muted-foreground">
                  Negócios em andamento
                </h3>
                <div className="space-y-2">
                  {deals.map((deal) => (
                    <DealRow
                      key={deal.id}
                      deal={deal}
                      selected={deal.id === activeDealId}
                      onSelect={() => {
                        onSelectDeal(deal.id);
                        setOpen(false);
                      }}
                    />
                  ))}
                </div>
              </section>
            ) : null}
            {standaloneLeads.length > 0 ? (
              <section>
                <h3 className="mb-2 text-caption font-bold uppercase tracking-wide text-muted-foreground">
                  Atendimentos
                </h3>
                <div className="space-y-2">
                  {standaloneLeads.map((lead) => (
                    <LeadRow
                      key={lead.id}
                      lead={lead}
                      selected={lead.id === activeLeadId}
                      onSelect={() => {
                        onSelectLead(lead.id);
                        setOpen(false);
                      }}
                    />
                  ))}
                </div>
              </section>
            ) : null}
            {count === 0 ? (
              <p className="flex items-center gap-2 text-sm text-muted-foreground">
                <Home className="size-4" aria-hidden="true" /> Nenhuma relação ativa no momento.
              </p>
            ) : null}
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
