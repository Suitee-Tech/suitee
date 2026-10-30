package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
)

type fakeJournalRepo struct {
	byID      map[string]*journal.Entry
	bySource  map[string]*journal.Entry
	createErr error
	updateOK  bool
}

func newFakeJournalRepo() *fakeJournalRepo {
	return &fakeJournalRepo{
		byID:     make(map[string]*journal.Entry),
		bySource: make(map[string]*journal.Entry),
		updateOK: true,
	}
}

func (f *fakeJournalRepo) Create(_ context.Context, e *journal.Entry) error {
	if f.createErr != nil {
		return f.createErr
	}
	if e.SourceEventID != "" {
		if _, ok := f.bySource[e.SourceEventID]; ok {
			return journal.ErrEntryAlreadyExists
		}
		f.bySource[e.SourceEventID] = e
	}
	f.byID[e.ID] = e
	return nil
}

func (f *fakeJournalRepo) GetByID(_ context.Context, tenantID, id string) (*journal.Entry, error) {
	e, ok := f.byID[id]
	if !ok || e.TenantID != tenantID {
		return nil, nil
	}
	return e, nil
}

func (f *fakeJournalRepo) GetBySourceEventID(_ context.Context, tenantID, sourceEventID string) (*journal.Entry, error) {
	e, ok := f.bySource[sourceEventID]
	if !ok || e.TenantID != tenantID {
		return nil, nil
	}
	return e, nil
}

func (f *fakeJournalRepo) UpdateStatus(_ context.Context, e *journal.Entry, _ time.Time) (bool, error) {
	if !f.updateOK {
		return false, nil
	}
	if existing, ok := f.byID[e.ID]; ok {
		existing.Status = e.Status
		existing.UpdatedAt = e.UpdatedAt
		existing.ReconciliationDate = e.ReconciliationDate
	}
	return true, nil
}

func (f *fakeJournalRepo) List(_ context.Context, tenantID, contractID string, limit, offset int64) ([]*journal.Entry, int64, error) {
	out := make([]*journal.Entry, 0, len(f.byID))
	for _, e := range f.byID {
		out = append(out, e)
	}
	return out, int64(len(out)), nil
}

func (f *fakeJournalRepo) AccountBalance(_ context.Context, tenantID string, account journal.Account) (int64, error) {
	var sum int64
	for _, e := range f.byID {
		for _, leg := range e.Legs {
			if leg.Account != account {
				continue
			}
			if leg.Direction == journal.Debit {
				sum += leg.AmountCents
			} else {
				sum -= leg.AmountCents
			}
		}
	}
	return sum, nil
}

func commissionEntry(tenantID string) *journal.Entry {
	now := time.Now().UTC()
	e, err := journal.New(tenantID, "contract-1", "comissão", now, []journal.Leg{
		{Account: journal.AccountCommissionExpense, Direction: journal.Debit, AmountCents: 15000},
		{Account: journal.AccountCommissionPayable, Direction: journal.Credit, AmountCents: 15000},
	})
	if err != nil {
		panic(err)
	}
	return e
}

func TestJournalService_CreateRejectsUnbalanced(t *testing.T) {
	svc := NewJournalService(newFakeJournalRepo())
	unbalanced := &journal.Entry{
		ID:       "x",
		TenantID: "t1",
		Status:   journal.StatusPosted,
		Legs: []journal.Leg{
			{Account: journal.AccountCash, Direction: journal.Debit, AmountCents: 100},
			{Account: journal.AccountRevenue, Direction: journal.Credit, AmountCents: 99},
		},
	}
	err := svc.Create(context.Background(), unbalanced)
	if !errors.Is(err, journal.ErrUnbalanced) {
		t.Fatalf("expected ErrUnbalanced, got %v", err)
	}
}

func TestJournalService_CreateWithSourceEventID_Idempotent(t *testing.T) {
	repo := newFakeJournalRepo()
	svc := NewJournalService(repo)
	ctx := context.Background()

	first := commissionEntry("t1")
	if err := svc.CreateWithSourceEventID(ctx, "evt-1", first); err != nil {
		t.Fatalf("first create: %v", err)
	}
	second := commissionEntry("t1")
	if err := svc.CreateWithSourceEventID(ctx, "evt-1", second); err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	if len(repo.bySource) != 1 {
		t.Fatalf("expected exactly one source entry, got %d", len(repo.bySource))
	}
}

func TestJournalService_CreateCanonicalWithSourceEventIDRejectsDivergentPosting(t *testing.T) {
	repo := newFakeJournalRepo()
	svc := NewJournalService(repo)
	ctx := context.Background()
	first := commissionEntry("t1")
	first.Date = time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	canonical, err := svc.CreateCanonicalWithSourceEventID(ctx, "evt-managed-bill-1", first)
	if err != nil {
		t.Fatalf("first canonical create: %v", err)
	}

	retry := *first
	retry.ID = "retry-entry-id"
	retry.Legs = append([]journal.Leg(nil), first.Legs...)
	got, err := svc.CreateCanonicalWithSourceEventID(ctx, "evt-managed-bill-1", &retry)
	if err != nil || got.ID != canonical.ID || len(repo.byID) != 1 {
		t.Fatalf("exact retry: got=%+v rows=%d err=%v", got, len(repo.byID), err)
	}

	divergent := retry
	divergent.ID = "divergent-entry-id"
	divergent.Legs = append([]journal.Leg(nil), retry.Legs...)
	for index := range divergent.Legs {
		divergent.Legs[index].AmountCents++
	}
	if _, err := svc.CreateCanonicalWithSourceEventID(ctx, "evt-managed-bill-1", &divergent); !errors.Is(err, journal.ErrSourceEventConflict) {
		t.Fatalf("divergent retry: got %v, want ErrSourceEventConflict", err)
	}
}

