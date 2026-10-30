package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/config"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/invoice"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
)

func openFinancialConsistencyPostgres(t *testing.T) *database.PostgresDB {
	t.Helper()
	databaseURI := os.Getenv("TOCA_TEST_DATABASE_URI")
	if databaseURI == "" {
		t.Skip("TOCA_TEST_DATABASE_URI is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{
		URI:         databaseURI,
		MaxPoolSize: 12,
		MinPoolSize: 0,
	})
	if err != nil {
		t.Fatalf("connect test postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// deleteJournalFixturesInTest resolves exact journal UUIDs, binds them to the
// current transaction and removes only that whitelist. The txid binding makes
// a mistakenly session-scoped GUC stale as soon as the transaction commits.
func deleteJournalFixturesInTest(ctx context.Context, db *database.PostgresDB, targetQuery string, args ...any) (int64, error) {
	var rows int64
	err := db.RunInTx(ctx, func(tx *sqlx.Tx) error {
		txCtx := database.WithTx(ctx, tx)
		targetRows, err := db.Query(txCtx, targetQuery, args...)
		if err != nil {
			return err
		}
		ids := make([]string, 0)
		for targetRows.Next() {
			var id string
			if err := targetRows.Scan(&id); err != nil {
				targetRows.Close()
				return err
			}
			ids = append(ids, id)
		}
		if err := targetRows.Err(); err != nil {
			targetRows.Close()
			return err
		}
		if err := targetRows.Close(); err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		whitelist := strings.Join(ids, ",")
		if _, err := db.Exec(txCtx, `SELECT
			set_config('toca.journal_fixture_cleanup_txid', pg_current_xact_id()::text, true),
			set_config('toca.journal_fixture_cleanup_ids', $1, true)`, whitelist); err != nil {
			return err
		}
		result, err := db.Exec(txCtx, `DELETE FROM financial.journal_entries
			WHERE id::text = ANY(string_to_array($1, ','))`, whitelist)
		if err != nil {
			return err
		}
		rows, err = result.RowsAffected()
		return err
	})
	return rows, err
}

func TestRentalChargeConfigPostgresConcurrentCreateHasOneCanonicalRow(t *testing.T) {
	db := openFinancialConsistencyPostgres(t)
	tenantID := uuid.NewString()
	contractID := uuid.NewString()
	ctx, cancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 20*time.Second)
	defer cancel()

	// The cleanup predicate contains both unique fixture identities. It cannot
	// match another test or tenant, and the residual count proves it completed.
	expectRow := false
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 10*time.Second)
		defer cleanupCancel()
		result, err := db.Exec(cleanupCtx, `DELETE FROM financial.rental_charge_configs
			WHERE tenant_id = $1
			  AND COALESCE(metadata->>'contract_id', metadata->>'ContractID') = $2`, tenantID, contractID)
		if err != nil {
			t.Errorf("cleanup rental charge config: %v", err)
			return
		}
		if expectRow {
			rows, _ := result.RowsAffected()
			if rows != 1 {
				t.Errorf("cleanup rental charge config removed %d rows, want 1", rows)
			}
		}
		var residual int
		if err := db.QueryRow(cleanupCtx, `SELECT COUNT(*) FROM financial.rental_charge_configs
			WHERE tenant_id = $1
			  AND COALESCE(metadata->>'contract_id', metadata->>'ContractID') = $2`, tenantID, contractID).Scan(&residual); err != nil {
			t.Errorf("verify rental charge config cleanup: %v", err)
		} else if residual != 0 {
			t.Errorf("rental charge config cleanup left %d rows", residual)
		}
	})

	repo := repositories.NewRentalChargeConfigPostgresRepository(db)
	svc := NewRentalChargeConfigService(repo, nil, nil)
	startDate := time.Now().UTC().AddDate(0, 0, 1)
	endDate := startDate.AddDate(1, 0, 0)

	type result struct {
		configID string
		err      error
	}
	const workers = 8
	start := make(chan struct{})
	results := make(chan result, workers)
	for i := 0; i < workers; i++ {
		go func() {
			<-start
			cfg, err := svc.CreateConfigOnly(
				ctx,
				contractID, tenantID, uuid.NewString(), uuid.NewString(), uuid.NewString(),
				startDate, endDate, startDate,
				rental_charge_config.SameMonthProRata, startDate.Day(), 150_000,
			)
			if err != nil {
				results <- result{err: err}
				return
			}
			results <- result{configID: cfg.ID}
		}()
	}
	close(start)

	canonicalIDs := make(map[string]struct{})
	for i := 0; i < workers; i++ {
		got := <-results
		if got.err != nil {
			t.Fatalf("concurrent CreateConfigOnly call %d: %v", i+1, got.err)
		}
		canonicalIDs[got.configID] = struct{}{}
	}
	if len(canonicalIDs) != 1 {
		t.Fatalf("concurrent callers observed %d config IDs, want one: %v", len(canonicalIDs), canonicalIDs)
	}

	var rowCount int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM financial.rental_charge_configs
		WHERE tenant_id = $1
		  AND COALESCE(metadata->>'contract_id', metadata->>'ContractID') = $2
		  AND deleted_at IS NULL`, tenantID, contractID).Scan(&rowCount); err != nil {
		t.Fatalf("count canonical config rows: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("canonical config rows = %d, want 1", rowCount)
	}
	expectRow = true

	missingByID, err := repo.GetByID(ctx, tenantID, uuid.NewString())
	if err != nil || missingByID != nil {
		t.Fatalf("GetByID missing row = (%+v, %v), want (nil, nil)", missingByID, err)
	}
	missingByContract, err := repo.GetByContractID(ctx, tenantID, uuid.NewString())
	if err != nil || missingByContract != nil {
		t.Fatalf("GetByContractID missing row = (%+v, %v), want (nil, nil)", missingByContract, err)
	}
}

func TestRentalIntermediationFundingPostgresIsAtomicRollbackSafeAndIdempotent(t *testing.T) {
	db := openFinancialConsistencyPostgres(t)
	tenantID := uuid.NewString()
	contractID := uuid.NewString()
	ctx, cancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 30*time.Second)
	defer cancel()

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 10*time.Second)
		defer cleanupCancel()
		result, err := db.Exec(cleanupCtx, `DELETE FROM financial.rental_charge_configs
			WHERE tenant_id = $1
			  AND COALESCE(metadata->>'contract_id', metadata->>'ContractID') = $2`, tenantID, contractID)
		if err != nil {
			t.Errorf("cleanup rental intermediation config: %v", err)
			return
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			t.Errorf("cleanup rental intermediation config removed %d rows, want 1", rows)
		}
	})

	startDate := time.Now().UTC().AddDate(0, 0, 1)
	cfg, err := rental_charge_config.New(
		tenantID, uuid.NewString(), contractID, uuid.NewString(), uuid.NewString(),
		startDate, startDate.AddDate(1, 0, 0), startDate,
		rental_charge_config.SameMonthProRata, 10, 500_000,
	)
	if err != nil {
		t.Fatal(err)
	}
	brokerID := uuid.NewString()
	commissionID := uuid.NewString()
	payoutID := uuid.NewString()
	if _, err := cfg.FreezeIntermediationPlan(rental_charge_config.IntermediationPlan{
		DealID: uuid.NewString(), PlanHash: "postgres-rental-plan", TotalAmountCents: 500_000,
		Shares: []rental_charge_config.IntermediationShare{{
			BrokerID: brokerID, Role: "fechamento", TargetAmountCents: 500_000,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.LinkIntermediationCommission(brokerID, "fechamento", commissionID); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.LinkIntermediationPayout(brokerID, "fechamento", payoutID); err != nil {
		t.Fatal(err)
	}

	repo := repositories.NewRentalChargeConfigPostgresRepository(db)
	if err := repo.Create(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	atomicRepo, ok := repo.(rentalIntermediationAtomicRepository)
	if !ok {
		t.Fatal("postgres rental config repository does not implement atomic funding")
	}
	fundedAt := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)

	rollbackSentinel := errors.New("rollback rental funding fixture")
	err = db.RunInTx(ctx, func(tx *sqlx.Tx) error {
		txCtx := database.WithTx(ctx, tx)
		_, entry, changed, err := atomicRepo.AllocateIntermediationFunding(
			txCtx, tenantID, contractID, "payment-rollback", "charge-rollback", "2026-08", 270_000, fundedAt,
		)
		if err != nil {
			return err
		}
		if !changed || entry.AppliedCents != 270_000 {
			return fmt.Errorf("rollback allocation changed=%v entry=%+v", changed, entry)
		}
		return rollbackSentinel
	})
	if !errors.Is(err, rollbackSentinel) {
		t.Fatalf("rollback transaction = %v", err)
	}
	afterRollback, err := repo.GetByContractID(ctx, tenantID, contractID)
	if err != nil {
		t.Fatal(err)
	}
	if afterRollback.IntermediationOutstandingCents() != 500_000 || len(afterRollback.IntermediationPlan.FundingEntries) != 0 {
		t.Fatalf("rolled-back funding leaked into persisted plan: %+v", afterRollback.IntermediationPlan)
	}

	_, firstEntry, changed, err := atomicRepo.AllocateIntermediationFunding(
		ctx, tenantID, contractID, "payment-aug", "charge-aug", "2026-08", 270_000, fundedAt,
	)
	if err != nil || !changed || firstEntry.AppliedCents != 270_000 {
		t.Fatalf("first committed allocation = (%v, %v), entry=%+v", changed, err, firstEntry)
	}
	_, retryEntry, changed, err := atomicRepo.AllocateIntermediationFunding(
		ctx, tenantID, contractID, "payment-aug", "charge-aug", "2026-08", 270_000, fundedAt,
	)
	if err != nil || changed || retryEntry.AppliedCents != 270_000 {
		t.Fatalf("idempotent allocation retry = (%v, %v), entry=%+v", changed, err, retryEntry)
	}

	type allocationResult struct {
		entry   rental_charge_config.IntermediationFundingEntry
		changed bool
		err     error
	}
	start := make(chan struct{})
	results := make(chan allocationResult, 2)
	for i := 0; i < 2; i++ {
		paymentID := fmt.Sprintf("payment-concurrent-%d", i)
		chargeID := fmt.Sprintf("charge-concurrent-%d", i)
		go func() {
			<-start
			_, entry, changed, err := atomicRepo.AllocateIntermediationFunding(
				ctx, tenantID, contractID, paymentID, chargeID, "2026-09", 300_000, fundedAt.AddDate(0, 1, 0),
			)
			results <- allocationResult{entry: entry, changed: changed, err: err}
		}()
	}
	close(start)
	var concurrentApplied int64
	var concurrentChanges int
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent funding %d: %v", i+1, result.err)
		}
		concurrentApplied += result.entry.AppliedCents
		if result.changed {
			concurrentChanges++
		}
	}
	if concurrentApplied != 230_000 || concurrentChanges != 1 {
		t.Fatalf("concurrent remainder applied=%d changes=%d, want 230000/1", concurrentApplied, concurrentChanges)
	}

	persisted, err := repo.GetByContractID(ctx, tenantID, contractID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.IntermediationOutstandingCents() != 0 || len(persisted.IntermediationPlan.FundingEntries) != 2 {
		t.Fatalf("persisted funding plan = %+v", persisted.IntermediationPlan)
	}
	if persisted.IntermediationPlan.Shares[0].FundedAmountCents != 500_000 {
		t.Fatalf("broker funding = %d, want 500000", persisted.IntermediationPlan.Shares[0].FundedAmountCents)
	}
}

func TestInvoicePostgresConcurrentCreateHasOneCanonicalCommand(t *testing.T) {
	db := openFinancialConsistencyPostgres(t)
	tenantID := uuid.NewString()
	chargeID := uuid.NewString()
	contractID := uuid.NewString()
	payerID := uuid.NewString()
	sourceEventID := uuid.NewString()
	ctx, cancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 20*time.Second)
	defer cancel()

	expectInvoice := false
	expectCharge := false
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 10*time.Second)
		defer cleanupCancel()
		invoiceResult, invoiceErr := db.Exec(cleanupCtx,
			"DELETE FROM financial.invoices WHERE tenant_id = $1 AND (metadata->>'source_event_id') = $2",
			tenantID, sourceEventID)
		if invoiceErr != nil {
			t.Errorf("cleanup concurrent invoice: %v", invoiceErr)
		} else if expectInvoice {
			if rows, _ := invoiceResult.RowsAffected(); rows != 1 {
				t.Errorf("cleanup concurrent invoice removed %d rows, want 1", rows)
			}
		}
		chargeResult, chargeErr := db.Exec(cleanupCtx,
			"DELETE FROM financial.charges WHERE tenant_id = $1 AND id = $2", tenantID, chargeID)
		if chargeErr != nil {
			t.Errorf("cleanup concurrent invoice charge: %v", chargeErr)
		} else if expectCharge {
			if rows, _ := chargeResult.RowsAffected(); rows != 1 {
				t.Errorf("cleanup concurrent invoice charge removed %d rows, want 1", rows)
			}
		}
		var residual int
		if err := db.QueryRow(cleanupCtx, `SELECT
			(SELECT COUNT(*) FROM financial.invoices WHERE tenant_id = $1 AND (metadata->>'source_event_id') = $2) +
			(SELECT COUNT(*) FROM financial.charges WHERE tenant_id = $1 AND id = $3)`,
			tenantID, sourceEventID, chargeID).Scan(&residual); err != nil {
			t.Errorf("verify concurrent invoice cleanup: %v", err)
		} else if residual != 0 {
			t.Errorf("concurrent invoice cleanup left %d rows", residual)
		}
	})

	chargeRepo := repositories.NewChargePostgresRepository(db)
	receivable, err := charge.New(contractID, payerID, uuid.NewString(), uuid.NewString(), tenantID, charge.TypeRent, "Concurrent invoice", "2026-08", 100, time.Now().UTC().AddDate(0, 0, 5))
	if err != nil {
		t.Fatalf("create concurrent invoice charge: %v", err)
	}
	receivable.ID = chargeID
	if err := chargeRepo.Create(ctx, receivable); err != nil {
		t.Fatalf("persist concurrent invoice charge: %v", err)
	}
	expectCharge = true

	invoiceRepo := repositories.NewInvoicePostgresRepository(db)
	svc := NewInvoiceService(invoiceRepo, nil).
		WithPaymentApplication(NewChargeService(chargeRepo, nil)).
		WithDB(db)
	dueDate := receivable.DueDate
	type createResult struct {
		invoiceID string
		err       error
	}
	const workers = 8
	start := make(chan struct{})
	results := make(chan createResult, workers)
	for i := 0; i < workers; i++ {
		go func() {
			<-start
			inv, err := svc.CreateWithSourceAndCharge(ctx, contractID, payerID, 40, dueDate, invoice.TypeRent, tenantID, sourceEventID, chargeID)
			if err != nil {
				results <- createResult{err: err}
				return
			}
			results <- createResult{invoiceID: inv.ID}
		}()
	}
	close(start)

	canonicalIDs := make(map[string]struct{})
	for i := 0; i < workers; i++ {
		got := <-results
		if got.err != nil {
			t.Fatalf("concurrent invoice create %d: %v", i+1, got.err)
		}
		canonicalIDs[got.invoiceID] = struct{}{}
	}
	if len(canonicalIDs) != 1 {
		t.Fatalf("concurrent invoice callers observed %d IDs, want one: %v", len(canonicalIDs), canonicalIDs)
	}
	var rowCount int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM financial.invoices
		WHERE tenant_id = $1 AND (metadata->>'source_event_id') = $2 AND deleted_at IS NULL`,
		tenantID, sourceEventID).Scan(&rowCount); err != nil {
		t.Fatalf("count canonical invoices: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("canonical invoice rows = %d, want 1", rowCount)
	}
	expectInvoice = true
	if _, err := invoiceRepo.GetByID(ctx, tenantID, uuid.NewString()); !errors.Is(err, invoice.ErrInvoiceNotFound) {
		t.Fatalf("missing PostgreSQL invoice error = %v, want ErrInvoiceNotFound", err)
	}

	_, err = svc.CreateWithSourceAndCharge(ctx, contractID, payerID, 41, dueDate, invoice.TypeRent, tenantID, sourceEventID, chargeID)
	if !errors.Is(err, ErrInvoiceIdempotencyConflict) {
		t.Fatalf("changed payload retry error = %v, want ErrInvoiceIdempotencyConflict", err)
	}
}

