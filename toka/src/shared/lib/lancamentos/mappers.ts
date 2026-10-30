import type { Charge } from "../../services/charge.service";
import type { Commission } from "../../services/commission.service";
import type { PayoutOrder } from "../../services/payoutOrder.service";
import type { OwnerTransfer } from "../../services/transfer.service";
import type { TransactionDTO } from "../../services/transaction.service";

import { hasDivergence } from "./divergence";
import { reconcileCommissionLedgerTransactions } from "./reconcile";
import { normalizeStatusOrDefault } from "./status";
import type {
  ChargeLancamento,
  CommissionLancamento,
  CounterpartyKind,
  Lancamento,
  RoyaltyLancamento,
  TransactionLancamento,
  TransferLancamento,
} from "./types";

export function chargeToLancamento(charge: Charge): ChargeLancamento {
  const status = normalizeStatusOrDefault("charge", charge.status);
  const lancamento: ChargeLancamento = {
    id: charge.id,
    kind: "charge",
    status,
    amount: charge.total_amount ?? charge.amount,
    net_amount: charge.paid_amount,
    currency: "BRL",
    due_at: charge.due_date,
    settled_at: charge.paid_at,
    counterparty: {
      kind: charge.tenant_party_id ? "tenant" : "desconhecido",
      id: charge.tenant_party_id,
      label: charge.description || "Cobrança",
    },
    origin: {
      entity_kind: "contract",
      entity_id: charge.contract_id,
      label: charge.reference_month ? `Contrato · ${charge.reference_month}` : "Contrato",
    },
    timeline_summary: {
      last_event_at: charge.updated_at,
      last_event_label: status === "pago" ? "Paga" : undefined,
    },
    permissions_required: ["finance:read"],
    created_at: charge.created_at,
    updated_at: charge.updated_at,
    kind_data: {
      payment_method: charge.payment_method,
      late_fee: charge.late_fee,
      interest: charge.interest,
      discount: charge.discount,
      paid_amount: charge.paid_amount,
      line_items: charge.line_items?.map((li) => ({
        id: li.id,
        description: li.description,
        amount_cents: li.amount_cents,
        source_type: li.source_type,
        source_id: li.source_id,
        economic_owner_id: li.economic_owner_id,
        settlement_mode: li.settlement_mode,
        rule_version_id: li.rule_version_id,
        allocation_type: li.allocation_type,
      })),
      reminders_count: charge.reminders_sent?.length ?? 0,
    },
    source: charge,
  };
  lancamento.timeline_summary.divergence_flag = hasDivergence(lancamento);
  return lancamento;
}

export function commissionToLancamento(
  commission: Commission,
  options: { linkedOperationId?: string } = {},
): CommissionLancamento {
  const status = normalizeStatusOrDefault("commission", commission.status);
  const deductionsTotal = commission.deductions?.reduce((sum, d) => sum + (d.amount || 0), 0) ?? 0;
  const lancamento: CommissionLancamento = {
    id: commission.id,
    kind: "commission",
    status,
    amount: commission.amount,
    net_amount: commission.net_amount,
    currency: commission.currency || "BRL",
    settled_at: commission.paid_at,
    counterparty: {
      kind: "broker",
      id: commission.broker_id,
      label: commission.role ? `Corretor · ${commission.role}` : "Corretor",
    },
    origin: {
      entity_kind: "deal",
      entity_id: commission.deal_id,
      label: commission.contract_id ? `Negócio · contrato vinculado` : "Negócio",
    },
    timeline_summary: {
      last_event_at: commission.updated_at,
      last_event_label:
        status === "pago" ? "Paga" : status === "aprovado" ? "Aprovada" : status === "cancelado" ? "Cancelada" : undefined,
    },
    permissions_required: ["finance:read"],
    created_at: commission.created_at,
    updated_at: commission.updated_at,
    kind_data: {
      gross_amount: commission.gross_amount,
      deductions_total: deductionsTotal,
      role: commission.role,
      participant_id: commission.broker_id,
      split_origin: "desconhecido",
      cancelled_at: commission.cancelled_at,
      proof_url: commission.proof_url,
      funding_required_cents: commission.funding_required_cents,
      funded_amount_cents: commission.funded_amount_cents,
      funding_outstanding_cents: Math.max(
        0,
        (commission.funding_required_cents ?? 0) - (commission.funded_amount_cents ?? 0),
      ),
      funding_entries: commission.funding_entries,
      ledger_transaction_id: commission.ledger_transaction_id,
      linked_operation_id: options.linkedOperationId,
    },
    source: commission,
  };
  lancamento.timeline_summary.divergence_flag = hasDivergence(lancamento);
  return lancamento;
}

