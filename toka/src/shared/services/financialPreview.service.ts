import api from "./api";

export interface ChargePreview {
  source_event_id: string;
  type: string;
  description: string;
  amount_cents: number;
  due_date: string;
  reference_month: string;
}

export interface CommissionPreview {
  broker_id: string;
  role: string;
  percentage: number;
  amount_cents: number;
  /** Economic remainder retained by the agency; never a broker payable. */
  retained?: boolean;
}

export interface TransferPreview {
  reference_month: string;
  gross_cents: number;
  admin_fee_pct: number;
  admin_fee_cents: number;
  intermediation_cents: number;
  commission_funded_cents: number;
  agency_intermediation_cents: number;
  intermediation_carry_cents: number;
  net_cents: number;
}

export interface EconomicCompositionPreview {
  flow: string;
  gross_cents: number;
  commission_pool_cents: number;
  commission_payable_cents: number;
  partner_payable_cents: number;
  agency_revenue_cents: number;
  admin_fee_cents?: number;
  intermediation_cents?: number;
  commission_funded_cents?: number;
  agency_retained_cents: number;
  owner_payable_cents: number;
  reserved_cents: number;
  available_cents: number;
  deductions_cents: number;
}

export interface FinancialPreview {
  partnership_fees?: Array<{
    kind: string;
    label: string;
    amount_cents: number;
    payer_tenant: string;
    payee_tenant: string;
    direction: "payable" | "receivable";
  }>;
  deal_id: string;
  contract_id?: string;
  deal_type: string;
  charges: ChargePreview[];
  commissions: CommissionPreview[];
  transfers?: TransferPreview[];
  composition?: EconomicCompositionPreview;
  total_charges_cents: number;
  total_commission_cents: number;
  total_transfers_cents: number;
  gateway_disabled: boolean;
  notes?: string[];
}

class FinancialPreviewService {
  private readonly baseUrl = "/financial-preview";

  /**
   * Projeção a partir do negócio.
   *
   * `dates` carrega o plano pré-contrato da locação (início/prazo): antes de a
   * minuta existir, o negócio não tem término, e sem as datas o backend assume
   * o prazo padrão de 30 meses — um calendário diferente do que o operador
   * configurou (pró-rata do primeiro mês e residual do último).
   */
  async getByDeal(dealId: string, dates?: { startDate?: string; endDate?: string }): Promise<FinancialPreview> {
    const response = await api.get<{ data: FinancialPreview }>(
      `${this.baseUrl}/deals/${dealId}`,
      {
        params: {
          start_date: dates?.startDate || undefined,
          end_date: dates?.endDate || undefined,
        },
      },
    );
    return response.data.data;
  }

  async getByContract(contractId: string): Promise<FinancialPreview> {
    const response = await api.get<{ data: FinancialPreview }>(
      `${this.baseUrl}/contracts/${contractId}`,
    );
    return response.data.data;
  }
}

export const financialPreviewService = new FinancialPreviewService();
