package commission

import (
	"errors"
	"testing"
	"time"
)

func TestRentalCommissionCannotBePaidUntilCollectedRentFundsIt(t *testing.T) {
	t.Parallel()

	c := NewWithRole("deal-1", "broker-1", "contract-1", "fechamento", 100, 5000, "tenant-1")
	c.FundingRequiredCents = 500000
	first := FundingEntry{
		PaymentID: "payment-aug", ChargeID: "charge-aug", ReferenceMonth: "2026-08",
		AmountCents: 225000, FundedAt: time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC),
	}
	changed, err := c.RecordFunding(first)
	if err != nil || !changed || c.FundingOutstandingCents() != 275000 {
		t.Fatalf("partial funding = (%v, %v), commission=%+v", changed, err, c)
	}
	if err := c.Approve(); !errors.Is(err, ErrFundingIncomplete) {
		t.Fatalf("Approve partial funding = %v, want ErrFundingIncomplete", err)
	}
	if err := c.MarkPaid("proof-1", ""); !errors.Is(err, ErrFundingIncomplete) {
		t.Fatalf("MarkPaid partial funding = %v, want ErrFundingIncomplete", err)
	}

	changed, err = c.RecordFunding(FundingEntry{
		PaymentID: "payment-sep", ChargeID: "charge-sep", ReferenceMonth: "2026-09",
		AmountCents: 275000, FundedAt: time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC),
	})
	if err != nil || !changed || c.FundingOutstandingCents() != 0 {
		t.Fatalf("complete funding = (%v, %v), commission=%+v", changed, err, c)
	}
	if err := c.Approve(); err != nil {
		t.Fatalf("Approve fully funded = %v", err)
	}
	if err := c.MarkPaid("proof-1", "Pago via PIX"); err != nil {
		t.Fatalf("MarkPaid fully funded = %v", err)
	}
	if c.Status != StatusPaid || c.FundedAmountCents != 500000 || len(c.FundingEntries) != 2 {
		t.Fatalf("final commission = %+v", c)
	}
}

func TestRentalCommissionFundingRetryIsIdempotent(t *testing.T) {
	t.Parallel()

	c := New("deal-1", "broker-1", 1000, "tenant-1")
	c.FundingRequiredCents = 100000
	entry := FundingEntry{
		PaymentID: "payment-1", ChargeID: "charge-1", ReferenceMonth: "2026-08",
		AmountCents: 100000, FundedAt: time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC),
	}
	if changed, err := c.RecordFunding(entry); err != nil || !changed {
		t.Fatalf("first funding = (%v, %v)", changed, err)
	}
	if changed, err := c.RecordFunding(entry); err != nil || changed || c.FundedAmountCents != 100000 || len(c.FundingEntries) != 1 {
		t.Fatalf("funding retry = (%v, %v), commission=%+v", changed, err, c)
	}
}

func TestEnableRentalFundingGateSafelyUpgradesOnlyPendingLegacyCommission(t *testing.T) {
	t.Parallel()

	legacy := NewWithRole("deal-1", "broker-1", "contract-1", "fechamento", 100, 5000, "tenant-1")
	changed, err := legacy.EnableRentalFundingGate(500000)
	if err != nil || !changed || legacy.FundingRequiredCents != 500000 {
		t.Fatalf("legacy upgrade = (%v, %v), commission=%+v", changed, err, legacy)
	}
	if changed, err := legacy.EnableRentalFundingGate(500000); err != nil || changed {
		t.Fatalf("idempotent upgrade = (%v, %v)", changed, err)
	}

	approved := NewWithRole("deal-2", "broker-2", "contract-2", "captador", 100, 5000, "tenant-1")
	approved.Status = StatusApproved
	if _, err := approved.EnableRentalFundingGate(500000); !errors.Is(err, ErrFundingGateConflict) {
		t.Fatalf("approved legacy upgrade = %v, want ErrFundingGateConflict", err)
	}
	if approved.FundingRequiredCents != 0 {
		t.Fatalf("unsafe upgrade mutated approved commission: %+v", approved)
	}
}
