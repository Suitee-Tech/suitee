package repositories

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oginoino/toca/toca-financial/internal/domain/report"
)

func TestFinalizeReconciliationSummaryCountsSeverity(t *testing.T) {
	t.Parallel()
	rep := &report.ReconciliationReport{
		Findings: []report.ReconciliationFinding{
			{Severity: "ERROR", Category: "duplicate"},
			{Severity: "WARN", Category: "orphan"},
			{Severity: "ERROR", Category: "consistency"},
		},
	}

	finalizeReconciliationSummary(rep)

	if rep.Summary.TotalFindings != 3 {
		t.Fatalf("total findings: got %d, want 3", rep.Summary.TotalFindings)
	}
	if rep.Summary.Errors != 2 {
		t.Fatalf("errors: got %d, want 2", rep.Summary.Errors)
	}
	if rep.Summary.Warnings != 1 {
		t.Fatalf("warnings: got %d, want 1", rep.Summary.Warnings)
	}
	if rep.Summary.Duplicates != 1 || rep.Summary.Orphans != 1 || rep.Summary.Consistency != 1 {
		t.Fatalf("category counters not derived from findings: %#v", rep.Summary)
	}
}

func TestFinalizeReconciliationSummaryRebuildsCountersAndFailsUnknownSeverityClosed(t *testing.T) {
	t.Parallel()
	rep := &report.ReconciliationReport{
		Summary: report.ReconciliationSummary{
			TotalFindings: 99,
			Errors:        99,
			Warnings:      99,
			Duplicates:    99,
		},
		Findings: []report.ReconciliationFinding{
			{Severity: " warn ", Category: "suballoc"},
			{Severity: "UNKNOWN", Category: "excess"},
		},
	}

	finalizeReconciliationSummary(rep)

	if rep.Summary.TotalFindings != 2 || rep.Summary.Errors != 1 || rep.Summary.Warnings != 1 {
		t.Fatalf("severity summary = %#v", rep.Summary)
	}
	if rep.Summary.SubAlloc != 1 || rep.Summary.ExcessAlloc != 1 || rep.Summary.Duplicates != 0 {
		t.Fatalf("category summary = %#v", rep.Summary)
	}
}

func TestRunReconciliationChecksFailsClosedAndStopsAfterFirstError(t *testing.T) {
	t.Parallel()
	sqlErr := errors.New("injected SQL failure")
	secondRan := false
	checks := []reconciliationCheck{
		{name: "payments", run: func(context.Context, string, *report.ReconciliationReport) error { return sqlErr }},
		{name: "later", run: func(context.Context, string, *report.ReconciliationReport) error {
			secondRan = true
			return nil
		}},
	}

	err := runReconciliationChecks(context.Background(), "tenant-1", &report.ReconciliationReport{}, checks)
	if !errors.Is(err, sqlErr) || !strings.Contains(err.Error(), "reconciliation payments") {
		t.Fatalf("wrapped SQL error: %v", err)
	}
	if secondRan {
		t.Fatal("reconciliation continued after an incomplete SQL check")
	}
}

func TestChargePaymentConsistencyFindingsCoversPartialsAndAppendOnlyMismatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		state chargePaymentState
		want  int
	}{
		{
			name: "valid partial",
			state: chargePaymentState{ID: "partial", Status: "PARTIALLY_PAID", TotalCents: 10000,
				LegacyPaidCents: 4000, PaymentsSumCents: 4000, EffectivePaidCents: 4000,
				PaymentCount: 2, HasLegacyPaidAmount: true},
			want: 0,
		},
		{
			name: "valid oldest paid row without paid amount",
			state: chargePaymentState{ID: "legacy-paid", Status: "PAID", TotalCents: 10000,
				EffectivePaidCents: 10000},
			want: 0,
		},
		{
			name: "partial equal total",
			state: chargePaymentState{ID: "bad-partial", Status: "PARTIALLY_PAID", TotalCents: 10000,
				LegacyPaidCents: 10000, PaymentsSumCents: 10000, EffectivePaidCents: 10000,
				PaymentCount: 2, HasLegacyPaidAmount: true},
			want: 1,
		},
		{
			name: "append only mismatch and invalid sign",
			state: chargePaymentState{ID: "bad-log", Status: "PAID", TotalCents: 10000,
				LegacyPaidCents: 10000, PaymentsSumCents: 9000, EffectivePaidCents: 9000,
				PaymentCount: 2, HasLegacyPaidAmount: true, HasNonPositivePayment: true},
			want: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chargePaymentConsistencyFindings(tt.state)
			if len(got) != tt.want {
				t.Fatalf("findings = %#v, want count %d", got, tt.want)
			}
		})
	}
}

func TestTransactionConsistencyFindingsCoversSignsAndReversals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		state transactionReversalState
		want  int
	}{
		{
			name: "valid reversal",
			state: transactionReversalState{ID: "rev", Type: "estorno", Status: "PAID", ValueCents: -10000,
				OriginalID: "orig", OriginalType: "receita", OriginalStatus: "REVERSED", OriginalCents: 10000},
			want: 0,
		},
		{
			name:  "normal posting with inverted sign",
			state: transactionReversalState{ID: "bad-sign", Type: "despesa", Status: "PAID", ValueCents: -100},
			want:  1,
		},
		{
			name: "orphan malformed reversal",
			state: transactionReversalState{ID: "bad-rev", Type: "estorno", Status: "PENDING", ValueCents: 100,
				OriginalID: ""},
			want: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := transactionConsistencyFindings(tt.state)
			if len(got) != tt.want {
				t.Fatalf("findings = %#v, want count %d", got, tt.want)
			}
		})
	}
}
