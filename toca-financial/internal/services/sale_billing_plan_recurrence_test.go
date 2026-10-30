package services

import (
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan"
)

func TestSaleInstallmentsRecurMonthlyAcrossShortMonths(t *testing.T) {
	first := time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)
	plan, err := sale_billing_plan.New(
		"tenant-1", "contract-1", "deal-1", "buyer-1", "owner-1", "property-1",
		sale_billing_plan.ModeInstallments,
		120000, 0, 40000, 3, first, 30, sale_billing_plan.CompositeInput{},
	)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	svc := NewSaleBillingPlanService(nil, nil)
	charges := svc.BuildChargesPreview(plan)
	if len(charges) != 3 {
		t.Fatalf("installment charges = %d, want 3", len(charges))
	}

	// Monthly cadence anchored to FirstDueDate's day-of-month. Short months
	// clamp, but the following month returns to the original day (31) — it
	// must not "slip" from Feb 28 to Mar 28.
	want := []time.Time{
		time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.February, 28, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC),
	}
	for i, ch := range charges {
		if !ch.DueDate.Equal(want[i]) {
			t.Fatalf("installment %d due = %s, want %s", i+1, ch.DueDate.Format("2006-01-02"), want[i].Format("2006-01-02"))
		}
	}
}

func TestSaleInstallmentsWithDownPaymentOffsetOneMonth(t *testing.T) {
	first := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)
	plan, err := sale_billing_plan.New(
		"tenant-1", "contract-1", "deal-1", "buyer-1", "owner-1", "property-1",
		sale_billing_plan.ModeInstallments,
		50000, 10000, 20000, 2, first, 30, sale_billing_plan.CompositeInput{},
	)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	svc := NewSaleBillingPlanService(nil, nil)
	charges := svc.BuildChargesPreview(plan)
	if len(charges) != 3 {
		t.Fatalf("charges (downpayment + installments) = %d, want 3", len(charges))
	}

	// Order: entrada on FirstDueDate, then installment.1 at +1 month,
	// installment.2 at +2 months.
	want := []time.Time{
		time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 15, 0, 0, 0, 0, time.UTC),
	}
	for i, ch := range charges {
		if !ch.DueDate.Equal(want[i]) {
			t.Fatalf("charge %d due = %s, want %s", i+1, ch.DueDate.Format("2006-01-02"), want[i].Format("2006-01-02"))
		}
	}
}
