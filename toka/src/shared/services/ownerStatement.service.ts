import api from "./api";
import type { EconomicBreakdown } from "./transfer.service";

export type StatementFlow = "sale" | "rent" | "unknown";

export interface StatementFlowSummary {
  flow: StatementFlow;
  transfer_count: number;
  gross_amount: number;
  admin_fee_amount: number;
  brokerage_amount: number | null;
  maintenance_deductions: number;
  other_deductions: number;
  discount_amount: number;
  /** Intermediação descontada do aluguel no período (comissão do 1º aluguel). */
  intermediation_amount?: number;
  /** Fatia da intermediação que foi para comissões de corretor. */
  intermediation_commission_funded_amount?: number;
  net_amount: number;
}

export interface OwnerBalanceBuckets {
  total_cents: number;
  reserved_cents: number;
  blocked_cents: number;
}

export interface ManagedBillLine {
  id: string;
  component: string;
  creditor_name: string;
  description: string;
  amount: number;
  status: string;
  due_date?: string | null;
}

export interface JournalTotals {
  revenue: number;
  owner_payable: number;
  accounts_receivable: number;
  cash: number;
}

export interface StatementDeduction {
  description: string;
  amount: number;
  /** MAINTENANCE, OTHER... — de onde o desconto veio. */
  type: string;
  /** Identidade da origem (ex.: id do chamado de manutenção). */
  reference_id?: string;
}

export interface MonthlyStatement {
  tenant_id: string;
  owner_lead_id: string;
  year: number;
  month: number;
  reference_month: string;
  /** Historical API field: combined gross of all modalities. */
  gross_rent: number;
  flow_summaries?: StatementFlowSummary[];
  admin_fee_amount: number;
  maintenance_deductions: number;
  other_deductions: number;
  net_amount: number;
  /** Intermediação do período e o saldo que ainda será descontado. */
  intermediation_amount?: number;
  intermediation_commission_funded_amount?: number;
  intermediation_outstanding_amount?: number;
  owner_balance_total?: number;
  reserved_amount?: number;
  blocked_amount?: number;
  available_amount?: number;
  balance_covered_count?: number;
  managed_bills?: ManagedBillLine[];
  managed_bills_total?: number;
  journal_totals?: JournalTotals;
  transfers: Array<{
    id: string;
    contract_id: string;
    /**
     * Deduções aplicadas ao repasse (ex.: manutenção descontada do
     * proprietário), com a descrição que veio do chamado.
     */
    deduction_details?: StatementDeduction[];
    property_id: string;
    reference_month: string;
    statement_flow?: StatementFlow;
    settlement_type?: string;
    economic_breakdown?: EconomicBreakdown;
    brokerage_amount?: number | null;
    intermediation_amount?: number | null;
    intermediation_commission_funded_amount?: number | null;
    intermediation_outstanding_amount?: number | null;
    gross_rent: number;
    admin_fee_amount: number;
    maintenance_deductions?: number;
    other_deductions?: number;
    net_amount: number;
    status: string;
    owner_balance?: OwnerBalanceBuckets;
  }>;
  generated_at: string;
}

// OwnerBalanceSummary é o saldo corrente (widget fixo): allocation-aware, com
// as movimentações de contas administradas reaplicadas sobre o último repasse.
export interface OwnerBalanceSummary {
  tenant_id: string;
  owner_lead_id: string;
  transfer_id: string;
  reference_month: string;
  total_cents: number;
  reserved_cents: number;
  blocked_cents: number;
  available_cents: number;
  has_balance: boolean;
  updated_at: string;
}

class OwnerStatementService {
  async getAdmin(ownerLeadId: string, year: number, month: number): Promise<MonthlyStatement> {
    const res = await api.get<{ data: MonthlyStatement }>(`/owner-statements/${ownerLeadId}`, {
      params: { year, month, format: "json" },
    });
    return res.data?.data;
  }

  async getBalance(ownerLeadId: string): Promise<OwnerBalanceSummary> {
    const res = await api.get<{ data: OwnerBalanceSummary }>(`/owner-statements/${ownerLeadId}/balance`);
    return res.data?.data;
  }

  getAdminPdfUrl(ownerLeadId: string, year: number, month: number): string {
    return `/owner-statements/${ownerLeadId}?year=${year}&month=${month}&format=pdf`;
  }

  async getMine(year: number, month: number): Promise<MonthlyStatement> {
    const res = await api.get<{ data: MonthlyStatement }>(`/client-portal/owner-statements`, {
      params: { year, month, format: "json" },
    });
    return res.data?.data;
  }

  getMinePdfUrl(year: number, month: number): string {
    return `/client-portal/owner-statements?year=${year}&month=${month}&format=pdf`;
  }

  async downloadAdminPdf(ownerLeadId: string, year: number, month: number): Promise<Blob> {
    const res = await api.get(`/owner-statements/${ownerLeadId}`, {
      params: { year, month, format: "pdf" },
      responseType: "blob",
    });
    return res.data as Blob;
  }
}

export const ownerStatementService = new OwnerStatementService();
