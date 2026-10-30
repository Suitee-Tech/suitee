import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import {
  Ban,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  CircleDollarSign,
  Eye,
  FileCheck2,
  Loader2,
  ReceiptText,
  RefreshCw,
  RotateCcw,
  Scale, Upload,
} from "lucide-react";

import {
  managedBillService,
  type ManagedBill,
  type ManagedBillMutationResult,
  type ManagedBillOwnerBalance,
  type ManagedBillStatus,
} from "../../services/managedBill.service";
import { usePermissionsContext } from "../../context/PermissionsContext";
import { useOptionalAuth } from "../../context/AuthContext";
import { EmptyState } from "../../ui/EmptyState";
import { Button } from "../../ui/button";
import { proofService } from "../../services/proof.service";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "../../ui/dialog";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "../../ui/sheet";
import { FinancialPartyTimeline } from "./FinancialPartyTimeline";

export type ManagedBillAction = "approve" | "pay" | "reconcile" | "reverse" | "cancel";

export function getManagedBillActions(status: ManagedBillStatus): ManagedBillAction[] {
  switch (status) {
    case "RESERVED":
      return ["approve", "cancel"];
    case "APPROVED":
      return ["pay", "cancel"];
    case "PAID":
      return ["reconcile", "reverse"];
    case "RECONCILED":
      return ["reverse"];
    case "CANCELLED":
      return [];
  }
}

const STATUS_PRESENTATION: Record<ManagedBillStatus, { label: string; className: string }> = {
  RESERVED: {
    label: "Reservada",
    className: "border-status-warning/30 bg-status-warning-soft text-status-warning",
  },
  APPROVED: {
    label: "Aprovada",
    className: "border-primary-200 bg-primary-50 text-primary-700",
  },
  PAID: {
    label: "Paga — aguardando conciliação",
    className: "border-status-info/30 bg-status-info-soft text-status-info",
  },
  RECONCILED: {
    label: "Conciliada",
    className: "border-status-ok/30 bg-status-ok-soft text-status-ok",
  },
  CANCELLED: {
    label: "Cancelada",
    className: "border-border bg-muted text-muted-foreground",
  },
};

const ACTION_PRESENTATION: Record<
  ManagedBillAction,
  { label: string; title: string; description: string; submitLabel: string }
> = {
  approve: {
    label: "Aprovar",
    title: "Aprovar conta administrada",
    description: "Confirme a autorização desta obrigação. A versão atual será validada pelo servidor.",
    submitLabel: "Confirmar aprovação",
  },
  pay: {
    label: "Registrar pagamento",
    title: "Registrar pagamento ao credor",
    description: "Informe a referência da execução. Registrar pagamento não concilia a conta automaticamente.",
    submitLabel: "Registrar pagamento",
  },
  reconcile: {
    label: "Conciliar",
    title: "Conciliar conta administrada",
    description: "Informe a referência que confirma a baixa junto ao credor ou extrato.",
    submitLabel: "Confirmar conciliação",
  },
  reverse: {
    label: "Estornar pagamento",
    title: "Estornar pagamento da conta",
    description: "Esta ação exige permissão elevada, cria lançamentos compensatórios e devolve a conta para APROVADA, permitindo um novo pagamento.",
    submitLabel: "Confirmar estorno",
  },
  cancel: {
    label: "Cancelar",
    title: "Cancelar conta administrada",
    description: "O motivo ficará vinculado ao comando auditável. Contas pagas não podem ser canceladas aqui.",
    submitLabel: "Confirmar cancelamento",
  },
};

export interface ManagedBillsPanelProps {
  contractId?: string | null;
  search?: string;
  propertyLabels?: Record<string, string>;
}

type OpenAction = { action: ManagedBillAction; bill: ManagedBill };

const MANAGED_BILLS_PAGE_SIZE = 25;

type ManagedBillsPagination = {
  page: number;
  total: number;
  totalPages: number;
};

const formatCents = (cents: number): string =>
  new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(cents / 100);

const formatDate = (value: string): string => {
  if (!value) return "—";
  const dateOnly = value.slice(0, 10);
  const date = new Date(`${dateOnly}T12:00:00`);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString("pt-BR");
};

const formatCompetence = (value: string): string => {
  const match = /^(\d{4})-(\d{2})/.exec(value);
  return match ? `${match[2]}/${match[1]}` : value || "—";
};

const formatDateTime = (value: string): string => {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("pt-BR");
};

const availableCents = (balance: ManagedBillOwnerBalance): number =>
  balance.available_cents;

/**
 * Conflito de escrita devolvido pelo backend com o motivo discriminado:
 * `stale_version` (o registro mudou) ou `already_paid` (o pagamento já existe).
 * Distinguir importa porque a recuperação é diferente — recarregar resolve o
 * primeiro, e só o estorno resolve o segundo.
 */
