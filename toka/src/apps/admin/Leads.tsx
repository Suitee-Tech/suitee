import { useState, useMemo, useEffect, useRef, useDeferredValue, useId } from "react";
import { downloadCsv } from "../../shared/utils/csvExport";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import * as z from "zod";
import {
  List,
  LayoutGrid,
  Plus,
  Filter,
  Download,
  Search,
  X,
  Settings,
  Users,
  ToggleLeft,
  ToggleRight,
  Check,
  Home,
  ShoppingBag,
  Key,
  Building2,
  Briefcase,
  ChevronsUpDown,
  Loader2,
} from "lucide-react";
import { clsx } from "clsx";
import { DndProvider } from "react-dnd";
import { HTML5Backend } from "react-dnd-html5-backend";
import { type Lead, type LeadStatus, type LeadType, type LeadSource } from "../../shared/data";
import { useLeadsContext } from "../../shared/context/LeadsContext";
import { useAgentsContext } from "../../shared/context/AgentsContext";
import { usePropertiesContext } from "../../shared/context/PropertiesContext";
import { BoardView } from "../../shared/components/BoardView";
import { ListView } from "../../shared/components/ListView";
import { motion, AnimatePresence } from "motion/react";
import { ExpandableSidePanel } from "../../shared/components/ExpandableSidePanel";
import { Popover, PopoverContent, PopoverTrigger } from "../../shared/ui/popover";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "../../shared/ui/command";
import { EmptyState } from "../../shared/ui/EmptyState";
import { PageHeader } from "../../shared/ui/page-header";
import { LoadError } from "../../shared/ui/load-error";
import { Skeleton } from "../../shared/ui/skeleton";
import { Badge } from "../../shared/ui/badge";
import { Button } from "../../shared/ui/button";
import { Avatar, AvatarImage, AvatarFallback } from "../../shared/ui/avatar";
import { getAvatarThumbUrl } from "../../shared/media";
import { leadService, type ApiLead } from "../../shared/services/lead.service";
import { RouletteUnassignedAlertDialog } from "../../shared/components/RouletteUnassignedAlertDialog";
import { PropertyPicker } from "../../shared/components/PropertyPicker";
import { CaptacaoWizard } from "../../shared/components/CaptacaoWizard";

import { useAuthUser } from "../../shared/context/AuthUserContext";
import { useViewPreferences } from "../../shared/context/ViewPreferencesContext";
import { Can } from "../../lib/permissions/Can";
import { usePermissionsContext } from "../../shared/context/PermissionsContext";
import { toast } from "sonner";
import { exportsService } from "../../shared/services/exports.service";
import { formatPhoneBR, parsePhoneBR } from "../../shared/utils/masks";
import { leadSchema, type LeadFormData } from "./leadFormSchema";
import { parseMoneyBR0 } from "../../shared/utils/format";
import { MoneyInput } from "../../shared/ui/money-input";
import { notifyApiError } from "../../shared/services/errorToast";

type ViewMode = "board" | "list";

