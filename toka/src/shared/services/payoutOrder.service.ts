import api, { unwrapApiData } from './api';

export type PayoutStatus =
  | 'DRAFT'
  | 'PENDING_REVIEW'
  | 'APPROVED'
  | 'PAID'
  | 'FAILED'
  | 'CANCELLED';

export type PayoutSourceType = 'charge' | 'manual' | 'royalty_settlement';
export type PayoutRecipientType =
  | 'broker'
  | 'partner_tenant'
  | 'royalty_payee'
  | 'external';

export type PayoutSettlementMethod =
  | 'MANUAL_PIX'
  | 'MANUAL_TED'
  | 'OTHER';

export interface PayoutFundingEntry {
  payment_id: string;
  charge_id: string;
  reference_month: string;
  amount_cents: number;
  funded_at: string;
}

/** Retrato da comissão que originou a ordem: bruto → descontos → líquido. */
export interface PayoutOrderCommissionDeduction {
  id?: string;
  kind: 'IRRF' | 'INSS' | 'ISS' | 'OUTROS' | 'ADVANCE' | string;
  description?: string;
  amount_cents: number;
  rate_pct?: number;
  base_cents?: number;
}

export interface PayoutOrderCommission {
  commission_id: string;
  broker_id?: string;
  role?: string;
  gross_cents: number;
  net_cents: number;
  deductions?: PayoutOrderCommissionDeduction[];
}

export interface PayoutOrder {
  id: string;
  tenant_id: string;
  source_type: PayoutSourceType;
  source_id?: string;
  source_ref?: Record<string, any>;
  contract_id?: string;
  recipient_type: PayoutRecipientType;
  recipient_id?: string;
  recipient_label?: string;
  recipient_pix_key?: string;
  amount_cents: number;
  funding_required_cents?: number;
  funded_amount_cents?: number;
  funding_entries?: PayoutFundingEntry[];
  currency: string;
  status: PayoutStatus;
  settlement_method?: PayoutSettlementMethod;
  external_id?: string;
  proof_url?: string;
  proof_filename?: string;
  group_id?: string;
  requested_by?: string;
  approved_by?: string;
  approved_at?: string;
  settled_at?: string;
  history?: Array<{ at: string; action: string; actor?: string; note?: string }>;
  metadata?: Record<string, any>;
  created_at: string;
  updated_at: string;
  /** Comissão de origem (bruto → descontos → líquido), quando houver. */
  commission?: PayoutOrderCommission;
}

export interface PayoutListFilter {
  status?: PayoutStatus | '';
  source_type?: PayoutSourceType | '';
  source_id?: string;
  contract_id?: string;
  recipient_type?: PayoutRecipientType | '';
  recipient_id?: string;
  group_id?: string;
  limit?: number;
  offset?: number;
}

export interface MarkPaidInput {
  method: PayoutSettlementMethod;
  proof_url: string;
  proof_filename?: string;
  note?: string;
}

export const payoutOrderService = {
  list: async (
    filter: PayoutListFilter = {},
  ): Promise<{ items: PayoutOrder[]; total?: number }> => {
    const params = new URLSearchParams();
    Object.entries(filter).forEach(([k, v]) => {
      if (v !== undefined && v !== null && v !== '') params.append(k, String(v));
    });
    const qs = params.toString();
    const response = await api.get<any>(`/payout-orders${qs ? `?${qs}` : ''}`);
    const body = response.data ?? response;
    return {
      items: body?.data ?? [],
      total: body?.meta?.total ?? body?.total,
    };
  },

  getById: async (id: string): Promise<PayoutOrder> => {
    const response = await api.get<any>(`/payout-orders/${id}`);
    const order = unwrapApiData<PayoutOrder>(response) as PayoutOrder & { commission?: PayoutOrderCommission | null };
    // O detalhe traz a comissão de origem fora do corpo da ordem (ela é um
    // agregado próprio): anexamos para a tela mostrar bruto → descontos → líquido.
    if (order && response?.data && 'commission' in response.data) {
      order.commission = response.data.commission ?? undefined;
    }
    return order;
  },

  approve: async (id: string): Promise<PayoutOrder> => {
    const response = await api.post<any>(`/payout-orders/${id}/approve`);
    return unwrapApiData<PayoutOrder>(response);
  },

  markPaid: async (id: string, input: MarkPaidInput): Promise<PayoutOrder> => {
    const response = await api.post<any>(`/payout-orders/${id}/mark-paid`, input);
    return unwrapApiData<PayoutOrder>(response);
  },

  cancel: async (id: string, reason?: string): Promise<PayoutOrder> => {
    const response = await api.post<any>(`/payout-orders/${id}/cancel`, { reason });
    return unwrapApiData<PayoutOrder>(response);
  },

  // Regerar: a ordem cancelada vira histórico e uma nova nasce pronta para
  // aprovar/lançar, com a comissão (e o lançamento) reabertos no servidor.
  regenerate: async (id: string): Promise<PayoutOrder> => {
    const response = await api.post<any>(`/payout-orders/${id}/regenerate`);
    return unwrapApiData<PayoutOrder>(response);
  },

  group: async (
    ids: string[],
  ): Promise<{ group_id: string; updated: PayoutOrder[] }> => {
    const response = await api.post<any>(`/payout-orders/group`, { ids });
    const body = response.data ?? response;
    return {
      group_id: body?.group_id ?? '',
      updated: body?.data ?? [],
    };
  },

  recomputeForCharge: async (
    chargeId: string,
  ): Promise<{ cancelled: number }> => {
    const response = await api.post<any>(
      `/payout-orders/charges/${chargeId}/recompute`,
    );
    const body = response.data ?? response;
    return { cancelled: body?.cancelled ?? 0 };
  },
};
