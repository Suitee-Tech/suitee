import { formatISODateBR } from "../../utils/format";
import type { Dispatch, SetStateAction } from "react";
import {
 AlertCircle,
 CheckCircle2,
 ChevronDown,
 Clock,
 Download,
 Filter,
 Search,
} from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { clsx } from "clsx";

import type { Contract } from "../../contracts/types";
import type { Transaction } from "../../types/transaction";
import type { Charge } from "../../services/charge.service";
import type { OwnerTransfer } from "../../services/transfer.service";
import type { ExportConfig } from "../../components/ExportReportModal";
import { EmptyState } from "../../ui/EmptyState";
import { Button } from "../../ui/button";
import type { FilterAction } from "./state";
import { formatCurrency } from "./presentation";
import { normalizeStatus } from "../../lib/lancamentos/status";
import { hasNoPayableBalance } from "../../lib/finance/ownerTransferFlow";

/**
 * Estados de baixa desta conferência. "Sem saldo" é uma categoria própria: o
 * repasse não tem valor a transferir, então não há baixa a aguardar nem
 * movimentação de caixa a conciliar — ele sai da contagem de pendentes e fica
 * visível até o operador concluí-lo em Repasses.
 */
type ConciliacaoStatus = 'Baixado' | 'Aguardando baixa' | 'Vencido' | 'Encerrado' | 'Sem saldo';

type ConciliacaoSectionProps = {
 contracts: Contract[];
 transactions: Transaction[];
 /** Cobranças (recebimentos de aluguel/venda) — fonte de crédito do caixa. */
 charges: Charge[];
 /** Repasses ao proprietário — fonte de débito do caixa. */
 transfers: OwnerTransfer[];
 focusMode: boolean;
 conciliacaoSearch: string;
 conciliacaoStatusFilter: string;
 conciliacaoStatusOpen: boolean;
 filtersDispatch: Dispatch<FilterAction>;
 setExportConfig: Dispatch<SetStateAction<ExportConfig | null>>;
};

