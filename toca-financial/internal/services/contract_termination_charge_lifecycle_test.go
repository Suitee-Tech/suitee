package services

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
)

type terminationChargeRepo struct {
	items   []*charge.Charge
	updates int
}

func (r *terminationChargeRepo) Create(context.Context, *charge.Charge) error { return nil }

func (r *terminationChargeRepo) Update(_ context.Context, candidate *charge.Charge) error {
	r.updates++
	for index, item := range r.items {
		if item.ID == candidate.ID {
			r.items[index] = candidate
		}
	}
	return nil
}

func (r *terminationChargeRepo) List(_ context.Context, _, _ string, _, _ string, limit, offset int64) ([]*charge.Charge, int64, error) {
	if offset >= int64(len(r.items)) {
		return nil, int64(len(r.items)), nil
	}
	end := offset + limit
	if end > int64(len(r.items)) {
		end = int64(len(r.items))
	}
	return r.items[offset:end], int64(len(r.items)), nil
}

func terminationCharge(t *testing.T, id string, status charge.Status, due time.Time) *charge.Charge {
	t.Helper()
	c, err := charge.New("contract-1", "payer-1", "owner-1", "property-1", "tenant-1", charge.TypeRent, "Aluguel", due.Format("2006-01"), 100, due)
	if err != nil {
		t.Fatalf("create charge %s: %v", id, err)
	}
	c.ID = id
	c.Status = status
	if status == charge.StatusPartiallyPaid {
		if _, err := c.ApplyPayment("partial-payment", 40, "PIX", due.Add(-time.Hour)); err != nil {
			t.Fatalf("apply partial payment: %v", err)
		}
	}
	return c
}

func TestContractTerminationCancelsOnlyPendingChargesAfterEffectiveDate(t *testing.T) {
	terminationDate := time.Date(2026, time.August, 15, 18, 30, 0, 0, time.FixedZone("BRT", -3*60*60))
	repo := &terminationChargeRepo{items: []*charge.Charge{
		terminationCharge(t, "pending-future", charge.StatusPending, time.Date(2026, time.August, 16, 0, 0, 0, 0, time.UTC)),
		terminationCharge(t, "pending-same-day", charge.StatusPending, time.Date(2026, time.August, 15, 23, 0, 0, 0, time.UTC)),
		terminationCharge(t, "pending-past", charge.StatusPending, time.Date(2026, time.August, 14, 0, 0, 0, 0, time.UTC)),
		terminationCharge(t, "overdue", charge.StatusOverdue, time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)),
		terminationCharge(t, "partial-future", charge.StatusPartiallyPaid, time.Date(2026, time.August, 16, 0, 0, 0, 0, time.UTC)),
		terminationCharge(t, "paid-future", charge.StatusPaid, time.Date(2026, time.August, 16, 0, 0, 0, 0, time.UTC)),
	}}
	handler := NewContractTerminatedEventHandler(nil, repo, nil, nil, nil)
	event := contracts.ContractTerminatedEvent{
		EventIDVal: "termination-1",
		Data: contracts.ContractTerminatedEventPayload{
			TenantID:        "tenant-1",
			ContractID:      "contract-1",
			TerminationDate: terminationDate.Format(time.RFC3339),
		},
	}

	if err := handler(context.Background(), event); err != nil {
		t.Fatalf("contract termination: %v", err)
	}
	if repo.updates != 1 {
		t.Fatalf("updates = %d, want exactly one future cancellation", repo.updates)
	}
	want := map[string]charge.Status{
		"pending-future":   charge.StatusCancelled,
		"pending-same-day": charge.StatusPending,
		"pending-past":     charge.StatusPending,
		"overdue":          charge.StatusOverdue,
		"partial-future":   charge.StatusPartiallyPaid,
		"paid-future":      charge.StatusPaid,
	}
	for _, item := range repo.items {
		if item.Status != want[item.ID] {
			t.Errorf("charge %s status = %s, want %s", item.ID, item.Status, want[item.ID])
		}
	}
}

func TestContractTerminationRequiresEffectiveDateBeforeCancelling(t *testing.T) {
	repo := &terminationChargeRepo{items: []*charge.Charge{
		terminationCharge(t, "pending-future", charge.StatusPending, time.Now().AddDate(0, 1, 0)),
	}}
	handler := NewContractTerminatedEventHandler(nil, repo, nil, nil, nil)
	event := contracts.ContractTerminatedEvent{
		EventIDVal: "termination-without-date",
		Data:       contracts.ContractTerminatedEventPayload{TenantID: "tenant-1", ContractID: "contract-1"},
	}

	if err := handler(context.Background(), event); err == nil {
		t.Fatal("expected invalid termination_date error")
	}
	if repo.updates != 0 || repo.items[0].Status != charge.StatusPending {
		t.Fatalf("missing termination date mutated charge: updates=%d status=%s", repo.updates, repo.items[0].Status)
	}
}
