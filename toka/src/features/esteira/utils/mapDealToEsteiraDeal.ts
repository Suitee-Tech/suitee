import type { Lead, Property } from "../../../shared/data";
import type { DealStage, EsteiraDeal } from "../../../shared/deals/types";
import type { DealDTO } from "../../../shared/services/deal.service";

const dealStatusReverseMap: Record<string, DealStage> = {
  NEGOTIATION: "Em Negociação",
  PROPOSAL: "Proposta",
  ANALYSIS: "Análise",
  CONTRACT: "Contrato",
  CONVERTED: "Concluído",
  CANCELLED: "Perdido",
};

export const mapDealToEsteiraDeal = (deal: DealDTO, properties: Property[], leads: Lead[]): EsteiraDeal => {
  const property = properties.find((p) => p.id === deal.property_id);
  const client = leads.find((l) => l.id === deal.customer_id);

  const proposals: Array<{ id?: string; status?: string; counter_value?: number; created_at?: string }> =
    Array.isArray(deal.proposals) ? deal.proposals : [];
  const latestCounter = proposals
    .slice()
    .reverse()
    .find((p) => p.status === "COUNTER_OFFER");
  const hasCounterOffer = !!latestCounter;

  return {
    id: deal.id,
    displayId: deal.display_id,
    client: client?.name || "Cliente Desconhecido",
    client_email: client?.email || "",
    client_phone: client?.phone || "",
    client_doc: client?.document || "",
    clientId: client?.id,
    ownerId: deal.owner_id,
    property: property?.title || "Imóvel Desconhecido",
    propertyId: property?.id,
    type: deal.type === "SALE" ? "Venda" : "Locação",
    value: deal.value,
    commission: deal.commission,
    brokerParticipations: deal.broker_participations,
    stage: dealStatusReverseMap[deal.status] || "Em Negociação",
    // ATENÇÃO: esta data é para EXIBIÇÃO ("30/09/2026"). Não passe por
    // `new Date(deal.date)`: o parser do JavaScript não entende dd/mm/aaaa e
    // devolve Invalid Date (foi o que zerou os gráficos de Vendas & Locações em
    // /relatorios). Para agrupar/filtrar use `updated_at`/`created_at` (ISO).
    date: new Date(deal.updated_at).toLocaleDateString("pt-BR"),
    created_at: deal.created_at,
    updated_at: deal.updated_at,
    avatarUrl: client?.avatar || "",
    alerts: [],
    brokerId: deal.broker_id,
    hasCounterOffer,
    latestCounterValue: latestCounter?.counter_value,
    latestCounterProposalId: latestCounter?.id,
  };
};

