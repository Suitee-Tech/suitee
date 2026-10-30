package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/report"
)

type financialReportRepoStub struct {
	charges        []report.StatusBucket
	withdrawals    []report.StatusBucket
	delinquency    report.DelinquencySnapshot
	chargeErr      error
	withdrawErr    error
	delinquencyErr error
	kpiErr         error
}

func (s *financialReportRepoStub) ChargeTotals(context.Context, string, *time.Time, *time.Time) ([]report.StatusBucket, error) {
	return s.charges, s.chargeErr
}

func (s *financialReportRepoStub) WithdrawalTotals(context.Context, string, *time.Time, *time.Time) ([]report.StatusBucket, error) {
	return s.withdrawals, s.withdrawErr
}

func (s *financialReportRepoStub) DelinquencyTotals(context.Context, string) (report.DelinquencySnapshot, error) {
	return s.delinquency, s.delinquencyErr
}

func (*financialReportRepoStub) RevenueByType(context.Context, string, time.Time, time.Time) ([]report.CategoryBucket, error) {
	return nil, nil
}

func (*financialReportRepoStub) ExpenseByCategory(context.Context, string, time.Time, time.Time) ([]report.CategoryBucket, error) {
	return nil, nil
}

func (*financialReportRepoStub) CashFlowByDay(context.Context, string, time.Time, time.Time) ([]report.CashFlowEntry, error) {
	return nil, nil
}

func (s *financialReportRepoStub) KpisSnapshot(context.Context, string) (report.KPIs, error) {
	return report.KPIs{}, s.kpiErr
}

func TestFinancialReportSummaryCanonicalizesLegacyOpenStatuses(t *testing.T) {
	t.Parallel()
	repo := &financialReportRepoStub{
		charges: []report.StatusBucket{
			{Status: " Pendente ", TotalCents: 100},
			{Status: "Em Atraso", TotalCents: 200},
			{Status: "Parcialmente Paga", TotalCents: 300},
			{Status: "Pago", TotalCents: 400},
			{Status: "Estornado", TotalCents: 500},
			{Status: "Cancelada", TotalCents: 600},
		},
	}

	got, err := NewFinancialReportService(repo).Summary(context.Background(), "tenant-legacy", nil, nil)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if got.Receivables.TotalCents != 600 {
		t.Fatalf("legacy open receivables: got %d, want 600", got.Receivables.TotalCents)
	}
}

func TestFinancialReportSummaryFailsClosedOnRepositoryError(t *testing.T) {
	t.Parallel()
	sqlErr := errors.New("injected SQL failure")
	tests := []struct {
		name string
		repo *financialReportRepoStub
	}{
		{name: "charges", repo: &financialReportRepoStub{chargeErr: sqlErr}},
		{name: "withdrawals", repo: &financialReportRepoStub{withdrawErr: sqlErr}},
		{name: "delinquency", repo: &financialReportRepoStub{delinquencyErr: sqlErr}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewFinancialReportService(tt.repo).Summary(context.Background(), "tenant-1", nil, nil)
			if !errors.Is(err, sqlErr) {
				t.Fatalf("error: got %v, want injected SQL failure", err)
			}
			if got != nil {
				t.Fatalf("partial summary returned on error: %#v", got)
			}
		})
	}
}

func TestFinancialReportKPIsPropagatesRepositoryError(t *testing.T) {
	t.Parallel()
	sqlErr := errors.New("injected KPI SQL failure")
	got, err := NewFinancialReportService(&financialReportRepoStub{kpiErr: sqlErr}).Kpis(context.Background(), "tenant-1")
	if !errors.Is(err, sqlErr) {
		t.Fatalf("error: got %v, want injected KPI SQL failure", err)
	}
	if got != nil {
		t.Fatalf("partial KPI snapshot returned on error: %#v", got)
	}
}

func TestFinancialReportSummaryTotalsOnlyOpenReceivableBalance(t *testing.T) {
	t.Parallel()
	repo := &financialReportRepoStub{
		charges: []report.StatusBucket{
			{Status: "PAID", TotalCents: 10000},
			{Status: "PENDING", TotalCents: 5000},
			{Status: "PARTIALLY_PAID", TotalCents: 3000},
			{Status: "CANCELLED", TotalCents: 2000},
		},
		withdrawals: []report.StatusBucket{
			{Status: "COMPLETED", TotalCents: 7000},
		},
	}

	got, err := NewFinancialReportService(repo).Summary(context.Background(), "tenant-1", nil, nil)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if got.Receivables.TotalCents != 8000 {
		t.Fatalf("open receivables: got %d, want 8000", got.Receivables.TotalCents)
	}
	if got.Withdrawals.TotalCents != 7000 {
		t.Fatalf("withdrawals: got %d, want 7000", got.Withdrawals.TotalCents)
	}
}
