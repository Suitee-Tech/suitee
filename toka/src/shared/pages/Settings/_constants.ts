import { Building2, Paintbrush, ShieldCheck, Bell, Link2, Type, Settings2, DollarSign } from "lucide-react";
import type { NotificationChannel, WebhookEndpoint } from "../../services/settings.service";

export type SettingsTab =
  | "profile"
  | "company"
  | "appearance"
  | "permissions"
  | "notifications"
  | "integrations"
  | "commission"
  | "general";

export interface Integration {
  id: string;
  name: string;
  description: string;
  status: "connected" | "disconnected" | "error";
  icon: string;
  category: string;
  lastSync: string | null;
  syncFrequency: string;
  apiKey?: string;
}

export interface GeneralSettings {
  timezone: string;
  dateFormat: string;
  currency: string;
  language: string;
  sessionTimeout: number;
  autoSave: boolean;
  compactTables: boolean;
  focusMode: boolean;
}

export const TABS: { id: SettingsTab; label: string; icon: any; badge?: string; permission?: string }[] = [
  { id: "profile", label: "Meu Perfil", icon: Type },
  { id: "company", label: "Empresa & Equipe", icon: Building2, permission: "company:manage_members" },
  { id: "appearance", label: "Aparência (marca própria)", icon: Paintbrush, permission: "settings:manage_appearance" },
  { id: "permissions", label: "Permissões & Acessos", icon: ShieldCheck, permission: "settings:manage_permissions" },
  { id: "notifications", label: "Notificações", icon: Bell, permission: "settings:manage_notifications" },
  { id: "integrations", label: "Integrações & APIs", icon: Link2, permission: "settings:manage_integrations" },
  // Sem `settings:manage_commission` no catálogo: a API de regras de comissão é
  // gated por `finance:read`/`finance:write`
  // (toca-financial/internal/handlers/router.go:391-392). Antes a aba ficava
  // invisível para todo não-root, inclusive `financeiro` (que tem finance:write).
  { id: "commission", label: "Comissões", icon: DollarSign, permission: "finance:write" },
  { id: "general", label: "Geral & Sistema", icon: Settings2, permission: "settings:manage_general" },
];

export const COLOR_PRESETS = [
  { name: "suitee Azul", value: "#355872" },
  { name: "Indigo", value: "#6366f1" },
  { name: "Esmeralda", value: "#10b981" },
  { name: "Violeta", value: "#8b5cf6" },
  { name: "Rose", value: "#f43f5e" },
  { name: "Amber", value: "#f59e0b" },
  { name: "Sky", value: "#0ea5e9" },
  { name: "Slate", value: "#475569" },
];

export const ACCENT_PRESETS = [
  { name: "Green Flag", value: "#1C905B" },
  { name: "Dark Green", value: "#064E3B" },
  { name: "Medium Green", value: "#059669" },
  { name: "Teal AI", value: "#00BABC" },
  { name: "Mint AI", value: "#00E6C3" },
  { name: "Light Green", value: "#D1FAE5" },
];

export { FONT_OPTIONS, TENANT_FONTS, TENANT_ICON_STYLES } from "../../lib/tenantAppearance";

export const INITIAL_NOTIFICATIONS: NotificationChannel[] = [
  { id: "n1", label: "Novo lead recebido na esteira", email: true, whatsapp: true, inApp: true, category: "Leads" },
  { id: "n2", label: "Lead atribuído via roleta", email: true, whatsapp: true, inApp: true, category: "Leads" },
  { id: "n3", label: "Lead sem interação há 48h", email: true, whatsapp: false, inApp: true, category: "Leads" },
  { id: "n4", label: "Proposta aceita pelo proprietário", email: true, whatsapp: true, inApp: true, category: "Negócios" },
  { id: "n5", label: "Negócio movido para próxima etapa", email: false, whatsapp: false, inApp: true, category: "Negócios" },
  { id: "n6", label: "Contrato próximo do vencimento (30 dias)", email: true, whatsapp: true, inApp: true, category: "Contratos" },
  { id: "n7", label: "Reajuste de aluguel programado", email: true, whatsapp: false, inApp: true, category: "Contratos" },
  { id: "n8", label: "Boleto vencido / inadimplência", email: true, whatsapp: true, inApp: true, category: "Financeiro" },
  { id: "n9", label: "Repasse gerado ao proprietário", email: true, whatsapp: false, inApp: true, category: "Financeiro" },
  { id: "n10", label: "Visita agendada (1h antes)", email: false, whatsapp: true, inApp: true, category: "Agenda" },
  { id: "n11", label: "Cancelamento de visita pelo cliente", email: true, whatsapp: true, inApp: true, category: "Agenda" },
  { id: "n12", label: "Nova OS de manutenção criada", email: true, whatsapp: false, inApp: true, category: "Manutenção" },
  { id: "n13", label: "Relatório de fechamento semanal", email: true, whatsapp: false, inApp: false, category: "Relatórios" },
  { id: "n14", label: "Mensagem recebida do cliente no portal", email: false, whatsapp: true, inApp: true, category: "Comunicação" },
];

