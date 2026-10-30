import { useState, useEffect, useRef, useCallback, useMemo, type SetStateAction } from "react";
import { Navigate, useNavigate, useParams, useSearchParams } from "react-router";
import {
  DRAFT_TTL_MS,
  LANCAMENTO_DRAFT_VERSION,
  LancamentoDraftEnvelopeSchema,
  STORAGE_KEY as LANCAMENTO_STORAGE_KEY,
} from "./Finance/schema";
import {
  useFinanceFilters,
  useLancamentoDraft,
  type FinanceModule,
} from "./Finance/state";
import {
  TrendingDown,
  Clock,
  AlertCircle,
  Search,
  Eye,
  X,
  Calendar,
  FileText,
  Plus,
  Info,
  DollarSign,
  Check,
  Users,
  Inbox,
} from "lucide-react";
import { motion, AnimatePresence } from "motion/react";
import { ChargeReceiptModal } from "../components/ChargeReceiptModal";
import type { Transaction } from "../types/transaction";
import { useFinanceContext } from "../context/FinanceContext";
import { useContractsContext } from "../context/ContractsContext";
import { useEsteiraDeals } from "../hooks/useEsteiraDeals";
import { usePropertiesContext } from "../context/PropertiesContext";
import { useLeadsContext } from "../context/LeadsContext";
import { formatPropertyAddress } from "../data";
import { clsx } from "clsx";
import { toast } from "sonner";
import { TransactionEditModal } from "../components/TransactionEditModal";
import { NovaCobrancaModal } from "../components/NovaCobrancaModal";
import { MarkPaidModal } from "../components/MarkPaidModal";
import { ChargeDetailDrawer } from "../components/ChargeDetailDrawer";
import { CancelarCobrancaDialog } from "../components/CancelarCobrancaDialog";
import { ComissoesTab } from "./Finance/ComissoesTab";
import { ExportReportModal, ExportConfig } from "../components/ExportReportModal";
import { AuditTrailPanel as SharedAuditTrailPanel } from "../components/AuditTrailPanel";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { ProcessRepasseModal } from "../components/ProcessRepasseModal";
import { useFinancialModals } from "../hooks/useFinancialModals";
import { TransactionDetailSlideover } from "./Finance/TransactionDetailSlideover";
import { invoiceService } from "../services/invoice.service";
import { brokerService } from "../services/broker.service";
import { tenantFinancialSettingsService } from "../services/tenantFinancialSettings.service";
import { commissionRulesService } from "../services/commissionRules.service";
import type { CreateTransactionBatchItem, ManualCommissionBatchItem } from "../services/transaction.service";
import { financialPreviewService, type FinancialPreview } from "../services/financialPreview.service";
import { FINANCE_AREA_BY_PATH, FINANCE_AREA_TABS, isFinanceModule } from "./Finance/navigation";
import { COMMISSION_BROKER_ROLE_OPTIONS, discountSummaryFromSalePlan, paymentPlanLabelFromPreview } from "./Finance/formatters";
import { saleBillingPlanService } from "../services/saleBillingPlan.service";
import { rentalChargeConfigService } from "../services/rentalChargeConfig.service";
import { Charge } from "../services/charge.service";
import { EmptyState } from "../ui/EmptyState";
import { Button } from "../ui/button";
import { useFocusMode } from "../context/FocusModeContext";
import { MoneyInput } from "../ui/money-input";
import { PercentInput } from "../ui/percent-input";
import { useAuditLogContext } from "../context/AuditLogContext";
import { useAuth } from "../context/AuthContext";
import { formatCurrency, mapInvoiceToTransaction, StatusBadge } from "./Finance/presentation";
import {
  allocateManualCommissionCents,
  calculateManualCommissionDistribution,
  clearManualPostingAttempt,
  findManualPostingAttempt,
  isPayableBrokerCommissionRole,
  localPostingDate,
  manualPostingIntentFingerprint,
  payableBrokerCommissionEntries,
  persistManualPostingAttempt,
  type ManualPostingAttempt,
} from "./Finance/manualPosting";
import { CobrancasSection } from "./Finance/CobrancasSection";
import { RepassesSection } from "./Finance/RepassesSection";
import { ContasSection } from "./Finance/ContasSection";
import { ConciliacaoSection } from "./Finance/ConciliacaoSection";
import { SettleLancamentoModal, type SettleLancamentoTarget } from "../components/lancamentos/SettleLancamentoModal";
import { resolveReceivableAction } from "../lib/finance/receivableAction";
import FinanceDashboard from "./FinanceDashboard";
import { chargeReceivableAmounts } from "../lib/finance/economicRecords";

