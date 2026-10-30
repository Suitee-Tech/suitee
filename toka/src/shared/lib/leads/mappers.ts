import type { Lead, LeadSource, LeadStatus, LeadType } from "../../data";
import type { ApiLead } from "../../services/lead.service";
import { getMediaUrl } from "../../media";

export const apiLeadStatusToLeadStatus: Record<string, LeadStatus> = {
  NEW: "Novo",
  PENDING_ACCEPTANCE: "Em Atendimento",
  CONTACTED: "Em Atendimento",
  QUALIFIED: "Qualificado",
  VISIT_SCHEDULED: "Visita Agendada",
  PROPOSAL: "Proposta Enviada",
  NEGOTIATION: "Em Negociação",
  CONVERTED: "Convertido",
  LOST: "Perdido",
};

export const leadStatusToApiLeadStatus: Record<LeadStatus, string> = {
  Novo: "NEW",
  "Em Atendimento": "CONTACTED",
  Qualificado: "QUALIFIED",
  "Visita Agendada": "VISIT_SCHEDULED",
  "Em Negociação": "NEGOTIATION",
  "Proposta Enviada": "PROPOSAL",
  Convertido: "CONVERTED",
  Perdido: "LOST",
};

export const roleToLeadType: Record<string, LeadType> = {
  owner: "captacao",
  buyer: "compra_venda",
  tenant: "locacao",
};

export const apiLeadSourceToLeadSource: Record<string, LeadSource> = {
  WEBSITE: "Site",
  REFERRAL: "Indicacao",
  SOCIAL: "Redes Sociais",
  MANUAL: "Manual",
  CAMPAIGN: "Campanha",
  OTHER: "Outros",
};

export const leadSourceToApiLeadSource: Record<LeadSource, string> = {
  Site: "WEBSITE",
  Indicacao: "REFERRAL",
  "Redes Sociais": "SOCIAL",
  Manual: "MANUAL",
  Campanha: "CAMPAIGN",
  Outros: "OTHER",
};

export const mapApiLeadToLead = (apiLead: ApiLead): Lead => {
  return {
    id: apiLead.id,
    displayId: apiLead.display_id,
    name: apiLead.name,
    document: apiLead.document,
    email: apiLead.email,
    phone: apiLead.phone,
    property: apiLead.property_interest?.preferred_location || "Interesse Geral",
    budget: apiLead.property_interest?.price_range?.max || 0,
    status: apiLeadStatusToLeadStatus[apiLead.status] || "Novo",
    avatar: apiLead.avatar ? getMediaUrl(apiLead.avatar) : undefined,
    date: apiLead.created_at,
    agentId: apiLead.assigned_broker_id,
    role: apiLead.role,
    isClient: apiLead.is_client,
    accountId: apiLead.account_id,
    leadType: apiLead.role ? roleToLeadType[apiLead.role] : undefined,
    source: apiLead.source ? apiLeadSourceToLeadSource[apiLead.source] : undefined,
  };
};
