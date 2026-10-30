import { FinanceLoadError } from "../components/finance/FinanceLoadError";
import { useMemo, useState, useCallback, useEffect, useRef } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { AlertOctagon, Plus } from "lucide-react";
import { toast } from "sonner";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "../ui/tabs";
import { parseOperationView } from "./lancamentosView";
import { Button } from "../ui/button";

import { LancamentosKpis, type LancamentosKpiView } from "../components/lancamentos/LancamentosKpis";
import { LancamentosFilters } from "../components/lancamentos/LancamentosFilters";
import { LancamentosTable } from "../components/lancamentos/LancamentosTable";
import { OperationsExplorer, type OperationViewMode } from "../components/lancamentos/OperationsExplorer";
import { LancamentoSimpleDrawer } from "../components/lancamentos/LancamentoSimpleDrawer";
import { MonthlySummaryView } from "../components/lancamentos/MonthlySummaryView";
import { NovoLancamentoModal, type NovoLancamentoKind } from "../components/lancamentos/NovoLancamentoModal";
import { NovaComissaoModal } from "../components/lancamentos/NovaComissaoModal";
import { NovaTransacaoModal, type NovaTransacaoDirection } from "../components/lancamentos/NovaTransacaoModal";
import { SettleLancamentoModal, type SettleLancamentoTarget } from "../components/lancamentos/SettleLancamentoModal";
import type { TransactionDTO } from "../services/transaction.service";
import { PagadoresView, type PagadorGroup } from "../components/lancamentos/PagadoresView";

import { ChargeDetailDrawer } from "../components/ChargeDetailDrawer";
import { CommissionDetailDrawer, type CommissionDrawerAction } from "../components/CommissionDetailDrawer";
import { NovaCobrancaModal } from "../components/NovaCobrancaModal";
import { MarkPaidModal } from "../components/MarkPaidModal";
import { MarkPaidCommissionModal, type MarkPaidCommissionMode } from "../components/MarkPaidCommissionModal";
import { ProcessRepasseModal } from "../components/ProcessRepasseModal";
import { CancelarCobrancaDialog } from "../components/CancelarCobrancaDialog";
import { SettlementEffects } from "../components/SettlementEffects";
import { FinanceAreaNav } from "../components/finance/FinanceAreaNav";

import { applyFilters, groupByKind, useLancamentosList, type LancamentosFilters as FiltersState } from "../queries/lancamentos";
import type { Lancamento, LancamentoKind, LancamentoStatus } from "../lib/lancamentos/types";
import { enrichLancamentos, groupEconomicOperations, groupByMonth, lancamentoMonth } from "../lib/lancamentos/presentation";
import type { OperationGroupId, OperationSortId } from "../lib/lancamentos/operationsView";
import { deriveSettlementEffects, settlementContextLabel } from "../lib/lancamentos/settlement";
import { getOwnerTransferPresentation } from "../lib/finance/ownerTransferFlow";

import { chargeService, type Charge } from "../services/charge.service";
import { commissionService, type Commission } from "../services/commission.service";
import { transferService, type OwnerTransfer } from "../services/transfer.service";
import { transactionService } from "../services/transaction.service";

import { useContractsContext } from "../context/ContractsContext";
import { usePropertiesContext } from "../context/PropertiesContext";
import { useLeadsContext } from "../context/LeadsContext";
import { useAgentsContext } from "../context/AgentsContext";
import { usePermissionsContext } from "../context/PermissionsContext";

import { formatCurrencyBRL0 } from "../utils/format";
import { mapChargeStatus } from "../utils/charge";
import { notifyApiError } from "../services/errorToast";

type ActiveTab = LancamentoKind | "all" | "ledger" | "pagadores" | "mensal";
const OPEN_STATUSES = ["pendente", "aprovado", "parcial", "vencido"] as const;
const FILTER_STATUSES = new Set<LancamentoStatus>([
  "pendente",
  "aprovado",
  "pago",
  "processado",
  "vencido",
  "parcial",
  "cancelado",
  "revertido",
]);

const isRentalContractType = (value?: string) => {
  const normalized = (value || "").trim().toLowerCase();
  return normalized === "rent" || normalized === "rental" || normalized === "locacao" || normalized === "locação";
};

function parseStatuses(value: string | null): LancamentoStatus[] {
  if (!value) return [];
  return value
    .split(",")
    .filter((status): status is LancamentoStatus => FILTER_STATUSES.has(status as LancamentoStatus));
}

function parseKindTab(v: string | null): ActiveTab {
  if (!v) return "all";
  if (v === "all") return "all";
  if (v === "ledger") return "ledger";
  if (v === "pagadores") return "pagadores";
  if (v === "mensal") return "mensal";
  if (v === "charge" || v === "commission" || v === "transfer" || v === "royalty" || v === "transaction") return v;
  return "all";
}

const OPERATION_GROUP_IDS: OperationGroupId[] = ["competence", "due", "origin", "kind", "status", "counterparty", "none"];
const OPERATION_SORT_IDS: OperationSortId[] = ["focus", "due", "recent", "amount"];

function parseOperationGroup(value: string | null): OperationGroupId {
  return OPERATION_GROUP_IDS.includes(value as OperationGroupId) ? (value as OperationGroupId) : "competence";
}

function parseOperationSort(value: string | null): OperationSortId {
  return OPERATION_SORT_IDS.includes(value as OperationSortId) ? (value as OperationSortId) : "focus";
}

function kindLabel(kind: ActiveTab) {
  switch (kind) {
    case "charge":
      return "Cobranças";
    case "commission":
      return "Comissões";
    case "transfer":
      return "Repasses";
    case "royalty":
      return "Parcerias";
    case "transaction":
      return "Caixa";
    case "pagadores":
      return "Partes";
    case "ledger":
      return "Livro";
    case "mensal":
      return "Visão mensal";
    default:
      return "Operações";
  }
}

