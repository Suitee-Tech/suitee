import api from "./api";

const newIdempotencyKey = () => {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `charge-${Date.now()}-${Math.random().toString(36).slice(2)}`;
};

export interface ChargeLineItem {
  id?: string;
  description: string;
  amount_cents: number;
  source_type?: string;
  source_id?: string;
  economic_owner_id?: string;
  settlement_mode?: ChargeSettlementMode;
  rule_version_id?: string;
  allocation_type?: ChargeAllocationType;
}

export type ChargeSettlementMode =
  | "arrecadacao"
  | "liquidacao_direta"
  | "reembolso"
  | "compensacao";

export type ChargeAllocationType =
  | "revenue"
  | "owner_payable"
  | "commission_payable"
  | "partner_payable"
  | "royalty_payable"
  | "tax_withheld"
  // Custo de terceiro adiantado pela imobiliária (manutenção/vistoria): o
  // recebimento baixa o direito a receber, sem virar receita própria.
  | "receivable";

export interface ChargeAllocation {
  id: string;
  payment_id: string;
  charge_item_id: string;
  amount_cents: number;
  allocation_type: ChargeAllocationType;
  beneficiary_party_id: string;
  rule_version_id: string;
  purpose?: string;
  related_entity_id?: string;
}

export interface ChargePaymentEntry {
  id?: string;
  amount_cents: number;
  method: string;
  paid_at: string;
  allocation_method?: string;
  allocations?: ChargeAllocation[];
}

export interface Charge {
  id: string;
  tenant_id: string;
  /** Identidade determinística do fato econômico que também pode ter gerado
   *  um espelho em /transactions. Nunca substituir por heurística. */
  source_event_id?: string;
  contract_id: string;
  tenant_party_id: string;
  owner_id: string;
  property_id: string;
  type: string;
  description: string;
  reference_month: string;
  amount: number;
  late_fee: number;
  interest: number;
  discount: number;
  total_amount: number;
  due_date: string;
  /**
   * Dias de atraso em DIAS CIVIS no fuso do produto (`Charge.OverdueDays` no
   * backend): a cobrança vence no dia e só atrasa no dia seguinte, então 0
   * significa "vence hoje ou ainda vai vencer" — nunca "venceu".
   *
   * Opcional de propósito: as listagens (`GET /charges`, `GET /charges/overdue`)
   * devolvem o campo na RESPOSTA; payload antigo (ou lista mockada) não traz e a
   * tela deriva do `due_date` sem quebrar (ver `utils/chargeDueLabel.ts`).
   */
  days_overdue?: number;
  /**
   * Nome do inquilino resolvido pelo backend via CRM nas LISTAGENS
   * (`GET /charges`, `GET /charges/overdue`).
   *
   * Opcional de propósito e NUNCA garantido: a resolução tem limite de busca e
   * falha em silêncio, então o item pode simplesmente não trazer o campo. A tela
   * mostra o nome quando vem e cai no `tenant_party_id` quando não vem — nunca
   * inventa um nome.
   */
  debtor_name?: string;
  paid_at?: string;
  paid_amount?: number;
  payment_method?: string;
  payments?: ChargePaymentEntry[];
  status: string;
  line_items?: ChargeLineItem[];
  reminders_sent?: ChargeReminderEntry[];
  created_at: string;
  updated_at: string;
}

export interface ChargeReminderEntry {
  sent_at: string;
  channels: string[];
  recipient?: string;
  sent_by?: string;
  sent_by_label?: string;
}

export interface CreateChargeDTO {
  contract_id: string;
  tenant_party_id: string;
  owner_id: string;
  property_id: string;
  type: string;
  description: string;
  reference_month: string;
  amount: number;
  due_date: string;
  discount?: number;
  line_items?: ChargeLineItem[];
  deal_id?: string;
  lead_id?: string;
}

export interface UpdateChargeDTO extends Partial<CreateChargeDTO> {
  late_fee?: number;
  interest?: number;
}

export interface MarkPaidDTO {
  payment_method: string;
  paid_amount: number;
  paid_at?: string;
  proof_url?: string;
  note?: string;
}

export interface SecurityDepositBalance {
  contract_id: string;
  held: number;
  deductions_pending: number;
  deductions_charged: number;
  return_due: number;
  deductions_count: number;
}

export interface NotifyChargeResult {
  charge_id: string;
  recipient?: string;
  channels_sent: string[];
  skipped?: string[];
}

class ChargeService {
  private readonly baseUrl = "/charges";

