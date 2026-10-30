import api from './api';

export type TransactionType = 'receita' | 'despesa' | 'repasse' | 'comissao';
// Canonical lifecycle enum (backend source of truth) plus the legacy Portuguese
// strings still produced by some read paths. New writes always use the
// canonical values; the legacy members remain for backward-compatible reads.
export type TransactionStatus =
  | 'PENDING'
  | 'PAID'
  | 'OVERDUE'
  | 'REVERSED'
  | 'CANCELLED'
  | 'Pendente'
  | 'Pago'
  | 'Em Atraso'
  | 'Estornado'
  | 'Cancelado';
export type PayerKind = 'tenant' | 'owner' | 'broker' | 'partner' | 'external';

export interface TransactionEconomicBreakdown {
  flow: 'sale' | 'rent' | string;
  gross_cents: number;
  commission_pool_cents: number;
  commission_payable_cents: number;
  commission_funded_cents?: number;
  commission_funding_known?: boolean;
  partner_payable_cents: number;
  agency_retained_cents: number;
  owner_payable_cents: number;
  discount_cents: number;
  /** Recurring administration revenue, separate from rental intermediation. */
  admin_fee_cents?: number;
}

export interface TransactionDTO {
 id: string;
 contract_id: string;
 type: TransactionType;
 description: string;
 value: number;
 date: string;
 status: TransactionStatus;
 due_date?: string;
 payment_date?: string | null;
 payment_method?: string | null;
 category?: string;
 payer_name?: string;
 payer_kind?: PayerKind | string;
 month?: string;
 gross_value?: number;
 admin_fee?: number;
 maintenance_deductions?: number;
 utility_deductions?: number;
 /** Plano de pagamentos programado na esteira (proveniência informacional). */
 payment_plan?: string;
 /** Comprovante da baixa (upload ou link) — mesmo padrão dos demais fluxos. */
 proof_url?: string | null;
 /** Observação do operador registrada na baixa. */
 note?: string | null;
 tenant_id: string;
 created_at: string;
 updated_at: string;
 /** Deterministic event key present on automatically generated ledger rows. */
 source_event_id?: string;
 /** Stable identifier shared by every line of one manual batch. */
 operation_id?: string;
 /** Stable unique identifier of this posting inside the batch. */
 posting_id?: string;
 /** Broker dimension carried by generated/manual commission ledger mirrors. */
 broker_id?: string;
 /** Financial role carried by generated/manual commission ledger mirrors. */
 role?: string;
 /** Hash of the commission plan that originated this posting. */
 plan_hash?: string;
 /** Manifest fields make an atomic manual posting inspectable. */
 batch_fingerprint?: string;
 batch_line_count?: number;
 commission_count?: number;
 deal_id?: string;
 economic_breakdown?: TransactionEconomicBreakdown;
}

export interface CreateTransactionDTO {
  contract_id: string;
  type: TransactionType;
  description: string;
  value: number;
  date: string;
  status?: TransactionStatus;
  due_date?: string;
  payment_date?: string;
  payment_method?: string;
  category?: string;
  payer_name?: string;
  payer_kind?: PayerKind | string;
  month?: string;
  gross_value?: number;
  admin_fee?: number;
  maintenance_deductions?: number;
  utility_deductions?: number;
  /** ID único para idempotência e rastreabilidade */
  source_event_id?: string;
}

export interface UpdateTransactionDTO {
 type?: TransactionType;
 description?: string;
 value?: number;
 date?: string;
 status?: TransactionStatus;
 due_date?: string;
 payment_date?: string | null;
 payment_method?: string | null;
 category?: string;
 payer_name?: string;
 payer_kind?: PayerKind | string;
 month?: string;
 gross_value?: number;
 admin_fee?: number;
 maintenance_deductions?: number;
 utility_deductions?: number;
}

export interface CreateTransactionBatchItem {
  contract_id: string;
  deal_id?: string;
  type: TransactionType;
  description: string;
  value: number;
  date: string;
  category?: string;
  payment_method?: string;
  broker_id?: string;
  role?: string;
  flow?: 'sale' | 'rent';
  /** Primeiro vencimento programado na esteira (YYYY-MM-DD). */
  due_date?: string;
  /** Plano de pagamentos programado na esteira (ex.: "Entrada + 11x de R$ 8.333,33"). */
  payment_plan?: string;
}

export interface CreateTransactionBatchResult {
  transaction: TransactionDTO;
  index: number;
  error?: string;
}

export interface ManualCommissionBatchItem {
  deal_id: string;
  contract_id?: string;
  broker_id: string;
  role?: string;
  percentage?: number;
  amount: number;
}

export interface ManualCommissionRetention {
  percentage: number;
  amount: number;
}

class TransactionService {
 private readonly baseUrl = '/transactions';

 async getAll(params?: { contract_id?: string; limit?: number; offset?: number }) {
 const response = await api.get<{ data: TransactionDTO[]; total?: number; meta?: { total?: number; limit?: number } }>(this.baseUrl, { params });
 return response.data;
 }

 async getById(id: string) {
 const response = await api.get<{ data: TransactionDTO }>(`${this.baseUrl}/${id}`);
 return response.data;
 }

 async create(data: CreateTransactionDTO) {
 const response = await api.post<{ data: TransactionDTO }>(this.baseUrl, data);
 return response.data;
 }

 async createBatch(
  items: CreateTransactionBatchItem[],
  operationId: string,
  commissions: ManualCommissionBatchItem[] = [],
  commissionRetention?: ManualCommissionRetention,
 ) {
  const response = await api.post<{ data: CreateTransactionBatchResult[] }>(
   `${this.baseUrl}/batch`,
   {
    items,
    ...(commissions.length > 0 ? { commissions } : {}),
    ...(commissionRetention ? { commission_retention: commissionRetention } : {}),
   },
   { headers: { "Idempotency-Key": operationId } },
  );
  return response.data;
 }

 async update(id: string, data: UpdateTransactionDTO) {
 const response = await api.put<{ data: TransactionDTO }>(`${this.baseUrl}/${id}`, data);
 return response.data;
 }

 async settle(id: string, data?: { payment_date?: string; payment_method?: string; proof_url?: string; note?: string }) {
 const response = await api.post<{ data: TransactionDTO }>(`${this.baseUrl}/${id}/settle`, data ?? {});
 return response.data;
 }

 /** Cancela um lançamento ainda não pago (PENDING/OVERDUE). */
 async cancel(id: string, reason?: string) {
 const response = await api.post<{ data: TransactionDTO }>(`${this.baseUrl}/${id}/cancel`, {
  reason: reason ?? "Cancelamento manual",
 });
 return response.data;
 }

 /** Estorna (reverte) um lançamento pago. */
 async reverse(id: string, reason: string) {
 const response = await api.post<{ data: TransactionDTO }>(`${this.baseUrl}/${id}/reverse`, { reason });
 return response.data;
 }

 async delete(id: string) {
 return api.delete(`${this.baseUrl}/${id}`);
 }
}

export const transactionService = new TransactionService();
