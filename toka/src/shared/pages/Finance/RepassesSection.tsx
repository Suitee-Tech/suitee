import type { Dispatch, SetStateAction } from "react";
import { Fragment } from "react";
import {
 ChevronDown,
 Download,
 Filter,
 Hand,
 History,
 Info,
 Search,
} from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { clsx } from "clsx";
import { toast } from "sonner";

import type { Contract } from "../../contracts/types";
import type { Transaction } from "../../types/transaction";
import type { OwnerTransfer } from "../../services/transfer.service";
import type { ExportConfig } from "../../components/ExportReportModal";
import { EmptyState } from "../../ui/EmptyState";
import { ResponsiveTable } from "../../ui/responsive-table";
import { PageState, PageStateSkeleton } from "../../ui/page-state";
import { SkeletonRow } from "../../components/finance/SkeletonRow";
import { Button } from "../../ui/button";
import type { FilterAction } from "./state";
import { formatCurrency } from "./presentation";
import { formatCompetencia, formatISODateBR } from "../../utils/format";
import { groupFinanceList, groupHeaderCell } from "./listGroups";
import { reconcileTransferTransactions } from "../../lib/finance/economicRecords";
import { normalizeStatus } from "../../lib/lancamentos/status";
import { getOwnerTransferOrigin, getOwnerTransferPresentation } from "../../lib/finance/ownerTransferFlow";
import { Can } from "../../permissions/Can";
import type { FinanceAuditSelection, FinanceConfirmOptions } from "./sectionTypes";

type RepassesSectionProps = {
 transfers: OwnerTransfer[];
 contracts: Contract[];
 transactions: Transaction[];
 focusMode: boolean;
 loadingInvoices: boolean;
 repasseSearch: string;
 repasseStatusFilter: string;
 repasseStatusOpen: boolean;
 filtersDispatch: Dispatch<FilterAction>;
 approveTransfer: (id: string) => Promise<void>;
 processTransfer: (id: string, evidence?: { proof_url?: string; note?: string }) => Promise<void>;
 openProcessRepasse: (transferId: string, transfer: OwnerTransfer) => void;
 setSelectedTransaction: Dispatch<SetStateAction<any>>;
 setExportConfig: Dispatch<SetStateAction<ExportConfig | null>>;
 setAudit: Dispatch<SetStateAction<FinanceAuditSelection | null>>;
 openConfirm: (options: FinanceConfirmOptions) => void;
 closeConfirm: () => void;
};

const repasseStatusPriority = (rawStatus: string): number => {
 const s = String(rawStatus).toUpperCase();
 if (s === 'PENDING') return 0;      // precisa aprovação — mais atual
 if (s === 'APPROVED') return 1;     // aprovada — aguarda processamento
 if (s === 'PROCESSED' || s === 'COMPLETED') return 2; // concluída
 return 3;
};
const legacyRepassePriority = (status: string): number => {
 const canonical = normalizeStatus('transaction', status);
 if (canonical === 'pago') return 2;
 if (canonical === 'cancelado' || canonical === 'revertido') return 3;
 return 0;
};

