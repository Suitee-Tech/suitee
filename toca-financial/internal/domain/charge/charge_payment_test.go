package charge

import (
	"errors"
	"testing"
	"time"
)

func newPaymentTestCharge(t *testing.T) *Charge {
	t.Helper()
	c, err := New("contract-1", "payer-1", "owner-1", "property-1", "tenant-1", TypeRent, "Rent", "2026-08", 100, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return c
}

func TestApplyPaymentAccumulatesExactlyAndIsIdempotent(t *testing.T) {
	c := newPaymentTestCharge(t)
	paidAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	changed, err := c.ApplyPayment("payment-1", 40, "PIX", paidAt)
	if err != nil || !changed {
		t.Fatalf("first ApplyPayment() = (%v, %v), want (true, nil)", changed, err)
	}
	if c.Status != StatusPartiallyPaid || c.PaidAmount == nil || c.PaidAmount.Cents() != 4000 {
		t.Fatalf("partial state = (%s, %v), want PARTIALLY_PAID and 4000 cents", c.Status, c.PaidAmount)
	}

	changed, err = c.ApplyPayment("payment-1", 40, "PIX", paidAt.Add(time.Minute))
	if err != nil || changed {
		t.Fatalf("retry ApplyPayment() = (%v, %v), want (false, nil)", changed, err)
	}
	if len(c.Payments) != 1 || c.PaidAmount.Cents() != 4000 {
		t.Fatalf("retry changed balance: payments=%d paid=%d", len(c.Payments), c.PaidAmount.Cents())
	}

	changed, err = c.ApplyPayment("payment-2", 60, "TED", paidAt.Add(time.Hour))
	if err != nil || !changed {
		t.Fatalf("final ApplyPayment() = (%v, %v), want (true, nil)", changed, err)
	}
	if c.Status != StatusPaid || c.PaidAmount.Cents() != 10000 || len(c.Payments) != 2 {
		t.Fatalf("final state = status=%s paid=%d payments=%d", c.Status, c.PaidAmount.Cents(), len(c.Payments))
	}
}

func TestApplyPaymentRejectsKeyReuseAndOverpaymentWithoutMutation(t *testing.T) {
	c := newPaymentTestCharge(t)
	paidAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	if _, err := c.ApplyPayment("payment-1", 40, "PIX", paidAt); err != nil {
		t.Fatal(err)
	}

	if _, err := c.ApplyPayment("payment-1", 41, "PIX", paidAt); err == nil {
		t.Fatal("expected reused payment id with a different amount to fail")
	}
	if _, err := c.ApplyPayment("payment-2", 61, "PIX", paidAt); err == nil {
		t.Fatal("expected overpayment to fail")
	}
	if len(c.Payments) != 1 || c.PaidAmount == nil || c.PaidAmount.Cents() != 4000 {
		t.Fatalf("rejected payments mutated charge: payments=%d paid=%v", len(c.Payments), c.PaidAmount)
	}
}

func TestApplyPaymentRejectsReversedCharge(t *testing.T) {
	c := newPaymentTestCharge(t)
	c.Status = StatusReversed
	if _, err := c.ApplyPayment("payment-1", 100, "PIX", time.Now()); err == nil {
		t.Fatal("expected reversed charge payment to fail")
	}
}

func TestReversedChargeRemainsClosedAcrossLifecycleAndDunning(t *testing.T) {
	c := newPaymentTestCharge(t)
	c.DueDate = time.Now().Add(-10 * 24 * time.Hour)
	c.Status = StatusReversed
	beforeStatus := c.Status
	beforeTotal := c.TotalAmount.Cents()

	if err := c.Cancel(); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Cancel() error = %v, want ErrInvalidState", err)
	}
	if err := c.MarkOverdue(); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("MarkOverdue() error = %v, want ErrInvalidState", err)
	}
	c.CalculateLateFees(2, 1)

	if c.Status != beforeStatus {
		t.Fatalf("terminal status reopened: got %s, want %s", c.Status, beforeStatus)
	}
	if c.LateFee.Cents() != 0 || c.Interest.Cents() != 0 || c.TotalAmount.Cents() != beforeTotal {
		t.Fatalf("terminal charge mutated by late fees: late=%d interest=%d total=%d", c.LateFee.Cents(), c.Interest.Cents(), c.TotalAmount.Cents())
	}
	if got := c.DaysOverdue(); got != 0 {
		t.Fatalf("DaysOverdue() = %d for REVERSED charge, want 0", got)
	}
	if IsOutstandingStatus(c.Status) || !IsClosedStatus(c.Status) {
		t.Fatalf("REVERSED classification is inconsistent: outstanding=%v closed=%v", IsOutstandingStatus(c.Status), IsClosedStatus(c.Status))
	}
}

func TestCancelRejectsPartiallyPaidCharge(t *testing.T) {
	c := newPaymentTestCharge(t)
	if _, err := c.ApplyPayment("payment-1", 40, "PIX", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := c.Cancel(); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Cancel() error = %v, want ErrInvalidState", err)
	}
	if c.Status != StatusPartiallyPaid || c.PaidAmount == nil || c.PaidAmount.Cents() != 4000 {
		t.Fatalf("partial payment was mutated by cancellation: status=%s paid=%v", c.Status, c.PaidAmount)
	}
}

