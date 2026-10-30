package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
)

func TestOwnerTransferSettlementPostgresAtomicIdempotentAndBalanced(t *testing.T) {
	db := openFinancialConsistencyPostgres(t)
	tenantID := uuid.NewString()
	ctx, cancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 30*time.Second)
	defer cancel()

	canonicalTransferID := uuid.NewString()
	conflictTransferID := uuid.NewString()
	concurrentTransferID := uuid.NewString()
	receiptSourceID := uuid.NewString()
	condoSourceID := uuid.NewString()
	iptuSourceID := uuid.NewString()
	canonicalSettlementSourceID := ownerTransferSettlementSourceEventID(tenantID, canonicalTransferID)
	conflictSettlementSourceID := ownerTransferSettlementSourceEventID(tenantID, conflictTransferID)
	concurrentSettlementSourceID := ownerTransferSettlementSourceEventID(tenantID, concurrentTransferID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 10*time.Second)
		defer cleanupCancel()
		if _, err := deleteJournalFixturesInTest(cleanupCtx, db, `SELECT id::text FROM financial.journal_entries
			WHERE tenant_id = $1
			  AND (metadata->>'source_event_id') IN ($2, $3, $4, $5, $6, $7)`,
			tenantID, receiptSourceID, condoSourceID, iptuSourceID,
			canonicalSettlementSourceID, conflictSettlementSourceID, concurrentSettlementSourceID); err != nil {
			t.Errorf("cleanup transfer settlement journal: %v", err)
		}
		if _, err := db.Exec(cleanupCtx, `DELETE FROM financial.transfers
			WHERE tenant_id = $1 AND id IN ($2::uuid, $3::uuid, $4::uuid)`,
			tenantID, canonicalTransferID, conflictTransferID, concurrentTransferID); err != nil {
			t.Errorf("cleanup transfer settlement transfers: %v", err)
		}
		var residual int
		if err := db.QueryRow(cleanupCtx, `SELECT
			(SELECT COUNT(*) FROM financial.journal_entries
			  WHERE tenant_id = $1
			    AND (metadata->>'source_event_id') IN ($2, $3, $4, $5, $6, $7)) +
			(SELECT COUNT(*) FROM financial.transfers
			  WHERE tenant_id = $1 AND id IN ($8::uuid, $9::uuid, $10::uuid))`,
			tenantID, receiptSourceID, condoSourceID, iptuSourceID,
			canonicalSettlementSourceID, conflictSettlementSourceID, concurrentSettlementSourceID,
			canonicalTransferID, conflictTransferID, concurrentTransferID).Scan(&residual); err != nil {
			t.Errorf("verify transfer settlement cleanup: %v", err)
		} else if residual != 0 {
			t.Errorf("transfer settlement cleanup left %d rows", residual)
		}
	})

	transferRepo := repositories.NewTransferPostgresRepository(db)
	journalRepo := repositories.NewJournalPostgresRepository(db)
	journalService := NewJournalService(journalRepo).WithDB(db)
	bus := &captureBus{}
	service := NewTransferService(transferRepo).
		WithDB(db).
		WithJournalService(journalService).
		WithEventBus(bus)

	canonical := newApprovedAllocatedTransfer(t, canonicalTransferID, tenantID, "2026-08")
	if err := transferRepo.Create(ctx, canonical); err != nil {
		t.Fatalf("create canonical owner transfer: %v", err)
	}
	seedCaseGuideJournal(t, ctx, journalService, canonical, receiptSourceID, condoSourceID, iptuSourceID)

	if err := service.Process(ctx, tenantID, canonical.ID, "payer-1", "Carlos"); err != nil {
		t.Fatalf("process canonical owner transfer: %v", err)
	}
	processed, err := transferRepo.GetByID(ctx, tenantID, canonical.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed == nil || processed.Status != transfer.StatusProcessed ||
		processed.SettlementJournalSourceID != canonicalSettlementSourceID {
		t.Fatalf("processed transfer lost settlement identity: %+v", processed)
	}
	settlement, err := journalService.GetBySourceEventID(ctx, tenantID, canonicalSettlementSourceID)
	if err != nil {
		t.Fatal(err)
	}
	assertOwnerTransferSettlementEntry(t, settlement, processed, 460000)
	ownerPayable, err := journalService.AccountBalance(ctx, tenantID, journal.AccountOwnerPayable)
	if err != nil {
		t.Fatal(err)
	}
	cash, err := journalService.AccountBalance(ctx, tenantID, journal.AccountCash)
	if err != nil {
		t.Fatal(err)
	}
	revenue, err := journalService.AccountBalance(ctx, tenantID, journal.AccountRevenue)
	if err != nil {
		t.Fatal(err)
	}
	if ownerPayable != 0 || cash != 40000 || revenue != -40000 {
		t.Fatalf("case-guide balances owner_payable/cash/revenue = %d/%d/%d, want 0/40000/-40000", ownerPayable, cash, revenue)
	}

	// Exact retry validates the canonical posting without adding history,
	// journal rows, audit observers or another transfer.processed event.
	if err := service.Process(ctx, tenantID, canonical.ID, "payer-1", "Carlos"); err != nil {
		t.Fatalf("retry canonical owner transfer: %v", err)
	}
	assertSettlementRowCount(t, ctx, db, tenantID, canonicalSettlementSourceID, 1)
	if got := len(bus.get()); got != 1 {
		t.Fatalf("processed event count after retry = %d, want 1", got)
	}
	processed, err = transferRepo.GetByID(ctx, tenantID, canonical.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := countTransferHistoryAction(processed, "processed"); got != 1 {
		t.Fatalf("processed history count after retry = %d, want 1", got)
	}

	// A divergent pre-existing canonical source makes journal validation fail
	// after the status CAS. The enclosing transaction must restore APPROVED and
	// must not publish an event for the rolled-back attempt.
	conflicting := newApprovedAllocatedTransfer(t, conflictTransferID, tenantID, "2026-09")
	if err := transferRepo.Create(ctx, conflicting); err != nil {
		t.Fatalf("create conflicting owner transfer: %v", err)
	}
	divergent, err := journal.New(tenantID, conflicting.ContractID, "divergent fixture", time.Now().UTC(), []journal.Leg{
		{Account: journal.AccountOwnerPayable, Direction: journal.Debit, AmountCents: 1, TransferID: conflicting.ID},
		{Account: journal.AccountCash, Direction: journal.Credit, AmountCents: 1, TransferID: conflicting.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	divergent.OperationID = "wrong-operation"
	if _, err := journalService.CreateCanonicalWithSourceEventID(ctx, conflictSettlementSourceID, divergent); err != nil {
		t.Fatalf("seed divergent settlement source: %v", err)
	}
	eventsBeforeConflict := len(bus.get())
	err = service.Process(ctx, tenantID, conflicting.ID, "payer-2", "Beatriz")
	if !errors.Is(err, journal.ErrSourceEventConflict) {
		t.Fatalf("divergent settlement error = %v, want ErrSourceEventConflict", err)
	}
	rolledBack, err := transferRepo.GetByID(ctx, tenantID, conflicting.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Status != transfer.StatusApproved || rolledBack.SettlementJournalSourceID != "" ||
		countTransferHistoryAction(rolledBack, "processed") != 0 {
		t.Fatalf("journal conflict did not roll back transfer: %+v", rolledBack)
	}
	if got := len(bus.get()); got != eventsBeforeConflict {
		t.Fatalf("rolled-back settlement published an event: before=%d after=%d", eventsBeforeConflict, got)
	}

	// The production RabbitMQ bus writes its outbox row inside this same
	// transaction. If that write fails, neither PROCESSED nor the canonical
	// journal may escape; a later retry must complete exactly once.
	rows, err := deleteJournalFixturesInTest(ctx, db, `SELECT id::text FROM financial.journal_entries
		WHERE tenant_id = $1 AND (metadata->>'source_event_id') = $2`,
		tenantID, conflictSettlementSourceID)
	if err != nil {
		t.Fatalf("delete divergent settlement fixture: %v", err)
	}
	if rows != 1 {
		t.Fatalf("deleted divergent settlement rows = %d, want 1", rows)
	}
	outboxErr := errors.New("forced transfer outbox failure")
	failingOutbox := &failingTransferOutboxBus{err: outboxErr}
	err = NewTransferService(transferRepo).
		WithDB(db).
		WithJournalService(journalService).
		WithEventBus(failingOutbox).
		Process(ctx, tenantID, conflicting.ID, "payer-2", "Beatriz")
	if !errors.Is(err, outboxErr) {
		t.Fatalf("outbox failure error = %v", err)
	}
	rolledBack, err = transferRepo.GetByID(ctx, tenantID, conflicting.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Status != transfer.StatusApproved || rolledBack.SettlementJournalSourceID != "" ||
		countTransferHistoryAction(rolledBack, "processed") != 0 {
		t.Fatalf("outbox failure did not roll back transfer: %+v", rolledBack)
	}
	if got := len(failingOutbox.get()); got != 0 {
		t.Fatalf("rolled-back outbox attempt published %d events", got)
	}
	assertSettlementRowCount(t, ctx, db, tenantID, conflictSettlementSourceID, 0)
	if err := service.Process(ctx, tenantID, conflicting.ID, "payer-2", "Beatriz"); err != nil {
		t.Fatalf("retry after rolled-back outbox failure: %v", err)
	}
	assertSettlementRowCount(t, ctx, db, tenantID, conflictSettlementSourceID, 1)
	recovered, err := transferRepo.GetByID(ctx, tenantID, conflicting.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != transfer.StatusProcessed || countTransferHistoryAction(recovered, "processed") != 1 {
		t.Fatalf("retry after outbox rollback did not complete exactly once: %+v", recovered)
	}

	// Two simultaneous commands collapse onto one status transition and one
	// canonical row. The losing CAS re-reads and validates the winner.
	concurrent := newApprovedAllocatedTransfer(t, concurrentTransferID, tenantID, "2026-10")
	if err := transferRepo.Create(ctx, concurrent); err != nil {
		t.Fatalf("create concurrent owner transfer: %v", err)
	}
	eventsBeforeConcurrent := len(bus.get())
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			results <- service.Process(ctx, tenantID, concurrent.ID, "payer-3", "Daniel")
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatalf("concurrent process %d: %v", i+1, err)
		}
	}
	assertSettlementRowCount(t, ctx, db, tenantID, concurrentSettlementSourceID, 1)
	concurrentProcessed, err := transferRepo.GetByID(ctx, tenantID, concurrent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if concurrentProcessed.Status != transfer.StatusProcessed || countTransferHistoryAction(concurrentProcessed, "processed") != 1 {
		t.Fatalf("concurrent settlement duplicated transition: %+v", concurrentProcessed)
	}
	if got := len(bus.get()) - eventsBeforeConcurrent; got != 1 {
		t.Fatalf("concurrent settlement event count = %d, want 1", got)
	}
}

type failingTransferOutboxBus struct {
	captureBus
	err error
}

func (b *failingTransferOutboxBus) EnqueueOutboxInTx(context.Context, *sqlx.Tx, events.Event) (string, error) {
	return "", b.err
}

func newApprovedAllocatedTransfer(t *testing.T, id, tenantID, referenceMonth string) *transfer.OwnerTransfer {
	t.Helper()
	item := transfer.New(uuid.NewString(), uuid.NewString(), uuid.NewString(), tenantID, referenceMonth, 5000, 8, 0, 0, nil, "")
	item.ID = id
	balance, err := transfer.NewOwnerBalance(560000, 100000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := item.SetAllocatedOwnerBalance(balance, 40000); err != nil {
		t.Fatal(err)
	}
	paidAt := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	for _, movement := range []transfer.OwnerBalanceMovement{
		{ID: uuid.NewString(), ManagedBillID: uuid.NewString(), Type: transfer.OwnerBalanceMovementConsumeReserved, AmountCents: 70000, Actor: "payer", At: paidAt},
		{ID: uuid.NewString(), ManagedBillID: uuid.NewString(), Type: transfer.OwnerBalanceMovementConsumeReserved, AmountCents: 30000, Actor: "payer", At: paidAt.Add(time.Minute)},
	} {
		if changed, err := item.ApplyOwnerBalanceMovement(movement); err != nil || !changed {
			t.Fatalf("apply owner balance movement: changed=%v err=%v", changed, err)
		}
	}
	if item.OwnerBalance.TotalCents != 460000 || item.OwnerBalance.ReservedCents != 0 || item.OwnerBalance.AvailableCents() != 460000 {
		t.Fatalf("case-guide transfer balance = %+v", item.OwnerBalance)
	}
	if err := item.Approve("approver", "Ana"); err != nil {
		t.Fatal(err)
	}
	return item
}

func seedCaseGuideJournal(t *testing.T, ctx context.Context, service *JournalService, item *transfer.OwnerTransfer, receiptSourceID, condoSourceID, iptuSourceID string) {
	t.Helper()
	receipt, err := journal.New(item.TenantID, item.ContractID, "Recebimento caso-guia", time.Now().UTC(), []journal.Leg{
		{Account: journal.AccountCash, Direction: journal.Debit, AmountCents: 600000, TransferID: item.ID},
		{Account: journal.AccountRevenue, Direction: journal.Credit, AmountCents: 40000, TransferID: item.ID},
		{Account: journal.AccountOwnerPayable, Direction: journal.Credit, AmountCents: 560000, TransferID: item.ID, BeneficiaryPartyID: item.OwnerID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateCanonicalWithSourceEventID(ctx, receiptSourceID, receipt); err != nil {
		t.Fatal(err)
	}
	for _, bill := range []struct {
		source string
		amount int64
	}{
		{source: condoSourceID, amount: 70000},
		{source: iptuSourceID, amount: 30000},
	} {
		entry, err := journal.New(item.TenantID, item.ContractID, "Pagamento de conta administrada", time.Now().UTC(), []journal.Leg{
			{Account: journal.AccountOwnerPayable, Direction: journal.Debit, AmountCents: bill.amount, TransferID: item.ID, BeneficiaryPartyID: item.OwnerID},
			{Account: journal.AccountCash, Direction: journal.Credit, AmountCents: bill.amount, TransferID: item.ID, BeneficiaryPartyID: item.OwnerID},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreateCanonicalWithSourceEventID(ctx, bill.source, entry); err != nil {
			t.Fatal(err)
		}
	}
}

func assertOwnerTransferSettlementEntry(t *testing.T, entry *journal.Entry, item *transfer.OwnerTransfer, amountCents int64) {
	t.Helper()
	if entry == nil || entry.OperationID != item.ID || !entry.Date.Equal(*item.ProcessedAt) || len(entry.Legs) != 2 {
		t.Fatalf("invalid owner transfer settlement entry: %+v", entry)
	}
	for i, leg := range entry.Legs {
		if leg.AmountCents != amountCents || leg.TransferID != item.ID || leg.BeneficiaryPartyID != item.OwnerID {
			t.Fatalf("settlement leg %d lost amount/trace: %+v", i, leg)
		}
	}
	if entry.Legs[0].Account != journal.AccountOwnerPayable || entry.Legs[0].Direction != journal.Debit ||
		entry.Legs[1].Account != journal.AccountCash || entry.Legs[1].Direction != journal.Credit {
		t.Fatalf("settlement posting = %+v", entry.Legs)
	}
}

func assertSettlementRowCount(t *testing.T, ctx context.Context, db *database.PostgresDB, tenantID, sourceEventID string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM financial.journal_entries
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'source_event_id') = $2`,
		tenantID, sourceEventID).Scan(&got); err != nil {
		t.Fatalf("count settlement rows: %v", err)
	}
	if got != want {
		t.Fatalf("settlement row count = %d, want %d", got, want)
	}
}

func countTransferHistoryAction(item *transfer.OwnerTransfer, action string) int {
	count := 0
	if item == nil {
		return count
	}
	for _, entry := range item.History {
		if entry.Action == action {
			count++
		}
	}
	return count
}
