import type { DealDTO } from "../../../shared/services/deal.service";
import type { PortalOverviewLead } from "../services/portal.service";

export interface LeadJourneyStep {
  key: string;
  label: string;
  state: "done" | "current" | "todo";
}

const LEAD_STATUS_RANK: Record<string, number> = {
  NEW: 1,
  PENDING_ACCEPTANCE: 1,
  CONTACTED: 2,
  QUALIFIED: 3,
  VISIT_SCHEDULED: 4,
  PROPOSAL: 5,
  NEGOTIATION: 6,
  CONVERTED: 7,
  LOST: 7,
  DISQUALIFIED: 7,
};

const LEAD_STATUS_LABELS: Record<string, string> = {
  NEW: "Novo contato",
  PENDING_ACCEPTANCE: "Aguardando aceite",
  CONTACTED: "Em atendimento",
  QUALIFIED: "Perfil qualificado",
  VISIT_SCHEDULED: "Visita agendada",
  PROPOSAL: "Proposta em andamento",
  NEGOTIATION: "Em negociação",
  CONVERTED: "Convertido em negócio",
  LOST: "Atendimento encerrado",
  DISQUALIFIED: "Sem potencial no momento",
};

export function leadStatusLabel(status?: string | null): string {
  const key = String(status ?? "").trim().toUpperCase();
  return LEAD_STATUS_LABELS[key] ?? (key ? key.replace(/_/g, " ").toLowerCase() : "Em atendimento");
}

export function leadStatusTone(status?: string | null): "neutral" | "positive" | "warning" | "critical" {
  const key = String(status ?? "").trim().toUpperCase();
  if (key === "CONVERTED" || key === "NEGOTIATION") return "positive";
  if (key === "LOST" || key === "DISQUALIFIED") return "critical";
  if (key === "PROPOSAL" || key === "VISIT_SCHEDULED" || key === "QUALIFIED") return "warning";
  return "neutral";
}

export function leadJourney(status?: string | null): LeadJourneyStep[] {
  const rank = LEAD_STATUS_RANK[String(status ?? "").trim().toUpperCase()] ?? 1;
  const steps = [
    { key: "contact", label: "Contato" },
    { key: "service", label: "Atendimento" },
    { key: "visit", label: "Visita" },
    { key: "proposal", label: "Proposta" },
    { key: "closed", label: "Negócio" },
  ];
  const thresholds = [1, 2, 4, 5, 7];
  return steps.map((step, index) => ({
    ...step,
    state: rank > thresholds[index] ? "done" : rank === thresholds[index] ? "current" : "todo",
  }));
}

/**
 * O CRM pode manter o lead em CONTACTED enquanto já existe uma visita
 * confirmada. Para o cliente, a etapa real é a visita; exibir "Em atendimento"
 * com a jornada em "Contato" ao lado de "Visita confirmada" é contraditório.
 */
export function effectiveLeadStatus(lead: PortalOverviewLead): string {
  const visit = String(lead.visit_request?.status ?? "").trim().toLowerCase();
  if (visit && !["cancelado", "cancelada", "cancelled", "closed", "encerrado"].includes(visit)) {
    return "VISIT_SCHEDULED";
  }
  return String(lead.status ?? "");
}

export function nextLeadAction(lead: PortalOverviewLead): { title: string; description: string } {
  const status = String(lead.status ?? "").toUpperCase();
  if (lead.visit_request?.status) {
    return {
      title: "Acompanhe sua visita",
      description: "Confira o horário confirmado e fale com o corretor se precisar remarcar.",
    };
  }
  if (status === "NEW" || status === "PENDING_ACCEPTANCE") {
    return {
      title: "Seu contato foi recebido",
      description: "A imobiliária vai assumir o atendimento e falar com você pelo chat.",
    };
  }
  if (status === "PROPOSAL" || status === "NEGOTIATION") {
    return {
      title: "Acompanhe a proposta",
      description: "Fale com o corretor para tirar dúvidas e acompanhar os próximos passos.",
    };
  }
  if (status === "LOST" || status === "DISQUALIFIED") {
    return {
      title: "Atendimento encerrado",
      description: "Você ainda pode explorar outros imóveis ou iniciar uma nova conversa.",
    };
  }
  return {
    title: "Fale com seu corretor",
    description: "Tire dúvidas sobre as opções separadas para você e combine a próxima visita.",
  };
}

export function leadPropertyLabel(lead: PortalOverviewLead): string {
  const property = lead.recommended_property;
  if (property?.title?.trim()) return property.title.trim();
  const interest = lead.interest as { preferred_location?: string } | undefined;
  if (interest?.preferred_location?.trim()) return interest.preferred_location.trim();
  return "Atendimento imobiliário";
}

export function leadLocationLabel(lead: PortalOverviewLead): string {
  const property = lead.recommended_property;
  if (!property) return "";
  return [property.neighborhood, property.city, property.state].filter(Boolean).join(", ");
}

export function leadRelationDate(lead: PortalOverviewLead): string | undefined {
  return lead.last_interaction || lead.updated_at || lead.created_at;
}

export function leadForDeal(deal: DealDTO, leads: PortalOverviewLead[]): PortalOverviewLead | null {
  return (
    leads.find((lead) => lead.id === deal.lead_id) ??
    leads.find((lead) => lead.id === (deal as { customer_id?: string }).customer_id) ??
    null
  );
}

export function dealForLead(lead: PortalOverviewLead, deals: DealDTO[]): DealDTO | null {
  return (
    deals.find((deal) => deal.lead_id === lead.id) ??
    deals.find((deal) => (deal as { customer_id?: string }).customer_id === lead.id) ??
    null
  );
}

export function relationshipDateLabel(value?: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleDateString("pt-BR", { day: "2-digit", month: "short" });
}

export function roleLabelForLead(lead: PortalOverviewLead): string {
  const role = String(lead.role ?? "").trim().toLowerCase();
  if (role === "owner") return "Proprietário(a)";
  if (role === "tenant") return "Locatário(a)";
  if (role === "buyer") return "Comprador(a)";
  if (role === "guarantor") return "Fiador(a)";
  return "Cliente";
}
