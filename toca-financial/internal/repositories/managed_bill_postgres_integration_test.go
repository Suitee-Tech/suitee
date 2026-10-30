package repositories

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/config"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/managed_bill"
)

func TestManagedBillPostgresTenantScopeCASAndTransactionReuse(t *testing.T) {
	databaseURI := os.Getenv("TOCA_TEST_DATABASE_URI")
	if databaseURI == "" {
		t.Skip("TOCA_TEST_DATABASE_URI is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{
		URI:         databaseURI,
		ServiceURI:  os.Getenv("DATABASE_URI_SERVICE_TEST"),
		MaxPoolSize: 8,
		MinPoolSize: 0,
	})
	if err != nil {
		t.Fatalf("connect managed bill test postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	chargeID := uuid.NewString()
	transferID := uuid.NewString()
	chargeItemID := uuid.NewString()
	rolledBackItemID := uuid.NewString()
	paymentJournalID := uuid.NewString()
	reversalJournalID := uuid.NewString()
	invalidReversalJournalID := uuid.NewString()
	forgedReversalJournalID := uuid.NewString()
	managedBillID := uuid.NewString()
	ctxA := pkgcontext.WithTenantID(ctx, tenantA)
	ctxB := pkgcontext.WithTenantID(ctx, tenantB)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantA), 10*time.Second)
		defer cleanupCancel()
		if _, cleanupErr := deleteManagedBillJournalFixturesInTest(cleanupCtx, db,
			"SELECT id::text FROM financial.journal_entries WHERE tenant_id = $1 AND id IN ($2, $3, $4, $5)",
			tenantA, paymentJournalID, reversalJournalID, invalidReversalJournalID, forgedReversalJournalID); cleanupErr != nil {
			t.Errorf("cleanup managed bill journals: %v", cleanupErr)
		}
		if _, cleanupErr := db.Exec(cleanupCtx,
			"DELETE FROM financial.managed_bills WHERE tenant_id = $1 AND id = $2",
			tenantA, managedBillID); cleanupErr != nil {
			t.Errorf("cleanup managed bills: %v", cleanupErr)
		}
		if _, cleanupErr := db.Exec(cleanupCtx,
			"DELETE FROM financial.transfers WHERE tenant_id = $1 AND id = $2",
			tenantA, transferID); cleanupErr != nil {
			t.Errorf("cleanup managed bill transfer: %v", cleanupErr)
		}
		if _, cleanupErr := db.Exec(cleanupCtx,
			"DELETE FROM financial.charges WHERE tenant_id = $1 AND id = $2",
			tenantA, chargeID); cleanupErr != nil {
			t.Errorf("cleanup managed bill charge: %v", cleanupErr)
		}

		var residualRows int64
		if cleanupErr := db.QueryRow(cleanupCtx, `
			SELECT
				(SELECT COUNT(*) FROM financial.journal_entries
					 WHERE tenant_id = $1 AND id IN ($2, $3, $4, $5))
				+ (SELECT COUNT(*) FROM financial.managed_bills
				   WHERE tenant_id = $1 AND id = $6)
				+ (SELECT COUNT(*) FROM financial.transfers
				   WHERE tenant_id = $1 AND id = $7)
				+ (SELECT COUNT(*) FROM financial.charges
				   WHERE tenant_id = $1 AND id = $8)`,
			tenantA, paymentJournalID, reversalJournalID, invalidReversalJournalID, forgedReversalJournalID,
			managedBillID, transferID, chargeID,
		).Scan(&residualRows); cleanupErr != nil {
			t.Errorf("verify managed bill fixture cleanup: %v", cleanupErr)
		} else if residualRows != 0 {
			t.Errorf("managed bill fixture cleanup left %d rows for exact fixture IDs", residualRows)
		}
	})

	if _, err := db.Exec(ctxA,
		`INSERT INTO financial.charges (id, tenant_id, metadata)
		 VALUES ($1::uuid, $2::uuid, jsonb_build_object(
			'id', ($1::uuid)::text,
			'tenant_id', ($2::uuid)::text
		 ))`,
		chargeID, tenantA); err != nil {
		t.Fatalf("seed managed bill charge: %v", err)
	}
	if _, err := db.Exec(ctxA,
		`INSERT INTO financial.transfers (id, tenant_id, metadata)
		 VALUES ($1::uuid, $2::uuid, jsonb_build_object(
			'id', ($1::uuid)::text,
			'tenant_id', ($2::uuid)::text,
			'source_charge_id', ($3::uuid)::text
		 ))`,
		transferID, tenantA, chargeID); err != nil {
		t.Fatalf("seed managed bill transfer: %v", err)
	}

	repo := NewManagedBillPostgresRepository(db)
	reservedAt := time.Date(2026, time.August, 20, 12, 34, 56, 123_456_789, time.UTC)
	bill := newPostgresManagedBillFixture(t, tenantA, transferID, chargeID, chargeItemID, "managed-bill:"+uuid.NewString(), reservedAt)
	managedBillID = bill.ID
	if err := repo.Create(ctxA, bill); err != nil {
		t.Fatalf("create managed bill: %v", err)
	}

	if _, err := db.Exec(ctxA, `
		INSERT INTO financial.managed_bills (
			id, tenant_id, source_transfer_id, source_charge_id, charge_item_id,
			source_key, contract_id, property_id, owner_id, component, creditor,
			description, amount_cents, currency, due_date, competence_month,
			status, version, evidence, reserved_by, approved_by, paid_by,
			reserved_at, approved_at, paid_at, payment_reference,
			payment_journal_source_id, history, created_at, updated_at
		) VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid,
			$6, 'forged-contract', 'forged-property', 'forged-owner',
			'CONDOMINIO', 'Forged creditor', 'Forged pre-paid bill',
			70000, 'BRL', $7::timestamptz::date, '2026-08',
			'PAID', 1, '[]'::jsonb, 'forged-reserver', 'forged-approver',
			'forged-payer', $7::timestamptz, $7::timestamptz, $7::timestamptz,
			'forged-payment', 'forged-journal',
			jsonb_build_array(jsonb_build_object(
				'action', 'reserved', 'actor', 'forged-reserver',
				'at', $7::timestamptz, 'version', 1
			)),
			$7::timestamptz, $7::timestamptz
		)`,
		uuid.NewString(), tenantA, transferID, chargeID, uuid.NewString(),
		"managed-bill:forged:"+uuid.NewString(), reservedAt); err == nil {
		t.Fatal("direct insert in PAID status unexpectedly succeeded")
	} else if !strings.Contains(err.Error(), "managed bill must be inserted as RESERVED version 1") {
		t.Fatalf("direct PAID insert failed for the wrong reason: %v", err)
	}

	if _, err := repo.GetByID(ctxB, tenantB, bill.ID); !errors.Is(err, managed_bill.ErrNotFound) {
		t.Fatalf("cross-tenant GetByID error = %v, want ErrNotFound", err)
	}
	if foreign, err := repo.GetBySourceKey(ctxB, tenantB, bill.SourceKey); err != nil || foreign != nil {
		t.Fatalf("cross-tenant GetBySourceKey = (%+v, %v), want (nil, nil)", foreign, err)
	}

	duplicate := newPostgresManagedBillFixture(t, tenantA, transferID, chargeID, uuid.NewString(), bill.SourceKey, reservedAt)
	if err := repo.Create(ctxA, duplicate); !errors.Is(err, managed_bill.ErrAlreadyExists) {
		t.Fatalf("duplicate source key error = %v, want ErrAlreadyExists", err)
	}

	reservedCents, err := repo.SumActiveReservedCentsByTransfer(ctxA, tenantA, transferID)
	if err != nil {
		t.Fatalf("sum active reservations: %v", err)
	}
	if reservedCents != bill.AmountCents {
		t.Fatalf("active reservation sum = %d, want %d", reservedCents, bill.AmountCents)
	}

	winner, err := repo.GetByID(ctxA, tenantA, bill.ID)
	if err != nil {
		t.Fatalf("load CAS winner: %v", err)
	}
	stale, err := repo.GetByID(ctxA, tenantA, bill.ID)
	if err != nil {
		t.Fatalf("load CAS stale copy: %v", err)
	}
	approvedAt := reservedAt.Add(time.Second)
	if err := winner.Approve("approver-a", approvedAt, managed_bill.Evidence{
		Kind: "approval_note", Reference: "approval-evidence", Actor: "approver-a", At: approvedAt,
	}); err != nil {
		t.Fatalf("approve winner: %v", err)
	}
	if err := stale.Approve("approver-b", approvedAt); err != nil {
		t.Fatalf("approve stale candidate: %v", err)
	}
	if updated, err := repo.UpdateIfVersion(ctxA, winner, 1); err != nil || !updated {
		t.Fatalf("persist CAS winner = (%t, %v), want (true, nil)", updated, err)
	}
	if updated, err := repo.UpdateIfVersion(ctxA, stale, 1); updated || !errors.Is(err, managed_bill.ErrConcurrentModification) {
		t.Fatalf("persist stale CAS = (%t, %v), want (false, ErrConcurrentModification)", updated, err)
	}

	if _, err := db.Exec(ctxA,
		`UPDATE financial.managed_bills
		 SET amount_cents = amount_cents + 1, version = version + 1
		 WHERE tenant_id = $1::uuid AND id = $2::uuid`,
		tenantA, bill.ID); err == nil {
		t.Fatal("direct economic mutation unexpectedly succeeded")
	} else if !strings.Contains(err.Error(), "managed bill economic attributes are immutable") {
		t.Fatalf("direct economic mutation failed for the wrong reason: %v", err)
	}
	if _, err := db.Exec(ctxA,
		`UPDATE financial.managed_bills
		 SET evidence = jsonb_build_array(jsonb_build_object('kind', 'tampered')),
		     history = history || jsonb_build_array(jsonb_build_object(
		         'action', 'evidence_attached', 'actor', 'tampered-actor',
		         'at', updated_at, 'version', version + 1
		     )),
		     version = version + 1
		 WHERE tenant_id = $1::uuid AND id = $2::uuid`,
		tenantA, bill.ID); err == nil {
		t.Fatal("direct evidence rewrite unexpectedly succeeded")
	} else if !strings.Contains(err.Error(), "managed bill evidence entries are immutable") {
		t.Fatalf("direct evidence rewrite failed for the wrong reason: %v", err)
	}
	if _, err := db.Exec(ctxA,
		`UPDATE financial.managed_bills
		 SET approved_by = 'tampered-approver',
		     history = history || jsonb_build_array(jsonb_build_object(
		         'action', 'tampered', 'actor', 'tampered-approver',
		         'at', updated_at, 'version', version + 1
		     )),
		     version = version + 1
		 WHERE tenant_id = $1::uuid AND id = $2::uuid`,
		tenantA, bill.ID); err == nil {
		t.Fatal("direct lifecycle milestone rewrite unexpectedly succeeded")
	} else if !strings.Contains(err.Error(), "managed bill approval milestone is write-once") {
		t.Fatalf("direct lifecycle rewrite failed for the wrong reason: %v", err)
	}
	if _, err := db.Exec(ctxA,
		`UPDATE financial.managed_bills
		 SET version = version + 1
		 WHERE tenant_id = $1::uuid AND id = $2::uuid`,
		tenantA, bill.ID); err == nil {
		t.Fatal("version bump without history append unexpectedly succeeded")
	} else if !strings.Contains(err.Error(), "managed bill history must append exactly one entry") {
		t.Fatalf("history-free version bump failed for the wrong reason: %v", err)
	}

	approved, total, err := repo.List(ctxA, tenantA, managed_bill.Filter{
		SourceTransferID: transferID,
		Status:           managed_bill.StatusApproved,
		Search:           "Condominio",
		Limit:            10,
	})
	if err != nil {
		t.Fatalf("list approved managed bills: %v", err)
	}
	if total != 1 || len(approved) != 1 || approved[0].ID != bill.ID || approved[0].Version != 2 {
		t.Fatalf("approved list = (%d, %+v), want canonical version 2", total, approved)
	}

	paidBill, err := repo.GetByID(ctxA, tenantA, bill.ID)
	if err != nil {
		t.Fatalf("load approved bill for payment: %v", err)
	}
	paidAt := approvedAt.Add(time.Hour)
	paymentJournalSourceID := "managed-bill.payment:" + uuid.NewString()
	paymentJournal, err := journal.New(
		tenantA,
		paidBill.ContractID,
		"Pagamento de conta administrada: "+paidBill.Description,
		paidAt,
		[]journal.Leg{
			{Account: journal.AccountOwnerPayable, Direction: journal.Debit, AmountCents: paidBill.AmountCents, ManagedBillID: paidBill.ID},
			{Account: journal.AccountCash, Direction: journal.Credit, AmountCents: paidBill.AmountCents, ManagedBillID: paidBill.ID},
		},
	)
	if err != nil {
		t.Fatalf("build managed bill payment journal: %v", err)
	}
	paymentJournal.ID = paymentJournalID
	paymentJournal.OperationID = "pix-payment-a"
	paymentJournal.SourceEventID = paymentJournalSourceID
	journalRepo := NewJournalPostgresRepository(db)
	if err := journalRepo.Create(ctxA, paymentJournal); err != nil {
		t.Fatalf("seed canonical managed bill payment journal: %v", err)
	}
	if err := paidBill.MarkPaid("payer-a", "pix-payment-a", paymentJournalSourceID, paidAt); err != nil {
		t.Fatalf("mark managed bill paid: %v", err)
	}
	if updated, err := repo.UpdateIfVersion(ctxA, paidBill, 2); err != nil || !updated {
		t.Fatalf("persist managed bill payment = (%t, %v), want (true, nil)", updated, err)
	}

	missingJournalBill, err := repo.GetByID(ctxA, tenantA, bill.ID)
	if err != nil {
		t.Fatalf("load paid bill for forged reversal: %v", err)
	}
	forgedReversedAt := paidAt.Add(time.Hour)
	if err := missingJournalBill.ReversePayment(
		"reverser-a", "refund-without-journal", "journal.reversal:missing", "bank rejected payment",
		forgedReversedAt,
	); err != nil {
		t.Fatalf("stage forged managed bill reversal: %v", err)
	}
	if updated, err := repo.UpdateIfVersion(ctxA, missingJournalBill, 3); updated || err == nil {
		t.Fatalf("reversal without canonical journal = (%t, %v), want rejected", updated, err)
	} else if !strings.Contains(err.Error(), "managed bill payment reversal journal is missing or economically inconsistent") {
		t.Fatalf("reversal without journal failed for the wrong reason: %v", err)
	}

	reversalJournalSourceID := "journal.reversal:" + uuid.NewString()
	reversedAt := forgedReversedAt.Add(time.Hour)
	canonicalReversal, err := paymentJournal.ReverseAt("bank rejected payment", "reverser-a", reversedAt)
	if err != nil {
		t.Fatalf("build canonical managed bill reversal journal: %v", err)
	}
	canonicalReversal.ID = reversalJournalID
	canonicalReversal.SourceEventID = reversalJournalSourceID
	forgedReversal, err := paymentJournal.ReverseAt("forged reason", "reverser-a", reversedAt)
	if err != nil {
		t.Fatalf("build forged managed bill reversal journal: %v", err)
	}
	forgedReversal.ID = forgedReversalJournalID
	forgedReversal.SourceEventID = reversalJournalSourceID

	expectedJournalVersion := paymentJournal.UpdatedAt
	if err := paymentJournal.MarkReversed(); err != nil {
		t.Fatalf("stage managed bill payment journal reversal status: %v", err)
	}
	if err := db.RunInTx(ctxA, func(tx *sqlx.Tx) error {
		txCtx := database.WithTx(ctxA, tx)
		updated, err := journalRepo.UpdateStatus(txCtx, paymentJournal, expectedJournalVersion)
		if err != nil {
			return err
		}
		if !updated {
			return errors.New("managed bill payment journal status was not updated")
		}
		return journalRepo.Create(txCtx, forgedReversal)
	}); err != nil {
		t.Fatalf("persist reversed journal with its initial mirror: %v", err)
	}

	if _, err := db.Exec(ctxA, `
		INSERT INTO financial.journal_entries (id, tenant_id, metadata)
		VALUES ($1::uuid, $2::uuid, jsonb_build_object(
			'id', ($1::uuid)::text,
			'tenant_id', ($2::uuid)::text,
			'contract_id', $3::text,
			'operation_id', $4::text,
			'source_event_id', $5::text,
			'date', $6::timestamptz,
			'description', $7::text,
			'status', 'POSTED',
			'reversal_of', ($8::uuid)::text,
			'reversed_by', 'reverser-a'
		))`, invalidReversalJournalID, tenantA, paidBill.ContractID, paidBill.PaymentReference,
		reversalJournalSourceID+":invalid", reversedAt, "Estorno sem pernas", paymentJournalID); err == nil {
		t.Fatal("journal reversal without legs unexpectedly succeeded")
	} else if !strings.Contains(err.Error(), "journal entry requires a date and at least two legs") {
		t.Fatalf("journal reversal without legs failed for the wrong reason: %v", err)
	}

	forgedReasonBill, err := repo.GetByID(ctxA, tenantA, bill.ID)
	if err != nil {
		t.Fatalf("reload paid bill for forged-reason reversal: %v", err)
	}
	if err := forgedReasonBill.ReversePayment(
		"reverser-a", "refund-forged-reason", reversalJournalSourceID, "bank rejected payment", reversedAt,
	); err != nil {
		t.Fatalf("stage forged-reason managed bill reversal: %v", err)
	}
	if updated, err := repo.UpdateIfVersion(ctxA, forgedReasonBill, 3); updated || err == nil {
		t.Fatalf("reversal with forged reason witness = (%t, %v), want rejected", updated, err)
	} else if !strings.Contains(err.Error(), "managed bill payment reversal journal is missing or economically inconsistent") {
		t.Fatalf("forged-reason reversal failed for the wrong reason: %v", err)
	}
	if rows, err := deleteManagedBillJournalFixturesInTest(
		ctxA,
		db,
		"SELECT id::text FROM financial.journal_entries WHERE tenant_id = $1 AND id = $2",
		tenantA,
		forgedReversalJournalID,
	); err != nil || rows != 1 {
		t.Fatalf("delete forged reversal fixture = (%d, %v), want (1, nil)", rows, err)
	}
	if err := journalRepo.Create(ctxA, canonicalReversal); err != nil {
		t.Fatalf("seed canonical managed bill reversal journal: %v", err)
	}

	reversedBill, err := repo.GetByID(ctxA, tenantA, bill.ID)
	if err != nil {
		t.Fatalf("reload paid bill for canonical reversal: %v", err)
	}
	if err := reversedBill.ReversePayment(
		"reverser-a", "refund-a", reversalJournalSourceID, "bank rejected payment", reversedAt,
	); err != nil {
		t.Fatalf("reverse managed bill payment: %v", err)
	}
	if updated, err := repo.UpdateIfVersion(ctxA, reversedBill, 3); err != nil || !updated {
		t.Fatalf("persist canonical managed bill reversal = (%t, %v), want (true, nil)", updated, err)
	}

	reopened, err := repo.GetByID(ctxA, tenantA, bill.ID)
	if err != nil {
		t.Fatalf("reload reversed managed bill: %v", err)
	}
	if reopened.Status != managed_bill.StatusApproved || reopened.Version != 4 ||
		reopened.PaidAt != nil || reopened.PaidBy != "" || reopened.PaymentReference != "" || reopened.PaymentJournalSourceID != "" ||
		reopened.ReconciledAt != nil || reopened.ReconciledBy != "" || reopened.ReconciliationReference != "" {
		t.Fatalf("reopened managed bill retained active payment fields: %+v", reopened)
	}
	if len(reopened.PaymentAttempts) != 1 {
		t.Fatalf("reopened managed bill payment attempts = %+v, want one snapshot", reopened.PaymentAttempts)
	}
	attempt := reopened.PaymentAttempts[0]
	if attempt.ID != paymentJournalSourceID || attempt.PaymentJournalSourceID != paymentJournalSourceID ||
		attempt.PaymentReference != "pix-payment-a" || attempt.ReversalReference != "refund-a" ||
		attempt.ReversalJournalSourceID != reversalJournalSourceID || attempt.ReversedBy != "reverser-a" {
		t.Fatalf("unexpected persisted payment attempt: %+v", attempt)
	}

	if _, err := db.Exec(ctxA, `
		UPDATE financial.managed_bills
		SET payment_attempts = jsonb_set(payment_attempts, '{0,payment_reference}', to_jsonb('tampered'::text)),
		    history = history || jsonb_build_array(jsonb_build_object(
		        'action', 'tampered', 'actor', 'tampered-actor',
		        'at', updated_at + interval '1 microsecond', 'version', version + 1
		    )),
		    version = version + 1,
		    updated_at = updated_at + interval '1 microsecond'
		WHERE tenant_id = $1::uuid AND id = $2::uuid`, tenantA, bill.ID); err == nil {
		t.Fatal("direct payment-attempt rewrite unexpectedly succeeded")
	} else if !strings.Contains(err.Error(), "managed bill payment attempts may change only on payment reversal") {
		t.Fatalf("direct payment-attempt rewrite failed for the wrong reason: %v", err)
	}

	rolledBack := newPostgresManagedBillFixture(t, tenantA, transferID, chargeID, rolledBackItemID, "managed-bill:"+uuid.NewString(), reservedAt)
	wantRollback := errors.New("force managed bill rollback")
	err = db.RunInTx(ctxA, func(tx *sqlx.Tx) error {
		if createErr := repo.Create(database.WithTx(ctxA, tx), rolledBack); createErr != nil {
			return createErr
		}
		return wantRollback
	})
	if !errors.Is(err, wantRollback) {
		t.Fatalf("managed bill rollback error = %v, want sentinel", err)
	}
	if _, err := repo.GetByID(ctxA, tenantA, rolledBack.ID); !errors.Is(err, managed_bill.ErrNotFound) {
		t.Fatalf("rolled-back managed bill error = %v, want ErrNotFound", err)
	}
}

