package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
)

type concurrentPayment struct {
	id     string
	amount float64
	method string
	paidAt time.Time
}

// optimisticChargeRepo mimics the PostgreSQL compare-and-swap contract. Each
// read returns a detached snapshot so a failed CAS cannot leak the caller's
// in-memory mutation into persisted state.
type optimisticChargeRepo struct {
	*mockChargeRepo
	stored         *charge.Charge
	casCalls       int
	inject         *concurrentPayment
	injected       bool
	alwaysConflict bool
}

func clonePaymentCharge(c *charge.Charge) *charge.Charge {
	if c == nil {
		return nil
	}
	cloned := *c
	cloned.Payments = append([]charge.PaymentEntry(nil), c.Payments...)
	cloned.LineItems = append([]charge.LineItem(nil), c.LineItems...)
	cloned.AppliedEventIDs = append([]string(nil), c.AppliedEventIDs...)
	cloned.RemindersSent = append([]charge.ReminderEntry(nil), c.RemindersSent...)
	if c.PaidAmount != nil {
		paid := *c.PaidAmount
		cloned.PaidAmount = &paid
	}
	if c.PaidAt != nil {
		paidAt := *c.PaidAt
		cloned.PaidAt = &paidAt
	}
	return &cloned
}

func (r *optimisticChargeRepo) GetByID(_ context.Context, tenantID, id string) (*charge.Charge, error) {
	if r.stored == nil || r.stored.TenantID != tenantID || r.stored.ID != id {
		return nil, nil
	}
	return clonePaymentCharge(r.stored), nil
}

func (r *optimisticChargeRepo) Update(_ context.Context, _ *charge.Charge) error {
	return errors.New("non-CAS Update must not be used by optimistic repository")
}

func (r *optimisticChargeRepo) UpdateIfVersion(_ context.Context, candidate *charge.Charge, expected time.Time) (bool, error) {
	r.casCalls++
	if r.stored == nil || !r.stored.UpdatedAt.Equal(expected) {
		return false, nil
	}
	if r.inject != nil && !r.injected {
		if _, err := r.stored.ApplyPayment(r.inject.id, r.inject.amount, r.inject.method, r.inject.paidAt); err != nil {
			return false, err
		}
		r.injected = true
		return false, nil
	}
	if r.alwaysConflict {
		return false, nil
	}
	r.stored = clonePaymentCharge(candidate)
	return true, nil
}

func newCASPaymentCharge(t *testing.T) *charge.Charge {
	t.Helper()
	c, err := charge.New(
		"contract-cas",
		"payer-cas",
		"owner-cas",
		"property-cas",
		"tenant-cas",
		charge.TypeRent,
		"Aluguel",
		"2026-08",
		100,
		time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("create charge fixture: %v", err)
	}
	c.ID = "charge-cas"
	c.UpdatedAt = time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	return c
}

func TestMarkPaidIdempotentRetriesCASWithoutLosingConcurrentPayment(t *testing.T) {
	paidAt := time.Date(2026, time.August, 10, 13, 0, 0, 0, time.UTC)
	repo := &optimisticChargeRepo{
		mockChargeRepo: &mockChargeRepo{},
		stored:         newCASPaymentCharge(t),
		inject: &concurrentPayment{
			id: "payment-external", amount: 30, method: "TED", paidAt: paidAt,
		},
	}
	svc := NewChargeService(repo, nil)

	got, err := svc.MarkPaidIdempotent(
		context.Background(),
		repo.stored.TenantID,
		repo.stored.ID,
		"PIX",
		40,
		"payment-requested",
		paidAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("mark paid after CAS conflict: %v", err)
	}

	if repo.casCalls != 2 {
		t.Fatalf("expected one conflict plus one successful CAS, got %d calls", repo.casCalls)
	}
	if got.PaidAmount == nil || got.PaidAmount.Cents() != 7000 {
		t.Fatalf("expected canonical accumulated amount of 7000 cents, got %+v", got.PaidAmount)
	}
	if len(got.Payments) != 2 || len(repo.stored.Payments) != 2 {
		t.Fatalf("expected both payment entries in result and storage, got result=%d stored=%d", len(got.Payments), len(repo.stored.Payments))
	}
	if got.Status != charge.StatusPartiallyPaid || repo.stored.Status != charge.StatusPartiallyPaid {
		t.Fatalf("expected PARTIALLY_PAID result/storage, got %s/%s", got.Status, repo.stored.Status)
	}
}

func TestMarkPaidIdempotentTreatsConcurrentSameCommandAsCanonicalRetry(t *testing.T) {
	paidAt := time.Date(2026, time.August, 10, 13, 0, 0, 0, time.UTC)
	repo := &optimisticChargeRepo{
		mockChargeRepo: &mockChargeRepo{},
		stored:         newCASPaymentCharge(t),
		inject: &concurrentPayment{
			id: "payment-same", amount: 40, method: "PIX", paidAt: paidAt,
		},
	}
	svc := NewChargeService(repo, nil)

	got, err := svc.MarkPaidIdempotent(
		context.Background(),
		repo.stored.TenantID,
		repo.stored.ID,
		"PIX",
		40,
		"payment-same",
		paidAt,
	)
	if err != nil {
		t.Fatalf("idempotent concurrent retry: %v", err)
	}

	if repo.casCalls != 1 {
		t.Fatalf("expected only the losing CAS before canonical reread, got %d", repo.casCalls)
	}
	if len(got.Payments) != 1 || got.PaidAmount == nil || got.PaidAmount.Cents() != 4000 {
		t.Fatalf("expected one canonical 4000-cent payment, got payments=%d paid=%+v", len(got.Payments), got.PaidAmount)
	}
}

func TestMarkPaidIdempotentReturnsConcurrentModificationAfterRetryBudget(t *testing.T) {
	repo := &optimisticChargeRepo{
		mockChargeRepo: &mockChargeRepo{},
		stored:         newCASPaymentCharge(t),
		alwaysConflict: true,
	}
	svc := NewChargeService(repo, nil)

	_, err := svc.MarkPaidIdempotent(
		context.Background(),
		repo.stored.TenantID,
		repo.stored.ID,
		"PIX",
		40,
		"payment-never-wins",
		time.Date(2026, time.August, 10, 13, 0, 0, 0, time.UTC),
	)

	if !errors.Is(err, charge.ErrConcurrentModification) {
		t.Fatalf("expected ErrConcurrentModification, got %v", err)
	}
	if repo.casCalls != 8 {
		t.Fatalf("expected eight bounded CAS attempts, got %d", repo.casCalls)
	}
	if len(repo.stored.Payments) != 0 || repo.stored.PaidAmount != nil || repo.stored.Status != charge.StatusPending {
		t.Fatalf("failed CAS leaked mutation into persisted state: %+v", repo.stored)
	}
}