func TestInvoicePostgresConcurrentDistinctCommandsCannotOverReserveCharge(t *testing.T) {
	db := openFinancialConsistencyPostgres(t)
	tenantID := uuid.NewString()
	chargeID := uuid.NewString()
	contractID := uuid.NewString()
	payerID := uuid.NewString()
	sourceEventIDs := [2]string{uuid.NewString(), uuid.NewString()}
	ctx, cancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 20*time.Second)
	defer cancel()

	expectCharge := false
	expectInvoice := false
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 10*time.Second)
		defer cleanupCancel()
		invoiceResult, invoiceErr := db.Exec(cleanupCtx, `DELETE FROM financial.invoices
			WHERE tenant_id = $1 AND (metadata->>'source_event_id') IN ($2, $3)`,
			tenantID, sourceEventIDs[0], sourceEventIDs[1])
		if invoiceErr != nil {
			t.Errorf("cleanup distinct-command invoices: %v", invoiceErr)
		} else if expectInvoice {
			if rows, _ := invoiceResult.RowsAffected(); rows != 1 {
				t.Errorf("cleanup distinct-command invoices removed %d rows, want 1", rows)
			}
		}
		chargeResult, chargeErr := db.Exec(cleanupCtx,
			"DELETE FROM financial.charges WHERE tenant_id = $1 AND id = $2", tenantID, chargeID)
		if chargeErr != nil {
			t.Errorf("cleanup distinct-command charge: %v", chargeErr)
		} else if expectCharge {
			if rows, _ := chargeResult.RowsAffected(); rows != 1 {
				t.Errorf("cleanup distinct-command charge removed %d rows, want 1", rows)
			}
		}
		var residual int
		if err := db.QueryRow(cleanupCtx, `SELECT
			(SELECT COUNT(*) FROM financial.invoices WHERE tenant_id = $1 AND (metadata->>'source_event_id') IN ($2, $3)) +
			(SELECT COUNT(*) FROM financial.charges WHERE tenant_id = $1 AND id = $4)`,
			tenantID, sourceEventIDs[0], sourceEventIDs[1], chargeID).Scan(&residual); err != nil {
			t.Errorf("verify distinct-command cleanup: %v", err)
		} else if residual != 0 {
			t.Errorf("distinct-command cleanup left %d rows", residual)
		}
	})

	chargeRepo := repositories.NewChargePostgresRepository(db)
	receivable, err := charge.New(
		contractID, payerID, uuid.NewString(), uuid.NewString(), tenantID,
		charge.TypeRent, "Concurrent distinct invoice commands", "2026-08", 100,
		time.Now().UTC().AddDate(0, 0, 5),
	)
	if err != nil {
		t.Fatalf("create distinct-command charge: %v", err)
	}
	receivable.ID = chargeID
	if err := chargeRepo.Create(ctx, receivable); err != nil {
		t.Fatalf("persist distinct-command charge: %v", err)
	}
	expectCharge = true

	invoiceRepo := repositories.NewInvoicePostgresRepository(db)
	svc := NewInvoiceService(invoiceRepo, nil).
		WithPaymentApplication(NewChargeService(chargeRepo, nil)).
		WithDB(db)
	type createResult struct {
		invoiceID string
		err       error
	}
	start := make(chan struct{})
	results := make(chan createResult, len(sourceEventIDs))
	for _, sourceEventID := range sourceEventIDs {
		sourceEventID := sourceEventID
		go func() {
			<-start
			inv, createErr := svc.CreateWithSourceAndCharge(
				ctx, contractID, payerID, 60, receivable.DueDate, invoice.TypeRent,
				tenantID, sourceEventID, chargeID,
			)
			if createErr != nil {
				results <- createResult{err: createErr}
				return
			}
			results <- createResult{invoiceID: inv.ID}
		}()
	}
	close(start)

	var created, rejected int
	for range sourceEventIDs {
		result := <-results
		switch {
		case result.err == nil && result.invoiceID != "":
			created++
		case errors.Is(result.err, ErrInvoiceAmountExceedsChargeBalance):
			rejected++
		default:
			t.Fatalf("unexpected distinct-command result: id=%q err=%v", result.invoiceID, result.err)
		}
	}
	if created != 1 || rejected != 1 {
		t.Fatalf("distinct commands created=%d rejected=%d, want 1/1", created, rejected)
	}
	expectInvoice = true

	var openCount int
	var openCents int64
	if err := db.QueryRow(ctx, `SELECT COUNT(*),
		COALESCE(SUM(ROUND((metadata->>'amount')::numeric * 100)), 0)::bigint
		FROM financial.invoices
		WHERE tenant_id = $1
		  AND (metadata->>'charge_id') = $2
		  AND COALESCE(metadata->>'status', '') NOT IN ('PAID', 'CANCELLED')
		  AND deleted_at IS NULL`, tenantID, chargeID).Scan(&openCount, &openCents); err != nil {
		t.Fatalf("read open invoice reservations: %v", err)
	}
	if openCount != 1 || openCents != 6_000 || openCents > receivable.TotalAmount.Cents() {
		t.Fatalf("open invoice reservation count=%d cents=%d charge=%d", openCount, openCents, receivable.TotalAmount.Cents())
	}
}

