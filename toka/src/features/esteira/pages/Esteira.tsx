import { useState, useMemo, useRef, useEffect } from "react";
import { matchesDealDate } from "../utils/dealDateFilter";
import { getMediaUrl } from "../../../shared/media";
import {
 FileCheck,
 UploadCloud,
 PenTool,
 CheckCircle2,
 Clock,
 AlertCircle,
 Search,
 Plus,
 ArrowRightLeft,
 Filter,
 MoreHorizontal,
 FileText,
 MessageSquare,
 XCircle,
 CheckCircle,
 FileSignature,
 Download,
 FolderArchive,
 RefreshCw,
 Wallet,
 LayoutGrid,
 List,
 SlidersHorizontal,
 ArrowDown,
 ArrowUp,
 Calendar,
 History,
 ArrowLeftRight,
  Loader2
} from "lucide-react";
import { clsx } from "clsx";
import { DndProvider, useDrag, useDrop } from "react-dnd";
import { HTML5Backend } from "react-dnd-html5-backend";
import { motion, AnimatePresence } from "motion/react";
import { EsteiraDealDetails } from "../components/EsteiraDealDetails";
import { EsteiraDealModal } from "../components/EsteiraDealModal";
import { AuditTrailPanel } from "../../../shared/components/AuditTrailPanel";
import { type DealStage, type EsteiraDeal } from "../../../shared/deals/types";
import { useEsteiraDealPipelineContext } from "../context/EsteiraDealPipelineContext";
import { useConfirmAction } from "../../../shared/hooks/useConfirmAction";
import { useEsteiraDeals } from "../hooks/useEsteiraDeals";
import { useViewPreferences } from "../../../shared/context/ViewPreferencesContext";
import { EmptyState } from "../../../shared/ui/EmptyState";
import { Badge } from "../../../shared/ui/badge";
import { Button } from "../../../shared/ui/button";
import { Can } from "../../../lib/permissions/Can";
import { useIsAtLeastMd } from "../../../shared/hooks/useMediaQuery";
import { useEsteiraRealtimeSync } from "../hooks/esteira/useEsteiraRealtimeSync";
import { useWsConnectionState } from "../../../shared/ws/useWsConnectionState";
import { ALL_STAGE_METAS, MAIN_STAGE_METAS, type StageMeta } from "../../../shared/deals/dealStages";

type EsteiraStageDescriptor = { id: DealStage; label: string; icon: StageMeta['icon']; colorClass: string; bgClass: string };

const toEsteiraStage = (meta: StageMeta): EsteiraStageDescriptor => ({
 id: meta.label as DealStage,
 label: meta.key === 'ANALYSIS' ? 'Análise (Doc)' : meta.label,
 icon: meta.icon,
 colorClass: meta.colorClass,
 bgClass: meta.bgClass,
});

const STAGES: EsteiraStageDescriptor[] = MAIN_STAGE_METAS.map(toEsteiraStage);
const ALL_STAGES: EsteiraStageDescriptor[] = ALL_STAGE_METAS.map(toEsteiraStage);

const ITEM_TYPE = 'CONTRACT_CARD';

