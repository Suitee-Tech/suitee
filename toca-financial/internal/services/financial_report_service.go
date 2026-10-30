package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/toca-financial/internal/domain/report"
)

// FinancialReportRepository runs the aggregate queries behind the consolidated
// financial report + dashboard (DRE/CashFlow/KPIs).
type FinancialReportRepository interface {
	ChargeTotals(ctx context.Context, tenantID string, from, to *time.Time) ([]report.StatusBucket, error)
	WithdrawalTotals(ctx context.Context, tenantID string, from, to *time.Time) ([]report.StatusBucket, error)
	DelinquencyTotals(ctx context.Context, tenantID string) (report.DelinquencySnapshot, error)
	RevenueByType(ctx context.Context, tenantID string, from, to time.Time) ([]report.CategoryBucket, error)
	ExpenseByCategory(ctx context.Context, tenantID string, from, to time.Time) ([]report.CategoryBucket, error)
	CashFlowByDay(ctx context.Context, tenantID string, from, to time.Time) ([]report.CashFlowEntry, error)
	KpisSnapshot(ctx context.Context, tenantID string) (report.KPIs, error)
}

// FinancialReportService composes the tenant-scoped consolidated financial
// report: receivables, withdrawals and delinquency.
type FinancialReportService struct {
	repo FinancialReportRepository
}

func NewFinancialReportService(repo FinancialReportRepository) *FinancialReportService {
	return &FinancialReportService{repo: repo}
}

