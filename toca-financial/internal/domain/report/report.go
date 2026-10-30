// Package report holds the value types for the consolidated financial report.
// All monetary values are in cents.
package report

import "time"

// StatusBucket aggregates rows of one status: a count and a cents total.
type StatusBucket struct {
	Status     string `json:"status"`
	Count      int    `json:"count"`
	TotalCents int64  `json:"total_cents"`
}

// DelinquencySnapshot summarises overdue receivables as of now.
type DelinquencySnapshot struct {
	OverdueCount       int     `json:"overdue_count"`
	OverdueTotalCents  int64   `json:"overdue_total_cents"`
	AverageDaysOverdue float64 `json:"average_days_overdue"`
}

// Section is one report area broken down by status, with a grand total.
type Section struct {
	ByStatus   []StatusBucket `json:"by_status"`
	TotalCents int64          `json:"total_cents"`
}

// FinancialSummary is the tenant-scoped consolidated financial report.
type FinancialSummary struct {
	TenantID    string     `json:"tenant_id"`
	GeneratedAt time.Time  `json:"generated_at"`
	From        *time.Time `json:"from,omitempty"`
	To          *time.Time `json:"to,omitempty"`

	Receivables Section             `json:"receivables"` // financial.charges
	Withdrawals Section             `json:"withdrawals"` // financial.withdrawals
	Delinquency DelinquencySnapshot `json:"delinquency"`
}

// CategoryBucket é como StatusBucket mas group by category — usado em
// Revenue (by ChargeType: RENT/SALE/CONDOMINIO/IPTU/UTILITY/MAINTENANCE/OTHER)
// e em Expense (by transaction.Category: Comissão, Repasse, Manutenção, etc.).
type CategoryBucket struct {
	Category   string `json:"category"`
	Count      int    `json:"count"`
	TotalCents int64  `json:"total_cents"`
}

// DREReport é o demonstrativo de resultado simplificado do período. Receita
// reconhece somente a parcela economicamente pertencente à imobiliária;
// dinheiro administrado de proprietários, corretores e parceiros permanece no
// fluxo de caixa e nos passivos. As linhas seguem o agrupamento do frontend.
type DREReport struct {
	TenantID                  string           `json:"tenant_id"`
	From                      time.Time        `json:"from"`
	To                        time.Time        `json:"to"`
	GeneratedAt               time.Time        `json:"generated_at"`
	Revenue                   []CategoryBucket `json:"revenue"`
	Expense                   []CategoryBucket `json:"expense"`
	NetCents                  int64            `json:"net_cents"`
	UnclassifiedReceiptsCents int64            `json:"unclassified_receipts_cents"`
}

// CashFlowGranularity controla a resolução do CashFlowReport. Day é a
// granularidade default; week/month agregam o mesmo recordset em buckets
// maiores.
type CashFlowGranularity string

const (
	CashFlowDaily   CashFlowGranularity = "day"
	CashFlowWeekly  CashFlowGranularity = "week"
	CashFlowMonthly CashFlowGranularity = "month"
)

// CashFlowEntry é uma linha do CashFlowReport.
type CashFlowEntry struct {
	// Period é o início do bucket (YYYY-MM-DD para day, YYYY-MM para month,
	// "YYYY-Www" para week — formato ISO).
	Period       string `json:"period"`
	InflowCents  int64  `json:"inflow_cents"`  // charges PAID + invoices PAID
	OutflowCents int64  `json:"outflow_cents"` // transfers PROCESSED + withdrawals COMPLETED
	NetCents     int64  `json:"net_cents"`
}

// CashFlowReport é a série temporal usada pelo gráfico de fluxo de caixa.
type CashFlowReport struct {
	TenantID    string              `json:"tenant_id"`
	From        time.Time           `json:"from"`
	To          time.Time           `json:"to"`
	Granularity CashFlowGranularity `json:"granularity"`
	GeneratedAt time.Time           `json:"generated_at"`
	Entries     []CashFlowEntry     `json:"entries"`
}

