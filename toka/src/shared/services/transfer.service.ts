import api from './api';

export interface Deduction {
description: string;
amount: number;
type: string;
reference_id?: string;
}

export interface OwnerBalance {
  total_cents: number;
  reserved_cents: number;
  blocked_cents: number;
  available_cents: number;
}

export interface EconomicBreakdown {
  flow: 'rent' | 'sale' | string;
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
}

export interface OwnerTransfer {
id: string;
tenant_id: string;
source_charge_id?: string;
source_transaction_id?: string;
contract_id: string;
owner_id: string;
property_id: string;
reference_month: string;
settlement_type?: 'rent' | 'sale' | string;
gross_rent: number;
admin_fee_percent: number;
admin_fee_amount: number;
maintenance_deductions: number;
other_deductions: number;
deduction_details?: Deduction[];
net_amount: number;
status: string;
processed_at?: string;
approved_at?: string;
approved_by?: string;
processed_by?: string;
settlement_journal_source_id?: string;
// Marcador persistido pelo backend quando o repasse foi encerrado por não
// haver nada a repassar (líquido e saldo disponível/reservado zerados). Essa
// conclusão não movimenta caixa nem gera comprovante. Campo opcional para
// conviver com payloads antigos: sem ele a UI cai na heurística de líquido
// zerado.
settled_without_balance?: boolean;
history?: TransferHistoryEntry[];
bank_account?: string;
notes?: string;
proof_url?: string;
created_at: string;
updated_at: string;
// Alocação econômica (repasse allocation-aware): total/reservado/bloqueado/
// disponível do OwnerBalance no momento do repasse (backend populate quando
// allocation-aware).
owner_balance?: OwnerBalance;
economic_breakdown?: EconomicBreakdown;
owner_balance_base_total_cents?: number;
owner_balance_base_reserved_cents?: number;
}

export interface TransferHistoryEntry {
at: string;
action: 'created' | 'approved' | 'processed' | 'cancelled';
actor?: string;
actor_label?: string;
note?: string;
}

export interface CreateTransferDTO {
contract_id: string;
owner_id: string;
property_id: string;
reference_month: string;
gross_rent: number;
admin_fee_percent: number;
maintenance_deductions?: number;
other_deductions?: number;
deduction_details?: Deduction[];
bank_account?: string;
notes?: string;
}

class TransferService {
private readonly baseUrl = '/transfers';

async getAll(params?: { contract_id?: string; owner_id?: string; status?: string; reference_month?: string; limit?: number; offset?: number }) {
const response = await api.get<{ data: OwnerTransfer[]; meta: any }>(this.baseUrl, { params });
return response.data;
}

async getById(id: string) {
const response = await api.get<{ data: OwnerTransfer }>(`${this.baseUrl}/${id}`);
return response.data;
}

async create(data: CreateTransferDTO) {
const response = await api.post<{ data: OwnerTransfer }>(this.baseUrl, data);
return response.data;
}

async approve(id: string) {
const response = await api.post<{ data: OwnerTransfer }>(`${this.baseUrl}/${id}/approve`, {});
return response.data;
}

async process(id: string, evidence?: { proof_url?: string; note?: string }) {
const response = await api.post<{ data: OwnerTransfer }>(`${this.baseUrl}/${id}/process`, evidence ?? {});
return response.data;
}

// Returns the PDF as a Blob ready for download. Uses the api axios instance
// so the auth/tenant headers are attached. Caller is responsible for
// turning the blob into an object URL and triggering the browser download.
async downloadReceipt(id: string): Promise<Blob> {
const response = await api.get(`${this.baseUrl}/${id}/receipt`, { responseType: 'blob' });
return response.data as Blob;
}
}

export const transferService = new TransferService();