function EsteiraBoard() {
 const { moveDeal, deleteDeal, movingDealIds } = useEsteiraDealPipelineContext();
 const { confirmAction, confirmationDialog } = useConfirmAction();
 const { deals, dealsLoading, dealsError, refreshDeals } = useEsteiraDeals();
 const isAtLeastMd = useIsAtLeastMd();
 const wsConnected = useWsConnectionState();
 useEsteiraRealtimeSync();
 const [esteiraView, setEsteiraView] = useViewPreferences<{
 viewMode: 'kanban' | 'list';
 filterType: 'Todos' | 'Venda' | 'Locação';
 dateFilter: 'all' | 'today' | 'week';
 sortBy: 'recent' | 'value-desc' | 'value-asc';
 activeTab: 'Consolidada' | DealStage;
 }>('esteira', {
 viewMode: 'kanban',
 filterType: 'Todos',
 dateFilter: 'all',
 sortBy: 'recent',
 activeTab: 'Consolidada',
 });
 const { viewMode: savedViewMode, filterType, dateFilter, sortBy, activeTab } = esteiraView;
 const viewMode = isAtLeastMd ? savedViewMode : 'list';
 const setViewMode = (v: 'kanban' | 'list') => setEsteiraView({ viewMode: v });
 const setFilterType = (v: 'Todos' | 'Venda' | 'Locação') => setEsteiraView({ filterType: v });
 const setDateFilter = (v: 'all' | 'today' | 'week') => setEsteiraView({ dateFilter: v });
 const setSortBy = (v: 'recent' | 'value-desc' | 'value-asc') => setEsteiraView({ sortBy: v });
 const setActiveTab = (v: 'Consolidada' | DealStage) => setEsteiraView({ activeTab: v });

 const [searchQuery, setSearchQuery] = useState('');
 const [selectedDealId, setSelectedDealId] = useState<string | null>(null);
 const [isModalOpen, setIsModalOpen] = useState(false);
 const [editingDealId, setEditingDealId] = useState<string | null>(null);
 const selectedDeal = useMemo(
  () => deals.find((deal) => deal.id === selectedDealId) ?? null,
  [deals, selectedDealId],
 );
 const editingDeal = useMemo(
  () => deals.find((deal) => deal.id === editingDealId) ?? null,
  [deals, editingDealId],
 );
 const setSelectedDeal = (deal: EsteiraDeal | null) => setSelectedDealId(deal?.id ?? null);
 const setEditingDeal = (deal: EsteiraDeal | null) => setEditingDealId(deal?.id ?? null);

 const [showFilters, setShowFilters] = useState(false);
 const [auditDealId, setAuditDealId] = useState<string | null>(null);

 const requestDelete = (id: string) => {
  const deal = deals.find((candidate) => candidate.id === id);
  const label = deal ? `${deal.property}${deal.client ? ` — ${deal.client}` : ''}` : 'este negócio';
  confirmAction(
   `Excluir definitivamente o negócio "${label}"? Esta ação não pode ser desfeita.`,
   () => deleteDeal(id),
   { title: 'Excluir negócio', confirmLabel: 'Excluir', variant: 'danger' },
  );
 };

 const filteredDeals = useMemo(() => {
 let result = deals.filter(c => {
 const matchSearch = c.client.toLowerCase().includes(searchQuery.toLowerCase()) || 
 c.property.toLowerCase().includes(searchQuery.toLowerCase()) ||
 c.id.toLowerCase().includes(searchQuery.toLowerCase());
 const matchType = filterType === 'Todos' || c.type === filterType;
 
 const matchDate = matchesDealDate(c.updated_at, dateFilter);

 return matchSearch && matchType && matchDate;
 });

 if (sortBy === 'value-desc') {
 result.sort((a, b) => b.value - a.value);
 } else if (sortBy === 'value-asc') {
 result.sort((a, b) => a.value - b.value);
 }

 return result;
 }, [deals, searchQuery, filterType, dateFilter, sortBy]);

 const formatCurrency = (val: number) => new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL", maximumFractionDigits: 0 }).format(val);

 const handleOpenEdit = (deal: EsteiraDeal) => {
 setEditingDeal(deal);
 setIsModalOpen(true);
 };

 const handleCreateNew = () => {
 setEditingDeal(null);
 setIsModalOpen(true);
 };

 return (
 <div className="flex flex-col h-full bg-muted overflow-hidden relative" aria-busy={dealsLoading}>
 {dealsError && <div role="alert" className="px-6 py-3 bg-card border-b border-destructive/30 flex items-center justify-between gap-3 text-sm">
 <p>{dealsError}</p><button type="button" onClick={() => void refreshDeals()} className="font-semibold underline">Tentar novamente</button>
 </div>}
 {dealsLoading && <p role="status" className="px-6 py-3 text-sm text-muted-foreground">Carregando negócios…</p>}
 <div className="flex-none px-6 py-4 lg:px-8 border-b border-border bg-card z-10">
 <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
 <div>
 <div className="flex items-center gap-2">
 <h1 className="text-2xl font-bold text-foreground tracking-tight font-sans">Esteira Digital</h1>
 <span
 className={clsx(
 "inline-flex items-center gap-1.5 text-micro font-medium px-2 py-0.5 rounded-full border",
 wsConnected
 ? "bg-status-ok-soft text-status-ok border-status-ok/20"
 : "bg-muted text-muted-foreground border-border"
 )}
 title={wsConnected ? "Atualizações em tempo real ativas" : "Sem conexão em tempo real — eventos chegarão após o reconnect"}
 >
 <span className={clsx("w-1.5 h-1.5 rounded-full", wsConnected ? "bg-status-ok animate-pulse" : "bg-muted-foreground")} />
 {wsConnected ? "ao vivo" : "offline"}
 </span>
 </div>
 <p className="text-sm text-muted-foreground mt-1">Gestão de propostas, due diligence e assinaturas.</p>
 </div>
 <div className="flex flex-col sm:flex-row items-center gap-3 w-full sm:w-auto">
 {isAtLeastMd && (
 <div className="flex items-center bg-card border border-border rounded-lg p-0.5 shadow-sm">
 <button
 onClick={() => setViewMode('kanban')}
 className={clsx("p-1.5 rounded-md transition-colors", savedViewMode === 'kanban' ? "bg-muted text-primary-600" : "text-muted-foreground hover:text-muted-foreground")}
 title="Visualização em Kanban"
 >
 <LayoutGrid className="w-4 h-4" />
 </button>
 <button
 onClick={() => setViewMode('list')}
 className={clsx("p-1.5 rounded-md transition-colors", savedViewMode === 'list' ? "bg-muted text-primary-600" : "text-muted-foreground hover:text-muted-foreground")}
 title="Visualização em Lista"
 >
 <List className="w-4 h-4" />
 </button>
 </div>
 )}
 
 <div className="relative w-full sm:w-64">
 <Search className="w-4 h-4 text-muted-foreground absolute left-3 top-1/2 -translate-y-1/2" />
 <input 
 type="text" 
 placeholder="Buscar negócio..." 
 value={searchQuery}
 onChange={(e) => setSearchQuery(e.target.value)}
 className="w-full pl-9 pr-4 py-2 bg-muted border-transparent rounded-lg text-sm focus:bg-card focus:border-primary-500 focus:ring-2 focus:ring-primary-200 outline-none transition-all"
 />
 </div>
 
 <div className="flex bg-muted p-1 rounded-lg w-full sm:w-auto">
 {(['Todos', 'Venda', 'Locação'] as const).map(type => (
 <button
 key={type}
 onClick={() => setFilterType(type)}
 className={clsx(
 "px-3 py-1.5 text-xs font-medium rounded-md flex-1 sm:flex-none transition-colors",
 filterType === type ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground"
 )}
 >
 {type}
 </button>
 ))}
 </div>

 <Button
 variant="outline"
 onClick={() => setShowFilters(!showFilters)}
 className={clsx(
 "shadow-sm whitespace-nowrap w-full sm:w-auto relative",
 (showFilters || dateFilter !== 'all' || sortBy !== 'recent') &&
 "bg-primary-50 text-primary-700 border-primary-200 hover:bg-primary-100 hover:text-primary-700"
 )}
 >
 <Filter className="w-4 h-4" />
 <span className="hidden sm:inline">Filtros</span>
 {(dateFilter !== 'all' || sortBy !== 'recent') && (
 <span className="absolute -top-1 -right-1 w-3 h-3 bg-primary-600 rounded-full border-2 border-card"></span>
 )}
 </Button>

 <Button
 onClick={handleCreateNew}
 className="whitespace-nowrap w-full sm:w-auto"
 >
 <Plus className="w-4 h-4" />
 <span className="hidden sm:inline">Nova Proposta</span>
 </Button>
 </div>
 </div>

 <AnimatePresence>
 {showFilters && (
 <motion.div 
 initial={{ height: 0, opacity: 0 }}
 animate={{ height: 'auto', opacity: 1 }}
 exit={{ height: 0, opacity: 0 }}
 className="overflow-hidden"
 >
 <div className="flex flex-col sm:flex-row gap-8 mt-4 pt-4 border-t border-border">
 <div className="space-y-2">
 <label className="text-xs font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-1.5">
 <Calendar className="w-3.5 h-3.5" /> Data de Atualização
 </label>
 <div className="flex flex-wrap gap-2">
 {[
 { id: 'all', label: 'Qualquer data' },
 { id: 'today', label: 'Hoje' },
 { id: 'week', label: 'Últimos 7 dias' }
 ].map(opt => (
 <button
 key={opt.id}
 onClick={() => setDateFilter(opt.id as any)}
 className={clsx(
 "px-3 py-1.5 text-xs font-medium rounded-lg border transition-all",
 dateFilter === opt.id 
 ? "bg-primary-50 border-primary-200 text-primary-700 shadow-sm" 
 : "bg-card border-border text-muted-foreground hover:bg-muted hover:text-foreground"
 )}
 >
 {opt.label}
 </button>
 ))}
 </div>
 </div>

 <div className="space-y-2">
 <label className="text-xs font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-1.5">
 <SlidersHorizontal className="w-3.5 h-3.5" /> Ordenar por
 </label>
 <div className="flex flex-wrap gap-2">
 {[
 { id: 'recent', label: 'Mais recentes', icon: Clock },
 { id: 'value-desc', label: 'Maior valor', icon: ArrowDown },
 { id: 'value-asc', label: 'Menor valor', icon: ArrowUp }
 ].map(opt => (
 <button
 key={opt.id}
 onClick={() => setSortBy(opt.id as any)}
 className={clsx(
 "flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border transition-all",
 sortBy === opt.id 
 ? "bg-primary-50 border-primary-200 text-primary-700 shadow-sm" 
 : "bg-card border-border text-muted-foreground hover:bg-muted hover:text-foreground"
 )}
 >
 <opt.icon className="w-3.5 h-3.5" />
 {opt.label}
 </button>
 ))}
 </div>
 </div>
 </div>
 </motion.div>
 )}
 </AnimatePresence>
 </div>

 <div className="flex-none px-6 lg:px-8 bg-card border-b border-border overflow-x-auto [&::-webkit-scrollbar]:hidden [-ms-overflow-style:none] [scrollbar-width:none]">
 <div className="flex gap-6 min-w-max">
 <button
 onClick={() => setActiveTab('Consolidada')}
 className={clsx(
 "py-3 text-sm font-medium border-b-2 transition-colors whitespace-nowrap",
 activeTab === 'Consolidada' ? "border-primary-600 text-primary-600" : "border-transparent text-muted-foreground hover:text-foreground hover:border-border"
 )}
 >
 Visão Consolidada
 </button>
 {ALL_STAGES.map(stage => (
 <button
 key={stage.id}
 onClick={() => setActiveTab(stage.id)}
 className={clsx(
 "py-3 text-sm font-medium flex items-center gap-2 border-b-2 transition-colors whitespace-nowrap",
 activeTab === stage.id ? "border-primary-600 text-primary-600" : "border-transparent text-muted-foreground hover:text-foreground hover:border-border"
 )}
 >
 {stage.icon && <stage.icon className="w-4 h-4" />}
 {stage.label}
 <span className={clsx(
 "px-2 py-0.5 rounded-full text-micro",
 activeTab === stage.id ? "bg-primary-100 text-primary-700" : "bg-muted text-muted-foreground"
 )}>
 {filteredDeals.filter(c => c.stage === stage.id).length}
 </span>
 </button>
 ))}
 </div>
 </div>

 {activeTab === 'Consolidada' ? (
 viewMode === 'kanban' ? (
 <div className="flex-1 overflow-x-auto p-6 lg:p-8">
 <div className="flex gap-6 h-full min-w-max pb-4">
 {ALL_STAGES.map((stage) => {
 const stageDeals = filteredDeals.filter(c => c.stage === stage.id);
 const totalValue = stageDeals.reduce((acc, curr) => acc + curr.value, 0);

 if (stage.id === 'Perdido' && stageDeals.length === 0) return null;

 return (
 <KanbanColumn 
 key={stage.id} 
 stage={stage} 
 deals={stageDeals} 
 totalValue={totalValue}
 formatCurrency={formatCurrency}
 moveDeal={moveDeal}
 movingDealIds={movingDealIds}
 onOpenDetails={(deal) => setSelectedDeal(deal)}
 onEdit={handleOpenEdit}
 onDelete={requestDelete}
 onAuditTrail={(id) => setAuditDealId(id)}
 />
 );
 })}
 </div>
 </div>
 ) : (
 <div className="flex-1 overflow-y-auto p-6 lg:p-8">
 <div className="bg-card border border-border rounded-2xl shadow-sm overflow-hidden">
 <table className="w-full text-left border-collapse min-w-[800px]">
 <thead>
 <tr className="bg-muted border-b border-border text-xs font-bold text-muted-foreground uppercase tracking-wider">
 <th className="px-6 py-4">Negócio</th>
 <th className="px-6 py-4">Cliente</th>
 <th className="px-6 py-4">Estágio</th>
 <th className="px-6 py-4">Valor</th>
 <th className="px-6 py-4 text-right">Ações</th>
 </tr>
 </thead>
 </table>
 {filteredDeals.length === 0 ? (
 <EmptyState
 icon={Search}
 title="Nenhum negócio encontrado"
 message="Tente ajustar os filtros ou os termos de busca."
 className="border-none bg-transparent my-12"
 />
 ) : (
 <table className="w-full text-left text-sm whitespace-nowrap">
 <tbody className="divide-y divide-border">
 {filteredDeals.map((deal) => {
 const stageInfo = ALL_STAGES.find(s => s.id === deal.stage);
 const StageIcon = stageInfo?.icon || ArrowRightLeft;

 return (
 <tr 
 key={deal.id} 
 className="hover:bg-muted/50 transition-colors cursor-pointer"
 onClick={() => setSelectedDeal(deal)}
 >
 <td className="px-6 py-4">
 <div className="flex items-center gap-3">
 <div className="w-10 h-10 rounded-xl bg-muted border border-border flex items-center justify-center flex-shrink-0">
 <FileText className="w-4 h-4 text-muted-foreground" />
 </div>
 <div>
 <div className="font-bold text-foreground">{deal.property}</div>
 <div className="flex items-center gap-2 mt-0.5">
 <span className="text-micro font-mono font-medium text-muted-foreground bg-muted px-1.5 py-0.5 rounded border border-border">
 {deal.displayId || deal.id.substring(0, 8)}
 </span>
 <span className={clsx(
 "text-micro font-bold px-1.5 py-0.5 rounded",
 deal.type === 'Venda' ? "bg-status-ok-soft text-status-ok" : "bg-primary-100 text-primary-700"
 )}>
 {deal.type}
 </span>
 </div>
 </div>
 </div>
 </td>
 <td className="px-6 py-4">
 <div className="flex items-center gap-3">
 {deal.avatarUrl ? (
 <img src={getMediaUrl(deal.avatarUrl)} alt={deal.client} className="w-8 h-8 rounded-full object-cover shrink-0" />
 ) : (
 <div className="w-8 h-8 rounded-full bg-primary-100 text-primary-700 flex items-center justify-center font-bold text-xs">
 {deal.client?.charAt(0).toUpperCase() || 'C'}
 </div>
 )}
 <div className="font-medium text-sm text-foreground">{deal.client}</div>
 </div>
 </td>
 <td className="px-6 py-4">
 <div className="flex flex-col gap-1.5 items-start">
 <div className={clsx("flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-xs font-bold w-fit", stageInfo?.bgClass, stageInfo?.colorClass)}>
 <StageIcon className="w-3.5 h-3.5" />
 {deal.stage}
 </div>
 <div className="flex items-center gap-1 text-micro text-muted-foreground font-medium">
 <Clock className="w-3 h-3" /> Atualizado {deal.date.toLowerCase()}
 </div>
 </div>
 </td>
 <td className="px-6 py-4">
 <div className="font-bold text-foreground">{deal.type === 'Locação' ? `${formatCurrency(deal.value)}/mês` : formatCurrency(deal.value)}</div>
 </td>
 <td className="px-6 py-4 text-right">
 <div className="flex items-center justify-end gap-2" onClick={(e) => e.stopPropagation()}>
 <Can permission="deals:write">
 <Button
 variant="ghost"
 size="icon"
 onClick={() => handleOpenEdit(deal)}
 className="size-9 text-muted-foreground hover:text-primary-600 hover:bg-primary-50"
 title="Editar Negócio"
 >
 <PenTool className="w-4 h-4" />
 </Button>
 </Can>
 <Can permission="system:view_audit_trail">
 <Button
 variant="ghost"
 size="icon"
 onClick={() => setAuditDealId(deal.id)}
 className="size-9 text-muted-foreground hover:text-violet-600 hover:bg-violet-50"
 title="Histórico de Alterações"
 >
 <History className="w-4 h-4" />
 </Button>
 </Can>
 {deal.stage !== 'Perdido' ? (
 <Can permission="deals:write">
 <Button
 variant="ghost"
 size="icon"
 onClick={() => moveDeal(deal.id, 'Perdido')}
 className="size-9 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft"
 title="Marcar como Perdido"
 >
 <XCircle className="w-4 h-4" />
 </Button>
 </Can>
 ) : (
 <Can permission="deals:delete">
 <Button
 variant="ghost"
 size="icon"
 onClick={() => requestDelete(deal.id)}
 className="size-9 text-status-critical hover:text-white hover:bg-status-critical"
 title="Excluir Definitivamente"
 >
 <XCircle className="w-4 h-4" />
 </Button>
 </Can>
 )}
 </div>
 </td>
 </tr>
 );
 })}
 </tbody>
 </table>
 )}
 </div>
 </div>
 )
 ) : (
 <StageDashboard 
 stage={ALL_STAGES.find(s => s.id === activeTab)!}
 deals={filteredDeals.filter(c => c.stage === activeTab)}
 formatCurrency={formatCurrency}
 onOpenDetails={(c) => setSelectedDeal(c)}
 onEdit={handleOpenEdit}
 onDelete={requestDelete}
 onAuditTrail={(id) => setAuditDealId(id)}
 moveDeal={moveDeal}
 />
 )}

 {selectedDeal && (
 <EsteiraDealDetails 
 contract={selectedDeal} 
 onClose={() => setSelectedDeal(null)} 
 />
 )}

 <EsteiraDealModal
  open={isModalOpen}
  onClose={() => setIsModalOpen(false)}
  dealToEdit={editingDeal}
 />

 {auditDealId && (
 <AuditTrailPanel
 entityId={auditDealId}
 entityLabel={`Negociacao ${auditDealId}`}
 entityType="deal"
 onClose={() => setAuditDealId(null)}
 />
 )}

      {confirmationDialog}
 </div>
 );
}

