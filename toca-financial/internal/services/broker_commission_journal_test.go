package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

type recordingJournalRepo struct {
	created []*journal.Entry
}

func (r *recordingJournalRepo) Create(_ context.Context, e *journal.Entry) error {
	r.created = append(r.created, e)
	return nil
}

func (r *recordingJournalRepo) GetByID(_ context.Context, tenantID, id string) (*journal.Entry, error) {
	return nil, nil
}

func (r *recordingJournalRepo) GetBySourceEventID(_ context.Context, tenantID, sourceEventID string) (*journal.Entry, error) {
	return nil, nil
}

func (r *recordingJournalRepo) UpdateStatus(_ context.Context, e *journal.Entry, _ time.Time) (bool, error) {
	return true, nil
}

func (r *recordingJournalRepo) List(_ context.Context, tenantID, contractID string, limit, offset int64) ([]*journal.Entry, int64, error) {
	return nil, 0, nil
}

func (r *recordingJournalRepo) AccountBalance(_ context.Context, tenantID string, account journal.Account) (int64, error) {
	return 0, nil
}

// FIN-E2E-33: o handler de comissão passa a materializar também a perna de
// dupla entrada no journal (débito em commission_expense ↔ crédito em
// commission_payable), balanceada e idempotente por source_event_id.
func TestBrokerCommissionHandler_SALE_emitsBalancedJournalEntries(t *testing.T) {
	txRepo := &mockTxRepo{}
	commRepo := &mockCommissionRepo{}
	journalRepo := &recordingJournalRepo{}

	journalService := service.NewJournalService(journalRepo)
	h := service.NewBrokerCommissionHandler(
		service.NewTransactionService(txRepo),
		service.NewCommissionService(commRepo),
		nil,
	).WithJournalService(journalService)

	err := h.Handle(context.Background(), newSALEBrokerInput())
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if len(journalRepo.created) != 2 {
		t.Fatalf("expected 2 journal entries, got %d", len(journalRepo.created))
	}
	for _, entry := range journalRepo.created {
		if !entry.Balanced() {
			t.Fatalf("entry %s is not balanced", entry.ID)
		}
		if len(entry.Legs) != 2 {
			t.Fatalf("entry %s expected 2 legs, got %d", entry.ID, len(entry.Legs))
		}
		expense := entry.Legs[0]
		payable := entry.Legs[1]
		if expense.Account != journal.AccountCommissionExpense || expense.Direction != journal.Debit {
			t.Fatalf("entry %s first leg must be debit commission_expense, got %+v", entry.ID, expense)
		}
		if payable.Account != journal.AccountCommissionPayable || payable.Direction != journal.Credit {
			t.Fatalf("entry %s second leg must be credit commission_payable, got %+v", entry.ID, payable)
		}
		if expense.AmountCents != payable.AmountCents || expense.AmountCents != 10000 {
			t.Fatalf("entry %s legs must match the 100.00 commission, got debit=%d credit=%d", entry.ID, expense.AmountCents, payable.AmountCents)
		}
	}
}
