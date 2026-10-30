import {
  Users,
  Calendar,
  FileText,
  Wallet,
  Clock,
  AlertTriangle,
  ArrowRight,
  CheckCircle2,
  TrendingUp,
  MapPin,
  MessageCircle,
  Phone,
  Home,
  Target,
} from "lucide-react";
import { EmptyState } from "../../shared/ui/EmptyState";
import { Link, useNavigate } from "react-router";
import { clsx } from "clsx";
import { motion, AnimatePresence } from "motion/react";
import { useAuth } from "../../shared/context/AuthContext";
import { useFocusMode } from "../../shared/context/FocusModeContext";
import { usePermissionsContext } from "../../shared/context/PermissionsContext";
import { useEffect, useState, useRef } from "react";
import { resolvePropertyDisplay } from "../../shared/data";
import { useLeadsContext } from "../../shared/context/LeadsContext";
import { usePropertiesContext } from "../../shared/context/PropertiesContext";
import { useCalendarContext } from "../../shared/context/CalendarContext";
import { useEsteiraDeals } from "../../shared/hooks/useEsteiraDeals";
import { useTheme } from "../../shared/context/ThemeContext";
import { CompanyLogo } from "../../shared/components/CompanyLogo";
import { leadService } from "../../shared/services/lead.service";
import { dealService } from "../../shared/services/deal.service";
import { brokerService } from "../../shared/services/broker.service";
import { commissionService } from "../../shared/services/commission.service";

import { Button } from "../../shared/ui/button";
import { Avatar, AvatarFallback, AvatarImage } from "../../shared/ui/avatar";
import { MediaImage } from "../../shared/components/MediaImage";
import { getAvatarThumbUrl } from "../../shared/media";