type DrawerKind = "charge" | "commission" | "transfer" | "royalty" | "transaction" | null;

type PendingLancAction = "approve" | "cancel" | "reverse" | "settle";

/**
 * Textos de confirmação das ações de lançamento sem evidência. A conclusão de
 * repasse sem saldo é a única que encerra o registro sem lançamento de caixa —
 * e por isso deixa isso explícito na descrição.
 */
const LANC_ACTION_COPY: Record<PendingLancAction, {
  title: string;
  description: string;
  confirmLabel: string;
  loadingText: string;
}> = {
  approve: {
    title: "Aprovar repasse",
    description: "A aprovação autoriza o repasse ao proprietário. O processamento (com comprovante) é a etapa seguinte.",
    confirmLabel: "Aprovar repasse",
    loadingText: "Aprovando...",
  },
  cancel: {
    title: "Cancelar lançamento",
    description: "O lançamento será marcado como CANCELADO e as comissões vinculadas pendentes também serão canceladas. Depois, você pode criar um novo lançamento.",
    confirmLabel: "Cancelar lançamento",
    loadingText: "Cancelando...",
  },
  reverse: {
    title: "Estornar recebimento",
    description: "O recebimento será estornado (valor negativo), as comissões vinculadas serão revertidas/canceladas e o repasse ao proprietário pendente será cancelado. Depois, você pode criar um novo lançamento.",
    confirmLabel: "Estornar recebimento",
    loadingText: "Estornando...",
  },
  settle: {
    title: "Concluir repasse sem saldo",
    description: "Não há valor a transferir nesta competência: o repasse será encerrado sem comprovante e nenhuma movimentação de caixa será registrada. Esta ação não pode ser desfeita.",
    confirmLabel: "Concluir sem saldo",
    loadingText: "Concluindo...",
  },
};

