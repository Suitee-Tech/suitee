import { PageState, PageStateSkeleton } from "../ui/page-state";
import { useCallback, useEffect, useMemo, useState, type Dispatch } from "react";
import { Receipt } from "lucide-react";
import { useNavigate } from "react-router";

import { CobrancasSection } from "./Finance/CobrancasSection";
import type { FilterAction } from "./Finance/state";
import { mapInvoiceToTransaction, formatCurrency } from "./Finance/presentation";
import { useFinanceContext } from "../context/FinanceContext";
import { SettleLancamentoModal, type SettleLancamentoTarget } from "../components/lancamentos/SettleLancamentoModal";
import { resolveReceivableAction } from "../lib/finance/receivableAction";
import { toast } from "sonner";
import { useContractsContext } from "../context/ContractsContext";
import { usePropertiesContext } from "../context/PropertiesContext";
import { useLeadsContext } from "../context/LeadsContext";
import { useFocusMode } from "../context/FocusModeContext";
import { useAuditLogContext } from "../context/AuditLogContext";
import { invoiceService } from "../services/invoice.service";
import { useFinancialModals } from "../hooks/useFinancialModals";
import { formatPropertyAddress } from "../data";
import type { Charge } from "../services/charge.service";
import type { Transaction } from "../types/transaction";
import { TransactionDetailSlideover } from "./Finance/TransactionDetailSlideover";
import { TransactionEditModal } from "../components/TransactionEditModal";
import { NovaCobrancaModal } from "../components/NovaCobrancaModal";
import { MarkPaidModal } from "../components/MarkPaidModal";
import { ChargeReceiptModal } from "../components/ChargeReceiptModal";
import { ChargeDetailDrawer } from "../components/ChargeDetailDrawer";
import { CancelarCobrancaDialog } from "../components/CancelarCobrancaDialog";
import { ExportReportModal } from "../components/ExportReportModal";
import { AuditTrailPanel } from "../components/AuditTrailPanel";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { FinanceAreaNav } from "../components/finance/FinanceAreaNav";
import { chargeReceivableAmounts } from "../lib/finance/economicRecords";

/**
 * Página "Recebimentos" — autossuficiente (charges + transações merge, modais
 * via useFinancialModals), extraída do mega-componente Finance.
 */
