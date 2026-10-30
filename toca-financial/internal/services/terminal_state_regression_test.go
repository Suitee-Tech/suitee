package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/collection"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
)

type staleDelinquencyChargeRepo struct {
	*mockChargeRepo
	tenantIDs   []string
	updateCalls int
}

type conflictingVersionedChargeRepo struct {
	*staleDelinquencyChargeRepo
	casCalls int
}

func (r *conflictingVersionedChargeRepo) UpdateIfVersion(_ context.Context, _ *charge.Charge, _ time.Time) (bool, error) {
	r.casCalls++
	return false, nil
}

func (r *staleDelinquencyChargeRepo) ListDistinctTenantIDs(context.Context) ([]string, error) {
	return r.tenantIDs, nil
}

func (r *staleDelinquencyChargeRepo) Update(_ context.Context, c *charge.Charge) error {
	r.updateCalls++
	r.ch = c
	return nil
}

func TestDelinquencyEngineSkipsStaleReversedCharge(t *testing.T) {
	c := makeChargeDue(t, "ch-reversed", time.Now().Add(-10*24*time.Hour), charge.StatusReversed)
	repo := &staleDelinquencyChargeRepo{
		mockChargeRepo: &mockChargeRepo{charges: []*charge.Charge{c}},
		tenantIDs:      []string{"t-1"},
	}
	svc := NewDelinquencyService(repo, &stubCollectionRepo{}, nil, DefaultDelinquencyConfig())

	result, err := svc.ProcessOverdueCharges(context.Background())
	if err != nil {
		t.Fatalf("ProcessOverdueCharges() error = %v", err)
	}
	if result.ChargesMarkedOverdue != 0 || result.LateFessCalculated != 0 || repo.updateCalls != 0 {
		t.Fatalf("REVERSED charge was processed: result=%+v updates=%d", result, repo.updateCalls)
	}
	if c.Status != charge.StatusReversed || c.LateFee.Cents() != 0 || c.Interest.Cents() != 0 {
		t.Fatalf("REVERSED charge mutated: status=%s late=%d interest=%d", c.Status, c.LateFee.Cents(), c.Interest.Cents())
	}
}

func TestDelinquencyEngineDoesNotOverwriteConcurrentLifecycleChange(t *testing.T) {
	c := makeChargeDue(t, "ch-pending", time.Now().Add(-10*24*time.Hour), charge.StatusPending)
	base := &staleDelinquencyChargeRepo{
		mockChargeRepo: &mockChargeRepo{charges: []*charge.Charge{c}},
		tenantIDs:      []string{"t-1"},
	}
	repo := &conflictingVersionedChargeRepo{staleDelinquencyChargeRepo: base}
	svc := NewDelinquencyService(repo, &stubCollectionRepo{}, nil, DefaultDelinquencyConfig())

	result, err := svc.ProcessOverdueCharges(context.Background())
	if err != nil {
		t.Fatalf("ProcessOverdueCharges() error = %v", err)
	}
	if repo.casCalls != 1 || repo.updateCalls != 0 {
		t.Fatalf("expected one CAS and no blind update, cas=%d update=%d", repo.casCalls, repo.updateCalls)
	}
	if result.ChargesMarkedOverdue != 0 || result.LateFessCalculated != 0 {
		t.Fatalf("conflicted write was counted as persisted: %+v", result)
	}
}

func TestDelinquencyEnginePreservesPartiallyPaidStatus(t *testing.T) {
	c := makeChargeDue(t, "ch-partial", time.Now().Add(-10*24*time.Hour), charge.StatusPending)
	if _, err := c.ApplyPayment("payment-1", 900, "PIX", time.Now()); err != nil {
		t.Fatalf("ApplyPayment() error = %v", err)
	}
	repo := &staleDelinquencyChargeRepo{
		mockChargeRepo: &mockChargeRepo{charges: []*charge.Charge{c}},
		tenantIDs:      []string{"t-1"},
	}
	svc := NewDelinquencyService(repo, &stubCollectionRepo{}, nil, DefaultDelinquencyConfig())

	result, err := svc.ProcessOverdueCharges(context.Background())
	if err != nil {
		t.Fatalf("ProcessOverdueCharges() error = %v", err)
	}
	if c.Status != charge.StatusPartiallyPaid {
		t.Fatalf("partial charge status = %s, want PARTIALLY_PAID", c.Status)
	}
	if result.ChargesMarkedOverdue != 0 || result.LateFessCalculated != 1 {
		t.Fatalf("result = %+v, want fees without PENDING→OVERDUE count", result)
	}
}