// Summary builds the consolidated report. The optional from/to window is
// applied to charge due dates and to withdrawal creation dates; delinquency is
// always computed as of now. Receivables.TotalCents is the open balance only.
func (s *FinancialReportService) Summary(ctx context.Context, tenantID string, from, to *time.Time) (*report.FinancialSummary, error) {
	charges, err := s.repo.ChargeTotals(ctx, tenantID, from, to)
	if err != nil {
		return nil, err
	}
	withdrawals, err := s.repo.WithdrawalTotals(ctx, tenantID, from, to)
	if err != nil {
		return nil, err
	}
	delinquency, err := s.repo.DelinquencyTotals(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	return &report.FinancialSummary{
		TenantID:    tenantID,
		GeneratedAt: time.Now(),
		From:        from,
		To:          to,
		Receivables: report.Section{ByStatus: charges, TotalCents: sumOpenReceivableBuckets(charges)},
		Withdrawals: report.Section{ByStatus: withdrawals, TotalCents: sumStatusBuckets(withdrawals)},
		Delinquency: delinquency,
	}, nil
}

func sumStatusBuckets(b []report.StatusBucket) int64 {
	var total int64
	for _, x := range b {
		total += x.TotalCents
	}
	return total
}

func sumOpenReceivableBuckets(b []report.StatusBucket) int64 {
	var total int64
	for _, x := range b {
		switch canonicalReportStatus(x.Status) {
		case "PENDING", "OVERDUE", "PARTIALLY_PAID":
			total += x.TotalCents
		}
	}
	return total
}

func canonicalReportStatus(status string) string {
	normalized := strings.ToUpper(strings.TrimSpace(status))
	switch normalized {
	case "PAGO", "PAGA":
		return "PAID"
	case "PENDENTE":
		return "PENDING"
	case "EM ATRASO":
		return "OVERDUE"
	case "PARCIALMENTE PAGO", "PARCIALMENTE PAGA":
		return "PARTIALLY_PAID"
	case "ESTORNADO", "ESTORNADA":
		return "REVERSED"
	case "CANCELADO", "CANCELADA":
		return "CANCELLED"
	case "COMPLETED", "DONE", "CONCLUIDO", "CONCLUÍDO", "CONCLUIDA", "CONCLUÍDA":
		return "COMPLETE"
	case "PROCESSADO", "PROCESSADA":
		return "PROCESSED"
	default:
		return normalized
	}
}

// DRE compõe o demonstrativo de resultado: pagamentos realizados de charges
// por type versus transactions de saída liquidadas por category no período.
func (s *FinancialReportService) DRE(ctx context.Context, tenantID string, from, to time.Time) (*report.DREReport, error) {
	revenue, err := s.repo.RevenueByType(ctx, tenantID, from, to)
	if err != nil {
		return nil, err
	}
	expense, err := s.repo.ExpenseByCategory(ctx, tenantID, from, to)
	if err != nil {
		return nil, err
	}
	var unclassifiedCents int64
	if repo, ok := s.repo.(interface {
		UnclassifiedReceiptTotal(context.Context, string, time.Time, time.Time) (int64, error)
	}); ok {
		unclassifiedCents, err = repo.UnclassifiedReceiptTotal(ctx, tenantID, from, to)
		if err != nil {
			return nil, err
		}
	}
	var revSum, expSum int64
	for _, r := range revenue {
		revSum += r.TotalCents
	}
	for _, e := range expense {
		expSum += e.TotalCents
	}
	return &report.DREReport{
		TenantID:                  tenantID,
		From:                      from,
		To:                        to,
		GeneratedAt:               time.Now().UTC(),
		Revenue:                   revenue,
		Expense:                   expense,
		NetCents:                  revSum - expSum,
		UnclassifiedReceiptsCents: unclassifiedCents,
	}, nil
}

// CashFlow devolve a série temporal de inflow/outflow. Granularidade
// week/month reagrega a série diária via bucketing local — evita CTEs
// específicos por granularidade no SQL.
func (s *FinancialReportService) CashFlow(ctx context.Context, tenantID string, from, to time.Time, granularity report.CashFlowGranularity) (*report.CashFlowReport, error) {
	if granularity == "" {
		granularity = report.CashFlowDaily
	}
	daily, err := s.repo.CashFlowByDay(ctx, tenantID, from, to)
	if err != nil {
		return nil, err
	}
	entries := daily
	switch granularity {
	case report.CashFlowWeekly:
		entries = rebucket(daily, func(t time.Time) string {
			y, w := t.ISOWeek()
			return fmt.Sprintf("%04d-W%02d", y, w)
		})
	case report.CashFlowMonthly:
		entries = rebucket(daily, func(t time.Time) string {
			return t.Format("2006-01")
		})
	}
	return &report.CashFlowReport{
		TenantID:    tenantID,
		From:        from,
		To:          to,
		Granularity: granularity,
		GeneratedAt: time.Now().UTC(),
		Entries:     entries,
	}, nil
}

// Kpis pass-through para o snapshot consolidado.
func (s *FinancialReportService) Kpis(ctx context.Context, tenantID string) (*report.KPIs, error) {
	k, err := s.repo.KpisSnapshot(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// rebucket é o helper de agrupamento por chave (week/month) sobre a série
// diária. Preserva a ordem natural das chaves (lexicograficamente
// ascendente — ISO week e YYYY-MM são bem-comportados).
func rebucket(daily []report.CashFlowEntry, keyOf func(time.Time) string) []report.CashFlowEntry {
	if len(daily) == 0 {
		return daily
	}
	byKey := make(map[string]report.CashFlowEntry, len(daily))
	order := make([]string, 0, len(daily))
	for _, d := range daily {
		// O rótulo diário é um DIA CIVIL no fuso configurado: parsear em UTC
		// (padrão do time.Parse) jogava o dia para trás em 3h e o agrupamento
		// semanal/mensal caía na semana/mês anterior.
		t, _ := timeutil.ParseDateOnly(d.Period)
		k := keyOf(t)
		e, ok := byKey[k]
		if !ok {
			e = report.CashFlowEntry{Period: k}
			order = append(order, k)
		}
		e.InflowCents += d.InflowCents
		e.OutflowCents += d.OutflowCents
		e.NetCents = e.InflowCents - e.OutflowCents
		byKey[k] = e
	}
	out := make([]report.CashFlowEntry, 0, len(order))
	for _, k := range order {
		out = append(out, byKey[k])
	}
	return out
}