const conflictReason = (error: unknown): string | null => {
  const candidate = error as { response?: { data?: { data?: { reason?: string } } } };
  return candidate.response?.data?.data?.reason ?? null;
};

const errorMessage = (error: unknown): string => {
  const candidate = error as { response?: { data?: { message?: string } }; message?: string };
  return candidate.response?.data?.message || candidate.message || "Não foi possível concluir a ação.";
};

export function ManagedBillsPanel({ contractId, search = "", propertyLabels = {} }: ManagedBillsPanelProps) {
  const { can } = usePermissionsContext();
  const canApprove = can("finance:approve_transfer");
  const canWrite = can("finance:write");
  const currentUserId = useOptionalAuth()?.user?.id ?? null;
  const [bills, setBills] = useState<ManagedBill[]>([]);
  const [page, setPage] = useState(1);
  const [pagination, setPagination] = useState<ManagedBillsPagination>({
    page: 1,
    total: 0,
    totalPages: 0,
  });
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [openAction, setOpenAction] = useState<OpenAction | null>(null);
  const [selectedBill, setSelectedBill] = useState<ManagedBill | null>(null);
  const [pendingBillId, setPendingBillId] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [paymentReference, setPaymentReference] = useState("");
  const [proofUrl, setProofUrl] = useState("");
  const [proofFile, setProofFile] = useState<File | null>(null);
  const [reconciliationReference, setReconciliationReference] = useState("");
  const [reversalReference, setReversalReference] = useState("");
  const [reversalReason, setReversalReason] = useState("");
  const [cancelReason, setCancelReason] = useState("");
  const [lastMutation, setLastMutation] = useState<ManagedBillMutationResult | null>(null);
  const requestSequence = useRef(0);
  const normalizedSearch = search.trim();

  useEffect(() => {
    setPage(1);
  }, [contractId, normalizedSearch]);

  const loadBills = useCallback(async () => {
    const sequence = ++requestSequence.current;
    setLoading(true);
    setLoadError(null);
    try {
      const result = await managedBillService.list({
        contract_id: contractId || undefined,
        search: normalizedSearch || undefined,
        limit: MANAGED_BILLS_PAGE_SIZE,
        offset: (page - 1) * MANAGED_BILLS_PAGE_SIZE,
      });
      if (sequence === requestSequence.current) {
        const total = Math.max(0, result.meta?.total ?? result.data.length);
        const totalPages = Math.max(
          0,
          result.meta?.totalPages ?? (total > 0 ? Math.ceil(total / MANAGED_BILLS_PAGE_SIZE) : 0),
        );
        setBills(result.data);
        setPagination({
          page: Math.max(1, result.meta?.page ?? page),
          total,
          totalPages,
        });
      }
    } catch (error) {
      if (sequence === requestSequence.current) {
        setBills([]);
        setPagination({ page, total: 0, totalPages: 0 });
        setLoadError(errorMessage(error));
      }
    } finally {
      if (sequence === requestSequence.current) setLoading(false);
    }
  }, [contractId, normalizedSearch, page]);

  useEffect(() => {
    void loadBills();
    return () => {
      requestSequence.current += 1;
    };
  }, [loadBills]);

  // Segregação de funções: quem aprova não paga — EXCETO quando responde pela
  // operação financeira da imobiliária. O administrador precisa concluir os dois
  // passos quando é o único operador, e é o backend quem decide isso pelo papel
  // autenticado; aqui a interface só evita oferecer uma ação que seria recusada.
  //
  // O critério é a mesma permissão de aprovação que já governa a autorização das
  // rotas: quem pode aprovar tem autoridade financeira, quem não pode continua
  // sujeito à regra de dois operadores.
  const isOwnApproval = useCallback(
    (bill: ManagedBill) => Boolean(currentUserId && bill.approved_by && bill.approved_by === currentUserId),
    [currentUserId],
  );

  const permittedActions = useCallback(
    (bill: ManagedBill) =>
      getManagedBillActions(bill.status).filter((action) => {
        const allowed = action === "approve" || action === "reverse" ? canApprove : canWrite;
        if (!allowed) return false;
        // Só bloqueia quem NÃO tem autoridade para aprovar: para esse, a
        // segregação continua valendo e o backend recusaria com 409.
        if (action === "pay" && isOwnApproval(bill) && !canApprove) return false;
        return true;
      }),
    [canApprove, canWrite, isOwnApproval],
  );

  const beginAction = (action: ManagedBillAction, bill: ManagedBill) => {
    setOpenAction({ action, bill });
    setActionError(null);
    setPaymentReference("");
    setProofUrl("");
    setReconciliationReference("");
    setReversalReference("");
    setReversalReason("");
    setCancelReason("");
  };

  const closeAction = () => {
    if (pendingBillId) return;
    setOpenAction(null);
    setActionError(null);
  };

  const submitAction = async () => {
    if (!openAction) return;

    const { action, bill } = openAction;
    const expectedVersion = bill.version;
    const cleanPaymentReference = paymentReference.trim();
    const cleanProofUrl = proofUrl.trim();
    const cleanReconciliationReference = reconciliationReference.trim();
    const cleanReversalReference = reversalReference.trim();
    const cleanReversalReason = reversalReason.trim();
    const cleanCancelReason = cancelReason.trim();

    const requiresHighTrust = action === "approve" || action === "reverse";
    if ((requiresHighTrust && !canApprove) || (!requiresHighTrust && !canWrite)) {
      setActionError("Você não tem permissão para executar esta ação.");
      return;
    }

    if (action === "pay" && !cleanPaymentReference) {
      setActionError("Informe a referência do pagamento.");
      return;
    }
    if (action === "reconcile" && !cleanReconciliationReference) {
      setActionError("Informe a referência da conciliação.");
      return;
    }
    if (action === "reverse" && !cleanReversalReference) {
      setActionError("Informe a referência do estorno.");
      return;
    }
    if (action === "reverse" && !cleanReversalReason) {
      setActionError("Informe o motivo do estorno.");
      return;
    }
    if (action === "cancel" && !cleanCancelReason) {
      setActionError("Informe o motivo do cancelamento.");
      return;
    }

    setPendingBillId(bill.id);
    setActionError(null);
    try {
      let result: ManagedBillMutationResult;
      if (action === "approve") {
        result = await managedBillService.approve(bill.id, { expected_version: expectedVersion });
      } else if (action === "pay") {
        let proofUrlFinal = cleanProofUrl || undefined;
        if (proofFile) {
          const uploaded = await proofService.upload(proofFile, "payout");
          proofUrlFinal = uploaded.url;
          setProofFile(null);
        }
        result = await managedBillService.pay(bill.id, {
          expected_version: expectedVersion,
          payment_reference: cleanPaymentReference,
          proof_url: proofUrlFinal,
        });
      } else if (action === "reconcile") {
        result = await managedBillService.reconcile(bill.id, {
          expected_version: expectedVersion,
          reconciliation_reference: cleanReconciliationReference,
        });
      } else if (action === "reverse") {
        let proofUrlFinal = cleanProofUrl || undefined;
        if (proofFile) {
          const uploaded = await proofService.upload(proofFile, "payout");
          proofUrlFinal = uploaded.url;
          setProofFile(null);
        }
        result = await managedBillService.reversePayment(bill.id, {
          expected_version: expectedVersion,
          reversal_reference: cleanReversalReference,
          reason: cleanReversalReason,
          proof_url: proofUrlFinal,
        });
      } else {
        result = await managedBillService.cancel(bill.id, {
          expected_version: expectedVersion,
          reason: cleanCancelReason,
        });
      }

      setBills((current) => current.map((item) => (item.id === result.bill.id ? result.bill : item)));
      setLastMutation(result);
      setOpenAction(null);
    } catch (error) {
      // Conflito de versão: o estado em mãos está defasado. Recarregar a lista
      // é o que devolve a operação ao trilho — sem isso o operador repetiria a
      // mesma tentativa com a mesma versão e receberia o mesmo 409.
      if (conflictReason(error)) {
        await loadBills();
      }
      setActionError(errorMessage(error));
    } finally {
      setPendingBillId(null);
    }
  };

  return (
    <section className="overflow-hidden rounded-2xl border border-border bg-card shadow-sm" aria-labelledby="managed-bills-title">
      <div className="flex flex-col gap-3 border-b border-border bg-muted/20 p-5 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2">
            <ReceiptText className="h-5 w-5 text-primary-600" />
            <h3 id="managed-bills-title" className="text-lg font-bold text-foreground">Contas administradas</h3>
            {!loading && (
              <span className="rounded-full border border-border bg-muted px-2 py-0.5 text-xs font-bold text-muted-foreground">
                {pagination.total}
              </span>
            )}
          </div>
          <p className="mt-1 max-w-3xl text-sm text-muted-foreground">
            Obrigações efetivas pagas pela imobiliária, separadas das configurações de contas de consumo. Cada ação registra ator e referência ou motivo; o comprovante de pagamento é opcional.
          </p>
          <p className="mt-1 flex items-center gap-1 text-xs text-muted-foreground">
            <Scale className="h-3.5 w-3.5" /> A composição econômica exibida abaixo não representa saldo bancário.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void loadBills()} disabled={loading}>
          {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
          Atualizar
        </Button>
      </div>

      {lastMutation && <BalanceImpact result={lastMutation} />}

      {loadError && (
        <div className="m-5 rounded-xl border border-status-critical/30 bg-status-critical-soft p-4 text-sm text-status-critical" role="alert">
          {loadError}
        </div>
      )}

      {loading ? (
        <div className="flex items-center justify-center gap-2 p-12 text-sm text-muted-foreground">
          <Loader2 className="h-5 w-5 animate-spin" /> Carregando contas administradas…
        </div>
      ) : bills.length === 0 && !loadError ? (
        <EmptyState
          icon={ReceiptText}
          title="Nenhuma conta administrada"
          message={search ? "Nenhuma obrigação corresponde à busca atual." : "As obrigações reservadas para pagamento aparecerão aqui."}
          className="border-none bg-transparent my-6"
        />
      ) : !loadError ? (
        <div>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[920px] text-left text-sm">
            <thead className="border-b border-border bg-muted/30 text-xs text-muted-foreground">
              <tr>
                <th className="px-5 py-3 font-bold">Conta / credor</th>
                <th className="px-5 py-3 font-bold">Imóvel / competência</th>
                <th className="px-5 py-3 font-bold">Vencimento</th>
                <th className="px-5 py-3 text-right font-bold">Valor</th>
                <th className="px-5 py-3 font-bold">Status</th>
                <th className="px-5 py-3 text-right font-bold">Ações</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {bills.map((bill) => {
                const status = STATUS_PRESENTATION[bill.status];
                const lifecycleActions = getManagedBillActions(bill.status);
                const actions = permittedActions(bill);
                const blockedBySegregation = isOwnApproval(bill) && !canApprove && lifecycleActions.includes("pay");
                const isPending = pendingBillId === bill.id;
                return (
                  <tr key={bill.id} className="align-top hover:bg-muted/20">
                    <td className="px-5 py-4">
                      <p className="font-bold text-foreground">{bill.component || "Conta administrada"}</p>
                      <p className="text-xs text-muted-foreground">{bill.creditor_name || "Credor não informado"}</p>
                      {bill.description && <p className="mt-1 max-w-xs text-xs text-muted-foreground">{bill.description}</p>}
                      <ManagedBillAuditDetails bill={bill} />
                    </td>
                    <td className="px-5 py-4">
                      <p className="font-medium text-foreground">{propertyLabels[bill.property_id] ?? bill.property_id}</p>
                      <p className="text-xs text-muted-foreground">Competência {formatCompetence(bill.competence_month)}</p>
                    </td>
                    <td className="px-5 py-4 text-muted-foreground">{formatDate(bill.due_date)}</td>
                    <td className="px-5 py-4 text-right font-bold text-foreground">{formatCents(bill.amount_cents)}</td>
                    <td className="px-5 py-4">
                      <span className={`inline-flex rounded-lg border px-2.5 py-1 text-xs font-bold ${status.className}`}>
                        {status.label}
                      </span>
                    </td>
                    <td className="px-5 py-4">
                      <div className="flex justify-end gap-2">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setSelectedBill(bill)}
                          aria-label={`Ver detalhes da conta de ${bill.creditor_name || bill.component}`}
                        >
                          <Eye className="h-3.5 w-3.5" /> Detalhes
                        </Button>
                        {actions.map((action) => (
                          <Button
                            key={action}
                            variant={action === "cancel" || action === "reverse" ? "ghost" : "outline"}
                            size="sm"
                            disabled={isPending}
                            onClick={() => beginAction(action, bill)}
                            className={action === "cancel" || action === "reverse" ? "text-status-critical hover:bg-status-critical-soft hover:text-status-critical" : undefined}
                          >
                            {isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : actionIcon(action)}
                            {ACTION_PRESENTATION[action].label}
                          </Button>
                        ))}
                        {actions.length === 0 && (
                          <span className="py-2 text-xs text-muted-foreground">
                            {blockedBySegregation
                              ? "Aprovada por você — o pagamento deve ser registrado por outro operador"
                              : lifecycleActions.length > 0
                                ? "Sem permissão para agir"
                                : "Sem ações pendentes"}
                          </span>
                        )}
                        {blockedBySegregation && actions.length > 0 && (
                          <span className="py-2 text-xs text-muted-foreground">
                            Pagamento por outro operador
                          </span>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
            </table>
          </div>

          <div className="flex flex-col gap-3 border-t border-border px-5 py-4 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
            <p>
              {pagination.total === 1
                ? "1 conta administrada"
                : `${pagination.total} contas administradas`}
            </p>
            {pagination.totalPages > 1 && (
              <div className="flex items-center gap-3" aria-label="Paginação de contas administradas">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPage((current) => Math.max(1, current - 1))}
                  disabled={loading || pagination.page <= 1}
                >
                  <ChevronLeft className="h-4 w-4" /> Anterior
                </Button>
                <span aria-live="polite">
                  Página {pagination.page} de {pagination.totalPages}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPage((current) => Math.min(pagination.totalPages, current + 1))}
                  disabled={loading || pagination.page >= pagination.totalPages}
                >
                  Próxima <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            )}
          </div>
        </div>
      ) : null}

      <ActionDialog
        openAction={openAction}
        pending={Boolean(openAction && pendingBillId === openAction.bill.id)}
        error={actionError}
        paymentReference={paymentReference}
        proofUrl={proofUrl}
        reconciliationReference={reconciliationReference}
        reversalReference={reversalReference}
        reversalReason={reversalReason}
        cancelReason={cancelReason}
        onPaymentReferenceChange={setPaymentReference}
        onProofUrlChange={setProofUrl}
        proofFile={proofFile}
        onProofFileChange={setProofFile}
        onReconciliationReferenceChange={setReconciliationReference}
        onReversalReferenceChange={setReversalReference}
        onReversalReasonChange={setReversalReason}
        onCancelReasonChange={setCancelReason}
        onClose={closeAction}
        onSubmit={submitAction}
      />

      <ManagedBillDetailDrawer
        bill={selectedBill}
        propertyLabel={selectedBill ? propertyLabels[selectedBill.property_id] : undefined}
        onClose={() => setSelectedBill(null)}
      />
    </section>
  );
}

function ManagedBillDetailDrawer({
  bill,
  propertyLabel,
  onClose,
}: {
  bill: ManagedBill | null;
  propertyLabel?: string;
  onClose: () => void;
}) {
  const status = bill ? STATUS_PRESENTATION[bill.status] : null;

  return (
    <Sheet open={Boolean(bill)} onOpenChange={(next) => { if (!next) onClose(); }}>
      <SheetContent side="right" expandable className="w-full gap-0 overflow-hidden p-0 sm:max-w-lg" data-testid="managed-bill-detail-drawer">
        {bill && status && (
          <>
            <div className="border-b border-border px-6 py-4 pr-20">
              <SheetTitle className="flex items-center gap-2 text-xl font-bold text-foreground">
                <ReceiptText className="h-5 w-5 text-primary-600" /> Detalhes da conta
              </SheetTitle>
              <SheetDescription className="mt-1 truncate font-mono text-xs text-muted-foreground">
                Código: {bill.id}
              </SheetDescription>
            </div>

            <div className="flex-1 space-y-6 overflow-y-auto p-6">
              <div className="flex justify-end">
                <span className={`inline-flex rounded-lg border px-2.5 py-1 text-xs font-bold ${status.className}`}>
                  {status.label}
                </span>
              </div>

              <FinancialPartyTimeline
                beneficiaryLabel={bill.creditor_name}
                beneficiaryKind={bill.component ? `Credor · ${bill.component}` : "Credor"}
                expectedDueAt={bill.due_date}
                settledAt={bill.paid_at}
              />

              <div className="rounded-xl border border-border bg-muted/20 p-5">
                <div className="flex items-center justify-between gap-4">
                  <span className="text-sm text-muted-foreground">Valor da obrigação</span>
                  <span className="text-lg font-bold text-foreground">{formatCents(bill.amount_cents)}</span>
                </div>
              </div>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <DetailFact label="Imóvel" value={propertyLabel || bill.property_id} />
                <DetailFact label="Competência" value={formatCompetence(bill.competence_month)} />
                <DetailFact label="Contrato" value={bill.contract_id} />
                <DetailFact label="Referência do pagamento" value={bill.payment_reference || "—"} />
                {bill.reconciled_at && <DetailFact label="Conciliação" value={formatDateTime(bill.reconciled_at)} />}
              </div>

              {bill.description && (
                <div>
                  <p className="text-sm font-bold text-foreground">Descrição</p>
                  <p className="mt-1 text-sm leading-relaxed text-muted-foreground">{bill.description}</p>
                </div>
              )}

              <ManagedBillAuditDetails bill={bill} />
            </div>

            <div className="flex justify-end border-t border-border bg-muted/30 px-6 py-4">
              <Button variant="outline" onClick={onClose}>Fechar</Button>
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}

function DetailFact({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <p className="text-xs font-bold text-muted-foreground">{label}</p>
      <p className="mt-1 truncate text-sm font-bold text-foreground" title={value}>{value || "—"}</p>
    </div>
  );
}

const HISTORY_ACTION_TONES: Record<string, { dot: string }> = {
  cancelled: { dot: "bg-gray-400" },
  payment_reversed: { dot: "bg-red-500" },
  reconciled: { dot: "bg-blue-500" },
  paid: { dot: "bg-emerald-500" },
};
function historyTone(action: string) {
  return HISTORY_ACTION_TONES[action] ?? { dot: "bg-amber-500" };
}

const HISTORY_ACTION_LABELS: Record<string, string> = {
  reserved: "Reserva criada",
  approved: "Aprovação registrada",
  paid: "Pagamento registrado",
  reconciled: "Conciliação registrada",
  payment_reversed: "Pagamento estornado",
  cancelled: "Cancelamento registrado",
};

function ManagedBillAuditDetails({ bill }: { bill: ManagedBill }) {
  const history = bill.history ?? [];
  const evidence = bill.evidence ?? [];
  const paymentAttempts = bill.payment_attempts ?? [];
  if (history.length === 0 && evidence.length === 0 && paymentAttempts.length === 0) return null;

  return (
    <details className="mt-3 max-w-sm rounded-lg border border-border bg-muted/20 p-2 text-xs">
      <summary className="cursor-pointer font-bold text-primary-700">
        Trilha ({history.length}) · tentativas ({paymentAttempts.length}) · evidências ({evidence.length})
      </summary>
      <div className="mt-3 space-y-3">
        {history.length > 0 && (
          <div>
            <p className="font-bold text-foreground">Histórico</p>
            <ol className="mt-1 space-y-2" aria-label="Histórico da conta administrada">
              {history.map((entry, index) => (
                <li key={`${entry.version}-${entry.action}-${index}`} className="relative flex gap-2.5 pl-1">
                  <span className={`mt-1.5 h-2.5 w-2.5 shrink-0 rounded-full ${historyTone(entry.action).dot}`} />
                  <div className="min-w-0">
                    <p className="text-xs font-medium text-foreground">
                      {HISTORY_ACTION_LABELS[entry.action] ?? entry.action} · v{entry.version}
                    </p>
                    <p className="text-[11px] text-muted-foreground">{entry.actor || "Ator não informado"} · {formatDateTime(entry.at)}</p>
                    {entry.reference && <p className="break-all text-[11px] text-muted-foreground">Ref: {entry.reference}</p>}
                    {entry.reason && <p className="break-words text-[11px] text-muted-foreground">Motivo: {entry.reason}</p>}
                  </div>
                </li>
              ))}
            </ol>
          </div>
        )}

        {evidence.length > 0 && (
          <div>
            <p className="font-bold text-foreground">Evidências</p>
            <ul className="mt-1 space-y-2" aria-label="Evidências da conta administrada">
              {evidence.map((entry, index) => (
                <li key={`${entry.kind}-${entry.at}-${index}`} className="rounded-md border border-border bg-card p-2 text-muted-foreground">
                  <p className="font-medium text-foreground">{entry.kind || "Evidência"}</p>
                  <p className="break-all">Referência: {entry.reference || "—"}</p>
                  <p>{entry.actor || "Ator não informado"} · {formatDateTime(entry.at)}</p>
                </li>
              ))}
            </ul>
          </div>
        )}

        {paymentAttempts.length > 0 && (
          <div>
            <p className="font-bold text-foreground">Tentativas anteriores de pagamento</p>
            <ol className="mt-1 space-y-2" aria-label="Tentativas anteriores de pagamento">
              {paymentAttempts.map((attempt, index) => (
                <li key={attempt.id || `${attempt.payment_reference}-${index}`} className="rounded-md border border-border bg-card p-2 text-muted-foreground">
                  <p className="font-medium text-foreground">
                    {attempt.payment_reference || `Tentativa ${index + 1}`} · estornada
                  </p>
                  <p>{attempt.paid_by || "Ator não informado"} · paga em {formatDateTime(attempt.paid_at)}</p>
                  {attempt.reconciliation_reference && (
                    <p className="break-all">Conciliação: {attempt.reconciliation_reference}</p>
                  )}
                  <p className="break-all">Estorno: {attempt.reversal_reference || "—"}</p>
                  <p>{attempt.reversed_by || "Ator não informado"} · {formatDateTime(attempt.reversed_at)}</p>
                  {attempt.reversal_reason && <p className="break-words">Motivo: {attempt.reversal_reason}</p>}
                </li>
              ))}
            </ol>
          </div>
        )}
      </div>
    </details>
  );
}

function actionIcon(action: ManagedBillAction) {
  if (action === "approve") return <CheckCircle2 className="h-3.5 w-3.5" />;
  if (action === "pay") return <CircleDollarSign className="h-3.5 w-3.5" />;
  if (action === "reconcile") return <FileCheck2 className="h-3.5 w-3.5" />;
  if (action === "reverse") return <RotateCcw className="h-3.5 w-3.5" />;
  return <Ban className="h-3.5 w-3.5" />;
}

function ActionDialog({
  openAction,
  pending,
  error,
  paymentReference,
  proofUrl,
  reconciliationReference,
  reversalReference,
  reversalReason,
  cancelReason,
  onPaymentReferenceChange,
  onProofUrlChange,
  proofFile,
  onProofFileChange,
  onReconciliationReferenceChange,
  onReversalReferenceChange,
  onReversalReasonChange,
  onCancelReasonChange,
  onClose,
  onSubmit,
}: {
  openAction: OpenAction | null;
  pending: boolean;
  error: string | null;
  paymentReference: string;
  proofUrl: string;
  reconciliationReference: string;
  reversalReference: string;
  reversalReason: string;
  cancelReason: string;
  onPaymentReferenceChange: (value: string) => void;
  onProofUrlChange: (value: string) => void;
  proofFile: File | null;
  onProofFileChange: (file: File | null) => void;
  onReconciliationReferenceChange: (value: string) => void;
  onReversalReferenceChange: (value: string) => void;
  onReversalReasonChange: (value: string) => void;
  onCancelReasonChange: (value: string) => void;
  onClose: () => void;
  onSubmit: () => Promise<void>;
}) {
  if (!openAction) return null;
  const presentation = ACTION_PRESENTATION[openAction.action];

  return (
    <Dialog open onOpenChange={(next) => { if (!next) onClose(); }}>
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-lg">
        <div className="border-b border-border bg-muted/30 px-6 py-4 pr-12">
          <DialogTitle>{presentation.title}</DialogTitle>
          <DialogDescription className="mt-1">{presentation.description}</DialogDescription>
        </div>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void onSubmit();
          }}
          className="space-y-4 p-6"
        >
          <div className="rounded-xl border border-border bg-muted/20 p-3 text-sm">
            <p className="font-bold text-foreground">{openAction.bill.component} · {openAction.bill.creditor_name}</p>
            <p className="mt-1 text-xs text-muted-foreground">
              {formatCents(openAction.bill.amount_cents)} · versão {openAction.bill.version}
            </p>
          </div>

          {openAction.action === "pay" && (
            <>
              <Field label="Referência do pagamento *" htmlFor="managed-bill-payment-reference">
                <input
                  id="managed-bill-payment-reference"
                  value={paymentReference}
                  onChange={(event) => onPaymentReferenceChange(event.target.value)}
                  placeholder="Ex.: PIX E2E, autenticação bancária"
                  className="h-10 w-full rounded-xl border border-border bg-card px-3 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
                  autoFocus
                />
              </Field>
              <Field label="Comprovante (opcional)" htmlFor="managed-bill-proof-file">
                <label className="flex h-10 w-full cursor-pointer items-center justify-center gap-2 rounded-xl border border-dashed border-border bg-card px-3 text-sm text-muted-foreground transition-colors hover:border-primary-500 hover:text-foreground">
                  <Upload className="h-4 w-4 shrink-0" />
                  <span className="truncate">{proofFile ? proofFile.name : "Anexar PDF, JPG ou PNG"}</span>
                  <input
                    id="managed-bill-proof-file"
                    type="file"
                    accept=".pdf,.jpg,.jpeg,.png"
                    className="sr-only"
                    onChange={(event) => onProofFileChange(event.target.files?.[0] ?? null)}
                  />
                </label>
              </Field>
              <Field label="ou informe o link do comprovante (opcional)" htmlFor="managed-bill-proof-url">
                <input
                  id="managed-bill-proof-url"
                  type="url"
                  value={proofUrl}
                  onChange={(event) => onProofUrlChange(event.target.value)}
                  placeholder="https://…"
                  className="h-10 w-full rounded-xl border border-border bg-card px-3 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
                />
              </Field>
            </>
          )}

          {openAction.action === "reconcile" && (
            <Field label="Referência da conciliação *" htmlFor="managed-bill-reconciliation-reference">
              <input
                id="managed-bill-reconciliation-reference"
                value={reconciliationReference}
                onChange={(event) => onReconciliationReferenceChange(event.target.value)}
                placeholder="Ex.: transação do extrato ou confirmação do credor"
                className="h-10 w-full rounded-xl border border-border bg-card px-3 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
                autoFocus
              />
            </Field>
          )}

          {openAction.action === "reverse" && (
            <>
              <Field label="Referência do estorno *" htmlFor="managed-bill-reversal-reference">
                <input
                  id="managed-bill-reversal-reference"
                  value={reversalReference}
                  onChange={(event) => onReversalReferenceChange(event.target.value)}
                  placeholder="Ex.: devolução bancária, protocolo ou ajuste"
                  className="h-10 w-full rounded-xl border border-border bg-card px-3 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
                  autoFocus
                />
              </Field>
              <Field label="Motivo do estorno *" htmlFor="managed-bill-reversal-reason">
                <textarea
                  id="managed-bill-reversal-reason"
                  value={reversalReason}
                  onChange={(event) => onReversalReasonChange(event.target.value)}
                  rows={3}
                  placeholder="Explique por que o pagamento precisa ser revertido"
                  className="w-full rounded-xl border border-border bg-card px-3 py-2 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
                />
              </Field>
              <Field label="URL do comprovante do estorno (opcional)" htmlFor="managed-bill-reversal-proof-url">
                <input
                  id="managed-bill-reversal-proof-url"
                  type="url"
                  value={proofUrl}
                  onChange={(event) => onProofUrlChange(event.target.value)}
                  placeholder="https://…"
                  className="h-10 w-full rounded-xl border border-border bg-card px-3 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
                />
              </Field>
            </>
          )}

          {openAction.action === "cancel" && (
            <Field label="Motivo do cancelamento *" htmlFor="managed-bill-cancel-reason">
              <textarea
                id="managed-bill-cancel-reason"
                value={cancelReason}
                onChange={(event) => onCancelReasonChange(event.target.value)}
                rows={3}
                placeholder="Descreva por que esta obrigação deve ser cancelada"
                className="w-full rounded-xl border border-border bg-card px-3 py-2 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
                autoFocus
              />
            </Field>
          )}

          {error && (
            <div className="rounded-xl border border-status-critical/30 bg-status-critical-soft p-3 text-sm text-status-critical" role="alert">
              {error}
            </div>
          )}

          <DialogFooter className="border-t border-border pt-4">
            <Button type="button" variant="outline" onClick={onClose} disabled={pending}>Voltar</Button>
            <Button
              type="submit"
              variant={openAction.action === "cancel" || openAction.action === "reverse" ? "destructive" : "default"}
              loading={pending}
              loadingText="Processando…"
            >
              {presentation.submitLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function Field({ label, htmlFor, children }: { label: string; htmlFor: string; children: ReactNode }) {
  return (
    <div>
      <label htmlFor={htmlFor} className="mb-1 block text-xs font-bold text-foreground">{label}</label>
      {children}
    </div>
  );
}

function BalanceImpact({ result }: { result: ManagedBillMutationResult }) {
  const hasSnapshots = Boolean(result.owner_balance_before && result.owner_balance_after);
  return (
    <div className="border-b border-border bg-primary-50/50 p-5" aria-live="polite">
      <div className="flex items-start gap-3">
        <Scale className="mt-0.5 h-5 w-5 shrink-0 text-primary-700" />
        <div className="min-w-0 flex-1 space-y-3">
          <div>
            <p className="text-sm font-bold text-primary-900">Impacto econômico da última ação</p>
            <p className="text-xs text-primary-800">
              Dados autoritativos retornados pelo motor financeiro; esta composição não representa saldo bancário.
            </p>
            {result.bill.status === "PAID" && (
              <p className="mt-1 text-xs font-bold text-status-info">PAID: pagamento registrado; ainda não conciliado.</p>
            )}
          </div>

          {!result.coverage_complete ? (
            <p className="rounded-lg border border-status-warning/30 bg-status-warning-soft p-3 text-xs text-status-warning">
              Cobertura financeira parcial. Os bolsões foram omitidos para não fabricar um saldo incompleto.
            </p>
          ) : hasSnapshots ? (
            <div className="grid gap-3 lg:grid-cols-2">
              <BalanceSnapshot label="Antes" balance={result.owner_balance_before!} testId="managed-bill-balance-before" />
              <BalanceSnapshot label="Depois" balance={result.owner_balance_after!} testId="managed-bill-balance-after" />
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">
              A ação foi registrada, mas não houve composição de bolsões retornada para esta conta.
            </p>
          )}
        </div>
      </div>
    </div>
  );
}

function BalanceSnapshot({
  label,
  balance,
  testId,
}: {
  label: string;
  balance: ManagedBillOwnerBalance;
  testId: string;
}) {
  return (
    <div className="rounded-xl border border-primary-200 bg-card p-3" data-testid={testId}>
      <p className="mb-2 text-xs font-bold uppercase tracking-wide text-primary-700">{label}</p>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-xs sm:grid-cols-4">
        <BalanceValue label="Total" cents={balance.total_cents} />
        <BalanceValue label="Reservado" cents={balance.reserved_cents} />
        <BalanceValue label="Bloqueado" cents={balance.blocked_cents} />
        <BalanceValue label="Disponível" cents={availableCents(balance)} />
      </dl>
    </div>
  );
}

function BalanceValue({ label, cents }: { label: string; cents: number }) {
  return (
    <div>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="font-bold text-foreground">{formatCents(cents)}</dd>
    </div>
  );
}