export function Finance() {
  const { user } = useAuth();
  const activeTenantID = user?.current_company_id ?? "";
  const { contracts } = useContractsContext();
  const { deals } = useEsteiraDeals();
  const { properties } = usePropertiesContext();
  const { leads } = useLeadsContext();
  const {
    addTransactionBatch,
    transactions: contextTransactions,
    refetchTransactions,
    charges,
    addCharge,
    markChargePaid,
    updateCharge,
    cancelCharge,
    notifyCharge,
    transfers,
    approveTransfer,
    processTransfer,
  } = useFinanceContext();
  const { focusMode } = useFocusMode();
  // Estado centralizado dos modais do Financeiro (hook compartilhado).
  const {
    state: modalState,
    confirm: confirmDialog,
    openConfirm,
    closeConfirm,
    openExport: openExportModal,
    openAudit: openAuditModal,
    openProcessRepasse,
    openReceipt: openReceiptModal,
    close: closeAnyModal,
  } = useFinancialModals();
  const navigate = useNavigate();
  const [financeSearchParams] = useSearchParams();
  const { getAuditLog, appendAuditLogEntry } = useAuditLogContext();
  // Phase 6 — real-data sources que substituem o hardcoded em participantes:
  //  - brokers: dropdown de nomes que existem mesmo no CRM
  //  - commissionRulesByRole: % padrão por role (captador/visitador/fechamento)
  //  - tenantDefaults: % padrão de comissão SALE/RENT (substitui 6/100 chumbado)
  const [brokers, setBrokers] = useState<Array<{ id: string; name: string }>>([]);
  const [commissionRulesByRole, setCommissionRulesByRole] = useState<Record<string, number>>({});
  const [tenantDefaults, setTenantDefaults] = useState<{ rent: number; sale: number; adminFeePct: number }>({ rent: 100, sale: 6, adminFeePct: 0 });

  useEffect(() => {
    (async () => {
      try {
        const [brokerList, rules, settings] = await Promise.all([
          brokerService.getAll().then((res: any) => (res.data?.data ?? [])).catch(() => []),
          commissionRulesService.get().catch(() => []),
          tenantFinancialSettingsService.get().catch(() => null),
        ]);
        const normalizedBrokers: Array<{ id: string; name: string }> = Array.isArray(brokerList)
          ? brokerList.map((b: any) => ({ id: b.id || b.broker_id || '', name: b.name || b.display_name || 'Sem nome' }))
          : [];
        setBrokers(normalizedBrokers);
        const map: Record<string, number> = {};
        (rules || []).forEach((r) => {
          map[r.role] = r.default_percentage;
        });
        setCommissionRulesByRole(map);
        if (settings) {
          setTenantDefaults({
            rent: settings.default_commission_pct_rent,
            sale: settings.default_commission_pct_sale,
            adminFeePct: settings.admin_fee_pct ?? 0,
          });
        }
      } catch (err) {
        // Silently degrade — UI ainda funciona com defaults locais e
        // sem dropdown popular de brokers.
        console.warn('Finance: failed to load empresa defaults', err);
      }
    })();
  }, []);
  const [isExpanded, setIsExpanded] = useState(false);
  const toggleExpand = useCallback(() => setIsExpanded(prev => !prev), []);

  const [apiTransactions, setApiTransactions] = useState<Transaction[]>([]);
  const [loadingInvoices, setLoadingInvoices] = useState(true);

  useEffect(() => {
    loadInvoices();
  }, []);

  const loadInvoices = async () => {
    try {
      setLoadingInvoices(true);
      const invoices = await invoiceService.list();
      const mapped = invoices.map(mapInvoiceToTransaction);
      setApiTransactions(mapped);
    } catch (error) {
      console.error("Failed to load invoices", error);
      toast.error("Erro ao carregar faturas");
    } finally {
      setLoadingInvoices(false);
    }
  };

  // Deduplicate: contextTransactions (from /transactions) take precedence over apiTransactions (from /invoices)
  const mergedMap = new Map<string, Transaction>();
  contextTransactions.forEach(t => mergedMap.set(t.id, t));
  apiTransactions.forEach(t => { if (!mergedMap.has(t.id)) mergedMap.set(t.id, t); });
  const transactions = Array.from(mergedMap.values());


  // Filtros consolidados — search/status/dropdown/date range agrupados
  // em um reducer único (state.ts). Reduz o churn de re-renders e elimina
  // bugs de "filtro X não limpa quando filtro Y muda".
  const [filters, filtersDispatch] = useFinanceFilters();
  const financeParams = useParams();
  const requestedFinanceModule = financeSearchParams.get("module");
  // Módulo ativo: rota tem prioridade; senão `?module=`; senão overview.
  const routeModule = FINANCE_AREA_BY_PATH[financeParams?.module ?? ""];
  const activeModule: FinanceModule =
    routeModule ?? (isFinanceModule(requestedFinanceModule) ? requestedFinanceModule : "overview");

  useEffect(() => {
    if (isFinanceModule(requestedFinanceModule) && requestedFinanceModule !== filters.module) {
      filtersDispatch({ type: "SET_MODULE", module: requestedFinanceModule });
    }
  }, [filters.module, filtersDispatch, requestedFinanceModule]);

  const activeTab = filters.cobrancaSubTab;
  const cobrancaSearch = filters.cobranca.search;
  const cobrancaStatusFilter = filters.cobranca.status;
  const cobrancaStatusOpen = filters.cobranca.statusOpen;
  const repasseSearch = filters.repasse.search;
  const repasseStatusFilter = filters.repasse.status;
  const repasseStatusOpen = filters.repasse.statusOpen;
  const conciliacaoSearch = filters.conciliacao.search;
  const conciliacaoStatusFilter = filters.conciliacao.status;
  const conciliacaoStatusOpen = filters.conciliacao.statusOpen;
  const contasSearch = filters.contas.search;
  const lancDateFrom = filters.lancDate.from;
  const lancDateTo = filters.lancDate.to;

  // Selection / modal state
  const [selectedTransaction, setSelectedTransaction] = useState<any>(null);
  // Baixa padronizada de lançamento manual (método + data + comprovante).
  const [settleLancamento, setSettleLancamento] = useState<SettleLancamentoTarget | null>(null);
  // Charge receipt modal — opens the cupom-fiscal-style receipt for a
  // generated charge. The id lookup against the live `charges` array
  // keeps the receipt in sync with any status change (e.g. PAID).
  // Charge receipt modal — estado centralizado no useFinancialModals.
  const receiptChargeId = modalState.modal === "receipt" ? modalState.chargeId : null;
  const setReceiptChargeId = useCallback((next: string | null) => {
    if (next) openReceiptModal(next);
    else closeAnyModal();
  }, [openReceiptModal, closeAnyModal]);

  // Edit Modal — tx + mode são sempre setados/limpos juntos.
  const [editing, setEditing] = useState<{ tx: Transaction; mode: 'cobranca' | 'repasse' } | null>(null);

  // Modal de nova cobrança. O repasse não possui criação avulsa: ele nasce da
  // liquidação canônica de cobrança/recebimento.
  const [showNovaCobranca, setShowNovaCobranca] = useState(false);

  // Mark Charge Paid Modal — id + form sempre abertos juntos.
  const [markingPaid, setMarkingPaid] = useState<{ chargeId: string; defaultAmount: number; charge?: Charge } | null>(null);

  // Novos: drawer/edit/cancel/QR dirigidos por id contra a lista live de
  // charges. Manter id (não objeto) evita estado obsoleto após optimistic
  // update — o lookup contra `charges` reflete sempre o estado mais novo.
  const [selectedChargeId, setSelectedChargeId] = useState<string | null>(null);
  const [editingChargeId, setEditingChargeId] = useState<string | null>(null);
  const [cancellingChargeId, setCancellingChargeId] = useState<string | null>(null);

  // Export Modal — estado centralizado no useFinancialModals (mantém nome/assinatura).
  const exportConfig = modalState.modal === "export" ? modalState.config : null;
  const setExportConfig = useCallback((next: SetStateAction<ExportConfig | null>) => {
    const config = typeof next === "function" ? null : next;
    if (config) openExportModal(config);
    else closeAnyModal();
  }, [openExportModal, closeAnyModal]);

  // Trilha de auditoria — entityId + label sempre setados/limpos juntos.
  const audit = modalState.modal === "audit" ? { entityId: modalState.entityId, label: modalState.label } : null;
  const setAudit = useCallback((next: SetStateAction<{ entityId: string; label: string } | null>) => {
    const value = typeof next === "function" ? null : next;
    if (value) openAuditModal(value.entityId, value.label);
    else closeAnyModal();
  }, [openAuditModal, closeAnyModal]);

  // Draft do Lançamento — formulário da aba Lançamentos & Comissões.
  // Reducer único valida shape via LancamentoSchema (Zod) no restore.
  const [draft, draftDispatch] = useLancamentoDraft(tenantDefaults.sale);
  // Seq incremental para descartar respostas de auto-fill obsoletas quando o
  // operador troca de contrato/negócio rapidamente.
  const contractFetchSeq = useRef(0);
  const lancamentoTipo = draft.lancamentoTipo;
  const lancamentoOrigem = draft.lancamentoOrigem;
  const selectedContractId = draft.selectedContractId;
  const selectedBeneficiaryId = draft.selectedBeneficiaryId || '';
  const selectedBeneficiaryName = draft.selectedBeneficiaryName || '';
  const baseValue = draft.baseValue;
  const commissionPercent = draft.commissionPercent;
  const distributeCommission = draft.distributeCommission;
  const participants = draft.participants;
  const paymentPlanLabel = draft.paymentPlanLabel ?? '';
  const firstDueDate = draft.firstDueDate ?? '';
  const discountSummary = draft.discountSummary ?? '';
  // A imobiliária é o residual econômico automático da distribuição. Mesmo
  // que um preview/draft legado traga uma linha explícita, ela nunca deve ser
  // exibida nem enviada como contraparte de corretor.
  const commissionBrokerParticipants = payableBrokerCommissionEntries(participants);
  const commissionDistribution = calculateManualCommissionDistribution(
    commissionBrokerParticipants.map((participant) => participant.percent),
  );
  const payableCommissionParticipants = commissionBrokerParticipants.filter(
    (participant) => participant.percent > 0,
  );
  const loadingCommission = draft.loadingCommission || false;
  const commissionDataLoaded = draft.commissionDataLoaded || false;
  const commissionDataError = draft.commissionDataError;

  // Validation & Preview
  const [formTouched, setFormTouched] = useState<Record<string, boolean>>({});
  const [showPreview, setShowPreview] = useState(false);
  const [shakeKey, setShakeKey] = useState(0);
  const isRestoringRef = useRef(false);
  const manualPostingInFlightRef = useRef(false);
  const manualPostingAttemptRef = useRef<ManualPostingAttempt | null>(null);
  const [manualPostingSaving, setManualPostingSaving] = useState(false);

  useEffect(() => {
    manualPostingAttemptRef.current = null;
    manualPostingInFlightRef.current = false;
    setManualPostingSaving(false);
  }, [activeTenantID]);

  // Despachante central de ações sobre uma cobrança. Compartilhado entre row
  // actions, drawer e atalhos de tecla — fonte única evita divergência (e.g.
  // um lugar mostrar "Cancelar" para PAID e outro não). availableChargeActions
  // já filtra por status; este handler aqui é só roteamento.
  const handleChargeAction = useCallback(async (action: 'pay' | 'edit' | 'cancel' | 'notify' | 'receipt', charge: Charge) => {
    switch (action) {
      case 'pay':
        // Estado renomeado num refactor anterior (markingPaidChargeId/markPaidForm
        // → markingPaid { chargeId, defaultAmount }) sem atualizar este handler.
        // O MarkPaidModal gerencia o próprio form internamente; só precisamos
        // passar o chargeId e o defaultAmount inicial.
        {
          const amounts = chargeReceivableAmounts(charge);
          setMarkingPaid({ chargeId: charge.id, defaultAmount: amounts.pending + amounts.overdue, charge });
        }
        break;
      case 'edit':
        setEditingChargeId(charge.id);
        break;
      case 'cancel':
        setCancellingChargeId(charge.id);
        break;
      case 'notify':
        try {
          await notifyCharge(charge.id);
        } catch {
          // toast já tratado em CRMContext
        }
        break;
      case 'receipt':
        setReceiptChargeId(charge.id);
        break;
    }
  }, [notifyCharge]);

  // localStorage persistence for lançamento form — envelope versionado
  // valida o payload com Zod no restore. Drafts incompatíveis (mudou
  // shape de campo, ex.: commission_percent → commissionPct) são
  // descartados em vez de aplicados quebrados.
  const saveDraft = useCallback(() => {
    if (isRestoringRef.current) return;
    const envelope = {
      _version: LANCAMENTO_DRAFT_VERSION,
      _savedAt: Date.now(),
      payload: draft,
    };
    try { localStorage.setItem(LANCAMENTO_STORAGE_KEY, JSON.stringify(envelope)); } catch { }
  }, [draft]);

  // Save draft on changes (debounced)
  useEffect(() => {
    const hasData = selectedContractId || baseValue > 0 || participants.length > 0 || selectedBeneficiaryId;
    if (hasData) {
      const t = setTimeout(saveDraft, 500);
      return () => clearTimeout(t);
    }
  }, [saveDraft, selectedContractId, baseValue, participants, selectedBeneficiaryId]);

  // Restore draft on mount
  useEffect(() => {
    const raw = localStorage.getItem(LANCAMENTO_STORAGE_KEY);
    if (!raw) return;
    let parsed: unknown;
    try { parsed = JSON.parse(raw); } catch {
      localStorage.removeItem(LANCAMENTO_STORAGE_KEY);
      return;
    }
    const envelope = LancamentoDraftEnvelopeSchema.safeParse(parsed);
    if (!envelope.success) {
      // Faltou _version ou shape do payload mudou. Descarta sem aplicar
      // valores quebrados, mas avisa o operador.
      localStorage.removeItem(LANCAMENTO_STORAGE_KEY);
      toast.info('Rascunho antigo foi descartado');
      return;
    }
    const { _savedAt, payload } = envelope.data;
    if (Date.now() - _savedAt > DRAFT_TTL_MS) {
      localStorage.removeItem(LANCAMENTO_STORAGE_KEY);
      return;
    }
    const hasData =
      payload.selectedContractId ||
      payload.baseValue > 0 ||
      payload.participants.length > 0 ||
      payload.selectedBeneficiaryId;
    if (!hasData) return;
    isRestoringRef.current = true;
    draftDispatch({ type: 'RESTORE', draft: payload });
    toast.info('Rascunho restaurado', {
      description: 'Um lançamento em andamento foi recuperado automaticamente.',
      action: {
        label: 'Descartar',
        onClick: () => {
          localStorage.removeItem(LANCAMENTO_STORAGE_KEY);
          draftDispatch({ type: 'CLEAR', commissionPercent: tenantDefaults.sale });
          setFormTouched({});
        },
      },
    });
    setTimeout(() => { isRestoringRef.current = false; }, 100);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const clearDraft = () => {
    try { localStorage.removeItem(LANCAMENTO_STORAGE_KEY); } catch { }
  };

  const markTouched = (field: string) => setFormTouched(prev => ({ ...prev, [field]: true }));

  const getFormErrors = () => {
    const errors: Record<string, string> = {};
    if (!selectedContractId) errors.contractId = 'Selecione um contrato';
    if (!baseValue || baseValue <= 0) errors.baseValue = 'Informe um valor maior que zero';

    // Validações específicas para comissão avulsa (lancamentoTipo === 'comissao')
    if (lancamentoTipo === 'comissao') {
      // Contrato de origem deve ser compatível com a origem selecionada (venda/locação)
      if (selectedContractId) {
        const contract = contracts.find(c => c.id === selectedContractId);
        const deal = deals.find(d => d.id === selectedContractId);
        const effectiveType = contract?.type || deal?.type?.toLowerCase();
        if (effectiveType && effectiveType !== lancamentoOrigem) {
          errors.contractId = `Este contrato é de ${effectiveType === 'venda' ? 'venda' : 'locação'}, mas a origem selecionada é ${lancamentoOrigem === 'venda' ? 'venda' : 'locação'}. Altere a origem ou selecione outro contrato.`;
        }
      }
      if (!selectedBeneficiaryId) errors.beneficiary = 'Selecione o corretor/parceiro beneficiário';
      if (commissionPercent <= 0 && selectedBeneficiaryId) errors.commissionPercent = 'Informe um percentual de comissão maior que zero';
      if (commissionDataError) errors.commissionData = 'Dados de comissionamento não foram carregados corretamente';
    }

    if (lancamentoTipo === 'recebimento' && lancamentoOrigem === 'venda') {
      if (!Number.isFinite(commissionPercent) || commissionPercent < 0 || commissionPercent > 100) {
        errors.commissionPercent = 'Informe uma comissão total entre 0% e 100%';
      }
    }

    if (lancamentoTipo === 'recebimento' && distributeCommission) {
      if (commissionDistribution.invalid) {
        errors.distribution = 'Cada percentual da distribuição deve estar entre 0% e 100%';
      } else if (commissionDistribution.overallocated) {
        errors.distribution = 'A soma destinada aos corretores não pode passar de 100%';
      }
      if (commissionDataError) errors.commissionData = 'Dados de comissionamento não foram carregados corretamente';
    }
    return errors;
  };

  const formErrors = getFormErrors();
  const hasErrors = Object.keys(formErrors).length > 0;

  const handleOrigemChange = (origem: 'venda' | 'locacao') => {
    const commissionPct = origem === 'venda' ? tenantDefaults.sale : tenantDefaults.rent;
    draftDispatch({ type: 'SET_ORIGEM', origem, commissionPercent: commissionPct });
    draftDispatch({ type: 'SET_COMMISSION_ERROR', error: '' });
    setFormTouched({});
    setShowPreview(false);
  };

  const handleContractChange = async (e: React.ChangeEvent<HTMLSelectElement>) => {
    const cId = e.target.value;
    const fetchSeq = ++contractFetchSeq.current;
    setShowPreview(false);
    markTouched('contractId');
    if (!cId) {
      draftDispatch({ type: 'SET_CONTRACT', contractId: '' });
      draftDispatch({ type: 'SET_COMMISSION_ERROR', error: '' });
      return;
    }

    const contract = contracts.find(c => c.id === cId);
    // Busca o deal associado via contract.deal_id — não pelo contract.id,
    // que é diferente do deal.id. Sem isso, brokerParticipations nunca
    // é encontrado e os splits da esteira são ignorados. Seleção direta de
    // um [Negócio] também é suportada (contract fica undefined e o deal é o cId).
    const matchingDeal = contract?.deal_id
      ? deals.find((d) => d.id === contract.deal_id)
      : deals.find((d) => d.id === cId);
    if (!contract && !matchingDeal) {
      draftDispatch({ type: 'SET_CONTRACT', contractId: cId });
      return;
    }

    // Phase 6 — monta participantes iniciais a partir de (a) deal.broker_participations
    // ou (b) commissionRulesByRole como esqueleto vazio.
    const pickName = (brokerId?: string): string => {
      if (!brokerId) return '';
      const b = brokers.find((x) => x.id === brokerId);
      return b?.name || '';
    };
    const pickPct = (role: string): number => commissionRulesByRole[role] ?? 0;
    const effectiveType = (contract?.type || matchingDeal?.type || '').toLowerCase();
    const bps: Array<{ broker_id?: string; role?: string; percentage?: number }> =
      matchingDeal?.brokerParticipations || [];
    let newParticipants;
    const brokerParticipations = payableBrokerCommissionEntries(bps);
    if (brokerParticipations.length > 0) {
      newParticipants = brokerParticipations.map((bp, idx) => ({
        id: idx + 1,
        role: bp.role || 'Participante',
        name: pickName(bp.broker_id),
        percent: bp.percentage ?? pickPct(String(bp.role || '').toLowerCase()),
        brokerId: bp.broker_id,
      }));
    } else if (effectiveType === 'venda') {
      newParticipants = [
        { id: 1, role: 'captador', name: '', percent: pickPct('captador'), brokerId: undefined },
        { id: 2, role: 'fechamento', name: '', percent: pickPct('fechamento'), brokerId: undefined },
      ];
    } else {
      newParticipants = [
        { id: 1, role: 'fechamento', name: '', percent: pickPct('fechamento') || 100, brokerId: undefined },
      ];
    }

    draftDispatch({
      type: 'SET_CONTRACT',
      contractId: cId,
      baseValue: contract?.value || matchingDeal?.value || 0,
      commissionPercent: contract?.commission_pct ?? matchingDeal?.commission ?? 0,
      participants: newParticipants,
    });

    // === Busca automática dos dados programados na esteira via API ===
    // Preenche: valor do lançamento, plano de pagamentos, primeiro vencimento,
    // descontos aplicáveis (dação/troca/saldo) e distribuição de comissão.
    // Tudo permanece editável — o preenchimento é apenas a sugestão inicial.
    draftDispatch({ type: 'SET_COMMISSION_LOADING', loading: true });

    try {
      // Tenta primeiro por contract, fallback por deal
      let preview: FinancialPreview;
      try {
        preview = await financialPreviewService.getByContract(cId);
      } catch {
        // Fallback: tenta buscar pelo deal associado ao contrato
        // (ou pelo próprio negócio selecionado diretamente).
        if (matchingDeal?.id) {
          preview = await financialPreviewService.getByDeal(matchingDeal.id);
        } else {
          throw new Error('Sem deal_id no contrato para fallback');
        }
      }

      // Plano de pagamentos + primeiro vencimento derivados das cobranças projetadas.
      const planLabel = paymentPlanLabelFromPreview(preview);
      let firstDue = preview.charges?.[0]?.due_date
        ? preview.charges[0].due_date.slice(0, 10)
        : undefined;

      // Descontos aplicáveis: dação/troca/saldo do plano de venda.
      let discountSummary: string | undefined;
      if (contract && effectiveType === 'venda') {
        try {
          const salePlan = await saleBillingPlanService.getByContractId(cId);
          if (salePlan) {
            discountSummary = discountSummaryFromSalePlan(salePlan);
          }
        } catch {
          // Plano de venda opcional — segue sem resumo de descontos.
        }
      } else if (contract && firstDue === undefined) {
        // Locação: fallback do primeiro vencimento na configuração recorrente.
        try {
          const rentConfig = await rentalChargeConfigService.getByContractId(cId);
          if (rentConfig?.first_charge_date) {
            firstDue = rentConfig.first_charge_date.slice(0, 10);
          }
        } catch {
          // Configuração opcional — segue sem primeiro vencimento.
        }
      }

      // Troca de contrato/negócio durante a busca: descarta a resposta obsoleta.
      if (fetchSeq !== contractFetchSeq.current) return;

      const hasCommissions = preview.commissions && preview.commissions.length > 0;

      if (!hasCommissions) {
        // Contrato sem dados de comissionamento — alerta o usuário
        draftDispatch({
          type: 'SET_COMMISSION_ERROR',
          error: 'Este contrato não possui dados de comissionamento cadastrados. Verifique as regras de comissão e as participações de corretores no negócio.',
        });
        draftDispatch({
          type: 'SET_CONTRACT',
          contractId: cId,
          paymentPlanLabel: planLabel,
          firstDueDate: firstDue,
          discountSummary,
        });
        toast.warning('Sem dados de comissionamento', {
          description: 'O contrato selecionado não possui regras de comissionamento configuradas. Os participantes foram carregados com valores padrão.',
          duration: 6000,
        });

        // Audit: registro de busca sem dados de comissionamento
        appendAuditLogEntry({
          id: `comm-lookup-${Date.now()}`,
          entityType: 'contract',
          entityId: cId,
          action: 'edicao',
          changes: [{
            field: 'commission_distribution',
            oldValue: 'unloaded',
            newValue: 'empty',
          }],
          timestamp: new Date().toISOString(),
          user: 'sistema',
        });
      } else {
        // Mapeia as comissões da API para participantes do formulário
        const apiParticipants = payableBrokerCommissionEntries(preview.commissions)
          .map((comm, idx) => ({
            id: idx + 1,
            role: comm.role || 'Participante',
            name: pickName(comm.broker_id),
            percent: comm.percentage,
            brokerId: comm.broker_id || undefined,
            amount: comm.amount_cents / 100,
            commissionType: comm.role,
          }));

        // Se houver participantes da API, usa-os; senão mantém os do contexto local
        if (apiParticipants.length > 0) {
          const totalCommissionCents = preview.total_commission_cents;
          const totalCommissionValue = totalCommissionCents / 100;
          // Recalcula o commissionPercent com base no baseValue.
          // Quando o preview não tem total (CommissionPct ainda não
          // configurado no deal), usa o fallback do deal/contract.
          const effectiveBaseValue = contract?.value && contract.value > 0
            ? contract.value
            : matchingDeal?.value || 0;
          const effectiveCommissionPct = effectiveBaseValue > 0 && totalCommissionCents > 0
            ? (totalCommissionValue / effectiveBaseValue) * 100
            : matchingDeal?.commission ?? contract?.commission_pct ?? 0;

          draftDispatch({
            type: 'SET_COMMISSION_DATA',
            participants: apiParticipants,
            baseValue: effectiveBaseValue || contract?.value || 0,
            commissionPercent: Math.round(effectiveCommissionPct * 100) / 100,
            paymentPlanLabel: planLabel,
            firstDueDate: firstDue,
            discountSummary,
          });

          toast.success('Comissionamento carregado', {
            description: `${apiParticipants.length} participante(s) carregado(s) do contrato. Total: ${new Intl.NumberFormat('pt-BR', { style: 'currency', currency: 'BRL' }).format(totalCommissionValue)}`,
            duration: 4000,
          });
        } else {
          draftDispatch({
            type: 'SET_CONTRACT',
            contractId: cId,
            paymentPlanLabel: planLabel,
            firstDueDate: firstDue,
            discountSummary,
          });
        }

        // Audit: registro de carregamento bem-sucedido dos dados de comissionamento
        appendAuditLogEntry({
          id: `comm-load-${Date.now()}`,
          entityType: 'contract',
          entityId: cId,
          action: 'edicao',
          changes: [{
            field: 'commission_distribution',
            oldValue: 'unloaded',
            newValue: `loaded_${preview.commissions.length}_participants`,
          }],
          timestamp: new Date().toISOString(),
          user: 'sistema',
        });
      }
    } catch (err: any) {
      // Tratamento de exceção: API indisponível ou erro de rede
      console.warn('Finance: failed to load commission distribution', err);
      const errorMsg = err?.response?.data?.message || err?.message || 'Erro ao buscar dados de comissionamento';

      draftDispatch({
        type: 'SET_COMMISSION_ERROR',
        error: `Falha ao carregar dados de comissionamento: ${errorMsg}. Os participantes foram carregados com valores do contexto local.`,
      });

      toast.error('Erro ao buscar comissionamento', {
        description: 'Não foi possível carregar os dados de comissionamento do contrato. Usando dados disponíveis localmente.',
        duration: 5000,
      });

      // Audit: registro de falha na busca
      appendAuditLogEntry({
        id: `comm-error-${Date.now()}`,
        entityType: 'contract',
        entityId: cId,
        action: 'edicao',
        changes: [{
          field: 'commission_distribution',
          oldValue: 'unloaded',
          newValue: `error_${errorMsg.substring(0, 50)}`,
        }],
        timestamp: new Date().toISOString(),
        user: 'sistema',
      });
    }
  };

  // === NOVO: Handler de seleção de beneficiário para comissão avulsa ===
  // Quando o usuário seleciona um corretor/parceiro no formulário de comissão
  // standalone, auto-popula o percentual de comissão com base em:
  //   (a) participação do broker no deal (broker_participations)
  //   (b) regra padrão por role (commissionRulesByRole)
  //   (c) percentual geral do deal (deal.commission)
  //   (d) default do tenant conforme origem (venda/locação)
  //
  // IMPORTANTE: a participação do broker (broker_participations.percentage
  // e commissionRulesByRole) representa o SPLIT da comissão total
  // (ex: 30% = 30% da comissão de 6%), NÃO o percentual direto sobre o
  // valor base. O commissionPercent do draft é o percentual EFETIVO sobre
  // o baseValue — ex: 6% * 30% = 1.8% (não 30%).
  // A edição manual subsequente é sempre permitida — o auto-populate é
  // apenas uma sugestão inicial.
  const handleBeneficiaryChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const brokerId = e.target.value;
    setShowPreview(false);
    markTouched('beneficiary');

    if (!brokerId) {
      draftDispatch({ type: 'SET_BENEFICIARY', beneficiaryId: '', beneficiaryName: '' });
      return;
    }

    const broker = brokers.find(b => b.id === brokerId);
    if (!broker) {
      draftDispatch({ type: 'SET_BENEFICIARY', beneficiaryId: brokerId, beneficiaryName: brokerId });
      return;
    }

    draftDispatch({ type: 'SET_BENEFICIARY', beneficiaryId: brokerId, beneficiaryName: broker.name });

    // Resolve o percentual geral de comissão (sobre o valor do negócio).
    // Hierarquia: deal.commission → contract.commission_pct → tenant default.
    const matchingDeal = deals.find(d => d.id === selectedContractId);
    const bps: Array<{ broker_id?: string; role?: string; percentage?: number }> =
      matchingDeal?.brokerParticipations || [];
    const overallPct = matchingDeal?.commission
      ?? (lancamentoOrigem === 'venda' ? tenantDefaults.sale : tenantDefaults.rent);

    // Auto-popula o percentual de comissão a partir dos dados disponíveis:
    // 1. Participação do broker no deal (broker_participations)
    //    bpMatch.percentage é o SPLIT (ex: 30%), precisa ser convertido
    //    para percentual efetivo: overallPct * (splitPct / 100)
    const bpMatch = bps.find(bp => bp.broker_id === brokerId);
    if (bpMatch && bpMatch.percentage && bpMatch.percentage > 0) {
      const effectivePct = overallPct * (bpMatch.percentage / 100);
      draftDispatch({ type: 'SET_COMMISSION_PERCENT', value: effectivePct });
      return;
    }

    // 2. Regra padrão por role (commissionRulesByRole)
    //    Mesmo tratamento: rolePct é split, converter para efetivo.
    if (bpMatch?.role) {
      const rolePct = commissionRulesByRole[bpMatch.role.toLowerCase()];
      if (rolePct && rolePct > 0) {
        const effectivePct = overallPct * (rolePct / 100);
        draftDispatch({ type: 'SET_COMMISSION_PERCENT', value: effectivePct });
        return;
      }
    }

    // 3. Percentual geral do deal como fallback (já é o valor efetivo)
    const dealCommission = matchingDeal?.commission;
    if (dealCommission && dealCommission > 0) {
      draftDispatch({ type: 'SET_COMMISSION_PERCENT', value: dealCommission });
      return;
    }

    // 4. Fallback: default do tenant conforme origem (já é valor efetivo)
    draftDispatch({ type: 'SET_COMMISSION_PERCENT', value: overallPct });
  };

  const handleParticipantPercentChange = (id: number, newPercent: string) => {
    const val = parseFloat(newPercent) || 0;
    draftDispatch({ type: 'UPDATE_PARTICIPANT_PERCENT', id, percent: val });
  };

  const handleParticipantBrokerChange = (id: number, brokerId: string) => {
    const broker = brokers.find((candidate) => candidate.id === brokerId);
    draftDispatch({
      type: 'UPDATE_PARTICIPANT_BROKER',
      id,
      brokerId,
      name: broker?.name || '',
    });
  };

  const handleParticipantRoleChange = (id: number, role: string) => {
    draftDispatch({ type: 'UPDATE_PARTICIPANT_ROLE', id, role });
  };

  const handleAddParticipant = () => {
    draftDispatch({
      type: 'ADD_PARTICIPANT',
      participant: { id: Date.now(), role: 'fechamento', name: '', percent: 0 },
    });
  };

  const handleRemoveParticipant = (id: number) => {
    draftDispatch({ type: 'REMOVE_PARTICIPANT', id });
  };

  const foundContract = contracts.find(c => c.id === selectedContractId);
  const foundDeal = deals.find(d => d.id === selectedContractId);
  const selectedContract = foundContract ? {
    id: foundContract.id,
    client: foundContract.client,
    property_title: foundContract.property_title
  } : foundDeal ? {
    id: foundDeal.id,
    client: foundDeal.client,
    property_title: foundDeal.property
  } : undefined;

  const totalCommissionValue = baseValue * (commissionPercent / 100);
  const totalCommissionPreviewCents = Math.max(0, Math.round(totalCommissionValue * 100));
  const commissionAllocationPreview = commissionDistribution.invalid || commissionDistribution.overallocated
    ? { brokerPoolCents: 0, brokerCents: [] as number[], agencyCents: 0 }
    : allocateManualCommissionCents(
      totalCommissionPreviewCents,
      distributeCommission ? payableCommissionParticipants.map((participant) => participant.percent) : [],
    );
  const ownerSalePreviewCents = Math.max(0, Math.round(baseValue * 100) - totalCommissionPreviewCents);
  const baseCentsForPreview = Math.round(baseValue * 100);
  const isRentReceipt = lancamentoTipo === 'recebimento' && lancamentoOrigem === 'locacao';
  const adminFeePreviewCents = isRentReceipt
    ? Math.round(baseCentsForPreview * (tenantDefaults.adminFeePct / 100))
    : 0;
  // A corretagem da locação é congelada na assinatura e financiada automaticamente
  // na baixa; o repasse exibido aqui é o valor antes da intermediação.
  const ownerRentPreviewCents = Math.max(0, baseCentsForPreview - adminFeePreviewCents);

  const ParticipantRow = ({ id, role, name, brokerId, percent, totalVal }: { id: number, role: string, name: string, brokerId?: string, percent: number, totalVal: number }) => {
    const calculatedValue = totalVal * (percent / 100);
    return (
      <div className="flex items-center gap-3 bg-card p-3 rounded-xl border border-border group hover:border-border transition-colors">
        <div className="w-8 h-8 rounded-lg bg-muted flex items-center justify-center shrink-0">
          <Users className="w-4 h-4 text-muted-foreground" />
        </div>
        <div className="flex-1 min-w-0">
          <select
            value={brokerId || ''}
            onChange={(event) => handleParticipantBrokerChange(id, event.target.value)}
            className="w-full px-0 py-0 bg-transparent border-none text-sm font-medium text-foreground focus:outline-none focus:ring-0 cursor-pointer"
          >
            <option value="">Selecione um corretor...</option>
            {brokerId && !brokers.some((broker) => broker.id === brokerId) && (
              <option value={brokerId}>{name || brokerId}</option>
            )}
            {brokers.map((broker) => (
              <option key={broker.id} value={broker.id}>{broker.name}</option>
            ))}
          </select>
          <select
            value={role}
            onChange={(event) => handleParticipantRoleChange(id, event.target.value)}
            aria-label="Papel financeiro"
            className="mt-0.5 w-full bg-transparent border-none p-0 text-micro font-medium text-muted-foreground uppercase tracking-wider focus:outline-none focus:ring-0 cursor-pointer"
          >
            {COMMISSION_BROKER_ROLE_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </select>
        </div>
        <div className="flex items-center gap-3 shrink-0">
          <div className="relative w-20">
            <input
              type="number"
              value={percent}
              onChange={(e) => handleParticipantPercentChange(id, e.target.value)}
              className="w-full pl-2 pr-6 py-1.5 bg-muted border border-border rounded-lg text-sm text-center font-medium focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
            />
            <span className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground text-xs font-medium">%</span>
          </div>
          <div className="w-28 text-right">
            <p className="text-sm font-bold text-foreground">{formatCurrency(calculatedValue)}</p>
          </div>
          <Button variant="ghost" size="icon" onClick={() => handleRemoveParticipant(id)} className="size-7 text-muted-foreground/60 hover:text-status-critical hover:bg-status-critical-soft opacity-0 group-hover:opacity-100" title="Remover parte">
            <X className="w-3.5 h-3.5" />
          </Button>
        </div>
      </div>
    );
  };

  const renderLancamentos = () => {
    const filteredContracts = contracts.filter(c => c.type === lancamentoOrigem);
    const convertedDeals = deals.filter(d =>
      d.stage === 'Concluído' &&
      (d.type.toLowerCase() === lancamentoOrigem ||
        (d.type === 'Locação' && lancamentoOrigem === 'locacao'))
    );

    const availableSources = [
      ...filteredContracts.map(c => ({ id: c.id, title: c.property_title, client: c.client, isDeal: false })),
      ...convertedDeals.map(d => ({ id: d.id, title: d.property, client: d.client, isDeal: true }))
    ];

    const hasDateFilter = lancDateFrom || lancDateTo;
    const recentLaunches = transactions
      .filter(t => t.type === 'receita' || t.type === 'comissao' || t.type === 'repasse')
      .filter(t => {
        if (!hasDateFilter) return true;
        const d = t.date;
        if (lancDateFrom && d < lancDateFrom) return false;
        if (lancDateTo && d > lancDateTo) return false;
        return true;
      })
      .slice(0, hasDateFilter ? 50 : 5)
      .map(tx => {
        const contract = contracts.find(c => c.id === tx.contractId);
        return { ...tx, contractLabel: contract?.property_title || tx.contractId, client: contract?.client || 'N/A' };
      });

    const totalReceitas = transactions.filter(t => t.type === 'receita').reduce((a, t) => a + Math.abs(t.value), 0);
    const totalComissoes = transactions.filter(t => t.type === 'comissao' || t.type === 'repasse').reduce((a, t) => a + Math.abs(t.value), 0);
    const totalPendentes = transactions.filter(t => (t.type === 'receita' || t.type === 'comissao') && t.status === 'Pendente').length;

    const summaryStats = [
      { label: "Receitas Lancadas", value: formatCurrency(totalReceitas), icon: DollarSign, iconBg: "bg-status-ok-soft", iconColor: "text-status-ok" },
      { label: "Comissões geradas", value: formatCurrency(totalComissoes), icon: TrendingDown, iconBg: "bg-primary-100", iconColor: "text-primary-600" },
      { label: "Pendentes", value: String(totalPendentes), icon: Clock, iconBg: "bg-status-warning-soft", iconColor: "text-status-warning" },
      { label: "Contratos Ativos", value: String(contracts.length), icon: FileText, iconBg: "bg-muted", iconColor: "text-muted-foreground" },
    ];

    const totalDistribuido = commissionDistribution.brokerPercentage;
    const saldoRestante = commissionDistribution.agencyPercentage;

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
              <div className="bg-primary-50/50 border border-primary-200 rounded-2xl p-5">
                <div className="flex items-start gap-4">
                  <div className="p-2.5 bg-primary-100 rounded-xl shrink-0">
                    <DollarSign className="w-5 h-5 text-primary-600" />
                  </div>
                  <div>
                    <h3 className="text-base font-bold text-foreground mb-1">Fluxo de geração</h3>
                    <p className="text-sm text-muted-foreground">
                      1. Selecione a origem (Venda ou Locação) e o contrato. <br />
                      2. Gere recebimentos ou configure o comissionamento. <br />
                      3. Adicione corretores e participantes para distribuir a comissão.
                    </p>
                  </div>
                </div>
              </div>
            </motion.div>
          )}
        </AnimatePresence>

        {/* Header */}
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
                    <h2 className="text-2xl font-bold text-foreground">Geração por contrato</h2>
                    <span className="px-2.5 py-0.5 bg-muted text-muted-foreground text-xs font-bold rounded-full border border-border">{transactions.length} registros</span>
                  </div>
                  <p className="text-sm text-muted-foreground mt-1">Gere recebimentos de contratos e distribua comissionamentos</p>
                </div>
              </div>
            </motion.div>
          )}
        </AnimatePresence>

        {/* Stats Cards */}
        <AnimatePresence>
          {!focusMode && (
            <motion.div
              initial={{ opacity: 0, height: 0 }}
              animate={{ opacity: 1, height: "auto" }}
              exit={{ opacity: 0, height: 0, overflow: 'hidden' }}
              transition={{ duration: 0.3, ease: "easeInOut" }}
            >
              <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
                {summaryStats.map((stat, i) => (
                  <div key={i} className="bg-card rounded-2xl border border-border p-4 shadow-sm min-w-0">
                    <div className="flex items-center gap-3">
                      <div className={clsx("w-10 h-10 shrink-0 rounded-xl flex items-center justify-center", stat.iconBg)}>
                        <stat.icon className={clsx("w-5 h-5", stat.iconColor)} />
                      </div>
                      <div className="min-w-0 flex-1">
                        <p className="text-micro font-bold text-muted-foreground uppercase tracking-wider truncate">{stat.label}</p>
                        <p className="text-xl font-bold text-foreground mt-0.5 break-words leading-tight">{stat.value}</p>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </motion.div>
          )}
        </AnimatePresence>

        {/* Form Card */}
        <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
          {/* Form Header */}
          <div className="px-6 py-4 border-b border-border flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div className="flex items-center gap-3">
              <div className="p-2 bg-primary-50 rounded-xl">
                {lancamentoTipo === 'recebimento' ? <DollarSign className="w-5 h-5 text-primary-600" /> : <TrendingDown className="w-5 h-5 text-primary-600" />}
              </div>
              <div>
                <h3 className="font-bold text-foreground">
                  {lancamentoTipo === 'recebimento' ? 'Novo Recebimento' : 'Novo Comissionamento'}
                </h3>
                <p className="text-xs text-muted-foreground mt-0.5">
                  {lancamentoTipo === 'recebimento' ? 'Lance um recebimento vinculado a contrato' : 'Registre comissão para corretor ou parceiro'}
                </p>
              </div>
            </div>

            {/* Tipo Toggle */}
            <div className="flex bg-muted p-1 rounded-xl border border-border">
              <button
                onClick={() => { draftDispatch({ type: 'SET_TIPO', tipo: 'recebimento' }); setShowPreview(false); }}
                className={clsx(
                  "px-4 py-2 text-xs font-bold rounded-lg transition-all",
                  lancamentoTipo === 'recebimento' ? "bg-card text-primary-700 shadow-sm" : "text-muted-foreground hover:text-foreground"
                )}
              >
                Recebimento
              </button>
              <button
                onClick={() => { draftDispatch({ type: 'SET_TIPO', tipo: 'comissao' }); setShowPreview(false); }}
                className={clsx(
                  "px-4 py-2 text-xs font-bold rounded-lg transition-all",
                  lancamentoTipo === 'comissao' ? "bg-card text-primary-700 shadow-sm" : "text-muted-foreground hover:text-foreground"
                )}
              >
                Comissão
              </button>
            </div>
          </div>

          <div className="p-6 space-y-6">
            {/* Section 1: Origem */}
            <div>
              <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-3">Origem do Contrato</p>
              <div className="flex gap-3">
                <button
                  onClick={() => handleOrigemChange('venda')}
                  className={clsx(
                    "flex-1 py-2.5 rounded-xl text-sm font-bold transition-all border",
                    lancamentoOrigem === 'venda' ? "bg-primary-50 border-primary-200 text-primary-700" : "bg-card border-border text-muted-foreground hover:bg-muted"
                  )}
                >
                  Venda de Imóvel
                </button>
                <button
                  onClick={() => handleOrigemChange('locacao')}
                  className={clsx(
                    "flex-1 py-2.5 rounded-xl text-sm font-bold transition-all border",
                    lancamentoOrigem === 'locacao' ? "bg-primary-50 border-primary-200 text-primary-700" : "bg-card border-border text-muted-foreground hover:bg-muted"
                  )}
                >
                  Locação
                </button>
              </div>
            </div>

            {/* Section 2: Dados do formulario */}
            {lancamentoTipo === 'recebimento' ? (
              <>
                <div className="bg-muted rounded-xl p-5 border border-border space-y-4">
                  <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Dados do Recebimento</p>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <motion.div
                      key={`contract-shake-${shakeKey}`}
                      animate={formTouched.contractId && formErrors.contractId && shakeKey > 0 ? { x: [0, -8, 8, -8, 8, -4, 4, 0] } : {}}
                      transition={{ duration: 0.4 }}
                    >
                      <label className="block text-sm font-medium text-foreground mb-1.5">
                        Contrato / Imóvel <span className="text-status-critical">*</span>
                      </label>
                      <select
                        value={selectedContractId}
                        onChange={handleContractChange}
                        onBlur={() => markTouched('contractId')}
                        className={clsx(
                          "w-full px-3 py-2.5 bg-card border rounded-xl text-sm focus:outline-none focus:ring-2 transition-all",
                          formTouched.contractId && formErrors.contractId
                            ? "border-status-critical/40 focus:ring-rose-600/20 focus:border-status-critical"
                            : "border-border focus:ring-primary-600/20 focus:border-primary-600"
                        )}
                      >
                        <option value="">Selecione o contrato ou negócio</option>
                        {availableSources.map(s => (
                          <option key={s.id} value={s.id}>
                            {s.isDeal ? '[Negócio] ' : ''}{s.id} - {s.title} ({s.client})
                          </option>
                        ))}
                      </select>
                      {formTouched.contractId && formErrors.contractId && (
                        <p className="text-caption text-status-critical font-medium mt-1 flex items-center gap-1">
                          <AlertCircle className="w-3 h-3" /> {formErrors.contractId}
                        </p>
                      )}
                    </motion.div>
                    <div>
                      <label className="block text-sm font-medium text-foreground mb-1.5">Cliente Pagador</label>
                      <input type="text" className="w-full px-3 py-2.5 bg-muted border border-border rounded-xl text-sm text-muted-foreground" disabled value={selectedContract?.client || ''} placeholder="Automático ao selecionar contrato" />
                    </div>
                  </div>

                  <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                    <motion.div
                      key={`value-shake-${shakeKey}`}
                      animate={formTouched.baseValue && formErrors.baseValue && shakeKey > 0 ? { x: [0, -8, 8, -8, 8, -4, 4, 0] } : {}}
                      transition={{ duration: 0.4 }}
                    >
                      <label className="block text-sm font-medium text-foreground mb-1.5">
                        Valor Total (R$) <span className="text-status-critical">*</span>
                      </label>
                      <MoneyInput
                        value={typeof baseValue === 'number' ? baseValue : undefined}
                        onChange={(v) => draftDispatch({ type: 'SET_BASE_VALUE', value: v ?? 0 })}
                        onBlur={() => markTouched('baseValue')}
                        placeholder="0,00"
                        className={clsx(
                          "w-full px-3 py-2.5 bg-card border rounded-xl text-sm focus:outline-none focus:ring-2 transition-all",
                          formTouched.baseValue && formErrors.baseValue
                            ? "border-status-critical/40 focus:ring-rose-600/20 focus:border-status-critical"
                            : "border-border focus:ring-primary-600/20 focus:border-primary-600"
                        )}
                      />
                      {formTouched.baseValue && formErrors.baseValue && (
                        <p className="text-caption text-status-critical font-medium mt-1 flex items-center gap-1">
                          <AlertCircle className="w-3 h-3" /> {formErrors.baseValue}
                        </p>
                      )}
                    </motion.div>
                    <div>
                      <label className="block text-sm font-medium text-foreground mb-1.5">
                        {lancamentoOrigem === 'venda' ? 'Parcelas' : 'Recorrencia'}
                      </label>
                      <select
                        value={paymentPlanLabel}
                        onChange={(e) => draftDispatch({ type: 'SET_PAYMENT_PLAN', label: e.target.value })}
                        className="w-full px-3 py-2.5 bg-card border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 transition-all"
                      >
                        {paymentPlanLabel && (
                          <option value={paymentPlanLabel}>{paymentPlanLabel} (da esteira)</option>
                        )}
                        {lancamentoOrigem === 'venda' ? (
                          <>
                            <option value="A vista (1x)">A vista (1x)</option>
                            <option value="Entrada + 12x">Entrada + 12x</option>
                            <option value="Entrada + 24x">Entrada + 24x</option>
                          </>
                        ) : (
                          <>
                            <option value="Mensal">Mensal</option>
                            <option value="Anual">Anual</option>
                          </>
                        )}
                      </select>
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-foreground mb-1.5">1o Vencimento</label>
                      <input
                        type="date"
                        value={firstDueDate}
                        onChange={(e) => draftDispatch({ type: 'SET_FIRST_DUE_DATE', value: e.target.value })}
                        className="w-full px-3 py-2.5 bg-card border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 transition-all"
                      />
                    </div>
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-foreground mb-1.5">Categoria / Conta</label>
                    <select className="w-full px-3 py-2.5 bg-card border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 transition-all">
                      <option>Receitas com Vendas</option>
                      <option>Receitas com Locação</option>
                      <option>Taxas Administrativas</option>
                    </select>
                  </div>

                  {discountSummary && (
                    <div className="flex items-start gap-2 rounded-xl border border-primary-200 bg-primary-50/60 px-3 py-2.5 text-xs">
                      <span className="font-semibold text-primary-700 shrink-0">Descontos aplicáveis (da esteira):</span>
                      <span className="text-muted-foreground">{discountSummary}</span>
                    </div>
                  )}
                </div>

                {/* Commission Distribution Section */}
                <div className="bg-muted rounded-xl border border-border overflow-hidden">
                  <button
                    onClick={() => draftDispatch({ type: 'SET_DISTRIBUTE', value: !distributeCommission })}
                    className="w-full flex items-center justify-between px-5 py-4 hover:bg-muted/50 transition-colors"
                  >
                    <div className="flex items-center gap-3">
                      <div className="p-2 bg-primary-100 rounded-lg">
                        <Users className="w-4 h-4 text-primary-600" />
                      </div>
                      <div className="text-left">
                        <p className="text-sm font-bold text-foreground">Distribuir Comissionamentos</p>
                        <p className="text-xs text-muted-foreground mt-0.5">Gerar contas a pagar para corretores e parceiros</p>
                      </div>
                    </div>
                    <div className={clsx(
                      "w-10 h-6 rounded-full transition-colors relative cursor-pointer shrink-0",
                      distributeCommission ? "bg-primary-600" : "bg-muted-foreground/30"
                    )}>
                      <div className={clsx(
                        "absolute top-0.5 w-5 h-5 bg-card rounded-full shadow-sm transition-transform",
                        distributeCommission ? "translate-x-4.5" : "translate-x-0.5"
                      )} />
                    </div>
                  </button>

                  <AnimatePresence>
                    {distributeCommission && (
                      <motion.div
                        initial={{ height: 0, opacity: 0 }}
                        animate={{ height: 'auto', opacity: 1 }}
                        exit={{ height: 0, opacity: 0 }}
                        className="overflow-hidden"
                      >
                        <div className="px-5 pb-5 space-y-4 border-t border-border pt-4">
                          {/* Loading indicator */}
                          {loadingCommission && (
                            <div className="flex items-center justify-center gap-3 py-6 bg-card rounded-xl border border-dashed border-border">
                              <div className="w-5 h-5 border-2 border-primary-600 border-t-transparent rounded-full animate-spin" />
                              <p className="text-sm text-muted-foreground">Carregando dados de comissionamento...</p>
                            </div>
                          )}

                          {/* Error/alert banner */}
                          {commissionDataError && !loadingCommission && (
                            <div className="flex items-start gap-3 p-4 bg-status-warning-soft border border-status-warning/30 rounded-xl">
                              <AlertCircle className="w-5 h-5 text-status-warning shrink-0 mt-0.5" />
                              <div>
                                <p className="text-sm font-bold text-status-warning">Atenção</p>
                                <p className="text-xs text-status-warning/80 mt-0.5">{commissionDataError}</p>
                              </div>
                            </div>
                          )}

                          {/* Success banner */}
                          {commissionDataLoaded && !loadingCommission && !commissionDataError && commissionBrokerParticipants.length > 0 && (
                            <div className="flex items-start gap-3 p-4 bg-status-ok-soft border border-status-ok/30 rounded-xl">
                              <Check className="w-5 h-5 text-status-ok shrink-0 mt-0.5" />
                              <div>
                                <p className="text-sm font-bold text-status-ok">Dados carregados</p>
                                <p className="text-xs text-status-ok/80 mt-0.5">
                                  {commissionBrokerParticipants.length} participante(s) com comissionamento carregados automaticamente do contrato.
                                </p>
                              </div>
                            </div>
                          )}

                          <div className="grid grid-cols-2 gap-4">
                            <div>
                              <label className="block text-xs font-medium text-muted-foreground mb-1.5">Comissão Total (%)</label>
                              <div className="relative">
                                <PercentInput
                                  value={commissionPercent}
                                  onChange={(v) => draftDispatch({ type: 'SET_COMMISSION_PERCENT', value: v ?? 0 })}
                                  className="w-full pl-3 pr-8 py-2.5 bg-card border border-border rounded-xl text-sm font-medium focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 transition-all"
                                />
                                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm font-medium">%</span>
                              </div>
                            </div>
                            <div>
                              <label className="block text-xs font-medium text-muted-foreground mb-1.5">Valor a Distribuir</label>
                              <div className="w-full px-3 py-2.5 bg-primary-50 border border-primary-200 rounded-xl text-sm font-bold text-primary-900 text-right">
                                {formatCurrency(totalCommissionValue)}
                              </div>
                            </div>
                          </div>

                          <div className="space-y-2">
                            {commissionBrokerParticipants.map((p) => (
                              <ParticipantRow key={p.id} id={p.id} role={p.role} name={p.name} brokerId={p.brokerId} percent={p.percent} totalVal={totalCommissionValue} />
                            ))}
                            {commissionBrokerParticipants.length === 0 && (
                              <div className="flex flex-col items-center justify-center py-6 text-center bg-card rounded-xl border border-dashed border-border">
                                <Users className="w-8 h-8 text-muted-foreground/60 mb-2" />
                                <p className="text-sm text-foreground font-medium">Nenhum corretor pagável informado</p>
                                <p className="text-xs text-muted-foreground mt-0.5">A comissão ficará integralmente com a imobiliária; você ainda pode adicionar participantes.</p>
                              </div>
                            )}
                          </div>

                          {commissionBrokerParticipants.length > 0 && (
                            <div className="bg-card rounded-xl border border-border p-3">
                              <div className="flex items-center justify-between text-xs mb-2">
                                <span className="font-medium text-muted-foreground">Distribuição Total</span>
                                <span className={clsx("font-bold", commissionDistribution.overallocated || commissionDistribution.invalid ? "text-status-critical" : "text-status-ok")}>
                                  {commissionDistribution.totalPercentage}% de 100%
                                </span>
                              </div>
                              <div className="w-full h-2 bg-muted rounded-full overflow-hidden">
                                <div
                                  className={clsx("h-full rounded-full transition-all", commissionDistribution.overallocated || commissionDistribution.invalid ? "bg-status-critical" : "bg-status-ok")}
                                  style={{ width: `${Math.min(commissionDistribution.totalPercentage, 100)}%` }}
                                />
                              </div>
                              {!commissionDistribution.overallocated && !commissionDistribution.invalid && (
                                <p className="text-micro text-muted-foreground font-medium mt-1.5">
                                  Corretores: {totalDistribuido}% · Imobiliária: {saldoRestante}%
                                  {saldoRestante > 0 ? ' (restante automático, sem selecionar participante)' : ''}
                                </p>
                              )}
                              {commissionDistribution.overallocated && (
                                <p className="text-micro text-status-critical font-medium mt-1.5">Excede em {totalDistribuido - 100}% o valor total</p>
                              )}
                            </div>
                          )}

                          <Button variant="outline" onClick={handleAddParticipant} className="w-full">
                            <Plus className="w-4 h-4" /> Adicionar Parte
                          </Button>
                        </div>
                      </motion.div>
                    )}
                  </AnimatePresence>
                </div>
              </>
            ) : (
              <div className="bg-muted rounded-xl p-5 border border-border space-y-4">
                <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Dados do Comissionamento</p>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <motion.div
                    key={`comissão-contract-shake-${shakeKey}`}
                    animate={formTouched.contractId && formErrors.contractId && shakeKey > 0 ? { x: [0, -8, 8, -8, 8, -4, 4, 0] } : {}}
                    transition={{ duration: 0.4 }}
                  >
                    <label className="block text-sm font-medium text-foreground mb-1.5">
                      Contrato de Origem <span className="text-status-critical">*</span>
                    </label>
                    <select
                      value={selectedContractId}
                      onChange={handleContractChange}
                      onBlur={() => markTouched('contractId')}
                      className={clsx(
                        "w-full px-3 py-2.5 bg-card border rounded-xl text-sm focus:outline-none focus:ring-2 transition-all",
                        formTouched.contractId && formErrors.contractId
                          ? "border-status-critical/40 focus:ring-rose-600/20 focus:border-status-critical"
                          : "border-border focus:ring-primary-600/20 focus:border-primary-600"
                      )}
                    >
                      <option value="">Selecione o contrato ou negócio</option>
                      {availableSources.map(s => (
                        <option key={s.id} value={s.id}>
                          {s.isDeal ? '[Negócio] ' : ''}{s.id} - {s.title}
                        </option>
                      ))}
                    </select>
                    {formTouched.contractId && formErrors.contractId && (
                      <p className="text-caption text-status-critical font-medium mt-1 flex items-center gap-1">
                        <AlertCircle className="w-3 h-3" /> {formErrors.contractId}
                      </p>
                    )}
                  </motion.div>
                  <motion.div
                    key={`beneficiary-shake-${shakeKey}`}
                    animate={formTouched.beneficiary && formErrors.beneficiary && shakeKey > 0 ? { x: [0, -8, 8, -8, 8, -4, 4, 0] } : {}}
                    transition={{ duration: 0.4 }}
                  >
                    <label className="block text-sm font-medium text-foreground mb-1.5">
                      Corretor / Parceiro <span className="text-status-critical">*</span>
                    </label>
                    <select
                      value={selectedBeneficiaryId}
                      onChange={handleBeneficiaryChange}
                      onBlur={() => markTouched('beneficiary')}
                      className={clsx(
                        "w-full px-3 py-2.5 bg-card border rounded-xl text-sm focus:outline-none focus:ring-2 transition-all",
                        formTouched.beneficiary && formErrors.beneficiary
                          ? "border-status-critical/40 focus:ring-rose-600/20 focus:border-status-critical"
                          : "border-border focus:ring-primary-600/20 focus:border-primary-600"
                      )}
                    >
                      <option value="">Selecione o beneficiario</option>
                      {!selectedContractId ? (
                        <option disabled>Selecione um contrato primeiro</option>
                      ) : commissionBrokerParticipants.length > 0 ? (
                        commissionBrokerParticipants.map(p => {
                          const brokerName = p.brokerId
                            ? (brokers.find(b => b.id === p.brokerId)?.name || p.name || `(escolher ${p.role})`)
                            : (p.name || `(escolher ${p.role})`);
                          return (
                            <option key={`${p.id}-${p.brokerId || p.role}`} value={p.brokerId || ''}>
                              {brokerName} ({p.role})
                            </option>
                          );
                        })
                      ) : brokers.length > 0 ? (
                        brokers.map((b) => (
                          <option key={b.id} value={b.id}>{b.name}</option>
                        ))
                      ) : (
                        <option disabled>Nenhum corretor disponível</option>
                      )}
                    </select>
                    {formTouched.beneficiary && formErrors.beneficiary && (
                      <p className="text-caption text-status-critical font-medium mt-1 flex items-center gap-1">
                        <AlertCircle className="w-3 h-3" /> {formErrors.beneficiary}
                      </p>
                    )}
                  </motion.div>
                </div>

                {/* Error/alert banner para comissão avulsa */}
                {commissionDataError && (
                  <div className="flex items-start gap-3 p-4 bg-status-warning-soft border border-status-warning/30 rounded-xl">
                    <AlertCircle className="w-5 h-5 text-status-warning shrink-0 mt-0.5" />
                    <div>
                      <p className="text-sm font-bold text-status-warning">Atenção</p>
                      <p className="text-xs text-status-warning/80 mt-0.5">{commissionDataError}</p>
                    </div>
                  </div>
                )}

                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                  <motion.div
                    key={`comissão-value-shake-${shakeKey}`}
                    animate={formTouched.baseValue && formErrors.baseValue && shakeKey > 0 ? { x: [0, -8, 8, -8, 8, -4, 4, 0] } : {}}
                    transition={{ duration: 0.4 }}
                  >
                    <label className="block text-sm font-medium text-foreground mb-1.5">
                      Base de Calculo (R$) <span className="text-status-critical">*</span>
                    </label>
                    <MoneyInput
                      value={typeof baseValue === 'number' ? baseValue : undefined}
                      onChange={(v) => draftDispatch({ type: 'SET_BASE_VALUE', value: v ?? 0 })}
                      onBlur={() => markTouched('baseValue')}
                      placeholder="0,00"
                      className={clsx(
                        "w-full px-3 py-2.5 bg-card border rounded-xl text-sm focus:outline-none focus:ring-2 transition-all",
                        formTouched.baseValue && formErrors.baseValue
                          ? "border-status-critical/40 focus:ring-rose-600/20 focus:border-status-critical"
                          : "border-border focus:ring-primary-600/20 focus:border-primary-600"
                      )}
                    />
                    {formTouched.baseValue && formErrors.baseValue && (
                      <p className="text-caption text-status-critical font-medium mt-1 flex items-center gap-1">
                        <AlertCircle className="w-3 h-3" /> {formErrors.baseValue}
                      </p>
                    )}
                  </motion.div>
                  <div>
                    <label className="block text-sm font-medium text-foreground mb-1.5">
                      % Comissão {selectedBeneficiaryId ? '' : ''}
                    </label>
                    <div className="relative">
                      <PercentInput
                        value={commissionPercent}
                        onChange={(v) => {
                          draftDispatch({ type: 'SET_COMMISSION_PERCENT', value: v ?? 0 });
                          markTouched('commissionPercent');
                        }}
                        className={clsx(
                          "w-full pl-3 pr-8 py-2.5 bg-card border rounded-xl text-sm focus:outline-none focus:ring-2 transition-all",
                          formTouched.commissionPercent && formErrors.commissionPercent
                            ? "border-status-critical/40 focus:ring-rose-600/20 focus:border-status-critical"
                            : "border-border focus:ring-primary-600/20 focus:border-primary-600"
                        )}
                      />
                      <span className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm font-medium">%</span>
                    </div>
                    {formTouched.commissionPercent && formErrors.commissionPercent && (
                      <p className="text-caption text-status-critical font-medium mt-1 flex items-center gap-1">
                        <AlertCircle className="w-3 h-3" /> {formErrors.commissionPercent}
                      </p>
                    )}
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-foreground mb-1.5">Valor a Pagar (R$)</label>
                    <div className="w-full px-3 py-2.5 bg-status-ok-soft border border-status-ok/30 rounded-xl text-sm font-bold text-status-ok text-right">
                      {formatCurrency(totalCommissionValue)}
                    </div>
                  </div>
                </div>

                {lancamentoOrigem === 'locacao' && (
                  <div className="bg-primary-50 p-4 rounded-xl border border-primary-100 flex items-start gap-3">
                    <Info className="w-4 h-4 text-primary-600 shrink-0 mt-0.5" />
                    <div className="text-xs">
                      <p className="font-bold text-primary-900">Regra de Locação</p>
                      <p className="text-primary-700 mt-0.5">Normalmente a comissão de locação equivale a 1 ou mais aluguéis, ou porcentagem da taxa de administração mensal.</p>
                    </div>
                  </div>
                )}

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-foreground mb-1.5">Condicao de Pagamento</label>
                    <select className="w-full px-3 py-2.5 bg-card border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 transition-all">
                      <option>A vista (Ato do recebimento)</option>
                      <option>Parcelado (Conforme cliente pagar)</option>
                    </select>
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-foreground mb-1.5">Data Prevista de Pgto</label>
                    <input type="date" className="w-full px-3 py-2.5 bg-card border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 transition-all" />
                  </div>
                </div>
              </div>
            )}

            {/* Actions */}
            <div className="flex items-center justify-between pt-2">
              <Button
                variant="ghost"
                onClick={() => {
                  const hadData = selectedContractId || baseValue > 0;
                  draftDispatch({ type: 'CLEAR', commissionPercent: tenantDefaults.sale });
                  draftDispatch({ type: 'SET_COMMISSION_ERROR', error: '' });
                  setFormTouched({});
                  setShowPreview(false);
                  clearDraft();
                  if (hadData) toast.info('Formulário limpo', { description: 'Todos os campos foram resetados.' });
                }}
              >
                Limpar
              </Button>
              <Button
                onClick={() => {
                  // Mark all as touched to show errors
                  setFormTouched({ contractId: true, baseValue: true, beneficiary: true, commissionPercent: true });
                  if (hasErrors) {
                    setShakeKey(prev => prev + 1);
                    toast.error('Preencha os campos obrigatorios', {
                      description: Object.values(formErrors).join('. '),
                    });
                    return;
                  }
                  setShowPreview(true);
                }}
              >
                <Eye className="w-4 h-4" /> Revisar Lançamento
              </Button>
            </div>

            {/* Preview Panel */}
            <AnimatePresence>
              {showPreview && !hasErrors && (
                <motion.div
                  initial={{ height: 0, opacity: 0 }}
                  animate={{ height: 'auto', opacity: 1 }}
                  exit={{ height: 0, opacity: 0 }}
                  transition={{ duration: 0.25 }}
                  className="overflow-hidden"
                >
                  <div className="bg-primary-50/50 border border-primary-200 rounded-xl p-5 space-y-4">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        <div className="p-1.5 bg-primary-100 rounded-lg">
                          <FileText className="w-4 h-4 text-primary-600" />
                        </div>
                        <h4 className="font-bold text-foreground">Resumo do Lançamento</h4>
                      </div>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => setShowPreview(false)}
                        className="size-8"
                      >
                        <X className="w-4 h-4" />
                      </Button>
                    </div>

                    <div className="bg-card rounded-xl border border-border divide-y divide-border">
                      <div className="flex items-center justify-between px-4 py-3">
                        <span className="text-xs font-medium text-muted-foreground">Tipo</span>
                        <span className="text-sm font-bold text-foreground">
                          {lancamentoTipo === 'recebimento' ? 'Recebimento' : 'Comissionamento'}
                        </span>
                      </div>
                      <div className="flex items-center justify-between px-4 py-3">
                        <span className="text-xs font-medium text-muted-foreground">Origem</span>
                        <span className="text-sm font-bold text-foreground">
                          {lancamentoOrigem === 'venda' ? 'Venda de Imóvel' : 'Locação'}
                        </span>
                      </div>
                      <div className="flex items-center justify-between px-4 py-3">
                        <span className="text-xs font-medium text-muted-foreground">Contrato</span>
                        <span className="text-sm font-bold text-foreground">
                          {selectedContract ? `${selectedContractId} - ${selectedContract.property_title}` : selectedContractId}
                        </span>
                      </div>
                      {selectedContract && (
                        <div className="flex items-center justify-between px-4 py-3">
                          <span className="text-xs font-medium text-muted-foreground">Cliente</span>
                          <span className="text-sm font-medium text-foreground">{selectedContract.client}</span>
                        </div>
                      )}
                      <div className="flex items-center justify-between px-4 py-3">
                        <span className="text-xs font-medium text-muted-foreground">Valor Principal</span>
                        <span className="text-sm font-bold text-foreground">
                          {lancamentoTipo === 'comissao'
                            ? formatCurrency(totalCommissionValue)
                            : formatCurrency(baseValue)}
                        </span>
                      </div>
                      {/* Preview específico para comissão avulsa standalone */}
                      {lancamentoTipo === 'comissao' && (
                        <>
                          <div className="flex items-center justify-between px-4 py-3">
                            <span className="text-xs font-medium text-muted-foreground">Base de Cálculo</span>
                            <span className="text-sm font-medium text-foreground">{formatCurrency(baseValue)}</span>
                          </div>
                          <div className="flex items-center justify-between px-4 py-3">
                            <span className="text-xs font-medium text-muted-foreground">Percentual</span>
                            <span className="text-sm font-medium text-foreground">{commissionPercent}%</span>
                          </div>
                          <div className="flex items-center justify-between px-4 py-3">
                            <span className="text-xs font-medium text-muted-foreground">Beneficiário</span>
                            <span className="text-sm font-bold text-foreground">
                              {selectedBeneficiaryName || 'Não selecionado'}
                            </span>
                          </div>
                        </>
                      )}
                      <div className="flex items-center justify-between px-4 py-3">
                        <span className="text-xs font-medium text-muted-foreground">Status Inicial</span>
                        <span className="inline-flex items-center px-2 py-0.5 rounded-md text-xs font-bold bg-status-warning-soft text-status-warning border border-status-warning/30">
                          <Clock className="w-3 h-3 mr-1" /> Pendente
                        </span>
                      </div>
                    </div>

                    {lancamentoTipo === 'recebimento' && lancamentoOrigem === 'venda' && (
                      <div className="bg-card rounded-xl border border-border overflow-hidden">
                        <div className="px-4 py-3 bg-muted border-b border-border flex items-center justify-between gap-3">
                          <div>
                            <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Destinos do valor</p>
                            <p className="text-xs text-muted-foreground mt-0.5">A baixa criará a obrigação de repasse ao proprietário.</p>
                          </div>
                          <span className="text-xs font-bold text-primary-700 bg-primary-50 border border-primary-200 rounded-full px-2.5 py-1">Venda</span>
                        </div>
                        <div className="divide-y divide-border">
                          <div className="flex items-center justify-between px-4 py-2.5 gap-4">
                            <span className="text-sm text-muted-foreground">Valor recebido</span>
                            <span className="text-sm font-bold text-foreground tabular-nums">{formatCurrency(baseValue)}</span>
                          </div>
                          <div className="flex items-center justify-between px-4 py-2.5 gap-4">
                            <span className="text-sm text-muted-foreground">Comissão total ({commissionPercent}%)</span>
                            <span className="text-sm font-bold text-foreground tabular-nums">{formatCurrency(totalCommissionValue)}</span>
                          </div>
                          {commissionAllocationPreview.brokerPoolCents > 0 && (
                            <div className="flex items-center justify-between px-4 py-2.5 gap-4">
                              <span className="text-sm text-muted-foreground">Comissões a corretores</span>
                              <span className="text-sm font-bold text-status-critical tabular-nums">−{formatCurrency(commissionAllocationPreview.brokerPoolCents / 100)}</span>
                            </div>
                          )}
                          <div className="flex items-center justify-between px-4 py-2.5 gap-4">
                            <span className="text-sm text-muted-foreground">Receita efetiva da imobiliária</span>
                            <span className="text-sm font-bold text-status-ok tabular-nums">{formatCurrency(commissionAllocationPreview.agencyCents / 100)}</span>
                          </div>
                          <div className="flex items-center justify-between px-4 py-3 gap-4 bg-status-ok-soft/50">
                            <span className="text-sm font-bold text-foreground">Obrigação ao proprietário</span>
                            <span className="text-base font-bold text-status-ok tabular-nums">{formatCurrency(ownerSalePreviewCents / 100)}</span>
                          </div>
                        </div>
                      </div>
                    )}

                    {isRentReceipt && (
                      <div className="bg-card rounded-xl border border-border overflow-hidden">
                        <div className="px-4 py-3 bg-muted border-b border-border flex items-center justify-between gap-3">
                          <div>
                            <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Destinos do valor</p>
                            <p className="text-xs text-muted-foreground mt-0.5">A baixa criará a obrigação de repasse ao proprietário.</p>
                          </div>
                          <span className="text-xs font-bold text-primary-700 bg-primary-50 border border-primary-200 rounded-full px-2.5 py-1">Locação</span>
                        </div>
                        <div className="divide-y divide-border">
                          <div className="flex items-center justify-between px-4 py-2.5 gap-4">
                            <span className="text-sm text-muted-foreground">Valor recebido</span>
                            <span className="text-sm font-bold text-foreground tabular-nums">{formatCurrency(baseValue)}</span>
                          </div>
                          {adminFeePreviewCents > 0 && (
                            <div className="flex items-center justify-between px-4 py-2.5 gap-4">
                              <span className="text-sm text-muted-foreground">Taxa de administração ({tenantDefaults.adminFeePct}%)</span>
                              <span className="text-sm font-bold text-status-critical tabular-nums">−{formatCurrency(adminFeePreviewCents / 100)}</span>
                            </div>
                          )}
                          <div className="flex items-center justify-between px-4 py-3 gap-4 bg-status-ok-soft/50">
                            <div>
                              <p className="text-sm font-bold text-foreground">Obrigação ao proprietário</p>
                              <p className="text-xs text-muted-foreground mt-0.5">A corretagem (intermediação) é financiada automaticamente na baixa.</p>
                            </div>
                            <span className="text-base font-bold text-status-ok tabular-nums">{formatCurrency(ownerRentPreviewCents / 100)}</span>
                          </div>
                        </div>
                      </div>
                    )}

                    {/* Commission split preview */}
                    {lancamentoTipo === 'recebimento' && lancamentoOrigem === 'venda' && distributeCommission && payableCommissionParticipants.length > 0 && (
                      <div className="bg-card rounded-xl border border-border overflow-hidden">
                        <div className="px-4 py-3 bg-muted border-b border-border">
                          <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Distribuição da Comissão</p>
                        </div>
                        <div className="divide-y divide-border">
                          {payableCommissionParticipants.map(p => {
                            const pValue = totalCommissionValue * (p.percent / 100);
                            return (
                              <div key={p.id} className="flex items-center justify-between px-4 py-2.5">
                                <div>
                                  <p className="text-sm font-medium text-foreground">{p.name}</p>
                                  <p className="text-micro text-muted-foreground uppercase">{p.role} - {p.percent}%</p>
                                </div>
                                <span className="text-sm font-bold text-status-critical">-{formatCurrency(pValue)}</span>
                              </div>
                            );
                          })}
                          {saldoRestante > 0 && !commissionDistribution.overallocated && !commissionDistribution.invalid && (
                            <div className="flex items-center justify-between px-4 py-2.5">
                              <div>
                                <p className="text-sm font-medium text-foreground">Imobiliária</p>
                                <p className="text-micro text-muted-foreground uppercase">
                                  Restante automático · {saldoRestante}% · sem contraparte
                                </p>
                              </div>
                              <span className="text-sm font-bold text-status-ok">
                                {formatCurrency(totalCommissionValue * (saldoRestante / 100))}
                              </span>
                            </div>
                          )}
                        </div>
                        <div className="px-4 py-3 bg-muted border-t border-border flex items-center justify-between">
                          <span className="text-xs font-bold text-muted-foreground">Total para corretores</span>
                          <span className="text-sm font-bold text-status-critical">
                            -{formatCurrency(payableCommissionParticipants.reduce((a, p) => a + totalCommissionValue * (p.percent / 100), 0))}
                          </span>
                        </div>
                      </div>
                    )}

                    <div className="flex items-center justify-between gap-3 pt-1">
                      <p className="text-caption text-muted-foreground">
                        {lancamentoTipo === 'recebimento' && lancamentoOrigem === 'venda'
                          ? `Será gerado 1 recebimento${payableCommissionParticipants.length > 0 ? ` + ${payableCommissionParticipants.length} ${payableCommissionParticipants.length > 1 ? 'comissões' : 'comissão'}` : ''}; o repasse do proprietário nasce somente na baixa`
                          : lancamentoTipo === 'recebimento' && distributeCommission && payableCommissionParticipants.length > 0
                          ? `Serão gerados ${1 + payableCommissionParticipants.length} lançamentos no total; o restante da imobiliária é retido automaticamente`
                          : 'Será gerado 1 lançamento'
                        }
                      </p>
                      <Button
                        disabled={manualPostingSaving}
                        onClick={async () => {
                          if (manualPostingInFlightRef.current) return;
                          if (!selectedContractId) {
                            toast.error('Selecione um contrato ou negócio antes de prosseguir.');
                            return;
                          }
                          if (!activeTenantID) {
                            toast.error('Selecione uma empresa antes de gerar o lançamento.');
                            return;
                          }
                          if (baseValue <= 0) {
                            toast.error('O valor deve ser maior que zero.');
                            return;
                          }

                          // Validações para comissão avulsa standalone
                          if (lancamentoTipo === 'comissao') {
                            if (!selectedBeneficiaryId) {
                              toast.error('Selecione o corretor/parceiro beneficiário.');
                              return;
                            }
                            if (!Number.isFinite(commissionPercent) || commissionPercent <= 0 || commissionPercent > 100) {
                              toast.error('Informe um percentual de comissão entre 0% e 100%.');
                              return;
                            }
                            if (totalCommissionValue <= 0) {
                              toast.error('O valor da comissão a pagar deve ser maior que zero.');
                              return;
                            }
                          }

                          // Valida dados de comissionamento obrigatórios
                          if (lancamentoTipo === 'recebimento' && distributeCommission) {
                            if (commissionDataError) {
                              toast.error('Dados de comissionamento não foram carregados corretamente.', {
                                description: commissionDataError,
                              });
                              return;
                            }
                            if (commissionDistribution.invalid) {
                              toast.error('Cada percentual da distribuição deve estar entre 0% e 100%.');
                              return;
                            }
                            if (commissionDistribution.overallocated) {
                              toast.error('A soma destinada aos corretores não pode passar de 100%.');
                              return;
                            }
                          }

                          // Comissão avulsa standalone: o valor da transação é o valor
                          // calculado da comissão (totalCommissionValue), NÃO o baseValue
                          // (que é apenas a base de cálculo). Isso garante independência
                          // de qualquer lançamento anterior — a comissão é criada do zero.
                          const baseCents = Math.round(baseValue * 100);
                          const totalCommissionCents = Math.round(baseCents * (commissionPercent / 100));
                          const effectiveValue = (lancamentoTipo === 'comissao' ? totalCommissionCents : baseCents) / 100;
                          const postingDate = localPostingDate();
                          const batchItems: CreateTransactionBatchItem[] = [{
                            contract_id: selectedContractId,
                            type: lancamentoTipo === 'recebimento' ? 'receita' as const : 'comissao' as const,
                            description: lancamentoTipo === 'comissao'
                              ? `Comissão Avulsa - ${selectedBeneficiaryName || 'Beneficiário'} (${commissionPercent}% de ${formatCurrency(baseValue)}) - ${lancamentoOrigem === 'venda' ? 'Venda' : 'Locação'}`
                              : `Recebimento - ${lancamentoOrigem === 'venda' ? 'Venda' : 'Locacao'}`,
                            value: effectiveValue,
                            date: postingDate,
                            ...(lancamentoTipo === 'recebimento' ? { flow: lancamentoOrigem === 'venda' ? 'sale' as const : 'rent' as const } : {}),
                            // Proveniência programada na esteira: primeiro vencimento e plano.
                            // Aplicada apenas ao recebimento — comissão avulsa não carrega plano.
                            ...(lancamentoTipo === 'recebimento' && firstDueDate ? { due_date: firstDueDate } : {}),
                            ...(lancamentoTipo === 'recebimento' && paymentPlanLabel ? { payment_plan: paymentPlanLabel } : {}),
                          }];

                          // Locação não distribui comissão no recebimento: a
                          // corretagem é congelada na assinatura e o recebimento
                          // financia o plano de intermediação automaticamente.
                          const eligibleParticipants = lancamentoTipo === 'recebimento' && distributeCommission && lancamentoOrigem === 'venda'
                            ? payableCommissionParticipants
                            : [];
                          const sourceDealID = foundContract?.deal_id || foundDeal?.id || '';
                          const sourceContractID = foundContract?.id || '';
                          const sourceDeal = foundContract?.deal_id
                            ? deals.find((deal) => deal.id === foundContract.deal_id)
                            : foundDeal;
                          const isSaleReceipt = lancamentoTipo === 'recebimento' && lancamentoOrigem === 'venda';
                          const requiresCommission = lancamentoTipo === 'comissao' || eligibleParticipants.length > 0 || isSaleReceipt || isRentReceipt;
                          if (requiresCommission && !sourceDealID) {
                            toast.error('O contrato selecionado não possui um negócio de origem válido.');
                            return;
                          }
                          if (requiresCommission && !sourceContractID) {
                            toast.error('Selecione um contrato formalizado para gerar obrigações de comissão.');
                            return;
                          }
                          if (isSaleReceipt && (!Number.isFinite(commissionPercent) || commissionPercent < 0 || commissionPercent > 100)) {
                            toast.error('Informe uma comissão total entre 0% e 100% para fechar o destino do recebimento.');
                            return;
                          }
                          if (lancamentoTipo === 'recebimento') {
                            batchItems[0].deal_id = sourceDealID;
                          }
                          const unresolvedParticipant = eligibleParticipants.find((p) => {
                            const role = String(p.role || '').toLowerCase();
                            return !p.brokerId || !isPayableBrokerCommissionRole(role);
                          });
                          if (unresolvedParticipant) {
                            toast.error(`Defina um corretor e um papel financeiro válido para ${unresolvedParticipant.name || unresolvedParticipant.role}.`);
                            return;
                          }
                          const commissionItems: ManualCommissionBatchItem[] = [];
                          if (lancamentoTipo === 'comissao' && selectedBeneficiaryId) {
                            const standaloneRole = String(
                              sourceDeal?.brokerParticipations?.find((participation) => participation.broker_id === selectedBeneficiaryId)?.role || '',
                            ).toLowerCase();
                            if (!isPayableBrokerCommissionRole(standaloneRole)) {
                              toast.error('O beneficiário selecionado não possui um papel financeiro válido neste negócio.');
                              return;
                            }
                            commissionItems.push({
                              deal_id: sourceDealID,
                              contract_id: sourceContractID,
                              broker_id: selectedBeneficiaryId,
                              role: standaloneRole,
                              percentage: commissionPercent,
                              amount: totalCommissionCents / 100,
                            });
                            batchItems[0].broker_id = selectedBeneficiaryId;
                            batchItems[0].role = standaloneRole;
                          }
                          const commissionAllocation = allocateManualCommissionCents(
                            totalCommissionCents,
                            eligibleParticipants.map((participant) => participant.percent),
                          );
                          const allocatedCommissionCents = commissionAllocation.brokerCents;
                          const agencyRetentionPercentage = distributeCommission
                            ? commissionDistribution.agencyPercentage
                            : 100;
                          const agencyRetentionCents = distributeCommission
                            ? commissionAllocation.agencyCents
                            : totalCommissionCents;
                          const commissionRetention =
                            lancamentoTipo === 'recebimento' &&
                            isSaleReceipt &&
                            agencyRetentionPercentage > 0
                              ? {
                                percentage: agencyRetentionPercentage,
                                amount: agencyRetentionCents / 100,
                              }
                              : undefined;
                          if (allocatedCommissionCents.some((cents) => cents <= 0)) {
                            toast.error('O valor da comissão é insuficiente para distribuir ao menos R$ 0,01 por participante.');
                            return;
                          }
                          eligibleParticipants.forEach((p, index) => {
                            const role = String(p.role || '').toLowerCase();
                            const amount = allocatedCommissionCents[index] / 100;
                            commissionItems.push({
                              deal_id: sourceDealID,
                              contract_id: sourceContractID,
                              broker_id: p.brokerId!,
                              role,
                              percentage: p.percent,
                              amount,
                            });
                            // Commission is the payable aggregate; this row is
                            // its exact accounting mirror. It is not an owner
                            // transfer: real repasses are created only after a
                            // Charge is fully paid and are bounded by cash.
                            batchItems.push({
                              contract_id: selectedContractId,
                              type: 'comissao',
                              description: `Comissão - ${p.role} (${p.name || 'sem broker'})`,
                              value: amount,
                              date: postingDate,
                              broker_id: p.brokerId!,
                              role,
                            });
                          });

                          const intentFingerprint = manualPostingIntentFingerprint(
                            batchItems,
                            commissionItems,
                            commissionRetention,
                          );
                          const postingTenantID = activeTenantID;
                          let postingAttempt: ManualPostingAttempt;
                          try {
                            postingAttempt = manualPostingAttemptRef.current?.intentFingerprint === intentFingerprint
                              ? manualPostingAttemptRef.current
                              : findManualPostingAttempt(postingTenantID, intentFingerprint) ?? {
                                intentFingerprint,
                                operationID: typeof crypto !== 'undefined' && crypto.randomUUID
                                  ? `manual-posting:${crypto.randomUUID()}`
                                  : `manual-posting:${Date.now()}`,
                                createdAt: Date.now(),
                                items: batchItems,
                                commissions: commissionItems,
                                commissionRetention,
                              };
                            // Persist the exact date and monetary payload before the
                            // request. A lost response, reload, or midnight rollover
                            // must retry the same server-side idempotency key.
                            persistManualPostingAttempt(postingTenantID, postingAttempt);
                            manualPostingAttemptRef.current = postingAttempt;
                          } catch (error) {
                            console.error('manual financial posting persistence failed', error);
                            toast.error('Não foi possível preparar uma tentativa recuperável. O lançamento não foi enviado.');
                            return;
                          }
                          manualPostingInFlightRef.current = true;
                          setManualPostingSaving(true);
                          try {
                            await addTransactionBatch(
                              postingAttempt.items,
                              postingAttempt.operationID,
                              postingAttempt.commissions,
                              postingAttempt.commissionRetention,
                            );
                            try {
                              clearManualPostingAttempt(postingTenantID, postingAttempt.intentFingerprint);
                              manualPostingAttemptRef.current = null;
                            } catch (error) {
                              // The financial command is already committed. Keep the
                              // in-memory key so another click remains an idempotent
                              // retry instead of creating a duplicate obligation.
                              console.error('manual financial posting cleanup failed', error);
                              toast.warning('Lançamento concluído, mas a confirmação local não pôde ser limpa com segurança.');
                            }
                          } catch (err) {
                            console.error('manual financial posting failed', err);
                            toast.error('Não foi possível concluir todos os lançamentos.');
                            return;
                          } finally {
                            manualPostingInFlightRef.current = false;
                            setManualPostingSaving(false);
                          }

                          const commissionCount = eligibleParticipants.length;

                          clearDraft();

                          // Toast específico para comissão avulsa
                          if (lancamentoTipo === 'comissao') {
                            toast.success('Comissão avulsa lançada com sucesso', {
                              description: `${formatCurrency(totalCommissionValue)} para ${selectedBeneficiaryName} (${commissionPercent}% sobre ${formatCurrency(baseValue)})`,
                              duration: 8000,
                            });
                          } else {
                            toast.success(
                              'Recebimento gerado com sucesso',
                              {
                                description: commissionCount > 0
                                  ? `${formatCurrency(baseValue)} registrado + ${commissionCount} comissão${commissionCount > 1 ? 's' : ''} gerada${commissionCount > 1 ? 's' : ''}`
                                  : `${formatCurrency(baseValue)} registrado como receita pendente`,
                                duration: 8000,
                              }
                            );
                          }

                          // Reset apenas os campos do conteúdo do lançamento; preserva tipo/origem
                          // para o operador continuar lançando do mesmo contexto.
                          draftDispatch({ type: 'SET_CONTRACT', contractId: '' });
                          draftDispatch({ type: 'SET_COMMISSION_ERROR', error: '' });
                          setFormTouched({});
                          setShowPreview(false);
                        }}
                      >
                        <Check className="w-4 h-4" /> {manualPostingSaving ? 'Gerando...' : 'Confirmar e Gerar'}
                      </Button>
                    </div>
                  </div>
                </motion.div>
              )}
            </AnimatePresence>
          </div>
        </div>

        {/* Recent Launches Table */}
        <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
          <div className="px-6 py-4 border-b border-border flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
            <div className="flex items-center gap-3">
              <h3 className="font-bold text-foreground">Registros gerados</h3>
              <span className="px-2 py-0.5 bg-muted text-muted-foreground text-micro font-bold rounded-full border border-border">{recentLaunches.length}</span>
              {!hasDateFilter && <span className="text-micro text-muted-foreground">Últimos 5</span>}
            </div>
            <div className="flex items-center gap-3 flex-wrap">
              <div className="flex items-center gap-2">
                <Calendar className="w-3.5 h-3.5 text-muted-foreground" />
                <input
                  type="date"
                  value={lancDateFrom}
                  onChange={e => filtersDispatch({ type: 'SET_LANC_DATE_FROM', value: e.target.value })}
                  className="px-2.5 py-1.5 bg-muted border border-border rounded-lg text-xs focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
                  placeholder="De"
                />
                <span className="text-xs text-muted-foreground">ate</span>
                <input
                  type="date"
                  value={lancDateTo}
                  onChange={e => filtersDispatch({ type: 'SET_LANC_DATE_TO', value: e.target.value })}
                  className="px-2.5 py-1.5 bg-muted border border-border rounded-lg text-xs focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
                  placeholder="Ate"
                />
              </div>
              {hasDateFilter && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => filtersDispatch({ type: 'CLEAR_LANC_DATE' })}
                  className="text-status-critical hover:text-status-critical hover:bg-status-critical-soft"
                >
                  Limpar
                </Button>
              )}
              <Button
                variant="link"
                size="sm"
                onClick={() => navigate('/financeiro/cobrancas')}
              >
                Ver todos
              </Button>
            </div>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse min-w-150">
              <thead>
                <tr className="bg-muted border-b border-border text-micro font-bold text-muted-foreground uppercase tracking-wider">
                  <th className="px-6 py-3">ID</th>
                  <th className="px-6 py-3">Descrição</th>
                  <th className="px-6 py-3">Contrato</th>
                  <th className="px-6 py-3">Data</th>
                  <th className="px-6 py-3 text-right">Valor</th>
                  <th className="px-6 py-3">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {recentLaunches.map(tx => (
                  <tr key={tx.id} className="hover:bg-muted/50 transition-colors">
                    <td className="px-6 py-3">
                      <span className="text-micro font-mono font-medium text-muted-foreground bg-muted px-1.5 py-0.5 rounded border border-border">{tx.id}</span>
                    </td>
                    <td className="px-6 py-3 text-sm text-foreground">{tx.description}</td>
                    <td className="px-6 py-3 text-sm text-muted-foreground">{tx.contractLabel}</td>
                    <td className="px-6 py-3 text-sm text-muted-foreground">
                      <span className="flex items-center gap-1"><Calendar className="w-3 h-3 text-muted-foreground" /> {tx.date}</span>
                    </td>
                    <td className="px-6 py-3 text-sm font-bold text-right">
                      <span className={tx.value < 0 ? "text-status-critical" : "text-foreground"}>
                        {tx.value < 0 ? '-' : ''}{formatCurrency(tx.value)}
                      </span>
                    </td>
                    <td className="px-6 py-3">
                      <StatusBadge status={tx.status} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {recentLaunches.length === 0 && (
              <EmptyState
                icon={Search}
                message={hasDateFilter ? 'Nenhum lançamento encontrado no período selecionado.' : 'Nenhum lançamento registrado ainda.'}
                className="border-none bg-transparent my-6"
              />
            )}
          </div>
        </div>
      </div>
    );
  };





  /**
   * Ponto único de "registrar recebimento/baixa" no módulo: qualquer entrada do
   * painel (detalhe do lançamento, tabela de cobranças/lançamentos) resolve a
   * mesma UX por origem — cobrança abre o painel de baixa de cobrança e as
   * demais origens abrem a baixa padronizada de lançamento (método + data +
   * comprovante + observação). Nenhuma origem cai em AlertDialog de status.
   */
  const handleRegisterPayment = (tx: any, defaultAmount?: number) => {
    const action = resolveReceivableAction(tx, {
      defaultAmount,
      readOnly: activeModule === 'repasses',
    });
    if (action.kind === 'blocked') {
      // Sem baixa possível: orienta em vez de abrir painel ou diálogo.
      toast.warning(
        action.reason === 'read-only'
          ? 'Registros a apurar são somente leitura. Concilie a origem antes de movimentar valores.'
          : 'Esta fatura é uma projeção sem cobrança vinculada. Concilie a origem em Conciliação antes de dar baixa.',
      );
      return;
    }
    if (action.kind === 'charge') {
      // Cobrança é o agregado canônico — mesma baixa para a linha de cobrança e
      // para a projeção de fatura vinculada a ela.
      setMarkingPaid({ chargeId: action.chargeId, defaultAmount: action.defaultAmount });
      setSelectedTransaction(null);
      return;
    }
    // Lançamento independente: baixa padronizada (método + data + comprovante +
    // observação) no painel lateral. Antes a origem "invoice" abria um
    // AlertDialog que só trocava o status, sem capturar a evidência do
    // pagamento — a mesma ação tinha duas UX no módulo.
    setSettleLancamento({
      id: action.id,
      type: action.type,
      description: action.description,
      value: action.value,
    });
  };

  return (
    <div className="flex flex-col h-full bg-muted/50 overflow-auto font-sans relative">
      <div className="flex-none px-6 lg:px-8 py-4 border-b border-border bg-card sticky top-0 z-10 space-y-4">
        <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
          <div>
            <h1 className="text-2xl font-bold text-foreground tracking-tight">Visão financeira</h1>
            <p className="text-sm text-muted-foreground mt-1">Análise, geração e gestão do ciclo financeiro</p>
          </div>
          <Button variant="outline" size="sm" onClick={() => navigate('/financeiro/lancamentos')}>
            <Inbox className="w-4 h-4" /> Abrir lançamentos
          </Button>
        </div>
        <div className="flex bg-muted p-1 rounded-xl border border-border overflow-x-auto max-w-full w-fit">
          {FINANCE_AREA_TABS.map(tab => (
              <button
                key={tab.id}
                onClick={() => navigate(tab.path)}
                className={clsx(
                  "px-4 py-2 text-sm font-bold rounded-lg transition-all whitespace-nowrap",
                  activeModule === tab.id
                    ? "bg-card text-primary-700 shadow-sm"
                    : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
                )}
              >
                {tab.label}
              </button>
          ))}
        </div>
      </div>

      <div className="flex-1 p-5 lg:p-6 space-y-4">
        <AnimatePresence mode="wait">
          <motion.div
            key={activeModule}
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -10 }}
            transition={{ duration: 0.2 }}
          >
            {activeModule === 'overview' && <FinanceDashboard embedded />}
            {activeModule === 'lancamentos' && renderLancamentos()}
            {activeModule === 'cobrancas' && (
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
                filtersDispatch={filtersDispatch}
                navigate={navigate}
                onRegisterPayment={handleRegisterPayment}
                setSelectedTransaction={setSelectedTransaction}
                setEditing={setEditing}
                setShowNovaCobranca={setShowNovaCobranca}
                setSelectedChargeId={setSelectedChargeId}
                setExportConfig={setExportConfig}
                setAudit={setAudit}
                handleChargeAction={handleChargeAction}
              />
            )}
            {activeModule === 'comissoes' && (
              <div className="space-y-4">
                <div>
                  <h2 className="text-xl font-bold text-foreground">Comissões</h2>
                  <p className="text-sm text-muted-foreground">
                    Gestão de comissões dos corretores — aprovação, pagamento com comprovante, deduções e cancelamento.
                  </p>
                </div>
                <ComissoesTab />
              </div>
            )}
            {activeModule === 'repasses' && (
              <RepassesSection
                transfers={transfers}
                contracts={contracts}
                transactions={transactions}
                focusMode={focusMode}
                loadingInvoices={loadingInvoices}
                repasseSearch={repasseSearch}
                repasseStatusFilter={repasseStatusFilter}
                repasseStatusOpen={repasseStatusOpen}
                filtersDispatch={filtersDispatch}
                approveTransfer={approveTransfer}
                processTransfer={processTransfer}
                openProcessRepasse={openProcessRepasse}
                setSelectedTransaction={setSelectedTransaction}
                setExportConfig={setExportConfig}
                setAudit={setAudit}
                openConfirm={openConfirm}
                closeConfirm={closeConfirm}
              />
            )}
            {activeModule === 'split_payouts' && <Navigate to="/financeiro/payouts" replace />}
            {activeModule === 'contas' && (
              <ContasSection
                contracts={contracts}
                selectedContractId={selectedContractId}
                focusMode={focusMode}
                contasSearch={contasSearch}
                filtersDispatch={filtersDispatch}
                setExportConfig={setExportConfig}
              />
            )}
            {activeModule === 'conciliacao' && (
              <ConciliacaoSection
                contracts={contracts}
                transactions={contextTransactions}
                charges={charges}
                transfers={transfers}
                focusMode={focusMode}
                conciliacaoSearch={conciliacaoSearch}
                conciliacaoStatusFilter={conciliacaoStatusFilter}
                conciliacaoStatusOpen={conciliacaoStatusOpen}
                filtersDispatch={filtersDispatch}
                setExportConfig={setExportConfig}
              />
            )}
          </motion.div>
        </AnimatePresence>
      </div>

		{/* Slide-over Detail Modal for Cobranças */}
		<TransactionDetailSlideover
			transaction={selectedTransaction}
			isExpanded={isExpanded}
			onToggleExpand={toggleExpand}
			onClose={() => setSelectedTransaction(null)}
			title={activeModule === 'repasses' ? 'Detalhes do repasse' : 'Detalhes da movimentação'}
			canRegisterPayment={activeModule === 'repasses' ? false : undefined}
			operationalNotice={activeModule === 'repasses'
				? 'A apurar: este registro legado não possui vínculo econômico confiável e permanece somente leitura até a conciliação da origem.'
				: undefined}
			onRegisterPayment={handleRegisterPayment}
		/>

      {settleLancamento && (
        <SettleLancamentoModal
          lancamento={settleLancamento}
          onClose={() => setSettleLancamento(null)}
          onSaved={() => {
            setSettleLancamento(null);
            setSelectedTransaction(null);
            void refetchTransactions();
            void loadInvoices();
          }}
        />
      )}

      {/* Edit Transaction Modal */}
      {editing && (
        <TransactionEditModal
          transaction={editing.tx}
          mode={editing.mode}
          onClose={() => setEditing(null)}
        />
      )}

      {/* Charge receipt — fiscal-coupon-styled demonstrativo. Opens from
     the "Recibo" action button on each charge row. */}
      {receiptChargeId && (() => {
        const charge = charges.find((c: Charge) => c.id === receiptChargeId);
        if (!charge) return null;
        const contract = contracts.find(c => c.id === charge.contract_id);
        return (
          <ChargeReceiptModal
            charge={charge}
            contractDisplayID={contract?.display_id || contract?.id?.slice(0, 8).toUpperCase()}
            payerName={contract?.client}
            onClose={() => setReceiptChargeId(null)}
          />
        );
      })()}

      {/* Nova Cobranca Modal */}
      {showNovaCobranca && (
        <NovaCobrancaModal
          contracts={contracts}
          properties={properties}
          onClose={() => setShowNovaCobranca(false)}
          onSave={async (data) => {
            await addCharge(data as any);
            toast.success('Cobrança criada com sucesso');
          }}
        />
      )}

      {/* Mark Paid Modal */}
      {markingPaid && (
        <MarkPaidModal
          chargeId={markingPaid.chargeId}
          defaultAmount={markingPaid.defaultAmount || 0}
          charge={markingPaid.charge}
          onClose={() => setMarkingPaid(null)}
          onSave={async (chargeId, data) => {
            await markChargePaid(chargeId, data);
            // Sem toast aqui — CRMContext.markChargePaid já dispara o toast.
          }}
        />
      )}

      {/* === Charge management surface (Phase 7) === */}
      {/* Charge Detail Drawer — abre ao clicar em "Detalhes" de uma linha
     cuja origem é 'charge'. Resolve a charge live da lista para refletir
     status pós-ação sem precisar reabrir. */}
      {(() => {
        const live = selectedChargeId ? charges.find((c: Charge) => c.id === selectedChargeId) : null;
        const contract = live ? contracts.find((c) => c.id === live.contract_id) : undefined;
        const property = live ? properties.find((p) => p.id === live.property_id) : undefined;
        return (
          <ChargeDetailDrawer
            charge={live ?? null}
            contract={contract ? { id: contract.id, property_title: contract.property_title, client: contract.client } : undefined}
            propertyLabel={
              // property.address é um objeto PropertyAddress — usar o formatter
              // dedicado em vez de passar o objeto cru ao drawer.
              property?.title ||
              (property?.address ? formatPropertyAddress(property.address) : undefined)
            }
            tenantLabel={contract?.client}
            // OwnerRef em PropertyDTO carrega apenas { id, type, percentage?, since? }
            // — sem `.name`. Mesma estratégia de PropertyDetails.tsx
            // (linhas ~1090): owner_id casa com lead.id no CRM. Fallback
            // para o id cru quando o lead ainda não foi carregado.
            ownerLabel={(() => {
              const ownerId = property?.owners?.[0]?.id;
              if (!ownerId) return undefined;
              const ownerLead = leads.find((l) => l.id === ownerId);
              return ownerLead?.name ?? ownerId;
            })()}
            onClose={() => setSelectedChargeId(null)}
            onAction={(action, charge) => {
              // Fecha o drawer ao despachar ação que abre outro modal — evita
              // sobreposição visual (modal por trás do drawer).
              if (action !== 'notify' && action !== 'receipt') {
                setSelectedChargeId(null);
              }
              handleChargeAction(action, charge);
            }}
          />
        );
      })()}

      {/* Editar Cobrança Modal — modo dual do NovaCobrancaModal */}
      {editingChargeId && (() => {
        const live = charges.find((c: Charge) => c.id === editingChargeId);
        if (!live) return null;
        return (
          <NovaCobrancaModal
            mode="edit"
            initial={{
              id: live.id,
              status: live.status,
              contract_id: live.contract_id,
              type: live.type,
              description: live.description,
              reference_month: live.reference_month,
              amount: live.amount,
              due_date: live.due_date,
              discount: live.discount,
              // Preserva a composição econômica (titular/modo/versão de regra)
              // na edição — §8/F1.1: a interface deve preservar e exibir os campos.
              line_items: live.line_items,
            }}
            contracts={contracts}
            properties={properties}
            onClose={() => setEditingChargeId(null)}
            onSave={async (data, id) => {
              // mode='edit' → discriminated union garante data é UpdateChargeDTO
              // e id é string (não opcional). Sem cast.
              await updateCharge(id, data);
            }}
          />
        );
      })()}

      {/* Cancelar Cobrança Dialog */}
      {cancellingChargeId && (() => {
        const live = charges.find((c: Charge) => c.id === cancellingChargeId);
        if (!live) return null;
        return (
          <CancelarCobrancaDialog
            open={!!cancellingChargeId}
            contextLine={live.description}
            onCancel={() => setCancellingChargeId(null)}
            onConfirm={async (reason) => {
              try {
                await cancelCharge(live.id, reason);
                setCancellingChargeId(null);
              } catch {
                // toast já tratado em CRMContext.cancelCharge; mantém o dialog
                // aberto para o operador ajustar e reenviar.
              }
            }}
          />
        );
      })()}

      {/* Export Report Modal */}
      {exportConfig && (
        <ExportReportModal
          config={exportConfig}
          onClose={() => setExportConfig(null)}
        />
      )}

      {/* Audit Trail Panel */}
      {audit && (
        <SharedAuditTrailPanel
          entityId={audit.entityId === '__all__' ? undefined : audit.entityId}
          entityLabel={audit.label}
          onClose={() => setAudit(null)}
          getAuditLog={getAuditLog}
        />
      )}

      {/* Confirm Dialog */}
      <ConfirmDialog
        open={!!confirmDialog}
        title={confirmDialog?.title ?? ""}
        description={confirmDialog?.description ?? ""}
        confirmLabel={confirmDialog?.confirmLabel}
        variant={confirmDialog?.variant}
        doubleConfirm={confirmDialog?.doubleConfirm}
        doubleConfirmLabel={confirmDialog?.doubleConfirmLabel}
        onConfirm={confirmDialog?.onConfirm ?? (() => {})}
        onCancel={closeConfirm}
      />

      {/* Processar repasse (com upload de comprovante — fluxo externo) */}
      {modalState.modal === "processRepasse" && (
        <ProcessRepasseModal
          transferId={modalState.transferId}
          transfer={modalState.transfer}
          onClose={closeAnyModal}
          onProcess={async (transferId, evidence) => {
            await processTransfer(transferId, evidence);
            closeAnyModal();
            toast.success("Repasse processado", { description: "Comprovante registrado no Trilha de auditoria." });
          }}
        />
      )}
    </div>
  );
}