function KanbanColumn({ 
 stage, 
 deals, 
 totalValue,
 formatCurrency,
 moveDeal,
 movingDealIds,
 onOpenDetails,
 onEdit,
 onDelete,
 onAuditTrail
}: { 
 stage: typeof ALL_STAGES[0]; 
 deals: EsteiraDeal[]; 
 totalValue: number;
 formatCurrency: (val: number) => string;
 moveDeal: (id: string, stage: DealStage) => void;
 movingDealIds: Set<string>;
 onOpenDetails: (deal: EsteiraDeal) => void;
 onEdit: (deal: EsteiraDeal) => void;
 onDelete: (id: string) => void;
 onAuditTrail: (id: string) => void;
}) {
 const [{ isOver, canDrop }, dropRef] = useDrop({
 accept: ITEM_TYPE,
 canDrop: (item: { id: string; stage: DealStage }) =>
  item.stage !== stage.id && !movingDealIds.has(item.id),
 drop: (item: { id: string; stage: DealStage }, monitor) => {
  if (monitor.canDrop()) moveDeal(item.id, stage.id);
 },
 collect: (monitor) => ({
 isOver: monitor.isOver({ shallow: true }),
 canDrop: monitor.canDrop(),
 }),
 });

 return (
 <div 
 ref={dropRef as any}
 className={clsx(
 "flex flex-col bg-muted/50 rounded-2xl border w-80 flex-shrink-0 h-full max-h-full transition-colors",
 isOver && canDrop ? "border-primary-400 bg-primary-50/30" : "border-border",
 stage?.id === 'Perdido' ? "opacity-70" : ""
 )}
 >
 <div className="p-4 border-b border-border/60">
 <div className="flex items-center justify-between mb-2">
 <h3 className="font-bold text-foreground flex items-center gap-2 text-sm">
 <span className={clsx("p-1.5 rounded-lg", stage?.bgClass)}>
 {stage?.icon && <stage.icon className={clsx("w-4 h-4", stage?.colorClass)} />}
 </span>
 {stage?.label}
 </h3>
 <span className="bg-muted text-muted-foreground text-xs font-bold px-2 py-1 rounded-full">
 {deals.length}
 </span>
 </div>
 <div className="text-xs font-mono text-muted-foreground flex justify-between">
 <span>Vol. Negociado:</span>
 <span className="font-medium text-foreground">{formatCurrency(totalValue)}</span>
 </div>
 </div>
 
 <div className="flex-1 p-3 overflow-y-auto space-y-3 [&::-webkit-scrollbar]:hidden [-ms-overflow-style:none] [scrollbar-width:none]">
 <AnimatePresence>
 {deals.map(deal => (
 <DraggableCard 
 key={deal.id} 
 deal={deal} 
 formatCurrency={formatCurrency}
 isMoving={movingDealIds.has(deal.id)}
 onClick={() => onOpenDetails(deal)}
 onEdit={() => onEdit(deal)}
 onDelete={() => onDelete(deal.id)}
 onAuditTrail={() => onAuditTrail(deal.id)}
 moveDeal={moveDeal}
 />
 ))}
 </AnimatePresence>
 </div>
 </div>
 );
}

