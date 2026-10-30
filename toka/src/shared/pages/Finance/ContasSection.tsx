import { FinanceLoadError } from "../../components/finance/FinanceLoadError";
import { useEffect, useState, useMemo, type Dispatch, type SetStateAction } from "react";
import {
 CheckCircle2,
 DollarSign,
 Download,
 Hand,
 Search,
 Zap,
 Loader2,
} from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { clsx } from "clsx";

import { ContractStatus, type Contract } from "../../contracts/types";
import type { ExportConfig } from "../../components/ExportReportModal";
import { ManagedBillsPanel } from "../../components/finance/ManagedBillsPanel";
import { EmptyState } from "../../ui/EmptyState";
import { ResponsiveTable } from "../../ui/responsive-table";
import { Button } from "../../ui/button";
import type { FilterAction } from "./state";
import { formatCurrency } from "./presentation";
import { utilityAccountService, type UtilityAccount, type UtilityAccountType } from "../../services/utilityAccount.service";

const UTILITY_TYPE_LABELS: Record<UtilityAccountType, string> = {
  AGUA: "Água",
  ENERGIA: "Luz",
  GAS: "Gás",
  CONDOMINIO: "Condomínio",
  IPTU: "IPTU",
  INTERNET: "Internet",
  SEGURO: "Seguro",
  OUTROS: "Outros",
};

const UTILITY_TYPE_COLORS: Record<UtilityAccountType, string> = {
  AGUA: "bg-primary-50 text-primary-600 border-primary-100",
  ENERGIA: "bg-status-warning-soft text-status-warning border-status-warning/20",
  GAS: "bg-status-critical-soft text-status-critical border-status-critical/20",
  CONDOMINIO: "bg-status-ok-soft text-status-ok border-status-ok/20",
  IPTU: "bg-muted text-muted-foreground border-border",
  INTERNET: "bg-primary-50 text-primary-600 border-primary-100",
  SEGURO: "bg-violet-50 text-violet-600 border-violet-100",
  OUTROS: "bg-muted text-muted-foreground border-border",
};

type ContasSectionProps = {
  contracts: Contract[];
  selectedContractId: string | null;
  focusMode: boolean;
  contasSearch: string;
  filtersDispatch: Dispatch<FilterAction>;
  setExportConfig: Dispatch<SetStateAction<ExportConfig | null>>;
};

interface DisplayConta {
  id: string;
  contractId: string;
  imovel: string;
  inquilino: string;
  tipo: string;
  tipoRaw: UtilityAccountType;
  valor: number;
  status: string;
  responsibleParty: string;
  autoPay: boolean;
  provider: string;
}

