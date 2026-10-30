import { useConfirmAction } from "../hooks/useConfirmAction";
import React, { useState, useMemo, useEffect, useDeferredValue, useRef } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { contractService } from "../services/contract.service";
import { transferService } from "../services/transfer.service";
import { propertyService, type PropertyDTO } from "../services/property.service";
import { unwrapApiData } from "../services/api";
import { mapPropertyDtoToProperty } from "../lib/properties/mappers";
import { contractsKeys } from "../contracts/queries";
import {
 Banknote, Bell, Building2, CheckCircle2, Download, Droplet, FileText,
 Flame, History, Receipt, Send, ShieldCheck, Zap,
} from "lucide-react";
import { clsx } from "clsx";
import { Button } from "../ui/button";
import { Contract, ContractStatus } from "../contracts/types";
import { NovaCobrancaModal } from "../components/RentalCreateModals";
import { MarkPaidModal } from "../components/MarkPaidModal";
import { ProcessRepasseModal } from "../components/ProcessRepasseModal";
import { ChargeReceiptModal } from "../components/ChargeReceiptModal";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { AuditTrailPanel as SharedAuditTrailPanel } from "../components/AuditTrailPanel";
import { ExpandableSidePanel } from "../components/ExpandableSidePanel";
import { useFocusMode } from "../context/FocusModeContext";
import { useIsMobile } from "../ui/use-mobile";
import { useLeadsContext } from "../context/LeadsContext";
import { usePropertiesContext } from "../context/PropertiesContext";
import { useFinanceContext } from "../context/FinanceContext";
import { useAgentsContext } from "../context/AgentsContext";
import {
  useContractsListQuery,
  useContractChargesQuery,
  useContractTransfersQuery,
  useMarkChargePaidMutation,
  useNotifyChargeMutation,
  useAdjustContractMutation,
  useApproveTransferMutation,
  useProcessTransferMutation,
  useTerminateContractMutation,
  useRentalChargeConfigQuery,
  useCreateRentalChargeConfigMutation,
  useUpdateRentalChargeConfigMutation,
  useGenerateRentalChargesMutation,
  useUtilityAccountsQuery,
  useUpdateUtilityAccountMutation,
  useDeleteUtilityAccountMutation,
  useReminderRulesQuery,
  useUpdateReminderRuleMutation,
  useDeleteReminderRuleMutation,
  useRunReminderRulesNowMutation,
} from "../hooks/useContracts";
import { UtilityAccountModal } from "../components/UtilityAccountModal";
import { UtilityChargeModal } from "../components/UtilityChargeModal";
import { BillingPlanWizard } from "../components/BillingPlanWizard";
import { ReminderRuleModal } from "../components/ReminderRuleModal";
import type { UtilityAccount, UtilityAccountType } from "../services/utilityAccount.service";
import type { ReminderRule } from "../services/reminderRule.service";
import { formatCurrencyBRL, formatISODateBR, parseMoneyBR } from "../utils/format";
import { buildContractIssues, showContractIssueToasts } from "../utils/contractIssues";
import { usePermissionsContext } from "../context/PermissionsContext";
import { useAuditLogContext } from "../context/AuditLogContext";
import type { DetailTab, ModalState, ViewState } from "./Rentals/presentation";
import { RentalSettingsPanel } from "./Rentals/RentalSettingsPanel";
import { RentalDetailView } from "./Rentals/RentalDetailView";
import { RentalListView } from "./Rentals/RentalListView";
import {
  RentalContractActionLauncher,
  type RentalHeaderAction,
} from "./Rentals/RentalContractActionLauncher";
import { tenantFinancialSettingsService } from "../services/tenantFinancialSettings.service";
import { availableChargeActions, mapChargeStatus } from "../utils/charge";
import {
  chargeReceivableAmounts,
  reconcileReceivableTransactions,
  reconcileTransferTransactions,
  transactionReceivableAmounts,
} from "../lib/finance/economicRecords";
import { normalizeStatus } from "../lib/lancamentos/status";
import { getOwnerTransferPresentation } from "../lib/finance/ownerTransferFlow";
import { notifyApiError } from "../services/errorToast";

