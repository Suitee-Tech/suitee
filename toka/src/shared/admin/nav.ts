import type { ComponentType } from "react";
import {
  AlertCircle,
  ArrowLeftRight,
  BadgeDollarSign,
  Banknote,
  Building2,
  CalendarDays,
  Crown,
  FileBarChart,
  FileCheck,
  FileSignature,
  FileText,
  Globe,
  HandCoins,
  Handshake,
  HelpCircle,
  History,
  Inbox,
  Key,
  Landmark,
  LayoutDashboard,
  MessageSquare,
  Receipt,
  Repeat,
  Settings,
  Shuffle,
  Stamp,
  Users,
  Wallet,
  Wrench,
} from "lucide-react";

type UserLike = { permission?: string; functional_role?: string } | null | undefined;

export type AdminNavItem = {
  icon: ComponentType<any>;
  label: string;
  to: string;
  permission?: string;
};

export type AdminNavGroup = {
  title: string;
  permission?: string;
  /** Identificador estável para grupos colapsáveis. */
  id?: string;
  collapsible?: boolean;
  defaultOpen?: boolean;
  items: AdminNavItem[];
};

export const adminNavGroups: AdminNavGroup[] = [
  {
    title: "",
    items: [{ icon: LayoutDashboard, label: "Painel", to: "/" }],
  },
  {
    title: "CRM & Relacionamento",
    permission: "module:crm",
    items: [
      { icon: Users, label: "Leads (CRM)", to: "/leads", permission: "leads:read" },
      { icon: Shuffle, label: "Roleta de Leads", to: "/roleta", permission: "leads:configure_roulette" },
      { icon: MessageSquare, label: "Mensagens", to: "/mensagens" },
      { icon: CalendarDays, label: "Agenda", to: "/agenda" },
    ],
  },
  {
    title: "Portfólio & Negócios",
    permission: "module:portfolio",
    items: [
      { icon: Building2, label: "Imóveis", to: "/imoveis", permission: "properties:read" },
      { icon: Globe, label: "Site de imóveis", to: "/site", permission: "sites:read" },
      { icon: FileCheck, label: "Esteira Digital", to: "/esteira", permission: "deals:read" },
      { icon: FileSignature, label: "Contratos", to: "/contratos", permission: "contracts:read" },
      { icon: FileText, label: "Modelos de Contrato", to: "/modelos-contrato", permission: "templates:read" },
      { icon: FileText, label: "Documentos", to: "/documentos", permission: "documents:read" },
    ],
  },
  {
    title: "Gestão de Locação",
    permission: "module:rentals",
    items: [
      { icon: Key, label: "Contratos de locação", to: "/locacao", permission: "rentals:read" },
      { icon: Wrench, label: "Manutenção", to: "/manutencao", permission: "rentals:inspect" },
      { icon: MessageSquare, label: "Chamados (Portal)", to: "/chamados", permission: "tickets:read" },
    ],
  },
  {
    title: "Financeiro",
    id: "finance",
    permission: "module:finance",
    collapsible: true,
    items: [
      { icon: Wallet, label: "Painel financeiro", to: "/financeiro", permission: "finance:read" },
      { icon: Inbox, label: "Lançamentos", to: "/financeiro/lancamentos", permission: "finance:read" },
      { icon: Receipt, label: "Recebimentos", to: "/financeiro/cobrancas", permission: "finance:read" },
      { icon: HandCoins, label: "Comissões", to: "/financeiro/comissoes", permission: "finance:read" },
      { icon: Banknote, label: "Adiantamentos", to: "/financeiro/adiantamentos", permission: "finance:read" },
      { icon: Stamp, label: "Impostos retidos", to: "/financeiro/impostos", permission: "finance:read" },
      { icon: ArrowLeftRight, label: "Repasses", to: "/financeiro/repasses", permission: "finance:read" },
      { icon: Landmark, label: "Contas & encargos", to: "/financeiro/contas", permission: "finance:read" },
      { icon: Repeat, label: "Conferência", to: "/financeiro/conciliacao", permission: "finance:read" },
      { icon: BadgeDollarSign, label: "Ordens de pagamento", to: "/financeiro/payouts", permission: "finance:read" },
      { icon: FileBarChart, label: "Fiscal", to: "/fiscal", permission: "fiscal:read" },
      { icon: AlertCircle, label: "Inadimplência", to: "/inadimplencia", permission: "charges:read" },
    ],
  },
  {
    title: "Inteligência & Expansão",
    permission: "module:intelligence",
    items: [
      { icon: FileBarChart, label: "Relatórios", to: "/relatorios", permission: "reports:read" },
      { icon: Handshake, label: "Parceiros & Franquias", to: "/parceiros" },
    ],
  },
  {
    title: "Administração",
    items: [
      { icon: Building2, label: "Empresas", to: "/empresas", permission: "company:read_members" },
      { icon: Crown, label: "Assinatura & Consumo", to: "/assinatura", permission: "module:admin" },
      { icon: History, label: "Trilha de auditoria", to: "/audit-trail", permission: "system:view_audit_trail" },
      { icon: Settings, label: "Configurações", to: "/configuracoes" },
      { icon: HelpCircle, label: "Ajuda & Documentação", to: "/ajuda" },
    ],
  },
];

export function getVisibleAdminNavGroups(params: {
  user: UserLike;
  can: (permission: string) => boolean;
}) {
  const { user, can } = params;

  const isAdmin =
    user?.permission === "owner" || user?.permission === "admin" || user?.functional_role === "admin";

  if (isAdmin) return adminNavGroups;

  return adminNavGroups
    .map((group) => {
      const hasModulePermission = group.permission ? can(group.permission) : true;
      if (!hasModulePermission) return { ...group, items: [] };
      return {
        ...group,
        items: group.items.filter((item) => (!item.permission ? true : can(item.permission))),
      };
    })
    .filter((g) => g.items.length > 0);
}