func TestJournalService_MarkReconciledBySourceEventIDIsTenantScopedAndIdempotent(t *testing.T) {
	repo := newFakeJournalRepo()
	svc := NewJournalService(repo)
	ctx := context.Background()
	entry := commissionEntry("tenant-1")
	if _, err := svc.CreateCanonicalWithSourceEventID(ctx, "evt-payment-1", entry); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MarkReconciledBySourceEventID(ctx, "tenant-2", "evt-payment-1", time.Now()); err == nil {
		t.Fatal("another tenant must not reconcile the entry")
	}
	reconciledAt := time.Date(2026, time.August, 13, 14, 0, 0, 0, time.UTC)
	reconciled, err := svc.MarkReconciledBySourceEventID(ctx, "tenant-1", "evt-payment-1", reconciledAt)
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.ReconciliationDate == nil || !reconciled.ReconciliationDate.Equal(reconciledAt) {
		t.Fatalf("reconciliation date = %v, want %v", reconciled.ReconciliationDate, reconciledAt)
	}
	retryAt := reconciledAt.Add(time.Hour)
	retry, err := svc.MarkReconciledBySourceEventID(ctx, "tenant-1", "evt-payment-1", retryAt)
	if err != nil {
		t.Fatal(err)
	}
	if retry.ReconciliationDate == nil || !retry.ReconciliationDate.Equal(reconciledAt) {
		t.Fatalf("retry replaced first reconciliation date: %v", retry.ReconciliationDate)
	}
}

func TestJournalService_Reverse_BalancedAndIdempotent(t *testing.T) {
	repo := newFakeJournalRepo()
	svc := NewJournalService(repo)
	ctx := context.Background()

	original := commissionEntry("t1")
	if err := svc.Create(ctx, original); err != nil {
		t.Fatalf("create original: %v", err)
	}

	effectiveAt := time.Date(2026, time.August, 14, 23, 59, 59, 654321000, time.UTC)
	reversed, err := svc.ReverseAt(ctx, original.ID, "t1", "correção", "op-1", effectiveAt)
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if reversed.ReversalOf != original.ID {
		t.Fatalf("expected ReversalOf %s, got %s", original.ID, reversed.ReversalOf)
	}
	if !reversed.Balanced() {
		t.Fatal("reversal must be balanced")
	}
	if reversed.ReversedBy != "op-1" || !reversed.Date.Equal(effectiveAt) {
		t.Fatalf("reversal audit identity = by=%q date=%v", reversed.ReversedBy, reversed.Date)
	}

	// Original deve estar REVERSED.
	stored, _ := repo.GetByID(ctx, "t1", original.ID)
	if stored.Status != journal.StatusReversed {
		t.Fatalf("expected REVERSED, got %s", stored.Status)
	}

	// Rechamada idempotente retorna o mesmo reversal.
	again, err := svc.ReverseAt(ctx, original.ID, "t1", "correção", "op-1", effectiveAt)
	if err != nil {
		t.Fatalf("idempotent reverse: %v", err)
	}
	if again.ID != reversed.ID {
		t.Fatalf("expected same reversal id %s, got %s", reversed.ID, again.ID)
	}
	if _, err := svc.ReverseAt(ctx, original.ID, "t1", "correção", "op-1", effectiveAt.Add(time.Second)); !errors.Is(err, journal.ErrSourceEventConflict) {
		t.Fatalf("divergent effective date retry: got %v, want ErrSourceEventConflict", err)
	}
}

func TestJournalService_ReverseRejectsDivergentExistingReversal(t *testing.T) {
	repo := newFakeJournalRepo()
	svc := NewJournalService(repo)
	ctx := context.Background()

	original := commissionEntry("t1")
	if err := svc.Create(ctx, original); err != nil {
		t.Fatalf("create original: %v", err)
	}
	reversed, err := svc.Reverse(ctx, original.ID, "t1", "correção", "op-1")
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if _, err := svc.Reverse(ctx, original.ID, "t1", "motivo diferente", "op-1"); !errors.Is(err, journal.ErrSourceEventConflict) {
		t.Fatalf("divergent reason retry: got %v, want ErrSourceEventConflict", err)
	}
	if _, err := svc.Reverse(ctx, original.ID, "t1", "correção", "op-2"); !errors.Is(err, journal.ErrSourceEventConflict) {
		t.Fatalf("divergent operator retry: got %v, want ErrSourceEventConflict", err)
	}
	reversed.Description = "Estorno divergente"

	if _, err := svc.Reverse(ctx, original.ID, "t1", "correção", "op-1"); !errors.Is(err, journal.ErrSourceEventConflict) {
		t.Fatalf("corrupted canonical retry: got %v, want ErrSourceEventConflict", err)
	}
}
