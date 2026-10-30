import api, { unwrapApiData } from "./api";

export type SaleBillingPlanMode = "SINGLE" | "INSTALLMENTS";

export interface SaleBillingPlan {
  id: string;
  tenant_id: string;
  contract_id: string;
  deal_id?: string;
  buyer_party_id?: string;
  owner_id?: string;
  property_id?: string;
  mode: SaleBillingPlanMode;
  total_amount_cents: number;
  down_payment_cents: number;
  installment_count: number;
  installment_amount_cents: number;
  first_due_date: string;
  recurrence_interval_days: number;
  dacao_amount_cents?: number;
  dacao_description?: string;
  troca_amount_cents?: number;
  troca_description?: string;
  saldo_amount_cents?: number;
  saldo_due_date?: string;
  saldo_description?: string;
  product_revision: number;
  created_at: string;
  updated_at: string;
}

export interface CreateSaleBillingPlanInput {
  deal_id?: string;
  buyer_party_id?: string;
  owner_id?: string;
  property_id?: string;
  mode: SaleBillingPlanMode;
  total_amount: number;
  down_payment_amount?: number;
  installment_amount?: number;
  installment_count: number;
  first_due_date: string;
  recurrence_interval_days?: number;
  dacao_amount?: number;
  dacao_description?: string;
  troca_amount?: number;
  troca_description?: string;
  saldo_amount?: number;
  saldo_due_date?: string;
  saldo_description?: string;
}

export interface SaleBillingPlanDraft {
  id: string;
  tenant_id: string;
  deal_id: string;
  mode: SaleBillingPlanMode;
  total_amount_cents: number;
  down_payment_cents: number;
  installment_amount_cents: number;
  installment_count: number;
  first_due_date: string;
  recurrence_interval_days: number;
  composite: {
    dacao_amount_cents?: number;
    dacao_description?: string;
    troca_amount_cents?: number;
    troca_description?: string;
    saldo_amount_cents?: number;
    saldo_due_date?: string;
    saldo_description?: string;
  };
  created_by?: string;
  consumed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface UpsertDraftInput {
  mode: SaleBillingPlanMode;
  total_amount: number;
  down_payment_amount?: number;
  installment_amount?: number;
  installment_count: number;
  first_due_date: string;
  recurrence_interval_days?: number;
  dacao_amount?: number;
  dacao_description?: string;
  troca_amount?: number;
  troca_description?: string;
  saldo_amount?: number;
  saldo_due_date?: string;
  saldo_description?: string;
}

export const saleBillingPlanService = {
  create: async (contractId: string, input: CreateSaleBillingPlanInput): Promise<{ plan: SaleBillingPlan; charges_created: number }> => {
    const response = await api.post<any>(`/sale-billing-plans/contracts/${contractId}`, input);
    const data = unwrapApiData<{ data: SaleBillingPlan; charges_created: number } | any>(response) as any;
    return {
      plan: data?.data ?? data,
      charges_created: data?.charges_created ?? 0,
    };
  },

  getByContractId: async (contractId: string): Promise<SaleBillingPlan | null> => {
    try {
      const response = await api.get<any>(`/sale-billing-plans/contracts/${contractId}`);
      return unwrapApiData<SaleBillingPlan>(response);
    } catch (error: any) {
      if (error?.response?.status === 404) return null;
      throw error;
    }
  },

  upsertDraft: async (dealId: string, input: UpsertDraftInput): Promise<SaleBillingPlanDraft> => {
    const response = await api.post<{ data: SaleBillingPlanDraft }>(`/sale-billing-plans/drafts/deals/${dealId}`, input);
    return unwrapApiData<SaleBillingPlanDraft>(response);
  },

  getDraft: async (dealId: string): Promise<SaleBillingPlanDraft | null> => {
    try {
      const response = await api.get<{ data: SaleBillingPlanDraft }>(`/sale-billing-plans/drafts/deals/${dealId}`);
      return unwrapApiData<SaleBillingPlanDraft>(response);
    } catch (error: any) {
      if (error?.response?.status === 404) return null;
      throw error;
    }
  },
};