export function Dashboard() {
  const navigate = useNavigate();
  const { leads } = useLeadsContext();
  const { deals } = useEsteiraDeals();
  const { appointments } = useCalendarContext();
  const { properties } = usePropertiesContext();
  const { user, companies, isLoading } = useAuth();
  const { theme } = useTheme();
  const { focusMode } = useFocusMode();
  const { can } = usePermissionsContext();

  const [leadStats, setLeadStats] = useState({ total: 0, conversion_rate: 0 });
  const [dealStats, setDealStats] = useState({ monthly_sales: 0 });
  const [commissionValue, setCommissionValue] = useState(0);

  const redirectAttempted = useRef(false);

  useEffect(() => {
    if (!isLoading && user && (!companies || companies.length === 0)) {
      // Avoid immediate redirect on first paint — the auth context may still
      // be settling after company creation or token refresh. A brief delay
      // prevents aborting lazy chunks and API requests already in-flight.
      if (redirectAttempted.current) return;
      redirectAttempted.current = true;
      const timer = setTimeout(() => {
        navigate("/onboarding");
      }, 800);
      return () => clearTimeout(timer);
    }
  }, [user, companies, isLoading, navigate]);

  useEffect(() => {
    if (user?.current_company_id) {
      leadService
        .getStats()
        .then((stats) => {
          setLeadStats({
            total: stats.total,
            conversion_rate: stats.conversion_rate,
          });
        })
        .catch(console.error);

      if (can("finance:read")) {
        commissionService
          .list()
          .then((res) => {
            const total = (res.data || []).reduce((sum, c) => sum + c.amount, 0);
            setCommissionValue(total);
          })
          .catch(() => setCommissionValue(0));
      }

      if (user?.permission === "corretor" || user?.permission === "broker") {
        brokerService
          .getPerformance(user.id)
          .then((response) => {
            if (response.data) {
              setDealStats({ monthly_sales: response.data.data.sales_volume || 0 });
              setLeadStats((prev) => ({ ...prev, conversion_rate: response.data.data.conversion_rate || 0 }));
            }
          })
          .catch(console.error);
      } else {
        dealService
          .getStats()
          .then((response) => {
            if (response.data) {
              setDealStats({ monthly_sales: response.data.data.monthly_sales || 0 });
            }
          })
          .catch(console.error);
      }
    }
  }, [user?.current_company_id]);

  const todayRaw = new Intl.DateTimeFormat("pt-BR", {
    weekday: "long",
    day: "numeric",
    month: "long",
  }).format(new Date());
  const today = todayRaw.charAt(0).toUpperCase() + todayRaw.slice(1);

  const formatCurrency = (val: number) =>
    new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL", maximumFractionDigits: 0 }).format(val);

  // O backend (lead stats e performance do corretor) devolve a taxa de
  // conversão como FRAÇÃO 0..1 (converted/total); o bloco exibe percentual.
  // Sem o ×100, 100% aparecia como "1%".
  const formatConversionRate = (rate: number | undefined) => `${Math.round((rate ?? 0) * 100)}%`;

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center bg-muted ">
        <div className="w-8 h-8 border-4 border-primary-500 border-t-transparent rounded-full animate-spin"></div>
      </div>
    );
  }

  const companiesList = Array.isArray(companies) ? companies : [];
  const currentCompany = companiesList.find((c) => c.id === user?.current_company_id) || companiesList[0];
  const companyName = currentCompany?.name || "Sua Imobiliária";

  const todayDate = new Date();
  const todayStr =
    todayDate.getFullYear() +
    "-" +
    String(todayDate.getMonth() + 1).padStart(2, "0") +
    "-" +
    String(todayDate.getDate()).padStart(2, "0");
  const dashboardTasks = appointments
    .filter((apt) => apt.date === todayStr && apt.status !== "Cancelado")
    .slice(0, 5)
    .map((apt) => ({
      id: apt.id,
      type: apt.type.toLowerCase(),
      title: apt.title,
      time: apt.time,
      client: apt.client,
      status: apt.status === "Concluido" ? "completed" : "pending",
    }));

  const dashboardLeads = leads
    .filter((l) => l.status === "Novo" || l.status === "Em Atendimento")
    .slice(0, 5)
    .map((l) => ({
      id: l.id,
      name: l.name,
      interest: resolvePropertyDisplay(l.property, properties),
      status: l.status,
      sla: "Novo Lead",
      time: "Recente",
      source: "Website",
      avatar: l.avatar,
    }));

  const myNegotiations = deals
    .filter((d) => d.brokerId === user?.id && d.stage !== "Concluído" && d.stage !== "Perdido")
    .slice(0, 5)
    .map((d) => {
      const prop = properties.find((p) => p.id === d.propertyId);
      return {
        id: d.id,
        client: d.client,
        property: d.property,
        value: d.value,
        stage: d.stage,
        action: "Ver detalhes",
        // Sem foto do imóvel, `MediaImage` já cai no próprio fallback — o
        // placeholder remoto do Unsplash saiu do projeto (a requisição externa
        // vazava do gate e2e).
        image: prop?.image,
      };
    });

  const activeLeadsCount = leads.filter((l) => l.status !== "Perdido" && l.status !== "Convertido").length;
  const visitsTodayCount = appointments.filter((a) => a.date === todayStr && a.type === "Visita").length;
  const pendingProposalsCount = deals.filter((d) => d.stage === "Proposta" || d.stage === "Análise").length;

  return (
    <div className="flex flex-col h-full bg-muted overflow-auto">
      {/*
        `z-20` (e não `z-10`) porque a linha do tempo da Agenda do Dia usa
        `z-10` nos marcadores: como eles vêm DEPOIS no DOM, com o mesmo
        z-index os marcadores pintavam por cima deste cabeçalho sticky quando a
        Home rolava — o marcador do stepper aparecia solto sobre "Atender Novo
        Lead". Continua abaixo do cabeçalho do app (`z-40`, em Layout.tsx).
      */}
      <div className="flex-none px-6 py-6 lg:px-8 border-b border-border bg-card sticky top-0 z-20">
        <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
          <div>
            <h1 className="text-2xl font-bold text-foreground tracking-tight">Painel</h1>
            <div className="flex items-center gap-2 mt-1">
              <span className="text-sm font-medium text-foreground">Olá, {user?.firstname || "Corretor"}</span>
              <span className="text-muted-foreground/60">|</span>
              <CompanyLogo logoUrl={theme?.logoUrl} className="w-4 h-4 object-contain opacity-80" />
              <p className="text-sm font-medium text-foreground ">{companyName}</p>
              <span className="text-muted-foreground/60">|</span>
              <p className="text-sm text-muted-foreground">{today}</p>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <Button onClick={() => navigate("/leads")} className="flex items-center justify-center gap-2" variant="default">
              <Users className="w-4 h-4" />
              <span>Atender Novo Lead</span>
            </Button>
          </div>
        </div>
      </div>

      <div className="flex-1 p-6 lg:p-8 space-y-6">
        <AnimatePresence initial={false}>
          {!focusMode && (
            <motion.div
              initial={{ opacity: 0, height: 0 }}
              animate={{ opacity: 1, height: "auto" }}
              exit={{ opacity: 0, height: 0 }}
              transition={{ duration: 0.3, ease: "easeInOut" }}
            >
              <div className="grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4">
                <div className="bg-card p-5 rounded-2xl border border-border shadow-sm flex items-center justify-between group cursor-pointer hover:border-primary-300 dark:hover:border-primary-600 transition-all">
                  <div>
                    <p className="text-muted-foreground text-sm font-medium mb-1">Leads Ativos</p>
                    <h2 className="text-2xl font-bold text-foreground ">{activeLeadsCount}</h2>
                  </div>
                  <div className="p-3 bg-primary-50 dark:bg-primary-900/40 text-primary-600 dark:text-primary-400 rounded-xl group-hover:bg-primary-100 dark:group-hover:bg-primary-900/60 transition-colors">
                    <Users className="w-6 h-6" />
                  </div>
                </div>

                <div
                  onClick={() => navigate("/agenda")}
                  className="bg-card p-5 rounded-2xl border border-border shadow-sm flex items-center justify-between group cursor-pointer hover:border-stage-contract/40 dark:hover:border-stage-contract transition-all"
                >
                  <div>
                    <p className="text-muted-foreground text-sm font-medium mb-1">Visitas Hoje</p>
                    <h2 className="text-2xl font-bold text-foreground ">{visitsTodayCount}</h2>
                  </div>
                  <div className="p-3 bg-stage-contract/10 text-stage-contract rounded-xl group-hover:bg-stage-contract/15 transition-colors">
                    <Calendar className="w-6 h-6" />
                  </div>
                </div>

                <div className="bg-card p-5 rounded-2xl border border-border shadow-sm flex items-center justify-between group cursor-pointer hover:border-orange-300 dark:hover:border-orange-600 transition-all">
                  <div>
                    <p className="text-muted-foreground text-sm font-medium mb-1">Propostas Pendentes</p>
                    <h2 className="text-2xl font-bold text-foreground ">{pendingProposalsCount}</h2>
                  </div>
                  <div className="p-3 bg-orange-50 dark:bg-orange-900/40 text-orange-600 dark:text-orange-400 rounded-xl group-hover:bg-orange-100 dark:group-hover:bg-orange-900/60 transition-colors">
                    <FileText className="w-6 h-6" />
                  </div>
                </div>

                <div className="bg-card p-5 rounded-2xl border border-border shadow-sm flex items-center justify-between group cursor-pointer hover:border-status-ok/40 dark:hover:border-status-ok transition-all">
                  <div>
                    <p className="text-muted-foreground text-sm font-medium mb-1">Comissões (Est.)</p>
                    <h2 className="text-2xl font-bold text-foreground ">{formatCurrency(commissionValue)}</h2>
                  </div>
                  <div className="p-3 bg-status-ok-soft text-status-ok rounded-xl group-hover:bg-status-ok-soft dark:group-hover:bg-status-ok/60 transition-colors">
                    <Wallet className="w-6 h-6" />
                  </div>
                </div>
              </div>
            </motion.div>
          )}
        </AnimatePresence>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <div className="bg-card rounded-2xl border border-border shadow-sm flex flex-col">
            <div className="p-5 border-b border-border flex items-center justify-between">
              <h3 className="font-bold text-foreground flex items-center gap-2">
                <Calendar className="w-5 h-5 text-primary-500 dark:text-primary-400" /> Agenda do Dia
              </h3>
              <Button variant="ghost" size="sm" onClick={() => navigate("/agenda")} className="text-xs h-auto py-1">
                Ver todas
              </Button>
            </div>
            <div className="p-5 flex-1">
              {dashboardTasks.length > 0 ? (
                <div className="space-y-6 relative before:absolute before:inset-0 before:ml-2 before:-translate-x-px before:h-full before:w-0.5 before:bg-linear-to-b before:from-transparent before:via-slate-200 dark:before:via-slate-700 before:to-transparent">
                  {dashboardTasks.map((task) => (
                    <div
                      key={task.id}
                      role="button"
                      tabIndex={0}
                      onClick={() => navigate(`/agenda?apt=${task.id}`)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          navigate(`/agenda?apt=${task.id}`);
                        }
                      }}
                      className="relative flex items-start gap-3 group is-active cursor-pointer"
                    >
                      <div
                        className={clsx(
                          "flex items-center justify-center w-5 h-5 rounded-full border-2 bg-card shrink-0 z-10 mt-1",
                          task.status === "completed"
                            ? "border-status-ok text-status-ok"
                            : "border-primary-500 text-primary-500",
                        )}
                      >
                        {task.status === "completed" ? (
                          <CheckCircle2 className="w-3 h-3" />
                        ) : (
                          <div className="w-1.5 h-1.5 bg-primary-500 rounded-full" />
                        )}
                      </div>
                      <div className="flex-1 min-w-0 overflow-hidden p-3 rounded-xl border border-border shadow-sm bg-muted/50 /50 hover:bg-muted transition-colors">
                        <div className="flex items-center justify-between mb-1">
                          <span className="text-xs font-bold text-muted-foreground">{task.time}</span>
                          {(task.type === "visita" || task.type === "vistoria") && (
                            <MapPin className="w-3.5 h-3.5 text-muted-foreground" />
                          )}
                          {task.type === "ligacao" && <Phone className="w-3.5 h-3.5 text-muted-foreground" />}
                          {task.type !== "visita" && task.type !== "vistoria" && task.type !== "ligacao" && (
                            <FileText className="w-3.5 h-3.5 text-muted-foreground" />
                          )}
                        </div>
                        <h4
                          title={task.title}
                          className={clsx(
                            "text-sm font-semibold text-foreground [wrap:anywhere] line-clamp-2",
                            task.status === "completed" && "line-through text-muted-foreground",
                          )}
                        >
                          {task.title}
                        </h4>
                        <p title={task.client} className="text-xs text-muted-foreground mt-1 truncate">
                          {task.client}
                        </p>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="h-full flex items-center justify-center">
                  <EmptyState
                    icon={Calendar}
                    title="Sua agenda está livre"
                    message="Você não tem compromissos para hoje. Aproveite para organizar suas próximas atividades."
                    action={
                      <Button onClick={() => navigate("/agenda")} variant="secondary" className="text-xs">
                        Agendar Compromisso
                      </Button>
                    }
                    className="border-none shadow-none bg-transparent"
                  />
                </div>
              )}
            </div>
            <div className="p-4 border-t border-border bg-muted /50 rounded-b-2xl">
              <Button variant="outline" onClick={() => navigate("/agenda")} className="w-full bg-card">
                + Adicionar Tarefa
              </Button>
            </div>
          </div>

          <div className="bg-card rounded-2xl border border-border shadow-sm flex flex-col">
            <div className="p-5 border-b border-border flex items-center justify-between">
              <h3 className="font-bold text-foreground flex items-center gap-2">
                <Users className="w-5 h-5 text-primary-600" /> Leads Recentes
              </h3>
            </div>
            <div className="p-5 flex-1 space-y-4">
              {dashboardLeads.length > 0 ? (
                dashboardLeads.map((lead) => (
                  <div
                    key={lead.id}
                    role="button"
                    tabIndex={0}
                    onClick={() => navigate(`/leads/${lead.id}`)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        navigate(`/leads/${lead.id}`);
                      }
                    }}
                    className="p-4 rounded-xl border border-border hover:border-border dark:hover:border-foreground/25 transition-colors cursor-pointer"
                  >
                    <div className="flex items-start justify-between mb-3">
                      <div className="flex items-center gap-3">
                        <Avatar className="w-10 h-10 border border-border shrink-0">
                          {lead.avatar ? <AvatarImage src={getAvatarThumbUrl(lead.avatar, 80)} alt={lead.name} /> : null}
                          <AvatarFallback>{lead.name.charAt(0).toUpperCase()}</AvatarFallback>
                        </Avatar>
                        <div>
                          <h4 className="text-sm font-bold text-foreground ">{lead.name}</h4>
                          <span className="text-micro text-muted-foreground bg-muted px-1.5 py-0.5 rounded uppercase font-medium">
                            {lead.source}
                          </span>
                        </div>
                      </div>
                      <span className={clsx("text-micro font-bold px-2 py-1 rounded-md", "bg-status-info-soft text-status-info")}>
                        {lead.status}
                      </span>
                    </div>
                    <div className="space-y-1.5 mb-4">
                      <p className="text-xs text-muted-foreground flex items-center gap-1.5">
                        <Home className="w-3.5 h-3.5 text-muted-foreground" /> {lead.interest}
                      </p>
                    </div>
                    <div className="flex gap-2">
                      <Button variant="secondary" onClick={(e) => e.stopPropagation()} className="flex-1 text-xs gap-1">
                        <MessageCircle className="w-3.5 h-3.5" /> WhatsApp
                      </Button>
                      <Button
                        variant="outline"
                        onClick={(e) => {
                          e.stopPropagation();
                          navigate(`/leads/${lead.id}`);
                        }}
                        className="flex-1 text-xs gap-1"
                      >
                        Ver Ficha <ArrowRight className="w-3.5 h-3.5" />
                      </Button>
                    </div>
                  </div>
                ))
              ) : (
                <div className="h-full flex items-center justify-center">
                  <EmptyState
                    icon={Users}
                    title="Sem leads recentes"
                    message="Sua base de leads está vazia. Comece prospectando novos clientes para movimentar sua esteira."
                    action={
                      <Button onClick={() => navigate("/leads")} variant="secondary" className="text-xs">
                        Adicionar Novo Lead
                      </Button>
                    }
                    className="border-none shadow-none bg-transparent"
                  />
                </div>
              )}
            </div>
            <div className="p-4 border-t border-border ">
              <Button variant="ghost" onClick={() => navigate("/leads")} className="w-full text-sm font-medium">
                Ver todos os leads ativos
              </Button>
            </div>
          </div>

          <div className="space-y-6">
            <div className="bg-card rounded-2xl border border-border shadow-sm p-5">
              <h3 className="font-bold text-foreground flex items-center gap-2 mb-4">
                <Target className="w-5 h-5 text-status-ok" /> Metas do Mês
              </h3>

              <div className="space-y-4">
                <div>
                  <div className="flex justify-between text-sm mb-1">
                    <span className="font-medium text-foreground ">VGV Vendido</span>
                    <span className="font-bold text-foreground ">{formatCurrency(dealStats?.monthly_sales || 0)}</span>
                  </div>
                </div>

                <div>
                  <div className="flex justify-between text-sm mb-1">
                    <span className="font-medium text-foreground ">Taxa de Conversão</span>
                    <span className="font-bold text-foreground ">{formatConversionRate(leadStats?.conversion_rate)}</span>
                  </div>
                </div>
              </div>
            </div>

            <div className="bg-card rounded-2xl border border-border shadow-sm flex flex-col flex-1">
              <div className="p-5 border-b border-border flex items-center justify-between">
                <h3 className="font-bold text-foreground flex items-center gap-2">
                  <TrendingUp className="w-5 h-5 text-primary-500 dark:text-primary-400" /> Negociações Ativas
                </h3>
              </div>
              <div className="p-5 space-y-4">
                {myNegotiations.length > 0 ? (
                  myNegotiations.map((neg) => (
                    <div
                      key={neg.id}
                      role="button"
                      tabIndex={0}
                      onClick={() => navigate(`/esteira/deals/${neg.id}`)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          navigate(`/esteira/deals/${neg.id}`);
                        }
                      }}
                      className="flex gap-4 group cursor-pointer"
                    >
                      <MediaImage src={neg.image} alt={neg.property} className="w-16 h-16 rounded-xl object-cover border border-border shrink-0" />
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center justify-between mb-1">
                          <h4 className="text-sm font-bold text-foreground truncate group-hover:text-primary-600 dark:group-hover:text-primary-400 transition-colors">
                            {neg.client}
                          </h4>
                          <span className="text-micro font-bold text-primary-700 dark:text-primary-300 bg-primary-50 dark:bg-primary-900/40 px-1.5 py-0.5 rounded whitespace-nowrap">
                            {neg.stage}
                          </span>
                        </div>
                        <p className="text-xs text-muted-foreground truncate mb-1">{neg.property}</p>
                        <div className="flex items-center justify-between">
                          <span className="text-xs font-semibold text-foreground ">{formatCurrency(neg.value)}</span>
                        </div>
                        <div className="mt-2 bg-status-warning-soft text-status-warning text-micro font-medium p-1.5 rounded flex items-center gap-1.5">
                          <AlertTriangle className="w-3 h-3 shrink-0" />
                          <span className="truncate">{neg.action}</span>
                        </div>
                      </div>
                    </div>
                  ))
                ) : (
                  <div className="h-full flex items-center justify-center">
                    <EmptyState
                      icon={TrendingUp}
                      title="Nenhuma negociação ativa"
                      message="Você ainda não possui negociações em andamento. Converta um lead para iniciar."
                      action={
                        <Button onClick={() => navigate("/esteira")} variant="secondary" className="text-xs">
                          Acessar Esteira Digital
                        </Button>
                      }
                      className="border-none shadow-none bg-transparent"
                    />
                  </div>
                )}
              </div>
              <div className="p-4 border-t border-border mt-auto">
                <Button variant="ghost" onClick={() => navigate("/esteira")} className="w-full text-sm font-medium">
                  Ir para Esteira Digital
                </Button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