export const STATIC_INTEGRATIONS: Integration[] = [
  { id: "int-4", name: "RD Station", description: "Automação de marketing: envie contatos e oportunidades para fluxos.", status: "disconnected", icon: "mail", category: "Marketing", lastSync: null, syncFrequency: "tempo_real" },
  { id: "int-5", name: "Google Calendar", description: "Sincronize agendamentos e visitas com o Google Calendar.", status: "connected", icon: "calendar", category: "Produtividade", lastSync: "12/03/2026 15:00", syncFrequency: "tempo_real" },
  { id: "int-6", name: "WhatsApp Business API", description: "Envio de notificações e mensagens automáticas via WhatsApp.", status: "connected", icon: "phone", category: "Comunicação", lastSync: "12/03/2026 15:45", syncFrequency: "tempo_real" },
  { id: "int-7", name: "Conta Azul", description: "Integração contábil para emissão de notas e conciliação.", status: "disconnected", icon: "database", category: "Contabilidade", lastSync: null, syncFrequency: "24h" },
];

export const PORTAL_DEFINITIONS: { portal: "zap_imoveis" | "olx_grupo_zap" | "vivareal"; name: string; description: string; icon: string }[] = [
  { portal: "zap_imoveis", name: "ZAP Imóveis", description: "Publicação automática de anúncios e captura de leads via XML.", icon: "zap" },
  { portal: "olx_grupo_zap", name: "OLX / Grupo ZAP", description: "Distribuição de anúncios para OLX e submarcas.", icon: "globe" },
  { portal: "vivareal", name: "VivaReal", description: "Integração com o portal VivaReal para anúncios.", icon: "home" },
];

export const PORTAL_PROPERTY_STATUSES = [
  { value: "available", label: "Disponível" },
  { value: "reserved", label: "Reservado" },
  { value: "sold", label: "Vendido" },
  { value: "rented", label: "Alugado" },
];
export const PORTAL_PROPERTY_TYPES = [
  { value: "apartment", label: "Apartamento" },
  { value: "house", label: "Casa" },
  { value: "land", label: "Terreno" },
  { value: "commercial", label: "Comercial" },
];
export const PORTAL_LISTING_TYPES = [
  { value: "sale", label: "Venda" },
  { value: "rent", label: "Aluguel" },
];

export const INITIAL_WEBHOOKS: WebhookEndpoint[] = [
  { id: "wh-1", url: "https://api.suaimobiliaria.com/webhooks/leads", events: ["lead.created", "lead.updated"], active: true, lastTriggered: "12/03/2026 14:22", secret: "whsec_abc123...xyz" },
  { id: "wh-2", url: "https://n8n.suaimobiliaria.com/webhook/deals", events: ["deal.stage_changed", "deal.won"], active: true, lastTriggered: "11/03/2026 18:45", secret: "whsec_def456...uvw" },
];

export const WEBHOOK_EVENT_OPTIONS = [
  "lead.created",
  "lead.updated",
  "lead.converted",
  "lead.lost",
  "deal.created",
  "deal.stage_changed",
  "deal.won",
  "deal.lost",
  "contract.created",
  "contract.expired",
  "transaction.created",
  "transaction.paid",
  "transaction.overdue",
  "property.created",
  "property.updated",
];