func TestInvoiceIssuanceAndChargePaymentSerializeWithoutNegativeAvailableBalance(t *testing.T) {
	db := openFinancialConsistencyPostgres(t)
	tenantID := uuid.NewString()
	chargeID := uuid.NewString()
	contractID := uuid.NewString()
	payerID := uuid.NewString()
	sourceEventID := uuid.NewString()
	paymentID := uuid.NewString()
	ctx, cancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 20*time.Second)
	defer cancel()

	expectCharge := false
	createdInvoices := int64(0)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 10*time.Second)
		defer cleanupCancel()
		invoiceResult, invoiceErr := db.Exec(cleanupCtx, `DELETE FROM financial.invoices
			WHERE tenant_id = $1 AND (metadata->>'source_event_id') = $2`, tenantID, sourceEventID)
		if invoiceErr != nil {
			t.Errorf("cleanup issue/payment race invoice: %v", invoiceErr)
		} else if rows, _ := invoiceResult.RowsAffected(); rows != createdInvoices {
			t.Errorf("cleanup issue/payment race invoice removed %d rows, want %d", rows, createdInvoices)
		}
		chargeResult, chargeErr := db.Exec(cleanupCtx,
			"DELETE FROM financial.charges WHERE tenant_id = $1 AND id = $2", tenantID, chargeID)
		if chargeErr != nil {
			t.Errorf("cleanup issue/payment race charge: %v", chargeErr)
		} else if expectCharge {
			if rows, _ := chargeResult.RowsAffected(); rows != 1 {
				t.Errorf("cleanup issue/payment race charge removed %d rows, want 1", rows)
			}
		}
		var residual int
		if err := db.QueryRow(cleanupCtx, `SELECT
			(SELECT COUNT(*) FROM financial.invoices WHERE tenant_id = $1 AND (metadata->>'source_event_id') = $2) +
			(SELECT COUNT(*) FROM financial.charges WHERE tenant_id = $1 AND id = $3)`,
			tenantID, sourceEventID, chargeID).Scan(&residual); err != nil {
			t.Errorf("verify issue/payment race cleanup: %v", err)
		} else if residual != 0 {
			t.Errorf("issue/payment race cleanup left %d rows", residual)
		}
	})

	chargeRepo := repositories.NewChargePostgresRepository(db)
	receivable, err := charge.New(
		contractID, payerID, uuid.NewString(), uuid.NewString(), tenantID,
		charge.TypeRent, "Invoice issuance versus payment", "2026-08", 100,
		time.Now().UTC().AddDate(0, 0, 5),
	)
	if err != nil {
		t.Fatalf("create issue/payment race charge: %v", err)
	}
	receivable.ID = chargeID
	if err := chargeRepo.Create(ctx, receivable); err != nil {
		t.Fatalf("persist issue/payment race charge: %v", err)
	}
	expectCharge = true

	invoiceRepo := repositories.NewInvoicePostgresRepository(db)
	chargeService := NewChargeService(chargeRepo, nil).
		WithDB(db).
		WithInvoiceReservations(invoiceRepo)
	invoiceService := NewInvoiceService(invoiceRepo, nil).
		WithPaymentApplication(chargeService).
		WithDB(db)

	type operationResult struct {
		kind string
		err  error
	}
	start := make(chan struct{})
	results := make(chan operationResult, 2)
	go func() {
		<-start
		_, createErr := invoiceService.CreateWithSourceAndCharge(
			ctx, contractID, payerID, 60, receivable.DueDate, invoice.TypeRent,
			tenantID, sourceEventID, chargeID,
		)
		results <- operationResult{kind: "invoice", err: createErr}
	}()
	go func() {
		<-start
		_, paymentErr := chargeService.MarkPaidIdempotent(
			ctx, tenantID, chargeID, "PIX", 80, paymentID, time.Now().UTC(),
		)
		results <- operationResult{kind: "payment", err: paymentErr}
	}()
	close(start)

	var invoiceApplied, paymentApplied bool
	for i := 0; i < 2; i++ {
		result := <-results
		switch result.kind {
		case "invoice":
			if result.err == nil {
				invoiceApplied = true
				continue
			}
			if !errors.Is(result.err, ErrInvoiceAmountExceedsChargeBalance) {
				t.Fatalf("invoice race result = %v, want success or balance conflict", result.err)
			}
		case "payment":
			if result.err == nil {
				paymentApplied = true
				continue
			}
			if !errors.Is(result.err, ErrChargePaymentExceedsUnreservedBalance) {
				t.Fatalf("payment race result = %v, want success or reservation conflict", result.err)
			}
		default:
			t.Fatalf("unknown race operation %q", result.kind)
		}
	}
	if invoiceApplied == paymentApplied {
		t.Fatalf("race effective operations invoice=%t payment=%t, want exactly one", invoiceApplied, paymentApplied)
	}
	if invoiceApplied {
		createdInvoices = 1
	}

	persistedCharge, err := chargeRepo.GetByID(ctx, tenantID, chargeID)
	if err != nil {
		t.Fatalf("reload issue/payment race charge: %v", err)
	}
	paidCents := int64(0)
	if persistedCharge.PaidAmount != nil {
		paidCents = persistedCharge.PaidAmount.Cents()
	}
	openCents, err := invoiceRepo.SumOpenAmountCentsByChargeID(ctx, tenantID, chargeID, "")
	if err != nil {
		t.Fatalf("read issue/payment race reservations: %v", err)
	}
	availableCents := persistedCharge.TotalAmount.Cents() - paidCents - openCents
	if availableCents < 0 {
		t.Fatalf("negative available balance: total=%d paid=%d open=%d available=%d", persistedCharge.TotalAmount.Cents(), paidCents, openCents, availableCents)
	}
	if invoiceApplied && (paidCents != 0 || openCents != 6_000 || availableCents != 4_000) {
		t.Fatalf("invoice-won state: paid=%d open=%d available=%d", paidCents, openCents, availableCents)
	}
	if paymentApplied && (paidCents != 8_000 || openCents != 0 || availableCents != 2_000) {
		t.Fatalf("payment-won state: paid=%d open=%d available=%d", paidCents, openCents, availableCents)
	}
}

