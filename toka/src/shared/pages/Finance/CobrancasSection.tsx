import type { Dispatch, SetStateAction } from "react";
import { Fragment } from "react";
import type { NavigateFunction } from "react-router";
import {
 AlertCircle,
 Calendar,
 CheckCircle2,
 ChevronDown,
 Clock,
 CreditCard,
 DollarSign,
 Download,
 Eye,
 FileText,
 History,
 Info,
 MoreHorizontal,
 Pencil,
 Plus,
 Receipt,
 Search,
 Send,
 Settings,
 TrendingDown,
 XCircle,
 Zap,
} from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { clsx } from "clsx";

import type { Contract } from "../../contracts/types";
import type { Transaction } from "../../types/transaction";
import type { Charge } from "../../services/charge.service";
import type { ExportConfig } from "../../components/ExportReportModal";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "../../ui/dropdown-menu";
import { EmptyState } from "../../ui/EmptyState";
import { ResponsiveTable } from "../../ui/responsive-table";
import { PageState, PageStateSkeleton } from "../../ui/page-state";
import { SkeletonRow } from "../../components/finance/SkeletonRow";
import { Button } from "../../ui/button";
import { Can } from "../../../lib/permissions/Can";
import { availableChargeActions, mapChargeStatus } from "../../utils/charge";
import type { FilterAction } from "./state";
import { formatCurrency, StatusBadge } from "./presentation";
import { groupFinanceList, groupHeaderCell } from "./listGroups";
import { formatCompetencia, formatISODateBR } from "../../utils/format";
import {
 chargeReceivableAmounts,
 reconcileReceivableTransactions,
 transactionReceivableAmounts,
} from "../../lib/finance/economicRecords";
import { normalizeStatus } from "../../lib/lancamentos/status";
import type { FinanceAuditSelection, FinanceEditSelection } from "./sectionTypes";

type CobrancasSectionProps = {
 charges: Charge[];
 contracts: Contract[];
 transactions: Transaction[];
 focusMode: boolean;
 loadingInvoices: boolean;
 activeTab: "todas" | "cobrancas";
 cobrancaSearch: string;
 cobrancaStatusFilter: string;
 cobrancaStatusOpen: boolean;
 filtersDispatch: Dispatch<FilterAction>;
 navigate: NavigateFunction;
 /**
  * Registra o recebimento da linha. É o mesmo fluxo do detalhe do lançamento:
  * cobrança abre a baixa de cobrança (comprovante/destinos) e as demais origens
  * abrem a baixa padronizada de lançamento (método + data + comprovante +
  * observação) no painel lateral — nunca um AlertDialog que só troca o status.
  */
 onRegisterPayment: (tx: any, defaultAmount?: number) => void;
 setSelectedTransaction: Dispatch<SetStateAction<any>>;
 setEditing: Dispatch<SetStateAction<FinanceEditSelection | null>>;
 setShowNovaCobranca: Dispatch<SetStateAction<boolean>>;
 setSelectedChargeId: Dispatch<SetStateAction<string | null>>;
 setExportConfig: Dispatch<SetStateAction<ExportConfig | null>>;
 setAudit: Dispatch<SetStateAction<FinanceAuditSelection | null>>;
 handleChargeAction: (action: "pay" | "edit" | "cancel" | "notify" | "receipt", charge: Charge) => Promise<void>;
};

const chargeStatusPriority = (status: string): number => {
 const s = (status || '').toUpperCase();
 if (s === 'OVERDUE') return 0;      // crítico — em atraso
 if (s === 'PENDING') return 1;      // a vencer (mais atual / próximo)
 if (s === 'PARTIALLY_PAID') return 2;
 if (s === 'PAID') return 3;
 return 4;                            // CANCELLED / outros
};
const transactionStatusPriority = (status: string): number => {
 const canonical = normalizeStatus('transaction', status);
 if (canonical === 'vencido') return 0;
 if (canonical === 'pendente' || canonical === 'parcial') return 1;
 if (canonical === 'pago') return 3;
 return 2;
};

