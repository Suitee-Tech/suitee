package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/withdrawal"
)

type fakeWithdrawalRepo struct {
	byID      map[string]*withdrawal.Withdrawal
	byExt     map[string]*withdrawal.Withdrawal
	createErr error
}

func newFakeWithdrawalRepo() *fakeWithdrawalRepo {
	return &fakeWithdrawalRepo{
		byID:  make(map[string]*withdrawal.Withdrawal),
		byExt: make(map[string]*withdrawal.Withdrawal),
	}
}

func (f *fakeWithdrawalRepo) Create(_ context.Context, w *withdrawal.Withdrawal) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.byID[w.ID] = w
	if w.ExternalID != "" {
		f.byExt[w.ExternalID] = w
	}
	return nil
}

func (f *fakeWithdrawalRepo) GetByID(_ context.Context, tenantID, id string) (*withdrawal.Withdrawal, error) {
	return f.byID[id], nil
}

func (f *fakeWithdrawalRepo) GetByExternalID(_ context.Context, tenantID, externalID string) (*withdrawal.Withdrawal, error) {
	return f.byExt[externalID], nil
}

func (f *fakeWithdrawalRepo) Update(_ context.Context, w *withdrawal.Withdrawal) error {
	f.byID[w.ID] = w
	return nil
}

func (f *fakeWithdrawalRepo) List(_ context.Context, tenantID, status string, limit, offset int64) ([]*withdrawal.Withdrawal, int64, error) {
	out := make([]*withdrawal.Withdrawal, 0, len(f.byID))
	for _, w := range f.byID {
		out = append(out, w)
	}
	return out, int64(len(out)), nil
}

// seedEquity cria um lançamento que credita a conta de patrimônio, tornando
// saldo disponível para saque. debit cash ↔ credit equity.
func seedEquity(t *testing.T, journalSvc *JournalService, tenantID string, cents int64) {
	t.Helper()
	entry, err := journal.New(tenantID, "", "seed equity", time.Now().UTC(), []journal.Leg{
		{Account: journal.AccountCash, Direction: journal.Debit, AmountCents: cents},
		{Account: journal.AccountEquity, Direction: journal.Credit, AmountCents: cents},
	})
	if err != nil {
		t.Fatalf("seed equity: %v", err)
	}
	if err := journalSvc.Create(context.Background(), entry); err != nil {
		t.Fatalf("seed equity create: %v", err)
	}
}

func journalEntryBySource(t *testing.T, repo *fakeJournalRepo, sourceEventID string) *journal.Entry {
	t.Helper()
	e, ok := repo.bySource[sourceEventID]
	if !ok {
		t.Fatalf("journal entry with source_event_id %q not found", sourceEventID)
	}
	return e
}

func assertTwoLegs(t *testing.T, e *journal.Entry, debitAccount journal.Account, creditAccount journal.Account, amount int64) {
	t.Helper()
	if !e.Balanced() {
		t.Fatalf("entry %s must be balanced", e.ID)
	}
	if len(e.Legs) != 2 {
		t.Fatalf("entry %s expected 2 legs, got %d", e.ID, len(e.Legs))
	}
	debit := e.Legs[0]
	credit := e.Legs[1]
	if debit.Account != debitAccount || debit.Direction != journal.Debit {
		t.Fatalf("entry %s first leg must be debit %s, got %+v", e.ID, debitAccount, debit)
	}
	if credit.Account != creditAccount || credit.Direction != journal.Credit {
		t.Fatalf("entry %s second leg must be credit %s, got %+v", e.ID, creditAccount, credit)
	}
	if debit.AmountCents != amount || credit.AmountCents != amount {
		t.Fatalf("entry %s legs must equal %d, got debit=%d credit=%d", e.ID, amount, debit.AmountCents, credit.AmountCents)
	}
}

// FIN-E2E-34: aprovar um saque reserva a equity do tenant no journal
// (débito em equity ↔ crédito em withdrawal_payable) e grava a chave
// source_event_id determinística para idempotência.
func TestWithdrawalService_Approve_ReservesEquityInJournal(t *testing.T) {
	withdrawalRepo := newFakeWithdrawalRepo()
	journalRepo := newFakeJournalRepo()
	journalSvc := NewJournalService(journalRepo)
	seedEquity(t, journalSvc, "tenant-1", 10000)

	svc := NewWithdrawalService(withdrawalRepo).WithJournalService(journalSvc)
	ctx := context.Background()

	w, err := svc.Request(ctx, "tenant-1", 1000, "saque teste", "requester")
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	approved, err := svc.Approve(ctx, "tenant-1", w.ID, "approver")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if approved.Status != withdrawal.StatusApproved {
		t.Fatalf("expected APPROVED, got %s", approved.Status)
	}

	reserve := journalEntryBySource(t, journalRepo, events.DeriveNotificationID("withdrawal.reserve", "tenant-1", w.ID))
	assertTwoLegs(t, reserve, journal.AccountEquity, journal.AccountWithdrawalPayable, 1000)
}