export function ContasSection({
  contracts,
  selectedContractId,
  focusMode,
  contasSearch,
  filtersDispatch,
  setExportConfig,
}: ContasSectionProps) {
  const [loading, setLoading] = useState(true);
  const [attempt, setAttempt] = useState(0);
  const [allAccounts, setAllAccounts] = useState<UtilityAccount[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);

  // Busca contas de consumo reais — por contrato selecionado ou todos
  useEffect(() => {
    let active = true;
    (async () => {
      setLoading(true);
      setLoadError(null);
      setAllAccounts([]);
      try {
        if (selectedContractId) {
          const accounts = await utilityAccountService.list(selectedContractId);
          if (active) setAllAccounts(accounts);
        } else {
          // Busca contas de todos os contratos de locação ativos
          const activeRentals = contracts.filter(
            c => c.type === 'locacao' && c.status === ContractStatus.ACTIVE
          );
          const results = await Promise.all(
            activeRentals.map(c => utilityAccountService.list(c.id))
          );
          if (active) { setAllAccounts(results.flat()); setLoadError(null); }
        }
      } catch {
        if (active) { setAllAccounts([]); setLoadError("Não foi possível carregar as contas de todos os contratos."); }
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => { active = false; };
  }, [contracts, selectedContractId, attempt]);

  // Mapeia contas de consumo reais para o formato de exibição
  const dynamicContas: DisplayConta[] = allAccounts.map(acc => {
    const contract = contracts.find(c => c.id === acc.contract_id);
    return {
      id: acc.id,
      contractId: acc.contract_id,
      imovel: contract?.property_title || acc.property_id,
      inquilino: contract?.client || "-",
      tipo: UTILITY_TYPE_LABELS[acc.type] || acc.type,
      tipoRaw: acc.type,
      valor: acc.estimated_amount_cents > 0
        ? acc.estimated_amount_cents / 100
        : 0,
      status: acc.auto_pay_enabled ? "Automático" : "Manual",
      responsibleParty: acc.responsible_party,
      autoPay: acc.auto_pay_enabled,
      provider: acc.provider || "-",
    };
  });

  const filtered = useMemo(
    () => dynamicContas.filter(item => {
      if (!contasSearch) return true;
      const q = contasSearch.toLowerCase();
      return item.imovel?.toLowerCase().includes(q) || item.inquilino?.toLowerCase().includes(q) || item.tipo?.toLowerCase().includes(q);
    }),
    [dynamicContas, contasSearch]
  );

  const propertyLabels = useMemo(
    () => Object.fromEntries(
      contracts.flatMap((contract) =>
        contract.property_id
          ? [[contract.property_id, contract.property_title || contract.property_id] as const]
          : [],
      ),
    ),
    [contracts],
  );

 const totalEstimado = filtered.reduce((acc, curr) => acc + curr.valor, 0);
 const countAutomaticas = filtered.filter(c => c.status === 'Automático').length;
 const countManuais = filtered.filter(c => c.status === 'Manual').length;
 const valorAutomatico = filtered.filter(c => c.status === 'Automático').reduce((acc, curr) => acc + curr.valor, 0);

 const dynamicStatsContas = [
 { label: "Total Estimado", value: formatCurrency(totalEstimado), icon: DollarSign, colorClass: "text-primary-600", bgClass: "bg-primary-50" },
 { label: "Débito configurado", value: countAutomaticas.toString(), icon: Zap, colorClass: "text-primary-600", bgClass: "bg-primary-50" },
 { label: "Manuais", value: countManuais.toString(), icon: Hand, colorClass: "text-muted-foreground", bgClass: "bg-muted" },
 { label: "Estimativa com débito", value: formatCurrency(valorAutomatico), icon: CheckCircle2, colorClass: "text-status-ok", bgClass: "bg-status-ok-soft" },
 ];

 if (loadError) return <FinanceLoadError message={loadError} onRetry={() => setAttempt(value => value + 1)} />;
 if (loading) return <p role="status">Carregando contas…</p>;

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
 <div className="bg-sky-50/50 border border-sky-200 rounded-2xl p-5">
 <div className="flex items-start gap-4">
 <div className="p-2.5 bg-sky-100 rounded-xl shrink-0">
 <Zap className="w-5 h-5 text-status-info" />
 </div>

  <div>
  <h3 className="text-base font-bold text-foreground mb-1">Contas de Consumo</h3>
 <p className="text-sm text-muted-foreground">
 Visualize as despesas vinculadas a imóveis locados (água, luz, IPTU, condomínio).<br/>
 Aqui você acompanha configurações de débito; pagamentos e conciliações efetivos ficam nas contas administradas.
 </p>
 </div>
 </div>
 </div>
 </motion.div>
 )}
 </AnimatePresence>

 <ManagedBillsPanel
 contractId={selectedContractId}
 search={contasSearch}
 propertyLabels={propertyLabels}
 />

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
 <h2 className="text-2xl font-bold text-foreground">Gestão de Contas</h2>
 <span className="px-2.5 py-0.5 bg-muted text-muted-foreground text-xs font-bold rounded-full border border-border">{dynamicContas.length} contas configuradas</span>
 </div>
 <p className="text-sm text-muted-foreground mt-1">Gerencie pagamentos realizados e contas configuradas dos contratos de locação</p>
 </div>
 <Button
 variant="outline"
 onClick={() => {
 const filteredContas = dynamicContas.filter(item => {
 if (!contasSearch) return true;
 const q = contasSearch.toLowerCase();
 return item.imovel?.toLowerCase().includes(q) || item.inquilino?.toLowerCase().includes(q) || item.tipo?.toLowerCase().includes(q);
 });
 setExportConfig({
 title: "Relatório de Contas de Consumo",
 subtitle: "Contas configuradas por imóvel - suitee",
 filterDescription: contasSearch ? `Busca: "${contasSearch}"` : undefined,
 columns: [
 { key: 'imovel', label: 'Imóvel' },
 { key: 'inquilino', label: 'Inquilino' },
 { key: 'tipo', label: 'Tipo de Conta' },
 { key: 'valor', label: 'Valor Estimado', align: 'right', format: (v: number) => formatCurrency(v) },
 { key: 'status', label: 'Status' },
 ],
 data: filteredContas,
 summaryRows: [
 { label: 'Total Estimado', value: formatCurrency(filteredContas.reduce((a, c) => a + c.valor, 0)) },
 { label: 'Automáticas', value: String(filteredContas.filter(c => c.status === 'Automático').length) },
 { label: 'Manuais', value: String(filteredContas.filter(c => c.status === 'Manual').length) },
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
 initial={{ opacity: 0, height: 0 }}
 animate={{ opacity: 1, height: "auto" }}
 exit={{ opacity: 0, height: 0, overflow: 'hidden' }}
 transition={{ duration: 0.3, ease: "easeInOut" }}
 >
 <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-6 mb-6">
 {dynamicStatsContas.map((stat, idx) => (
 <div key={idx} className="bg-card p-6 rounded-2xl border border-border shadow-sm flex items-center gap-4 hover:border-primary-200 transition-colors min-w-0">
 <div className={clsx("w-12 h-12 shrink-0 rounded-xl flex items-center justify-center border border-border", stat.bgClass)}>
 <stat.icon className={clsx("w-6 h-6", stat.colorClass)} />
 </div>
 <div className="min-w-0 flex-1">
 <p className="text-xs font-medium text-muted-foreground mb-1 truncate">{stat.label}</p>
 <h3 className="text-2xl font-bold text-foreground leading-tight break-words">{stat.value}</h3>
 </div>
 </div>
 ))}
 </div>
 </motion.div>
 )}
 </AnimatePresence>

 <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
 <div className="p-5 border-b border-border flex flex-col sm:flex-row justify-between sm:items-center gap-4">
 <h3 className="text-lg font-bold text-foreground">Contas Configuradas por Imóvel</h3>
 <div className="relative max-w-xs w-full">
 <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
 <input
 type="text"
 value={contasSearch}
 onChange={e => filtersDispatch({ type: 'SET_TAB_SEARCH', tab: 'contas', search: e.target.value })}
 placeholder="Buscar por imóvel, inquilino ou tipo..."
 className="w-full pl-9 pr-4 py-2 border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 bg-muted/50"
 />
 </div>
 </div>

 <div className="flex bg-muted border-b border-border">
 <button className="flex-1 py-3 text-sm font-medium text-muted-foreground hover:text-foreground border-b-2 border-transparent">Todas ({dynamicContas.length})</button>
 <button className="flex-1 py-3 text-sm font-bold text-primary-600 border-b-2 border-primary-600">Com débito ({countAutomaticas})</button>
 <button className="flex-1 py-3 text-sm font-medium text-muted-foreground hover:text-foreground border-b-2 border-transparent">Manuais ({countManuais})</button>
 </div>

 <div className="overflow-x-auto">
        {loading ? (
          <div className="p-12 flex items-center justify-center">
            <Loader2 className="w-6 h-6 animate-spin text-muted-foreground" />
          </div>
        ) : (
        <ResponsiveTable
          data={filtered}
          caption="Contas configuradas por imóvel"
          getRowKey={(item) => item.id}
          empty={<EmptyState icon={Search} message="Nenhuma conta encontrada para os filtros atuais." className="border-none bg-transparent" />}
          columns={[
            { key: "imovel", header: "Imóvel", primary: true, cell: (item) => item.imovel },
            { key: "inquilino", header: "Inquilino", cell: (item) => item.inquilino },
            {
              key: "tipo",
              header: "Tipo de Conta",
              cell: (item) => (
                <span className={clsx(
                  "px-2.5 py-1 text-xs font-medium rounded-full border",
                  UTILITY_TYPE_COLORS[item.tipoRaw] || "bg-muted text-muted-foreground border-border",
                )}>
                  {item.tipo}
                </span>
              ),
            },
            { key: "valor", header: "Valor Estimado", cell: (item) => <span className="font-bold">{formatCurrency(item.valor)}</span> },
            {
              key: "status",
              header: "Status",
              cell: (item) => item.autoPay ? (
                <div className="flex items-center gap-2">
                  <span className="inline-flex items-center gap-1.5 px-2.5 py-1 text-xs font-bold text-primary-900 bg-primary-50 border border-primary-200 rounded-lg">
                    <Zap className="w-3.5 h-3.5" /> Débito configurado
                  </span>
                  <span className="inline-flex items-center px-2 py-1 text-micro font-bold text-primary-foreground bg-primary rounded-md">
                    Automático
                  </span>
                </div>
              ) : (
                <span className="inline-flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-muted-foreground bg-muted border border-border rounded-lg">
                  <Hand className="w-3.5 h-3.5" /> Manual
                </span>
              ),
            },
          ]}
          mobileCard={(item) => (
            <article className="rounded-xl border border-border bg-card p-4">
              <div className="flex items-start justify-between gap-2">
                <p className="font-bold text-foreground">{item.imovel}</p>
                <span className={clsx(
                  "px-2 py-0.5 text-micro font-medium rounded-full border",
                  UTILITY_TYPE_COLORS[item.tipoRaw] || "bg-muted text-muted-foreground border-border",
                )}>{item.tipo}</span>
              </div>
              <p className="mt-1 text-sm text-muted-foreground">{item.inquilino || "Sem inquilino"}</p>
              <div className="mt-3 flex items-center justify-between gap-2">
                <span className="text-sm font-bold text-foreground">{formatCurrency(item.valor)}</span>
                <span className={clsx("text-xs font-bold", item.autoPay ? "text-primary-700" : "text-muted-foreground")}>
                  {item.autoPay ? "Débito automático" : "Manual"}
                </span>
              </div>
            </article>
          )}
        />
        )}
 {dynamicContas.length === 0 && (
 <EmptyState
 icon={Search}
 message="Nenhuma conta configurada. Você não possui contratos de locação ativos."
 className="border-none bg-transparent my-6"
 />
 )}
 </div>
 </div>
 </div>
 );
}