export function transferToLancamento(transfer: OwnerTransfer): TransferLancamento {
  const status = normalizeStatusOrDefault("transfer", transfer.status);
  const lancamento: TransferLancamento = {
    id: transfer.id,
    kind: "transfer",
    status,
    // A lista unificada representa a obrigação financeira que efetivamente
    // será paga. O valor bruto continua disponível no detalhamento para
    // explicar a composição (taxas, descontos e demais retenções).
    amount: transfer.net_amount,
    net_amount: transfer.net_amount,
    currency: "BRL",
    settled_at: transfer.processed_at,
    counterparty: {
      kind: "owner",
      id: transfer.owner_id,
      label: "Proprietário",
    },
    origin: {
      entity_kind: "contract",
      entity_id: transfer.contract_id,
      label: transfer.reference_month
        ? `${transfer.settlement_type === "sale" || transfer.economic_breakdown?.flow === "sale" ? "Venda" : "Locação"} · ${transfer.reference_month}`
        : transfer.settlement_type === "sale" || transfer.economic_breakdown?.flow === "sale" ? "Venda" : "Locação",
    },
    timeline_summary: {
      last_event_at: transfer.updated_at,
      last_event_label: lastTransferAction(transfer),
    },
    permissions_required: ["finance:read"],
    created_at: transfer.created_at,
    updated_at: transfer.updated_at,
    kind_data: {
      admin_fee_amount: transfer.admin_fee_amount,
      admin_fee_percent: transfer.admin_fee_percent,
      maintenance_deductions: transfer.maintenance_deductions,
      other_deductions: transfer.other_deductions,
      gross_rent: transfer.gross_rent,
      approved_at: transfer.approved_at,
      processed_at: transfer.processed_at,
      history_count: transfer.history?.length ?? 0,
      settlement_type: transfer.settlement_type,
      economic_breakdown: transfer.economic_breakdown,
    },
    source: transfer,
  };
  lancamento.timeline_summary.divergence_flag = hasDivergence(lancamento);
  return lancamento;
}

export function payoutOrderToRoyaltyLancamento(payout: PayoutOrder): RoyaltyLancamento | null {
  if (payout.source_type !== "royalty_settlement") return null;
  const status = normalizeStatusOrDefault("royalty", payout.status);
  const lancamento: RoyaltyLancamento = {
    id: payout.id,
    kind: "royalty",
    status,
    amount: payout.amount_cents / 100,
    currency: payout.currency || "BRL",
    settled_at: payout.settled_at,
    counterparty: {
      kind: "partner",
      id: payout.recipient_id,
      label: payout.recipient_label || "Parceiro",
    },
    origin: {
      entity_kind: "partnership",
      entity_id: payout.source_id,
      label: payout.source_ref?.obligation_kind === "admin_fee" ? "Taxa administrativa da parceria" : payout.source_ref?.obligation_kind === "transaction_fee" ? "Taxa por negócio da parceria" : "Royalty de franquia",
    },
    timeline_summary: {
      last_event_at: payout.updated_at,
      last_event_label: payout.history?.[payout.history.length - 1]?.action,
    },
    permissions_required: ["finance:read", "partnerships:read"],
    created_at: payout.created_at,
    updated_at: payout.updated_at,
    kind_data: {
      source_deal_id: payout.source_ref?.deal_id,
      obligation_kind: payout.source_ref?.obligation_kind,
      recipient_label: payout.recipient_label,
      proof_url: payout.proof_url,
    },
    source: payout,
  };
  lancamento.timeline_summary.divergence_flag = hasDivergence(lancamento);
  return lancamento;
}

const PAYER_KINDS: CounterpartyKind[] = ["tenant", "owner", "broker", "partner", "external"];

