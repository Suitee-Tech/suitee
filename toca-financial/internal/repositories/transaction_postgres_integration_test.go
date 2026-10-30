package repositories_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
	"github.com/oginoino/toca/toca-financial/internal/services"
)

// TestTransactionPostgresRepository_CrossTenantOperationsAreInvisible exercises
// the real SQL repository. It is opt-in because unit-test environments do not
// necessarily expose a migrated PostgreSQL instance. CI/local integration runs
// set TOCA_TEST_DATABASE_URI to an isolated database and execute this test.
func TestTransactionPostgresRepository_CrossTenantOperationsAreInvisible(t *testing.T) {
	databaseURI := os.Getenv("TOCA_TEST_DATABASE_URI")
	if databaseURI == "" {
		t.Skip("TOCA_TEST_DATABASE_URI is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{
		URI:         databaseURI,
		MaxPoolSize: 4,
		MinPoolSize: 0,
	})
	if err != nil {
		t.Fatalf("connect test postgres: %v", err)
	}
	defer db.Close()

	repo := repositories.NewTransactionPostgresRepository(db)
	svc := services.NewTransactionService(repo).WithDB(db)
	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	tenantACtx := pkgcontext.WithTenantID(ctx, tenantA)
	tenantBCtx := pkgcontext.WithTenantID(ctx, tenantB)

	paid := transaction.New("contract-tenant-a", transaction.TypeReceita, "tenant A paid", 100, time.Now(), tenantA)
	paid.Status = transaction.StatusPaid
	pending := transaction.New("contract-tenant-a", transaction.TypeDespesa, "tenant A pending", 25, time.Now(), tenantA)
	failedBatchID := uuid.NewString()
	for _, tx := range []*transaction.Transaction{paid, pending} {
		if err := repo.Create(tenantACtx, tx); err != nil {
			t.Fatalf("seed transaction %s: %v", tx.ID, err)
		}
	}
	defer func() {
		cleanupCtx := pkgcontext.WithTenantID(context.Background(), tenantA)
		_, _ = db.Exec(cleanupCtx,
			"DELETE FROM financial.transactions WHERE id IN ($1, $2, $3)", paid.ID, pending.ID, failedBatchID)
	}()

	batchLine1 := transaction.New("contract-tenant-a", transaction.TypeReceita, "batch line 1", 50, time.Now(), tenantA)
	batchLine1.ID = failedBatchID
	batchLine1.PostingID = failedBatchID
	batchLine1.OperationID = "atomic-batch"
	batchLine1.SourceEventID = uuid.NewString()
	batchLine2 := transaction.New("contract-tenant-a", transaction.TypeRepasse, "batch line 2", 10, time.Now(), tenantA)
	batchLine2.ID = failedBatchID // forces a PK error after line 1 was inserted
	batchLine2.PostingID = failedBatchID
	batchLine2.OperationID = "atomic-batch"
	batchLine2.SourceEventID = uuid.NewString()
	if err := repo.CreateBatch(tenantACtx, []*transaction.Transaction{batchLine1, batchLine2}); err == nil {
		t.Fatal("batch with a failing second line must return an error")
	}
	if _, err := repo.GetByID(tenantACtx, tenantA, failedBatchID); err == nil {
		t.Fatal("failed batch left its first line persisted")
	}

	if _, err := svc.GetByID(tenantBCtx, tenantB, paid.ID); err == nil {
		t.Fatal("cross-tenant get must not reveal the transaction")
	}

	foreignUpdate := *paid
	foreignUpdate.TenantID = tenantB
	foreignUpdate.Description = "cross-tenant overwrite"
	if err := svc.Update(tenantBCtx, &foreignUpdate); err == nil {
		t.Fatal("cross-tenant update must fail")
	}

	if _, err := svc.Reverse(tenantBCtx, paid.ID, tenantB, "cross tenant", "operator"); err == nil {
		t.Fatal("cross-tenant reversal must fail")
	}

	if err := svc.Delete(tenantBCtx, pending.ID, tenantB); err == nil {
		t.Fatal("cross-tenant delete must fail")
	}

	gotPaid, err := svc.GetByID(tenantACtx, tenantA, paid.ID)
	if err != nil {
		t.Fatalf("tenant A paid transaction disappeared: %v", err)
	}
	if gotPaid.Description != paid.Description || gotPaid.Status != transaction.StatusPaid {
		t.Fatalf("cross-tenant mutations changed paid row: %#v", gotPaid)
	}
	if _, err := svc.GetByID(tenantACtx, tenantA, pending.ID); err != nil {
		t.Fatalf("cross-tenant delete removed tenant A row: %v", err)
	}
}