type failingInvoiceUpdateRepository struct {
	invoice.Repository
	locking invoicePaymentLockingRepository
	err     error
}

type postgresInvoiceOutboxBus struct {
	enqueued []events.Event
}

func (b *postgresInvoiceOutboxBus) Publish(context.Context, events.Event) error {
	return errors.New("integration invoice event must use transactional outbox")
}
func (b *postgresInvoiceOutboxBus) PublishReliable(context.Context, events.Event) error {
	return errors.New("integration invoice event must use transactional outbox")
}
func (b *postgresInvoiceOutboxBus) Subscribe(string, events.EventHandler) error { return nil }
func (b *postgresInvoiceOutboxBus) SubscribeWithQueue(string, string, events.EventHandler) error {
	return nil
}
func (b *postgresInvoiceOutboxBus) EnqueueOutboxInTx(ctx context.Context, tx *sqlx.Tx, event events.Event) (string, error) {
	if tx == nil {
		return "", errors.New("transaction is required")
	}
	payload, err := json.Marshal(event.Payload())
	if err != nil {
		return "", err
	}
	typed, ok := event.Payload().(contracts.InvoicePaidEventPayload)
	if !ok {
		return "", fmt.Errorf("unexpected integration event payload %T", event.Payload())
	}
	var id string
	if err := tx.QueryRowContext(ctx, `INSERT INTO events.outbox_events (tenant_id, event_name, payload, status)
		VALUES ($1, $2, $3, 'PENDING') RETURNING id`, typed.TenantID, event.Name(), string(payload)).Scan(&id); err != nil {
		return "", err
	}
	b.enqueued = append(b.enqueued, event)
	return id, nil
}