export function transactionToLancamento(tx: TransactionDTO): TransactionLancamento {
  const status = normalizeStatusOrDefault("transaction", tx.status);
  const direction: "entrada" | "saida" = tx.type === "receita" ? "entrada" : "saida";
  const manual = !tx.contract_id;
  const payerKind: CounterpartyKind =
    tx.payer_kind && PAYER_KINDS.includes(tx.payer_kind as CounterpartyKind) ? (tx.payer_kind as CounterpartyKind) : "desconhecido";
  const counterpartyLabel = tx.payer_name || tx.category || tx.description || (direction === "entrada" ? "Entrada" : "Saída");
  const lancamento: TransactionLancamento = {
    id: tx.id,
    kind: "transaction",
    status,
    amount: tx.value,
    currency: "BRL",
    due_at: tx.due_date,
    settled_at: tx.payment_date ?? undefined,
    counterparty: {
      kind: payerKind,
      // payer_name is a display value, not an entity identifier. A canonical
      // ID is attached later only when the presentation layer resolves the
      // party against leads/corretores already carregados.
      id: undefined,
      label: counterpartyLabel,
    },
    origin: manual
      ? { entity_kind: "manual", label: "Lançamento avulso" }
      : { entity_kind: "contract", entity_id: tx.contract_id, label: tx.month ? `Contrato · ${tx.month}` : "Contrato" },
    timeline_summary: {
      last_event_at: tx.updated_at,
      last_event_label: status === "pago" ? "Baixado" : undefined,
    },
    permissions_required: ["finance:read"],
    created_at: tx.created_at,
    updated_at: tx.updated_at,
    kind_data: {
      transaction_type: tx.type,
      direction,
      category: tx.category,
      payment_method: tx.payment_method ?? undefined,
      payment_date: tx.payment_date ?? undefined,
      payment_plan: tx.payment_plan || undefined,
      contract_id: tx.contract_id || undefined,
      deal_id: tx.deal_id,
      economic_breakdown: tx.economic_breakdown,
      manual,
      proof_url: tx.proof_url ?? undefined,
      note: tx.note ?? undefined,
    },
    source: tx,
  };
  lancamento.timeline_summary.divergence_flag = hasDivergence(lancamento);
  return lancamento;
}

function lastTransferAction(transfer: OwnerTransfer): string | undefined {
  const last = transfer.history?.[transfer.history.length - 1];
  if (!last) return undefined;
  switch (last.action) {
    case "created":
      return "Criado";
    case "approved":
      return "Aprovado";
    case "processed":
      return "Processado";
    case "cancelled":
      return "Cancelado";
    default:
      return last.action;
  }
}

export function mapLancamentos(input: {
  charges?: Charge[];
  commissions?: Commission[];
  transfers?: OwnerTransfer[];
  payouts?: PayoutOrder[];
  transactions?: TransactionDTO[];
}): Lancamento[] {
  const out: Lancamento[] = [];
  const commissions = input.commissions ?? [];
  const sourceTransactions = input.transactions ?? [];
  const transactions = reconcileCommissionLedgerTransactions(commissions, sourceTransactions);

  input.charges?.forEach((c) => out.push(chargeToLancamento(c)));
  commissions.forEach((c) => out.push(commissionToLancamento(c, {
    linkedOperationId: linkedCommissionOperationID(c, sourceTransactions),
  })));
  input.transfers?.forEach((t) => out.push(transferToLancamento(t)));
  input.payouts?.forEach((p) => {
    const r = payoutOrderToRoyaltyLancamento(p);
    if (r) out.push(r);
  });
  transactions.forEach((t) => out.push(transactionToLancamento(t)));
  return out;
}

function linkedCommissionOperationID(commission: Commission, transactions: TransactionDTO[]): string | undefined {
  const explicitLedgerID = commission.ledger_transaction_id?.trim();
  if (explicitLedgerID) {
    const explicit = transactions.find((transaction) => transaction.id === explicitLedgerID);
    if (explicit?.operation_id?.trim()) return explicit.operation_id.trim();
  }

  const role = commission.role?.trim().toLocaleLowerCase("pt-BR");
  const candidates = transactions.filter((transaction) => (
    transaction.type === "comissao" &&
    !!transaction.operation_id?.trim() &&
    transaction.contract_id === commission.contract_id &&
    transaction.broker_id === commission.broker_id &&
    transaction.role?.trim().toLocaleLowerCase("pt-BR") === role &&
    Math.round(transaction.value * 100) === Math.round(commission.amount * 100)
  ));
  return candidates.length === 1 ? candidates[0].operation_id?.trim() : undefined;
}