export function Leads() {
  const { leads, leadsLoading, leadsError, fetchLeads, addLead, updateLead, updateLeadStatus, movingLeadIds } = useLeadsContext();
  const { agents } = useAgentsContext();
  const { user } = useAuthUser();
  const { can, isLoading: permissionsLoading, hasTenantWideScope } = usePermissionsContext();
  // Enquanto as permissões carregam, NÃO se recorta a lista: o servidor já
  // devolveu o que o usuário pode ver, e filtrar por "não é elevado" no escuro
  // esconderia tudo por um instante.
  const isElevated = !permissionsLoading && hasTenantWideScope;

  const [leadsView, setLeadsView] = useViewPreferences<{
    viewMode: ViewMode;
    statusFilter: LeadStatus | "Todos";
    isClientFilter: boolean | "Todos";
    leadTypeFilter: LeadType | "Todos";
  }>("leads", {
    viewMode: "board",
    statusFilter: "Todos",
    isClientFilter: "Todos",
    leadTypeFilter: "Todos",
  });
  const { viewMode, statusFilter, isClientFilter, leadTypeFilter } = leadsView;
  const setViewMode = (v: ViewMode) => setLeadsView({ viewMode: v });
  const setStatusFilter = (v: LeadStatus | "Todos") => setLeadsView({ statusFilter: v });
  const setIsClientFilter = (v: boolean | "Todos") => setLeadsView({ isClientFilter: v });
  const setLeadTypeFilter = (v: LeadType | "Todos") => setLeadsView({ leadTypeFilter: v });

  const [searchTerm, setSearchTerm] = useState("");

  const canSeeShared = !permissionsLoading && can("shared_content:read");
  const [sharedTab, setSharedTab] = useState<"todos" | "compartilhados">("todos");

  // No mobile o painel de busca/filtros fica recolhido: sem isso o cabeçalho
  // consumia ~40% da tela antes de qualquer conteúdo. `mobilePanel` guarda qual
  // dos dois botões de ícone abriu o painel (o de busca também foca o campo).
  // Em ≥sm o painel é sempre visível e os botões somem.
  const [mobilePanel, setMobilePanel] = useState<"search" | "filters" | null>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (mobilePanel === "search") searchInputRef.current?.focus();
  }, [mobilePanel]);

  const activeFilterCount =
    (statusFilter !== "Todos" ? 1 : 0) +
    (leadTypeFilter !== "Todos" ? 1 : 0) +
    (isClientFilter !== "Todos" ? 1 : 0) +
    (canSeeShared && sharedTab !== "todos" ? 1 : 0);

  const [isNewLeadModalOpen, setIsNewLeadModalOpen] = useState(false);
  const [isRouletteModalOpen, setIsRouletteModalOpen] = useState(false);
  const [isCaptacaoWizardOpen, setIsCaptacaoWizardOpen] = useState(false);

  const deferredSearchTerm = useDeferredValue(searchTerm);

  const filteredLeads = useMemo(() => {
    const normalizedSearch = (deferredSearchTerm || "").trim().toLowerCase();

    return leads.filter((lead) => {
      const matchesSearch =
        normalizedSearch === "" ||
        (lead.name?.toLowerCase() || "").includes(normalizedSearch) ||
        (lead.email?.toLowerCase() || "").includes(normalizedSearch) ||
        (lead.phone || "").includes(normalizedSearch) ||
        (lead.property?.toLowerCase() || "").includes(normalizedSearch);

      const matchesStatus = statusFilter === "Todos" || lead.status === statusFilter;
      const matchesClient = isClientFilter === "Todos" || lead.isClient === isClientFilter;
      const matchesLeadType = leadTypeFilter === "Todos" || lead.leadType === leadTypeFilter;

      const matchesShared = sharedTab === "todos" || !!lead.sharedFrom;

      const matchesAgent = leadMatchesAgentScope(lead, user?.id, isElevated);

      return matchesSearch && matchesStatus && matchesClient && matchesLeadType && matchesShared && matchesAgent;
    });
  }, [deferredSearchTerm, isClientFilter, isElevated, leadTypeFilter, leads, sharedTab, statusFilter, user]);

  const moveLead = (leadId: string, newStatus: LeadStatus) => {
    void updateLeadStatus(leadId, newStatus).catch(() => undefined);
  };

  const canAssignLead = !permissionsLoading && can("leads:write");
  const assignLead = (leadId: string, agentId: string) => {
    void updateLead(leadId, { agentId }).catch(() => undefined);
  };

  const handleExportCSV = () => {
    const headers = ["ID", "Nome", "Email", "Telefone", "Imóvel de Interesse", "Orçamento", "Status", "Data de Criação"];
    const csvRows = [headers.join(",")];

    for (const lead of filteredLeads) {
      const row = [
        lead.displayId || lead.id.substring(0, 8),
        `"${lead.name}"`,
        `"${lead.email}"`,
        `"${lead.phone}"`,
        `"${lead.property}"`,
        lead.budget,
        `"${lead.status}"`,
        `"${lead.date}"`,
      ];
      csvRows.push(row.join(","));
    }

    // downloadCsv troca a data-URI (que não carrega o BOM e passava por
    // encodeURI) por um Blob com BOM UTF-8 — acentos corretos no Excel.
    downloadCsv(`leads_export_${new Date().toISOString().split("T")[0]}.csv`, csvRows.join("\n"));
  };

  const [asyncExporting, setAsyncExporting] = useState(false);
  const handleAsyncExport = async () => {
    if (asyncExporting) return;
    setAsyncExporting(true);
    try {
      const job = await exportsService.createLeadExport({ format: "csv" });
      toast.success("Export enfileirado. Gerando arquivo...");
      const done = await exportsService.pollUntilDone(job.id, { pollEveryMs: 2000, timeoutMs: 5 * 60 * 1000 });
      if (done.status === "FAILED") {
        toast.error(`Falha no export: ${done.error_message || "erro desconhecido"}`);
        return;
      }
      if (done.result_url) {
        window.open(done.result_url, "_blank");
        toast.success(`Pronto! ${done.row_count ?? 0} leads exportados.`);
      }
    } catch (err) {
      const msg = (err as Error)?.message || "Erro ao processar export.";
      toast.error(msg);
    } finally {
      setAsyncExporting(false);
    }
  };

  return (
    <DndProvider backend={HTML5Backend}>
      <div className="flex flex-col h-full bg-muted font-sans">
        <div className="flex-none px-6 py-4 lg:px-8 border-b border-border bg-card sticky top-0 z-10">
          <PageHeader
            title="Gestão de Leads"
            description="Acompanhe e mova seus contatos pelo funil de vendas."
            className="mb-4"
          >
            {/* flex-wrap: no mobile os botões não cabem numa linha só e, sem
                quebra, o min-content da linha empurrava o `main` do layout
                para além da viewport — o conteúdo da página inteira ganhava
                scroll horizontal (o do kanban é interno e não deve vazar). */}
            <div className="flex flex-wrap items-center gap-3 w-full sm:w-auto">
              <div className="flex bg-muted p-1 rounded-lg border border-border" role="group" aria-label="Modo de visualização">
                <button
                  type="button"
                  onClick={() => setViewMode("list")}
                  aria-pressed={viewMode === "list"}
                  aria-label="Exibir leads em lista"
                  className={clsx(
                    "flex items-center gap-2 px-3 py-1.5 rounded-md text-sm font-medium transition-all",
                    viewMode === "list" ? "bg-card text-primary-700 shadow-sm" : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  <List className="w-4 h-4" aria-hidden="true" />
                  <span className="hidden sm:inline">Lista</span>
                </button>
                <button
                  type="button"
                  onClick={() => setViewMode("board")}
                  aria-pressed={viewMode === "board"}
                  aria-label="Exibir leads em quadro kanban"
                  className={clsx(
                    "flex items-center gap-2 px-3 py-1.5 rounded-md text-sm font-medium transition-all",
                    viewMode === "board" ? "bg-card text-primary-700 dark:text-primary-400 shadow-sm" : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  <LayoutGrid className="w-4 h-4" aria-hidden="true" />
                  <span className="hidden sm:inline">Quadro</span>
                </button>
              </div>

              {/* Mobile: busca e filtros recolhidos atrás de ícones. O estado
                  ativo continua visível (ponto no ícone de busca e contador no
                  de filtros) para ninguém ficar com uma lista filtrada sem
                  saber. Em ≥sm os dois botões desaparecem. */}
              <div className="flex items-center gap-1 sm:hidden">
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  onClick={() => setMobilePanel((prev) => (prev === "search" ? null : "search"))}
                  aria-expanded={mobilePanel !== null}
                  aria-controls="leads-search-filters"
                  aria-label={searchTerm ? `Buscar leads (busca ativa: ${searchTerm})` : "Buscar leads"}
                  className={clsx("relative", (mobilePanel === "search" || searchTerm) && "border-primary-600 text-primary-700")}
                >
                  <Search className="w-4 h-4" aria-hidden="true" />
                  {searchTerm && (
                    <span className="absolute -top-1 -right-1 w-2.5 h-2.5 rounded-full bg-primary-600 border-2 border-card" aria-hidden="true" />
                  )}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  onClick={() => setMobilePanel((prev) => (prev === "filters" ? null : "filters"))}
                  aria-expanded={mobilePanel !== null}
                  aria-controls="leads-search-filters"
                  aria-label={activeFilterCount > 0 ? `Filtrar leads (${activeFilterCount} ${activeFilterCount === 1 ? "filtro ativo" : "filtros ativos"})` : "Filtrar leads"}
                  className={clsx("relative", (mobilePanel === "filters" || activeFilterCount > 0) && "border-primary-600 text-primary-700")}
                >
                  <Filter className="w-4 h-4" aria-hidden="true" />
                  {activeFilterCount > 0 && (
                    <span className="absolute -top-1.5 -right-1.5 min-w-4 h-4 px-1 rounded-full bg-primary-600 text-primary-foreground text-micro font-bold flex items-center justify-center border-2 border-card" aria-hidden="true">
                      {activeFilterCount}
                    </span>
                  )}
                </Button>
              </div>

              <div className="h-6 w-px bg-muted-foreground/30 hidden sm:block"></div>

              <Can permission="leads:import_export">
                <Button
                  variant="outline"
                  size="icon"
                  onClick={handleExportCSV}
                  className="hidden sm:flex"
                  title="Exportar CSV (página atual)"
                >
                  <Download className="w-4 h-4" />
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleAsyncExport}
                  loading={asyncExporting}
                  loadingText="Gerando..."
                  className="hidden text-xs sm:flex"
                  title="Exportar tudo (servidor)"
                >
                  <Download className="w-4 h-4" />
                  Tudo
                </Button>
              </Can>
              <Can permission="leads:write">
                <Button onClick={() => setIsNewLeadModalOpen(true)} className="flex-1 sm:flex-none">
                  <Plus className="w-4 h-4" />
                  Novo Lead
                </Button>
              </Can>
              <Can permission="properties:capture">
                <Button variant="outline" onClick={() => setIsCaptacaoWizardOpen(true)} className="flex-1 sm:flex-none">
                  <Briefcase className="w-4 h-4" />
                  Iniciar Captação
                </Button>
              </Can>
              <Can permission="leads:configure_roulette">
                <Button
                  variant="outline"
                  size="icon"
                  onClick={() => setIsRouletteModalOpen(true)}
                  title="Configurar Roleta de Leads"
                >
                  <Settings className="w-4 h-4" />
                </Button>
              </Can>
            </div>
          </PageHeader>

          <div
            id="leads-search-filters"
            className={clsx(
              "gap-3 items-center bg-muted p-3 rounded-lg border border-border",
              // Mobile: só renderiza quando um dos ícones abre o painel.
              // ≥sm: sempre visível (layout atual, sem botões de ícone).
              mobilePanel ? "flex flex-col" : "hidden",
              "sm:flex sm:flex-row",
            )}
          >
            <div className="relative flex-1 w-full">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <input
                ref={searchInputRef}
                type="text"
                placeholder="Buscar por nome, email, telefone ou imóvel..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                aria-label="Buscar leads por nome, email, telefone ou imóvel"
                className="w-full pl-9 pr-4 py-2 bg-card border border-border rounded-md text-sm focus:border-primary-500 focus:ring-1 focus:ring-primary-500 outline-none transition-all text-foreground placeholder:text-muted-foreground"
              />
            </div>

            {/* flex-wrap: os três selects usam w-full no mobile; numa linha
                sem quebra eles somavam ~3× a largura disponível e vazavam
                para fora do conteúdo do board. */}
            <div className="flex flex-wrap items-center gap-2 w-full sm:w-auto">
              {canSeeShared && (
                <div className="flex bg-card p-1 rounded-md border border-border" role="group" aria-label="Origem dos leads">
                  <button
                    type="button"
                    onClick={() => setSharedTab("todos")}
                    aria-pressed={sharedTab === "todos"}
                    className={clsx(
                      "px-3 py-1.5 rounded-sm text-sm font-medium transition-all whitespace-nowrap",
                      sharedTab === "todos" ? "bg-primary-50 text-primary-700" : "text-muted-foreground hover:text-foreground",
                    )}
                  >
                    Todos
                  </button>
                  <button
                    type="button"
                    onClick={() => setSharedTab("compartilhados")}
                    aria-pressed={sharedTab === "compartilhados"}
                    className={clsx(
                      "px-3 py-1.5 rounded-sm text-sm font-medium transition-all whitespace-nowrap",
                      sharedTab === "compartilhados"
                        ? "bg-primary-50 text-primary-700"
                        : "text-muted-foreground hover:text-foreground",
                    )}
                  >
                    Compartilhados
                  </button>
                </div>
              )}
              <Filter className="w-4 h-4 text-muted-foreground hidden sm:block" />
              <select
                value={statusFilter}
                onChange={(e) => setStatusFilter(e.target.value as any)}
                aria-label="Filtrar por status do lead"
                className="w-full sm:w-auto px-3 py-2 bg-card border border-border rounded-md text-sm text-foreground focus:border-primary-500 focus:ring-1 focus:ring-primary-500 outline-none"
              >
                <option value="Todos">Todos os Status</option>
                <option value="Novo">Novo</option>
                <option value="Em Atendimento">Em Atendimento</option>
                <option value="Qualificado">Qualificado</option>
                <option value="Visita Agendada">Visita Agendada</option>
                <option value="Em Negociação">Em Negociação</option>
                <option value="Proposta Enviada">Proposta Enviada</option>
                <option value="Convertido">Convertido</option>
                <option value="Perdido">Perdido</option>
              </select>
              <select
                value={leadTypeFilter}
                onChange={(e) => setLeadTypeFilter(e.target.value as any)}
                aria-label="Filtrar por tipo de lead"
                className="w-full sm:w-auto px-3 py-2 bg-card border border-border rounded-md text-sm text-foreground focus:border-primary-500 focus:ring-1 focus:ring-primary-500 outline-none"
              >
                <option value="Todos">Tipo de Lead</option>
                <option value="captacao">Captação</option>
                <option value="compra_venda">Compra e Venda</option>
                <option value="locacao">Locação</option>
              </select>
              <select
                value={isClientFilter as any}
                onChange={(e) => setIsClientFilter(e.target.value === "Todos" ? "Todos" : e.target.value === "true")}
                aria-label="Filtrar leads ou clientes"
                className="w-full sm:w-auto px-3 py-2 bg-card border border-border rounded-md text-sm text-foreground focus:border-primary-500 focus:ring-1 focus:ring-primary-500 outline-none"
              >
                <option value="Todos">Leads e Clientes</option>
                <option value="false">Leads</option>
                <option value="true">Clientes</option>
              </select>
            </div>
          </div>
        </div>

        <div className="flex-1 overflow-hidden p-6 lg:p-8 relative">
          {leadsLoading && leads.length === 0 ? (
            <LeadsLoadingState viewMode={viewMode} />
          ) : leadsError && leads.length === 0 ? (
            <div className="h-full flex items-center justify-center">
              <LoadError
                description="A leitura dos leads falhou. Verifique a conexão e tente novamente; a base não foi considerada vazia."
                onRetry={() => fetchLeads()}
                className="border-none bg-transparent"
              />
            </div>
          ) : leads.length === 0 ? (
            <div className="h-full flex items-center justify-center">
              <EmptyState
                icon={Users}
                title="Sua base de leads está vazia"
                message="Use o botão Novo Lead no topo da página para adicionar seu primeiro lead e acompanhá-lo no funil de vendas."
              />
            </div>
          ) : filteredLeads.length === 0 ? (
            <div className="h-full flex items-center justify-center">
              <EmptyState
                icon={Search}
                title="Nenhum lead encontrado"
                message="Não encontramos nenhum lead com os filtros aplicados. Tente ajustar seus termos de busca."
                action={
                  <Button
                    variant="outline"
                    onClick={() => {
                      setSearchTerm("");
                      setStatusFilter("Todos");
                      setIsClientFilter("Todos");
                      setLeadTypeFilter("Todos");
                    }}
                  >
                    Limpar Filtros
                  </Button>
                }
              />
            </div>
          ) : viewMode === "board" ? (
            <BoardView
              leads={filteredLeads}
              onMoveLead={moveLead}
              agents={agents}
              user={user}
              canAssign={canAssignLead}
              onAssignLead={assignLead}
              movingLeadIds={movingLeadIds}
            />
          ) : (
            <ListView leads={filteredLeads} agents={agents} user={user} canAssign={canAssignLead} onAssignLead={assignLead} />
          )}
        </div>
      </div>

      <ExpandableSidePanel
        open={isNewLeadModalOpen}
        onClose={() => setIsNewLeadModalOpen(false)}
        defaultSize="lg"
        expandedSize="full"
        bgClass="bg-card "
        renderHeader={({ expandButton, closeButton }) => (
          <div className="px-6 py-4 border-b border-border bg-muted /50 flex items-center justify-between flex-shrink-0">
            <div>
              <h2 className="text-lg font-bold text-foreground tracking-tight">Novo Lead</h2>
              <p className="text-sm text-muted-foreground">Selecione o tipo de lead ou preencha os dados</p>
            </div>
            <div className="flex items-center gap-2">
              {expandButton}
              {closeButton}
            </div>
          </div>
        )}
      >
        <NewLeadPanel
          onClose={() => setIsNewLeadModalOpen(false)}
          onCreate={async (newLead) => {
            await addLead(newLead);
            setIsNewLeadModalOpen(false);
          }}
          onUpdate={async (id, updates) => {
            await updateLead(id, updates);
            setIsNewLeadModalOpen(false);
          }}
        />
      </ExpandableSidePanel>

      <AnimatePresence>
        {isRouletteModalOpen && <RouletteModal onClose={() => setIsRouletteModalOpen(false)} />}
      </AnimatePresence>

      <CaptacaoWizard open={isCaptacaoWizardOpen} onClose={() => setIsCaptacaoWizardOpen(false)} />
    </DndProvider>
  );
}

function RouletteModal({ onClose }: { onClose: () => void }) {
  const {
    isRouletteEnabled,
    isRouletteMutationPending,
    setIsRouletteEnabled,
    activeRouletteAgents,
    setActiveRouletteAgents,
  } = useLeadsContext();
  const { agents } = useAgentsContext();
  const [unassignedDialogOpen, setUnassignedDialogOpen] = useState(false);
  const [unassignedCount, setUnassignedCount] = useState(0);
  const [unassignedDialogLoading, setUnassignedDialogLoading] = useState(false);

  const handleToggleRoulette = async () => {
    if (isRouletteMutationPending) return;

    try {
      if (isRouletteEnabled) {
        await setIsRouletteEnabled(false);
        return;
      }

      const status = await leadService.getRouletteUnassignedStatus();
      const hasUnassigned = !!status?.has_unassigned;
      const count = Number(status?.count || 0);
      if (hasUnassigned && count > 0) {
        setUnassignedCount(count);
        setUnassignedDialogOpen(true);
        return;
      }
      await setIsRouletteEnabled(true);
    } catch {
      // LeadsContext already restored the optimistic state and surfaced mutation errors.
    }
  };

  const handleEnableAssignAutomatically = async () => {
    setUnassignedDialogLoading(true);
    try {
      await setIsRouletteEnabled(true, { assignUnassignedLeads: true });
      setUnassignedDialogOpen(false);
    } catch {
      // Keep the decision dialog open so the user can retry.
    } finally {
      setUnassignedDialogLoading(false);
    }
  };

  const handleEnableKeepManual = async () => {
    setUnassignedDialogLoading(true);
    try {
      await setIsRouletteEnabled(true, { assignUnassignedLeads: false });
      setUnassignedDialogOpen(false);
    } catch {
      // Keep the decision dialog open so the user can retry.
    } finally {
      setUnassignedDialogLoading(false);
    }
  };

  const handleUnassignedDialogCancel = () => {
    if (unassignedDialogLoading) return;
    setUnassignedDialogOpen(false);
  };

  const toggleAgent = (agentId: string) => {
    setActiveRouletteAgents((prev) => {
      if (prev.includes(agentId)) {
        return prev.filter((id) => id !== agentId);
      }
      return [...prev, agentId];
    });
  };

  return (
    <div className="relative z-50">
      <motion.div
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        className="fixed inset-0 bg-foreground/40 backdrop-blur-sm"
        onClick={() => {
          if (!isRouletteMutationPending) onClose();
        }}
      />
      <motion.div
        initial={{ opacity: 0, scale: 0.95, y: 20 }}
        animate={{ opacity: 1, scale: 1, y: 0 }}
        exit={{ opacity: 0, scale: 0.95, y: 20 }}
        className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 w-full max-w-md bg-card rounded-xl shadow-2xl overflow-hidden flex flex-col font-sans"
      >
        <div className="flex items-center justify-between px-6 py-4 border-b border-border bg-muted">
          <div className="flex items-center gap-2">
            <Users className="w-5 h-5 text-primary-600" />
            <h2 className="text-lg font-bold text-foreground tracking-tight">Roleta de Leads</h2>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={onClose}
            disabled={isRouletteMutationPending}
            className="size-8"
          >
            <X className="w-5 h-5" />
          </Button>
        </div>

        <div className="p-6">
          <div className="flex items-center justify-between p-4 bg-muted border border-border rounded-xl mb-6">
            <div>
              <h3 className="text-sm font-bold text-foreground">Distribuição Automática</h3>
              <p className="text-xs text-muted-foreground mt-0.5">Distribuir novos leads uniformemente</p>
            </div>
            <button
              onClick={handleToggleRoulette}
              disabled={isRouletteMutationPending}
              aria-busy={isRouletteMutationPending}
              className={clsx(
                "p-1 rounded-full transition-colors disabled:cursor-wait disabled:opacity-60",
                isRouletteEnabled ? "text-primary-600" : "text-muted-foreground",
              )}
            >
              {isRouletteMutationPending ? (
                <Loader2 className="w-8 h-8 animate-spin" />
              ) : isRouletteEnabled ? (
                <ToggleRight className="w-8 h-8" />
              ) : (
                <ToggleLeft className="w-8 h-8" />
              )}
            </button>
          </div>
          <RouletteUnassignedAlertDialog
            open={unassignedDialogOpen}
            count={unassignedCount}
            isLoading={unassignedDialogLoading}
            onAssignAutomatically={handleEnableAssignAutomatically}
            onKeepManual={handleEnableKeepManual}
            onCancel={handleUnassignedDialogCancel}
          />

          <div className={clsx("transition-opacity", !isRouletteEnabled && "opacity-50 pointer-events-none")}>
            <h3 className="text-sm font-bold text-foreground mb-3">Corretores na Roleta</h3>
            <div className="space-y-2 max-h-60 overflow-y-auto pr-2 custom-scrollbar">
              {agents.map((agent) => {
                const isActive = activeRouletteAgents.includes(agent.id);
                return (
                  <div
                    key={agent.id}
                    onClick={() => toggleAgent(agent.id)}
                    className={clsx(
                      "flex items-center justify-between p-3 rounded-lg border cursor-pointer transition-all",
                      isActive ? "bg-primary-50/50 border-primary-200" : "bg-card border-border hover:border-primary-200",
                    )}
                  >
                    <div className="flex items-center gap-3">
                      <Avatar className="w-8 h-8 border border-border">
                        <AvatarImage src={agent.avatar} alt={agent.name} />
                        <AvatarFallback className="text-xs font-bold">{agent.name.charAt(0).toUpperCase()}</AvatarFallback>
                      </Avatar>
                      <div>
                        <p className="text-sm font-bold text-foreground">{agent.name}</p>
                        <p className="text-xs text-muted-foreground">{agent.role}</p>
                      </div>
                    </div>
                    <div
                      className={clsx(
                        "w-5 h-5 rounded-md flex items-center justify-center border",
                        isActive ? "bg-primary border-primary text-primary-foreground" : "bg-card border-border",
                      )}
                    >
                      {isActive && <Check className="w-3 h-3" />}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </div>

        <div className="px-6 py-4 bg-muted border-t border-border flex justify-end">
          <Button onClick={onClose} disabled={isRouletteMutationPending}>
            {isRouletteMutationPending ? "Salvando..." : "Concluir"}
          </Button>
        </div>
      </motion.div>
    </div>
  );
}

const leadTypeOptions: { value: LeadType; label: string; description: string; icon: typeof Home }[] = [
  {
    value: "captacao",
    label: "Captação de imóvel",
    description: "Proprietário que quer cadastrar um imóvel para venda ou locação",
    icon: Building2,
  },
  {
    value: "compra_venda",
    label: "Compra e venda",
    description: "Cliente interessado em comprar um imóvel",
    icon: ShoppingBag,
  },
  {
    value: "locacao",
    label: "Locação",
    description: "Cliente interessado em alugar um imóvel",
    icon: Key,
  },
];

const leadTypeConfig: Record<
  LeadType,
  {
    nameLabel: string;
    namePlaceholder: string;
    budgetLabel: string;
    propertyLabel: string;
    showOwnerNotice: boolean;
    listingFilter: "sale" | "rent" | null;
  }
> = {
  captacao: {
    nameLabel: "Nome do proprietário",
    namePlaceholder: "Ex: Maria da Silva (Proprietária)",
    budgetLabel: "Valor pretendido (R$)",
    propertyLabel: "Imóvel para captação",
    showOwnerNotice: true,
    listingFilter: null,
  },
  compra_venda: {
    nameLabel: "Nome completo",
    namePlaceholder: "Ex: João Silva",
    budgetLabel: "Orçamento (R$)",
    propertyLabel: "Imóvel de interesse",
    showOwnerNotice: false,
    listingFilter: "sale",
  },
  locacao: {
    nameLabel: "Nome completo",
    namePlaceholder: "Ex: João Silva",
    budgetLabel: "Orçamento mensal (R$)",
    propertyLabel: "Imóvel de interesse",
    showOwnerNotice: false,
    listingFilter: "rent",
  },
};

/**
 * Recorte por corretor na LISTA de leads.
 *
 * `tenantWideScope` vem do critério compartilhado (perfil + papel funcional +
 * papel base do perfil de acesso), o mesmo do servidor. Antes a tela decidia por
 * `user.permission` — o eixo do PERFIL, que num membro comum é `member` mesmo
 * com papel `admin` — e a lista aparecia vazia mesmo com a API devolvendo os
 * leads ("O perfil de Yuri mesmo sendo administrador não vê leads", incidente de
 * 2026-09-21). Lead compartilhado por parceria continua visível para todos.
 */
export function leadMatchesAgentScope(
  lead: { agentId?: string; sharedFrom?: unknown },
  userId: string | undefined,
  tenantWideScope: boolean,
): boolean {
  if (tenantWideScope) return true;
  if (lead.sharedFrom) return true;
  return Boolean(userId) && lead.agentId === userId;
}

function NewLeadPanel({
  onClose,
  onCreate,
  onUpdate,
}: {
  onClose: () => void;
  onCreate: (lead: Lead) => Promise<void>;
  onUpdate: (id: string, updates: Partial<Lead>) => Promise<void>;
}) {
  const { properties } = usePropertiesContext();
  const { agents } = useAgentsContext();
  const { isRouletteEnabled } = useLeadsContext();
  const { user } = useAuthUser();
  const { isLoading: permissionsLoading, hasTenantWideScope } = usePermissionsContext();
  const isAdmin = !permissionsLoading && hasTenantWideScope;
  const [selectedLeadType, setSelectedLeadType] = useState<LeadType | null>(null);
  const [leadMode, setLeadMode] = useState<"new" | "existing">("new");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [leadComboOpen, setLeadComboOpen] = useState(false);
  const [searchLeadTerm, setSearchLeadTerm] = useState("");
  const [suggestedLeads, setSuggestedLeads] = useState<ApiLead[]>([]);
  const [searchingLeads, setSearchingLeads] = useState(false);
  const [selectedExistingLead, setSelectedExistingLead] = useState<ApiLead | null>(null);
  const [duplicateCandidates, setDuplicateCandidates] = useState<ApiLead[]>([]);
  const [duplicateQueryKey, setDuplicateQueryKey] = useState("");
  const [dismissedDuplicateQueryKey, setDismissedDuplicateQueryKey] = useState("");
  const [brokerComboOpen, setBrokerComboOpen] = useState(false);
  const [leadTypeComboOpen, setLeadTypeComboOpen] = useState(false);
  const hasPreSelected = useRef(false);

  const displayAgents = useMemo(() => {
    const currentAgents = Array.isArray(agents) ? [...agents] : [];
    if (user?.id && !currentAgents.some((a) => a.id === user.id || (a as any).email === user.email)) {
      currentAgents.push({
        id: user.id,
        name: user.fullname || `${user.firstname || ""} ${user.lastname || ""}`.trim() || "Você",
        role: user.role || user.permission || "Corretor",
        avatar: user.avatar_url || "",
      });
    }
    return currentAgents;
  }, [agents, user]);

  const fieldId = useId();
  const { register, handleSubmit, formState: { errors }, setValue, watch } = useForm<LeadFormData>({
    resolver: zodResolver(leadSchema as any) as any,
    defaultValues: {
      name: "",
      email: "",
      phone: "",
      property: "",
      budget: undefined,
      status: "Novo",
      source: "Manual",
      agentId: "",
    },
  });

  useEffect(() => {
    if (user && !hasPreSelected.current) {
      if (isRouletteEnabled && isAdmin) {
        setValue("agentId", "", { shouldDirty: true, shouldValidate: true });
      } else if (!isAdmin) {
        setValue("agentId", user.id ?? "", { shouldDirty: true, shouldValidate: true });
      } else {
        setValue("agentId", user.id ?? "", { shouldDirty: true, shouldValidate: true });
      }

      hasPreSelected.current = true;
    }
  }, [user, setValue, isRouletteEnabled]);

  const selectedBrokerId = watch("agentId");
  const selectedBroker =
    displayAgents.find((a) => a.id === selectedBrokerId) ||
    (user?.id && selectedBrokerId === user.id
      ? {
          id: user.id,
          name: user.fullname || `${user.firstname || ""} ${user.lastname || ""}`.trim() || "Você",
          avatar: user.avatar_url,
        }
      : undefined);
  const selectedPropertyId = watch("property");
  const selectedProperty = properties.find((p) => p.id === selectedPropertyId);
  const watchName = watch("name");
  const watchEmail = watch("email");
  const watchPhone = watch("phone");
  const watchBudget = watch("budget");

  const roleToLeadTypeMap: Record<string, LeadType> = {
    owner: "captacao",
    buyer: "compra_venda",
    tenant: "locacao",
  };

  const apiStatusToFormStatus: Record<string, LeadFormData["status"]> = {
    NEW: "Novo",
    CONTACTED: "Em Atendimento",
    QUALIFIED: "Qualificado",
    PROPOSAL: "Qualificado",
    NEGOTIATION: "Qualificado",
    CONVERTED: "Qualificado",
    LOST: "Qualificado",
  };

  const apiSourceToFormSource: Record<string, LeadSource> = {
    WEBSITE: "Site",
    REFERRAL: "Indicacao",
    SOCIAL: "Redes Sociais",
    MANUAL: "Manual",
    CAMPAIGN: "Campanha",
    OTHER: "Outros",
  };

  const applyExistingLeadToForm = (lead: ApiLead) => {
    setSelectedExistingLead(lead);
    setLeadMode("existing");
    setSearchLeadTerm("");
    setSuggestedLeads([]);
    setLeadComboOpen(false);

    const mappedType = lead.role ? roleToLeadTypeMap[lead.role] : null;
    if (mappedType) setSelectedLeadType(mappedType);

    setValue("name", lead.name || "", { shouldDirty: true, shouldValidate: true });
    setValue("email", lead.email || "", { shouldDirty: true, shouldValidate: true });
    setValue("phone", parsePhoneBR(lead.phone || ""), { shouldDirty: true, shouldValidate: true });
    setValue("agentId", lead.assigned_broker_id || "", { shouldDirty: true, shouldValidate: true });

    const budget = lead.property_interest?.price_range?.max ?? 0;
    setValue("budget", budget, { shouldDirty: true, shouldValidate: true });

    const status = apiStatusToFormStatus[lead.status] || "Novo";
    setValue("status", status, { shouldDirty: true, shouldValidate: true });

    const source = lead.source && apiSourceToFormSource[lead.source] ? apiSourceToFormSource[lead.source] : "Manual";
    setValue("source", source, { shouldDirty: true, shouldValidate: true });

    const propertyId = lead.property_ids?.[0] || "";
    setValue("property", propertyId, { shouldDirty: true, shouldValidate: true });
  };

  useEffect(() => {
    if (leadMode === "new") {
      if (selectedExistingLead) {
        setSelectedExistingLead(null);
        setValue("name", "", { shouldDirty: true, shouldValidate: true });
        setValue("email", "", { shouldDirty: true, shouldValidate: true });
        setValue("phone", "", { shouldDirty: true, shouldValidate: true });
        setValue("property", "", { shouldDirty: true, shouldValidate: true });
        setValue("budget", 0, { shouldDirty: true, shouldValidate: true });
        setValue("status", "Novo", { shouldDirty: true, shouldValidate: true });
        setValue("source", "Manual", { shouldDirty: true, shouldValidate: true });
        setDismissedDuplicateQueryKey("");
      }
    }
  }, [leadMode, selectedExistingLead, setValue]);

  useEffect(() => {
    if (!leadComboOpen) return;

    if (!searchLeadTerm || searchLeadTerm.length < 2) {
      setSearchingLeads(false);
      setSuggestedLeads([]);
      return;
    }

    const timer = setTimeout(() => {
      setSearchingLeads(true);
      leadService
        .getAll({ search: searchLeadTerm, limit: 10, scope: "tenant" })
        .then((data) => setSuggestedLeads(data.data || []))
        .catch(() => setSuggestedLeads([]))
        .finally(() => setSearchingLeads(false));
    }, 300);

    return () => clearTimeout(timer);
  }, [searchLeadTerm, leadComboOpen]);

  useEffect(() => {
    if (leadMode !== "new") return;
    if (selectedExistingLead) return;

    const email = (watchEmail || "").trim();
    const name = (watchName || "").trim();
    const digits = (watchPhone || "").replace(/\D/g, "");

    let query = "";
    if (email.includes("@") && email.length >= 5) query = email;
    else if (digits.length >= 8) query = digits;
    else if (name.length >= 4) query = name;

    if (!query || query === duplicateQueryKey || query === dismissedDuplicateQueryKey) {
      setDuplicateCandidates([]);
      return;
    }

    setDuplicateQueryKey(query);
    const timer = setTimeout(() => {
      leadService
        .getAll({ search: query, limit: 5, scope: "tenant" })
        .then((res) => {
          const data = res.data || [];
          setDuplicateCandidates(data);
        })
        .catch(() => setDuplicateCandidates([]));
    }, 350);

    return () => clearTimeout(timer);
  }, [watchEmail, watchName, watchPhone, leadMode, selectedExistingLead, duplicateQueryKey, dismissedDuplicateQueryKey]);

  const selectedLeadTypeOption = selectedLeadType ? leadTypeOptions.find((o) => o.value === selectedLeadType) : undefined;

  const { properties: allProperties } = usePropertiesContext();
  const propertyOptions = useMemo(() => (Array.isArray(allProperties) ? allProperties : []), [allProperties]);

  const typeCfg = selectedLeadType ? leadTypeConfig[selectedLeadType] : null;
  const filteredPropertyOptions = useMemo(() => {
    const want = typeCfg?.listingFilter;
    if (!want) return propertyOptions;
    return propertyOptions.filter(
      (p) => p.listingType === want || (Array.isArray(p.availableFor) && p.availableFor.includes(want)),
    );
  }, [propertyOptions, typeCfg?.listingFilter]);

  const onSubmit = handleSubmit(async (data) => {
    if (isSubmitting) return;
    setIsSubmitting(true);
    try {
      const isClient = data.status === "Qualificado";
      const now = new Date().toISOString();

      const phoneDigits = parsePhoneBR(data.phone);

      if (selectedExistingLead) {
        await onUpdate(selectedExistingLead.id, {
          name: data.name,
          email: data.email,
          phone: phoneDigits,
          property: data.property,
          budget: data.budget,
          status: data.status as any,
          source: data.source,
          agentId: data.agentId,
          date: now,
          isClient,
          leadType: selectedLeadType || undefined,
        });
        toast.success("Lead atualizado!");
        return;
      }

      const newLead: Lead = {
        id: crypto.randomUUID(),
        name: data.name,
        email: data.email,
        phone: phoneDigits,
        property: data.property || "",
        budget: data.budget || 0,
        status: data.status as any,
        date: now,
        agentId: data.agentId,
        isClient,
        source: data.source,
        leadType: selectedLeadType || undefined,
      };

      // Single create path: onCreate → LeadsContext.addLead POSTs /leads (role derived from
      // leadType), dedups via upsertLeadFromApi and emits the 'Lead "<nome>" criado com
      // sucesso!' toast. Previously this block also called leadService.create() directly,
      // creating the lead twice (duplicate cards). If addLead throws, the panel stays open
      // and we surface the backend message below.
      await onCreate(newLead);
    } catch (error) {
      // Update errors are already surfaced by LeadsContext, which also rolls the
      // optimistic state back. Creation errors still originate here.
      if (!selectedExistingLead) notifyApiError(error, { context: "Criar lead" });
      console.error(error);
    } finally {
      setIsSubmitting(false);
    }
  });

  const handleDismissDuplicate = () => {
    setDismissedDuplicateQueryKey(duplicateQueryKey);
    setDuplicateCandidates([]);
  };

  return (
    // `flex-1 min-h-0`, não `h-full`: o painel já tem um header como irmão, e
    // `h-full` mede 100% do painel inteiro — com o header, o conteúdo passava da
    // altura e o excesso era recortado pelo `overflow-hidden` do painel, levando
    // o rodapé de ações junto. `min-h-0` deixa o flex encolher abaixo do
    // conteúdo, que é o que permite a rolagem acontecer dentro da área certa.
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-6 py-6">
        <div className="space-y-4">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setLeadMode("new")}
              className={clsx(
                "px-3 py-1.5 rounded-md text-sm font-medium border transition-all",
                leadMode === "new" ? "bg-primary-50 border-primary-200 text-primary-700" : "bg-card border-border text-muted-foreground",
              )}
            >
              Criar do zero
            </button>
            <button
              type="button"
              onClick={() => setLeadMode("existing")}
              className={clsx(
                "px-3 py-1.5 rounded-md text-sm font-medium border transition-all",
                leadMode === "existing"
                  ? "bg-primary-50 border-primary-200 text-primary-700"
                  : "bg-card border-border text-muted-foreground",
              )}
            >
              Aproveitar existente
            </button>
          </div>

          {leadMode === "existing" && (
            <div>
              <label className="text-sm font-bold text-foreground">Buscar lead existente</label>
              <Popover open={leadComboOpen} onOpenChange={setLeadComboOpen}>
                <PopoverTrigger asChild>
                  <Button variant="outline" role="combobox" aria-expanded={leadComboOpen} className="w-full justify-between mt-1">
                    {selectedExistingLead ? selectedExistingLead.name : "Digite nome, email ou telefone..."}
                    <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                  </Button>
                </PopoverTrigger>
                <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0" align="start">
                  <Command>
                    <CommandInput placeholder="Buscar lead..." value={searchLeadTerm} onValueChange={setSearchLeadTerm} />
                    <CommandList>
                      <CommandEmpty>
                        {searchLeadTerm.length < 2
                          ? "Digite ao menos 2 caracteres"
                          : searchingLeads
                            ? "Buscando..."
                            : "Nenhum lead encontrado"}
                      </CommandEmpty>
                      <CommandGroup heading="Resultados">
                        {suggestedLeads.map((l) => (
                          <CommandItem key={l.id} value={l.name} onSelect={() => applyExistingLeadToForm(l)}>
                            <div className="flex items-center gap-2 min-w-0">
                              <Avatar className="w-6 h-6 border border-border">
                                {l.avatar ? <AvatarImage src={getAvatarThumbUrl(l.avatar, 48)} alt={l.name} /> : null}
                                <AvatarFallback className="text-xs font-bold">{l.name?.charAt(0)?.toUpperCase() || "L"}</AvatarFallback>
                              </Avatar>
                              <div className="min-w-0">
                                <p className="text-sm font-medium truncate">{l.name}</p>
                                <p className="text-xs text-muted-foreground truncate">{l.email || l.phone}</p>
                              </div>
                            </div>
                          </CommandItem>
                        ))}
                      </CommandGroup>
                    </CommandList>
                  </Command>
                </PopoverContent>
              </Popover>
            </div>
          )}

          {leadMode === "new" && duplicateCandidates.length > 0 && (
            <div className="rounded-xl border border-status-warning/30 bg-status-warning-soft p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="text-sm font-bold text-status-warning">Possível duplicado</p>
                  <p className="text-xs text-muted-foreground mt-0.5">
                    Encontramos leads similares. Você quer vincular um existente?
                  </p>
                </div>
                <Button variant="ghost" size="icon" onClick={handleDismissDuplicate} className="size-8">
                  <X className="w-4 h-4" />
                </Button>
              </div>
              <div className="mt-3 space-y-2">
                {duplicateCandidates.map((l) => (
                  <button
                    key={l.id}
                    type="button"
                    onClick={() => applyExistingLeadToForm(l)}
                    className="w-full flex items-center justify-between gap-3 px-3 py-2 rounded-lg border border-border bg-card hover:bg-muted transition-colors"
                  >
                    <div className="min-w-0 text-left">
                      <p className="text-sm font-bold truncate">{l.name}</p>
                      <p className="text-xs text-muted-foreground truncate">{l.email || l.phone}</p>
                    </div>
                    <span className="text-xs text-muted-foreground">Selecionar</span>
                  </button>
                ))}
              </div>
            </div>
          )}

          {leadMode === "new" && !selectedLeadType && (
            <div>
              <label className="text-sm font-bold text-foreground">Tipo de lead</label>
              <div className="mt-2 grid grid-cols-1 gap-2">
                {leadTypeOptions.map((opt) => (
                  <button
                    key={opt.value}
                    type="button"
                    onClick={() => setSelectedLeadType(opt.value)}
                    className="flex items-start gap-3 rounded-xl border border-border bg-card p-3 text-left transition-all hover:border-primary-300 hover:bg-muted/50"
                  >
                    <span className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary-50 text-primary-600">
                      <opt.icon className="h-5 w-5" />
                    </span>
                    <span className="min-w-0">
                      <span className="block text-sm font-bold text-foreground">{opt.label}</span>
                      <span className="block text-xs text-muted-foreground">{opt.description}</span>
                    </span>
                  </button>
                ))}
              </div>
            </div>
          )}

          {selectedLeadType && selectedLeadTypeOption && (
            <div className="flex items-center gap-2">
              {leadMode === "new" && (
                <Button variant="ghost" onClick={() => setSelectedLeadType(null)}>
                  Voltar
                </Button>
              )}
              <Popover open={leadTypeComboOpen} onOpenChange={setLeadTypeComboOpen}>
                <PopoverTrigger asChild>
                  <Button variant="outline" role="combobox" aria-expanded={leadTypeComboOpen} className="flex-1 justify-between">
                    <div className="flex items-center gap-2 min-w-0">
                      <selectedLeadTypeOption.icon className="w-4 h-4 text-muted-foreground" />
                      <span className="truncate">{selectedLeadTypeOption.label}</span>
                    </div>
                    <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                  </Button>
                </PopoverTrigger>
                <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0" align="start">
                  <Command>
                    <CommandList>
                      <CommandGroup heading="Tipos">
                        {leadTypeOptions.map((opt) => (
                          <CommandItem
                            key={opt.value}
                            value={opt.value}
                            onSelect={() => {
                              setSelectedLeadType(opt.value);
                              setLeadTypeComboOpen(false);
                            }}
                          >
                            <div className="flex items-start gap-2">
                              <opt.icon className="w-4 h-4 mt-0.5 text-muted-foreground" />
                              <div>
                                <p className="text-sm font-bold">{opt.label}</p>
                                <p className="text-xs text-muted-foreground">{opt.description}</p>
                              </div>
                            </div>
                            {selectedLeadType === opt.value && <Check className="ml-auto w-4 h-4 text-primary-600" />}
                          </CommandItem>
                        ))}
                      </CommandGroup>
                    </CommandList>
                  </Command>
                </PopoverContent>
              </Popover>
            </div>
          )}

          {typeCfg?.showOwnerNotice && (
            <div className="rounded-xl border border-primary-200 bg-primary-50/50 p-3 dark:border-primary-800 dark:bg-primary-900/40">
              <p className="text-xs text-primary-700 dark:text-primary-200">Este lead é um proprietário que deseja cadastrar um imóvel. Preencha os dados do proprietário abaixo.</p>
            </div>
          )}

          {selectedLeadType && typeCfg && (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label htmlFor={`${fieldId}-name`} className="text-sm font-bold text-foreground">{typeCfg.nameLabel} <span aria-hidden="true" className="text-status-critical">*</span></label>
              <input
                {...register("name")}
                id={`${fieldId}-name`}
                aria-required="true"
                aria-invalid={errors.name ? true : undefined}
                aria-describedby={errors.name ? `${fieldId}-name-error` : undefined}
                className={clsx(
                  "mt-1 w-full px-3 py-2 bg-card border rounded-md text-sm outline-none focus:ring-1 focus:ring-primary-500",
                  errors.name ? "border-status-critical" : "border-border",
                )}
                placeholder={typeCfg.namePlaceholder}
              />
              {errors.name && <p id={`${fieldId}-name-error`} role="alert" className="text-xs text-status-critical mt-1">{errors.name.message}</p>}
            </div>
            <div>
              <label htmlFor={`${fieldId}-email`} className="text-sm font-bold text-foreground">E-mail <span aria-hidden="true" className="text-status-critical">*</span></label>
              <input
                {...register("email")}
                id={`${fieldId}-email`}
                type="email"
                aria-required="true"
                aria-invalid={errors.email ? true : undefined}
                aria-describedby={errors.email ? `${fieldId}-email-error` : undefined}
                className={clsx(
                  "mt-1 w-full px-3 py-2 bg-card border rounded-md text-sm outline-none focus:ring-1 focus:ring-primary-500",
                  errors.email ? "border-status-critical" : "border-border",
                )}
                placeholder="Ex: joao@email.com"
              />
              {errors.email && <p id={`${fieldId}-email-error`} role="alert" className="text-xs text-status-critical mt-1">{errors.email.message}</p>}
            </div>
            <div>
              <label htmlFor={`${fieldId}-phone`} className="text-sm font-bold text-foreground">Telefone / WhatsApp <span aria-hidden="true" className="text-status-critical">*</span></label>
              <input
                id={`${fieldId}-phone`}
                value={formatPhoneBR(watchPhone || "")}
                onChange={(e) => setValue("phone", parsePhoneBR(e.target.value), { shouldDirty: true, shouldValidate: true })}
                inputMode="tel"
                aria-required="true"
                aria-invalid={errors.phone ? true : undefined}
                aria-describedby={errors.phone ? `${fieldId}-phone-error` : undefined}
                className={clsx(
                  "mt-1 w-full px-3 py-2 bg-card border rounded-md text-sm outline-none focus:ring-1 focus:ring-primary-500",
                  errors.phone ? "border-status-critical" : "border-border",
                )}
                placeholder="Ex: (11) 99999-9999"
              />
              {errors.phone && <p id={`${fieldId}-phone-error`} role="alert" className="text-xs text-status-critical mt-1">{errors.phone.message as any}</p>}
            </div>
            <div>
              <label htmlFor={`${fieldId}-budget`} className="text-sm font-bold text-foreground">{typeCfg.budgetLabel}</label>
              <MoneyInput
                id={`${fieldId}-budget`}
                aria-describedby={errors.budget ? `${fieldId}-budget-error` : undefined}
                fractionDigits={0}
                withSymbol
                value={typeof watchBudget === "number" ? watchBudget : undefined}
                onChange={(v) =>
                  setValue("budget", v ?? 0, { shouldDirty: true, shouldValidate: true })
                }
                className={clsx(
                  "mt-1 w-full px-3 py-2 bg-card border rounded-md text-sm outline-none focus:ring-1 focus:ring-primary-500",
                  errors.budget ? "border-status-critical" : "border-border",
                )}
                placeholder="R$ 0"
              />
              {errors.budget && <p id={`${fieldId}-budget-error`} role="alert" className="text-xs text-status-critical mt-1">{errors.budget.message as any}</p>}
            </div>
          </div>
          )}

          {selectedLeadType && typeCfg && (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label htmlFor={`${fieldId}-property`} className="text-sm font-bold text-foreground">{typeCfg.propertyLabel}</label>
              <PropertyPicker
                id={`${fieldId}-property`}
                properties={filteredPropertyOptions}
                value={selectedPropertyId || ""}
                onChange={(id) => setValue("property", id, { shouldDirty: true, shouldValidate: true })}
              />
              {selectedProperty ? (
                <p className="text-xs text-muted-foreground mt-1 truncate">{selectedProperty.title || selectedProperty.displayId}</p>
              ) : null}
            </div>

            <div>
              <label htmlFor={`${fieldId}-status`} className="text-sm font-bold text-foreground">Status</label>
              <select
                {...register("status")}
                id={`${fieldId}-status`}
                className={clsx(
                  "mt-1 w-full px-3 py-2 bg-card border rounded-md text-sm outline-none focus:ring-1 focus:ring-primary-500",
                  errors.status ? "border-status-critical" : "border-border",
                )}
              >
                <option value="Novo">Novo</option>
                <option value="Em Atendimento">Em Atendimento</option>
                <option value="Qualificado">Qualificado</option>
              </select>
            </div>
          </div>
          )}

          {selectedLeadType && (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label htmlFor={`${fieldId}-source`} className="text-sm font-bold text-foreground">Origem</label>
              <select
                {...register("source")}
                id={`${fieldId}-source`}
                className={clsx(
                  "mt-1 w-full px-3 py-2 bg-card border rounded-md text-sm outline-none focus:ring-1 focus:ring-primary-500",
                  errors.source ? "border-status-critical" : "border-border",
                )}
              >
                <option value="Manual">Manual</option>
                <option value="Site">Site</option>
                <option value="Indicacao">Indicação</option>
                <option value="Redes Sociais">Redes Sociais</option>
                <option value="Campanha">Campanha</option>
                <option value="Outros">Outros</option>
              </select>
            </div>

            <div>
              <label htmlFor={`${fieldId}-broker`} className="text-sm font-bold text-foreground">Corretor Responsável</label>
              <Popover open={brokerComboOpen} onOpenChange={setBrokerComboOpen}>
                <PopoverTrigger asChild>
                  <Button id={`${fieldId}-broker`} aria-label="Corretor Responsável" variant="outline" role="combobox" aria-expanded={brokerComboOpen} className="w-full justify-between mt-1">
                    {selectedBroker ? (
                      <div className="flex items-center gap-2 min-w-0">
                        <Avatar className="w-6 h-6 border border-border">
                          {selectedBroker.avatar ? <AvatarImage src={getAvatarThumbUrl(selectedBroker.avatar, 48)} alt={selectedBroker.name} /> : null}
                          <AvatarFallback className="text-xs font-bold">{selectedBroker.name.charAt(0).toUpperCase()}</AvatarFallback>
                        </Avatar>
                        <span className="truncate">{selectedBroker.name}</span>
                      </div>
                    ) : (
                      <span className="truncate">Distribuir automaticamente (Roleta)</span>
                    )}
                    <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                  </Button>
                </PopoverTrigger>
                <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0" align="start">
                  <Command>
                    <CommandInput placeholder="Buscar corretor..." />
                    <CommandList>
                      <CommandEmpty>Nenhum corretor encontrado</CommandEmpty>
                      <CommandGroup heading="Corretores">
                        {displayAgents.map((a) => (
                          <CommandItem
                            key={a.id}
                            value={a.name}
                            onSelect={() => {
                              setValue("agentId", a.id, { shouldDirty: true, shouldValidate: true });
                              setBrokerComboOpen(false);
                            }}
                          >
                            <div className="flex items-center gap-2 min-w-0">
                              <Avatar className="w-6 h-6 border border-border">
                                {a.avatar ? <AvatarImage src={getAvatarThumbUrl(a.avatar, 48)} alt={a.name} /> : null}
                                <AvatarFallback className="text-xs font-bold">{a.name.charAt(0).toUpperCase()}</AvatarFallback>
                              </Avatar>
                              <div className="min-w-0">
                                <p className="text-sm font-medium truncate">{a.name}</p>
                                <p className="text-xs text-muted-foreground truncate">{a.role}</p>
                              </div>
                            </div>
                            {selectedBrokerId === a.id && <Check className="ml-auto w-4 h-4 text-primary-600" />}
                          </CommandItem>
                        ))}
                      </CommandGroup>
                    </CommandList>
                  </Command>
                </PopoverContent>
              </Popover>
            </div>
          </div>
          )}

        </div>
      </div>

      {/*
        Ações no rodapé FIXO do painel, fora da área rolável: no mobile o
        formulário é mais alto que a tela e esta linha era o último item do
        conteúdo rolável — com o recorte de altura do painel, "Criar Lead" e
        "Cancelar" ficavam inalcançáveis. Como rodapé `shrink-0` eles não
        dependem do scroll do formulário, e o `env(safe-area-inset-bottom)`
        impede que o indicador de home do aparelho cubra os botões.
      */}
      <div className="flex shrink-0 items-center justify-between gap-3 border-t border-border bg-card px-6 pt-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
        <Button variant="ghost" onClick={onClose}>
          Cancelar
        </Button>
        {selectedLeadType && (
          <Button onClick={onSubmit} loading={isSubmitting} loadingText="Salvando...">
            {selectedExistingLead ? "Salvar" : "Criar Lead"}
          </Button>
        )}
      </div>
    </div>
  );
}

function LeadsLoadingState({ viewMode }: { viewMode: ViewMode }) {
  const cards = viewMode === "board" ? 8 : 6;
  return (
    <div role="status" aria-label="Carregando leads" aria-busy="true" className="space-y-4">
      <div className="flex flex-wrap items-center gap-3" aria-hidden="true">
        <Skeleton className="h-10 w-full max-w-sm rounded-lg" />
        <Skeleton className="h-10 w-28 rounded-lg" />
        <Skeleton className="h-10 w-28 rounded-lg" />
      </div>
      <div
        className={
          viewMode === "board"
            ? "grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4"
            : "space-y-3"
        }
        aria-hidden="true"
      >
        {Array.from({ length: cards }).map((_, index) => (
          <Skeleton
            key={index}
            className={viewMode === "board" ? "h-48 w-full rounded-2xl" : "h-20 w-full rounded-xl"}
          />
        ))}
      </div>
    </div>
  );
}