func TestJournalPostgresRejectsConcurrentDuplicateReversals(t *testing.T) {
	databaseURI := os.Getenv("TOCA_TEST_DATABASE_URI")
	if databaseURI == "" {
		t.Skip("TOCA_TEST_DATABASE_URI is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{
		URI:         databaseURI,
		ServiceURI:  os.Getenv("DATABASE_URI_SERVICE_TEST"),
		MaxPoolSize: 8,
		MinPoolSize: 0,
	})
	if err != nil {
		t.Fatalf("connect journal reversal uniqueness test postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	tenantID := uuid.NewString()
	originalID := uuid.NewString()
	seedReversalID := uuid.NewString()
	reversalAID := uuid.NewString()
	reversalBID := uuid.NewString()
	tenantCtx := pkgcontext.WithTenantID(ctx, tenantID)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 10*time.Second)
		defer cleanupCancel()
		rows, cleanupErr := deleteManagedBillJournalFixturesInTest(
			cleanupCtx,
			db,
			"SELECT id::text FROM financial.journal_entries WHERE tenant_id = $1::uuid AND id IN ($2::uuid, $3::uuid, $4::uuid, $5::uuid)",
			tenantID,
			originalID,
			seedReversalID,
			reversalAID,
			reversalBID,
		)
		if cleanupErr != nil {
			t.Errorf("cleanup journal reversal uniqueness fixtures: %v", cleanupErr)
			return
		}
		if rows != 2 {
			t.Errorf("cleanup journal reversal uniqueness fixtures removed %d rows, want 2", rows)
		}
	})

	repo := NewJournalPostgresRepository(db)
	postedAt := time.Date(2026, time.August, 22, 18, 0, 0, 0, time.UTC)
	original, err := journal.New(tenantID, "contract-"+uuid.NewString(), "Pagamento concorrente", postedAt, []journal.Leg{
		{Account: journal.AccountOwnerPayable, Direction: journal.Debit, AmountCents: 70_000},
		{Account: journal.AccountCash, Direction: journal.Credit, AmountCents: 70_000},
	})
	if err != nil {
		t.Fatalf("build original journal entry: %v", err)
	}
	original.ID = originalID
	original.OperationID = "payment-" + uuid.NewString()
	original.SourceEventID = "managed-bill.payment:" + uuid.NewString()
	if err := repo.Create(tenantCtx, original); err != nil {
		t.Fatalf("persist original journal entry: %v", err)
	}

	missingMirrorVersion := original.UpdatedAt
	if err := original.MarkReversed(); err != nil {
		t.Fatalf("stage journal reversal without mirror: %v", err)
	}
	if updated, err := repo.UpdateStatus(tenantCtx, original, missingMirrorVersion); err == nil || updated {
		t.Fatalf("journal reversal without mirror = (%t, %v), want rejected at commit", updated, err)
	} else if !strings.Contains(err.Error(), "journal reversal must commit with exactly one compensating entry") {
		t.Fatalf("journal reversal without mirror failed for the wrong reason: %v", err)
	}
	original, err = repo.GetByID(tenantCtx, tenantID, originalID)
	if err != nil || original == nil || original.Status != journal.StatusPosted {
		t.Fatalf("reload original after rejected mirrorless reversal = (%+v, %v)", original, err)
	}

	reversedAt := postedAt.Add(time.Hour)
	seedReversal, err := original.ReverseAt("concurrent bank return", "controller-a", reversedAt)
	if err != nil {
		t.Fatalf("build seed journal reversal: %v", err)
	}
	seedReversal.ID = seedReversalID
	seedReversal.SourceEventID = "journal.reversal:" + uuid.NewString()
	reversalA, err := original.ReverseAt("concurrent bank return", "controller-a", reversedAt)
	if err != nil {
		t.Fatalf("build first journal reversal: %v", err)
	}
	reversalA.ID = reversalAID
	reversalA.SourceEventID = "journal.reversal:" + uuid.NewString()
	reversalB, err := original.ReverseAt("concurrent bank return", "controller-a", reversedAt)
	if err != nil {
		t.Fatalf("build second journal reversal: %v", err)
	}
	reversalB.ID = reversalBID
	reversalB.SourceEventID = "journal.reversal:" + uuid.NewString()

	expectedUpdatedAt := original.UpdatedAt
	if err := original.MarkReversed(); err != nil {
		t.Fatalf("mark original journal reversed: %v", err)
	}
	if err := db.RunInTx(tenantCtx, func(tx *sqlx.Tx) error {
		txCtx := database.WithTx(tenantCtx, tx)
		updated, err := repo.UpdateStatus(txCtx, original, expectedUpdatedAt)
		if err != nil {
			return err
		}
		if !updated {
			return errors.New("original journal status was not updated")
		}
		return repo.Create(txCtx, seedReversal)
	}); err != nil {
		t.Fatalf("persist original journal with seed reversal: %v", err)
	}
	if rows, err := deleteManagedBillJournalFixturesInTest(
		tenantCtx,
		db,
		"SELECT id::text FROM financial.journal_entries WHERE tenant_id = $1::uuid AND id = $2::uuid",
		tenantID,
		seedReversalID,
	); err != nil || rows != 1 {
		t.Fatalf("remove seed journal reversal = (%d, %v), want (1, nil)", rows, err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, candidate := range []*journal.Entry{reversalA, reversalB} {
		candidate := candidate
		go func() {
			<-start
			results <- repo.Create(tenantCtx, candidate)
		}()
	}
	close(start)

	succeeded, rejected := 0, 0
	for range 2 {
		createErr := <-results
		if createErr == nil {
			succeeded++
			continue
		}
		if !strings.Contains(createErr.Error(), "uq_finance_journal_entries_reversal_of") {
			t.Fatalf("duplicate reversal failed for the wrong reason: %v", createErr)
		}
		rejected++
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent reversal outcomes = success:%d rejected:%d, want 1/1", succeeded, rejected)
	}

	var reversalRows int
	if err := db.QueryRow(tenantCtx, `SELECT COUNT(*)
		FROM financial.journal_entries
		WHERE tenant_id = $1::uuid
		  AND deleted_at IS NULL
		  AND metadata ->> 'reversal_of' = $2`, tenantID, originalID).Scan(&reversalRows); err != nil {
		t.Fatalf("count canonical journal reversals: %v", err)
	}
	if reversalRows != 1 {
		t.Fatalf("journal reversals persisted = %d, want exactly one", reversalRows)
	}
}

func deleteManagedBillJournalFixturesInTest(
	ctx context.Context,
	db *database.PostgresDB,
	targetQuery string,
	args ...any,
) (int64, error) {
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

func newPostgresManagedBillFixture(
	t *testing.T,
	tenantID, transferID, chargeID, chargeItemID, sourceKey string,
	reservedAt time.Time,
) *managed_bill.ManagedBill {
	t.Helper()
	bill, err := managed_bill.NewReserved(managed_bill.NewReservedInput{
		TenantID:           tenantID,
		SourceKey:          sourceKey,
		SourceTransferID:   transferID,
		SourceChargeID:     chargeID,
		SourceChargeItemID: chargeItemID,
		ContractID:         "contract-" + uuid.NewString(),
		PropertyID:         "property-" + uuid.NewString(),
		OwnerID:            "owner-" + uuid.NewString(),
		Component:          "CONDOMINIO",
		Creditor:           "Condominio Teste",
		Description:        "Conta administrada de teste",
		AmountCents:        70_000,
		DueDate:            reservedAt.AddDate(0, 0, 10),
		CompetenceMonth:    "2026-08",
		ReservedBy:         "operator-test",
		ReservedAt:         reservedAt,
	})
	if err != nil {
		t.Fatalf("build managed bill fixture: %v", err)
	}
	return bill
}
