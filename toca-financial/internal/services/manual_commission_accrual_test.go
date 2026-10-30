package services

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
)

type recordingManualJournalRepo struct {
	entries []*journal.Entry
}

func (r *recordingManualJournalRepo) Create(_ context.Context, e *journal.Entry) error {
	r.entries = append(r.entries, e)
	return nil
}

func (r *recordingManualJournalRepo) GetByID(_ context.Context, _, _ string) (*journal.Entry, error) {
	return nil, nil
}

func (r *recordingManualJournalRepo) GetBySourceEventID(_ context.Context, _, _ string) (*journal.Entry, error) {
	return nil, nil
}

func (r *recordingManualJournalRepo) UpdateStatus(_ context.Context, _ *journal.Entry, _ time.Time) (bool, error) {
	return true, nil
}

func (r *recordingManualJournalRepo) List(_ context.Context, _, _ string, _, _ int64) ([]*journal.Entry, int64, error) {
	return nil, 0, nil
}

func (r *recordingManualJournalRepo) AccountBalance(_ context.Context, _ string, _ journal.Account) (int64, error) {
	return 0, nil
}

func TestBatchCreatePostingAccruesReceiptBackedCommission(t *testing.T) {
	txRepo := newStoringTxRepo()
	commissionRepo := newStoringManualCommissionRepo()
	journalRepo := &recordingManualJournalRepo{}
	svc := NewTransactionService(txRepo).
		WithManualCommissionService(NewCommissionService(commissionRepo)).
		WithJournalService(NewJournalService(journalRepo))

	ctx := database.WithTx(context.Background(), &sqlx.Tx{})
	items := []BatchCreateInput{
		{ContractID: "contract-sale", DealID: "deal-sale", Type: string(transaction.TypeReceita), Description: "Recebimento - Venda", Value: 980_000, DateStr: "2026-08-29", Flow: "sale"},
		{ContractID: "contract-sale", Type: string(transaction.TypeComissao), Description: "Comissão - captador", Value: 11_760, DateStr: "2026-08-29", BrokerID: "broker-cap", Role: "captador"},
		{ContractID: "contract-sale", Type: string(transaction.TypeComissao), Description: "Comissão - fechamento", Value: 23_520, DateStr: "2026-08-29", BrokerID: "broker-close", Role: "fechamento"},
	}
	commissions := []ManualCommissionInput{
		{DealID: "deal-sale", ContractID: "contract-sale", BrokerID: "broker-cap", Role: "captador", Percentage: 20, Amount: 11_760},
		{DealID: "deal-sale", ContractID: "contract-sale", BrokerID: "broker-close", Role: "fechamento", Percentage: 40, Amount: 23_520},
	}
	retention := &ManualCommissionRetentionInput{Percentage: 40, Amount: 23_520}

	if _, err := svc.BatchCreatePosting(ctx, "tenant-sale", "accrual-sale", items, commissions, retention); err != nil {
		t.Fatalf("create manual sale posting: %v", err)
	}
	if len(journalRepo.entries) != 2 {
		t.Fatalf("accrual entries = %d, want 2", len(journalRepo.entries))
	}
	var expenseDebit, payableCredit int64
	for _, entry := range journalRepo.entries {
		if !entry.Balanced() {
			t.Fatalf("accrual entry must be balanced: %+v", entry.Legs)
		}
		for _, leg := range entry.Legs {
			switch {
			case leg.Account == journal.AccountCommissionExpense && leg.Direction == journal.Debit:
				expenseDebit += leg.AmountCents
			case leg.Account == journal.AccountCommissionPayable && leg.Direction == journal.Credit:
				payableCredit += leg.AmountCents
			}
		}
	}
	if expenseDebit != 3_528_000 || payableCredit != 3_528_000 {
		t.Fatalf("accrual totals: expense=%d payable=%d, want 3528000 each", expenseDebit, payableCredit)
	}
}

func TestBatchCreatePostingAccruesStandaloneCommission(t *testing.T) {
	txRepo := newStoringTxRepo()
	commissionRepo := newStoringManualCommissionRepo()
	journalRepo := &recordingManualJournalRepo{}
	svc := NewTransactionService(txRepo).
		WithManualCommissionService(NewCommissionService(commissionRepo)).
		WithJournalService(NewJournalService(journalRepo))

	ctx := database.WithTx(context.Background(), &sqlx.Tx{})
	items := []BatchCreateInput{
		{ContractID: "contract-sale", DealID: "deal-sale", Type: string(transaction.TypeComissao), Description: "Comissão Avulsa - Corretor", Value: 6_000, DateStr: "2026-08-29", BrokerID: "broker-cap", Role: "captador"},
	}
	commissions := []ManualCommissionInput{
		{DealID: "deal-sale", ContractID: "contract-sale", BrokerID: "broker-cap", Role: "captador", Percentage: 100, Amount: 6_000},
	}

	if _, err := svc.BatchCreatePosting(ctx, "tenant-sale", "accrual-standalone", items, commissions, nil); err != nil {
		t.Fatalf("create standalone commission: %v", err)
	}
	if len(journalRepo.entries) != 1 {
		t.Fatalf("standalone commission must accrue once, got %d entries", len(journalRepo.entries))
	}
	entry := journalRepo.entries[0]
	if !entry.Balanced() {
		t.Fatalf("standalone accrual must be balanced: %+v", entry.Legs)
	}
	assertJournalLeg(t, entry, journal.AccountCommissionExpense, journal.Debit, 600_000, "")
	assertJournalLeg(t, entry, journal.AccountCommissionPayable, journal.Credit, 600_000, "broker-cap")
}
