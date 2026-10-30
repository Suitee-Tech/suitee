import { Building2, Home, Key, ShoppingBag, type LucideIcon } from "lucide-react";
import { apiLeadStatusToLeadStatus } from "./mappers";

export interface LeadTypeDetails {
  label: string;
  icon: LucideIcon;
  color: string;
}

const FALLBACK: LeadTypeDetails = {
  label: "Geral",
  icon: Home,
  color: "bg-muted text-foreground border-border",
};

const LEAD_TYPE_DETAILS: Record<string, LeadTypeDetails> = {
  captacao: { label: "Captação", icon: Building2, color: "bg-status-ok-soft text-status-ok border-status-ok/20" },
  compra_venda: {
    label: "Compra e Venda",
    icon: ShoppingBag,
    color: "bg-stage-proposal/10 text-stage-proposal border-stage-proposal/20",
  },
  locacao: { label: "Locação", icon: Key, color: "bg-stage-contract/10 text-stage-contract border-stage-contract/20" },
};

export function getLeadTypeDetails(type?: string | null): LeadTypeDetails {
  if (!type) return FALLBACK;
  return LEAD_TYPE_DETAILS[type] ?? FALLBACK;
}

export function leadStatusLabel(apiStatus?: string | null): string {
  if (!apiStatus) return "—";
  return apiLeadStatusToLeadStatus[apiStatus] ?? apiStatus;
}