function DraggableCard({ deal, formatCurrency, isMoving, onClick, onEdit, onDelete, onAuditTrail, moveDeal }: { deal: EsteiraDeal; formatCurrency: (val: number) => string; isMoving?: boolean; onClick?: () => void, onEdit: () => void, onDelete: () => void, onAuditTrail?: () => void, moveDeal: (id: string, toStage: DealStage) => void }) {
 const [menuOpen, setMenuOpen] = useState(false);
 const menuRef = useRef<HTMLDivElement>(null);

 const [{ isDragging }, dragRef] = useDrag({
 type: ITEM_TYPE,
 item: { id: deal.id, stage: deal.stage },
 canDrag: !isMoving,
 collect: (monitor) => ({
 isDragging: monitor.isDragging(),
 }),
 });

 useEffect(() => {
 function handleClickOutside(event: MouseEvent) {
 if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
 setMenuOpen(false);
 }
 }
 document.addEventListener("mousedown", handleClickOutside);
 return () => document.removeEventListener("mousedown", handleClickOutside);
 }, []);

 const getStageActions = (stage: DealStage) => {
    switch(stage) {
      case 'Em Negociação':
        return [
          { label: 'Editar Negócio', icon: PenTool, color: 'text-primary-600', action: onEdit, permission: 'deals:write' },
          { label: 'Avançar para Proposta', icon: FileCheck, color: 'text-status-ok', action: () => moveDeal(deal.id, 'Proposta'), permission: 'deals:write' },
          { label: 'Marcar como Perdido', icon: XCircle, color: 'text-status-critical', action: () => moveDeal(deal.id, 'Perdido'), permission: 'deals:write' },
        ];
      case 'Proposta':
        return [
          { label: 'Editar Proposta', icon: PenTool, color: 'text-primary-600', action: onEdit, permission: 'deals:write' },
          { label: 'Avançar para Análise', icon: UploadCloud, color: 'text-status-warning', action: () => moveDeal(deal.id, 'Análise'), permission: 'deals:write' },
          { label: 'Voltar para Negociação', icon: ArrowRightLeft, color: 'text-muted-foreground', action: () => moveDeal(deal.id, 'Em Negociação'), permission: 'deals:write' },
          { label: 'Marcar como Perdido', icon: XCircle, color: 'text-status-critical', action: () => moveDeal(deal.id, 'Perdido'), permission: 'deals:write' },
        ];
      case 'Análise':
        return [
          { label: 'Editar Análise', icon: PenTool, color: 'text-primary-600', action: onEdit, permission: 'deals:write' },
          { label: 'Aprovar Análise', icon: PenTool, color: 'text-stage-contract', action: () => moveDeal(deal.id, 'Contrato'), permission: 'deals:approve' },
          { label: 'Reprovar Análise (Perdido)', icon: XCircle, color: 'text-status-critical', action: () => moveDeal(deal.id, 'Perdido'), permission: 'deals:approve' },
        ];
      case 'Contrato':
        return [
          { label: 'Editar Contrato', icon: PenTool, color: 'text-primary-600', action: onEdit, permission: 'deals:write' },
          { label: 'Concluir Negócio', icon: CheckCircle2, color: 'text-status-ok', action: () => moveDeal(deal.id, 'Concluído'), permission: 'deals:write' },
          { label: 'Cancelar (Perdido)', icon: XCircle, color: 'text-status-critical', action: () => moveDeal(deal.id, 'Perdido'), permission: 'deals:write' },
        ];
      case 'Concluído':
        return [
          { label: 'Editar Registro', icon: PenTool, color: 'text-primary-600', action: onEdit, permission: 'deals:write' },
          { label: 'Excluir Negócio', icon: XCircle, color: 'text-status-critical', action: onDelete, permission: 'deals:delete' },
        ];
      case 'Perdido':
        return [
          { label: 'Reativar Negócio', icon: ArrowRightLeft, color: 'text-primary-600', action: () => moveDeal(deal.id, 'Em Negociação'), permission: 'deals:write' },
          { label: 'Excluir Definitivamente', icon: XCircle, color: 'text-status-critical', action: onDelete, permission: 'deals:delete' },
        ];
      default:
        return [
          { label: 'Editar', icon: PenTool, color: 'text-primary-600', action: onEdit, permission: 'deals:write' },
          { label: 'Excluir', icon: XCircle, color: 'text-status-critical', action: onDelete, permission: 'deals:delete' },
        ];
    }
  };

 const actions = getStageActions(deal.stage);

 return (
 <motion.div
 ref={dragRef as any}
 layout
 onClick={onClick}
 initial={{ opacity: 0, scale: 0.95 }}
 animate={{ opacity: 1, scale: 1 }}
 exit={{ opacity: 0, scale: 0.95 }}
 whileHover={{ y: -2 }}
 className={clsx(
 "bg-card p-4 rounded-xl shadow-sm border cursor-grab active:cursor-grabbing group transition-all relative",
 isDragging ? "opacity-50 shadow-lg scale-105 rotate-2 z-50" : "opacity-100 hover:shadow-md",
 isMoving ? "opacity-70 pointer-events-none" : "",
 deal.hasCounterOffer && !isDragging
    ? "border-status-warning ring-2 ring-status-warning/30 motion-safe:animate-pulse motion-safe:[animation-duration:2.5s]"
   : "border-border hover:border-border"
)}
 >
 {isMoving && (
   <div className="absolute inset-0 bg-card/60 rounded-xl flex items-center justify-center z-10">
     <Loader2 className="w-5 h-5 text-primary animate-spin" />
   </div>
 )}
 <div className="flex justify-between items-start mb-3">
 <span className="text-micro font-mono font-medium text-muted-foreground bg-muted px-1.5 py-0.5 rounded border border-border">
 {deal.displayId || deal.id.substring(0, 8)}
 </span>
 <div className="relative" ref={menuRef}>
 <Button
 variant="ghost"
 size="icon"
 onClick={(e) => {
 e.stopPropagation();
 setMenuOpen(!menuOpen);
 }}
 className="size-7 text-muted-foreground"
 >
 <MoreHorizontal className="w-4 h-4" />
 </Button>
 
 {menuOpen && (
 <div className="absolute right-0 top-full mt-1 w-48 bg-card rounded-xl shadow-lg border border-border py-1.5 z-[100] overflow-hidden">
 <div className="px-3 py-1.5 border-b border-border mb-1">
 <span className="text-micro font-bold text-muted-foreground uppercase tracking-wider">Ações do Estágio</span>
 </div>
 {actions.map((action, idx) => (
 <Can key={idx} permission={action.permission as any}>
 <Button
 variant="ghost"
 size="sm"
 onClick={(e) => {
 e.stopPropagation();
 setMenuOpen(false);
 action.action();
 }}
 className="w-full justify-start font-normal text-xs"
 >
 <action.icon className={clsx("w-3.5 h-3.5", action.color)} />
 {action.label}
 </Button>
 </Can>
 ))}
 <div className="border-t border-border mt-1 pt-1">
 <Button
 variant="ghost"
 size="sm"
 onClick={(e) => {
 e.stopPropagation();
 setMenuOpen(false);
 onAuditTrail && onAuditTrail();
 }}
 className="w-full justify-start font-normal text-xs"
 >
 <History className="w-3.5 h-3.5 text-violet-600" />
 Histórico de Alterações
 </Button>
 </div>
 </div>
 )}
 </div>
 </div>

 {deal.hasCounterOffer && (
 <div className="mb-3 -mt-1 inline-flex items-center gap-1.5 px-2 py-1 rounded-md bg-status-warning-soft text-status-warning border border-status-warning/30 text-caption font-bold uppercase tracking-wide">
   <ArrowLeftRight className="w-3.5 h-3.5" aria-hidden="true" />
   <span>Contraproposta</span>
   {typeof deal.latestCounterValue === "number" && (
   <span className="font-mono tabular-nums normal-case font-medium">
     {formatCurrency(deal.latestCounterValue)}
   </span>
   )}
 </div>
 )}

 <div className="flex items-center gap-3 mb-3">
 {deal.avatarUrl ? (
 <img
 src={getMediaUrl(deal.avatarUrl)}
 alt={deal.client}
 className="w-8 h-8 rounded-full border border-border object-cover"
 />
 ) : (
 <div className="w-8 h-8 rounded-full bg-primary-100 text-primary-700 flex items-center justify-center font-bold text-xs border border-border">
 {deal.client?.charAt(0).toUpperCase() || 'C'}
 </div>
 )}
 <div className="flex-1 min-w-0">
 <h4 className="font-bold text-foreground text-sm truncate">{deal.client}</h4>
 <p className="text-xs text-muted-foreground truncate">{deal.property}</p>
 </div>
 </div>
 
 {deal.alerts && deal.alerts.length > 0 && (
 <div className="mb-3 space-y-1.5">
 {deal.alerts.map(alert => (
 <div key={alert} className="flex items-start gap-1.5 text-status-warning bg-status-warning-soft p-2 rounded-lg text-caption font-medium leading-tight">
 <AlertCircle className="w-3.5 h-3.5 flex-shrink-0 mt-0.5" /> 
 <span>{alert}</span>
 </div>
 ))}
 </div>
 )}

 <div className="flex items-center justify-between mt-4 pt-3 border-t border-border">
 <div>
 <span className={clsx(
 "text-micro font-bold px-2 py-0.5 rounded-full",
 deal.type === 'Venda' ? "bg-primary-50 text-primary-700" : "bg-status-ok-soft text-status-ok"
 )}>
 {deal.type}
 </span>
 <div className="font-bold text-foreground text-sm mt-1">{deal.type === 'Locação' ? `${formatCurrency(deal.value)}/mês` : formatCurrency(deal.value)}</div>
 </div>
 <div className="flex items-center text-muted-foreground text-xs font-medium bg-muted px-2 py-1 rounded-md">
 <Clock className="w-3 h-3 mr-1" /> {deal.date}
 </div>
 </div>
 </motion.div>
 );
}

