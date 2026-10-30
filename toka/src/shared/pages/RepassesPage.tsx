import { PageState, PageStateSkeleton } from "../ui/page-state";
import { useCallback, useState, type Dispatch } from "react";
import { ArrowLeftRight } from "lucide-react";
import { toast } from "sonner";

import { RepassesSection } from "./Finance/RepassesSection";
import type { FilterAction } from "./Finance/state";
import { useFinanceContext } from "../context/FinanceContext";
import { useContractsContext } from "../context/ContractsContext";
import { useFocusMode } from "../context/FocusModeContext";
import { useAuditLogContext } from "../context/AuditLogContext";
import { useFinancialModals } from "../hooks/useFinancialModals";
import { TransactionDetailSlideover } from "./Finance/TransactionDetailSlideover";
import { ProcessRepasseModal } from "../components/ProcessRepasseModal";
import { ExportReportModal } from "../components/ExportReportModal";
import { AuditTrailPanel } from "../components/AuditTrailPanel";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { FinanceAreaNav } from "../components/finance/FinanceAreaNav";

/**
 * Página "Repasses" — autossuficiente (transfers + transações merge, approve/
 * process e modais via useFinancialModals), extraída do mega-componente Finance.
 */
export default function RepassesPage() {
  const { contracts } = useContractsContext();
  const { focusMode } = useFocusMode();
  const { getAuditLog } = useAuditLogContext();
  const {
    transfers,
    approveTransfer,
    processTransfer,
    transactions,
    loading,
    errors,
    refetchTransfers,
    refetchTransactions,
  } = useFinanceContext();
  const modals = useFinancialModals();

  const [isExpanded, setIsExpanded] = useState(false);

  // Filtros do módulo de repasses.
  const [repasseSearch, setRepasseSearch] = useState("");
  const [repasseStatusFilter, setRepasseStatusFilter] = useState("Todos");
  const [repasseStatusOpen, setRepasseStatusOpen] = useState(false);
  const dispatchRepasse: Dispatch<FilterAction> = useCallback((action) => {
    if (!("type" in action)) return;
    const a = action as { type: string; tab?: string; search?: string; status?: string };
    if (a.type === "SET_TAB_SEARCH" && a.tab === "repasse") setRepasseSearch(a.search ?? "");
    else if (a.type === "TOGGLE_TAB_STATUS_OPEN" && a.tab === "repasse") setRepasseStatusOpen((v) => !v);
    else if (a.type === "SET_TAB_STATUS" && a.tab === "repasse") {
      setRepasseStatusFilter(a.status ?? "Todos");
      setRepasseStatusOpen(false);
    }
  }, []);

  if (errors?.transfers || errors?.transactions) {
    return (
      <div className="p-6 space-y-4">
        <FinanceAreaNav />
        <PageState
          error="Repasses incompletos: não foi possível carregar todas as origens."
          onRetry={() => { void refetchTransfers(); void refetchTransactions(); }}
        >{null}</PageState>
      </div>
    );
  }
  if (loading.transactions || loading.transfers) {
    return (
      <div className="p-6 space-y-4">
        <FinanceAreaNav />
        <PageState loading skeleton={<PageStateSkeleton rows={5} />}>{null}</PageState>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-7xl space-y-6 p-5 lg:p-6">
      <FinanceAreaNav />
      <div>
        <h1 className="text-2xl font-bold flex items-center gap-2 text-foreground">
          <ArrowLeftRight className="w-6 h-6 text-primary-600" /> Repasses
        </h1>
        <p className="text-sm text-muted-foreground mt-1">
          Repasse ao proprietário (locação e venda), com aprovação e processamento.
        </p>
      </div>

      <RepassesSection
        transfers={transfers}
        contracts={contracts}
        transactions={transactions}
        focusMode={focusMode}
        loadingInvoices={loading.transactions || loading.transfers}
        repasseSearch={repasseSearch}
        repasseStatusFilter={repasseStatusFilter}
        repasseStatusOpen={repasseStatusOpen}
        filtersDispatch={dispatchRepasse}
        approveTransfer={approveTransfer}
        processTransfer={processTransfer}
        openProcessRepasse={modals.openProcessRepasse}
        setSelectedTransaction={(tx) => { if (tx) modals.openTransaction(tx); else modals.close(); }}
        setExportConfig={(cfg: any) => { const c = typeof cfg === "function" ? null : cfg; if (c) modals.openExport(c); else modals.close(); }}
        setAudit={(sel: any) => { if (sel) modals.openAudit(sel.entityId, sel.label); else modals.close(); }}
        openConfirm={modals.openConfirm}
        closeConfirm={modals.closeConfirm}
      />

      {/* Slide-over de transação */}
      <TransactionDetailSlideover
        transaction={modals.state.modal === "transaction" ? modals.state.transaction : null}
        isExpanded={isExpanded}
        onToggleExpand={() => setIsExpanded((v) => !v)}
        onClose={() => modals.close()}
        onRegisterPayment={() => {}}
        title="Detalhes do repasse"
        canRegisterPayment={false}
        operationalNotice={
          modals.state.modal === "transaction"
            && (modals.state.transaction as typeof modals.state.transaction & { _includedInTotals?: boolean })?._includedInTotals === false
            ? "A apurar: este registro legado não possui vínculo econômico confiável e permanece somente leitura até a conciliação da origem."
            : undefined
        }
      />

      {/* Processar repasse (com upload de comprovante — fluxo externo) */}
      {modals.state.modal === "processRepasse" && (
        <ProcessRepasseModal
          transferId={modals.state.transferId}
          transfer={modals.state.transfer}
          onClose={() => modals.close()}
          onProcess={async (transferId, evidence) => {
            await processTransfer(transferId, evidence);
            modals.close();
            toast.success("Repasse processado", { description: "Comprovante registrado no audit trail." });
          }}
        />
      )}

      {/* Export */}
      {modals.state.modal === "export" && (
        <ExportReportModal config={modals.state.config} onClose={() => modals.close()} />
      )}

      {/* Audit trail */}
      {modals.state.modal === "audit" && (
        <AuditTrailPanel
          entityId={modals.state.entityId === "__all__" ? undefined : modals.state.entityId}
          entityLabel={modals.state.label}
          onClose={() => modals.close()}
          getAuditLog={getAuditLog}
        />
      )}

      {/* Confirm dialog */}
      <ConfirmDialog
        open={!!modals.confirm}
        title={modals.confirm?.title ?? ""}
        description={modals.confirm?.description ?? ""}
        confirmLabel={modals.confirm?.confirmLabel}
        variant={modals.confirm?.variant}
        doubleConfirm={modals.confirm?.doubleConfirm}
        doubleConfirmLabel={modals.confirm?.doubleConfirmLabel}
        onConfirm={modals.confirm?.onConfirm ?? (() => {})}
        onCancel={modals.closeConfirm}
      />
    </div>
  );
}
