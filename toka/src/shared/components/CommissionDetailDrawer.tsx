import { useState } from "react";
import {
  AlertTriangle,
  Briefcase,
  Calendar,
  CheckCircle2,
  Copy,
  DollarSign,
  FileText,
  Pencil,
  RotateCcw,
  User,
  XCircle,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { useNavigate } from "react-router";

import { Button } from "../ui/button";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "../ui/sheet";
import { CommissionDeductionsEditor } from "./CommissionDeductionsEditor";
import { DivergenceResolutionCard } from "./lancamentos/DivergenceResolutionCard";
import { FinancialProofCard } from "./lancamentos/FinancialProofCard";
import { RentalFundingProgressCard } from "./lancamentos/RentalFundingProgressCard";
import { FinancialPartyTimeline } from "./finance/FinancialPartyTimeline";
import { commissionToLancamento } from "../lib/lancamentos/mappers";
import { computeDivergence } from "../lib/lancamentos/divergence";
import type { Commission, CommissionStatus } from "../services/commission.service";

export type CommissionDrawerAction = "approve" | "pay" | "attach-proof" | "cancel" | "reverse";

interface CommissionDetailDrawerProps {
  commission: Commission | null;
  brokerLabel?: string;
  contractLabel?: string;
  rentalOrigin?: boolean;
  onClose: () => void;
  onAction: (action: CommissionDrawerAction, commission: Commission) => void;
  onCommissionChange?: (updated: Commission) => void;
}

const formatDateTime = (iso?: string) => {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("pt-BR", { day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" });
  } catch {
    return iso;
  }
};

const STATUS_META: Record<CommissionStatus, { label: string; cls: string; icon: typeof CheckCircle2 }> = {
  PENDING: { label: "Pendente", cls: "bg-status-warning-soft text-status-warning border-status-warning/30", icon: AlertTriangle },
  APPROVED: { label: "Aprovada", cls: "bg-status-info-soft text-status-info border-status-info/30", icon: Pencil },
  PAID: { label: "Paga", cls: "bg-status-ok-soft text-status-ok border-status-ok/30", icon: CheckCircle2 },
  REVERSED: { label: "Estornada", cls: "bg-status-critical-soft text-status-critical border-status-critical/30", icon: XCircle },
  CANCELLED: { label: "Cancelada", cls: "bg-muted text-muted-foreground border-border", icon: XCircle },
};

const ROLE_LABEL: Record<string, string> = {
  captador: "Captador",
  co_captador: "Co-captador",
  visitador: "Visitador",
  atendente: "Atendente",
  fechamento: "Fechamento",
  imobiliaria: "Imobiliária",
};

const availableCommissionActions = (status: CommissionStatus): CommissionDrawerAction[] => {
  switch (status) {
    case "PENDING":
      return ["approve", "pay", "cancel"];
    case "APPROVED":
      return ["pay", "cancel", "reverse"];
    case "PAID":
      return ["reverse"];
    case "REVERSED":
    case "CANCELLED":
      return [];
  }
};

/**
 * Por que o desconto está bloqueado, por status. O backend só aceita
 * `POST /commissions/:id/deductions` enquanto a comissão está `PENDING` — que é
 * exatamente a janela em que a ordem de pagamento ainda aceita ajuste. Depois
 * disso o líquido já está comprometido, e o caminho é cancelar/regerar a ordem,
 * não "furar" o valor por trás.
 */
const deductionBlockedReason = (status?: CommissionStatus): string | undefined => {
  switch (status) {
    case undefined:
    case "PENDING":
      return undefined;
    case "APPROVED":
      return "A comissão já foi aprovada e o valor da ordem de pagamento está comprometido. Para lançar um desconto, cancele ou regenere a ordem e lance antes de aprovar de novo.";
    case "PAID":
      return "A comissão já foi paga: o líquido saiu e não pode ser recalculado por aqui. Um desconto lançado a mais exige estorno da comissão.";
    case "REVERSED":
      return "Comissão estornada — não há comissão a descontar. Recrie a comissão com a composição correta.";
    case "CANCELLED":
      return "Comissão cancelada — não há comissão a descontar. Recrie a comissão com a composição correta.";
  }
};

export function CommissionDetailDrawer({ commission, brokerLabel, contractLabel, rentalOrigin = false, onClose, onAction, onCommissionChange }: CommissionDetailDrawerProps) {
  const navigate = useNavigate();
  const status: CommissionStatus | undefined = commission?.status;
  const statusMeta = status ? STATUS_META[status] : null;
  const managedByPayout = Boolean(commission?.payout_order_id);
  const fundingRequiredCents = commission?.funding_required_cents ?? 0;
  const fundedAmountCents = commission?.funded_amount_cents ?? 0;
  const fundingOutstandingCents = Math.max(0, fundingRequiredCents - fundedAmountCents);
  const fundingReconciliationRequired = rentalOrigin && fundingRequiredCents <= 0 && (status === "PENDING" || status === "APPROVED");
  const fundingBlocked = fundingReconciliationRequired || (fundingRequiredCents > 0 && fundingOutstandingCents > 0);
  const actions = status && !managedByPayout
    ? availableCommissionActions(status).filter((action) => !fundingBlocked || (action !== "approve" && action !== "pay"))
    : [];
  const lancamento = commission ? commissionToLancamento(commission) : null;
  const divergence = lancamento ? computeDivergence(lancamento) : null;
  const divergenceActionLabel = divergence
    ? divergence.reason === "missing_proof"
      ? managedByPayout
        ? "Abrir ordem vinculada"
        : "Anexar comprovante"
      : divergence.reason === "amount_mismatch" && (status === "PAID" || status === "APPROVED")
        ? "Estornar para corrigir"
        : undefined
    : undefined;

  const copyId = () => {
    if (!commission?.id) return;
    navigator.clipboard.writeText(commission.id).then(() => toast.success("ID copiado"));
  };

  return (
    // Primitiva compartilhada (Radix Dialog via Sheet): entrega papel de
    // diálogo, aria-modal, título associado, portal, contenção de foco, Escape
    // e retorno do foco ao gatilho. O overlay artesanal com motion.div não
    // tinha nenhum disso — nem Escape, o que impedia fechar pelo teclado.
    <Sheet open={Boolean(commission)} onOpenChange={(next) => { if (!next) onClose(); }}>
      <SheetContent
        side="right"
        expandable
        className="w-full gap-0 overflow-hidden p-0 sm:max-w-lg"
        data-testid="commission-detail-drawer"
      >
        {commission && (
          <>
            <div className="px-6 py-4 border-b border-border flex items-start justify-between gap-3 pr-20">
              <div className="min-w-0">
                <SheetTitle className="text-xl font-bold text-foreground flex items-center gap-2">
                  <Briefcase className="w-5 h-5 text-primary-600" />
                  Detalhes da Comissão
                </SheetTitle>
                <SheetDescription className="sr-only">
                  Composição, evidências e ações da comissão selecionada.
                </SheetDescription>
                <button
                  type="button"
                  onClick={copyId}
                  className="mt-1 inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors group"
                  aria-label={`Copiar ID da comissão ${commission.id}`}
                >
                  <span className="font-mono truncate max-w-[260px]">{commission.id}</span>
                  <Copy className="w-3 h-3 opacity-50 group-hover:opacity-100" aria-hidden="true" />
                </button>
              </div>
            </div>

            <div className="flex-1 overflow-y-auto p-6 space-y-6">
              <div className="flex justify-end">
                {statusMeta && (
                  <span className={clsx("inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-bold border", statusMeta.cls)}>
                    <statusMeta.icon className="w-3.5 h-3.5" /> {statusMeta.label}
                  </span>
                )}
              </div>

              <FinancialPartyTimeline
                beneficiaryLabel={brokerLabel || commission.broker_id}
                beneficiaryKind={commission.role ? `Corretor · ${ROLE_LABEL[commission.role] || commission.role}` : "Corretor"}
                beneficiaryId={commission.broker_id}
                settledAt={commission.paid_at}
              />

              {lancamento && (
                <DivergenceResolutionCard
                  lancamento={lancamento}
                  actionLabel={divergenceActionLabel}
                  onAction={
                    divergenceActionLabel
                      ? () =>
                          onAction(
                            divergence?.reason === "missing_proof" ? "attach-proof" : "reverse",
                            commission,
                          )
                      : undefined
                  }
                />
              )}

              {fundingReconciliationRequired && (
                <div className="rounded-xl border border-status-warning/30 bg-status-warning-soft p-4 space-y-1.5">
                  <p className="text-sm font-bold text-status-warning">Cobertura do aluguel ainda não reconciliada</p>
                  <p className="text-xs leading-relaxed text-muted-foreground">
                    Esta comissão de locação foi criada antes do controle de origem de caixa. Aprovação e pagamento permanecem bloqueados até o plano ser reprocessado e vinculado aos aluguéis recebidos.
                  </p>
                </div>
              )}

              <RentalFundingProgressCard
                requiredCents={fundingRequiredCents}
                fundedCents={fundedAmountCents}
                entries={commission.funding_entries}
              />

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
                <Fact icon={FileText} label="Contrato" value={contractLabel || commission.contract_id || commission.deal_id} />
                <Fact icon={DollarSign} label="% Comissão" value={commission.percentage !== undefined ? `${commission.percentage}%` : "—"} />
                <Fact icon={Calendar} label="Criada em" value={formatDateTime(commission.created_at)} />
                <Fact icon={Calendar} label="Atualizada em" value={formatDateTime(commission.updated_at)} />
                {commission.paid_at && <Fact icon={CheckCircle2} label="Paga em" value={formatDateTime(commission.paid_at)} accent="ok" />}
                {commission.cancelled_at && <Fact icon={XCircle} label="Cancelada em" value={formatDateTime(commission.cancelled_at)} accent="critical" />}
              </div>

              {(commission.deal_id || commission.contract_id) && (
                <div className="flex flex-wrap items-center gap-2">
                  {commission.deal_id && (
                    <Button variant="outline" size="sm" onClick={() => { onClose(); navigate(`/esteira/deals/${commission.deal_id}`); }}>
                      <Briefcase className="w-4 h-4" /> Ver negócio
                    </Button>
                  )}
                  {commission.contract_id && (
                    <Button variant="outline" size="sm" onClick={() => { onClose(); navigate(`/contratos/${commission.contract_id}`); }}>
                      <FileText className="w-4 h-4" /> Ver contrato
                    </Button>
                  )}
                </div>
              )}

              <CommissionDeductionsEditor
                commission={commission}
                onChange={(updated) => onCommissionChange?.(updated)}
                readOnly={status !== "PENDING"}
                readOnlyReason={deductionBlockedReason(status)}
              />

              {managedByPayout && (
                <div className={clsx(
                  "rounded-xl border p-4 space-y-3",
                  fundingBlocked
                    ? "border-status-warning/30 bg-status-warning-soft"
                    : "border-status-info/30 bg-status-info-soft",
                )}>
                  <p className={clsx("text-sm font-bold", fundingBlocked ? "text-status-warning" : "text-status-info")}>
                    {fundingBlocked ? "Pagamento aguardando cobertura" : "Ciclo gerenciado pela ordem de pagamento"}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {fundingBlocked
                      ? `A ordem já foi provisionada, mas só será liberada para revisão após os aluguéis cobrirem os ${formatCurrencyFromCents(fundingOutstandingCents)} restantes.`
                      : "Aprovação, pagamento e cancelamento devem ser feitos na ordem de pagamento vinculada para manter os dois registros sincronizados."}
                  </p>
                  <Button variant="outline" size="sm" onClick={() => { onClose(); navigate("/financeiro/payouts"); }}>
                    <DollarSign className="w-4 h-4" /> Abrir ordem de pagamento
                  </Button>
                </div>
              )}

              {(commission.proof_url || commission.note) && (
                <div className="rounded-xl border border-border bg-muted/30 p-4 space-y-2">
                  <p className="text-xs font-bold uppercase tracking-wider text-muted-foreground">Pagamento</p>
                  {commission.proof_url && <FinancialProofCard url={commission.proof_url} />}
                  {commission.note && <p className="text-sm text-foreground/80 whitespace-pre-wrap">{commission.note}</p>}
                </div>
              )}

              {commission.payout_order_id && <Fact icon={User} label="Ordem de pagamento" value={commission.payout_order_id} />}
            </div>

            <div className="px-6 py-4 border-t border-border bg-muted/30 flex items-center justify-between flex-wrap gap-2">
              <Button variant="ghost" onClick={onClose}>
                Fechar
              </Button>
              <div className="flex items-center gap-2 flex-wrap justify-end">
                {actions.length === 0 ? (
                  <span className="text-xs text-muted-foreground italic">
                    {fundingBlocked
                      ? fundingReconciliationRequired
                        ? "Reprocesse o plano de locação para liberar a aprovação"
                        : "Aprovação liberada após a cobertura pelos recebimentos"
                      : managedByPayout
                        ? "Use a ordem de pagamento vinculada"
                        : "Nenhuma ação disponível para este status"}
                  </span>
                ) : (
                  actions.map((a) => {
                    if (a === "approve") {
                      return (
                        <Button key={a} variant="outline" size="sm" onClick={() => onAction("approve", commission)}>
                          <CheckCircle2 className="w-4 h-4" /> Aprovar
                        </Button>
                      );
                    }
                    if (a === "pay") {
                      return (
                        <Button key={a} variant="default" size="sm" onClick={() => onAction("pay", commission)}>
                          <DollarSign className="w-4 h-4" /> Marcar Paga
                        </Button>
                      );
                    }
                    if (a === "reverse") {
                      return (
                        <Button key={a} variant="outline" size="sm" className="text-status-critical hover:text-status-critical hover:bg-status-critical-soft border-status-critical/20" onClick={() => onAction("reverse", commission)}>
                          <RotateCcw className="w-4 h-4" /> Estornar
                        </Button>
                      );
                    }
                    return (
                      <Button key={a} variant="destructive" size="sm" onClick={() => onAction("cancel", commission)}>
                        <XCircle className="w-4 h-4" /> Cancelar
                      </Button>
                    );
                  })
                )}
              </div>
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}

function formatCurrencyFromCents(cents: number) {
  return new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(cents / 100);
}

function Fact({ icon: Icon, label, value, accent }: { icon: typeof DollarSign; label: string; value: React.ReactNode; accent?: "ok" | "critical" }) {
  const color = accent === "ok" ? "text-status-ok" : accent === "critical" ? "text-status-critical" : "text-foreground";
  return (
    <div>
      <p className="text-xs font-bold text-muted-foreground mb-1">{label}</p>
      <div className={clsx("flex items-center gap-2 text-sm font-bold", color)}>
        <Icon className={clsx("w-4 h-4 shrink-0", accent ? color : "text-primary-600")} />
        <span className="truncate">{value || "—"}</span>
      </div>
    </div>
  );
}