func TestMarkOverduePreservesPartiallyPaidStatus(t *testing.T) {
	c := newPaymentTestCharge(t)
	c.DueDate = time.Now().Add(-5 * 24 * time.Hour)
	if _, err := c.ApplyPayment("payment-1", 40, "PIX", time.Now()); err != nil {
		t.Fatal(err)
	}

	if err := c.MarkOverdue(); err != nil {
		t.Fatalf("MarkOverdue() error = %v", err)
	}
	if c.Status != StatusPartiallyPaid {
		t.Fatalf("MarkOverdue() lost partial state: got %s", c.Status)
	}
	if got := c.DaysOverdue(); got < 4 {
		t.Fatalf("partially-paid charge should still report lateness by due date, got %d days", got)
	}
}

func TestNewAllocationValidatesInputs(t *testing.T) {
	if _, err := NewAllocation("", "item-1", "owner-1", AllocationTypeOwnerPayable, 100, "rule-1"); err == nil {
		t.Fatal("expected error for empty payment_id")
	}
	if _, err := NewAllocation("pay-1", "", "owner-1", AllocationTypeOwnerPayable, 100, "rule-1"); err == nil {
		t.Fatal("expected error for empty charge_item_id")
	}
	if _, err := NewAllocation("pay-1", "item-1", "", AllocationTypeOwnerPayable, 100, "rule-1"); err == nil {
		t.Fatal("expected error for empty beneficiary_party_id")
	}
	if _, err := NewAllocation("pay-1", "item-1", "owner-1", AllocationType("bogus"), 100, "rule-1"); err == nil {
		t.Fatal("expected error for unknown allocation_type")
	}
	if _, err := NewAllocation("pay-1", "item-1", "owner-1", AllocationTypeOwnerPayable, 0, "rule-1"); err == nil {
		t.Fatal("expected error for non-positive amount")
	}
	if _, err := NewAllocation("pay-1", "item-1", "owner-1", AllocationTypeOwnerPayable, 100, ""); err == nil {
		t.Fatal("expected error for empty rule_version_id")
	}

	a, err := NewAllocation("pay-1", "item-1", "owner-1", AllocationTypeOwnerPayable, 100, "rule-1")
	if err != nil {
		t.Fatalf("NewAllocation() error = %v", err)
	}
	if a.ID == "" || a.PaymentID != "pay-1" || a.ChargeItemID != "item-1" ||
		a.BeneficiaryPartyID != "owner-1" || a.AllocationType != AllocationTypeOwnerPayable ||
		a.AmountCents != 100 || a.RuleVersionID != "rule-1" {
		t.Fatalf("unexpected allocation: %#v", a)
	}
}

func TestLineItemIdentitiesAreStableAcrossRecurringChargeRebuilds(t *testing.T) {
	items := []LineItem{{
		Description:     "Aluguel",
		AmountCents:     10000,
		SourceType:      "rental_base",
		SourceID:        "cfg-1",
		EconomicOwnerID: "owner-1",
		SettlementMode:  SettlementModeCollection,
	}}
	first, err := NewWithLineItems("contract-1", "payer-1", "owner-1", "property-1", "tenant-1", TypeRent, "Rent", "2026-08", 100, time.Now(), items)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewWithLineItems("contract-1", "payer-1", "owner-1", "property-1", "tenant-1", TypeRent, "Rent", "2026-08", 100, time.Now(), items)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("test setup expected distinct aggregate IDs")
	}
	if first.LineItems[0].ID == "" || first.LineItems[0].ID != second.LineItems[0].ID {
		t.Fatalf("line item IDs are not stable: %q != %q", first.LineItems[0].ID, second.LineItems[0].ID)
	}
	if first.LineItems[0].RuleVersionID == "" || first.LineItems[0].RuleVersionID != second.LineItems[0].RuleVersionID {
		t.Fatalf("fallback rule versions are not stable: %q != %q", first.LineItems[0].RuleVersionID, second.LineItems[0].RuleVersionID)
	}
}

func TestValidateAllocationsReconcilesWithPayment(t *testing.T) {
	allocations := []Allocation{
		{ID: "a-1", PaymentID: "pay-1", ChargeItemID: "item-1", AmountCents: 60, AllocationType: AllocationTypeOwnerPayable, BeneficiaryPartyID: "owner-1", RuleVersionID: "rule-1"},
		{ID: "a-2", PaymentID: "pay-1", ChargeItemID: "item-1", AmountCents: 40, AllocationType: AllocationTypeRevenue, BeneficiaryPartyID: "imob-1", RuleVersionID: "rule-1"},
	}
	if err := ValidateAllocations(nil, 100); err != nil {
		t.Fatalf("empty allocations should be valid: %v", err)
	}
	if err := ValidateAllocations(allocations, 100); err != nil {
		t.Fatalf("balanced allocations should be valid: %v", err)
	}
	if err := ValidateAllocations(allocations, 101); err == nil {
		t.Fatal("expected error when allocations do not reconcile with payment")
	}

	bad := append([]Allocation(nil), allocations...)
	bad[0].AllocationType = AllocationType("bogus")
	if err := ValidateAllocations(bad, 100); err == nil {
		t.Fatal("expected error for unknown allocation_type in slice")
	}
}
