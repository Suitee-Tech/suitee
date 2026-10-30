package payout_order

import (
	"errors"
	"testing"
	"time"
)

func TestRentalPayoutStaysDraftUntilCommissionIsFullyFunded(t *testing.T) {
	t.Parallel()

	order, err := New("tenant-1", SourceManual, "contract-1", RecipientBroker, "broker-1", "Corretor", 500000, "system")
	if err != nil {
		t.Fatal(err)
	}
	order.FundingRequiredCents = 500000
	order.Status = StatusDraft
	if err := order.Approve("operator-1"); !errors.Is(err, ErrFundingIncomplete) {
		t.Fatalf("Approve unfunded = %v, want ErrFundingIncomplete", err)
	}

	changed, err := order.RecordFunding(FundingEntry{
		PaymentID: "payment-aug", ChargeID: "charge-aug", ReferenceMonth: "2026-08",
		AmountCents: 225000, FundedAt: time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC),
	})
	if err != nil || !changed || order.Status != StatusDraft || order.FundingOutstandingCents() != 275000 {
		t.Fatalf("partial payout funding = (%v, %v), order=%+v", changed, err, order)
	}

	changed, err = order.RecordFunding(FundingEntry{
		PaymentID: "payment-sep", ChargeID: "charge-sep", ReferenceMonth: "2026-09",
		AmountCents: 275000, FundedAt: time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC),
	})
	if err != nil || !changed || order.Status != StatusPendingReview || order.FundingOutstandingCents() != 0 {
		t.Fatalf("complete payout funding = (%v, %v), order=%+v", changed, err, order)
	}
	if err := order.Approve("operator-1"); err != nil {
		t.Fatalf("Approve fully funded = %v", err)
	}
	if order.Status != StatusApproved {
		t.Fatalf("status = %s, want APPROVED", order.Status)
	}
}

func TestEnableRentalFundingGateMovesLegacyReviewOrderBackToDraft(t *testing.T) {
	t.Parallel()

	order, err := New("tenant-1", SourceManual, "contract-1", RecipientBroker, "broker-1", "Corretor", 500000, "system")
	if err != nil {
		t.Fatal(err)
	}
	changed, err := order.EnableRentalFundingGate(500000)
	if err != nil || !changed || order.Status != StatusDraft || order.FundingRequiredCents != 500000 {
		t.Fatalf("legacy payout upgrade = (%v, %v), order=%+v", changed, err, order)
	}
	if changed, err := order.EnableRentalFundingGate(500000); err != nil || changed {
		t.Fatalf("idempotent payout upgrade = (%v, %v)", changed, err)
	}

	order.Status = StatusApproved
	order.FundingRequiredCents = 0
	if _, err := order.EnableRentalFundingGate(500000); !errors.Is(err, ErrFundingGateConflict) {
		t.Fatalf("approved legacy payout upgrade = %v, want ErrFundingGateConflict", err)
	}
}

func TestLegacyRentalPayoutRequiresReconciliationBeforeApprovalOrPayment(t *testing.T) {
	t.Parallel()

	order, err := New("tenant-legacy-rent", SourceManual, "contract-legacy-rent", RecipientBroker, "broker-legacy-rent", "Corretor", 320000, "system")
	if err != nil {
		t.Fatal(err)
	}
	order.SourceRef = map[string]any{"charge_type": "RENT"}

	if err := order.Approve("operator-1"); !errors.Is(err, ErrFundingGateConflict) {
		t.Fatalf("Approve legacy rental payout = %v, want ErrFundingGateConflict", err)
	}
	if order.Status != StatusPendingReview {
		t.Fatalf("status after blocked approval = %s, want PENDING_REVIEW", order.Status)
	}

	order.Status = StatusApproved
	if err := order.MarkPaid("operator-1", string(SettlementManualPIX), "proof.pdf", "proof.pdf", "", true); !errors.Is(err, ErrFundingGateConflict) {
		t.Fatalf("MarkPaid legacy rental payout = %v, want ErrFundingGateConflict", err)
	}
	if order.Status != StatusApproved {
		t.Fatalf("status after blocked payment = %s, want APPROVED", order.Status)
	}
}

func TestLegacySalePayoutRequiresFundingReconciliation(t *testing.T) {
	t.Parallel()

	order, err := New("tenant-legacy-sale", SourceManual, "contract-legacy-sale", RecipientBroker, "broker-legacy-sale", "Corretor", 320000, "system")
	if err != nil {
		t.Fatal(err)
	}
	order.SourceRef = map[string]any{"charge_type": "SALE"}

	if err := order.Approve("operator-1"); !errors.Is(err, ErrFundingGateConflict) {
		t.Fatalf("Approve legacy sale payout = %v, want ErrFundingGateConflict", err)
	}
	if order.Status != StatusPendingReview {
		t.Fatalf("status = %s, want PENDING_REVIEW", order.Status)
	}
	order.Status = StatusApproved
	if err := order.MarkPaid("operator-1", string(SettlementManualPIX), "proof.pdf", "proof.pdf", "", true); !errors.Is(err, ErrFundingGateConflict) {
		t.Fatalf("MarkPaid legacy sale payout = %v, want ErrFundingGateConflict", err)
	}
	if order.Status != StatusApproved {
		t.Fatalf("status after blocked payment = %s, want APPROVED", order.Status)
	}
}