func TestChargeServiceLifecycleMutationsRejectConcurrentWrite(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status charge.Status
		act    func(*ChargeService) error
	}{
		{
			name:   "update",
			status: charge.StatusPending,
			act: func(svc *ChargeService) error {
				return svc.Update(context.Background(), svc.repo.(*conflictingVersionedChargeRepo).ch)
			},
		},
		{
			name:   "cancel",
			status: charge.StatusPending,
			act: func(svc *ChargeService) error {
				_, err := svc.Cancel(context.Background(), "t-1", "ch-1", "reason")
				return err
			},
		},
		{
			name:   "reverse",
			status: charge.StatusPaid,
			act: func(svc *ChargeService) error {
				_, err := svc.Reverse(context.Background(), "t-1", "ch-1", "reason")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := makeChargeDue(t, "ch-1", time.Now().Add(24*time.Hour), tc.status)
			base := &staleDelinquencyChargeRepo{mockChargeRepo: &mockChargeRepo{ch: c}}
			repo := &conflictingVersionedChargeRepo{staleDelinquencyChargeRepo: base}
			svc := NewChargeService(repo, nil)

			err := tc.act(svc)
			if !errors.Is(err, charge.ErrConcurrentModification) {
				t.Fatalf("operation error = %v, want ErrConcurrentModification", err)
			}
			if repo.casCalls != 1 || repo.updateCalls != 0 {
				t.Fatalf("expected one CAS and no blind update, cas=%d update=%d", repo.casCalls, repo.updateCalls)
			}
		})
	}
}

func TestChargeServiceReverseRetryDoesNotWriteAgain(t *testing.T) {
	c := makeChargeDue(t, "ch-reversed", time.Now().Add(24*time.Hour), charge.StatusReversed)
	base := &staleDelinquencyChargeRepo{mockChargeRepo: &mockChargeRepo{ch: c}}
	repo := &conflictingVersionedChargeRepo{staleDelinquencyChargeRepo: base}
	svc := NewChargeService(repo, nil)

	got, err := svc.Reverse(context.Background(), "t-1", c.ID, "same retry")
	if err != nil || got != c {
		t.Fatalf("Reverse() retry = (%v, %v), want existing charge and nil", got, err)
	}
	if repo.casCalls != 0 || repo.updateCalls != 0 {
		t.Fatalf("idempotent reverse retried a write: cas=%d update=%d", repo.casCalls, repo.updateCalls)
	}
}

func TestResolveChargePaymentTreatsReversedAsClosed(t *testing.T) {
	reversed := makeChargeDue(t, "ch-reversed", time.Now().Add(-10*24*time.Hour), charge.StatusReversed)
	paid := makeChargeDue(t, "ch-paid", time.Now().Add(-5*24*time.Hour), charge.StatusPaid)
	coll := collection.New("contract-1", "payer-1", "Payer", "t-1", 2500, 10, []string{reversed.ID, paid.ID})
	chargeRepo := &mockChargeRepo{charges: []*charge.Charge{reversed, paid}}
	collectionRepo := &stubCollectionRepo{byCharge: map[string]*collection.Collection{paid.ID: coll}}
	svc := NewDelinquencyService(chargeRepo, collectionRepo, nil, DefaultDelinquencyConfig())

	if err := svc.ResolveChargePayment(context.Background(), "t-1", paid.ID); err != nil {
		t.Fatalf("ResolveChargePayment() error = %v", err)
	}
	if coll.Status != collection.StatusResolved || coll.TotalDebt != 0 {
		t.Fatalf("collection not resolved with only closed charges: status=%s debt=%v", coll.Status, coll.TotalDebt)
	}
}