export function LancamentosInbox() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const [activeTab, setActiveTab] = useState<ActiveTab>(() => parseKindTab(searchParams.get("kind")));
  const [filters, setFilters] = useState<FiltersState>(() => {
    const counterparty_id = searchParams.get("counterparty_id");
    const counterparty_kind = searchParams.get("counterparty_kind") || undefined;
    const q = searchParams.get("q") || "";
    const divergentOnly = searchParams.get("divergent") === "1";
    const flow = searchParams.get("flow");
    const dateBasis = searchParams.get("date_basis");
    return {
      search: q,
      statuses: parseStatuses(searchParams.get("status")),
      from: searchParams.get("from") || undefined,
      to: searchParams.get("to") || undefined,
      divergentOnly: divergentOnly || undefined,
      flow: flow === "receber" || flow === "pagar" ? flow : undefined,
      dateBasis: dateBasis === "settled" || dateBasis === "created" ? dateBasis : "operational",
      counterparty: counterparty_id ? { id: counterparty_id, kind: (counterparty_kind as any) || undefined } : undefined,
      competence: searchParams.get("competence") || undefined,
      dueMonth: searchParams.get("due_month") || undefined,
      overdueOnly: searchParams.get("overdue") === "1" || undefined,
    };
  });

  // Recorte e leitura da aba de operações ficam na URL: o link compartilhado
  // reabre exatamente o mês, o agrupamento e a visão que o operador estava usando.
  const [operationView, setOperationView] = useState<OperationViewMode>(() => parseOperationView(searchParams.get("view")));
  const [operationGroup, setOperationGroup] = useState<OperationGroupId>(() => parseOperationGroup(searchParams.get("group")));
  const [operationSort, setOperationSort] = useState<OperationSortId>(() => parseOperationSort(searchParams.get("sort")));

  const { can } = usePermissionsContext();
  const canWrite = can("finance:write");
  const canApproveTransfer = can("finance:approve_transfer");

  const { contracts } = useContractsContext();
  const { properties } = usePropertiesContext();
  const { leads } = useLeadsContext();
  const { agents } = useAgentsContext();

  const { data: rawLancamentos, isLoading, partialFailures, refetchAll } = useLancamentosList();
  const allLancamentos = useMemo(
    () => enrichLancamentos(rawLancamentos, { contracts, properties, leads, agents }),
    [rawLancamentos, contracts, properties, leads, agents],
  );
  const lancamentos = useMemo(() => applyFilters(allLancamentos, filters), [allLancamentos, filters]);
  const grouped = useMemo(() => groupByKind(lancamentos), [lancamentos]);
  const operations = useMemo(() => groupEconomicOperations(lancamentos), [lancamentos]);
  // Facetas dos atalhos: aplicam os filtros do painel, mas não os atalhos de
  // estado — assim a contagem não pisca ao ligar/desligar "Em atraso".
  const operationFacets = useMemo(
    () => groupEconomicOperations(applyFilters(allLancamentos, { ...filters, overdueOnly: undefined, flow: undefined, divergentOnly: undefined, statuses: [] })),
    [allLancamentos, filters],
  );
  const monthly = useMemo(() => groupByMonth(lancamentos), [lancamentos]);
  const [selectedMonth, setSelectedMonth] = useState<string | null>(null);
  const monthlyItems = useMemo(
    () => (selectedMonth ? lancamentos.filter((l) => lancamentoMonth(l) === selectedMonth) : []),
    [selectedMonth, lancamentos],
  );
  const allCount = lancamentos.length;

  const [drawerOpen, setDrawerOpen] = useState<DrawerKind>(null);
  const [selectedLancamento, setSelectedLancamento] = useState<Lancamento | null>(null);

  const [showNovoLancamento, setShowNovoLancamento] = useState(false);
  const [novoKind, setNovoKind] = useState<NovoLancamentoKind | null>(null);

  const [editingCharge, setEditingCharge] = useState<Charge | null>(null);
  const [markPaidCharge, setMarkPaidCharge] = useState<Charge | null>(null);
  const [cancelCharge, setCancelCharge] = useState<Charge | null>(null);

  const [editingCommission, setEditingCommission] = useState<Commission | null>(null);
  const [markPaidCommission, setMarkPaidCommission] = useState<Commission | null>(null);
  const [markPaidCommissionMode, setMarkPaidCommissionMode] = useState<MarkPaidCommissionMode>("mark-paid");
  const [cancelCommission, setCancelCommission] = useState<Commission | null>(null);
  const [reverseCommission, setReverseCommission] = useState<Commission | null>(null);

  // Confirmações que não exigem evidência: cancelamento, estorno e a conclusão
  // de repasse sem saldo são ações críticas e destrutivas. A baixa do lançamento
  // e o processamento do repasse usam o painel lateral padrão, com comprovante.
  const [pendingLancAction, setPendingLancAction] = useState<{
    action: PendingLancAction;
    lancamento: Lancamento;
  } | null>(null);

  const [processingTransfer, setProcessingTransfer] = useState<OwnerTransfer | null>(null);
  const [novaTransacaoDir, setNovaTransacaoDir] = useState<NovaTransacaoDirection | null>(null);
  const [settlingTransactionId, setSettlingTransactionId] = useState<string | null>(null);
  const settlingTransactionRef = useRef<string | null>(null);
  // Baixa padronizada de lançamento (método + data + comprovante).
  const [settleTarget, setSettleTarget] = useState<SettleLancamentoTarget | null>(null);
  const [transferActionKey, setTransferActionKey] = useState<string | null>(null);
  const transferActionRef = useRef<string | null>(null);

  useEffect(() => {
    const next = parseKindTab(searchParams.get("kind"));
    setActiveTab(next);
    setOperationView(parseOperationView(searchParams.get("view")));
    setOperationGroup(parseOperationGroup(searchParams.get("group")));
    setOperationSort(parseOperationSort(searchParams.get("sort")));
  }, [searchParams]);

  const updateUrl = useCallback(
    (patch: Record<string, string | null | undefined>) => {
      const next = new URLSearchParams(searchParams);
      Object.entries(patch).forEach(([k, v]) => {
        if (!v) next.delete(k);
        else next.set(k, v);
      });
      setSearchParams(next, { replace: true });
    },
    [searchParams, setSearchParams],
  );

  const onTabChange = (tab: ActiveTab) => {
    setActiveTab(tab);
    updateUrl({ kind: tab === "all" ? null : tab });
  };

  const handleKpiSelect = (view: LancamentosKpiView) => {
    // Atalho de KPI é um recorte de estado: zera competência/vencimento e os
    // demais recortes para não misturar dois contextos na mesma tela.
    const next: FiltersState = {
      ...filters,
      statuses: view === "receber" || view === "pagar" ? [...OPEN_STATUSES] : view === "vencidos" ? ["vencido"] : [],
      flow: view === "receber" || view === "pagar" ? view : undefined,
      divergentOnly: view === "divergencias" ? true : undefined,
      overdueOnly: undefined,
      competence: undefined,
      dueMonth: undefined,
      from: undefined,
      to: undefined,
      dateBasis: "operational",
    };
    setFilters(next);
    setActiveTab("all");
    updateUrl({
      kind: null,
      flow: next.flow,
      divergent: next.divergentOnly ? "1" : null,
      status: next.statuses?.join(",") || null,
      from: null,
      to: null,
      date_basis: null,
      competence: null,
      due_month: null,
      overdue: null,
    });
  };

  const handleFiltersChange = useCallback(
    (next: FiltersState) => {
      setFilters(next);
      updateUrl({
        q: next.search || null,
        divergent: next.divergentOnly ? "1" : null,
        flow: next.flow,
        status: next.statuses?.join(",") || null,
        from: next.from,
        to: next.to,
        date_basis: next.dateBasis && next.dateBasis !== "operational" ? next.dateBasis : null,
        competence: next.competence,
        due_month: next.dueMonth,
        overdue: next.overdueOnly ? "1" : null,
      });
    },
    [updateUrl],
  );

  const handleSelect = (l: Lancamento) => {
    setSelectedLancamento(l);
    setDrawerOpen(l.kind);
  };

  const closeDrawer = () => {
    setDrawerOpen(null);
    setSelectedLancamento(null);
  };

  const openGenericDrawer = (l: Lancamento) => {
    setSelectedLancamento(l);
    setDrawerOpen(l.kind);
  };

  const openChargeDetail = async (l: Lancamento) => {
    if (l.kind !== "charge") return;
    setSelectedLancamento(l);
    const charge = l.source as Charge;
    setEditingCharge(charge);
  };

  const openCommissionDetail = async (l: Lancamento) => {
    if (l.kind !== "commission") return;
    setSelectedLancamento(l);
    const commission = l.source as Commission;
    setEditingCommission(commission);
  };

  const openTransferDetail = (l: Lancamento) => {
    openGenericDrawer(l);
  };

  const openRoyaltyDetail = (l: Lancamento) => {
    openGenericDrawer(l);
  };

  const navigateToSpecific = (l: Lancamento) => {
    if (l.kind === "charge") {
      navigate("/financeiro?module=cobrancas");
      return;
    }
    if (l.kind === "commission") {
      navigate("/financeiro?module=comissoes");
      return;
    }
    if (l.kind === "transfer") {
      navigate("/financeiro?module=repasses");
      return;
    }
    if (l.kind === "royalty") {
      navigate("/financeiro/payouts");
      return;
    }
    if (l.kind === "transaction") {
      navigate("/financeiro/lancamentos?kind=transaction");
      return;
    }
  };

  const handleSettleTransaction = async (l: Lancamento) => {
    if (l.kind !== "transaction") return;
    if (settlingTransactionRef.current === l.id) return;

    // Fluxo padrão de baixa: método + data + comprovante (upload/link) +
    // observação, igual aos demais pagamentos do financeiro.
    const tx = l.source as TransactionDTO;
    setSettleTarget({
      id: l.id,
      type: tx?.type ?? "receita",
      description: tx?.description || l.counterparty.label || "Lançamento",
      value: l.amount,
    });
  };

  const handleCancelTransaction = async (l: Lancamento) => {
    if (l.kind !== "transaction") return;
    try {
      await transactionService.cancel(l.id, "Cancelamento manual do recebimento");
      toast.success("Lançamento cancelado. Você pode criar um novo lançamento.");
      closeDrawer();
      refetchAll();
    } catch (err) {
      const error = err as { response?: { status?: number; data?: { message?: string } } };
      if (error.response?.status === 404 || error.response?.status === 409) {
        closeDrawer();
        refetchAll();
        toast.warning("Este lançamento já foi cancelado ou alterado por outra operação. A lista foi atualizada.");
      } else {
        console.error(err);
        notifyApiError(error, { context: "Cancelar lançamento" });
      }
    }
  };

  const handleReverseTransaction = async (l: Lancamento) => {
    if (l.kind !== "transaction") return;
    try {
      await transactionService.reverse(l.id, "Estorno do recebimento para novo lançamento");
      toast.success("Recebimento estornado. Você pode criar um novo lançamento.");
      closeDrawer();
      refetchAll();
    } catch (err) {
      const error = err as { response?: { status?: number; data?: { message?: string } } };
      if (error.response?.status === 404 || error.response?.status === 409) {
        closeDrawer();
        refetchAll();
        toast.warning("Este lançamento já foi estornado ou alterado por outra operação. A lista foi atualizada.");
      } else {
        console.error(err);
        notifyApiError(error, { context: "Estornar recebimento" });
      }
    }
  };

  const handleApproveTransfer = async (l: Lancamento) => {
    if (l.kind !== "transfer") return;
    const actionKey = `approve:${l.id}`;
    if (transferActionRef.current === actionKey) return;
    transferActionRef.current = actionKey;
    setTransferActionKey(actionKey);
    try {
      await transferService.approve(l.id);
      toast.success("Repasse aprovado.");
      closeDrawer();
      refetchAll();
    } catch (err) {
      const error = err as { response?: { status?: number; data?: { message?: string } } };
      if (error.response?.status === 404 || error.response?.status === 409) {
        closeDrawer();
        refetchAll();
        toast.warning("O repasse foi alterado por outra operação. A lista foi atualizada; confira o status atual.");
      } else {
        console.error(err);
        notifyApiError(error, { context: "Aprovar repasse" });
      }
    } finally {
      if (transferActionRef.current === actionKey) {
        transferActionRef.current = null;
        setTransferActionKey(null);
      }
    }
  };

  const handleProcessTransfer = async (
    transferId: string,
    evidence: { proof_url?: string; note?: string },
  ) => {
    const actionKey = `process:${transferId}`;
    if (transferActionRef.current === actionKey) return;
    transferActionRef.current = actionKey;
    setTransferActionKey(actionKey);
    try {
      await transferService.process(transferId, evidence);
      toast.success("Repasse processado.");
      closeDrawer();
      refetchAll();
    } catch (err) {
      const error = err as { response?: { status?: number; data?: { message?: string } } };
      if (error.response?.status === 404 || error.response?.status === 409) {
        closeDrawer();
        refetchAll();
        toast.warning("O repasse foi alterado por outra operação. A lista foi atualizada; confira o status atual.");
      } else {
        console.error(err);
        throw err;
      }
    } finally {
      if (transferActionRef.current === actionKey) {
        transferActionRef.current = null;
        setTransferActionKey(null);
      }
    }
  };

  /**
   * Conclusão de repasse sem saldo a repassar: o backend encerra o repasse sem
   * exigir comprovante (nada sai do caixa). Mantém a mesma proteção contra
   * duplo clique e o mesmo tratamento de conflito das demais ações de repasse.
   */
  const handleSettleTransferWithoutBalance = async (l: Lancamento) => {
    if (l.kind !== "transfer") return;
    const actionKey = `settle:${l.id}`;
    if (transferActionRef.current === actionKey) return;
    transferActionRef.current = actionKey;
    setTransferActionKey(actionKey);
    try {
      await transferService.process(l.id, {});
      toast.success("Repasse concluído sem saldo.", {
        description: "Nenhuma movimentação de caixa foi registrada.",
      });
      closeDrawer();
      refetchAll();
    } catch (err) {
      const error = err as { response?: { status?: number; data?: { message?: string } } };
      if (error.response?.status === 404 || error.response?.status === 409) {
        closeDrawer();
        refetchAll();
        toast.warning("O repasse foi alterado por outra operação. A lista foi atualizada; confira o status atual.");
      } else {
        console.error(err);
        toast.error(
          error.response?.data?.message ||
            "Não foi possível concluir o repasse sem saldo. O repasse mudou ou não está mais disponível; atualize a página e tente novamente.",
        );
      }
    } finally {
      if (transferActionRef.current === actionKey) {
        transferActionRef.current = null;
        setTransferActionKey(null);
      }
    }
  };

  const onSelectPagador = (g: PagadorGroup) => {
    if (g.id) {
      setFilters((f) => ({ ...f, counterparty: { id: g.id as string, kind: g.kind as never } }));
      updateUrl({ counterparty_id: g.id, counterparty_kind: g.kind });
    } else {
      setFilters((f) => ({ ...f, search: g.label }));
      updateUrl({ q: g.label });
    }
    onTabChange("all");
  };

  const selectedContractInfo = useMemo(() => {
    if (!selectedLancamento) return undefined;
    const contractId = selectedLancamento.origin.entity_kind === "contract" ? selectedLancamento.origin.entity_id : undefined;
    if (!contractId) return undefined;
    const contract = contracts.find((c) => c.id === contractId);
    const property = properties.find((p) => p.id === contract?.property_id);
    return {
      id: contract?.id,
      property_title: property?.title,
      client: contract?.client,
    };
  }, [selectedLancamento, contracts, properties]);

  const selectedChargeLabels = useMemo(() => {
    if (!selectedLancamento || selectedLancamento.kind !== "charge") return {};
    const charge = selectedLancamento.source as Charge;
    const contract = contracts.find((c) => c.id === charge.contract_id);
    const property = properties.find((p) => p.id === charge.property_id);
    return {
      tenantLabel: contract?.client,
      propertyLabel: property?.title,
      ownerLabel: leads.find((lead) => lead.id === charge.owner_id)?.name || charge.owner_id,
    };
  }, [selectedLancamento, contracts, properties, leads]);

  const selectedCommissionLabels = useMemo(() => {
    if (!selectedLancamento || selectedLancamento.kind !== "commission") return {};
    const commission = selectedLancamento.source as Commission;
    const contract = contracts.find((item) => item.id === commission.contract_id || item.deal_id === commission.deal_id);
    const property = properties.find((item) => item.id === contract?.property_id);
    const contractReference = contract?.display_id || (contract?.id ? `#${contract.id.slice(0, 8)}` : undefined);
    return {
      brokerLabel: selectedLancamento.counterparty.label,
      contractLabel: [property?.title || contract?.property_title, contractReference ? `Contrato ${contractReference}` : undefined].filter(Boolean).join(" · "),
      rentalOrigin: isRentalContractType(contract?.type),
    };
  }, [selectedLancamento, contracts, properties]);

  const onChargeAction = async (action: any, charge: Charge) => {
    if (action === "edit") {
      setEditingCharge(charge);
      return;
    }
    if (action === "pay") {
      setMarkPaidCharge(charge);
      return;
    }
    if (action === "cancel") {
      setCancelCharge(charge);
      return;
    }
    if (action === "notify") {
      toast.info("Notificação enviada.");
      return;
    }
    if (action === "receipt") {
      toast.info("Abrindo recibo...");
      return;
    }
  };

  const onCommissionAction = async (action: CommissionDrawerAction, commission: Commission) => {
    if (commission.payout_order_id) {
      toast.info("Esta comissão é gerenciada pela ordem de pagamento vinculada.");
      setEditingCommission(null);
      closeDrawer();
      navigate("/financeiro/payouts");
      return;
    }
    if (action === "pay") {
      setEditingCommission(null);
      setMarkPaidCommissionMode("mark-paid");
      setMarkPaidCommission(commission);
      return;
    }
    if (action === "attach-proof") {
      setEditingCommission(null);
      setMarkPaidCommissionMode("attach-proof");
      setMarkPaidCommission(commission);
      return;
    }
    if (action === "approve") {
      try {
        await commissionService.approve(commission.id);
        toast.success("Comissão aprovada.");
        refetchAll();
        closeDrawer();
      } catch (err) {
        console.error(err);
        // A13: a recusa de segregação de funções vem do servidor com a causa
        // (quem pediu não aprova). Sem a mensagem, o operador só via "não foi
        // possível" e não sabia que precisava de outra pessoa.
        toast.error(
          (err as { response?: { data?: { message?: string } } })?.response?.data?.message ||
            "Não foi possível aprovar a comissão."
        );
      }
      return;
    }
    if (action === "cancel") {
      setEditingCommission(null);
      setCancelCommission(commission);
      return;
    }
    if (action === "reverse") {
      setReverseCommission(commission);
      return;
    }
  };

  const handleReverseCommission = async (reason: string) => {
    if (!reverseCommission) return;
    try {
      await commissionService.reverse(reverseCommission.id, reason);
      toast.success("Comissão estornada. Recrie a composição com os valores corrigidos.");
      setReverseCommission(null);
      setEditingCommission(null);
      refetchAll();
    } catch (err) {
      console.error(err);
      toast.error("Não foi possível estornar a comissão.");
    }
  };

  const handleCancelCommission = async (reason: string) => {
    if (!cancelCommission) return;
    try {
      await commissionService.cancel(cancelCommission.id, reason);
      toast.success("Comissão cancelada.");
      setCancelCommission(null);
      setEditingCommission(null);
      refetchAll();
    } catch (err) {
      console.error(err);
      toast.error("Não foi possível cancelar a comissão.");
    }
  };

  const handleCreateNovoKind = (kind: NovoLancamentoKind) => {
    setShowNovoLancamento(false);
    if (kind === "entrada" || kind === "saida") {
      setNovaTransacaoDir(kind);
      return;
    }
    setNovoKind(kind);
  };

  const handleCreateCharge = async (data: any) => {
    try {
      await chargeService.create(data);
      toast.success("Cobrança criada.");
      setNovoKind(null);
      refetchAll();
    } catch (err) {
      console.error(err);
      toast.error("Não foi possível criar a cobrança.");
    }
  };

  const handleMarkPaidCharge = async (chargeId: string, payload: any) => {
    try {
      await chargeService.markPaid(chargeId, payload);
      toast.success("Cobrança marcada como paga.");
      setMarkPaidCharge(null);
      refetchAll();
    } catch (err) {
      console.error(err);
      toast.error("Não foi possível marcar como paga.");
    }
  };

  const handleCancelCharge = async (reason: string) => {
    if (!cancelCharge) return;
    try {
      await chargeService.cancel(cancelCharge.id, reason);
      toast.success("Cobrança cancelada.");
      setCancelCharge(null);
      refetchAll();
    } catch (err) {
      console.error(err);
      toast.error("Não foi possível cancelar a cobrança.");
    }
  };

  const handleMarkPaidCommission = async (commissionId: string, payload: any) => {
    try {
      await commissionService.pay(commissionId, payload);
      toast.success(markPaidCommissionMode === "attach-proof" ? "Comprovante anexado à comissão." : "Comissão marcada como paga.");
      setMarkPaidCommission(null);
      refetchAll();
    } catch (err) {
      console.error(err);
      throw err;
    }
  };

  const headerRight = (
    <div className="flex items-center gap-2">
      {partialFailures.length > 0 && (
        <button type="button" onClick={refetchAll} className="inline-flex items-center gap-2 px-3 py-2 text-xs font-bold rounded-xl border border-status-warning/40 bg-status-warning-soft text-status-warning">
          <AlertOctagon className="w-4 h-4" />
          Falha parcial ({partialFailures.length}) — tentar novamente
        </button>
      )}
      {canWrite && (
        <Button onClick={() => setShowNovoLancamento(true)}>
          <Plus className="w-4 h-4" /> Novo
        </Button>
      )}
    </div>
  );

  const visible = useMemo(() => {
    if (activeTab === "all" || activeTab === "ledger" || activeTab === "pagadores") return lancamentos;
    return grouped[activeTab as LancamentoKind];
  }, [activeTab, grouped, lancamentos]);

  const extensionColumn = useMemo(() => {
    if (activeTab !== "charge") return undefined;
    return {
      header: "Inquilino",
      render: (l: Lancamento) => {
        if (l.kind !== "charge") return null;
        const charge = l.source as Charge;
        const tenantPartyId = charge.tenant_party_id;
        if (!tenantPartyId) return <span className="text-muted-foreground">—</span>;
        const contract = contracts.find((c) => c.id === charge.contract_id);
        return <span className="text-sm text-foreground">{contract?.client || tenantPartyId}</span>;
      },
    };
  }, [activeTab, contracts]);

  const tableSelect = (l: Lancamento) => {
    if (l.kind === "charge") openChargeDetail(l);
    else if (l.kind === "commission") openCommissionDetail(l);
    else if (l.kind === "transfer") openTransferDetail(l);
    else if (l.kind === "royalty") openRoyaltyDetail(l);
    else openGenericDrawer(l);
  };

  const selectedTransferPresentation = useMemo(() => {
    if (!selectedLancamento || selectedLancamento.kind !== "transfer") return null;
    return getOwnerTransferPresentation(selectedLancamento.source as OwnerTransfer);
  }, [selectedLancamento]);

  if (partialFailures.length > 0) return <div className="p-6 space-y-4"><FinanceAreaNav /><FinanceLoadError message="Operações incompletas: não foi possível carregar todas as origens. Os totais estão indisponíveis." onRetry={refetchAll} /></div>;
  if (isLoading) return <div className="p-6 space-y-4"><FinanceAreaNav /><p role="status">Carregando todas as operações…</p></div>;

  return (
    // A carteira é lida em planilha: a página usa a largura disponível em vez
    // de prender a tabela em 1280px.
    <div className="mx-auto max-w-[1600px] space-y-4 p-6">
      <FinanceAreaNav />
      <div className="flex items-start justify-between gap-4 flex-wrap">
        <div>
          <h1 className="text-2xl font-bold text-foreground">Lançamentos</h1>
          <p className="text-sm text-muted-foreground">
            Eventos econômicos, obrigações e caixa em uma trilha auditável · {operations.length} {operations.length === 1 ? "operação" : "operações"}
          </p>
        </div>
        {headerRight}
      </div>

      <LancamentosKpis lancamentos={allLancamentos} onSelect={handleKpiSelect} />

      <Tabs value={activeTab} onValueChange={(v) => onTabChange(parseKindTab(v))} className="space-y-3">
        <div className="flex items-center justify-between gap-3 flex-wrap">
          <div className="max-w-full overflow-x-auto pb-1">
            <TabsList>
              <TabsTrigger value="all">Operações ({operations.length})</TabsTrigger>
              <TabsTrigger value="mensal">Mensal</TabsTrigger>
              <TabsTrigger value="ledger">Livro ({allCount})</TabsTrigger>
              <TabsTrigger value="royalty">Parcerias ({grouped.royalty.length})</TabsTrigger>
              <TabsTrigger value="transaction">Caixa ({grouped.transaction.length})</TabsTrigger>
              <TabsTrigger value="pagadores">Partes</TabsTrigger>
            </TabsList>
          </div>
          <div className="text-xs text-muted-foreground font-bold">{activeTab === "all" ? "Visão por evento" : kindLabel(activeTab)}</div>
        </div>

        <div className="flex flex-wrap items-center gap-2 text-xs">
          <span className="font-bold text-muted-foreground">Ver em:</span>
          <button type="button" className="font-bold text-primary-600 hover:underline" onClick={() => navigate("/financeiro/cobrancas")}>
            Recebimentos
          </button>
          <span className="text-muted-foreground">·</span>
          <button type="button" className="font-bold text-primary-600 hover:underline" onClick={() => navigate("/financeiro/comissoes")}>
            Comissões
          </button>
          <span className="text-muted-foreground">·</span>
          <button type="button" className="font-bold text-primary-600 hover:underline" onClick={() => navigate("/financeiro/repasses")}>
            Repasses
          </button>
        </div>

        <LancamentosFilters value={filters} onChange={handleFiltersChange} />

        <TabsContent value={activeTab}>
          {activeTab === "pagadores" ? (
            <PagadoresView lancamentos={lancamentos} onSelect={onSelectPagador} />
          ) : activeTab === "mensal" ? (
            <div className="space-y-3">
              <MonthlySummaryView buckets={monthly} selectedMonth={selectedMonth} onSelectMonth={setSelectedMonth} />
              {selectedMonth && (
                <LancamentosTable
                  lancamentos={monthlyItems}
                  isLoading={isLoading}
                  onSelect={tableSelect}
                  extensionColumn={extensionColumn}
                />
              )}
            </div>
          ) : activeTab === "all" ? (
            <OperationsExplorer
              operations={operations}
              facetSource={operationFacets}
              filters={filters}
              onFiltersChange={handleFiltersChange}
              view={operationView}
              onViewChange={(next) => {
                setOperationView(next);
                // A planilha é o padrão: sai da URL para o link limpo do menu
                // cair nela; só o modo cartão precisa ser explícito.
                updateUrl({ view: next === "sheet" ? null : next });
              }}
              groupBy={operationGroup}
              onGroupChange={(next) => {
                setOperationGroup(next);
                updateUrl({ group: next === "competence" ? null : next });
              }}
              sort={operationSort}
              onSortChange={(next) => {
                setOperationSort(next);
                updateUrl({ sort: next === "focus" ? null : next });
              }}
              isLoading={isLoading}
              onSelect={tableSelect}
              onOpenRentalPlan={(contractId) => navigate(`/locacao/${contractId}`)}
            />
          ) : (
            <LancamentosTable
              lancamentos={visible}
              isLoading={isLoading}
              onSelect={tableSelect}
              extensionColumn={extensionColumn}
            />
          )}
        </TabsContent>
      </Tabs>

      {drawerOpen && selectedLancamento && (drawerOpen === "transfer" || drawerOpen === "royalty" || drawerOpen === "transaction") && (
        <LancamentoSimpleDrawer
          lancamento={selectedLancamento}
          onClose={closeDrawer}
          onNavigate={navigateToSpecific}
          divergenceActionLabel={
            drawerOpen === "royalty" ? "Abrir ordem de pagamento" : drawerOpen === "transfer" ? "Abrir repasses" : undefined
          }
          operationalNotice={
            drawerOpen === "transfer" && selectedTransferPresentation
              ? selectedTransferPresentation.action === "settle"
                ? "Sem saldo a repassar nesta competência: conclua o repasse para encerrá-lo. Nenhuma movimentação de caixa será registrada."
                : !selectedTransferPresentation.settlementReady
                  ? "A apurar: este registro não possui origem econômica e saldo do proprietário conciliados. Revise-o em Repasses antes de qualquer movimentação."
                  : undefined
              : undefined
          }
          onResolveDivergence={(lancamento) => {
            closeDrawer();
            navigate(lancamento.kind === "royalty" ? "/financeiro/payouts" : "/financeiro/repasses");
          }}
          actions={{
            canApprove: drawerOpen === "transfer" && selectedTransferPresentation?.action === "approve" && canApproveTransfer,
            canProcess: drawerOpen === "transfer" && selectedTransferPresentation?.action === "process" && canApproveTransfer,
            canSettleWithoutBalance: drawerOpen === "transfer" && selectedTransferPresentation?.action === "settle" && canApproveTransfer,
            canSettle: drawerOpen === "transaction" && (selectedLancamento.status === "pendente" || selectedLancamento.status === "vencido") && canWrite,
            canCancel: drawerOpen === "transaction" && (selectedLancamento.status === "pendente" || selectedLancamento.status === "vencido") && canWrite,
            canReverse: drawerOpen === "transaction" && selectedLancamento.status === "pago" && canWrite,
            approving: transferActionKey === `approve:${selectedLancamento.id}`,
            processing: transferActionKey === `process:${selectedLancamento.id}`,
            settling: settlingTransactionId === selectedLancamento.id,
            settlingWithoutBalance: transferActionKey === `settle:${selectedLancamento.id}`,
            onApprove: () => setPendingLancAction({ action: "approve", lancamento: selectedLancamento }),
            onProcess: () => setProcessingTransfer(selectedLancamento.source as OwnerTransfer),
            onSettleWithoutBalance: () => setPendingLancAction({ action: "settle", lancamento: selectedLancamento }),
            // Baixa de lançamento é formulário (método + data + comprovante):
            // abre direto o painel lateral padrão, sem confirmação intermediária.
            onSettle: () => void handleSettleTransaction(selectedLancamento),
            onCancel: () => setPendingLancAction({ action: "cancel", lancamento: selectedLancamento }),
            onReverse: () => setPendingLancAction({ action: "reverse", lancamento: selectedLancamento }),
          }}
        />
      )}

      {editingCharge && (
        <ChargeDetailDrawer
          charge={editingCharge}
          contract={selectedContractInfo}
          ownerLabel={selectedChargeLabels.ownerLabel}
          tenantLabel={selectedChargeLabels.tenantLabel}
          propertyLabel={selectedChargeLabels.propertyLabel}
          onClose={() => setEditingCharge(null)}
          onAction={onChargeAction}
          onResolveDivergence={() => {
            setEditingCharge(null);
            navigate("/financeiro?module=conciliacao");
          }}
        />
      )}

      {editingCommission && (
        <CommissionDetailDrawer
          commission={editingCommission}
          brokerLabel={selectedCommissionLabels.brokerLabel || editingCommission.broker_id}
          contractLabel={selectedCommissionLabels.contractLabel || editingCommission.contract_id}
          rentalOrigin={selectedCommissionLabels.rentalOrigin}
          onClose={() => setEditingCommission(null)}
          onAction={onCommissionAction}
          onCommissionChange={(updated) => setEditingCommission(updated)}
        />
      )}

      {showNovoLancamento && <NovoLancamentoModal onClose={() => setShowNovoLancamento(false)} onSelect={handleCreateNovoKind} />}

      {novoKind === "charge" && (
        <NovaCobrancaModal
          onClose={() => setNovoKind(null)}
          onSave={handleCreateCharge}
          contracts={contracts}
          properties={properties}
        />
      )}

      {novoKind === "commission" && <NovaComissaoModal onClose={() => setNovoKind(null)} onCreated={refetchAll} />}

      {novaTransacaoDir && (
        <NovaTransacaoModal
          direction={novaTransacaoDir}
          onClose={() => setNovaTransacaoDir(null)}
          onCreated={refetchAll}
          contracts={contracts}
          properties={properties}
          leads={leads}
          agents={agents}
        />
      )}

      {processingTransfer && (
        <ProcessRepasseModal
          transferId={processingTransfer.id}
          transfer={processingTransfer}
          onClose={() => setProcessingTransfer(null)}
          onProcess={handleProcessTransfer}
        />
      )}

      {markPaidCharge && (
        <MarkPaidModal
          chargeId={markPaidCharge.id}
          defaultAmount={markPaidCharge.total_amount || markPaidCharge.amount}
          charge={markPaidCharge}
          onClose={() => setMarkPaidCharge(null)}
          onSave={handleMarkPaidCharge}
        />
      )}

      {markPaidCommission && (
        <MarkPaidCommissionModal
          mode={markPaidCommissionMode}
          commissionId={markPaidCommission.id}
          brokerLabel={agents.find((agent) => agent.id === markPaidCommission.broker_id)?.name || markPaidCommission.broker_id}
          netAmount={markPaidCommission.net_amount}
          onClose={() => setMarkPaidCommission(null)}
          onSave={handleMarkPaidCommission}
        />
      )}

      <CancelarCobrancaDialog
        open={!!cancelCharge}
        contextLine={cancelCharge ? `${cancelCharge.description || "Cobrança"} · ${formatCurrencyBRL0(cancelCharge.total_amount || cancelCharge.amount)} · ${mapChargeStatus(cancelCharge.status)}` : undefined}
        onCancel={() => setCancelCharge(null)}
        onConfirm={handleCancelCharge}
      />

      <CancelarCobrancaDialog
        open={!!reverseCommission}
        title="Estornar comissão"
        description="O estorno preserva a comissão original no histórico e reverte o lançamento de caixa vinculado. Depois, recrie a comissão com a composição correta."
        contextLine={
          reverseCommission
            ? `${reverseCommission.role || "Comissão"} · ${formatCurrencyBRL0(reverseCommission.net_amount ?? reverseCommission.amount)}`
            : undefined
        }
        confirmLabel="Confirmar estorno"
        loadingText="Estornando..."
        onCancel={() => setReverseCommission(null)}
        onConfirm={handleReverseCommission}
      />

      {pendingLancAction && (
        <CancelarCobrancaDialog
          open
          title={LANC_ACTION_COPY[pendingLancAction.action].title}
          description={LANC_ACTION_COPY[pendingLancAction.action].description}
          contextLine={settlementContextLabel(pendingLancAction.lancamento)}
          variant="default"
          wide
          requireReason={false}
          confirmLabel={LANC_ACTION_COPY[pendingLancAction.action].confirmLabel}
          loadingText={LANC_ACTION_COPY[pendingLancAction.action].loadingText}
          effects={
            pendingLancAction.action === "approve" ? (
              <SettlementEffects data={deriveSettlementEffects(pendingLancAction.action, pendingLancAction.lancamento)} />
            ) : undefined
          }
          onCancel={() => setPendingLancAction(null)}
          onConfirm={async () => {
            const target = pendingLancAction;
            setPendingLancAction(null);
            if (target.action === "cancel") await handleCancelTransaction(target.lancamento);
            else if (target.action === "reverse") await handleReverseTransaction(target.lancamento);
            else if (target.action === "settle") await handleSettleTransferWithoutBalance(target.lancamento);
            else await handleApproveTransfer(target.lancamento);
          }}
        />
      )}

      <CancelarCobrancaDialog
        open={!!cancelCommission}
        title="Cancelar Comissão"
        description="A comissão será marcada como CANCELADA. O motivo entra no audit trail. Nenhum valor foi movimentado enquanto ela estava sem pagamento."
        contextLine={
          cancelCommission
            ? `${cancelCommission.role || "Comissão"} · ${formatCurrencyBRL0(cancelCommission.net_amount ?? cancelCommission.amount)}`
            : undefined
        }
        confirmLabel="Cancelar Comissão"
        loadingText="Cancelando..."
        onCancel={() => setCancelCommission(null)}
        onConfirm={handleCancelCommission}
      />

      {settleTarget && (
        <SettleLancamentoModal
          lancamento={settleTarget}
          onClose={() => setSettleTarget(null)}
          onSaved={() => {
            setSettleTarget(null);
            closeDrawer();
            refetchAll();
          }}
        />
      )}
    </div>
  );
}
