import api from "./api";

export type CommissionStatus = "PENDING" | "APPROVED" | "PAID" | "REVERSED" | "CANCELLED";
export type CommissionRole = "captador" | "co_captador" | "visitador" | "atendente" | "fechamento" | "imobiliaria";
/**
 * `ADVANCE` não é tributo: é o abatimento do adiantamento que a imobiliária fez
 * ao corretor. O backend consome o saldo do adiantamento mais antigo para o mais
 * novo e RECUSA (400) o lançamento que exceder o saldo. Sempre valor fixo —
 * `rate_pct`/`base_cents` não se aplicam.
 */
export type DeductionKind = "IRRF" | "INSS" | "ISS" | "OUTROS" | "ADVANCE";

export interface Deduction {
  id?: string;
  kind: DeductionKind;
  description?: string;
  amount: number;
  /**
   * Alíquota usada quando o lançamento é por percentual (ex.: 20 = INSS de
   * autônomo, 2.5 = ISS municipal). Sem ela a auditoria não distingue um valor
   * digitado de um valor calculado.
   */
  rate_pct?: number;
  /**
   * Base de cálculo em centavos (bruto da comissão) quando o lançamento é por
   * percentual. O backend valida que `amount` fecha com
   * `base_cents * rate_pct / 100` (±1 centavo).
   */
  base_cents?: number;
}

export interface CommissionFundingEntry {
  payment_id: string;
  charge_id: string;
  reference_month: string;
  amount_cents: number;
  funded_at: string;
}

export interface Commission {
  id: string;
  deal_id: string;
  broker_id: string;
  role?: string;
  percentage?: number;
  amount: number;
  gross_amount?: number;
  deductions?: Deduction[];
  net_amount?: number;
  currency: string;
  status: CommissionStatus;
  contract_id?: string;
  payout_order_id?: string;
  /** Exact accounting mirror created with a manual posting. */
  ledger_transaction_id?: string;
  source_event_id?: string;
  funding_required_cents?: number;
  funded_amount_cents?: number;
  funding_entries?: CommissionFundingEntry[];
  proof_url?: string;
  note?: string;
  created_at: string;
  updated_at: string;
  paid_at?: string;
  cancelled_at?: string;
  tenant_id: string;
}

export interface CreateCommissionInput {
  deal_id: string;
  broker_id: string;
  amount: number;
  /** ID único para idempotência — evita duplicação em caso de retry */
  source_event_id?: string;
}

export interface CommissionSplitInput {
  broker_id: string;
  role?: string;
  percentage?: number;
  amount: number;
}

export interface CreateCommissionBatchInput {
  deal_id: string;
  contract_id?: string;
  splits: CommissionSplitInput[];
}

class CommissionService {
  private readonly baseUrl = "/commissions";

  async list(limit = 50, offset = 0): Promise<{ data: Commission[]; total?: number }> {
    const response = await api.get<{ data: Commission[]; meta: { total: number } }>(this.baseUrl, {
      params: { limit, offset },
    });
    return {
      data: response.data?.data || [],
      total: response.data?.meta?.total,
    };
  }

  async getById(id: string): Promise<Commission> {
    const response = await api.get<{ data: Commission }>(`${this.baseUrl}/${id}`);
    return response.data?.data;
  }

  async create(input: CreateCommissionInput): Promise<Commission> {
    const res = await api.post<{ data: Commission }>(this.baseUrl, input);
    return res.data?.data;
  }

  async createBatch(input: CreateCommissionBatchInput, idempotencyKey?: string): Promise<Commission[]> {
    const headers: Record<string, string> = {};
    if (idempotencyKey) headers["X-Idempotency-Key"] = idempotencyKey;
    const res = await api.post<{ data: Commission[] }>(`${this.baseUrl}/batch`, input, { headers });
    return res.data?.data || [];
  }

  async listByDeal(dealId: string): Promise<Commission[]> {
    const res = await api.get<{ data: Commission[] }>(`${this.baseUrl}/by-deal/${dealId}`);
    return res.data?.data || [];
  }

  async listByBroker(brokerId: string, limit = 50, offset = 0): Promise<{ data: Commission[]; total?: number }> {
    const res = await api.get<{ data: Commission[]; meta: { total: number } }>(
      `${this.baseUrl}/by-broker/${brokerId}`,
      { params: { limit, offset } },
    );
    return { data: res.data?.data || [], total: res.data?.meta?.total };
  }

  async approve(id: string): Promise<Commission> {
    const res = await api.post<{ data: Commission }>(`${this.baseUrl}/${id}/approve`);
    return res.data?.data;
  }

  async pay(id: string, body?: { proof_url?: string; note?: string }): Promise<Commission> {
    const res = await api.post<{ data: Commission }>(`${this.baseUrl}/${id}/pay`, body || {});
    return res.data?.data;
  }

  async cancel(id: string, reason?: string): Promise<Commission> {
    const res = await api.post<{ data: Commission }>(`${this.baseUrl}/${id}/cancel`, { reason });
    return res.data?.data;
  }

  async reverse(id: string, reason?: string): Promise<Commission> {
    const res = await api.post<{ data: Commission }>(`${this.baseUrl}/${id}/reverse`, { reason });
    return res.data?.data;
  }

  async addDeduction(id: string, deduction: Deduction & { deduction_id?: string }): Promise<Commission> {
    const res = await api.post<{ data: Commission }>(`${this.baseUrl}/${id}/deductions`, deduction);
    return res.data?.data;
  }

  async removeDeduction(id: string, deductionId: string): Promise<Commission> {
    const res = await api.delete<{ data: Commission }>(
      `${this.baseUrl}/${id}/deductions/${encodeURIComponent(deductionId)}`,
    );
    return res.data?.data;
  }
}

export const commissionService = new CommissionService();