function StageDashboard({ 
 stage, 
 deals, 
 formatCurrency, 
 onOpenDetails,
 onEdit,
 onDelete,
 onAuditTrail,
 moveDeal
}: { 
 stage: typeof ALL_STAGES[0]; 
 deals: EsteiraDeal[]; 
 formatCurrency: (val: number) => string; 
 onOpenDetails: (c: EsteiraDeal) => void;
 onEdit: (c: EsteiraDeal) => void;
 onDelete: (id: string) => void;
 onAuditTrail?: (id: string) => void;
 moveDeal: (id: string, stage: DealStage) => void;
}) {
 const totalValue = deals.reduce((acc, c) => acc + c.value, 0);

 return (
 <div className="flex-1 overflow-y-auto p-6 lg:p-8 bg-muted">
 <div className="max-w-7xl mx-auto">
 <div className="mb-8 bg-card p-6 rounded-2xl border border-border shadow-sm flex flex-col sm:flex-row items-start sm:items-center gap-6">
 <div className={clsx("w-16 h-16 rounded-2xl flex items-center justify-center flex-shrink-0", stage?.bgClass, stage?.colorClass)}>
 {stage?.icon && <stage.icon className="w-8 h-8" />}
 </div>
 <div>
 <h2 className="text-2xl font-bold text-foreground">{stage?.label}</h2>
 <p className="text-muted-foreground mt-1">
 <span className="font-medium text-foreground">{deals.length}</span> negócio{deals.length === 1 ? "" : "s"} nesta etapa, 
 totalizando <span className="font-medium text-foreground">{formatCurrency(totalValue)}</span>
 </p>
 </div>
 </div>
 
 {deals.length === 0 ? (
 <EmptyState
 icon={stage?.icon || FileText}
 title="Nenhum negócio"
 message={`Não há negócios na etapa ${stage?.label} no momento.`}
 className="border-2 border-dashed border-border bg-card/50 mt-8"
 />
 ) : (
 <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
 {deals.map(deal => (
 <DraggableCard 
 key={deal.id} 
 deal={deal} 
 formatCurrency={formatCurrency}
 onClick={() => onOpenDetails(deal)}
 onEdit={() => onEdit(deal)}
 onDelete={() => onDelete(deal.id)}
 onAuditTrail={onAuditTrail ? () => onAuditTrail(deal.id) : undefined}
 moveDeal={moveDeal}
 />
 ))}
 </div>
 )}
 </div>
 </div>
 );
}

export function Esteira() {
 return (
 <DndProvider backend={HTML5Backend}>
 <EsteiraBoard />
 </DndProvider>
 );
}