// KPIs é o snapshot que alimenta os cards do dashboard. Métricas todas em
// cents (UI converte) — exceto percentuais (DelinquencyPct, AverageTicket
// é cents) e contagens.
type KPIs struct {
	TenantID            string    `json:"tenant_id"`
	GeneratedAt         time.Time `json:"generated_at"`
	RevenueMonthCents   int64     `json:"revenue_month_cents"`   // receita própria realizada no mês
	RevenueLast30Cents  int64     `json:"revenue_last_30_cents"` // receita própria na janela móvel de 30 dias
	ReceivableCents     int64     `json:"receivable_cents"`      // PENDING + OVERDUE + saldo de PARTIALLY_PAID
	OverdueCents        int64     `json:"overdue_cents"`         // só OVERDUE
	OverdueCount        int       `json:"overdue_count"`
	DelinquencyPct      float64   `json:"delinquency_pct"` // overdue / (overdue + recebimento bruto do mês) × 100
	AverageTicketCents  int64     `json:"average_ticket_cents"`
	ActiveContractCount int       `json:"active_contract_count"`
	CommissionPaidMonth int64     `json:"commission_paid_month_cents"`

	// Posição contábil corrente derivada exclusivamente do journal. Saldos de
	// passivo são apresentados como obrigações positivas, embora o ledger os
	// armazene naturalmente como crédito (valor assinado negativo).
	CashBalanceCents            int64 `json:"cash_balance_cents"`
	OwnerPayableCents           int64 `json:"owner_payable_cents"`
	CommissionPayableCents      int64 `json:"commission_payable_cents"`
	PartnerPayableCents         int64 `json:"partner_payable_cents"`
	RoyaltyPayableCents         int64 `json:"royalty_payable_cents"`
	TaxWithheldCents            int64 `json:"tax_withheld_cents"`
	OtherPayableCents           int64 `json:"other_payable_cents"`
	WithdrawalPayableCents      int64 `json:"withdrawal_payable_cents"`
	ThirdPartyObligationsCents  int64 `json:"third_party_obligations_cents"`
	OtherCommitmentsCents       int64 `json:"other_commitments_cents"`
	CommittedFundsCents         int64 `json:"committed_funds_cents"`
	AgencyAvailableCashCents    int64 `json:"agency_available_cash_cents"`
	CoverageGapCents            int64 `json:"coverage_gap_cents"`
	UnexpectedPayableDebitCents int64 `json:"unexpected_payable_debit_cents"`
}

// ── Reconciliation Report (FIN-COM-14) ────────────────────────────

// ReconciliationFinding is a single detected issue in the reconciliation
// dry-run. Each finding has a severity, category, entity reference, and
// a human-readable description.
type ReconciliationFinding struct {
	Severity   string `json:"severity"` // ERROR | WARN
	Category   string `json:"category"` // duplicate | orphan | excess | suballoc | consistency
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Reference  string `json:"reference"` // related entity or value
	Message    string `json:"message"`   // human-readable description
}

// ReconciliationReport is the top-level output of the reconciliation
// dry-run. It lists all findings across the four detection categories
// and includes summary counts. The report is read-only — it never
// modifies data.
type ReconciliationReport struct {
	TenantID    string                  `json:"tenant_id"`
	GeneratedAt time.Time               `json:"generated_at"`
	DryRun      bool                    `json:"dry_run"`
	Summary     ReconciliationSummary   `json:"summary"`
	Findings    []ReconciliationFinding `json:"findings"`
}

// ReconciliationSummary provides aggregate counts of findings by category
// so operators can prioritise investigation.
type ReconciliationSummary struct {
	TotalFindings int `json:"total_findings"`
	Errors        int `json:"errors"`
	Warnings      int `json:"warnings"`
	Duplicates    int `json:"duplicates"`
	Orphans       int `json:"orphans"`
	ExcessAlloc   int `json:"excess_alloc"`
	SubAlloc      int `json:"sub_alloc"`
	Consistency   int `json:"consistency"`
}