func (r *failingInvoiceUpdateRepository) GetByIDForUpdate(ctx context.Context, tenantID, id string) (*invoice.Invoice, error) {
	return r.locking.GetByIDForUpdate(ctx, tenantID, id)
}

func (r *failingInvoiceUpdateRepository) Update(context.Context, *invoice.Invoice) error {
	return r.err
}

func TestInvoiceChargePostgresPaymentCommitsAndRollsBackAsOneUnit(t *testing.T) {
	db := openFinancialConsistencyPostgres(t)
	tenantID := uuid.NewString()
	ctx, cancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 20*time.Second)
	defer cancel()

	chargeIDs := [2]string{uuid.NewString(), uuid.NewString()}
	invoiceIDs := [2]string{uuid.NewString(), uuid.NewString()}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 10*time.Second)
		defer cleanupCancel()
		outboxResult, outboxErr := db.Exec(cleanupCtx, `DELETE FROM events.outbox_events
			WHERE tenant_id = $1 AND event_name = 'invoice.paid'
			  AND payload->>'invoice_id' IN ($2, $3)`, tenantID, invoiceIDs[0], invoiceIDs[1])
		if outboxErr != nil {
			t.Errorf("cleanup invoice outbox: %v", outboxErr)
		} else if rows, _ := outboxResult.RowsAffected(); rows != 1 {
			t.Errorf("cleanup invoice outbox removed %d rows, want 1", rows)
		}
		invoiceResult, invoiceErr := db.Exec(cleanupCtx,
			"DELETE FROM financial.invoices WHERE tenant_id = $1 AND id IN ($2, $3)",
			tenantID, invoiceIDs[0], invoiceIDs[1])
		if invoiceErr != nil {
			t.Errorf("cleanup invoices: %v", invoiceErr)
		} else if rows, _ := invoiceResult.RowsAffected(); rows != 2 {
			t.Errorf("cleanup invoices removed %d rows, want 2", rows)
		}
		chargeResult, chargeErr := db.Exec(cleanupCtx,
			"DELETE FROM financial.charges WHERE tenant_id = $1 AND id IN ($2, $3)",
			tenantID, chargeIDs[0], chargeIDs[1])
		if chargeErr != nil {
			t.Errorf("cleanup charges: %v", chargeErr)
		} else if rows, _ := chargeResult.RowsAffected(); rows != 2 {
			t.Errorf("cleanup charges removed %d rows, want 2", rows)
		}
		var residual int
		if err := db.QueryRow(cleanupCtx, `SELECT
			(SELECT COUNT(*) FROM financial.invoices WHERE tenant_id = $1 AND id::text IN ($2::text, $3::text)) +
			(SELECT COUNT(*) FROM financial.charges WHERE tenant_id = $1 AND id::text IN ($4::text, $5::text)) +
			(SELECT COUNT(*) FROM events.outbox_events WHERE tenant_id = $1 AND event_name = 'invoice.paid' AND payload->>'invoice_id' IN ($2::text, $3::text))`,
			tenantID, invoiceIDs[0], invoiceIDs[1], chargeIDs[0], chargeIDs[1]).Scan(&residual); err != nil {
			t.Errorf("verify invoice/charge cleanup: %v", err)
		} else if residual != 0 {
			t.Errorf("invoice/charge cleanup left %d rows", residual)
		}
	})

	chargeRepo := repositories.NewChargePostgresRepository(db)
	invoiceRepo := repositories.NewInvoicePostgresRepository(db)
	for i := range chargeIDs {
		receivable, err := charge.New(
			"contract-"+uuid.NewString(), "payer-"+uuid.NewString(), "owner-"+uuid.NewString(), "property-"+uuid.NewString(),
			tenantID, charge.TypeRent, "Postgres payment consistency", "2026-08", 100, time.Now().UTC().AddDate(0, 0, 5),
		)
		if err != nil {
			t.Fatalf("create charge fixture %d: %v", i, err)
		}
		receivable.ID = chargeIDs[i]
		if err := chargeRepo.Create(ctx, receivable); err != nil {
			t.Fatalf("persist charge fixture %d: %v", i, err)
		}

		inv, err := invoice.New(receivable.ContractID, receivable.TenantPartyID, 40, receivable.DueDate, invoice.TypeRent, tenantID)
		if err != nil {
			t.Fatalf("create invoice fixture %d: %v", i, err)
		}
		inv.ID = invoiceIDs[i]
		inv.ChargeID = receivable.ID
		inv.Status = invoice.StatusIssued
		if err := invoiceRepo.Create(ctx, inv); err != nil {
			t.Fatalf("persist invoice fixture %d: %v", i, err)
		}
	}

	chargeService := NewChargeService(chargeRepo, nil).
		WithDB(db).
		WithInvoiceReservations(invoiceRepo)
	outboxBus := &postgresInvoiceOutboxBus{}
	invoiceService := NewInvoiceService(invoiceRepo, outboxBus).
		WithPaymentApplication(chargeService).
		WithDB(db)
	paidAt := time.Now().UTC().Truncate(time.Microsecond)
	paid, err := invoiceService.PayIdempotent(ctx, tenantID, invoiceIDs[0], "payment-"+uuid.NewString(), "PIX", paidAt)
	if err != nil {
		t.Fatalf("pay linked invoice: %v", err)
	}
	if paid.Status != invoice.StatusPaid {
		t.Fatalf("invoice status = %s, want PAID", paid.Status)
	}
	persistedCharge, err := chargeRepo.GetByID(ctx, tenantID, chargeIDs[0])
	if err != nil {
		t.Fatalf("reload committed charge: %v", err)
	}
	if len(persistedCharge.Payments) != 1 || persistedCharge.PaidAmount == nil || persistedCharge.PaidAmount.Cents() != 4_000 || persistedCharge.Status != charge.StatusPartiallyPaid {
		t.Fatalf("committed charge payment = %+v", persistedCharge)
	}
	persistedInvoice, err := invoiceRepo.GetByID(ctx, tenantID, invoiceIDs[0])
	if err != nil || persistedInvoice.Status != invoice.StatusPaid {
		t.Fatalf("committed invoice projection = (%+v, %v)", persistedInvoice, err)
	}
	var outboxCount int
	var outboxChargeID string
	var outboxAmount float64
	if err := db.QueryRow(ctx, `SELECT COUNT(*),
		COALESCE(MAX(payload->>'charge_id'), ''),
		COALESCE(MAX((payload->>'amount')::numeric), 0)
		FROM events.outbox_events
		WHERE tenant_id = $1 AND event_name = 'invoice.paid' AND payload->>'invoice_id' = $2`,
		tenantID, invoiceIDs[0]).Scan(&outboxCount, &outboxChargeID, &outboxAmount); err != nil {
		t.Fatalf("read committed invoice.paid outbox: %v", err)
	}
	if outboxCount != 1 || outboxChargeID != chargeIDs[0] || outboxAmount != 40 || len(outboxBus.enqueued) != 1 {
		t.Fatalf("invoice.paid outbox count=%d charge=%s amount=%.2f captured=%d", outboxCount, outboxChargeID, outboxAmount, len(outboxBus.enqueued))
	}

	locker, ok := invoiceRepo.(invoicePaymentLockingRepository)
	if !ok {
		t.Fatal("postgres invoice repository does not expose GetByIDForUpdate")
	}
	wantRollback := errors.New("force invoice projection rollback")
	failingRepo := &failingInvoiceUpdateRepository{Repository: invoiceRepo, locking: locker, err: wantRollback}
	failingService := NewInvoiceService(failingRepo, outboxBus).
		WithPaymentApplication(chargeService).
		WithDB(db)
	_, err = failingService.PayIdempotent(ctx, tenantID, invoiceIDs[1], "payment-"+uuid.NewString(), "PIX", paidAt)
	if !errors.Is(err, wantRollback) {
		t.Fatalf("failing projection payment error = %v, want rollback sentinel", err)
	}

	rolledBackCharge, err := chargeRepo.GetByID(ctx, tenantID, chargeIDs[1])
	if err != nil {
		t.Fatalf("reload rolled-back charge: %v", err)
	}
	if len(rolledBackCharge.Payments) != 0 || rolledBackCharge.PaidAmount != nil || rolledBackCharge.Status != charge.StatusPending {
		t.Fatalf("charge payment escaped rolled-back transaction: %+v", rolledBackCharge)
	}
	rolledBackInvoice, err := invoiceRepo.GetByID(ctx, tenantID, invoiceIDs[1])
	if err != nil || rolledBackInvoice.Status != invoice.StatusIssued || rolledBackInvoice.PaidAt != nil {
		t.Fatalf("rolled-back invoice projection = (%+v, %v)", rolledBackInvoice, err)
	}
}
