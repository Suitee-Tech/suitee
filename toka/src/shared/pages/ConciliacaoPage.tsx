import { FinanceLoadError } from "../components/finance/FinanceLoadError";
import { useCallback, useState, type Dispatch } from "react";
import { Repeat } from "lucide-react";

import { ConciliacaoSection } from "./Finance/ConciliacaoSection";
import type { FilterAction } from "./Finance/state";
import { useFinanceContext } from "../context/FinanceContext";
import { useContractsContext } from "../context/ContractsContext";
import { useFocusMode } from "../context/FocusModeContext";
import { ExportReportModal, type ExportConfig } from "../components/ExportReportModal";
import { FinanceAreaNav } from "../components/finance/FinanceAreaNav";
import { ReconciliationReportSection } from "../components/finance/ReconciliationReportSection";

/**
 * Conferência operacional das baixas do ledger. Invoices não são adicionadas:
 * elas são documentos/projeções, não uma segunda movimentação financeira.
 */
export default function ConciliacaoPage() {
  const { contracts } = useContractsContext();
  const { focusMode } = useFocusMode();
  const { transactions, charges, transfers, errors, loading, refetchCharges, refetchTransactions, refetchTransfers } = useFinanceContext();

  // Filtros do módulo de conciliação.
  const [conciliacaoSearch, setConciliacaoSearch] = useState("");
  const [conciliacaoStatusFilter, setConciliacaoStatusFilter] = useState("Todos");
  const [conciliacaoStatusOpen, setConciliacaoStatusOpen] = useState(false);
  const dispatchConciliacao: Dispatch<FilterAction> = useCallback((action) => {
    if (!("type" in action)) return;
    const a = action as { type: string; tab?: string; search?: string; status?: string };
    if (a.type === "SET_TAB_SEARCH" && a.tab === "conciliacao") setConciliacaoSearch(a.search ?? "");
    else if (a.type === "TOGGLE_TAB_STATUS_OPEN" && a.tab === "conciliacao") setConciliacaoStatusOpen((v) => !v);
    else if (a.type === "SET_TAB_STATUS" && a.tab === "conciliacao") {
      setConciliacaoStatusFilter(a.status ?? "Todos");
      setConciliacaoStatusOpen(false);
    }
  }, []);

  // Export.
  const [exportConfig, setExportConfig] = useState<ExportConfig | null>(null);

  if (errors?.charges || errors?.transfers || errors?.transactions) return <div className="p-6 space-y-4"><FinanceAreaNav /><FinanceLoadError
    message="Conferência indisponível: não foi possível carregar todas as movimentações."
    onRetry={() => { void refetchCharges(); void refetchTransfers(); void refetchTransactions(); }}
  /></div>;
  if (loading.charges || loading.transfers || loading.transactions) return <div className="p-6 space-y-4"><FinanceAreaNav /><p role="status">Carregando movimentações…</p></div>;

  return (
    <div className="mx-auto max-w-7xl space-y-6 p-5 lg:p-6">
      <FinanceAreaNav />
      <div>
        <h1 className="text-2xl font-bold flex items-center gap-2 text-foreground">
          <Repeat className="w-6 h-6 text-primary-600" /> Conferência financeira
        </h1>
        <p className="text-sm text-muted-foreground mt-1">
          Revise baixas, vencimentos e encerramentos sem confundir lançamentos internos com extrato bancário.
        </p>
      </div>

      <ConciliacaoSection
        contracts={contracts}
        transactions={transactions}
        charges={charges}
        transfers={transfers}
        focusMode={focusMode}
        conciliacaoSearch={conciliacaoSearch}
        conciliacaoStatusFilter={conciliacaoStatusFilter}
        conciliacaoStatusOpen={conciliacaoStatusOpen}
        filtersDispatch={dispatchConciliacao}
        setExportConfig={setExportConfig}
      />

      {/* Verificações técnicas de integridade (duplicatas, órfãos, alocação).
          Ficam separadas do ledger operacional acima porque não são uma
          movimentação: são um diagnóstico somente leitura sobre os registros. */}
      <ReconciliationReportSection />

      {exportConfig && <ExportReportModal config={exportConfig} onClose={() => setExportConfig(null)} />}
    </div>
  );
}