export function ConciliacaoSection({
 contracts,
 transactions,
 charges,
 transfers,
 focusMode,
 conciliacaoSearch,
 conciliacaoStatusFilter,
 conciliacaoStatusOpen,
 filtersDispatch,
 setExportConfig,
}: ConciliacaoSectionProps) {
 // Esta visão confere o estado das movimentações econômicas internas. Sem uma
 // linha de extrato persistida, "Pago" prova baixa operacional, não conciliação
 // bancária. Reúne as TRÊS fontes do caixa: lançamentos manuais (/transactions),
 // recebimentos de cobranças (charges pagas) e repasses ao proprietário
 // (transfers) — sem elas o total de créditos/débitos não fecha para a venda/locação.
 const transactionRows = transactions.map(tx => {
  const contract = contracts.find(c => c.id === tx.contractId);
  const isCredit = tx.type === 'receita';
  const isDebit = tx.type === 'despesa' || tx.type === 'repasse' || tx.type === 'comissao';
  const valor = isDebit ? -Math.abs(tx.value) : Math.abs(tx.value);

  const canonicalStatus = normalizeStatus('transaction', tx.status);
  let concStatus: ConciliacaoStatus = 'Aguardando baixa';
  let subdesc: string | undefined;
  let isAlert = false;

  if (canonicalStatus === 'pago' || canonicalStatus === 'processado') {
    concStatus = 'Baixado';
    subdesc = tx.paymentDate ? `Baixa registrada em: ${tx.paymentDate}` : `Baixa registrada em: ${tx.date}`;
  } else if (canonicalStatus === 'vencido') {
    concStatus = 'Vencido';
    subdesc = `Vencimento original: ${tx.dueDate || tx.date}`;
    isAlert = true;
  } else if (canonicalStatus === 'cancelado' || canonicalStatus === 'revertido') {
    concStatus = 'Encerrado';
    subdesc = canonicalStatus === 'revertido' ? 'Lançamento revertido; histórico preservado' : 'Lançamento cancelado';
  }

  return {
    id: tx.id,
    data: tx.date,
    tipo: isCredit ? 'Crédito' as const : 'Débito' as const,
    desc: `${tx.description} - ${tx.contractId}${contract ? ` - ${contract.client}` : ''}`,
    subdesc,
    isAlert,
    contrato: tx.contractId,
    valor,
    status: concStatus,
  };
 });

 // Recebimentos de cobranças: entrada de caixa originada pelo charge pago.
 const chargeRows = charges.map(c => {
  const contract = contracts.find(item => item.id === c.contract_id);
  const canonicalStatus = normalizeStatus('charge', c.status);
  let concStatus: ConciliacaoStatus = 'Aguardando baixa';
  let subdesc: string | undefined;
  let isAlert = false;
  if (canonicalStatus === 'pago') {
    concStatus = 'Baixado';
    subdesc = c.paid_at ? `Baixa registrada em: ${c.paid_at}` : undefined;
  } else if (canonicalStatus === 'vencido') {
    concStatus = 'Vencido';
    subdesc = `Vencimento original: ${c.due_date || ''}`;
    isAlert = true;
  } else if (canonicalStatus === 'cancelado' || canonicalStatus === 'revertido') {
    concStatus = 'Encerrado';
    subdesc = 'Cobrança encerrada; histórico preservado';
  }
  return {
    id: c.id,
    data: c.paid_at ?? c.due_date,
    tipo: 'Crédito' as const,
    desc: `${c.description} - ${c.contract_id}${contract ? ` - ${contract.client}` : ''}`,
    subdesc,
    isAlert,
    contrato: c.contract_id,
    valor: (c.paid_amount ?? c.total_amount) ?? c.amount,
    status: concStatus,
  };
 });

 // Repasses ao proprietário: saída de caixa quando processados.
 const transferRows = transfers.map(t => {
  const contract = contracts.find(c => c.id === t.contract_id);
  const canonicalStatus = normalizeStatus('transfer', t.status);
  let concStatus: ConciliacaoStatus = 'Aguardando baixa';
  let subdesc: string | undefined;
  if (canonicalStatus === 'processado') {
    concStatus = 'Baixado';
    subdesc = t.processed_at ? `Baixa registrada em: ${t.processed_at}` : undefined;
  } else if (canonicalStatus === 'cancelado' || canonicalStatus === 'revertido') {
    concStatus = 'Encerrado';
    subdesc = 'Repasse encerrado; histórico preservado';
  } else if (hasNoPayableBalance(t)) {
    // Repasse sem valor a transferir: não há baixa a aguardar, então ele sai da
    // contagem de pendentes e fica visível na própria categoria até a conclusão
    // em Repasses (nenhuma movimentação de caixa será registrada).
    concStatus = 'Sem saldo';
    subdesc = 'Nada a repassar nesta competência';
  } else if (canonicalStatus === 'aprovado') {
    concStatus = 'Aguardando baixa';
    subdesc = t.approved_at ? `Aprovado em: ${t.approved_at}` : 'Aprovado aguardando processamento';
  }
  return {
    id: t.id,
    data: t.processed_at ?? t.approved_at ?? t.created_at,
    tipo: 'Débito' as const,
    desc: `Repasse ao proprietário - ${t.contract_id}${contract ? ` - ${contract.client}` : ''}`,
    subdesc,
    isAlert: false,
    contrato: t.contract_id,
    valor: -Math.abs(t.net_amount),
    status: concStatus,
  };
 });

 const conciliacaoItems = [...chargeRows, ...transferRows, ...transactionRows]
  .filter(item => {
    const q = conciliacaoSearch.trim().toLocaleLowerCase('pt-BR');
    return (!q || `${item.desc} ${item.contrato}`.toLocaleLowerCase('pt-BR').includes(q)) &&
      (conciliacaoStatusFilter === 'Todos' || item.status === conciliacaoStatusFilter);
  })
  .sort((a, b) => (b.data ?? '').localeCompare(a.data ?? ''));

 const totalItems = conciliacaoItems.length;
 const baixados = conciliacaoItems.filter(i => i.status === 'Baixado');
 const pendentes = conciliacaoItems.filter(i => i.status === 'Aguardando baixa');
 const vencidos = conciliacaoItems.filter(i => i.status === 'Vencido');
 const encerrados = conciliacaoItems.filter(i => i.status === 'Encerrado');
 // Repasses sem valor a transferir ficam fora de `pendentes`: não há baixa a
 // esperar, só a conclusão do repasse em Repasses.
 const semSaldo = conciliacaoItems.filter(i => i.status === 'Sem saldo');
 const pctBaixado = totalItems > 0 ? Math.round((baixados.length / totalItems) * 100) : 0;

 const totalCreditos = conciliacaoItems.filter(i => i.valor >= 0).reduce((acc, i) => acc + i.valor, 0);
 const totalDebitos = conciliacaoItems.filter(i => i.valor < 0).reduce((acc, i) => acc + Math.abs(i.valor), 0);

 const dynamicStatsConciliacao = [
 { label: "Total de Movimentações", value: totalItems.toString(), highlight: "", color: "" },
 { label: "Baixadas", value: baixados.length.toString(), highlight: `(${pctBaixado}%)`, color: "text-status-ok" },
 { label: "Aguardando baixa", value: pendentes.length.toString(), highlight: "", color: "text-status-warning" },
 { label: "Vencidas", value: vencidos.length.toString(), highlight: "", color: "text-status-critical" },
 { label: "Sem saldo", value: semSaldo.length.toString(), highlight: "", color: "text-status-info" },
 { label: "Encerradas", value: encerrados.length.toString(), highlight: "", color: "text-muted-foreground" },
 ];

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
 <div className="bg-status-ok-soft/50 border border-status-ok/30 rounded-2xl p-5">
 <div className="flex items-start gap-4">
 <div className="p-2.5 bg-status-ok-soft rounded-xl shrink-0">
 <CheckCircle2 className="w-5 h-5 text-status-ok" />
 </div>
 <div>
 <h3 className="text-base font-bold text-foreground mb-1">Conferência manual de baixas</h3>
 <p className="text-sm text-muted-foreground">
 Esta tela não importa nem cruza extrato bancário. Ela organiza o estado dos lançamentos internos.<br/>
 Registre recebimentos, repasses e contas na área de origem; Contas &amp; encargos persiste a referência de conciliação quando aplicável.
 </p>
 </div>
 </div>
 </div>
 </motion.div>
 )}
 </AnimatePresence>

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
 <h2 className="text-2xl font-bold text-foreground">Movimentações por estado de baixa</h2>
 <p className="text-sm text-muted-foreground mt-1">Conferência operacional; não representa saldo ou correspondência com extrato bancário</p>
 </div>
 <Button
 variant="outline"
 onClick={() => {
 const filteredConciliacao = conciliacaoItems.filter(item => {
 const matchesSearch = !conciliacaoSearch ||
 item.desc.toLowerCase().includes(conciliacaoSearch.toLowerCase()) ||
 item.contrato.toLowerCase().includes(conciliacaoSearch.toLowerCase());
 const matchesStatus = conciliacaoStatusFilter === 'Todos' || item.status === conciliacaoStatusFilter;
 return matchesSearch && matchesStatus;
 });
 setExportConfig({
 title: "Relatório de Conferência Financeira",
 subtitle: "Movimentações por estado de baixa — Suíte",
 filterDescription: [conciliacaoSearch && `Busca: "${conciliacaoSearch}"`, conciliacaoStatusFilter !== 'Todos' && `Status: ${conciliacaoStatusFilter}`].filter(Boolean).join(' | ') || undefined,
 columns: [
 { key: 'data', label: 'Data' },
 { key: 'tipo', label: 'Tipo' },
 { key: 'desc', label: 'Descrição' },
 { key: 'contrato', label: 'Contrato' },
 { key: 'valor', label: 'Valor (R$)', align: 'right', format: (v: number) => v >= 0 ? formatCurrency(v) : `-${formatCurrency(v)}` },
 { key: 'status', label: 'Status' },
 ],
 data: filteredConciliacao,
 dateKey: 'data',
 summaryRows: [
 { label: 'Movimentações', value: String(filteredConciliacao.length) },
 { label: 'Baixadas', value: String(filteredConciliacao.filter(i => i.status === 'Baixado').length), color: '#059669' },
 { label: 'Aguardando baixa', value: String(filteredConciliacao.filter(i => i.status === 'Aguardando baixa').length), color: '#d97706' },
 { label: 'Vencidas', value: String(filteredConciliacao.filter(i => i.status === 'Vencido').length), color: '#e11d48' },
 { label: 'Sem saldo', value: String(filteredConciliacao.filter(i => i.status === 'Sem saldo').length) },
 { label: 'Encerradas', value: String(filteredConciliacao.filter(i => i.status === 'Encerrado').length) },
 ],
 recalcSummary: (d) => [
 { label: 'Movimentações', value: String(d.length) },
 { label: 'Baixadas', value: String(d.filter((i: any) => i.status === 'Baixado').length), color: '#059669' },
 { label: 'Aguardando baixa', value: String(d.filter((i: any) => i.status === 'Aguardando baixa').length), color: '#d97706' },
 { label: 'Vencidas', value: String(d.filter((i: any) => i.status === 'Vencido').length), color: '#e11d48' },
 { label: 'Sem saldo', value: String(d.filter((i: any) => i.status === 'Sem saldo').length) },
 { label: 'Encerradas', value: String(d.filter((i: any) => i.status === 'Encerrado').length) },
 ],
 });
 }}
 >
 <Download className="w-4 h-4" /> Exportar
 </Button>
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
 <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-6">
 {dynamicStatsConciliacao.map((stat, idx) => (
 <div key={idx} className="bg-card p-6 rounded-2xl border border-border shadow-sm min-w-0">
 <p className="text-sm font-medium text-muted-foreground mb-2 truncate">{stat.label}</p>
 <div className="flex items-baseline gap-2 min-w-0">
 <h3 className={clsx("text-3xl font-bold leading-tight break-words", stat.color || "text-foreground")}>{stat.value}</h3>
 {stat.highlight && <span className={clsx("text-sm font-medium whitespace-nowrap", stat.color)}>{stat.highlight}</span>}
 </div>
 </div>
 ))}
 </div>
 </motion.div>
 )}
 </AnimatePresence>

 <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
 <div className="p-5 border-b border-border space-y-4">
 <h3 className="text-lg font-bold text-foreground">Movimentações</h3>

 <div className="flex flex-wrap gap-4">
 <div className="relative flex-1 min-w-[200px]">
 <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
 <input
 type="text"
 value={conciliacaoSearch}
 onChange={e => filtersDispatch({ type: 'SET_TAB_SEARCH', tab: 'conciliacao', search: e.target.value })}
 placeholder="Buscar descrição, contrato..."
 className="w-full pl-9 pr-4 py-2 border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 bg-muted/50"
 />
 </div>
 <div className="flex items-center gap-3 relative">
 <Button
 variant="outline"
 size="sm"
 onClick={() => filtersDispatch({ type: 'TOGGLE_TAB_STATUS_OPEN', tab: 'conciliacao' })}
 className="font-medium"
 >
 <Filter className="w-4 h-4 text-muted-foreground" /> {conciliacaoStatusFilter} <ChevronDown className="w-4 h-4" />
 </Button>
 {conciliacaoStatusOpen && (
 <div className="absolute top-full right-0 mt-1 bg-card border border-border rounded-xl shadow-lg z-20 min-w-40 py-1">
 {['Todos', 'Baixado', 'Aguardando baixa', 'Vencido', 'Sem saldo', 'Encerrado'].map(s => (
 <Button
 key={s}
 variant="ghost"
 size="sm"
 onClick={() => filtersDispatch({ type: 'SET_TAB_STATUS', tab: 'conciliacao', status: s })}
 className={clsx("w-full justify-start font-normal", conciliacaoStatusFilter === s ? "font-bold text-primary-600 bg-primary-50" : "text-foreground")}
 >
 {s}
 </Button>
 ))}
 </div>
 )}
 </div>
 </div>
 </div>

 <div className="overflow-x-auto">
 <table className="w-full text-left border-collapse">
 <thead>
 <tr className="border-b border-border text-muted-foreground text-xs font-medium tracking-wide bg-muted/50">
 <th className="px-6 py-4">Data</th>
 <th className="px-6 py-4">Tipo</th>
 <th className="px-6 py-4">Descrição</th>
 <th className="px-6 py-4">Contrato</th>
 <th className="px-6 py-4">Valor</th>
 <th className="px-6 py-4">Status</th>
 </tr>
 </thead>
 <tbody className="divide-y divide-border">
 {conciliacaoItems.filter(item => {
 const matchesSearch = !conciliacaoSearch ||
 item.desc.toLowerCase().includes(conciliacaoSearch.toLowerCase()) ||
 item.contrato.toLowerCase().includes(conciliacaoSearch.toLowerCase());
 const matchesStatus = conciliacaoStatusFilter === 'Todos' || item.status === conciliacaoStatusFilter;
 return matchesSearch && matchesStatus;
 }).map((item) => (
 <tr key={item.id} className="hover:bg-muted/50 transition-colors">
 <td className="px-6 py-4 text-sm font-medium text-foreground">{formatISODateBR(item.data)}</td>
 <td className="px-6 py-4">
 <span className={clsx(
 "px-2.5 py-1 text-micro font-bold uppercase tracking-wider rounded-md border",
 item.tipo === 'Crédito' ? "bg-status-ok-soft text-status-ok border-status-ok/20" : "bg-status-critical-soft text-status-critical border-status-critical/20"
 )}>
 {item.tipo}
 </span>
 </td>
 <td className="px-6 py-4">
 <p className="text-sm font-medium text-foreground">{item.desc}</p>
 {item.subdesc && (
 <p className={clsx("text-xs mt-0.5", item.isAlert ? "text-status-critical font-medium" : "text-muted-foreground")}>
 {item.subdesc}
 </p>
 )}
 </td>
 <td className="px-6 py-4">
 <span className="px-2.5 py-1 bg-muted border border-border text-muted-foreground text-micro font-bold rounded-md">
 {item.contrato}
 </span>
 </td>
 <td className="px-6 py-4">
 <span className={clsx("text-sm font-bold", item.valor >= 0 ? "text-status-ok" : "text-status-critical")}>
 {item.valor >= 0 ? formatCurrency(item.valor) : `-${formatCurrency(item.valor)}`}
 </span>
 </td>
 <td className="px-6 py-4">
 {item.status === 'Baixado' && (
 <span className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium text-status-ok bg-status-ok-soft border border-status-ok/30 rounded-full">
 <CheckCircle2 className="w-3.5 h-3.5" /> Baixado
 </span>
 )}
 {item.status === 'Aguardando baixa' && (
 <span className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium text-status-warning bg-status-warning-soft border border-status-warning/30 rounded-full">
 <Clock className="w-3.5 h-3.5" /> Aguardando baixa
 </span>
 )}
 {item.status === 'Vencido' && (
 <span className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium text-status-critical bg-status-critical-soft border border-status-critical/30 rounded-full">
 <AlertCircle className="w-3.5 h-3.5" /> Vencido
 </span>
 )}
 {item.status === 'Sem saldo' && (
 <span className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium text-status-info bg-status-info-soft border border-status-info/30 rounded-full">
 <Clock className="w-3.5 h-3.5" /> Sem saldo
 </span>
 )}
 {item.status === 'Encerrado' && (
 <span className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium text-muted-foreground bg-muted border border-border rounded-full">
 Encerrado
 </span>
 )}
 </td>
 </tr>
 ))}
 </tbody>
 </table>
 {conciliacaoItems.length === 0 && (
 <EmptyState
 icon={Search}
 message="Nenhuma movimentação para conferir."
 className="border-none bg-transparent my-6"
 />
 )}
 </div>

 <div className="p-6 border-t border-border bg-muted/80 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
 <p className="text-xs text-muted-foreground sm:max-w-xs">Valores exibidos são lançamentos internos e não constituem saldo bancário conciliado.</p>
 <div>
 <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">Total de Créditos</p>
 <p className="text-xl font-bold text-status-ok">{formatCurrency(totalCreditos)}</p>
 </div>
 <div className="text-right">
 <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-1">Total de Débitos</p>
 <p className="text-xl font-bold text-status-critical">{formatCurrency(totalDebitos)}</p>
 </div>
 </div>
 </div>
 </div>
 );
}