func TestEnsureCollectionUsesRemainingPartiallyPaidBalance(t *testing.T) {
	partial := makeChargeDue(t, "ch-partial", time.Now().Add(-10*24*time.Hour), charge.StatusPending)
	if _, err := partial.ApplyPayment("payment-1", 900, "PIX", time.Now()); err != nil {
		t.Fatalf("ApplyPayment() error = %v", err)
	}
	if partial.Status != charge.StatusPartiallyPaid {
		t.Fatalf("fixture status = %s, want PARTIALLY_PAID", partial.Status)
	}
	coll := collection.New("contract-1", "p-1", "Payer", "t-1", partial.TotalAmount.Float(), 10, []string{partial.ID})
	chargeRepo := &mockChargeRepo{charges: []*charge.Charge{partial}}
	collectionRepo := &stubCollectionRepo{byCharge: map[string]*collection.Collection{partial.ID: coll}}
	svc := NewDelinquencyService(chargeRepo, collectionRepo, nil, DefaultDelinquencyConfig())
	result := &ProcessResult{}

	if err := svc.ensureCollectionCase(context.Background(), partial, result); err != nil {
		t.Fatalf("ensureCollectionCase() error = %v", err)
	}
	if coll.TotalDebt != 600 {
		t.Fatalf("collection debt = %.2f, want remaining 600.00", coll.TotalDebt)
	}
	if result.CollectionsUpdated != 1 {
		t.Fatalf("CollectionsUpdated = %d, want 1", result.CollectionsUpdated)
	}
}

func TestContractTerminationDoesNotCancelReversedCharge(t *testing.T) {
	c := makeChargeDue(t, "ch-reversed", time.Now().Add(30*24*time.Hour), charge.StatusReversed)
	repo := &staleDelinquencyChargeRepo{
		mockChargeRepo: &mockChargeRepo{charges: []*charge.Charge{c}, total: 1},
	}

	if err := cancelFutureCharges(context.Background(), repo, "t-1", "contract-1", time.Now()); err != nil {
		t.Fatalf("cancelFutureCharges() error = %v", err)
	}
	if repo.updateCalls != 0 || c.Status != charge.StatusReversed {
		t.Fatalf("contract termination reopened terminal charge: updates=%d status=%s", repo.updateCalls, c.Status)
	}
}

func TestRentalConfigEditDoesNotRewriteReversedCharge(t *testing.T) {
	oldDue := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	c := makeChargeDue(t, "ch-reversed", oldDue, charge.StatusReversed)
	c.ReferenceMonth = "2026-06"
	repo := &staleDelinquencyChargeRepo{
		mockChargeRepo: &mockChargeRepo{charges: []*charge.Charge{c}, total: 1},
	}
	cfg := &rental_charge_config.RentalChargeConfig{
		TenantID:        "t-1",
		ContractID:      "contract-1",
		StartDate:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		FirstChargeDate: oldDue.AddDate(0, 0, 5),
		DefaultDueDay:   15,
	}
	svc := NewRentalChargeConfigService(nil, repo, nil)

	if err := svc.updateChargeDates(context.Background(), cfg, oldDue); err != nil {
		t.Fatalf("updateChargeDates() error = %v", err)
	}
	if repo.updateCalls != 0 || !c.DueDate.Equal(oldDue) || c.Status != charge.StatusReversed {
		t.Fatalf("config edit mutated terminal charge: updates=%d due=%s status=%s", repo.updateCalls, c.DueDate, c.Status)
	}
}

func TestRentalConfigEditReturnsConflictInsteadOfOverwritingPayment(t *testing.T) {
	oldDue := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	c := makeChargeDue(t, "ch-pending", oldDue, charge.StatusPending)
	c.ReferenceMonth = "2026-06"
	base := &staleDelinquencyChargeRepo{
		mockChargeRepo: &mockChargeRepo{charges: []*charge.Charge{c}, total: 1},
	}
	repo := &conflictingVersionedChargeRepo{staleDelinquencyChargeRepo: base}
	cfg := &rental_charge_config.RentalChargeConfig{
		TenantID:        "t-1",
		ContractID:      "contract-1",
		StartDate:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		FirstChargeDate: oldDue.AddDate(0, 0, 5),
		DefaultDueDay:   15,
	}
	svc := NewRentalChargeConfigService(nil, repo, nil)

	err := svc.updateChargeDates(context.Background(), cfg, oldDue)
	if !errors.Is(err, charge.ErrConcurrentModification) {
		t.Fatalf("updateChargeDates() error = %v, want ErrConcurrentModification", err)
	}
	if repo.casCalls != 1 || repo.updateCalls != 0 {
		t.Fatalf("expected one CAS and no blind update, cas=%d update=%d", repo.casCalls, repo.updateCalls)
	}
}
