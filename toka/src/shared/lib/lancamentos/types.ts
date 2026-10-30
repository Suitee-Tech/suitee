export type LancamentoKind = "charge" | "commission" | "transfer" | "royalty" | "transaction";

export type LancamentoStatus = "pendente" | "aprovado" | "pago" | "processado" | "cancelado" | "vencido" | "parcial" | "revertido";

export type CounterpartyKind = "tenant" | "owner" | "broker" | "partner" | "external" | "desconhecido";

export interface Counterparty {
  kind: CounterpartyKind;
  id?: string;
  label: string;
}

export type OriginEntityKind = "contract" | "deal" | "lead" | "partnership" | "charge" | "manual";

export interface Origin {
  entity_kind: OriginEntityKind;
  entity_id?: string;
  label?: string;
}

export interface TimelineSummary {
  last_event_at?: string;
  last_event_label?: string;
  divergence_flag?: boolean;
}

interface LancamentoBase<K extends LancamentoKind, D> {
  id: string;
  kind: K;
  status: LancamentoStatus;
  amount: number;
  net_amount?: number;
  currency: string;
  due_at?: string;
  settled_at?: string;
  counterparty: Counterparty;
  origin: Origin;
  timeline_summary: TimelineSummary;
  /**
   * Contexto exclusivamente de apresentação. Mantém a linha contábil intacta,
   * mas permite reuni-la com as demais obrigações do mesmo evento econômico.
   */
  presentation?: {
    operation_key: string;
    operation_title: string;
    operation_subtitle?: string;
    contract_id?: string;
    property_title?: string;
    reference_month?: string;
    flow?: "sale" | "rent" | string;
  };
  permissions_required: string[];
  created_at: string;
  updated_at: string;
  kind_data: D;
  source: unknown;
}

export interface ChargeKindData {
  payment_method?: string;
  late_fee?: number;
  interest?: number;
  discount?: number;
  paid_amount?: number;
  line_items?: Array<{
    id?: string;
    description: string;
    amount_cents: number;
    source_type?: string;
    source_id?: string;
    economic_owner_id?: string;
    settlement_mode?: string;
    rule_version_id?: string;
    allocation_type?: string;
  }>;
  reminders_count: number;
}

export interface CommissionKindData {
  gross_amount?: number;
  deductions_total: number;
  role?: string;
  participant_id: string;
  split_origin: "modelo" | "legado" | "desconhecido";
  cancelled_at?: string;
  proof_url?: string;
  funding_required_cents?: number;
  funded_amount_cents?: number;
  funding_outstanding_cents?: number;
  funding_entries?: Array<{
    payment_id: string;
    charge_id: string;
    reference_month: string;
    amount_cents: number;
    funded_at: string;
  }>;
  ledger_transaction_id?: string;
  linked_operation_id?: string;
}

export interface TransferKindData {
  admin_fee_amount: number;
  admin_fee_percent: number;
  maintenance_deductions: number;
  other_deductions: number;
  gross_rent: number;
  approved_at?: string;
  processed_at?: string;
  history_count: number;
  settlement_type?: string;
  economic_breakdown?: {
    flow: string;
    gross_cents: number;
    owner_payable_cents: number;
    reserved_cents: number;
    available_cents: number;
    agency_revenue_cents: number;
    admin_fee_cents?: number;
    intermediation_revenue_cents?: number;
    commission_funded_cents?: number;
    commission_payable_cents: number;
    partner_payable_cents: number;
    agency_retained_cents: number;
    discount_cents: number;
    deductions_cents: number;
  };
}

export interface RoyaltyKindData {
  obligation_kind?: "royalty" | "admin_fee" | "transaction_fee";
  royalty_base?: "gross_commission" | "net_commission" | "revenue";
  source_deal_id?: string;
  recipient_label?: string;
  proof_url?: string;
}

export interface TransactionKindData {
  transaction_type: "receita" | "despesa" | "repasse" | "comissao";
  /** receita = entrada; everything else = saída. */
  direction: "entrada" | "saida";
  category?: string;
  payment_method?: string;
  payment_date?: string;
  /** Plano de pagamentos programado na esteira (proveniência informacional). */
  payment_plan?: string;
  contract_id?: string;
  deal_id?: string;
  economic_breakdown?: {
    flow: string;
    gross_cents: number;
    commission_pool_cents: number;
    admin_fee_cents?: number;
    commission_payable_cents: number;
    commission_funded_cents?: number;
    commission_funding_known?: boolean;
    partner_payable_cents: number;
    agency_retained_cents: number;
    owner_payable_cents: number;
    discount_cents: number;
  };
  /** True when this is an ad-hoc entry not tied to a contract. */
  manual: boolean;
  /** Comprovante da baixa (upload ou link) — padrão dos demais fluxos. */
  proof_url?: string;
  /** Observação do operador registrada na baixa. */
  note?: string;
}

export type ChargeLancamento = LancamentoBase<"charge", ChargeKindData>;
export type CommissionLancamento = LancamentoBase<"commission", CommissionKindData>;
export type TransferLancamento = LancamentoBase<"transfer", TransferKindData>;
export type RoyaltyLancamento = LancamentoBase<"royalty", RoyaltyKindData>;
export type TransactionLancamento = LancamentoBase<"transaction", TransactionKindData>;

export type Lancamento =
  | ChargeLancamento
  | CommissionLancamento
  | TransferLancamento
  | RoyaltyLancamento
  | TransactionLancamento;

export type LancamentoAction = "markPaid" | "approve" | "cancel" | "processTransfer" | "editDeductions" | "notify" | "viewReceipt" | "settle";