export default function CobrancasPage() {
  const navigate = useNavigate();
  const { contracts } = useContractsContext();
  const { properties } = usePropertiesContext();
  const { leads } = useLeadsContext();
  const { focusMode } = useFocusMode();
  const { getAuditLog } = useAuditLogContext();
  const {
    charges,
    addCharge,
    markChargePaid,
    updateCharge,
    cancelCharge,
    notifyCharge,
    transactions: contextTransactions,
    errors: financeErrors,
    loading: financeLoading,
    refetchCharges,
    refetchTransactions,
  } = useFinanceContext();
  const modals = useFinancialModals();

  const [isExpanded, setIsExpanded] = useState(false);

  // Merge de transações (contexto + invoices).
  const [apiTransactions, setApiTransactions] = useState<Transaction[]>([]);
  const [loadingInvoices, setLoadingInvoices] = useState(true);
  const [invoiceError, setInvoiceError] = useState(false);
  const loadApiInvoices = useCallback(async () => {
    try {
      setLoadingInvoices(true);
      setInvoiceError(false);
      const invoices = await invoiceService.list();
      setApiTransactions(invoices.map(mapInvoiceToTransaction));
    } catch {
      setInvoiceError(true);
    } finally {
      setLoadingInvoices(false);
    }
  }, []);
  useEffect(() => { loadApiInvoices(); }, [loadApiInvoices]);

  const transactions = useMemo(() => {
    const merged = new Map<string, Transaction>();
    contextTransactions.forEach((t) => merged.set(t.id, t));
    apiTransactions.forEach((t) => { if (!merged.has(t.id)) merged.set(t.id, t); });
    return Array.from(merged.values());
  }, [contextTransactions, apiTransactions]);

  // Filtros do módulo de cobranças.
  const [activeTab, setActiveTab] = useState<"todas" | "cobrancas">("todas");
  const [cobrancaSearch, setCobrancaSearch] = useState("");
  const [cobrancaStatusFilter, setCobrancaStatusFilter] = useState("Todos");
  const [cobrancaStatusOpen, setCobrancaStatusOpen] = useState(false);
  const dispatchCobranca: Dispatch<FilterAction> = useCallback((action) => {
    if (!("type" in action)) return;
    const a = action as { type: string; tab?: string; status?: string; search?: string };
    if (a.type === "SET_COBRANCA_SUB_TAB") setActiveTab((a.tab as "todas" | "cobrancas") ?? "todas");
    else if (a.type === "SET_TAB_SEARCH" && a.tab === "cobranca") setCobrancaSearch(a.search ?? "");
    else if (a.type === "TOGGLE_TAB_STATUS_OPEN" && a.tab === "cobranca") setCobrancaStatusOpen((v) => !v);
    else if (a.type === "SET_TAB_STATUS" && a.tab === "cobranca") { setCobrancaStatusFilter(a.status ?? "Todos"); setCobrancaStatusOpen(false); }
  }, []);

  const handleChargeAction = useCallback(async (action: "pay" | "edit" | "cancel" | "notify" | "receipt", charge: Charge) => {
    if (action === "pay") {
      const amounts = chargeReceivableAmounts(charge);
      modals.openMarkPaid(charge.id, amounts.pending + amounts.overdue, charge);
    }
    else if (action === "edit") modals.openEditCharge(charge.id);
    else if (action === "cancel") modals.openCancelCharge(charge.id);
    else if (action === "notify") { try { await notifyCharge(charge.id); } catch { /* toast já tratado */ } }
    else if (action === "receipt") modals.openReceipt(charge.id);
  }, [modals, notifyCharge]);

  // Baixa padronizada: abre o MESMO painel lateral dos demais fluxos do
  // financeiro (método + data + comprovante + observação). Antes esta tela
  // abria um AlertDialog que só trocava o status, sem capturar a evidência.
  const handleRegisterPayment = useCallback((tx: any, defaultAmount?: number) => {
    // Mesma decisão por origem do módulo Financeiro (uma regra só, testada em
    // lib/finance/receivableAction): cobrança — linha de cobrança ou fatura
    // vinculada a ela — usa o painel de baixa de cobrança; lançamento
    // independente usa a baixa padronizada; sem vínculo, orienta conciliação.
    const action = resolveReceivableAction(tx, { defaultAmount });
    if (action.kind === "blocked") {
      toast.warning(
        action.reason === "read-only"
          ? "Registros a apurar são somente leitura. Concilie a origem antes de movimentar valores."
          : "Esta fatura é uma projeção sem cobrança vinculada. Concilie a origem antes de dar baixa.",
      );
      return;
    }
    if (action.kind === "charge") {
      modals.openMarkPaid(action.chargeId, action.defaultAmount);
      return;
    }
    setSettleTarget({
      id: action.id,
      type: action.type,
      description: action.description,
      value: action.value,
    });
  }, [modals]);

  // Payloads de modais derivados do estado do hook.
  const [settleTarget, setSettleTarget] = useState<SettleLancamentoTarget | null>(null);

  const detailChargeId = modals.state.modal === "chargeDetail" ? modals.state.chargeId : null;
  const detailCharge = detailChargeId ? charges.find((c) => c.id === detailChargeId) : null;
  const markingPaid = modals.state.modal === "markPaid" ? modals.state : null;
  const receiptChargeId = modals.state.modal === "receipt" ? modals.state.chargeId : null;
  const receiptCharge = receiptChargeId ? charges.find((c) => c.id === receiptChargeId) : null;
  const editingChargeId = modals.state.modal === "editCharge" ? modals.state.chargeId : null;
  const editingCharge = editingChargeId ? charges.find((c) => c.id === editingChargeId) : null;
  const cancellingChargeId = modals.state.modal === "cancelCharge" ? modals.state.chargeId : null;
  const cancellingCharge = cancellingChargeId ? charges.find((c) => c.id === cancellingChargeId) : null;

  if (invoiceError || financeErrors?.charges || financeErrors?.transactions) {
    return (
      <div className="p-6 space-y-4">
        <FinanceAreaNav />
        <PageState
          error="Recebimentos incompletos: não foi possível carregar todas as cobranças e documentos."
          onRetry={() => { void loadApiInvoices(); void refetchCharges(); void refetchTransactions(); }}
        >{null}</PageState>
      </div>
    );
  }
  if (loadingInvoices || financeLoading?.charges || financeLoading?.transactions) {
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
          <Receipt className="w-6 h-6 text-primary-600" /> Recebimentos
        </h1>
        <p className="text-sm text-muted-foreground mt-1">
          Cobranças a receber, pagamentos registrados e conferência do fluxo de recebimentos.
        </p>
      </div>

      <CobrancasSection
        charges={charges}
        contracts={contracts}
        transactions={transactions}
        focusMode={focusMode}
        loadingInvoices={loadingInvoices}
        activeTab={activeTab}
        cobrancaSearch={cobrancaSearch}
        cobrancaStatusFilter={cobrancaStatusFilter}
        cobrancaStatusOpen={cobrancaStatusOpen}
        filtersDispatch={dispatchCobranca}
        navigate={navigate}
        onRegisterPayment={handleRegisterPayment}
        setSelectedTransaction={(tx) => { if (tx) modals.openTransaction(tx); else modals.close(); }}
        setEditing={(sel: any) => { if (sel) modals.openEditTransaction(sel.tx, sel.mode); else modals.close(); }}
        setShowNovaCobranca={(open) => { if (open) modals.openNovaCobranca(); else modals.close(); }}
        setSelectedChargeId={(id: any) => { const c = typeof id === "function" ? null : id; if (c) modals.openChargeDetail(c); else modals.close(); }}
        setExportConfig={(cfg: any) => { const c = typeof cfg === "function" ? null : cfg; if (c) modals.openExport(c); else modals.close(); }}
        setAudit={(sel: any) => { if (sel) modals.openAudit(sel.entityId, sel.label); else modals.close(); }}
        handleChargeAction={handleChargeAction}
      />

      {settleTarget && (
        <SettleLancamentoModal
          lancamento={settleTarget}
          onClose={() => setSettleTarget(null)}
          onSaved={() => {
            setSettleTarget(null);
            modals.close();
            void refetchTransactions();
            void refetchCharges();
          }}
        />
      )}

      <TransactionDetailSlideover
        transaction={modals.state.modal === "transaction" ? modals.state.transaction : null}
        isExpanded={isExpanded}
        onToggleExpand={() => setIsExpanded((v) => !v)}
        onClose={() => modals.close()}
        onRegisterPayment={handleRegisterPayment}
        title="Detalhes do recebimento"
      />

      {modals.state.modal === "editTransaction" && (
        <TransactionEditModal transaction={modals.state.tx} mode={modals.state.mode} onClose={() => modals.close()} />
      )}

      {modals.state.modal === "novaCobranca" && (
        <NovaCobrancaModal
          contracts={contracts}
          properties={properties}
          onClose={() => modals.close()}
          onSave={async (data) => { await addCharge(data as any); modals.close(); }}
        />
      )}

      {editingCharge && (
        <NovaCobrancaModal
          mode="edit"
          initial={{
            id: editingCharge.id,
            status: editingCharge.status,
            contract_id: editingCharge.contract_id,
            type: editingCharge.type,
            description: editingCharge.description,
            reference_month: editingCharge.reference_month,
            amount: editingCharge.amount,
            due_date: editingCharge.due_date,
            discount: editingCharge.discount,
            line_items: editingCharge.line_items,
          }}
          contracts={contracts}
          properties={properties}
          onClose={() => modals.close()}
          onSave={async (data, id) => { await updateCharge(id, data); modals.close(); }}
        />
      )}

      {markingPaid && (
        <MarkPaidModal
          chargeId={markingPaid.chargeId}
          defaultAmount={markingPaid.defaultAmount || 0}
          charge={markingPaid.charge}
          onClose={() => modals.close()}
          onSave={async (chargeId, data) => { await markChargePaid(chargeId, data); }}
        />
      )}

      {receiptCharge && (
        <ChargeReceiptModal
          charge={receiptCharge}
          contractDisplayID={contracts.find((c) => c.id === receiptCharge.contract_id)?.display_id || undefined}
          payerName={contracts.find((c) => c.id === receiptCharge.contract_id)?.client}
          onClose={() => modals.close()}
        />
      )}

      {detailCharge && (
        <ChargeDetailDrawer
          charge={detailCharge}
          contract={(() => {
            const c = contracts.find((x) => x.id === detailCharge.contract_id);
            return c ? { id: c.id, property_title: c.property_title, client: c.client } : undefined;
          })()}
          propertyLabel={(() => {
            const p = properties.find((x) => x.id === detailCharge.property_id);
            return p?.title || (p?.address ? formatPropertyAddress(p.address) : undefined);
          })()}
          tenantLabel={contracts.find((c) => c.id === detailCharge.contract_id)?.client}
          ownerLabel={(() => {
            const p = properties.find((x) => x.id === detailCharge.property_id);
            const ownerId = p?.owners?.[0]?.id;
            if (!ownerId) return undefined;
            return leads.find((l) => l.id === ownerId)?.name ?? ownerId;
          })()}
          onClose={() => modals.close()}
          onAction={(action, charge) => {
            if (action !== "notify" && action !== "receipt") modals.close();
            handleChargeAction(action, charge);
          }}
        />
      )}

      {cancellingCharge && (
        <CancelarCobrancaDialog
          open={!!cancellingChargeId}
          contextLine={cancellingCharge.description}
          onCancel={() => modals.close()}
          onConfirm={async (reason) => {
            try { await cancelCharge(cancellingCharge.id, reason); modals.close(); } catch { /* mantém aberto */ }
          }}
        />
      )}

      {modals.state.modal === "export" && (
        <ExportReportModal config={modals.state.config} onClose={() => modals.close()} />
      )}

      {modals.state.modal === "audit" && (
        <AuditTrailPanel
          entityId={modals.state.entityId === "__all__" ? undefined : modals.state.entityId}
          entityLabel={modals.state.label}
          onClose={() => modals.close()}
          getAuditLog={getAuditLog}
        />
      )}

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