export function RepassesSection({
 transfers,
 contracts,
 transactions,
 focusMode,
 loadingInvoices,
 repasseSearch,
 repasseStatusFilter,
 repasseStatusOpen,
 filtersDispatch,
 approveTransfer,
 processTransfer,
 openProcessRepasse,
 setSelectedTransaction,
 setExportConfig,
 setAudit,
 openConfirm,
 closeConfirm,
}: RepassesSectionProps) {
 const reconciliation = reconcileTransferTransactions(transfers, transactions);

 // Map real transfers from backend
 const transfersList = transfers.map((tr: OwnerTransfer) => {
 const contract = contracts.find(c => c.id === tr.contract_id);
 const presentation = getOwnerTransferPresentation(tr);
 const origin = getOwnerTransferOrigin(tr, contract?.type);
 const mappedStatus = presentation.status === 'Processado'
  ? 'Concluído'
  : presentation.status === 'Cancelado'
   ? 'Encerrado'
   : presentation.status;
 return {
 id: tr.id,
 type: 'repasse',
 categoria: 'Repasse',
 prop: contract?.client || tr.owner_id || 'Proprietário',
 imovel: contract?.property_title || tr.property_id || 'Imóvel',
 modalidade: origin.label,
 origem: origin.source,
 geracao: origin.automatic ? 'Gerado pelo recebimento' : 'Origem a apurar',
 _flow: origin.flow,
 _automatic: origin.automatic,
 periodo: formatCompetencia(tr.reference_month),
 aluguel: tr.gross_rent,
 taxa: tr.economic_breakdown
   ? tr.economic_breakdown.deductions_cents / 100
   : tr.admin_fee_amount + tr.maintenance_deductions + tr.other_deductions,
 liquido: tr.net_amount,
 data: tr.processed_at ? formatISODateBR(tr.processed_at) : (mappedStatus === 'Concluído' ? formatISODateBR(tr.updated_at) : '-'),
 _beneficiaryLabel: contract?.client || tr.owner_id || 'Proprietário não identificado',
 _beneficiaryKind: 'Proprietário',
 _expectedDueAt: undefined,
 _settledAt: tr.processed_at,
 sortDate: tr.created_at.split('T')[0],
 status: mappedStatus,
 _transferId: tr.id,
 _rawStatus: tr.status,
 _sortMonth: tr.reference_month,
 _priority: repasseStatusPriority(tr.status),
 _includedInTotals: tr.net_amount <= 0 || presentation.settlementReady,
 _settlementAction: presentation.action,
 _legacy: false,
 // Allocation-aware (doc §8/§11): reservado vs disponível no momento do repasse.
 _ownerBalance: tr.owner_balance,
 _economicBreakdown: tr.economic_breakdown,
 };
 });

 // Also include legacy transaction-based repasses not in transfers
 const legacyRepasses = reconciliation.transactions
 .map(tx => {
 const contract = contracts.find(c => c.id === tx.contractId);
 const valorLiquido = Math.abs(tx.value);
 const valorBruto = (tx as any).grossValue || (valorLiquido / 0.9);
 const taxa = (tx as any).adminFee || (valorBruto - valorLiquido);
 const canonicalStatus = normalizeStatus('transaction', tx.status);
 const legacyStatus = 'A apurar';

 return {
 id: tx.id,
 type: 'repasse',
 categoria: 'Repasse legado',
 prop: contract?.client || 'Proprietário não identificado',
 imovel: contract?.property_title || 'Imóvel avulso',
 modalidade: getOwnerTransferOrigin({}, contract?.type).label,
 origem: 'Origem a apurar',
 geracao: 'Legado',
 _flow: getOwnerTransferOrigin({}, contract?.type).flow,
 _automatic: false,
 periodo: formatCompetencia((tx as any).month || tx.date.substring(0, 7)),
 aluguel: valorBruto,
 taxa: taxa,
 liquido: valorLiquido,
 data: canonicalStatus === 'pago' ? formatISODateBR(((tx as any).paymentDate || tx.date)) : '-',
 _beneficiaryLabel: contract?.client || 'Proprietário não identificado',
 _beneficiaryKind: 'Proprietário',
 _expectedDueAt: (tx as any).dueDate || (tx as any).due_date,
 _settledAt: canonicalStatus === 'pago' ? ((tx as any).paymentDate || tx.date) : undefined,
 sortDate: tx.date,
 status: legacyStatus,
 _transferId: null as string | null,
 _ownerBalance: undefined,
 _economicBreakdown: undefined,
 _rawStatus: tx.status,

  _sortMonth: (tx as any).month || tx.date.substring(0, 7),
  _priority: legacyRepassePriority(tx.status),
 _includedInTotals: false,
  _settlementAction: null,
  _legacy: true,
  };
 });

 // Ordena: abertas primeiro (pendentes/aprovadas) e, dentro da mesma
 // faixa, pela competência mais recente. Concluídas e competências
 // antigas descem (separadas do atual).
 const repassesList = [...transfersList, ...legacyRepasses].sort((a, b) => {
  if (a._priority !== b._priority) return a._priority - b._priority;
  return (b._sortMonth || '').localeCompare(a._sortMonth || '');
 });

 // Apply filters
 const filteredRepasses = repassesList.filter(r => {
 const matchesSearch = !repasseSearch ||
 r.prop.toLowerCase().includes(repasseSearch.toLowerCase()) ||
 r.imovel.toLowerCase().includes(repasseSearch.toLowerCase()) ||
 r.modalidade.toLowerCase().includes(repasseSearch.toLowerCase()) ||
 r.origem.toLowerCase().includes(repasseSearch.toLowerCase()) ||
 r.id.toLowerCase().includes(repasseSearch.toLowerCase());
 const matchesStatus = repasseStatusFilter === 'Todos' || r.status === repasseStatusFilter;
 return matchesSearch && matchesStatus;
 });
 const canonicalFilteredRepasses = filteredRepasses.filter(r => r._includedInTotals);
 const legacyFilteredRepasses = filteredRepasses.filter(r => r._legacy);
 const unreconciledTransfers = transfersList.filter(r => !r._includedInTotals && r.status !== 'Sem saldo');

 // Dados agrupados por urgência (Em atraso / A vencer / Concluídas)
 const grouped = groupFinanceList(filteredRepasses, (r) => r.status);

 return (
 <div className="space-y-6 animate-in fade-in duration-300">
 <AnimatePresence>
 {!focusMode && (
 <motion.div
 initial={{ opacity: 0, height: 0, marginBottom: 0 }}
 animate={{ opacity: 1, height: "auto", marginBottom: 24 }}
 exit={{ opacity: 0, height: 0, marginBottom: 0, overflow: 'hidden' }}
 transition={{ duration: 0.3, ease: "easeInOut" }}
 >
 {/* Tab Flow Description */}
 <div className="bg-status-critical-soft/50 border border-status-critical/30 rounded-2xl p-5">
 <div className="flex items-start gap-4">
 <div className="p-2.5 bg-status-critical-soft rounded-xl shrink-0">
 <Hand className="w-5 h-5 text-status-critical" />
 </div>
 <div>
 <h3 className="text-base font-bold text-foreground mb-1">Controle de Repasses</h3>
 <p className="text-sm text-muted-foreground">
 Vendas e locações geram o repasse automaticamente após a quitação da cobrança, com o saldo líquido do proprietário.<br/>
 Confira a origem, aprove o repasse e registre a confirmação do pagamento para concluir a transferência.
 </p>
 </div>
 </div>
 </div>
 </motion.div>
 )}
 </AnimatePresence>

 {(reconciliation.consolidatedTransactionIDs.length > 0 || reconciliation.legacyTransactionIDs.length > 0 || unreconciledTransfers.length > 0) && (
  <div className="flex items-start gap-2 rounded-xl border border-status-info/30 bg-status-info-soft/50 px-4 py-3 text-sm text-muted-foreground" role="status">
   <Info className="mt-0.5 h-4 w-4 shrink-0 text-status-info" />
   <p>
    {reconciliation.consolidatedTransactionIDs.length > 0 && <><strong className="text-foreground">{reconciliation.consolidatedTransactionIDs.length}</strong> lançamento(s) com vínculo exato foram consolidados. </>}
    {reconciliation.legacyTransactionIDs.length > 0 && <><strong className="text-foreground">{reconciliation.legacyTransactionIDs.length}</strong> registro(s) legado(s) aparecem como <strong className="text-foreground">A apurar</strong> e não compõem os totais canônicos.</>}
    {unreconciledTransfers.length > 0 && <> <strong className="text-foreground">{unreconciledTransfers.length}</strong> repasse(s) sem origem econômica ou cobertura exata aparecem como <strong className="text-foreground">A apurar</strong> e permanecem somente leitura.</>}
   </p>
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
 <div className="flex items-center gap-3">
 <h2 className="text-2xl font-bold text-foreground">Repasses</h2>
 <span className="px-2.5 py-0.5 bg-muted text-muted-foreground text-xs font-bold rounded-full border border-border">
  {transfersList.filter(item => item._includedInTotals).length} conciliado(s)
  {unreconciledTransfers.length + legacyRepasses.length > 0 ? ` · ${unreconciledTransfers.length + legacyRepasses.length} a apurar` : ''}
 </span>
 </div>
 <p className="text-sm text-muted-foreground mt-1">Gerencie os repasses aos proprietários de imóveis e parceiros</p>
 </div>
 <div className="flex items-center gap-3">
 <Can permission="finance:write">
 <Button
 variant="outline"
 onClick={() => setExportConfig({
 title: "Relatório de Repasses",
 subtitle: "Repasses aos proprietários — Suíte",
 filterDescription: [repasseSearch && `Busca: "${repasseSearch}"`, repasseStatusFilter !== 'Todos' && `Status: ${repasseStatusFilter}`].filter(Boolean).join(' | ') || undefined,
 columns: [
 { key: 'id', label: 'Código' },
 { key: 'prop', label: 'Beneficiário' },
 { key: 'imovel', label: 'Origem' },
 { key: 'modalidade', label: 'Modalidade' },
 { key: 'origem', label: 'Recebimento de origem' },
 { key: 'geracao', label: 'Geração' },
 { key: 'periodo', label: 'Referência' },
 { key: 'aluguel', label: 'Valor Base', align: 'right', format: (v: number) => formatCurrency(v) },
 { key: 'taxa', label: 'Retenções e deduções', align: 'right', format: (v: number) => `-${formatCurrency(v)}` },
 { key: 'liquido', label: 'Valor Líquido', align: 'right', format: (v: number) => formatCurrency(v) },
 { key: 'data', label: 'Data Repasse' },
 { key: 'status', label: 'Status' },
 ],
 data: filteredRepasses,
 dateKey: 'sortDate',
 summaryRows: [
 { label: 'Repasses canônicos', value: String(canonicalFilteredRepasses.length) },
 { label: 'Valor Bruto Canônico', value: formatCurrency(canonicalFilteredRepasses.reduce((a, r) => a + r.aluguel, 0)) },
 { label: 'Deduções Canônicas', value: `-${formatCurrency(canonicalFilteredRepasses.reduce((a, r) => a + r.taxa, 0))}`, color: '#e11d48' },
 { label: 'Líquido Canônico', value: formatCurrency(canonicalFilteredRepasses.reduce((a, r) => a + r.liquido, 0)), color: '#059669' },
 { label: 'Legados a apurar', value: String(legacyFilteredRepasses.length), color: '#d97706' },
 ],
 recalcSummary: (d) => {
  const canonical = d.filter((r: any) => r._includedInTotals);
  const legacy = d.filter((r: any) => r._legacy);
  return [
   { label: 'Repasses canônicos', value: String(canonical.length) },
   { label: 'Valor Bruto Canônico', value: formatCurrency(canonical.reduce((a: number, r: any) => a + r.aluguel, 0)) },
   { label: 'Deduções Canônicas', value: `-${formatCurrency(canonical.reduce((a: number, r: any) => a + r.taxa, 0))}`, color: '#e11d48' },
   { label: 'Líquido Canônico', value: formatCurrency(canonical.reduce((a: number, r: any) => a + r.liquido, 0)), color: '#059669' },
   { label: 'Legados a apurar', value: String(legacy.length), color: '#d97706' },
  ];
 },
 })}
 >
 <Download className="w-4 h-4" /> Exportar
 </Button>
 </Can>
 <Can permission="finance:approve_transfer">
 <Button
 variant="outline"
 onClick={() => {
 const pendingTransfers = transfers.filter((t: OwnerTransfer) => getOwnerTransferPresentation(t).action === 'approve');
 if (pendingTransfers.length === 0) {
 toast.info('Nenhum repasse pendente para aprovar.');
 return;
 }
 openConfirm({
 title: 'Aprovar Repasses Pendentes',
 description: `Deseja aprovar ${pendingTransfers.length} repasse(s) pendente(s)? O pagamento continuará exigindo processamento e comprovante individual.`,
 confirmLabel: 'Aprovar Todos',
 variant: 'default',
 doubleConfirm: true,
 doubleConfirmLabel: 'Os repasses serão apenas aprovados; nenhum pagamento será marcado como realizado.',
 onConfirm: async () => {
 let ok = 0;
 let failed = 0;
 for (const tr of pendingTransfers) {
  try {
   await approveTransfer(tr.id);
   ok++;
  } catch {
   failed++;
  }
 }
 closeConfirm();
 if (ok === 0) {
  toast.error(`Nenhum repasse foi aprovado${failed ? ` (${failed} falha(s))` : ""}. Tente novamente.`);
 } else if (failed > 0) {
  toast.warning(`${ok} repasse(s) aprovado(s), ${failed} com falha. Revise os pendentes antes de repetir.`);
 } else {
  toast.success(`${ok} repasse(s) aprovado(s) com sucesso.`);
 }
 },
 });
 }}
 >
 Aprovar pendentes
 </Button>
 </Can>
 </div>
 </div>
 </motion.div>
 )}
 </AnimatePresence>

 <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
 <div className="p-4 border-b border-border bg-muted/50 flex flex-col sm:flex-row gap-3 items-start sm:items-center">
 <div className="relative flex-1 max-w-md">
 <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
 <input
 type="text"
 value={repasseSearch}
 onChange={e => filtersDispatch({ type: 'SET_TAB_SEARCH', tab: 'repasse', search: e.target.value })}
 placeholder="Buscar por proprietário, imóvel, cobrança ou modalidade..."
 className="w-full pl-9 pr-4 py-2 bg-card border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
 />
 </div>
 <div className="relative">
 <Button
 variant="outline"
 size="sm"
 onClick={() => filtersDispatch({ type: 'TOGGLE_TAB_STATUS_OPEN', tab: 'repasse' })}
 className="font-medium"
 >
 <Filter className="w-3.5 h-3.5 text-muted-foreground" /> {repasseStatusFilter} <ChevronDown className="w-4 h-4 text-muted-foreground" />
 </Button>
 {repasseStatusOpen && (
 <div className="absolute top-full right-0 mt-1 bg-card border border-border rounded-xl shadow-lg z-20 min-w-[140px] py-1">
 {['Todos', 'Pendente', 'Aprovado', 'Concluído', 'Liquidado sem saldo', 'A apurar', 'Sem saldo', 'Encerrado'].map(s => (
 <Button
 key={s}
 variant="ghost"
 size="sm"
 onClick={() => filtersDispatch({ type: 'SET_TAB_STATUS', tab: 'repasse', status: s })}
 className={clsx("w-full justify-start font-normal", repasseStatusFilter === s ? "font-bold text-primary-600 bg-primary-50" : "text-foreground")}
 >
 {s}
 </Button>
 ))}
 </div>
 )}
 </div>
 <span className="text-xs text-muted-foreground">{filteredRepasses.length} de {repassesList.length}</span>
 </div>
 <div className="overflow-x-auto">
 {loadingInvoices && filteredRepasses.length === 0 ? (
 <PageState loading skeleton={<PageStateSkeleton rows={5} />}>{null}</PageState>
 ) : (
 <ResponsiveTable
 data={[]}
 groups={grouped.map((group) => ({
 key: group.key,
 label: <span className="flex items-center gap-2">{group.label}<span className="font-medium normal-case tracking-normal text-muted-foreground">({group.items.length})</span></span>,
 data: group.items,
 }))}
 caption="Repasses a proprietários"
 getRowKey={(item) => item.id}
 empty={(
 <EmptyState
 icon={Search}
 message={repasseSearch || repasseStatusFilter !== "Todos" ? "Nenhum repasse encontrado para os filtros selecionados." : "Nenhum repasse encontrado. Eles serão gerados ao liquidar cobranças ou recebimentos vinculados."}
 className="border-none bg-transparent my-6"
 />
 )}
 columns={[
 {
 key: "id",
 header: "Código",
 primary: true,
 cell: (item) => (
 <div className="text-xs font-bold">
 {item.id}
 {!item._includedInTotals && <span className="mt-1 block w-fit rounded-full border border-status-warning/30 bg-status-warning-soft px-2 py-0.5 text-caption text-status-warning">{item._legacy ? 'Legado' : 'Sem conciliação'} · A apurar</span>}
 </div>
 ),
 },
 { key: "prop", header: "Beneficiário", cell: (item) => <span className="font-medium">{item.prop}</span> },
 {
 key: "origem",
 header: "Origem",
 cell: (item) => (
 <div className="text-muted-foreground">
 <span className="block">{item.imovel}</span>
 <span className="mt-1 block text-[11px] font-semibold text-foreground">{item.modalidade}</span>
 {item._automatic && <span className="mt-1 block text-[11px]">Gerado pelo recebimento</span>}
 <span className="mt-1 block max-w-56 break-all text-caption" title={item.origem}>{item.origem}</span>
 </div>
 ),
 },
 { key: "periodo", header: "Referência", cell: (item) => <span className="text-muted-foreground">{item.periodo}</span> },
 { key: "aluguel", header: "Valor Base", cell: (item) => formatCurrency(item.aluguel) },
 {
 key: "taxa",
 header: "Retenções e deduções",
 cell: (item) => (
 <div className="text-status-critical">
 -{formatCurrency(item.taxa)}
 {item._economicBreakdown && (
 <span className="mt-1 block text-[11px] font-normal leading-relaxed text-muted-foreground">
 {item._flow === 'rent' && <>Adm. {formatCurrency((item._economicBreakdown.admin_fee_cents ?? 0) / 100)}</>}
 {(item._economicBreakdown.intermediation_revenue_cents ?? 0) > 0 && `${item._flow === 'rent' ? ' • ' : ''}Intermediação ${formatCurrency((item._economicBreakdown.intermediation_revenue_cents ?? 0) / 100)}`}
 {item._economicBreakdown.commission_payable_cents > 0 && <span className="block">Corretores a pagar {formatCurrency(item._economicBreakdown.commission_payable_cents / 100)}</span>}
 </span>
 )}
 </div>
 ),
 },
 {
 key: "liquido",
 header: "Valor Líquido",
 cell: (item) => (
 <div className="font-bold">
 {formatCurrency(item.liquido)}
 {(item.status === 'Sem saldo' || item.status === 'Liquidado sem saldo') && <span className="mt-1 block text-[11px] font-normal text-muted-foreground">Nada a transferir nesta competência</span>}
 </div>
 ),
 },
 {
 key: "saldo",
 header: "Reservado / Disponível",
 cell: (item) => item._ownerBalance ? (
 <span className="tabular-nums text-muted-foreground">
 {formatCurrency(item._ownerBalance.reserved_cents / 100)} reservado{' • '}
 {formatCurrency(item._ownerBalance.available_cents / 100)} disponível
 </span>
 ) : '—',
 },
 { key: "data", header: "Data Repasse", cell: (item) => <span className="text-muted-foreground">{item.data}</span> },
 {
 key: "status",
 header: "Status",
 cell: (item) => (
 <span className={clsx("px-2.5 py-1 text-caption font-bold rounded-full", item.status === 'Concluído' ? "bg-primary-900 text-white" : item.status === 'Aprovado' ? "bg-status-ok-soft text-status-ok" : item.status === 'Liquidado sem saldo' ? "bg-status-ok-soft text-status-ok" : item.status === 'Sem saldo' || item.status === 'Encerrado' ? "bg-muted text-muted-foreground" : "bg-status-warning-soft text-status-warning")}>
 {item.status}
 </span>
 ),
 },
 {
 key: "actions",
 header: "Ações",
 className: "text-right",
 cell: (item) => (
  <div className="flex items-center justify-end gap-2">
  {item._settlementAction === 'approve' && item._transferId && (
  <Can permission="finance:approve_transfer">
  <Button
  variant="ghost"
  size="sm"
  className="text-primary-600 hover:text-primary-700 hover:bg-primary-50"
  onClick={() => {
  openConfirm({
  title: 'Aprovar Repasse',
  description: `Deseja aprovar o repasse de ${formatCurrency(item.liquido)} para "${item.prop}"?`,
  confirmLabel: 'Aprovar',
  variant: 'default',
  onConfirm: async () => {
  try {
  await approveTransfer(item._transferId!);
  closeConfirm();
  toast.success('Repasse aprovado', { description: `${item.prop} - ${formatCurrency(item.liquido)}` });
  } catch (error) {
  toast.error('Erro ao aprovar repasse');
  throw error;
  }
  },
  });
  }}
  >
  Aprovar
  </Button>
  </Can>
  )}
  {item._settlementAction === 'process' && item._transferId && (
  <Can permission="finance:approve_transfer">
  <Button
  variant="ghost"
  size="sm"
  className="text-status-ok hover:text-status-ok hover:bg-status-ok-soft"
  onClick={() => {
   const transfer = transfers.find((t: OwnerTransfer) => t.id === item._transferId);
   if (transfer) {
     openProcessRepasse(item._transferId!, transfer);
   } else {
     toast.error('O repasse mudou ou não está mais disponível. Atualize a página antes de processar.');
   }
 }}
  >
  Processar
  </Button>
  </Can>
  )}
  {item._settlementAction === 'settle' && item._transferId && (
  <Can permission="finance:approve_transfer">
  <Button
  variant="ghost"
  size="sm"
  className="text-status-ok hover:text-status-ok hover:bg-status-ok-soft"
  onClick={() => {
   const transferId = item._transferId!;
   openConfirm({
   title: 'Concluir repasse sem saldo',
   description: `Não há valor a transferir para "${item.prop}" nesta competência (líquido ${formatCurrency(item.liquido)}). O repasse será encerrado sem comprovante e nenhuma movimentação de caixa será registrada. Esta ação não pode ser desfeita.`,
   confirmLabel: 'Concluir sem saldo',
   variant: 'default',
   onConfirm: async () => {
   try {
   await processTransfer(transferId, {});
   closeConfirm();
   toast.success('Repasse concluído sem saldo', { description: `${item.prop} - ${item.periodo}` });
   } catch (error) {
   toast.error('Não foi possível concluir o repasse sem saldo. O repasse mudou ou não está mais disponível; atualize a página e tente novamente.');
   throw error;
   }
   },
   });
  }}
  >
  Concluir sem saldo
  </Button>
  </Can>
  )}
  <Button
  variant="ghost"
  size="sm"
  onClick={() => setSelectedTransaction(item)}
  className="text-muted-foreground hover:text-primary-600 hover:bg-primary-50"
  >
  Detalhes
  </Button>
  <Button
  variant="ghost"
  size="icon"
  onClick={() => setAudit({ entityId: item.id, label: item.prop + ' - ' + item.periodo })}
  className="size-7 text-muted-foreground"
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
 </div>
 </div>
 </div>
 );
}