export function Rentals() {
 const { confirmAction, confirmationDialog } = useConfirmAction();
 const { leads } = useLeadsContext();
 const { properties, fetchProperties } = usePropertiesContext();
 const { transactions } = useFinanceContext();
 const { agents } = useAgentsContext();
 const { focusMode } = useFocusMode();
 const isMobile = useIsMobile();
 const navigate = useNavigate();
 const [searchParams] = useSearchParams();
 const queryClient = useQueryClient();
 const { can } = usePermissionsContext();
 const { getAuditLog } = useAuditLogContext();

 // Permission gates — derived once per render and consumed by the buttons
 // throughout the page. The backend enforces the same checks (routes.go in
 // toca-financial and contract_handler.go in toca-deal), so hiding here is
 // a UX concern (operators shouldn't see buttons that 403 on click) rather
 // than a security boundary.
 const perms = useMemo(() => ({
   chargesWrite: can('charges:write'),
   chargesRead: can('charges:read'),
   financeRead: can('finance:read'),
   financeWrite: can('finance:write'),
   financeApproveTransfer: can('finance:approve_transfer'),
   contractsWrite: can('contracts:write'),
 }), [can]);

 // Contracts now come from React Query so cache invalidation and refetch are
 // first-class. CRMContext is kept around only for cross-entity enrichment
 // (leads/properties) and legacy transactions, not as the source of truth.
 const { data: contractsRaw = [], isLoading: contractsLoading } = useContractsListQuery();
 const { data: tenantFinancialSettings } = useQuery({
   queryKey: ["financial-settings"],
   queryFn: () => tenantFinancialSettingsService.get(),
   staleTime: 5 * 60_000,
   retry: false,
 });
 const defaultAdminFeePct = tenantFinancialSettings?.admin_fee_pct ?? 10;
 const defaultLateFeePct = tenantFinancialSettings?.default_late_fee_pct ?? 2;
 const defaultMonthlyInterestPct = tenantFinancialSettings?.default_monthly_interest_pct ?? 1;
 const defaultDueDay = tenantFinancialSettings?.default_due_day ?? 10;

 // Enrich each contract with client/property fallbacks derived from the
 // already-loaded CRM data, so rows render properly even for legacy records
 // that never had the denormalized fields populated server-side.
 const contracts = useMemo<Contract[]>(() => {
   return contractsRaw.map((c) => {
     const lead = c.customer_id ? leads.find((l) => l.id === c.customer_id) : undefined;
     const property = c.property_id ? properties.find((p) => p.id === c.property_id) : undefined;
     const client = c.client && c.client.trim().length > 0 ? c.client : lead?.name ?? "";
     const property_title =
       c.property_title && c.property_title.trim().length > 0
         ? c.property_title
         : property?.title ?? "";
     return { ...c, client, property_title } as Contract;
   });
 }, [contractsRaw, leads, properties]);

 // State for filtering / sorting
 const [contractTypeFilter, setContractTypeFilter] = useState<'Todos' | 'locacao'>('Todos');
 const [statusFilter, setStatusFilter] = useState<'Todos' | 'Ativos'>('Todos');
 const [sortBy, setSortBy] = useState<'Inquilino' | 'Valor'>('Inquilino');
 const [searchInput, setSearchInput] = useState('');
 const deferredSearch = useDeferredValue(searchInput);

 const filteredContracts = useMemo(() => {
   const term = deferredSearch.trim().toLowerCase();
   const filtered = contracts.filter((c) => {
     if (c.status === ContractStatus.DRAFT) return false;
     if (c.type !== 'locacao' && c.type !== 'RENTAL') return false;
     if (statusFilter === 'Ativos') {
       const isActive = c.status === ContractStatus.SIGNED || c.status === ContractStatus.ACTIVE;
       if (!isActive) return false;
     }
     if (term) {
       const haystack = `${c.client ?? ''} ${c.property_title ?? ''} ${c.display_id ?? c.id}`.toLowerCase();
       if (!haystack.includes(term)) return false;
     }
     return true;
   });
   const sorted = [...filtered];
   if (sortBy === 'Inquilino') {
     sorted.sort((a, b) => (a.client ?? '').localeCompare(b.client ?? '', 'pt-BR'));
   } else {
     sorted.sort((a, b) => (b.value ?? 0) - (a.value ?? 0));
   }
   return sorted;
 }, [contracts, deferredSearch, statusFilter, sortBy]);

 const activeContracts = useMemo(
   () => contracts.filter(
     (c) =>
       (c.type === 'locacao' || c.type === 'RENTAL') &&
       (c.status === ContractStatus.SIGNED || c.status === ContractStatus.ACTIVE)
   ),
   [contracts]
 );

 const managedRentTotal = useMemo(
   () => activeContracts.reduce((acc, c) => acc + (c.value || 0), 0),
   [activeContracts]
 );

 const agencyAdminRevenue = useMemo(
   () => activeContracts.reduce(
     (acc, contract) => acc + (contract.value || 0) * ((contract.commission_pct ?? defaultAdminFeePct) / 100),
     0,
   ),
   [activeContracts, defaultAdminFeePct],
 );

 const upcomingAdjustments = useMemo(() => {
   const threshold = Date.now() + 90 * 24 * 60 * 60 * 1000;
   return activeContracts.filter((c) => {
     const next = c.next_adjustment ?? c.nextAdjustment;
     if (!next) return false;
     const ts = new Date(next).getTime();
     return Number.isFinite(ts) && ts < threshold;
   }).length;
 }, [activeContracts]);

 const [view, setView] = useState<ViewState>('list');
 const [activeModal, setActiveModal] = useState<ModalState>('none');
 const [selectedContractId, setSelectedContractId] = useState<string | null>(null);
 const selectedContract = useMemo(
   () => contracts.find((contract) => contract.id === selectedContractId) ?? null,
   [contracts, selectedContractId],
 );
 const setSelectedContract = (contract: Contract | null) => setSelectedContractId(contract?.id ?? null);
 const [detailTab, setDetailTab] = useState<DetailTab>('geral');
 const [auditContractId, setAuditContractId] = useState<string | null>(null);
 const [auditLabel, setAuditLabel] = useState('');
 const [chargeFilter, setChargeFilter] = useState<string>('Todos');
 // Cobranças tab — extra slice/sort controls layered on top of the status
 // pills. All client-side so the filter pipeline can respond instantly while
 // the underlying query stays cached.
 const [chargeSearch, setChargeSearch] = useState<string>('');
 const deferredChargeSearch = useDeferredValue(chargeSearch);
 const [chargeSortBy, setChargeSortBy] = useState<'date_desc' | 'date_asc' | 'value_desc' | 'value_asc'>('date_desc');
 const [chargePeriod, setChargePeriod] = useState<'all' | 'this_month' | 'last_3' | 'last_6'>('all');

 // Settings Panel Tabs
 const [settingsTab, setSettingsTab] = useState<'plano' | 'contas' | 'regua'>('plano');
 const [headerAction, setHeaderAction] = useState<RentalHeaderAction | null>(null);
 const [showRentalComposition, setShowRentalComposition] = useState(false);
 const [utilityChargeAccountId, setUtilityChargeAccountId] = useState<string | null>(null);
 const [markingPaidChargeId, setMarkingPaidChargeId] = useState<string | null>(null);
 const [receiptChargeId, setReceiptChargeId] = useState<string | null>(null);
 const [processingTransferId, setProcessingTransferId] = useState<string | null>(null);
 const [bulkApprovalOpen, setBulkApprovalOpen] = useState(false);
 const handledDeepLinkRef = useRef<string | null>(null);

 useEffect(() => {
   const contractId = searchParams.get('contract');
   if (!contractId || contracts.length === 0) return;
   const deepLinkKey = `${contractId}:${searchParams.get('tab') ?? ''}:${searchParams.get('setup') ?? ''}`;
   if (handledDeepLinkRef.current === deepLinkKey) return;
   const contract = contracts.find((item) => item.id === contractId);
   if (!contract) return;
   handledDeepLinkRef.current = deepLinkKey;
   setSelectedContract(contract);
   setView('detail');
   const requestedTab = searchParams.get('tab');
   if (requestedTab && ['geral', 'financeiro', 'contas', 'cobrancas', 'repasses', 'documentos'].includes(requestedTab)) {
     setDetailTab(requestedTab as DetailTab);
   } else {
     setDetailTab('financeiro');
   }
   if (searchParams.get('setup') === '1') {
     setSettingsTab('plano');
     setActiveModal('cobrancas');
   }
 }, [contracts, searchParams]);

 const formatCurrency = (val: number) => formatCurrencyBRL(val);

 // Charges for the selected contract: pulled live from /charges via React
 // Query. The CRMContext path stayed too out-of-sync after pagamentos and
 // novas cobranças (it needed a manual refetch), so we go to the API.
 const { data: realCharges = [] } = useContractChargesQuery(selectedContract?.id);
 const contractTransactions = useMemo(
   () => transactions.filter((transaction) => transaction.contractId === selectedContract?.id),
   [selectedContract?.id, transactions],
 );
 const receivableReconciliation = useMemo(
   () => reconcileReceivableTransactions(realCharges, contractTransactions),
   [contractTransactions, realCharges],
 );

 // --- Derive charges from React Query + legacy CRMContext transactions ---
 const charges = useMemo(() => {
   if (!selectedContract) return [];

   const apiCharges = realCharges.map((c) => {
     const reminders = c.reminders_sent ?? [];
     const lastReminder = reminders.length > 0 ? reminders[reminders.length - 1] : null;
     const mappedStatus = mapChargeStatus(c.status);
     const displayStatus = mappedStatus === 'Pendente' ? 'A Vencer' : mappedStatus;
     const amounts = chargeReceivableAmounts(c);
     return {
       id: c.id,
       description: c.description || `Cobrança ${c.reference_month}`,
       type: c.type || 'Aluguel',
       value: c.total_amount,
       dueDate: c.due_date ? c.due_date.split('T')[0] : '',
       paymentDate: c.paid_at ? c.paid_at.split('T')[0] : null,
       status: displayStatus,
       filterStatus: mappedStatus === 'Cancelada' || mappedStatus === 'Reversada' ? 'Encerrada' : displayStatus,
       method: c.payment_method || null,
       remindersCount: reminders.length,
       lastReminderAt: lastReminder?.sent_at ?? null,
       remindersSent: reminders,
       referenceMonth: c.reference_month,
       _source: 'charge' as const,
       _charge: c,
       _canMarkPaid: availableChargeActions(c.status).includes('pay') && amounts.pending + amounts.overdue > 0,
       _canNotify: availableChargeActions(c.status).includes('notify'),
       _remainingAmount: amounts.pending + amounts.overdue,
       _includedInTotals: true,
       _paidAmount: amounts.paid,
       _pendingAmount: amounts.pending,
       _lateAmount: amounts.overdue,
     };
   });

   const unresolvedProjectionIDs = new Set(receivableReconciliation.unresolvedProjectionIDs);
   const txCharges = receivableReconciliation.transactions
     .filter((t) => t.dueDate)
     .map((t) => {
       const unresolved = unresolvedProjectionIDs.has(t.id);
       const canonicalStatus = normalizeStatus('transaction', t.status);
       const status = unresolved
         ? 'A apurar'
         : canonicalStatus === 'pago'
           ? 'Pago'
           : canonicalStatus === 'vencido'
             ? 'Em Atraso'
             : canonicalStatus === 'parcial'
               ? 'Parcial'
               : canonicalStatus === 'cancelado' || canonicalStatus === 'revertido'
                 ? 'Encerrada'
                 : 'A Vencer';
       const amounts = unresolved
         ? { paid: 0, pending: 0, overdue: 0 }
         : transactionReceivableAmounts(t);
       return {
       id: t.id,
       description: t.description,
       type: t.category || 'Outros',
       value: t.value,
       dueDate: t.dueDate!,
       paymentDate: t.paymentDate || null,
       status,
       filterStatus: status,
       method: t.paymentMethod || null,
       remindersCount: 0,
       lastReminderAt: null,
       remindersSent: [] as { sent_at: string; channels: string[]; recipient?: string; sent_by?: string; sent_by_label?: string }[],
       referenceMonth: t.month ?? '',
       _source: 'transaction' as const,
       _charge: undefined,
       _canMarkPaid: false,
       _canNotify: false,
       _remainingAmount: 0,
       _includedInTotals: !unresolved,
       _paidAmount: amounts.paid,
       _pendingAmount: amounts.pending,
       _lateAmount: amounts.overdue,
     };
   });

   return [...apiCharges, ...txCharges].sort((a, b) => b.dueDate.localeCompare(a.dueDate));
 }, [realCharges, receivableReconciliation, selectedContract]);

 // --- Repasses for the selected contract: /transfers via React Query ---
 const { data: realTransfers = [] } = useContractTransfersQuery(selectedContract?.id);
 const transferReconciliation = useMemo(
   () => reconcileTransferTransactions(realTransfers, contractTransactions),
   [contractTransactions, realTransfers],
 );

 const repasses = useMemo(() => {
   if (!selectedContract) return [];
   const apiRepasses = realTransfers.map((t) => {
     const presentation = getOwnerTransferPresentation(t);
     return {
     id: t.id,
     month: t.reference_month || '',
     grossRent: t.gross_rent || 0,
     adminFee: t.admin_fee_amount || 0,
     maintenanceDeductions: t.maintenance_deductions || 0,
     utilityDeductions: 0,
     netRepasse: t.net_amount || 0,
     status: presentation.status,
     action: presentation.action,
     settlementReady: presentation.settlementReady,
     paymentDate: t.processed_at ? t.processed_at.split('T')[0] : null,
     approvedAt: t.approved_at ?? null,
     approvedBy: t.approved_by ?? null,
     approvedLabel: t.history?.find((h) => h.action === 'approved')?.actor_label ?? null,
     processedAt: t.processed_at ?? null,
     processedBy: t.processed_by ?? null,
     processedLabel: t.history?.find((h) => h.action === 'processed')?.actor_label ?? null,
     history: t.history ?? [],
     _source: 'transfer' as const,
     _transfer: t,
   };
   });
   const txRepasses = transferReconciliation.transactions
     .map((t) => ({
       id: t.id,
       month: t.month || '',
       grossRent: t.grossValue || 0,
       adminFee: t.adminFee || 0,
       maintenanceDeductions: t.maintenanceDeductions || 0,
       utilityDeductions: t.utilityDeductions || 0,
       netRepasse: t.value,
       status: 'A apurar' as const,
       action: null,
       settlementReady: false,
       paymentDate: t.paymentDate || null,
       approvedAt: null,
       approvedBy: null,
       approvedLabel: null,
       processedAt: t.paymentDate ?? null,
       processedBy: null,
       processedLabel: null,
       history: [] as { at: string; action: string; actor?: string; actor_label?: string; note?: string }[],
       _source: 'transaction' as const,
       _transfer: undefined,
     }));
   return [...apiRepasses, ...txRepasses];
 }, [realTransfers, selectedContract, transferReconciliation]);

 // --- Derive financial history from real charges + transfers (last 6 months) ---
 const financialHistory = useMemo(() => {
 if (!selectedContract) return [];
 const monthLabelsPt = ['Jan', 'Fev', 'Mar', 'Abr', 'Mai', 'Jun', 'Jul', 'Ago', 'Set', 'Out', 'Nov', 'Dez'];

 // Build the trailing 6-month window anchored on today, in chronological order.
 const now = new Date();
 const window: { key: string; label: string }[] = [];
 for (let i = 5; i >= 0; i--) {
 const d = new Date(now.getFullYear(), now.getMonth() - i, 1);
 const monthNum = String(d.getMonth() + 1).padStart(2, '0');
 window.push({ key: `${d.getFullYear()}-${monthNum}`, label: monthLabelsPt[d.getMonth()] });
 }

 const monthMap: Record<string, { receita: number; despesa: number; repasse: number }> = {};
 window.forEach((w) => { monthMap[w.key] = { receita: 0, despesa: 0, repasse: 0 }; });

 // Recebimentos brutos: inclui a parcela efetivamente paga de uma cobrança
 // parcial e exclui estados cancelados/reversados.
 realCharges.forEach((c) => {
 if (!c.reference_month) return;
 // reference_month comes from the backend either as YYYY-MM or MM/YYYY
 const ref = c.reference_month.includes('/')
 ? c.reference_month.split('/').reverse().join('-')
 : c.reference_month;
 if (!monthMap[ref]) return;
 monthMap[ref].receita += chargeReceivableAmounts(c).paid;
 });

 // Repasse e deduções só entram no histórico realizado após processamento.
 realTransfers.forEach((t) => {
 if (normalizeStatus('transfer', t.status) !== 'processado') return;
 const ref = t.reference_month;
 if (!ref || !monthMap[ref]) return;
 monthMap[ref].repasse += t.net_amount || 0;
 monthMap[ref].despesa += (t.admin_fee_amount || 0) + (t.maintenance_deductions || 0) + (t.other_deductions || 0);
 });

 // Projeções ligadas a charges/transfers já foram contabilizadas acima. Um
 // repasse legado sem vínculo continua visível como "A apurar", mas não entra
 // no gráfico realizado até haver identidade econômica confiável.
 const consolidatedTransactionIDs = new Set([
   ...receivableReconciliation.consolidatedTransactionIDs,
   ...transferReconciliation.consolidatedTransactionIDs,
 ]);
 const unresolvedProjectionIDs = new Set(receivableReconciliation.unresolvedProjectionIDs);
 contractTransactions.forEach((tx) => {
 if (consolidatedTransactionIDs.has(tx.id) || unresolvedProjectionIDs.has(tx.id) || tx.type === 'repasse') return;
 const monthNum = tx.date.split('-')[1];
 const year = tx.date.split('-')[0];
 const key = year && monthNum ? `${year}-${monthNum}` : null;
 if (!key || !monthMap[key]) return;
 if (tx.type === 'receita') monthMap[key].receita += transactionReceivableAmounts(tx).paid;
 else if (tx.type === 'despesa' && normalizeStatus('transaction', tx.status) === 'pago') monthMap[key].despesa += tx.value;
 });

 return window.map((w) => ({ month: w.label, ...monthMap[w.key] }));
 }, [contractTransactions, realCharges, realTransfers, receivableReconciliation, selectedContract, transferReconciliation]);

 const openModal = (type: ModalState, contract: any) => {
 setSelectedContract(contract);
 if (type === 'cobrancas') setSettingsTab('plano');
 setActiveModal(type);
 };

 const viewDetail = (contract: any) => {
 setSelectedContract(contract);
 setDetailTab('geral');
 setView('detail');
 };

 const markChargePaidMutation = useMarkChargePaidMutation(selectedContract?.id);
 const notifyChargeMutation = useNotifyChargeMutation();
 const adjustContractMutation = useAdjustContractMutation();
 const approveTransferMutation = useApproveTransferMutation(selectedContract?.id);
 const processTransferMutation = useProcessTransferMutation(selectedContract?.id);
 const terminateContractMutation = useTerminateContractMutation();

 const {
   data: utilityAccounts = [],
   isLoading: utilityAccountsLoading,
 } = useUtilityAccountsQuery(selectedContract?.id);
 const updateUtilityAccountMutation = useUpdateUtilityAccountMutation(selectedContract?.id);
 const deleteUtilityAccountMutation = useDeleteUtilityAccountMutation(selectedContract?.id);
 const selectedUtilityChargeAccount = useMemo(
   () => utilityAccounts.find((account) => account.id === utilityChargeAccountId) ?? null,
   [utilityAccounts, utilityChargeAccountId],
 );
 const selectedAdminFeePct = selectedContract?.commission_pct ?? defaultAdminFeePct;
 // Régua de Comunicação — rules are tenant-wide so the query has no
 // dependency on the selected contract.
 const {
   data: reminderRules = [],
   isLoading: reminderRulesLoading,
 } = useReminderRulesQuery();
 const updateReminderRuleMutation = useUpdateReminderRuleMutation();
 const deleteReminderRuleMutation = useDeleteReminderRuleMutation();
 const runRemindersNowMutation = useRunReminderRulesNowMutation();
 const [editingRule, setEditingRule] = useState<ReminderRule | null>(null);
 const [reminderRuleModalOpen, setReminderRuleModalOpen] = useState(false);
 // Charge audit panel — opens when the operator clicks the reminders badge
 // on a charge row. We store the charge id (not the object) so the panel
 // always reflects the latest React Query data after refetches.
 const [auditChargeId, setAuditChargeId] = useState<string | null>(null);
 // Repasse audit panel — same idea but keyed by transfer id. Reads the
 // history[] field that the OwnerTransfer domain already maintains
 // (created → approved → processed).
 const [auditTransferId, setAuditTransferId] = useState<string | null>(null);

 // Index agents by id for O(1) lookups. Brokers/agents are synced from auth
 // members so `Agent.id` is the same value that lands in JWT `account_id`,
 // which is what gets stamped onto audit entries as `user:<id>`.
 const agentsById = useMemo(() => {
   const map = new Map<string, { name: string; role?: string }>();
   for (const a of agents) {
     map.set(a.id, { name: a.name, role: a.role });
   }
   return map;
 }, [agents]);

 // Resolve a sent_by/actor token ("rule:abc-123", "user:account-99",
 // "system", "") to a human-friendly label. Pure derivation off the
 // reminderRules + agents data already loaded — no extra network calls.
 // Unknown formats degrade to the raw token so the operator can copy/paste
 // it into logs.
 const resolveSentByLabel = (sentBy?: string | null): { label: string; sublabel?: string } => {
   if (!sentBy) return { label: 'Manual' };
   if (sentBy === 'system') return { label: 'Sistema' };
   if (sentBy.startsWith('rule:')) {
     const ruleId = sentBy.slice(5);
     const rule = reminderRules.find((r) => r.id === ruleId);
     return {
       label: rule ? `Régua: ${rule.name}` : 'Régua (regra removida)',
       sublabel: rule?.description,
     };
   }
   if (sentBy.startsWith('user:')) {
     const id = sentBy.slice(5);
     const agent = agentsById.get(id);
     if (agent) {
       return { label: agent.name, sublabel: agent.role };
     }
     // Account never matched any broker — show the id as a fallback so it
     // stays traceable instead of silently dropping the attribution.
     return { label: `Operador ${id.slice(0, 8)}…`, sublabel: id };
   }
   return { label: sentBy };
 };

 // Resolve a raw account_id (no "user:" prefix). Used by the inline labels
 // in the repasses table where the backend writes the actor as the bare
 // account id rather than as a typed token.
 const resolveActorName = (rawActor?: string | null): string => {
   if (!rawActor) return '';
   if (rawActor === 'system') return 'Sistema';
   return agentsById.get(rawActor)?.name ?? rawActor;
 };

 const auditCharge = useMemo(
   () => (auditChargeId ? charges.find((c) => c.id === auditChargeId) ?? null : null),
   [auditChargeId, charges]
 );

 const auditTransfer = useMemo(
   () => (auditTransferId ? repasses.find((r) => r.id === auditTransferId) ?? null : null),
   [auditTransferId, repasses]
 );

 // --- Repasses filters (mirrors charges) ---
 const [repasseStatusFilter, setRepasseStatusFilter] = useState<
   'Todos' | 'Pendente' | 'Aprovado' | 'Processado' | 'Cancelado' | 'A apurar' | 'Sem saldo' | 'Liquidado sem saldo'
 >('Todos');
 const [repasseSearch, setRepasseSearch] = useState<string>('');
 const deferredRepasseSearch = useDeferredValue(repasseSearch);
 const [repasseSortBy, setRepasseSortBy] = useState<'month_desc' | 'month_asc' | 'value_desc' | 'value_asc'>('month_desc');
 const [repassePeriod, setRepassePeriod] = useState<'all' | 'last_3' | 'last_6' | 'last_12'>('all');

 const repassePeriodCutoff = useMemo(() => {
   if (repassePeriod === 'all') return null;
   const now = new Date();
   const months = repassePeriod === 'last_3' ? 3 : repassePeriod === 'last_6' ? 6 : 12;
   const d = new Date(now.getFullYear(), now.getMonth() - months, 1);
   // reference_month is "YYYY-MM" in API repasses; legacy ones use "MM/YYYY".
   // Build the cutoff in YYYY-MM for direct localeCompare against API entries
   // and short-circuit the legacy ones by always keeping them.
   return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
 }, [repassePeriod]);

 const filteredRepasses = useMemo(() => {
   const term = deferredRepasseSearch.trim().toLowerCase();
   let out = repasses;
   if (repasseStatusFilter !== 'Todos') {
     out = out.filter((r) => r.status === repasseStatusFilter);
   }
   if (repassePeriodCutoff) {
     out = out.filter((r) => {
       // Only API entries follow the YYYY-MM convention. Keep legacy
       // shape-shifted entries (they don't sort cleanly anyway).
       if (!/^\d{4}-\d{2}$/.test(r.month)) return true;
       return r.month >= repassePeriodCutoff;
     });
   }
   if (term) {
     out = out.filter((r) => {
       const hay = `${r.month ?? ''} ${r.approvedBy ?? ''} ${r.processedBy ?? ''}`.toLowerCase();
       return hay.includes(term);
     });
   }
   const sorted = [...out];
   sorted.sort((a, b) => {
     switch (repasseSortBy) {
       case 'month_asc':
         return (a.month ?? '').localeCompare(b.month ?? '');
       case 'value_desc':
         return (b.netRepasse ?? 0) - (a.netRepasse ?? 0);
       case 'value_asc':
         return (a.netRepasse ?? 0) - (b.netRepasse ?? 0);
       case 'month_desc':
       default:
         return (b.month ?? '').localeCompare(a.month ?? '');
     }
   });
   return sorted;
 }, [repasses, repasseStatusFilter, deferredRepasseSearch, repassePeriodCutoff, repasseSortBy]);

 const hasActiveRepasseFilter =
   repasseStatusFilter !== 'Todos' || repassePeriod !== 'all' || deferredRepasseSearch.trim() !== '';

 const clearRepasseFilters = () => {
   setRepasseStatusFilter('Todos');
   setRepassePeriod('all');
   setRepasseSearch('');
 };

 // --- Repasses bulk selection (mirrors charges flow) ---
 const [selectedTransferIds, setSelectedTransferIds] = useState<Set<string>>(new Set());
 // Selectable = only API-backed repasses currently visible AND ready for
 // approval. Processing stays individual because it requires evidence.
 // hidden ones can't be acted on without the operator seeing them first.
 const selectableTransferIds = useMemo(
   () => filteredRepasses.filter((r) => r.action === 'approve').map((r) => r.id),
   [filteredRepasses]
 );

 // Auto-prune when the visible repasse list changes (filter applied, query
 // refetch, etc.). Selecting a row that the operator can no longer see
 // would be confusing, so we drop those quietly.
 useEffect(() => {
   setSelectedTransferIds((prev) => {
     if (prev.size === 0) return prev;
     const visible = new Set(filteredRepasses.map((r) => r.id));
     const next = new Set<string>();
     prev.forEach((id) => { if (visible.has(id)) next.add(id); });
     return next.size === prev.size ? prev : next;
   });
 }, [filteredRepasses]);

 const allVisibleTransfersSelected =
   selectableTransferIds.length > 0 && selectableTransferIds.every((id) => selectedTransferIds.has(id));

 const toggleTransferSelected = (id: string) => {
   setSelectedTransferIds((prev) => {
     const next = new Set(prev);
     if (next.has(id)) next.delete(id);
     else next.add(id);
     return next;
   });
 };

 const toggleSelectAllTransfers = () => {
   setSelectedTransferIds((prev) => {
     if (allVisibleTransfersSelected) {
       const next = new Set(prev);
       selectableTransferIds.forEach((id) => next.delete(id));
       return next;
     }
     const next = new Set(prev);
     selectableTransferIds.forEach((id) => next.add(id));
     return next;
   });
 };

 const clearTransferSelection = () => setSelectedTransferIds(new Set());

 const handleBulkApproveRepasses = () => {
   if (selectedTransferIds.size === 0) return;
   setBulkApprovalOpen(true);
 };
 // Local draft for the recurring payment-method inline editor in the
 // Contas tab. Keyed by account id so each card has its own
 // open/closed state without lifting the whole tab into form state.
 const [editingPaymentForAccount, setEditingPaymentForAccount] = useState<string | null>(null);
 const [paymentMethodDraft, setPaymentMethodDraft] = useState("");

 const utilityStats = useMemo(() => {
   const tenantAccounts = utilityAccounts.filter((u) => u.responsible_party === 'TENANT');
   const ownerAccounts = utilityAccounts.filter((u) => u.responsible_party === 'OWNER');
   const inactive = utilityAccounts.filter((u) => u.status === 'INACTIVE');
   return {
     tenantTotalCents: tenantAccounts.reduce((s, u) => s + (u.estimated_amount_cents || 0), 0),
     tenantCount: tenantAccounts.length,
     ownerTotalCents: ownerAccounts.reduce((s, u) => s + (u.estimated_amount_cents || 0), 0),
     ownerCount: ownerAccounts.length,
     inactiveCount: inactive.length,
   };
 }, [utilityAccounts]);

 const utilityTypeLabel = (type: UtilityAccountType): string => ({
   AGUA: 'Água',
   ENERGIA: 'Energia',
   GAS: 'Gás',
   CONDOMINIO: 'Condomínio',
   IPTU: 'IPTU',
   INTERNET: 'Internet',
   SEGURO: 'Seguro',
   OUTROS: 'Outros',
 })[type] ?? type;

 const utilityTypeIcon = (type: UtilityAccountType): React.ReactNode => {
   switch (type) {
     case 'AGUA': return <Droplet className="w-5 h-5" />;
     case 'ENERGIA': return <Zap className="w-5 h-5" />;
     case 'GAS': return <Flame className="w-5 h-5" />;
     case 'CONDOMINIO': return <Building2 className="w-5 h-5" />;
     case 'IPTU': return <Receipt className="w-5 h-5" />;
     case 'SEGURO': return <ShieldCheck className="w-5 h-5" />;
     default: return <FileText className="w-5 h-5" />;
   }
 };

 const handleRemoveUtilityAccount = (account: UtilityAccount) => {
   confirmAction(`Remover ${utilityTypeLabel(account.type)} (${account.provider})?`, () => deleteUtilityAccountMutation.mutateAsync(account.id), { title: "Remover conta", confirmLabel: "Remover", variant: "danger" });
 };

 const {
   data: rentalChargeConfig,
   isLoading: rentalChargeConfigLoading,
 } = useRentalChargeConfigQuery(selectedContract?.id);
 const createRentalChargeConfigMutation = useCreateRentalChargeConfigMutation();
 const updateRentalChargeConfigMutation = useUpdateRentalChargeConfigMutation(selectedContract?.id);
 const generateRentalChargesMutation = useGenerateRentalChargesMutation(selectedContract?.id);

 // Form for the "Configuração de Cobranças" panel. Hydrated from the existing
 // RentalChargeConfig when the panel opens, otherwise primed with sensible
 // defaults derived from the contract (rent value, due day 10, first charge
 // next month). Financial rates (admin fee, late fee, interest) come from the
 // contract itself since they're persisted on the Contract domain, not the
 // charge config.
 const [chargeConfigForm, setChargeConfigForm] = useState({
   monthlyAmount: "",
   defaultDueDay: String(defaultDueDay),
   firstChargeDate: "",
   firstChargeType: "same_month_pro_rata" as "same_month_pro_rata" | "next_month_accumulated",
   adminFeePct: "",
   lateFeePct: "",
   monthlyInterestPct: "",
 });

 useEffect(() => {
   if (activeModal !== 'cobrancas' || !selectedContract) return;
   const adminFee = selectedContract.commission_pct ?? "";
   const lateFee = selectedContract.late_fee_pct ?? "";
   const interest = selectedContract.monthly_interest_pct ?? "";
   if (rentalChargeConfig) {
     setChargeConfigForm({
       monthlyAmount: String(rentalChargeConfig.monthly_amount / 100),
       defaultDueDay: String(rentalChargeConfig.default_due_day),
       firstChargeDate: rentalChargeConfig.first_charge_date?.split('T')[0] ?? "",
       firstChargeType: rentalChargeConfig.first_charge_type ?? "same_month_pro_rata",
       adminFeePct: adminFee === "" ? "" : String(adminFee),
       lateFeePct: lateFee === "" ? "" : String(lateFee),
       monthlyInterestPct: interest === "" ? "" : String(interest),
     });
   } else {
     const today = new Date();
     const nextMonth = new Date(today.getFullYear(), today.getMonth() + 1, Math.min(defaultDueDay, 28));
     setChargeConfigForm({
       monthlyAmount: String(selectedContract.value ?? 0),
       defaultDueDay: String(defaultDueDay),
       firstChargeDate: nextMonth.toISOString().split('T')[0],
       firstChargeType: "same_month_pro_rata",
       adminFeePct: adminFee === "" ? "" : String(adminFee),
       lateFeePct: lateFee === "" ? "" : String(lateFee),
       monthlyInterestPct: interest === "" ? "" : String(interest),
     });
   }
 }, [activeModal, selectedContract, rentalChargeConfig, defaultDueDay]);

 // The financial rates form is independent of the rental charge config — it
 // edits the underlying Contract. We persist via PUT /contracts/:id reusing
 // the existing contract update mutation infrastructure.
 const updateContractRatesMutation = useMutation({
   mutationFn: ({ id, data }: { id: string; data: { commission_pct?: number; late_fee_pct?: number; monthly_interest_pct?: number } }) =>
     contractService.update(id, data as any),
   onSuccess: (updated) => {
     queryClient.setQueryData<Contract[]>(contractsKeys.list(), (current = []) =>
       current.map((contract) => (contract.id === updated.id ? updated : contract)),
     );
     queryClient.invalidateQueries({ queryKey: contractsKeys.list() });
     toast.success("Taxas atualizadas");
   },
   onError: (error: any) => {
     notifyApiError(error, { context: "Atualizar taxas" });
   },
 });

 const handleSaveContractRates = () => {
   if (!selectedContract) return;
   const admin = parseMoneyBR(chargeConfigForm.adminFeePct);
   const late = parseMoneyBR(chargeConfigForm.lateFeePct);
   const interest = parseMoneyBR(chargeConfigForm.monthlyInterestPct);
   const payload: { commission_pct?: number; late_fee_pct?: number; monthly_interest_pct?: number } = {};
   if (admin !== null && admin > 0) payload.commission_pct = admin;
   if (late !== null && late > 0) payload.late_fee_pct = late;
   if (interest !== null && interest > 0) payload.monthly_interest_pct = interest;
   if (Object.keys(payload).length === 0) {
     toast.error("Informe ao menos uma taxa válida");
     return;
   }
   updateContractRatesMutation.mutate({ id: selectedContract.id, data: payload });
 };

 const handleSaveChargeConfig = async () => {
   if (!selectedContract) return;
   if (rentalChargeConfigLoading) {
     toast.message("Aguarde o carregamento da configuração atual.");
     return;
   }
   const dueDay = parseInt(chargeConfigForm.defaultDueDay, 10);
   if (!dueDay || dueDay < 1 || dueDay > 31) {
     toast.error("Dia de vencimento inválido (1-31)");
     return;
   }
   if (!chargeConfigForm.firstChargeDate) {
     toast.error("Informe a data da primeira cobrança");
     return;
   }

   if (rentalChargeConfig) {
     updateRentalChargeConfigMutation.mutate({
       id: rentalChargeConfig.id,
       data: {
         // The Go handler binds first_charge_date into a time.Time, which only
         // accepts RFC3339 — a bare YYYY-MM-DD fails with "cannot parse as T".
         // The date input gives us a date-only value, so widen it to midnight
         // UTC ISO, matching ContractTab's create flow.
         first_charge_date: new Date(`${chargeConfigForm.firstChargeDate}T00:00:00Z`).toISOString(),
         default_due_day: dueDay,
       },
     });
     return;
   }

   // Create flow: resolve owner_id and tenant_party_id the same way the
   // modals do, so the backend has everything it needs to issue charges.
   // PropertiesContext loads once per session; if an owner was attached
   // mid-session (captação on existing property, PropertyDetails edit), the
   // cached entry may be stale. Mirror PropertyDetails' fallback: when the
   // cached copy has no owner, refetch by id and update the global cache.
   let property = selectedContract.property_id
     ? properties.find((p) => p.id === selectedContract.property_id)
     : undefined;
   const cachedOwnerId = property?.ownerId || property?.owners?.[0]?.id || "";
   if (selectedContract.property_id && !cachedOwnerId) {
     try {
       const res = await propertyService.getById(selectedContract.property_id);
       const dto = unwrapApiData<PropertyDTO>(res);
       property = mapPropertyDtoToProperty(dto);
       fetchProperties().catch(() => { /* non-blocking cache sync */ });
     } catch (err) {
       console.warn("charge config: property refetch failed", err);
     }
   }
   const ownerId = property?.ownerId || property?.owners?.[0]?.id || "";
   const chargeConfigIssues = buildContractIssues(selectedContract, property, navigate);
   if (chargeConfigIssues.length > 0) {
     showContractIssueToasts(chargeConfigIssues);
     return;
   }

   const monthly = parseMoneyBR(chargeConfigForm.monthlyAmount) ?? 0;
   if (monthly <= 0) {
     toast.error("Valor mensal deve ser maior que zero");
     return;
   }

   const start = selectedContract.start_date?.split('T')[0] ?? new Date().toISOString().split('T')[0];
   const end = selectedContract.end_date?.split('T')[0]
     ?? new Date(new Date(start).getTime() + 30 * 30 * 24 * 60 * 60 * 1000).toISOString().split('T')[0];

   createRentalChargeConfigMutation.mutate({
     contract_id: selectedContract.id,
     // buildContractIssues above guarantees customer_id, property_id, and
     // ownerId are all present — the ! satisfies TS without runtime cost.
     tenant_party_id: selectedContract.customer_id!,
     owner_id: ownerId,
     property_id: selectedContract.property_id!,
     // The Go handler binds these into time.Time fields (RFC3339-only). The
     // values above are date-only (YYYY-MM-DD), so widen them to midnight UTC
     // ISO before sending — otherwise binding fails on start_date with
     // "parsing time ... cannot parse \"\" as \"T\"".
     start_date: new Date(`${start}T00:00:00Z`).toISOString(),
     end_date: new Date(`${end}T00:00:00Z`).toISOString(),
     first_charge_date: new Date(`${chargeConfigForm.firstChargeDate}T00:00:00Z`).toISOString(),
     first_charge_type: chargeConfigForm.firstChargeType,
     default_due_day: dueDay,
     monthly_amount: monthly,
   });
 };

 const handleGenerateCharges = () => {
   if (!rentalChargeConfig) {
     toast.error("Salve o plano-base antes de emitir cobranças.");
     return;
   }
   confirmAction("Emitir agora as cobranças previstas pelo plano? O sistema não duplicará competências já emitidas.", () => generateRentalChargesMutation.mutateAsync(rentalChargeConfig.id), { title: "Emitir cobranças", confirmLabel: "Emitir" });
 };

 const handleOpenRentalComposition = () => {
   if (!rentalChargeConfig) {
     setSettingsTab('plano');
     setActiveModal('cobrancas');
     toast.message("Crie primeiro o plano-base da locação.");
     return;
   }
   setActiveModal('none');
   setShowRentalComposition(true);
 };

 const handleLaunchUtilityCharge = (account: UtilityAccount) => {
   if (account.billing_mode !== 'VARIABLE_SEPARATE') {
     toast.error("Esta conta não está configurada como valor variável por mês.");
     return;
   }
   setUtilityChargeAccountId(account.id);
 };

 const chargeConfigLocked = Boolean(rentalChargeConfig?.first_charge_paid);

 // Resolve full lead/owner objects for the selected contract — used by the
 // Visão Geral tab to render real contact details instead of placeholders.
 const tenantLead = useMemo(() => {
   if (!selectedContract?.customer_id) return undefined;
   return leads.find((l) => l.id === selectedContract.customer_id);
 }, [selectedContract?.customer_id, leads]);

 const propertyForContract = useMemo(() => {
   if (!selectedContract?.property_id) return undefined;
   return properties.find((p) => p.id === selectedContract.property_id);
 }, [selectedContract?.property_id, properties]);

 const ownerLead = useMemo(() => {
   const ownerId = propertyForContract?.ownerId || propertyForContract?.owners?.[0]?.id;
   if (!ownerId) return undefined;
   return leads.find((l) => l.id === ownerId);
 }, [propertyForContract, leads]);

 // A baixa financeira é individual: o modal preserva pagamento parcial,
 // método, data RFC3339, observação e comprovante. Registros legados não são
 // alterados por ID nesta visão, pois não provam vínculo com a cobrança.
 const handleMarkAsPaid = (chargeId: string) => {
   const charge = charges.find((item) => item.id === chargeId);
   if (!charge) return;
   if (charge._source !== 'charge' || !charge._canMarkPaid) {
     toast.warning('Este registro precisa ser revisado em Lançamentos antes da baixa.');
     navigate('/financeiro/lancamentos');
     return;
   }
   setMarkingPaidChargeId(chargeId);
 };

 const handleHeaderRegistrarConta = () => {
   if (activeContracts.length === 0) {
     toast.error("Nenhum contrato ativo de locação disponível.");
     return;
   }
   setHeaderAction('account');
 };

 const handleHeaderGerarRepasse = () => {
   if (activeContracts.length === 0) {
     toast.error("Nenhum contrato ativo de locação disponível.");
     return;
   }
   setHeaderAction('transfer');
 };

 const handleHeaderConfigurarPlano = () => {
   if (activeContracts.length === 0) {
     toast.error("Nenhum contrato ativo de locação disponível.");
     return;
   }
   setHeaderAction('plan');
 };

 const handleHeaderContractSelected = (contract: Contract, action: RentalHeaderAction) => {
   setSelectedContract(contract);
   setHeaderAction(null);
   if (action === 'plan') {
     setSettingsTab('plano');
     setActiveModal('cobrancas');
   } else if (action === 'account') {
     setActiveModal('vincularConta');
   } else {
     setView('detail');
     setDetailTab('cobrancas');
     toast.info('O repasse é gerado pela quitação integral da cobrança e sua composição financeira.');
   }
 };

 const handleHeaderNovoContrato = () => {
   navigate('/negocios');
 };

 // Reajuste form state. Initialised when the reajuste panel opens for a
 // contract; persisted across re-renders so users don't lose typing if they
 // pan the side panel.
 const [reajusteForm, setReajusteForm] = useState({ index: 'IGPM', percent: '', baseDate: '' });

 useEffect(() => {
   if (activeModal === 'reajuste' && selectedContract) {
     setReajusteForm({
       index: selectedContract.adjust_index ?? selectedContract.index ?? 'IGPM',
       percent: '',
       baseDate: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString().split('T')[0],
     });
   }
 }, [activeModal, selectedContract]);

 const reajustePreviewValue = useMemo(() => {
   const pct = parseMoneyBR(reajusteForm.percent) ?? 0;
   const base = Number(selectedContract?.value ?? 0);
   return Number.isFinite(base) ? base + (base * pct) / 100 : 0;
 }, [reajusteForm.percent, selectedContract?.value]);

 const handleApplyReajuste = () => {
   if (!selectedContract) return;
   const pct = parseMoneyBR(reajusteForm.percent);
   if (!pct || pct <= 0) {
     toast.error("Informe o percentual do reajuste");
     return;
   }
   if (!reajusteForm.baseDate) {
     toast.error("Selecione a próxima data de reajuste");
     return;
   }
   adjustContractMutation.mutate(
     {
       id: selectedContract.id,
       data: {
         value: reajustePreviewValue,
         adjust_index: reajusteForm.index,
         next_adjustment: reajusteForm.baseDate,
       },
     },
     {
       onSuccess: () => {
         setActiveModal('none');
       },
     }
   );
 };

 const handleApproveRepasse = async (repasseId: string) => {
   const repasse = repasses.find((item) => item.id === repasseId);
   if (!repasse || repasse.action !== 'approve') {
     toast.warning('Este repasse não está pronto para aprovação. Revise a composição financeira.');
     return false;
   }
   try {
     await approveTransferMutation.mutateAsync(repasseId);
     setSelectedTransferIds((current) => {
       const next = new Set(current);
       next.delete(repasseId);
       return next;
     });
     return true;
   } catch {
     return false;
   }
 };

 const handleProcessRepasse = (repasseId: string) => {
   const repasse = repasses.find((item) => item.id === repasseId);
   if (!repasse || repasse.action !== 'process' || !repasse._transfer) {
     toast.warning('Apenas repasses aprovados e com cobertura canônica podem ser processados.');
     return;
   }
   setProcessingTransferId(repasseId);
 };

 // Repasse sem saldo a repassar: encerra sem comprovante e sem lançamento de
 // caixa. Não passa pelo painel de comprovante do processamento normal.
 const handleSettleRepasseWithoutBalance = (repasseId: string) => {
   const repasse = repasses.find((item) => item.id === repasseId);
   if (!repasse || repasse.action !== 'settle' || !repasse._transfer) {
     toast.warning('Este repasse não está mais sem saldo a repassar. Revise a composição financeira.');
     return;
   }
   confirmAction(
     `Não há valor a transferir na competência ${repasse.month}. O repasse será encerrado sem comprovante e nenhuma movimentação de caixa será registrada. Esta ação não pode ser desfeita.`,
     async () => {
       await processTransferMutation.mutateAsync({ transferId: repasseId, evidence: {} });
     },
     { title: 'Concluir repasse sem saldo', confirmLabel: 'Concluir sem saldo', variant: 'warning' },
   );
 };

 // --- Encerrar contrato ---
 const handleTerminateContract = () => {
   if (!selectedContract) return;
   const contractId = selectedContract.id;
   confirmAction(`Encerrar o contrato ${selectedContract.display_id ?? contractId}? Esta ação não pode ser desfeita.`, async () => {
     await terminateContractMutation.mutateAsync({ id: contractId });
     setView('list');
     setSelectedContract(null);
   }, { title: "Encerrar contrato", confirmLabel: "Encerrar", variant: "danger" });
 };

 // --- Download repasse receipt PDF ---
 // Pulls the PDF as a Blob (so auth headers attach via axios) and triggers a
 // browser-side download. Skips API-less legacy CRMContext entries since the
 // receipt endpoint only knows about real /transfers records.
 const handleDownloadRepasseReceipt = async (repasseId: string) => {
   const isApiTransfer = realTransfers.some((t) => t.id === repasseId);
   if (!isApiTransfer) {
     toast.error("Comprovante disponível apenas para repasses registrados no novo motor financeiro.");
     return;
   }
   try {
     const blob = await transferService.downloadReceipt(repasseId);
     const url = URL.createObjectURL(blob);
     const link = document.createElement('a');
     link.href = url;
     link.download = `repasse-${repasseId.slice(0, 8)}.pdf`;
     document.body.appendChild(link);
     link.click();
     link.remove();
     setTimeout(() => URL.revokeObjectURL(url), 1000);
   } catch (error: any) {
     notifyApiError(error, { context: "Baixar comprovante" });
   }
 };

 // --- Send reminder ---
 // Real API call: POST /charges/:id/notify resolves the payer via CRM and
 // dispatches email + in-app via the comm service. Legacy CRMContext-only
 // charges are skipped with a clear error so the operator knows why.
 const REMINDER_DEBOUNCE_HOURS = 24;
 const handleSendChargeReminder = (chargeId: string, options: { skipConfirm?: boolean } = {}) => {
   const charge = charges.find((c) => c.id === chargeId);
   if (!charge) return;
   if (charge._source !== 'charge') {
     toast.error("Lembrete disponível apenas para cobranças registradas no motor financeiro.");
     return;
   }
   // Soft idempotency: warn the operator before re-sending within 24h of
   // the last reminder. Auto-régua flows (future) will skip this check by
   // passing skipConfirm.
   if (!options.skipConfirm && charge.lastReminderAt) {
     const lastTs = new Date(charge.lastReminderAt).getTime();
     const hoursSince = (Date.now() - lastTs) / (1000 * 60 * 60);
     if (hoursSince < REMINDER_DEBOUNCE_HOURS) {
       confirmAction(`Último lembrete enviado há ${Math.floor(hoursSince)}h. Reenviar agora?`, () => notifyChargeMutation.mutateAsync(chargeId), { title: "Reenviar lembrete", confirmLabel: "Reenviar" });
       return;
     }
   }
   if (options.skipConfirm) return notifyChargeMutation.mutateAsync(chargeId);
   notifyChargeMutation.mutate(chargeId);
 };

 // --- Cobranças filter pipeline: status → period → search → sort ---
 // Keeping each transform explicit (rather than chaining .filter().sort())
 // so the active criteria are easy to reason about + debug. We also build a
 // helper to know when any non-default filter is engaged, to drive the
 // "limpar filtros" affordance in the empty state.
 const periodCutoffISO = useMemo(() => {
   if (chargePeriod === 'all') return null;
   const now = new Date();
   if (chargePeriod === 'this_month') {
     return new Date(now.getFullYear(), now.getMonth(), 1).toISOString().split('T')[0];
   }
   const monthsBack = chargePeriod === 'last_3' ? 3 : 6;
   return new Date(now.getFullYear(), now.getMonth() - monthsBack, 1).toISOString().split('T')[0];
 }, [chargePeriod]);

 const filteredCharges = useMemo(() => {
   const term = deferredChargeSearch.trim().toLowerCase();
   let out = charges;
   if (chargeFilter !== 'Todos') {
     out = out.filter((c) => c.filterStatus === chargeFilter);
   }
   if (periodCutoffISO) {
     out = out.filter((c) => !c.dueDate || c.dueDate >= periodCutoffISO);
   }
   if (term) {
     out = out.filter((c) => {
       const hay = `${c.description ?? ''} ${c.type ?? ''} ${c.id ?? ''}`.toLowerCase();
       return hay.includes(term);
     });
   }
   const sorted = [...out];
   sorted.sort((a, b) => {
     switch (chargeSortBy) {
       case 'date_asc':
         return (a.dueDate ?? '').localeCompare(b.dueDate ?? '');
       case 'value_desc':
         return (b.value ?? 0) - (a.value ?? 0);
       case 'value_asc':
         return (a.value ?? 0) - (b.value ?? 0);
       case 'date_desc':
       default:
         return (b.dueDate ?? '').localeCompare(a.dueDate ?? '');
     }
   });
   return sorted;
 }, [charges, chargeFilter, deferredChargeSearch, periodCutoffISO, chargeSortBy]);

 const hasActiveChargeFilter =
   chargeFilter !== 'Todos' || chargePeriod !== 'all' || deferredChargeSearch.trim() !== '';

 const clearChargeFilters = () => {
   setChargeFilter('Todos');
   setChargePeriod('all');
   setChargeSearch('');
 };

 // --- Cobranças bulk selection ---
 // We keep selection scoped to the currently filtered set: when the user
 // changes filters, anything no longer visible is dropped from the
 // selection automatically so they can't "accidentally" act on rows they
 // can't see. The chargeFilter/period/search dependencies in the effect
 // below do the prune.
 const [selectedChargeIds, setSelectedChargeIds] = useState<Set<string>>(new Set());

 useEffect(() => {
   setSelectedChargeIds((prev) => {
     if (prev.size === 0) return prev;
     const visible = new Set(filteredCharges.map((c) => c.id));
     const next = new Set<string>();
     prev.forEach((id) => { if (visible.has(id)) next.add(id); });
     return next.size === prev.size ? prev : next;
   });
 }, [filteredCharges]);

 const visibleSelectableIds = useMemo(
   () => filteredCharges.filter((c) => c._source === 'charge' && c._canMarkPaid).map((c) => c.id),
   [filteredCharges]
 );

 const allVisibleSelected =
   visibleSelectableIds.length > 0 && visibleSelectableIds.every((id) => selectedChargeIds.has(id));

 const toggleChargeSelected = (id: string) => {
   setSelectedChargeIds((prev) => {
     const next = new Set(prev);
     if (next.has(id)) next.delete(id);
     else next.add(id);
     return next;
   });
 };

 const toggleSelectAllVisible = () => {
   setSelectedChargeIds((prev) => {
     if (allVisibleSelected) {
       const next = new Set(prev);
       visibleSelectableIds.forEach((id) => next.delete(id));
       return next;
     }
     const next = new Set(prev);
     visibleSelectableIds.forEach((id) => next.add(id));
     return next;
   });
 };

 const clearChargeSelection = () => setSelectedChargeIds(new Set());

 // Pagamento exige conferência individual de valor, método e evidência. A
 // seleção múltipla continua útil para lembretes, mas nunca dispara baixas em
 // massa com dados presumidos.
 const handleBulkMarkPaid = () => {
   const ids = Array.from(selectedChargeIds);
   if (ids.length === 0) return;
   if (ids.length !== 1) {
     toast.info('A baixa exige conferência individual. Selecione uma cobrança por vez.');
     return;
   }
   handleMarkAsPaid(ids[0]);
 };

 const handleBulkNotify = async () => {
   const ids = Array.from(selectedChargeIds).filter((id) => {
     const c = charges.find((c) => c.id === id);
     return c && c._source === 'charge' && c.status !== 'Pago';
   });
   if (ids.length === 0) {
     toast.error("Nenhuma cobrança elegível selecionada.");
     return;
   }
   const remaining = new Set(ids);
   confirmAction(`Enviar lembrete para ${ids.length} cobrança(s)?`, async () => {
     const results = await Promise.allSettled([...remaining].map(async id => {
       await notifyChargeMutation.mutateAsync(id);
       remaining.delete(id);
     }));
     if (results.some(result => result.status === "rejected")) throw new Error("Alguns lembretes falharam. Tente novamente para reenviar apenas os pendentes.");
     clearChargeSelection();
   }, { title: "Enviar lembretes", confirmLabel: "Enviar" });
 };

 const chargeStats = charges.reduce(
   (stats, charge) => {
     if (charge._includedInTotals === false) return stats;
     stats.paid += charge._paidAmount;
     stats.overdue += charge._lateAmount;
     stats.pending += charge._pendingAmount;
     stats.total += charge._paidAmount + charge._lateAmount + charge._pendingAmount;
     return stats;
   },
   { total: 0, paid: 0, overdue: 0, pending: 0 },
 );
 const markingPaidCharge = markingPaidChargeId
   ? realCharges.find((charge) => charge.id === markingPaidChargeId) ?? null
   : null;
 const receiptCharge = receiptChargeId
   ? realCharges.find((charge) => charge.id === receiptChargeId) ?? null
   : null;
 const processingTransfer = processingTransferId
   ? realTransfers.find((transfer) => transfer.id === processingTransferId) ?? null
   : null;

 // ====== SIDE PANEL (Cobrancas config / Reajuste) ======
 // ====== TAB: VISAO GERAL ======

 // ====== MAIN RETURN ======
 return (
 <div className="flex flex-col h-full bg-muted/50 font-sans relative overflow-hidden">
 {confirmationDialog}
 <RentalContractActionLauncher
 action={headerAction}
 contracts={activeContracts}
 onClose={() => setHeaderAction(null)}
 onSelect={handleHeaderContractSelected}
 />
 <RentalSettingsPanel
 selectedContract={selectedContract}
 activeModal={activeModal}
 setActiveModal={setActiveModal}
 settingsTab={settingsTab}
 setSettingsTab={setSettingsTab}
 formatCurrency={formatCurrency}
 adjustContractMutation={adjustContractMutation}
 utilityAccounts={utilityAccounts}
 utilityAccountsLoading={utilityAccountsLoading}
 updateUtilityAccountMutation={updateUtilityAccountMutation}
 reminderRules={reminderRules}
 reminderRulesLoading={reminderRulesLoading}
 updateReminderRuleMutation={updateReminderRuleMutation}
 deleteReminderRuleMutation={deleteReminderRuleMutation}
 runRemindersNowMutation={runRemindersNowMutation}
 setEditingRule={setEditingRule}
 setReminderRuleModalOpen={setReminderRuleModalOpen}
 editingPaymentForAccount={editingPaymentForAccount}
 setEditingPaymentForAccount={setEditingPaymentForAccount}
 paymentMethodDraft={paymentMethodDraft}
 setPaymentMethodDraft={setPaymentMethodDraft}
 utilityTypeLabel={utilityTypeLabel}
 utilityTypeIcon={utilityTypeIcon}
 rentalChargeConfig={rentalChargeConfig}
 rentalChargeConfigLoading={rentalChargeConfigLoading}
 chargesCount={realCharges.length}
 defaultAdminFeePct={defaultAdminFeePct}
 defaultLateFeePct={defaultLateFeePct}
 defaultMonthlyInterestPct={defaultMonthlyInterestPct}
 onOpenComposition={handleOpenRentalComposition}
 onLaunchUtilityCharge={handleLaunchUtilityCharge}
 createRentalChargeConfigMutation={createRentalChargeConfigMutation}
 updateRentalChargeConfigMutation={updateRentalChargeConfigMutation}
 generateRentalChargesMutation={generateRentalChargesMutation}
 chargeConfigForm={chargeConfigForm}
 setChargeConfigForm={setChargeConfigForm}
 updateContractRatesMutation={updateContractRatesMutation}
 handleSaveContractRates={handleSaveContractRates}
 handleSaveChargeConfig={handleSaveChargeConfig}
 handleGenerateCharges={handleGenerateCharges}
 chargeConfigLocked={chargeConfigLocked}
 reajusteForm={reajusteForm}
 setReajusteForm={setReajusteForm}
 reajustePreviewValue={reajustePreviewValue}
 handleApplyReajuste={handleApplyReajuste}
 />

 {/* Creation Modals */}
 {activeModal === 'novaCobranca' && selectedContract && (
 <NovaCobrancaModal
 contract={selectedContract}
 onClose={() => setActiveModal('none')}
 />
 )}
 {markingPaidCharge && (
 <MarkPaidModal
 chargeId={markingPaidCharge.id}
 defaultAmount={chargeReceivableAmounts(markingPaidCharge).pending + chargeReceivableAmounts(markingPaidCharge).overdue}
 charge={markingPaidCharge}
 onClose={() => setMarkingPaidChargeId(null)}
 onSave={async (chargeId, data) => {
   await markChargePaidMutation.mutateAsync({ chargeId, data });
   setSelectedChargeIds((current) => {
     const next = new Set(current);
     next.delete(chargeId);
     return next;
   });
 }}
 />
 )}
 {receiptCharge && selectedContract && (
 <ChargeReceiptModal
 charge={receiptCharge}
 payerName={selectedContract.client}
 contractDisplayID={selectedContract.display_id ?? selectedContract.id}
 onClose={() => setReceiptChargeId(null)}
 />
 )}
 {processingTransfer && (
 <ProcessRepasseModal
 transferId={processingTransfer.id}
 transfer={processingTransfer}
 onClose={() => setProcessingTransferId(null)}
 onProcess={async (transferId, evidence) => {
   await processTransferMutation.mutateAsync({ transferId, evidence });
 }}
 />
 )}
 <ConfirmDialog
 open={bulkApprovalOpen}
 title="Aprovar repasses selecionados"
 description={`Aprovar ${selectedTransferIds.size} repasse(s) com composição financeira conferida? O processamento bancário continuará individual e exigirá comprovante.`}
 confirmLabel="Aprovar selecionados"
 variant="warning"
 onCancel={() => setBulkApprovalOpen(false)}
 onConfirm={async () => {
   const ids = Array.from(selectedTransferIds).filter((id) =>
     repasses.some((repasse) => repasse.id === id && repasse.action === 'approve'),
   );
   for (const id of ids) {
     await approveTransferMutation.mutateAsync(id);
   }
   clearTransferSelection();
   setBulkApprovalOpen(false);
 }}
 />
 {activeModal === 'vincularConta' && selectedContract && (
 <UtilityAccountModal
 contract={selectedContract}
 rentalConfig={rentalChargeConfig}
 onConfigurePlan={handleOpenRentalComposition}
 onClose={() => setActiveModal('none')}
 />
 )}
 {showRentalComposition && selectedContract && (
 <BillingPlanWizard
 contractId={selectedContract.id}
 contractType="RENT"
 defaultMonthlyAmount={rentalChargeConfig ? rentalChargeConfig.monthly_amount / 100 : (selectedContract.value ?? 0)}
 onClose={() => setShowRentalComposition(false)}
 onSaved={() => {
 queryClient.invalidateQueries({ queryKey: ["contracts", selectedContract.id, "rental-charge-config"] });
 queryClient.invalidateQueries({ queryKey: contractsKeys.charges(selectedContract.id) });
 }}
 />
 )}
 {selectedContract && selectedUtilityChargeAccount && (
 <UtilityChargeModal
 contract={selectedContract}
 account={selectedUtilityChargeAccount}
 accountLabel={utilityTypeLabel(selectedUtilityChargeAccount.type)}
 onClose={() => setUtilityChargeAccountId(null)}
 />
 )}
 {reminderRuleModalOpen && (
 <ReminderRuleModal
 rule={editingRule}
 onClose={() => {
 setReminderRuleModalOpen(false);
 setEditingRule(null);
 }}
 />
 )}
 {auditTransfer && (
 <ExpandableSidePanel
 open={true}
 onClose={() => setAuditTransferId(null)}
 defaultSize="xl"
 expandedSize="full"
 bgClass="bg-muted"
 renderHeader={({ expandButton, closeButton }) => (
 <div className="bg-card px-6 py-4 border-b border-border flex items-center justify-between z-10 shrink-0">
 <div className="flex items-center gap-3">
 <div className="p-2 bg-status-info-soft text-status-info rounded-lg">
 <History className="w-5 h-5" />
 </div>
 <div>
 <h2 className="text-lg font-bold text-foreground">Histórico do Repasse</h2>
 <p className="text-sm text-muted-foreground">Competência {auditTransfer.month || '—'}</p>
 </div>
 </div>
 <div className="flex items-center gap-2">{expandButton}{closeButton}</div>
 </div>
 )}
 >
 <div className="flex-1 overflow-y-auto p-6 space-y-6">
 {/* Resumo financeiro */}
 <div className="bg-card p-5 rounded-xl border border-border shadow-sm grid grid-cols-2 gap-4">
 <div>
 <p className="text-caption text-muted-foreground uppercase tracking-wider mb-1">Bruto</p>
 <p className="text-sm font-bold text-foreground">{formatCurrency(auditTransfer.grossRent)}</p>
 </div>
 <div>
 <p className="text-caption text-muted-foreground uppercase tracking-wider mb-1">Taxa Admin.</p>
 <p className="text-sm font-bold text-status-critical">-{formatCurrency(auditTransfer.adminFee)}</p>
 </div>
 <div>
 <p className="text-caption text-muted-foreground uppercase tracking-wider mb-1">Deduções</p>
 <p className="text-sm font-bold text-status-critical">-{formatCurrency(auditTransfer.utilityDeductions + auditTransfer.maintenanceDeductions)}</p>
 </div>
 <div>
 <p className="text-caption text-muted-foreground uppercase tracking-wider mb-1">Líquido</p>
 <p className="text-lg font-bold text-status-ok">{formatCurrency(auditTransfer.netRepasse)}</p>
 </div>
 <div className="col-span-2">
 <p className="text-caption text-muted-foreground uppercase tracking-wider mb-1">Status</p>
 <span className={clsx(
 "inline-flex items-center gap-1.5 py-0.5 px-2 rounded-full text-caption font-bold border uppercase tracking-wider",
 auditTransfer.status === 'Processado'
   ? "bg-status-ok-soft text-status-ok border-status-ok/30"
   : auditTransfer.status === 'Aprovado'
     ? "bg-status-info-soft text-status-info border-status-info/30"
     : auditTransfer.status === 'Pendente'
       ? "bg-status-warning-soft text-status-warning border-status-warning/30"
       : "bg-muted text-muted-foreground border-border"
 )}>{auditTransfer.status}</span>
 </div>
 </div>

 {/* Ações rápidas */}
 <div className="flex flex-wrap gap-2">
 <Button
 variant="outline"
 size="sm"
 onClick={() => handleDownloadRepasseReceipt(auditTransfer.id)}
 ><Download className="w-3.5 h-3.5" /> Baixar PDF</Button>
 {perms.financeApproveTransfer && auditTransfer.action === 'approve' && (
 <Button
 variant="outline"
 size="sm"
 onClick={async () => {
   if (await handleApproveRepasse(auditTransfer.id)) setAuditTransferId(null);
 }}
 disabled={approveTransferMutation.isPending}
 className="text-status-ok hover:text-status-ok bg-status-ok-soft hover:bg-status-ok/20 border-status-ok/30"
 ><CheckCircle2 className="w-3.5 h-3.5" /> Aprovar</Button>
 )}
 {perms.financeApproveTransfer && auditTransfer.action === 'process' && (
 <Button
 variant="outline"
 size="sm"
 onClick={() => {
   setAuditTransferId(null);
   handleProcessRepasse(auditTransfer.id);
 }}
 disabled={processTransferMutation.isPending}
 className="text-status-ok hover:text-status-ok bg-status-ok-soft hover:bg-status-ok/20 border-status-ok/30"
 ><Banknote className="w-3.5 h-3.5" /> Processar com comprovante</Button>
 )}
 </div>

 {/* Timeline */}
 <div>
 <h3 className="text-sm font-bold text-foreground mb-3 flex items-center gap-2">
 <History className="w-4 h-4 text-primary-600" /> Linha do Tempo
 <span className="text-xs font-medium text-muted-foreground">({auditTransfer.history?.length ?? 0})</span>
 </h3>
 {(!auditTransfer.history || auditTransfer.history.length === 0) ? (
 <div className="bg-card border border-dashed border-border rounded-xl p-6 text-center text-xs text-muted-foreground">
 Sem histórico registrado para este repasse.
 </div>
 ) : (
 <div className="space-y-2">
 {[...auditTransfer.history]
 .sort((a, b) => (b.at ?? '').localeCompare(a.at ?? ''))
 .map((entry, idx) => {
 const at = entry.at ? new Date(entry.at) : null;
 const resolved = resolveSentByLabel(entry.actor ? (entry.actor === 'system' ? 'system' : `user:${entry.actor}`) : null);
 const actor = entry.actor_label
 ? { label: entry.actor_label, sublabel: resolved.sublabel }
 : resolved;
 const actionLabel = entry.action === 'created' ? 'Criado'
   : entry.action === 'approved' ? 'Aprovado'
   : entry.action === 'processed' ? 'Processado'
   : entry.action === 'cancelled' ? 'Cancelado'
   : entry.action;
 const dotColor = entry.action === 'processed' ? 'bg-status-ok'
   : entry.action === 'approved' ? 'bg-status-info'
   : entry.action === 'cancelled' ? 'bg-status-critical'
   : 'bg-muted-foreground';
 return (
 <div key={`${entry.at}-${idx}`} className="bg-card border border-border p-4 rounded-xl shadow-sm">
 <div className="flex items-start gap-3">
 <div className={clsx("w-2 h-2 rounded-full mt-1.5 shrink-0", dotColor)} />
 <div className="min-w-0 flex-1">
 <div className="flex items-center gap-2 mb-0.5">
 <span className="text-sm font-bold text-foreground">{actionLabel}</span>
 <span className="text-[11px] text-muted-foreground">
 {at ? at.toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—'}
 </span>
 </div>
 <p className="text-[11px] text-muted-foreground">Por: <span className="font-medium text-foreground">{actor.label}</span></p>
 {entry.note && <p className="text-[11px] text-muted-foreground italic mt-1">{entry.note}</p>}
 </div>
 </div>
 </div>
 );
 })}
 </div>
 )}
 </div>
 </div>
 </ExpandableSidePanel>
 )}

 {auditCharge && (
 <ExpandableSidePanel
 open={true}
 onClose={() => setAuditChargeId(null)}
 defaultSize="xl"
 expandedSize="full"
 bgClass="bg-muted"
 renderHeader={({ expandButton, closeButton }) => (
 <div className="bg-card px-6 py-4 border-b border-border flex items-center justify-between z-10 shrink-0">
 <div className="flex items-center gap-3">
 <div className="p-2 bg-status-info-soft text-status-info rounded-lg">
 <History className="w-5 h-5" />
 </div>
 <div>
 <h2 className="text-lg font-bold text-foreground">Histórico da Cobrança</h2>
 <p className="text-sm text-muted-foreground">{auditCharge.description}</p>
 </div>
 </div>
 <div className="flex items-center gap-2">{expandButton}{closeButton}</div>
 </div>
 )}
 >
 <div className="flex-1 overflow-y-auto p-6 space-y-6">
 {/* Resumo */}
 <div className="bg-card p-5 rounded-xl border border-border shadow-sm grid grid-cols-2 gap-4">
 <div>
 <p className="text-caption text-muted-foreground uppercase tracking-wider mb-1">Valor</p>
 <p className="text-lg font-bold text-foreground">{formatCurrency(auditCharge.value || 0)}</p>
 </div>
 <div>
 <p className="text-caption text-muted-foreground uppercase tracking-wider mb-1">Vencimento</p>
 <p className="text-sm font-bold text-foreground">{formatISODateBR(auditCharge.dueDate) || auditCharge.dueDate}</p>
 </div>
 <div>
 <p className="text-caption text-muted-foreground uppercase tracking-wider mb-1">Status</p>
 <span className={clsx(
 "inline-flex items-center gap-1.5 py-0.5 px-2 rounded-full text-caption font-bold border uppercase tracking-wider",
 auditCharge.status === 'Pago' ? "bg-status-ok-soft text-status-ok border-status-ok/30" :
 auditCharge.status === 'A Vencer' ? "bg-status-info-soft text-status-info border-status-info/30" :
 auditCharge.status === 'Em Atraso' ? "bg-status-critical-soft text-status-critical border-status-critical/30" :
 auditCharge.status === 'Parcial' ? "bg-status-warning-soft text-status-warning border-status-warning/30" :
 "bg-muted text-muted-foreground border-border"
 )}>{auditCharge.status}</span>
 </div>
 <div>
 <p className="text-caption text-muted-foreground uppercase tracking-wider mb-1">Referência</p>
 <p className="text-sm font-bold text-foreground">{auditCharge.referenceMonth || '—'}</p>
 </div>
 </div>

 {/* Ações disponíveis no painel */}
 <div className="flex flex-wrap gap-2">
 {auditCharge._source === 'charge' && (
 <>
 {perms.chargesWrite && auditCharge._canNotify && (
 <Button
 variant="outline"
 size="sm"
 onClick={() => handleSendChargeReminder(auditCharge.id)}
 loading={notifyChargeMutation.isPending}
 loadingText="Enviando…"
 ><Send className="w-3.5 h-3.5" /> Enviar Lembrete Agora</Button>
 )}
 {perms.financeWrite && auditCharge._canMarkPaid && (
 <Button
 variant="outline"
 size="sm"
 onClick={() => handleMarkAsPaid(auditCharge.id)}
 className="text-status-ok hover:text-status-ok bg-status-ok-soft hover:bg-status-ok/20 border-status-ok/30"
 ><CheckCircle2 className="w-3.5 h-3.5" /> Dar Baixa</Button>
 )}
 </>
 )}
 </div>

 {/* Timeline de lembretes */}
 <div>
 <h3 className="text-sm font-bold text-foreground mb-3 flex items-center gap-2">
 <Bell className="w-4 h-4 text-primary-600" /> Lembretes Enviados
 <span className="text-xs font-medium text-muted-foreground">({auditCharge.remindersSent.length})</span>
 </h3>
 {auditCharge.remindersSent.length === 0 ? (
 <div className="bg-card border border-dashed border-border rounded-xl p-6 text-center text-xs text-muted-foreground">
 Nenhum lembrete enviado para esta cobrança ainda.
 </div>
 ) : (
 <div className="space-y-2">
 {[...auditCharge.remindersSent]
 .sort((a, b) => (b.sent_at ?? '').localeCompare(a.sent_at ?? ''))
 .map((entry, idx) => {
 // Prefer the snapshot label stamped at dispatch time over a live
 // lookup. Falls back to the resolver when the entry pre-dates the
 // label rollout (or when an integration writes a typed token only).
 const resolved = resolveSentByLabel(entry.sent_by);
 const actor = entry.sent_by_label
 ? { label: entry.sent_by_label, sublabel: resolved.sublabel }
 : resolved;
 const sentAt = entry.sent_at ? new Date(entry.sent_at) : null;
 return (
 <div key={`${entry.sent_at}-${idx}`} className="bg-card border border-border p-4 rounded-xl shadow-sm">
 <div className="flex items-start justify-between gap-3">
 <div className="min-w-0 flex-1">
 <div className="flex items-center gap-2 mb-1 flex-wrap">
 <span className="text-xs font-bold text-foreground">
 {sentAt ? sentAt.toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—'}
 </span>
 {entry.channels.map((ch) => (
 <span key={ch} className="text-caption font-semibold text-primary-700 bg-primary-50 px-2 py-0.5 rounded-full border border-primary-200 uppercase tracking-wider">{ch}</span>
 ))}
 </div>
 {entry.recipient && (
 <p className="text-[11px] text-muted-foreground truncate">Para: <span className="font-medium text-foreground">{entry.recipient}</span></p>
 )}
 <p className="text-[11px] text-muted-foreground mt-1">
 Disparado por: <span className="font-medium text-foreground">{actor.label}</span>
 {actor.sublabel && <span className="text-muted-foreground italic ml-1">— {actor.sublabel}</span>}
 </p>
 </div>
 <div className="text-muted-foreground shrink-0 mt-1">
 <Bell className="w-4 h-4" />
 </div>
 </div>
 </div>
 );
 })}
 </div>
 )}
 </div>
 </div>
 </ExpandableSidePanel>
 )}

 {view === 'detail' && selectedContract ? (
 <RentalDetailView
 selectedContract={selectedContract}
 perms={perms}
 detailTab={detailTab}
 setDetailTab={setDetailTab}
 setView={setView}
 setActiveModal={setActiveModal}
 setAuditContractId={setAuditContractId}
 setAuditLabel={setAuditLabel}
 openModal={openModal}
 handleTerminateContract={handleTerminateContract}
 terminateContractMutation={terminateContractMutation}
 formatCurrency={formatCurrency}
 tenantLead={tenantLead}
 propertyForContract={propertyForContract}
 ownerLead={ownerLead}
 chargeStats={chargeStats}
 charges={charges}
 financialHistory={financialHistory}
 utilityAccounts={utilityAccounts}
 utilityStats={utilityStats}
 utilityTypeLabel={utilityTypeLabel}
 utilityTypeIcon={utilityTypeIcon}
 handleRemoveUtilityAccount={handleRemoveUtilityAccount}
 onLaunchUtilityCharge={handleLaunchUtilityCharge}
 deleteUtilityAccountMutation={deleteUtilityAccountMutation}
 adminFeePct={selectedAdminFeePct}
 filteredCharges={filteredCharges}
 hasActiveChargeFilter={hasActiveChargeFilter}
 clearChargeFilters={clearChargeFilters}
 selectedChargeIds={selectedChargeIds}
 visibleSelectableIds={visibleSelectableIds}
 allVisibleSelected={allVisibleSelected}
 toggleChargeSelected={toggleChargeSelected}
 toggleSelectAllVisible={toggleSelectAllVisible}
 clearChargeSelection={clearChargeSelection}
 handleBulkMarkPaid={handleBulkMarkPaid}
 handleBulkNotify={handleBulkNotify}
 chargeFilter={chargeFilter}
 setChargeFilter={setChargeFilter}
 chargeSearch={chargeSearch}
 setChargeSearch={setChargeSearch}
 chargeSortBy={chargeSortBy}
 setChargeSortBy={setChargeSortBy}
 chargePeriod={chargePeriod}
 setChargePeriod={setChargePeriod}
 setAuditChargeId={setAuditChargeId}
 handleMarkAsPaid={handleMarkAsPaid}
 handleViewChargeReceipt={setReceiptChargeId}
 handleSendChargeReminder={handleSendChargeReminder}
 realTransfers={realTransfers}
 repasses={repasses}
 filteredRepasses={filteredRepasses}
 handleDownloadRepasseReceipt={handleDownloadRepasseReceipt}
 approveTransferMutation={approveTransferMutation}
 processTransferMutation={processTransferMutation}
 setAuditTransferId={setAuditTransferId}
 resolveActorName={resolveActorName}
 repasseStatusFilter={repasseStatusFilter}
 setRepasseStatusFilter={setRepasseStatusFilter}
 repasseSearch={repasseSearch}
 setRepasseSearch={setRepasseSearch}
 repasseSortBy={repasseSortBy}
 setRepasseSortBy={setRepasseSortBy}
 repassePeriod={repassePeriod}
 setRepassePeriod={setRepassePeriod}
 hasActiveRepasseFilter={hasActiveRepasseFilter}
 clearRepasseFilters={clearRepasseFilters}
 selectedTransferIds={selectedTransferIds}
 selectableTransferIds={selectableTransferIds}
 allVisibleTransfersSelected={allVisibleTransfersSelected}
 toggleTransferSelected={toggleTransferSelected}
 toggleSelectAllTransfers={toggleSelectAllTransfers}
 clearTransferSelection={clearTransferSelection}
 handleBulkApproveRepasses={handleBulkApproveRepasses}
 handleApproveRepasse={handleApproveRepasse}
 handleProcessRepasse={handleProcessRepasse}
 handleSettleRepasseWithoutBalance={handleSettleRepasseWithoutBalance}
 />
 ) : (
 <RentalListView
 perms={perms}
 handleHeaderConfigurarPlano={handleHeaderConfigurarPlano}
 handleHeaderRegistrarConta={handleHeaderRegistrarConta}
 handleHeaderGerarRepasse={handleHeaderGerarRepasse}
 handleHeaderNovoContrato={handleHeaderNovoContrato}
 focusMode={focusMode}
 activeContracts={activeContracts}
 managedRentTotal={managedRentTotal}
 agencyAdminRevenue={agencyAdminRevenue}
 upcomingAdjustments={upcomingAdjustments}
 formatCurrency={formatCurrency}
 searchInput={searchInput}
 setSearchInput={setSearchInput}
 contractTypeFilter={contractTypeFilter}
 setContractTypeFilter={setContractTypeFilter}
 statusFilter={statusFilter}
 setStatusFilter={setStatusFilter}
 sortBy={sortBy}
 setSortBy={setSortBy}
 filteredContracts={filteredContracts}
 contractsLoading={contractsLoading}
 isMobile={isMobile}
 openModal={openModal}
 viewDetail={viewDetail}
 />
 )}

 {/* Audit Trail Panel */}
 {auditContractId && (
 <SharedAuditTrailPanel
 entityId={auditContractId}
 entityLabel={auditLabel}
 onClose={() => { setAuditContractId(null); setAuditLabel(''); }}
 getAuditLog={getAuditLog}
 />
 )}
 </div>
 );
}