export function CobrancasSection({
 charges,
 contracts,
 transactions,
 focusMode,
 loadingInvoices,
 activeTab,
 cobrancaSearch,
 cobrancaStatusFilter,
 cobrancaStatusOpen,
 filtersDispatch,
 navigate,
 onRegisterPayment,
 setSelectedTransaction,
 setEditing,
 setShowNovaCobranca,
 setSelectedChargeId,
 setExportConfig,
 setAudit,
 handleChargeAction,
}: CobrancasSectionProps) {
 const handleTabChange = (tab: string) => {
  filtersDispatch({ type: "SET_COBRANCA_SUB_TAB", tab: tab as "todas" | "cobrancas" });
 };
 const reconciliation = reconcileReceivableTransactions(charges, transactions);
 const unresolvedProjectionIDs = new Set(reconciliation.unresolvedProjectionIDs);

 // Map charges from backend
 const chargesList = charges.map((ch: Charge) => {
 const contract = contracts.find(c => c.id === ch.contract_id);
 const mappedStatus = mapChargeStatus(ch.status);
 const amounts = chargeReceivableAmounts(ch);
 const filterStatus = mappedStatus === 'Cancelada' || mappedStatus === 'Reversada' ? 'Encerrado' : mappedStatus;
 return {
 id: ch.id,
 tipo: 'Cobrança',
 pessoa: contract?.client || ch.tenant_party_id || 'Inquilino',
 imovel: contract?.property_title || ch.property_id || '—',
 valor: ch.total_amount,
 data: formatISODateBR(ch.due_date || ch.created_at),
 recebidoEm: formatISODateBR(ch.paid_at),
 status: mappedStatus,
 categoria: ({ SALE: 'Venda', RENT: 'Locação', UTILITY: 'Encargos', MAINTENANCE: 'Manutenção/Vistoria', OTHER: 'Outros' } as Record<string, string>)[ch.type] || ch.type || 'Aluguel/Venda',
 desc: ch.description || `Cobrança ${formatCompetencia(ch.reference_month)}`,
 contractId: ch.contract_id,
 _source: 'charge' as const,
 _chargeId: ch.id,
 _sortDate: ch.due_date || ch.created_at,
 _priority: chargeStatusPriority(ch.status),
 _filterStatus: filterStatus,
 _includedInTotals: true,
 _paidAmount: amounts.paid,
 _pendingAmount: amounts.pending,
 _lateAmount: amounts.overdue,
 };
 });

 // Mantém somente recebíveis independentes. Faturas e espelhos ligados a uma
 // Charge são projeções do mesmo fato econômico e não viram uma segunda linha.
 const txList = reconciliation.transactions.map(tx => {
 const contract = contracts.find(c => c.id === tx.contractId);
 const isMaintenance = tx.description.includes('OS ') || tx.description.includes('Manutenção') || tx.description.includes('Serviço de');
 const isUnresolvedProjection = unresolvedProjectionIDs.has(tx.id);
 const amounts = isUnresolvedProjection ? { paid: 0, pending: 0, overdue: 0 } : transactionReceivableAmounts(tx);
 const canonicalStatus = normalizeStatus('transaction', tx.status);
 const filterStatus = canonicalStatus === 'cancelado' || canonicalStatus === 'revertido' ? 'Encerrado' : tx.status;

 return {
 id: tx.id,
 tipo: isUnresolvedProjection ? 'Documento' : 'Receita',
 pessoa: contract?.client || 'Sistema',
 imovel: contract?.property_title || tx.contractId || 'N/A',
 valor: tx.value,
 data: formatISODateBR(tx.dueDate || tx.date),
 recebidoEm: formatISODateBR(tx.paymentDate),
 status: tx.status,
 categoria: isUnresolvedProjection ? 'Vínculo a apurar' : (isMaintenance ? 'Manutenção/Vistoria' : 'Aluguel/Venda'),
 desc: tx.description,
 contractId: tx.contractId,
 _source: tx.recordSource === 'invoice' ? 'invoice' as const : 'transaction' as const,
 _chargeId: null as string | null,
 _sortDate: tx.date,
 _priority: transactionStatusPriority(tx.status),
 _filterStatus: filterStatus,
 _includedInTotals: !isUnresolvedProjection,
 _paidAmount: amounts.paid,
 _pendingAmount: amounts.pending,
 _lateAmount: amounts.overdue,
 };
 });

 // Ordena por prioridade (status) e, dentro da mesma faixa, por vencimento
 // mais próximo. Itens em atraso/a vencer (atuais) sobem; pagas e outras
 // competências descem.
 const transactionsList = [...chargesList, ...txList].sort((a, b) => {
  if (a._priority !== b._priority) return a._priority - b._priority;
  return (a._sortDate || '').localeCompare(b._sortDate || '');
 });

 // Apply filters
 const filteredList = transactionsList.filter(t => {
 const matchesSearch = !cobrancaSearch ||
 t.pessoa.toLowerCase().includes(cobrancaSearch.toLowerCase()) ||
 t.imovel.toLowerCase().includes(cobrancaSearch.toLowerCase()) ||
 t.desc.toLowerCase().includes(cobrancaSearch.toLowerCase()) ||
 t.id.toLowerCase().includes(cobrancaSearch.toLowerCase());
 const matchesStatus = cobrancaStatusFilter === 'Todos' || t._filterStatus === cobrancaStatusFilter;
 const matchesTab = activeTab === 'cobrancas' ? t.categoria === 'Manutenção/Vistoria' : true;
 return matchesSearch && matchesStatus && matchesTab;
 });

 // Dados agrupados por urgência (Em atraso / A vencer / Concluídas)
 const grouped = groupFinanceList(filteredList, (tx) => tx.status);

 // Cada linha carrega a decomposição do seu próprio valor. Assim uma cobrança
 // parcial contribui simultaneamente para "Pagas" e "Pendentes", sem duplicar
 // o valor total do fato econômico.
 const pendingTotal = filteredList.reduce((sum, item) => sum + item._pendingAmount, 0);
 const paidTotal = filteredList.reduce((sum, item) => sum + item._paidAmount, 0);
 const lateTotal = filteredList.reduce((sum, item) => sum + item._lateAmount, 0);
 const allTotal = pendingTotal + paidTotal + lateTotal;

 const dynamicStats = [
 { label: "Total consolidado", value: formatCurrency(allTotal), icon: TrendingDown, iconBg: "bg-muted", iconColor: "text-muted-foreground" },
 { label: "Pagas", value: formatCurrency(paidTotal), icon: CheckCircle2, iconBg: "bg-status-ok-soft", iconColor: "text-status-ok" },
 { label: "Pendentes", value: formatCurrency(pendingTotal), icon: Clock, iconBg: "bg-status-warning-soft", iconColor: "text-status-warning" },
 { label: "Em Atraso", value: formatCurrency(lateTotal), icon: AlertCircle, iconBg: "bg-status-critical-soft", iconColor: "text-status-critical" },
 ];

 return (
 <div className="space-y-8 animate-in fade-in duration-300">
 <AnimatePresence>
 {!focusMode && (
 <motion.div
 initial={{ opacity: 0, height: 0, marginBottom: 0 }}
 animate={{ opacity: 1, height: "auto", marginBottom: 24 }}
 exit={{ opacity: 0, height: 0, marginBottom: 0, overflow: 'hidden' }}
 transition={{ duration: 0.3, ease: "easeInOut" }}
 >
 {/* Tab Flow Description */}
 <div className="bg-primary-50/50 border border-primary-200 rounded-2xl p-5">
 <div className="flex items-start gap-4">
 <div className="p-2.5 bg-primary-100 rounded-xl shrink-0">
 <CreditCard className="w-5 h-5 text-primary-600" />
 </div>
 <div>
 <h3 className="text-base font-bold text-foreground mb-1">Gestão de Cobranças</h3>
 <p className="text-sm text-muted-foreground">
 Acompanhe e gerencie todos os recebimentos. Crie novas cobranças manuais para inquilinos,<br/>
 monitore os pagamentos e registre a confirmação do valor recebido.
 </p>
 </div>
 </div>
 </div>
 </motion.div>
 )}
 </AnimatePresence>

 {(reconciliation.consolidatedTransactionIDs.length > 0 || reconciliation.unresolvedProjectionIDs.length > 0) && (
  <div className="flex flex-wrap items-start gap-2 rounded-xl border border-status-info/30 bg-status-info-soft/50 px-4 py-3 text-sm text-muted-foreground" role="status">
   <Info className="mt-0.5 h-4 w-4 shrink-0 text-status-info" />
   <div>
    {reconciliation.consolidatedTransactionIDs.length > 0 && (
     <p><strong className="text-foreground">{reconciliation.consolidatedTransactionIDs.length}</strong> projeção(ões) vinculada(s) foram consolidadas na cobrança canônica e não entram novamente nos totais.</p>
    )}
    {reconciliation.unresolvedProjectionIDs.length > 0 && (
     <p><strong className="text-foreground">{reconciliation.unresolvedProjectionIDs.length}</strong> documento(s) aparecem como <strong className="text-foreground">A apurar</strong> e ficam fora do total consolidado até a cobrança vinculada estar disponível.</p>
    )}
   </div>
  </div>
 )}

 <AnimatePresence>
 {!focusMode && (
 <motion.div
 initial={{ opacity: 0, height: 0 }}
 animate={{ opacity: 1, height: "auto" }}
 exit={{ opacity: 0, height: 0, overflow: 'hidden' }}
 transition={{ duration: 0.3, ease: "easeInOut" }}
 >
 <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 mb-6">
 <div>
 <h2 className="text-2xl font-bold text-foreground">Cobranças e Transações</h2>
 <p className="text-sm text-muted-foreground mt-1">Visão geral do faturamento, incluindo cobranças de manutenções</p>
 </div>
 <div className="flex flex-wrap items-center gap-3">
            <Can permission="finance:export">
              <Button
 variant="outline"
 onClick={() => setExportConfig({
 title: "Relatório de Cobranças e Transações",
 subtitle: "Módulo Financeiro — Suíte",
 filterDescription: [cobrancaSearch && `Busca: "${cobrancaSearch}"`, cobrancaStatusFilter !== 'Todos' && `Status: ${cobrancaStatusFilter}`, activeTab !== 'todas' && 'Apenas Manutenções'].filter(Boolean).join(' | ') || undefined,
 columns: [
 { key: 'id', label: 'ID' },
 { key: 'pessoa', label: 'Pessoa / Contrato' },
 { key: 'imovel', label: 'Imóvel' },
 { key: 'categoria', label: 'Categoria' },
 { key: 'desc', label: 'Descrição' },
 { key: 'valor', label: 'Valor (R$)', align: 'right', format: (v: number) => formatCurrency(v) },
 { key: 'data', label: 'Vencimento' },
 { key: 'recebidoEm', label: 'Recebido em' },
 { key: 'status', label: 'Status' },
 ],
 data: filteredList,
 dateKey: 'data',
 summaryRows: [
 { label: 'Total', value: formatCurrency(allTotal) },
 { label: 'Pagas', value: formatCurrency(paidTotal), color: '#059669' },
 { label: 'Pendentes', value: formatCurrency(pendingTotal), color: '#d97706' },
 { label: 'Em Atraso', value: formatCurrency(lateTotal), color: '#e11d48' },
 ],
 recalcSummary: (d) => {
 const trusted = d.filter((t: any) => t._includedInTotals !== false);
 const paid = trusted.reduce((a: number, c: any) => a + (c._paidAmount || 0), 0);
 const pending = trusted.reduce((a: number, c: any) => a + (c._pendingAmount || 0), 0);
 const late = trusted.reduce((a: number, c: any) => a + (c._lateAmount || 0), 0);
 return [
 { label: 'Total', value: formatCurrency(paid + pending + late) },
 { label: 'Pagas', value: formatCurrency(paid), color: '#059669' },
 { label: 'Pendentes', value: formatCurrency(pending), color: '#d97706' },
 { label: 'Em Atraso', value: formatCurrency(late), color: '#e11d48' },
 ];
 },
 })}
              >
                <Download className="w-4 h-4" /> Exportar
              </Button>
            </Can>
            <Can permission="system:view_audit_trail">
              <Button
                variant="outline"
                onClick={() => setAudit({ entityId: '__all__', label: 'Todas as Transacoes' })}
              >
                <History className="w-4 h-4" /> Histórico
              </Button>
            </Can>
            <Can permission="charges:write">
              <Button variant="outline" onClick={() => navigate('/financeiro/regua')}>
                <Settings className="w-4 h-4" /> Régua
              </Button>
            </Can>
            <Can permission="finance:write">
              <Button
                onClick={() => setShowNovaCobranca(true)}
              >
                <Plus className="w-4 h-4" /> Nova Cobrança
              </Button>
            </Can>
          </div>
        </div>
 </motion.div>
 )}
 </AnimatePresence>

 <AnimatePresence>
 {!focusMode && (
 <motion.div
 initial={{ opacity: 0, height: 0, marginBottom: 0 }}
 animate={{ opacity: 1, height: "auto", marginBottom: 24 }}
 exit={{ opacity: 0, height: 0, marginBottom: 0, overflow: 'hidden' }}
 transition={{ duration: 0.3, ease: "easeInOut" }}
 >
 <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
 {dynamicStats.map((stat, idx) => (
 <div key={idx} className="bg-card p-6 rounded-2xl border border-border shadow-sm flex items-center gap-5 hover:border-primary-200 transition-colors min-w-0">
 <div className={`w-12 h-12 shrink-0 rounded-xl flex items-center justify-center border border-border ${stat.iconBg}`}>
 <stat.icon className={`w-6 h-6 ${stat.iconColor}`} />
 </div>
 <div className="min-w-0 flex-1">
 <p className="text-sm font-medium text-muted-foreground mb-1 truncate">{stat.label}</p>
 <h3 className="text-3xl font-bold text-foreground leading-tight break-words">{stat.value}</h3>
 </div>
 </div>
 ))}
 </div>
 </motion.div>
 )}
 </AnimatePresence>

 <div className="bg-card rounded-2xl border border-border shadow-sm flex flex-col overflow-hidden">
 <div className="p-5 border-b border-border flex flex-col lg:flex-row justify-between lg:items-center gap-4">
 <div>
 <h2 className="text-lg font-bold text-foreground mb-3">Lista de Movimentações</h2>
 <div className="flex items-center gap-4 text-sm text-muted-foreground">
 <div className="flex items-center gap-2 relative">
 <span>Status:</span>
 <Button
 variant="outline"
 size="sm"
 onClick={() => filtersDispatch({ type: 'TOGGLE_TAB_STATUS_OPEN', tab: 'cobranca' })}
 className="font-medium"
 >
 {cobrancaStatusFilter} <ChevronDown className="w-4 h-4 text-muted-foreground" />
 </Button>
 {cobrancaStatusOpen && (
 <div className="absolute top-full left-0 mt-1 bg-card border border-border rounded-xl shadow-lg z-20 min-w-[140px] py-1">
 {['Todos', 'Pendente', 'Parcial', 'Pago', 'Em Atraso', 'Encerrado'].map(s => (
 <Button
 key={s}
 variant="ghost"
 size="sm"
 onClick={() => filtersDispatch({ type: 'SET_TAB_STATUS', tab: 'cobranca', status: s })}
 className={clsx("w-full justify-start font-normal", cobrancaStatusFilter === s ? "font-bold text-primary-600 bg-primary-50" : "text-foreground")}
 >
 {s}
 </Button>
 ))}
 </div>
 )}
 </div>
 </div>
 </div>

 <div className="flex flex-col items-end gap-2 w-full lg:w-auto">
 <div className="relative w-full max-w-xs">
 <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
 <input
 type="text"
 value={cobrancaSearch}
 onChange={e => filtersDispatch({ type: 'SET_TAB_SEARCH', tab: 'cobranca', search: e.target.value })}
 placeholder="Buscar pessoa, imóvel, descrição..."
 className="w-full pl-9 pr-4 py-2 border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
 />
 </div>
 <span className="text-xs text-muted-foreground">{filteredList.length} de {transactionsList.length} movimentações</span>
 </div>
 </div>

 <div className="p-5">
 <div className="bg-primary-50 border border-primary-100 rounded-xl p-3 flex items-center gap-3 mb-5">
 <Info className="w-5 h-5 text-primary-600" />
 <p className="text-sm font-medium text-primary-700">
 As cobranças geradas nas <strong>Manutenções e Vistorias</strong> já estão refletidas aqui de forma unificada.
 </p>
 </div>

 <div className="flex bg-muted/50 p-1 rounded-xl mb-5 border border-border">
 <button onClick={() => handleTabChange('todas')} className={clsx("flex-1 text-sm font-bold py-2.5 rounded-lg transition-colors", activeTab === 'todas' ? 'bg-card text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground')}>
 Todas Transações
 </button>
 <button onClick={() => handleTabChange('cobrancas')} className={clsx("flex-1 flex items-center justify-center gap-2 text-sm font-bold py-2.5 rounded-lg transition-colors", activeTab === 'cobrancas' ? 'bg-card text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground')}>
 <FileText className="w-4 h-4" /> Apenas Manutenções
 </button>
 </div>

 <div className="overflow-x-auto">
 {loadingInvoices && filteredList.length === 0 ? (
 <PageState loading skeleton={<PageStateSkeleton rows={5} />}>{null}</PageState>
 ) : (
 <ResponsiveTable
 data={[]}
 groups={grouped.map((group) => ({
 key: group.key,
 label: <span className="flex items-center gap-2">{group.label}<span className="font-medium normal-case tracking-normal text-muted-foreground">({group.items.length})</span></span>,
 data: group.items,
 }))}
 caption="Cobranças e recebimentos"
 getRowKey={(tx) => tx.id}
 empty={(
 <EmptyState
 icon={Search}
 message={cobrancaSearch || cobrancaStatusFilter !== "Todos" ? "Nenhuma cobrança encontrada para os filtros selecionados." : "Nenhuma cobrança encontrada."}
 className="border-none bg-transparent my-6"
 />
 )}
 columns={[
 { key: "pessoa", header: "Pessoa / Contrato", primary: true, cell: (tx) => <span className="font-bold">{tx.pessoa}</span> },
 { key: "imovel", header: "Imóvel", cell: (tx) => <span className="text-muted-foreground">{tx.imovel}</span> },
 {
 key: "categoria",
 header: "Categoria / Descrição",
 cell: (tx) => (
 <div className="flex flex-col gap-1">
 <span className={clsx("inline-flex items-center w-fit gap-1.5 px-2 py-0.5 rounded text-micro font-bold border", tx.categoria === 'Manutenção/Vistoria' ? 'bg-primary-50 text-primary-700 border-primary-200' : 'bg-muted text-muted-foreground border-border')}>
 <Zap className="w-3 h-3" /> {tx.categoria}
 </span>
 <span className="text-xs text-muted-foreground truncate max-w-[200px]" title={tx.desc}>{tx.desc}</span>
 </div>
 ),
 },
 { key: "valor", header: "Valor", cell: (tx) => <span className="font-bold">{formatCurrency(tx.valor)}</span> },
 {
 key: "data",
 header: "Vencimento / Recebimento",
 cell: (tx) => (
 <div className="text-muted-foreground">
 <div className="flex items-center gap-1.5"><Calendar className="w-3.5 h-3.5" /> {tx.data}</div>
 {tx.recebidoEm && <div className="mt-1 text-xs">Recebido em {tx.recebidoEm}</div>}
 </div>
 ),
 },
 { key: "status", header: "Status", cell: (tx) => <StatusBadge status={tx.status} /> },
 {
 key: "actions",
 header: "Ações",
 className: "text-right",
 cell: (tx) => (
  <div className="flex items-center justify-end gap-2 relative">
  {tx._includedInTotals && (tx.status === 'Pendente' || tx.status === 'Parcial') && (
  <Button
  variant="outline"
  size="sm"
  className="text-status-ok hover:text-status-ok hover:bg-status-ok-soft border-status-ok/20"
  onClick={() => {
  // Baixa é formulário (valor/destinos ou método + data + comprovante),
  // resolvida pelo mesmo handler do detalhe do lançamento: cobrança abre a
  // baixa de cobrança e as demais origens, a baixa padronizada de lançamento.
  onRegisterPayment(tx, tx._pendingAmount + tx._lateAmount);
  }}
  >
  <CheckCircle2 className="w-3.5 h-3.5" /> Baixar
  </Button>
  )}
  {tx._source === 'transaction' && (
  <Button
  variant="ghost"
  size="sm"
  onClick={() => {
  const originalTx = transactions.find(t => t.id === tx.id);
  if (originalTx) setEditing({ tx: originalTx, mode: 'cobranca' });
  }}
  className="text-muted-foreground hover:text-primary-600 hover:bg-primary-50"
  >
  <Pencil className="w-3.5 h-3.5" /> Editar
  </Button>
  )}
  <Button
    variant="ghost"
    size="sm"
    onClick={() => {
      // Para charges, abre o drawer especializado (audit/line items/ações
      // condicionais por status). Caso contrário, mantém o painel legado
      // de Transaction, que ainda atende outras origens.
      if (tx._source === 'charge' && tx._chargeId) {
        setSelectedChargeId(tx._chargeId as string);
      } else {
        setSelectedTransaction(tx);
      }
    }}
    className="text-muted-foreground hover:text-primary-600 hover:bg-primary-50"
  >
    <Eye className="w-4 h-4" /> Detalhes
  </Button>
  {tx._source === 'charge' && tx._chargeId && (() => {
    const liveCharge = charges.find((c: Charge) => c.id === tx._chargeId);
    if (!liveCharge) return null;
    const acts = availableChargeActions(liveCharge.status);
    if (acts.length === 0) return null;
    return (
      <DropdownMenu>
        <DropdownMenuTrigger
          type="button"
          className="inline-flex items-center justify-center h-8 w-8 rounded-md text-muted-foreground hover:bg-muted hover:text-foreground transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
          aria-label="Mais ações"
          title="Mais ações"
        >
          <MoreHorizontal className="w-4 h-4" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-52">
          {acts.includes('pay') && (
            <DropdownMenuItem onSelect={() => handleChargeAction('pay', liveCharge)}>
              <DollarSign className="w-4 h-4 mr-2" /> Marcar Pago
            </DropdownMenuItem>
          )}
          {acts.includes('edit') && (
            <DropdownMenuItem onSelect={() => handleChargeAction('edit', liveCharge)}>
              <Pencil className="w-4 h-4 mr-2" /> Editar
            </DropdownMenuItem>
          )}
          {acts.includes('notify') && (
            <DropdownMenuItem onSelect={() => handleChargeAction('notify', liveCharge)}>
              <Send className="w-4 h-4 mr-2" /> Notificar
            </DropdownMenuItem>
          )}
          {acts.includes('receipt') && (
            <DropdownMenuItem onSelect={() => handleChargeAction('receipt', liveCharge)}>
              <Receipt className="w-4 h-4 mr-2" /> Recibo
            </DropdownMenuItem>
          )}
          {acts.includes('cancel') && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onSelect={() => handleChargeAction('cancel', liveCharge)}
                className="text-status-critical focus:bg-status-critical-soft focus:text-status-critical"
              >
                <XCircle className="w-4 h-4 mr-2" /> Cancelar Cobrança
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    );
  })()}
  <Button
  variant="ghost"
  size="icon"
  onClick={() => setAudit({ entityId: tx.id, label: tx.desc })}
  className="size-8 text-muted-foreground"
  title="Histórico de alterações"
  >
  <History className="w-3.5 h-3.5" />
  </Button>
  </div>

 ),
 },
 ]}
 />
 )}
 {!loadingInvoices && filteredList.length === 0 && (
 <EmptyState
 icon={Search}
 message={cobrancaSearch || cobrancaStatusFilter !== 'Todos' ? 'Nenhuma cobrança encontrada para os filtros selecionados.' : 'Nenhuma cobrança encontrada.'}
 className="border-none bg-transparent my-6"
 />
 )}
 </div>
 </div>
 </div>
 </div>
 );
}