// FIN-E2E-34: sem saldo de equity disponível, a aprovação é rejeitada e o
// saque permanece PENDING.
func TestWithdrawalService_Approve_InsufficientBalance(t *testing.T) {
	withdrawalRepo := newFakeWithdrawalRepo()
	journalSvc := NewJournalService(newFakeJournalRepo())

	svc := NewWithdrawalService(withdrawalRepo).WithJournalService(journalSvc)
	ctx := context.Background()

	w, err := svc.Request(ctx, "tenant-1", 1000, "saque", "requester")
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	if _, err := svc.Approve(ctx, "tenant-1", w.ID, "approver"); !errors.Is(err, withdrawal.ErrInsufficientBalance) {
		t.Fatalf("expected ErrInsufficientBalance, got %v", err)
	}
	stored := withdrawalRepo.byID[w.ID]
	if stored.Status != withdrawal.StatusPending {
		t.Fatalf("expected PENDING, got %s", stored.Status)
	}
}

// FIN-E2E-34: liquidar um saque aprovado baixa o passivo contra caixa
// (débito em withdrawal_payable ↔ crédito em cash).
func TestWithdrawalService_Settle_PostsCashEntry(t *testing.T) {
	withdrawalRepo := newFakeWithdrawalRepo()
	journalRepo := newFakeJournalRepo()
	journalSvc := NewJournalService(journalRepo)
	seedEquity(t, journalSvc, "tenant-1", 10000)

	svc := NewWithdrawalService(withdrawalRepo).WithJournalService(journalSvc)
	ctx := context.Background()

	w, err := svc.Request(ctx, "tenant-1", 1000, "saque", "requester")
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if _, err := svc.Approve(ctx, "tenant-1", w.ID, "approver"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	settled, err := svc.Settle(ctx, "tenant-1", w.ID, "operator", "PIX-REF-123", "https://proof.example/x.png")
	if err != nil {
		t.Fatalf("Settle: %v", err)
	}
	if settled.Status != withdrawal.StatusComplete {
		t.Fatalf("expected COMPLETE, got %s", settled.Status)
	}
	if settled.SettlementRef != "PIX-REF-123" {
		t.Fatalf("expected SettlementRef PIX-REF-123, got %q", settled.SettlementRef)
	}

	settle := journalEntryBySource(t, journalRepo, events.DeriveNotificationID("withdrawal.settle", "tenant-1", w.ID))
	assertTwoLegs(t, settle, journal.AccountWithdrawalPayable, journal.AccountCash, 1000)
}

// FIN-E2E-34: cancelar um saque aprovado libera a reserva de volta para a
// equity (débito em withdrawal_payable ↔ crédito em equity).
func TestWithdrawalService_CancelApproved_ReleasesReserve(t *testing.T) {
	withdrawalRepo := newFakeWithdrawalRepo()
	journalRepo := newFakeJournalRepo()
	journalSvc := NewJournalService(journalRepo)
	seedEquity(t, journalSvc, "tenant-1", 10000)

	svc := NewWithdrawalService(withdrawalRepo).WithJournalService(journalSvc)
	ctx := context.Background()

	w, err := svc.Request(ctx, "tenant-1", 1000, "saque", "requester")
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if _, err := svc.Approve(ctx, "tenant-1", w.ID, "approver"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	cancelled, err := svc.Cancel(ctx, "tenant-1", w.ID, "operator")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Status != withdrawal.StatusCancelled {
		t.Fatalf("expected CANCELLED, got %s", cancelled.Status)
	}

	release := journalEntryBySource(t, journalRepo, events.DeriveNotificationID("withdrawal.release", "tenant-1", w.ID))
	assertTwoLegs(t, release, journal.AccountWithdrawalPayable, journal.AccountEquity, 1000)
}

// FIN-E2E-34: aprovar duas vezes não duplica a reserva — a segunda chamada
// devolve ErrNotPending e o journal mantém uma única entrada de reserva.
func TestWithdrawalService_Approve_DoesNotDoubleReserve(t *testing.T) {
	withdrawalRepo := newFakeWithdrawalRepo()
	journalRepo := newFakeJournalRepo()
	journalSvc := NewJournalService(journalRepo)
	seedEquity(t, journalSvc, "tenant-1", 10000)

	svc := NewWithdrawalService(withdrawalRepo).WithJournalService(journalSvc)
	ctx := context.Background()

	w, err := svc.Request(ctx, "tenant-1", 1000, "saque", "requester")
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if _, err := svc.Approve(ctx, "tenant-1", w.ID, "approver"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if _, err := svc.Approve(ctx, "tenant-1", w.ID, "approver"); !errors.Is(err, withdrawal.ErrNotPending) {
		t.Fatalf("expected ErrNotPending on second approve, got %v", err)
	}

	reserveID := events.DeriveNotificationID("withdrawal.reserve", "tenant-1", w.ID)
	if _, ok := journalRepo.bySource[reserveID]; !ok {
		t.Fatalf("expected a reserve entry with source_event_id %q", reserveID)
	}
	count := 0
	for _, e := range journalRepo.bySource {
		if e.SourceEventID == reserveID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one reserve entry, got %d", count)
	}
}
