import { useEffect, useMemo, useState, type ReactNode } from "react";
import { usePortalAuth } from "../context/PortalAuthContext";
import { useNavigate } from "react-router";
import { portalService } from "../services/portal.service";
import {
  User,
  Settings,
  Bell,
  Lock,
  FileText,
  CreditCard,
  Wrench,
  RefreshCw,
  Wallet,
  FileSpreadsheet,
  FileBarChart,
  MessageSquare,
  Phone,
  HelpCircle,
  Heart,
  Star,
  LogOut,
  ChevronRight,
} from "lucide-react";
import { clsx } from "clsx";
import { Badge } from "../../../shared/ui/badge";
import { Button } from "../../../shared/ui/button";
import { usePortalDeals } from "../context/PortalDealsContext";
import { PortalPage } from "../shell/PortalPage";
import { PortalPageHeader } from "../shell/PortalPageHeader";
import { usePortalPersona } from "../hooks/usePortalPersona";

interface MenuItem {
  label: string;
  icon: typeof User;
  href?: string;
  onClick?: () => void;
  disabled?: boolean;
  badge?: ReactNode;
}

interface MenuSection {
  title: string;
  items: MenuItem[];
}

export function PortalMenu() {
  const { user, logout } = usePortalAuth();
  const navigate = useNavigate();
  const { activeDeal } = usePortalDeals();
  const [openMaintenanceCount, setOpenMaintenanceCount] = useState<number>(0);
  const [openTicketsCount, setOpenTicketsCount] = useState<number>(0);

  useEffect(() => {
    if (!activeDeal?.id) return;
    (async () => {
      try {
        const [tickets, maintenance] = await Promise.all([
          portalService.getDealTickets(activeDeal.id),
          portalService.getDealMaintenance(activeDeal.id),
        ]);
        const ticketList = Array.isArray(tickets) ? tickets : [];
        const maintenanceList = Array.isArray(maintenance) ? maintenance : [];
        const openTicketStatuses = new Set(["open", "assigned", "in_progress", "awaiting_client"]);
        const openTickets = ticketList.filter((t) => openTicketStatuses.has((t.status || "").toLowerCase())).length;
        const openMaintenance = maintenanceList.filter(
          (t) => (t.status || "").toLowerCase() !== "concluído" && (t.status || "").toLowerCase() !== "concluido",
        ).length;
        setOpenTicketsCount(openTickets);
        setOpenMaintenanceCount(openMaintenance);
      } catch {
        setOpenTicketsCount(0);
        setOpenMaintenanceCount(0);
      }
    })();
  }, [activeDeal?.id]);

  const initials = user?.name
    ? user.name
        .split(" ")
        .filter(Boolean)
        .map((w) => w[0])
        .slice(0, 2)
        .join("")
        .toUpperCase()
    : "—";

  const maintenanceBadge = useMemo(() => {
    const total = openMaintenanceCount + openTicketsCount;
    if (!total) return null;
    return (
      <Badge variant="secondary" className="ml-2 bg-muted text-muted-foreground font-semibold text-micro py-0 px-2 h-5">
        {total}
      </Badge>
    );
  }, [openMaintenanceCount, openTicketsCount]);

  // Itens de locação (boletos, manutenção, reajuste) não são fluxo de compra e
  // venda; e um locador não paga boleto do próprio imóvel — ele recebe repasse.
  const { persona, copy } = usePortalPersona();
  const isRentJourney = persona.isRent || persona.actor === "tenant" || persona.actor === "landlord";
  const isSeeker = persona.isSeeker || persona.isUnidentified;

  const rawSections: MenuSection[] = [
    {
      title: "MINHA CONTA",
      items: [
        { label: "Dados pessoais", icon: User, onClick: () => navigate("/portal/perfil") },
        { label: "Meus atendimentos", icon: Heart, onClick: () => navigate("/portal/leads") },
        ...(isSeeker
          ? [{ label: "Preferências de busca", icon: Settings, onClick: () => navigate("/portal/buscar") }]
          : []),
        { label: "Notificações", icon: Bell, onClick: () => navigate("/portal/notificacoes") },
        { label: "Segurança e senha", icon: Lock, onClick: () => navigate("/portal/perfil?tab=seguranca") },
      ],
    },
    {
      title: isRentJourney || persona.isOwnerSide ? "MEU IMÓVEL" : "MEU NEGÓCIO",
      items: [
        {
          label: "Dados do contrato",
          icon: FileText,
          onClick: () => (activeDeal ? navigate(`/portal/deals/${activeDeal.id}`) : navigate("/portal")),
        },
        { label: copy.financeEntryLabel, icon: CreditCard, onClick: () => navigate("/portal/finance") },
        ...(isRentJourney
          ? [
              {
                label: "Manutenção",
                icon: Wrench,
                onClick: () => navigate("/portal/chamados?tab=maintenance"),
                badge: maintenanceBadge,
              },
              { label: "Reajuste e renovação", icon: RefreshCw, disabled: true },
            ]
          : []),
      ],
    },
    {
      title: "FINANCEIRO",
      items: [
        { label: copy.financeEntryLabel, icon: Wallet, onClick: () => navigate("/portal/finance") },
        {
          label: persona.isOwnerSide ? `Extrato de ${copy.financeEntryLabel.toLowerCase().replace("meus ", "")}` : "Extrato de pagamentos",
          icon: FileSpreadsheet,
          onClick: () => navigate("/portal/finance?tab=pagos"),
        },
        { label: "Declaração para IR", icon: FileBarChart, onClick: () => navigate("/portal/informe-rendimentos") },
      ],
    },
    {
      title: "SUPORTE",
      items: [
        { label: "Chat com meu corretor", icon: MessageSquare, onClick: () => navigate("/portal/mensagens") },
        { label: "Falar com o corretor", icon: Phone, onClick: () => navigate("/portal/mensagens?contact=1") },
        { label: "Central de ajuda", icon: HelpCircle, onClick: () => navigate("/portal/suporte") },
        { label: "Avaliar o app", icon: Star, disabled: true },
      ],
    },
  ];

  // Ações ainda não disponíveis continuam visíveis para não criar uma
  // navegação inconsistente nem esconder capacidades previstas do portal.
  const sections = rawSections;

  return (
    <PortalPage>
      <PortalPageHeader title="Menu" />
      <div className="px-5 pt-8 pb-4">
        <div className="flex items-center gap-4 mb-10">
          <div className="w-[52px] h-[52px] rounded-full bg-muted flex items-center justify-center">
            <span className="text-title-lg font-bold text-foreground">{initials}</span>
          </div>
          <div>
            <p className="text-title-lg font-bold text-foreground">{user?.name || user?.email || "—"}</p>
            <p className="text-label text-muted-foreground">{user?.role || "—"}</p>
          </div>
        </div>

        <div className="space-y-8">
          {sections.map((section) => (
            <div key={section.title}>
              <p className="text-caption font-semibold text-muted-foreground uppercase tracking-[0.05em] mb-3 px-1">
                {section.title}
              </p>
              <div className="bg-card border border-border rounded-[20px] shadow-sm overflow-hidden">
                {section.items.map((item, index) => {
                  const Icon = item.icon;
                  const isLast = index === section.items.length - 1;
                  return (
                    <button
                      key={item.label}
                      onClick={item.onClick}
                      disabled={item.disabled}
                      aria-disabled={item.disabled}
                      className={clsx(
                        "w-full flex items-center justify-between px-5 py-4 transition-colors",
                        !item.disabled && "hover:bg-muted/50",
                        item.disabled && "opacity-60 cursor-not-allowed",
                        !isLast && "border-b border-border",
                      )}
                    >
                      <div className="flex items-center gap-3.5">
                        <Icon className="w-[18px] h-[18px] text-muted-foreground/70" />
                        <span className="text-title font-medium text-foreground">{item.label}</span>
                        {!item.disabled && item.badge}
                        {item.disabled && (
                          <Badge
                            variant="secondary"
                            className="ml-2 bg-muted text-muted-foreground font-medium text-micro py-0 px-2 h-5"
                          >
                            Em breve
                          </Badge>
                        )}
                      </div>
                      {!item.disabled && <ChevronRight className="w-4 h-4 text-muted-foreground/60" />}
                    </button>
                  );
                })}
              </div>
            </div>
          ))}
        </div>

        <div className="text-center mt-12 mb-4 space-y-4">
          <p className="text-caption font-medium text-muted-foreground/70">Suitee v1.0.0</p>
          <Button variant="ghost" onClick={logout} className="text-title font-semibold text-status-critical hover:text-status-critical">
            <LogOut className="w-4 h-4" />
            Sair da conta
          </Button>
        </div>
      </div>
    </PortalPage>
  );
}