// TestTransactionService_BatchCreatePostingCommitsAndRollsBackAtomically proves
// the real PostgreSQL boundary shared by manual ledger lines and Commission
// aggregates. It uses unique fixture IDs and removes only those exact IDs.
func TestTransactionService_BatchCreatePostingCommitsAndRollsBackAtomically(t *testing.T) {
	databaseURI := os.Getenv("TOCA_TEST_DATABASE_URI")
	if databaseURI == "" {
		t.Skip("TOCA_TEST_DATABASE_URI is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{
		URI:         databaseURI,
		MaxPoolSize: 4,
		MinPoolSize: 0,
	})
	if err != nil {
		t.Fatalf("connect test postgres: %v", err)
	}
	defer db.Close()

	tenantID := uuid.NewString()
	tenantCtx := pkgcontext.WithTenantID(ctx, tenantID)
	txRepo := repositories.NewTransactionPostgresRepository(db)
	commissionRepo := repositories.NewCommissionPostgresRepository(db)
	commissionSvc := services.NewCommissionService(commissionRepo)
	txSvc := services.NewTransactionService(txRepo).
		WithDB(db).
		WithManualCommissionService(commissionSvc)

	successKey := "pg-manual-posting-success:" + uuid.NewString()
	conflictKey := "pg-manual-posting-conflict:" + uuid.NewString()
	txIDFor := func(operationKey string, index int) string {
		operationID := events.DeriveNotificationID(operationKey, "batch", tenantID)
		sourceID := events.DeriveNotificationID(operationID, "batch_line", strconv.Itoa(index))
		return events.DeriveNotificationID(sourceID, "posting")
	}
	successTxIDs := []string{txIDFor(successKey, 0), txIDFor(successKey, 1)}
	conflictTxID := txIDFor(conflictKey, 0)
	commissionIDs := make([]string, 0, 2)
	defer func() {
		cleanupCtx := pkgcontext.WithTenantID(context.Background(), tenantID)
		for _, id := range append(successTxIDs, conflictTxID) {
			if _, err := db.Exec(cleanupCtx, "DELETE FROM financial.transactions WHERE tenant_id = $1 AND id = $2", tenantID, id); err != nil {
				t.Errorf("cleanup transaction %s: %v", id, err)
			}
		}
		for _, id := range commissionIDs {
			if _, err := db.Exec(cleanupCtx, "DELETE FROM financial.commissions WHERE tenant_id = $1 AND id = $2", tenantID, id); err != nil {
				t.Errorf("cleanup commission %s: %v", id, err)
			}
		}
		var residual int
		if err := db.QueryRow(cleanupCtx, `
			SELECT
				(SELECT COUNT(*) FROM financial.transactions WHERE tenant_id = $1) +
				(SELECT COUNT(*) FROM financial.commissions WHERE tenant_id = $1)
		`, tenantID).Scan(&residual); err != nil {
			t.Errorf("verify manual posting cleanup: %v", err)
		} else if residual != 0 {
			t.Errorf("manual posting cleanup left %d rows for tenant %s", residual, tenantID)
		}
	}()

	items := []services.BatchCreateInput{
		{ContractID: "contract-" + uuid.NewString(), Type: string(transaction.TypeReceita), Description: "Receita conjunta", Value: 100, DateStr: "2026-08-13"},
		{ContractID: "", Type: string(transaction.TypeComissao), Description: "Comissão conjunta", Value: 12, DateStr: "2026-08-13"},
	}
	items[1].ContractID = items[0].ContractID
	items[1].BrokerID = "broker-" + uuid.NewString()
	items[1].Role = "fechamento"
	commissions := []services.ManualCommissionInput{{
		DealID: "deal-" + uuid.NewString(), ContractID: items[0].ContractID, BrokerID: items[1].BrokerID, Role: items[1].Role, Percentage: 80, Amount: 12,
	}}
	retention := &services.ManualCommissionRetentionInput{Percentage: 20, Amount: 3}

	outputs, err := txSvc.BatchCreatePosting(tenantCtx, tenantID, successKey, items, commissions, retention)
	if err != nil {
		t.Fatalf("commit combined posting: %v", err)
	}
	if len(outputs) != 2 || outputs[0].Transaction.ID != successTxIDs[0] || outputs[1].Transaction.ID != successTxIDs[1] {
		t.Fatalf("unexpected committed transaction IDs: %#v", outputs)
	}
	successCommissionSource := events.DeriveNotificationID(successKey, "manual_commission", tenantID, "0")
	committedCommission, err := commissionRepo.GetBySourceEventID(tenantCtx, tenantID, successCommissionSource)
	if err != nil || committedCommission == nil {
		t.Fatalf("combined commission was not committed: commission=%#v err=%v", committedCommission, err)
	}
	commissionIDs = append(commissionIDs, committedCommission.ID)

	var committedTxCount, committedCommissionCount int
	if err := db.QueryRow(tenantCtx,
		"SELECT COUNT(*) FROM financial.transactions WHERE tenant_id = $1 AND id IN ($2, $3) AND deleted_at IS NULL",
		tenantID, successTxIDs[0], successTxIDs[1],
	).Scan(&committedTxCount); err != nil {
		t.Fatalf("count committed transactions: %v", err)
	}
	if err := db.QueryRow(tenantCtx,
		"SELECT COUNT(*) FROM financial.commissions WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL",
		tenantID, committedCommission.ID,
	).Scan(&committedCommissionCount); err != nil {
		t.Fatalf("count committed commission: %v", err)
	}
	if committedTxCount != 2 || committedCommissionCount != 1 {
		t.Fatalf("combined commit is incomplete: transactions=%d commissions=%d", committedTxCount, committedCommissionCount)
	}

	retry, err := txSvc.BatchCreatePosting(tenantCtx, tenantID, successKey, items, commissions, retention)
	if err != nil {
		t.Fatalf("retry combined posting: %v", err)
	}
	if len(retry) != 2 || retry[0].Transaction.ID != outputs[0].Transaction.ID || retry[1].Transaction.ID != outputs[1].Transaction.ID {
		t.Fatalf("retry did not return canonical transactions: %#v", retry)
	}

	conflictCommissionSource := events.DeriveNotificationID(conflictKey, "manual_commission", tenantID, "0")
	preexisting, err := commissionSvc.CreateWithRoleFromEvent(
		tenantCtx, conflictCommissionSource, "deal-preexisting", "broker-preexisting", "contract-preexisting",
		"captador", 11, 11, tenantID, "", "", "", "", "",
	)
	if err != nil {
		t.Fatalf("seed exact idempotency conflict: %v", err)
	}
	commissionIDs = append(commissionIDs, preexisting.ID)

	_, err = txSvc.BatchCreatePosting(tenantCtx, tenantID, conflictKey, []services.BatchCreateInput{{
		ContractID: "contract-conflict", Type: string(transaction.TypeComissao), Description: "Must roll back", Value: 12, DateStr: "2026-08-13", BrokerID: "broker-different", Role: "fechamento",
	}}, []services.ManualCommissionInput{{
		DealID: "deal-different", ContractID: "contract-conflict", BrokerID: "broker-different", Role: "fechamento", Percentage: 12, Amount: 12,
	}}, nil)
	if !errors.Is(err, transaction.ErrBatchIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	var rolledBackTxCount int
	if err := db.QueryRow(tenantCtx,
		"SELECT COUNT(*) FROM financial.transactions WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL",
		tenantID, conflictTxID,
	).Scan(&rolledBackTxCount); err != nil {
		t.Fatalf("count rolled-back transaction: %v", err)
	}
	if rolledBackTxCount != 0 {
		t.Fatalf("commission conflict left %d transaction rows committed", rolledBackTxCount)
	}
	canonicalPreexisting, err := commissionRepo.GetByID(tenantCtx, tenantID, preexisting.ID)
	if err != nil {
		t.Fatalf("reload preexisting commission: %v", err)
	}
	if canonicalPreexisting.DealID != "deal-preexisting" || canonicalPreexisting.Amount != 11 {
		t.Fatalf("rollback changed the preexisting commission: %#v", canonicalPreexisting)
	}
}
