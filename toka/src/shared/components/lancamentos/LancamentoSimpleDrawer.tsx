import { useState } from "react";
import { motion, AnimatePresence } from "motion/react";
import {
  AlertTriangle,
  Briefcase,
  Calendar,
  CheckCircle2,
  Copy,
  Crown,
  DollarSign,
  Download,
  ExternalLink,
  FileText,
  Home,
  LayoutList,
  Maximize2,
  Minimize2,
  PlayCircle,
  User,
  Wallet,
  X,
  XCircle,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";

import { Button } from "../../ui/button";
import { formatMoneyBR } from "../../utils/format";
import { lancamentoSituationLabel, lancamentoSituationVariant } from "../../lib/lancamentos/chargeDue";
import type { Lancamento, RoyaltyLancamento, TransactionLancamento, TransferLancamento } from "../../lib/lancamentos/types";
import { DivergenceResolutionCard } from "./DivergenceResolutionCard";
import { FinancialProofCard } from "./FinancialProofCard";

export interface LancamentoSimpleDrawerActions {
  canApprove?: boolean;
  canProcess?: boolean;
  canDownloadReceipt?: boolean;
  canSettle?: boolean;
  /** Repasse sem saldo a repassar: encerra sem comprovante nem caixa. */
  canSettleWithoutBalance?: boolean;
  canCancel?: boolean;
  canReverse?: boolean;
  approving?: boolean;
  processing?: boolean;
  settling?: boolean;
  settlingWithoutBalance?: boolean;
  cancelling?: boolean;
  reversing?: boolean;
  onApprove?: () => void;
  onProcess?: () => void;
  onDownloadReceipt?: () => void;
  onSettle?: () => void;
  onSettleWithoutBalance?: () => void;
  onCancel?: () => void;
  onReverse?: () => void;
}

interface LancamentoSimpleDrawerProps {
  lancamento: Lancamento | null;
  onClose: () => void;
  onNavigate?: (l: Lancamento) => void;
  divergenceActionLabel?: string;
  onResolveDivergence?: (l: Lancamento) => void;
  operationalNotice?: string;
  actions?: LancamentoSimpleDrawerActions;
}

const formatDateTime = (iso?: string) => {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString("pt-BR", {
      day: "2-digit",
      month: "2-digit",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
};

const KIND_HEADER: Record<Lancamento["kind"], { title: string; icon: typeof FileText }> = {
  charge: { title: "Detalhes do Lançamento", icon: FileText },
  commission: { title: "Detalhes do Lançamento", icon: Briefcase },
  transfer: { title: "Detalhes do Repasse", icon: Home },
  royalty: { title: "Detalhes do Royalty", icon: Crown },
  transaction: { title: "Detalhes do Lançamento de Caixa", icon: Wallet },
};

export function LancamentoSimpleDrawer({ lancamento, onClose, onNavigate, divergenceActionLabel, onResolveDivergence, operationalNotice, actions }: LancamentoSimpleDrawerProps) {
  const [isExpanded, setIsExpanded] = useState(false);

  const copyId = () => {
    if (!lancamento?.id) return;
    navigator.clipboard.writeText(lancamento.id).then(() => toast.success("ID copiado"));
  };

  return (
    <AnimatePresence>
      {lancamento && (
        <>
          <motion.div
            key="lanc-simple-overlay"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            onClick={onClose}
            className="fixed inset-0 bg-foreground/50 backdrop-blur-sm z-40"
          />
          <motion.div
            key="lanc-simple-panel"
            initial={{ x: "100%" }}
            animate={{ x: 0 }}
            exit={{ x: "100%" }}
            transition={{ type: "spring", damping: 25, stiffness: 200 }}
            className={clsx(
              "fixed inset-y-0 right-0 bg-card shadow-2xl z-50 flex flex-col border-l border-border transition-[max-width] duration-300 ease-in-out",
              isExpanded ? "w-full max-w-full" : "w-full max-w-lg",
            )}
          >
            <Header
              lancamento={lancamento}
              isExpanded={isExpanded}
              onToggleExpand={() => setIsExpanded((v) => !v)}
              onClose={onClose}
              onCopyId={copyId}
            />

            <div className="flex-1 overflow-y-auto p-6 space-y-6">
              <StatusBlock lancamento={lancamento} />
              <DivergenceResolutionCard
                lancamento={lancamento}
                actionLabel={divergenceActionLabel}
                onAction={onResolveDivergence ? () => onResolveDivergence(lancamento) : undefined}
              />
              {operationalNotice && (
                <div role="status" className="flex gap-3 rounded-xl border border-status-warning/30 bg-status-warning-soft p-4 text-sm text-status-warning">
                  <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
                  <p className="leading-relaxed">{operationalNotice}</p>
                </div>
              )}
              <FactsBlock lancamento={lancamento} />
              {lancamento.kind === "transfer" && <TransferSection l={lancamento} />}
              {lancamento.kind === "royalty" && <RoyaltySection l={lancamento} />}
              {lancamento.kind === "transaction" && <TransactionSection l={lancamento} />}
              <MetadataBlock lancamento={lancamento} />
            </div>

            <Footer lancamento={lancamento} onClose={onClose} onNavigate={onNavigate} actions={actions} />
          </motion.div>
        </>
      )}
    </AnimatePresence>
  );
}

function Header({
  lancamento,
  isExpanded,
  onToggleExpand,
  onClose,
  onCopyId,
}: {
  lancamento: Lancamento;
  isExpanded: boolean;
  onToggleExpand: () => void;
  onClose: () => void;
  onCopyId: () => void;
}) {
  const meta = KIND_HEADER[lancamento.kind];
  const Icon = meta.icon;
  return (
    <div className="px-6 py-4 border-b border-border flex items-start justify-between gap-3">
      <div className="min-w-0">
        <h2 className="text-xl font-bold text-foreground flex items-center gap-2">
          <Icon className="w-5 h-5 text-primary-600" /> {meta.title}
        </h2>
        <button
          type="button"
          onClick={onCopyId}
          className="mt-1 inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors group"
          title="Copiar ID"
        >
          <span className="font-mono truncate max-w-[260px]">{lancamento.id}</span>
          <Copy className="w-3 h-3 opacity-50 group-hover:opacity-100" />
        </button>
      </div>
      <div className="flex items-center gap-2 shrink-0">
        <Button variant="outline" size="sm" onClick={onToggleExpand} title={isExpanded ? "Recolher" : "Expandir"}>
          {isExpanded ? <Minimize2 className="w-4 h-4" /> : <Maximize2 className="w-4 h-4" />}
          <span className="hidden sm:inline">{isExpanded ? "Recolher" : "Expandir"}</span>
        </Button>
        <Button variant="ghost" size="icon" onClick={onClose} aria-label="Fechar">
          <X className="w-5 h-5" />
        </Button>
      </div>
    </div>
  );
}

function StatusBlock({ lancamento }: { lancamento: Lancamento }) {
  const variant = lancamentoSituationVariant(lancamento);
  return (
    <div className="flex items-start justify-between gap-4 flex-wrap">
      <div className="min-w-0">
        <p className="text-xs font-bold text-muted-foreground mb-1">Contraparte</p>
        <p className="text-base font-bold text-foreground break-words">{lancamento.counterparty.label}</p>
      </div>
      <span className={clsx("inline-flex items-center px-3 py-1 rounded-full text-xs font-bold border", variant.bg, variant.text, variant.border)}>
        {lancamentoSituationLabel(lancamento)}
      </span>
    </div>
  );
}

function FactsBlock({ lancamento }: { lancamento: Lancamento }) {
  const hasDistinctNetAmount =
    lancamento.net_amount !== undefined &&
    Math.abs(lancamento.net_amount - lancamento.amount) > 0.005;
  const paymentPlan =
    lancamento.kind === "transaction" ? lancamento.kind_data.payment_plan : undefined;

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <Fact icon={DollarSign} label="Valor" value={formatMoneyBR(lancamento.amount, { withSymbol: true })} />
      {hasDistinctNetAmount && <Fact icon={DollarSign} label="Líquido" value={formatMoneyBR(lancamento.net_amount!, { withSymbol: true })} />}
      {lancamento.due_at && <Fact icon={Calendar} label="Vencimento" value={formatDateTime(lancamento.due_at)} />}
      {lancamento.settled_at && <Fact icon={Calendar} label="Liquidado em" value={formatDateTime(lancamento.settled_at)} accent="ok" />}
      {paymentPlan && <Fact icon={LayoutList} label="Plano de pagamentos" value={paymentPlan} />}
      <Fact icon={FileText} label="Origem" value={lancamento.origin.label ?? lancamento.origin.entity_kind} />
    </div>
  );
}

function TransferSection({ l }: { l: TransferLancamento }) {
  const breakdown = l.kind_data.economic_breakdown;
  const isSale = l.kind_data.settlement_type === "sale" || breakdown?.flow === "sale";
  return (
    <Section title={isSale ? "Liquidação da venda" : "Detalhes do Repasse"}>
      {isSale && breakdown ? (
        <>
          <Fact icon={DollarSign} label="Valor recebido" value={formatMoneyBR(breakdown.gross_cents / 100, { withSymbol: true })} />
          <Fact icon={DollarSign} label="Obrigação ao proprietário" value={formatMoneyBR(breakdown.owner_payable_cents / 100, { withSymbol: true })} accent="ok" />
          <Fact icon={DollarSign} label="Comissões a terceiros" value={formatMoneyBR(breakdown.commission_payable_cents / 100, { withSymbol: true })} />
          <Fact icon={DollarSign} label="Obrigações a parceiros" value={formatMoneyBR(breakdown.partner_payable_cents / 100, { withSymbol: true })} />
          {breakdown.discount_cents > 0 && <Fact icon={DollarSign} label="Descontos aplicados" value={formatMoneyBR(breakdown.discount_cents / 100, { withSymbol: true })} accent="critical" />}
          <Fact icon={DollarSign} label="Receita da imobiliária" value={formatMoneyBR((breakdown.agency_revenue_cents || breakdown.agency_retained_cents) / 100, { withSymbol: true })} accent="ok" />
          <Fact icon={DollarSign} label="Disponível para repasse" value={formatMoneyBR(breakdown.available_cents / 100, { withSymbol: true })} accent="ok" />
        </>
      ) : (
        <>
          <Fact icon={DollarSign} label="Aluguel recebido" value={formatMoneyBR(l.kind_data.gross_rent, { withSymbol: true })} />
          <Fact
            icon={DollarSign}
            label={`Receita de administração (${l.kind_data.admin_fee_percent}%)`}
            value={formatMoneyBR((breakdown?.admin_fee_cents ?? Math.round(l.kind_data.admin_fee_amount * 100)) / 100, { withSymbol: true })}
            accent="ok"
          />
          {(breakdown?.intermediation_revenue_cents ?? 0) > 0 && (
            <Fact icon={DollarSign} label="Intermediação apropriada" value={formatMoneyBR((breakdown?.intermediation_revenue_cents ?? 0) / 100, { withSymbol: true })} />
          )}
          {(breakdown?.commission_funded_cents ?? 0) > 0 && (
            <Fact icon={DollarSign} label="Comissão coberta neste aluguel" value={formatMoneyBR((breakdown?.commission_funded_cents ?? 0) / 100, { withSymbol: true })} />
          )}
          {(breakdown?.agency_retained_cents ?? 0) > 0 && (
            <Fact icon={DollarSign} label="Receita própria líquida" value={formatMoneyBR((breakdown?.agency_retained_cents ?? 0) / 100, { withSymbol: true })} accent="ok" />
          )}
          {l.kind_data.maintenance_deductions > 0 && <Fact icon={DollarSign} label="Manutenção" value={formatMoneyBR(l.kind_data.maintenance_deductions, { withSymbol: true })} />}
          {l.kind_data.other_deductions > 0 && <Fact icon={DollarSign} label="Outras deduções" value={formatMoneyBR(l.kind_data.other_deductions, { withSymbol: true })} />}
          {breakdown && <Fact icon={DollarSign} label="Reservado para encargos" value={formatMoneyBR(breakdown.reserved_cents / 100, { withSymbol: true })} />}
          {breakdown && <Fact icon={DollarSign} label="Repasse líquido ao proprietário" value={formatMoneyBR(breakdown.available_cents / 100, { withSymbol: true })} accent="ok" />}
          {(breakdown?.intermediation_revenue_cents ?? 0) > 0 && (
            <p className="col-span-full rounded-lg border border-border bg-muted/50 p-3 text-xs leading-relaxed text-muted-foreground">
              Ordem desta competência: taxa de administração, cobertura da intermediação pendente e, por último, saldo do proprietário. Se o aluguel proporcional não cobrir toda a comissão, o restante segue automaticamente para as próximas competências.
            </p>
          )}
        </>
      )}
      {l.kind_data.approved_at && <Fact icon={Calendar} label="Aprovado em" value={formatDateTime(l.kind_data.approved_at)} accent="ok" />}
      {l.kind_data.processed_at && <Fact icon={Calendar} label="Processado em" value={formatDateTime(l.kind_data.processed_at)} accent="ok" />}
    </Section>
  );
}

function RoyaltySection({ l }: { l: RoyaltyLancamento }) {
  return (
    <Section title={l.origin.label || "Obrigação de parceria"}>
      {l.kind_data.royalty_base && <Fact icon={DollarSign} label="Base de cálculo" value={l.kind_data.royalty_base} />}
      {l.kind_data.recipient_label && <Fact icon={User} label="Recebedor" value={l.kind_data.recipient_label} />}
      {l.kind_data.source_deal_id && <Fact icon={FileText} label="Negócio origem" value={l.kind_data.source_deal_id} />}
      {l.kind_data.proof_url && (
        <div className="col-span-2">
          <FinancialProofCard url={l.kind_data.proof_url} />
        </div>
      )}
    </Section>
  );
}

function TransactionSection({ l }: { l: TransactionLancamento }) {
  const breakdown = l.kind_data.economic_breakdown;
  const proof = l.kind_data.proof_url;
  const note = l.kind_data.note;
  if ((!breakdown || breakdown.flow !== "sale") && !proof && !note) return null;
  return (
    <Section title="Baixa do lançamento">
      {proof && (
        <div className="col-span-2">
          <FinancialProofCard url={proof} />
        </div>
      )}
      {note && <Fact icon={FileText} label="Observação da baixa" value={note} />}
      {breakdown && breakdown.flow === "sale" && (<>
        <Fact icon={DollarSign} label="Valor recebido" value={formatMoneyBR(breakdown.gross_cents / 100, { withSymbol: true })} />
        <Fact icon={DollarSign} label="Comissão total" value={formatMoneyBR(breakdown.commission_pool_cents / 100, { withSymbol: true })} />
        <Fact icon={DollarSign} label="Comissões a corretores" value={formatMoneyBR(breakdown.commission_payable_cents / 100, { withSymbol: true })} />
        {breakdown.partner_payable_cents > 0 && <Fact icon={DollarSign} label="Parceiros e entidades" value={formatMoneyBR(breakdown.partner_payable_cents / 100, { withSymbol: true })} />}
        {breakdown.discount_cents > 0 && <Fact icon={DollarSign} label="Descontos aplicados" value={formatMoneyBR(breakdown.discount_cents / 100, { withSymbol: true })} accent="critical" />}
        <Fact icon={DollarSign} label="Receita efetiva da imobiliária" value={formatMoneyBR(breakdown.agency_retained_cents / 100, { withSymbol: true })} accent="ok" />
        <Fact icon={Home} label="Obrigação ao proprietário" value={formatMoneyBR(breakdown.owner_payable_cents / 100, { withSymbol: true })} />
        <p className="col-span-full text-xs text-muted-foreground rounded-lg bg-muted/50 border border-border p-3">
          {l.status === "pago" || l.status === "processado"
            ? "Recebimento liquidado. A obrigação do proprietário foi registrada no mesmo evento e deve ser acompanhada no repasse vinculado."
            : "Ao dar baixa, a obrigação do proprietário será criada no mesmo evento econômico. A baixa e o repasse são registrados de forma atômica."}
        </p>
      </>)}
      {!breakdown && (
        <p className="col-span-full text-xs text-muted-foreground rounded-lg bg-muted/50 border border-border p-3">
          {l.status === "pago"
            ? "Baixa registrada no caixa (journal). Acompanhe o plano de contas na visão de posição financeira."
            : "Ao dar baixa, o valor é registrado no caixa e aparece no plano de contas."}
        </p>
      )}
    </Section>
  );
}

function MetadataBlock({ lancamento }: { lancamento: Lancamento }) {
  return (
    <Section title="Histórico do registro">
      <Fact icon={Calendar} label="Criado em" value={formatDateTime(lancamento.created_at)} />
      <Fact icon={Calendar} label="Atualizado em" value={formatDateTime(lancamento.updated_at)} />
    </Section>
  );
}

function Footer({
  lancamento,
  onClose,
  onNavigate,
  actions,
}: {
  lancamento: Lancamento;
  onClose: () => void;
  onNavigate?: (l: Lancamento) => void;
  actions?: LancamentoSimpleDrawerActions;
}) {
  const hasInlineActions = !!actions && (actions.canApprove || actions.canProcess || actions.canDownloadReceipt || actions.canSettle || actions.canSettleWithoutBalance || actions.canCancel || actions.canReverse);
  return (
    <div className="px-6 py-4 border-t border-border bg-muted/30 flex items-center justify-between flex-wrap gap-2">
      <Button variant="ghost" onClick={onClose}>
        Fechar
      </Button>
      <div className="flex items-center gap-2 flex-wrap justify-end">
        {actions?.canDownloadReceipt && (
          <Button variant="outline" size="sm" onClick={actions.onDownloadReceipt}>
            <Download className="w-4 h-4" /> Baixar recibo
          </Button>
        )}
        {actions?.canApprove && (
          <Button
            variant="outline"
            size="sm"
            onClick={actions.onApprove}
            loading={actions.approving}
            loadingText="Aprovando..."
          >
            <CheckCircle2 className="w-4 h-4" /> Aprovar
          </Button>
        )}
        {actions?.canProcess && (
          <Button
            variant="default"
            size="sm"
            onClick={actions.onProcess}
            loading={actions.processing}
            loadingText="Processando..."
          >
            <PlayCircle className="w-4 h-4" /> Processar
          </Button>
        )}
        {actions?.canSettle && (
          <Button
            variant="default"
            size="sm"
            onClick={actions.onSettle}
            loading={actions.settling}
            loadingText="Baixando..."
          >
            <CheckCircle2 className="w-4 h-4" /> Dar baixa
          </Button>
        )}
        {actions?.canSettleWithoutBalance && (
          <Button
            variant="outline"
            size="sm"
            onClick={actions.onSettleWithoutBalance}
            loading={actions.settlingWithoutBalance}
            loadingText="Concluindo..."
          >
            <CheckCircle2 className="w-4 h-4" /> Concluir sem saldo
          </Button>
        )}
        {actions?.canCancel && (
          <Button
            variant="outline"
            size="sm"
            onClick={actions.onCancel}
            loading={actions.cancelling}
            loadingText="Cancelando..."
            className="text-status-critical border-status-critical/40 hover:bg-status-critical-soft"
          >
            <XCircle className="w-4 h-4" /> Cancelar
          </Button>
        )}
        {actions?.canReverse && (
          <Button
            variant="outline"
            size="sm"
            onClick={actions.onReverse}
            loading={actions.reversing}
            loadingText="Estornando..."
            className="text-status-critical border-status-critical/40 hover:bg-status-critical-soft"
          >
            <XCircle className="w-4 h-4" /> Estornar
          </Button>
        )}
        {!hasInlineActions && onNavigate && (
          <Button variant="default" size="sm" onClick={() => onNavigate(lancamento)}>
            <ExternalLink className="w-4 h-4" /> Abrir módulo de origem
          </Button>
        )}
      </div>
    </div>
  );
}

function Fact({
  icon: Icon,
  label,
  value,
  accent,
}: {
  icon: typeof DollarSign;
  label: string;
  value: React.ReactNode;
  accent?: "ok" | "critical";
}) {
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

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <h3 className="text-xs font-bold uppercase tracking-wider text-muted-foreground mb-2">{title}</h3>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">{children}</div>
    </div>
  );
}
