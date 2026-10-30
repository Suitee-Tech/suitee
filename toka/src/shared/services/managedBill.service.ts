import api, { unwrapApiData } from "./api";

export type ManagedBillStatus =
  | "RESERVED"
  | "APPROVED"
  | "PAID"
  | "RECONCILED"
  | "CANCELLED";

export interface ManagedBillOwnerBalance {
  total_cents: number;
  reserved_cents: number;
  blocked_cents: number;
  available_cents: number;
}

export interface ManagedBillEvidence {
  kind: string;
  reference: string;
  actor?: string;
  at: string;
}

export interface ManagedBillHistoryEntry {
  action: string;
  actor: string;
  at: string;
  version: number;
  reference?: string;
  reason?: string;
}

export interface ManagedBillPaymentAttempt {
  id: string;
  payment_reference: string;
  payment_journal_source_id: string;
  paid_by: string;
  paid_at: string;
  reconciled_by?: string;
  reconciled_at?: string;
  reconciliation_reference?: string;
  reversal_reference: string;
  reversal_journal_source_id: string;
  reversal_proof_reference?: string;
  reversed_by: string;
  reversed_at: string;
  reversal_reason: string;
}

export interface ManagedBill {
  id: string;
  tenant_id: string;
  source_key: string;
  source_transfer_id: string;
  source_charge_id: string;
  charge_item_id: string;
  contract_id: string;
  property_id: string;
  owner_id: string;
  component: string;
  creditor_name: string;
  description: string;
  amount_cents: number;
  due_date: string;
  competence_month: string;
  status: ManagedBillStatus;
  version: number;
  reserved_by: string;
  reserved_at: string;
  approved_by?: string;
  paid_by?: string;
  reconciled_by?: string;
  cancelled_by?: string;
  payment_reference?: string;
  payment_journal_source_id?: string;
  reconciliation_reference?: string;
  cancellation_reason?: string;
  evidence?: ManagedBillEvidence[];
  history: ManagedBillHistoryEntry[];
  payment_attempts?: ManagedBillPaymentAttempt[];
  approved_at?: string;
  paid_at?: string;
  reconciled_at?: string;
  cancelled_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ManagedBillListParams {
  contract_id?: string;
  status?: ManagedBillStatus;
  search?: string;
  limit?: number;
  offset?: number;
}

export interface ManagedBillListResult {
  data: ManagedBill[];
  meta?: {
    total?: number;
    limit?: number;
    page?: number;
    totalPages?: number;
  };
}

export interface ManagedBillMutationResult {
  bill: ManagedBill;
  owner_balance_before?: ManagedBillOwnerBalance;
  owner_balance_after?: ManagedBillOwnerBalance;
  coverage_complete: boolean;
}

export interface ManagedBillVersionedCommand {
  expected_version: number;
}

export interface PayManagedBillDTO extends ManagedBillVersionedCommand {
  payment_reference: string;
  proof_url?: string;
}

export interface ReconcileManagedBillDTO extends ManagedBillVersionedCommand {
  reconciliation_reference: string;
}

export interface ReverseManagedBillPaymentDTO extends ManagedBillVersionedCommand {
  reversal_reference: string;
  reason: string;
  proof_url?: string;
}

export interface CancelManagedBillDTO extends ManagedBillVersionedCommand {
  reason: string;
}

class ManagedBillService {
  private readonly baseUrl = "/managed-bills";

  async list(params: ManagedBillListParams = {}): Promise<ManagedBillListResult> {
    const response = await api.get<{ data: ManagedBill[]; meta?: ManagedBillListResult["meta"] }>(
      this.baseUrl,
      { params },
    );
    return {
      data: response.data?.data ?? [],
      meta: response.data?.meta,
    };
  }

  async approve(id: string, data: ManagedBillVersionedCommand): Promise<ManagedBillMutationResult> {
    const response = await api.post<{ data: ManagedBillMutationResult }>(
      `${this.baseUrl}/${id}/approve`,
      data,
    );
    return unwrapApiData<ManagedBillMutationResult>(response);
  }

  async pay(id: string, data: PayManagedBillDTO): Promise<ManagedBillMutationResult> {
    const response = await api.post<{ data: ManagedBillMutationResult }>(
      `${this.baseUrl}/${id}/pay`,
      data,
    );
    return unwrapApiData<ManagedBillMutationResult>(response);
  }

  async reconcile(id: string, data: ReconcileManagedBillDTO): Promise<ManagedBillMutationResult> {
    const response = await api.post<{ data: ManagedBillMutationResult }>(
      `${this.baseUrl}/${id}/reconcile`,
      data,
    );
    return unwrapApiData<ManagedBillMutationResult>(response);
  }

  async reversePayment(id: string, data: ReverseManagedBillPaymentDTO): Promise<ManagedBillMutationResult> {
    const response = await api.post<{ data: ManagedBillMutationResult }>(
      `${this.baseUrl}/${id}/reverse-payment`,
      data,
    );
    return unwrapApiData<ManagedBillMutationResult>(response);
  }

  async cancel(id: string, data: CancelManagedBillDTO): Promise<ManagedBillMutationResult> {
    const response = await api.post<{ data: ManagedBillMutationResult }>(
      `${this.baseUrl}/${id}/cancel`,
      data,
    );
    return unwrapApiData<ManagedBillMutationResult>(response);
  }
}

export const managedBillService = new ManagedBillService();
