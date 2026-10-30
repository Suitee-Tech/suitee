import api from './api';

export type FinancialFlow = 'sale' | 'rent';
export type FinancialAccountNature = 'asset' | 'liability' | 'equity' | 'revenue' | 'expense';

export interface FinancialAccount {
  code: string;
  name: string;
  nature: FinancialAccountNature;
  ledger_account: string;
  flows: FinancialFlow[];
  description: string;
  parent_code?: string;
}

export interface ChartOfAccounts {
  version: string;
  accounts: FinancialAccount[];
}

export interface CategoryBucket {
  category: string;
  count: number;
  total_cents: number;
}

export interface DREReport {
  tenant_id: string;
  from: string;
  to: string;
  generated_at: string;
  revenue: CategoryBucket[];
  expense: CategoryBucket[];
  net_cents: number;
  unclassified_receipts_cents?: number;
}

export type CashFlowGranularity = 'day' | 'week' | 'month';

export interface CashFlowEntry {
  period: string;
  inflow_cents: number;
  outflow_cents: number;
  net_cents: number;
}

export interface CashFlowReport {
  tenant_id: string;
  from: string;
  to: string;
  granularity: CashFlowGranularity;
  generated_at: string;
  entries: CashFlowEntry[];
}

export interface StatusBucket {
  status: string;
  count: number;
  total_cents: number;
}

/** Uma área do relatório quebrada por status, com o total do período. */
export interface ReportSection {
  by_status: StatusBucket[] | null;
  total_cents: number;
}

export interface DelinquencySnapshot {
  overdue_count: number;
  overdue_total_cents: number;
  average_days_overdue: number;
}

/** Relatório consolidado — GET /reports/financial/summary. */
export interface FinancialSummary {
  tenant_id: string;
  generated_at: string;
  from?: string;
  to?: string;
  receivables: ReportSection;
  withdrawals: ReportSection;
  delinquency: DelinquencySnapshot;
}

/** Gravidade de um achado da reconciliação. */
export type ReconciliationSeverity = "ERROR" | "WARN";

/**
 * Categoria de detecção. `total_findings` pode exceder a soma dos contadores
 * por categoria: uma categoria desconhecida não entra em nenhum bucket, então a
 * interface deve exibir o total do relatório e não uma soma calculada no cliente.
 */
export type ReconciliationCategory =
  | "duplicate"
  | "orphan"
  | "excess"
  | "suballoc"
  | "consistency";

export interface ReconciliationFinding {
  severity: ReconciliationSeverity;
  category: ReconciliationCategory;
  entity_type: string;
  /**
   * Em `duplicate`, é o `source_event_id` compartilhado, NÃO o id da linha:
   * não há como abrir o registro específico a partir daqui sem uma segunda
   * consulta. A interface não deve prometer esse atalho.
   */
  entity_id: string;
  reference: string;
  message: string;
}

export interface ReconciliationSummary {
  total_findings: number;
  errors: number;
  warnings: number;
  duplicates: number;
  orphans: number;
  excess_alloc: number;
  sub_alloc: number;
  consistency: number;
}

/** Relatório dry-run de integridade — GET /reports/financial/reconciliation. */
export interface ReconciliationReport {
  tenant_id: string;
  generated_at: string;
  dry_run: boolean;
  summary: ReconciliationSummary;
  findings: ReconciliationFinding[] | null;
}

export interface KPIs {  tenant_id: string;
  generated_at: string;
  revenue_month_cents: number;
  revenue_last_30_cents: number;
  receivable_cents: number;
  overdue_cents: number;
  overdue_count: number;
  delinquency_pct: number;
  average_ticket_cents: number;
  active_contract_count: number;
  commission_paid_month_cents: number;
  cash_balance_cents: number;
  owner_payable_cents: number;
  commission_payable_cents: number;
  partner_payable_cents: number;
  royalty_payable_cents: number;
  tax_withheld_cents: number;
  other_payable_cents: number;
  withdrawal_payable_cents: number;
  third_party_obligations_cents: number;
  other_commitments_cents: number;
  committed_funds_cents: number;
  agency_available_cash_cents: number;
  coverage_gap_cents: number;
  unexpected_payable_debit_cents: number;
}

interface DateRange {
  from: string;
  to: string;
}

class FinancialReportService {
  private readonly baseUrl = '/reports/financial';

  async chartOfAccounts(): Promise<ChartOfAccounts> {
    const res = await api.get<{ data: ChartOfAccounts }>(`${this.baseUrl}/chart-of-accounts`);
    return res.data.data;
  }

  async kpis(): Promise<KPIs> {
    const res = await api.get<{ data: KPIs }>(`${this.baseUrl}/kpis`);
    return res.data?.data;
  }

  async dre(range: DateRange): Promise<DREReport> {
    const res = await api.get<{ data: DREReport }>(`${this.baseUrl}/dre`, {
      params: range,
    });
    return res.data?.data;
  }

  /**
   * Relatório consolidado do período. Existe no backend desde a Fase 6 e não
   * tinha consumidor: as telas de total varriam a carteira inteira pelo
   * `collectPages` para somar no cliente. Agregar no servidor é o caminho para
   * reduzir essa dependência.
   *
   * O mesmo `to` em data-only é inclusivo aqui, como no DRE e no CashFlow.
   */
  async summary(range: DateRange): Promise<FinancialSummary> {
    const res = await api.get<{ data: FinancialSummary }>(`${this.baseUrl}/summary`, {
      params: range,
    });
    return res.data?.data;
  }

  /**
   * Relatório de integridade (dry-run): duplicatas, órfãos, alocação e
   * cascatas. Somente leitura — nunca altera registros.
   *
   * O backend aborta o relatório inteiro em qualquer erro de consulta
   * (fail-closed), então uma resposta bem-sucedida sempre representa a
   * verificação completa; não existe relatório parcial a interpretar.
   */
  async reconciliation(): Promise<ReconciliationReport> {
    const res = await api.get<{ data: ReconciliationReport }>(`${this.baseUrl}/reconciliation`);
    return res.data?.data;
  }

  async cashflow(
    range: DateRange,
    granularity: CashFlowGranularity = 'day',
  ): Promise<CashFlowReport> {
    const res = await api.get<{ data: CashFlowReport }>(
      `${this.baseUrl}/cashflow`,
      {
        params: { ...range, granularity },
      },
    );
    return res.data?.data;
  }
}

export const financialReportService = new FinancialReportService();