  async getAll(params?: {
    contract_id?: string;
    deal_id?: string;
    lead_id?: string;
    status?: string;
    reference_month?: string;
    /** Filtro de vencimento (RFC3339 ou yyyy-mm-dd) — usado no acerto de rescisão. */
    due_from?: string;
    due_to?: string;
    limit?: number;
    offset?: number;
  }) {
    const response = await api.get<{ data: Charge[]; meta: any }>(this.baseUrl, { params });
    return response.data;
  }

  async getById(id: string) {
    const response = await api.get<{ data: Charge }>(`${this.baseUrl}/${id}`);
    return response.data;
  }

  async create(data: CreateChargeDTO) {
    const response = await api.post<{ data: Charge }>(this.baseUrl, data);
    return response.data;
  }

  async update(id: string, data: UpdateChargeDTO) {
    const response = await api.put<{ data: Charge }>(`${this.baseUrl}/${id}`, data);
    return response.data;
  }

  async markPaid(id: string, data: MarkPaidDTO) {
    const hasEvidence = !!(data.proof_url || data.note || data.paid_at);
    if (hasEvidence) {
      return this.reconcileManual(id, data);
    }
    const response = await api.post<{ data: Charge }>(`${this.baseUrl}/${id}/pay`, data, {
      headers: { "Idempotency-Key": newIdempotencyKey() },
    });
    return response.data;
  }

  async reconcileManual(id: string, data: MarkPaidDTO) {
    const body = {
      paid_at: data.paid_at || new Date().toISOString(),
      paid_amount: data.paid_amount,
      payment_method: data.payment_method,
      proof_url: data.proof_url || undefined,
      note: data.note || undefined,
    };
    const response = await api.post<{ data: Charge }>(`${this.baseUrl}/${id}/reconcile-manual`, body, {
      headers: { "Idempotency-Key": newIdempotencyKey() },
    });
    return response.data;
  }

  async cancel(id: string, reason: string) {
    const response = await api.post<{ data: Charge }>(`${this.baseUrl}/${id}/cancel`, { reason });
    return response.data;
  }

  async getOverdue() {
    const response = await api.get<{ data: Charge[] }>(`${this.baseUrl}/overdue`);
    return response.data;
  }

  async notify(id: string) {
    const response = await api.post<{ data: NotifyChargeResult }>(`${this.baseUrl}/${id}/notify`, {});
    return response.data;
  }

  /**
   * Repara as alocações de uma cobrança de locação JÁ QUITADA antes de o plano
   * de intermediação existir: re-deriva a divisão (financiando a comissão do
   * corretor e o payout), estorna/regrava o lançamento do recebimento e
   * reconcilia o repasse do proprietário. Idempotente — se já estiver correto,
   * o backend responde com skipped_reason.
   */
  /**
   * Recibo oficial em PDF gerado pelo backend (agora com o encoding cp1252
   * correto). Só existe para cobrança quitada — o handler responde 422 para os
   * demais status. O caller transforma o blob em download.
   */
  async downloadReceipt(id: string): Promise<Blob> {
    const response = await api.get(`${this.baseUrl}/${id}/receipt`, { responseType: "blob" });
    return response.data as Blob;
  }

  /** Consulta read-only: só oferece o botão quando há de fato o que reparar. */
  async getRepairRentalAllocationsEligibility(id: string) {
    const response = await api.get<{ data: RepairRentalAllocationsEligibility }>(
      `${this.baseUrl}/${id}/repair-rental-allocations/eligibility`,
    );
    return response.data;
  }

  async repairRentalAllocations(id: string, reason: string) {
    const response = await api.post<{ data: RepairRentalAllocationsResult }>(
      `${this.baseUrl}/${id}/repair-rental-allocations`,
      { reason },
    );
    return response.data;
  }

  async getSecurityDepositBalance(contractId: string, held: number) {
    const response = await api.get<{ data: SecurityDepositBalance }>(`/contracts/${contractId}/security-deposit-balance`, {
      params: { held },
    });
    return response.data;
  }
}

/** Elegibilidade do reparo (read-only) — gate do botão no painel. */
export interface RepairRentalAllocationsEligibility {
  charge_id: string;
  eligible: boolean;
  reason?: string;
  pending_payments: number;
  outstanding_cents: number;
  planned_cents: number;
  funded_cents: number;
}

/** Resultado do reparo de alocação da intermediação de uma locação. */
export interface RepairRentalAllocationsResult {
  charge_id: string;
  repaired_payments: string[] | null;
  intermediation_funded_cents: number;
  journal_entries_reversed: number;
  journal_entries_reposted: number;
  transfer_reconciled: boolean;
  skipped_reason?: string;
  warnings?: string[];
}

export const chargeService = new ChargeService();
