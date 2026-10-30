package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/managed_bill"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
)

func TestManagedBillPaymentReversalPostgresAtomicLifecycle(t *testing.T) {
	db := openFinancialConsistencyPostgres(t)
	tenantID := uuid.NewString()
	foreignTenantID := uuid.NewString()
	ctx, cancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 30*time.Second)
	defer cancel()

	successIDs := newManagedBillPaymentReversalPostgresIDs()
	rollbackIDs := newManagedBillPaymentReversalPostgresIDs()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 10*time.Second)
		defer cleanupCancel()

		if _, err := deleteJournalFixturesInTest(cleanupCtx, db, `SELECT id::text FROM financial.journal_entries
			WHERE tenant_id = $1::uuid
			  AND EXISTS (
				SELECT 1
				FROM jsonb_array_elements(COALESCE(metadata->'legs', '[]'::jsonb)) AS leg
				WHERE leg->>'managed_bill_id' IN ($2, $3)
				  )`, tenantID, successIDs.billID, rollbackIDs.billID); err != nil {
			t.Errorf("cleanup managed-bill reversal journals: %v", err)
		}
		if _, err := db.Exec(cleanupCtx, `DELETE FROM financial.managed_bills
			WHERE tenant_id = $1::uuid AND id IN ($2::uuid, $3::uuid)`,
			tenantID, successIDs.billID, rollbackIDs.billID); err != nil {
			t.Errorf("cleanup managed-bill reversal bills: %v", err)
		}
		if _, err := db.Exec(cleanupCtx, `DELETE FROM financial.transfers
			WHERE tenant_id = $1::uuid AND id IN ($2::uuid, $3::uuid)`,
			tenantID, successIDs.transferID, rollbackIDs.transferID); err != nil {
			t.Errorf("cleanup managed-bill reversal transfers: %v", err)
		}
		if _, err := db.Exec(cleanupCtx, `DELETE FROM financial.charges
			WHERE tenant_id = $1::uuid AND id IN ($2::uuid, $3::uuid)`,
			tenantID, successIDs.chargeID, rollbackIDs.chargeID); err != nil {
			t.Errorf("cleanup managed-bill reversal charges: %v", err)
		}

		var residual int
		if err := db.QueryRow(cleanupCtx, `SELECT
			(SELECT COUNT(*) FROM financial.journal_entries
			 WHERE tenant_id = $1::uuid
			   AND EXISTS (
				SELECT 1
				FROM jsonb_array_elements(COALESCE(metadata->'legs', '[]'::jsonb)) AS leg
				WHERE leg->>'managed_bill_id' IN ($2, $3)
			   ))
			+ (SELECT COUNT(*) FROM financial.managed_bills
			   WHERE tenant_id = $1::uuid AND id IN ($4::uuid, $5::uuid))
			+ (SELECT COUNT(*) FROM financial.transfers
			   WHERE tenant_id = $1::uuid AND id IN ($6::uuid, $7::uuid))
			+ (SELECT COUNT(*) FROM financial.charges
			   WHERE tenant_id = $1::uuid AND id IN ($8::uuid, $9::uuid))`,
			tenantID,
			successIDs.billID, rollbackIDs.billID,
			successIDs.billID, rollbackIDs.billID,
			successIDs.transferID, rollbackIDs.transferID,
			successIDs.chargeID, rollbackIDs.chargeID,
		).Scan(&residual); err != nil {
			t.Errorf("verify managed-bill reversal cleanup: %v", err)
		} else if residual != 0 {
			t.Errorf("managed-bill reversal cleanup left %d rows for the exact fixture IDs", residual)
		}
	})

	chargeRepo := repositories.NewChargePostgresRepository(db)
	transferRepo := repositories.NewTransferPostgresRepository(db)
	billRepo := repositories.NewManagedBillPostgresRepository(db)
	journalRepo := repositories.NewJournalPostgresRepository(db)
	journalService := NewJournalService(journalRepo).WithDB(db)
	service := NewManagedBillService(billRepo, transferRepo).
		WithDB(db).
		WithJournalService(journalService)

	successBill := persistManagedBillPaymentReversalPostgresFixture(
		t, ctx, tenantID, successIDs, chargeRepo, transferRepo, billRepo,
	)
	rollbackBill := persistManagedBillPaymentReversalPostgresFixture(
		t, ctx, tenantID, rollbackIDs, chargeRepo, transferRepo, billRepo,
	)

	approved, err := service.Approve(ctx, tenantID, successBill.ID, "approver-success", successBill.Version)
	if err != nil {
		t.Fatalf("approve successful reversal fixture: %v", err)
	}
	paid, err := service.Pay(
		ctx, tenantID, successBill.ID, "payer-success", approved.Bill.Version,
		"pix-success-original", "https://proof.example/payment-success", time.Time{},
	)
	if err != nil {
		t.Fatalf("pay successful reversal fixture: %v", err)
	}
	assertManagedBillBalance(t, paid.BalanceBefore, 560_000, 100_000, 0, 460_000)
	assertManagedBillBalance(t, paid.BalanceAfter, 490_000, 30_000, 0, 460_000)

	reconciled, err := service.Reconcile(
		ctx, tenantID, successBill.ID, "reconciler-success", paid.Bill.Version,
		"statement-line-success", time.Time{},
	)
	if err != nil {
		t.Fatalf("reconcile successful reversal fixture: %v", err)
	}
	original, err := journalService.GetBySourceEventID(ctx, tenantID, paid.Bill.PaymentJournalSourceID)
	if err != nil || original == nil {
		t.Fatalf("load original managed-bill payment journal = (%+v, %v)", original, err)
	}
	if original.Status != journal.StatusPosted || original.ReconciliationDate == nil {
		t.Fatalf("original payment journal before reversal = %+v", original)
	}

	reversed, err := service.ReversePayment(
		ctx, tenantID, successBill.ID, "controller-success", reconciled.Bill.Version,
		"bank-return-success", "beneficiary account rejected the payment",
		"https://proof.example/reversal-success", time.Time{},
	)
	if err != nil {
		t.Fatalf("reverse reconciled managed-bill payment: %v", err)
	}
	assertManagedBillBalance(t, reversed.BalanceBefore, 490_000, 30_000, 0, 460_000)
	assertManagedBillBalance(t, reversed.BalanceAfter, 560_000, 100_000, 0, 460_000)
	if reversed.Bill.Status != managed_bill.StatusApproved || reversed.Bill.Version != reconciled.Bill.Version+1 ||
		reversed.Bill.PaidAt != nil || reversed.Bill.PaidBy != "" || reversed.Bill.PaymentReference != "" ||
		reversed.Bill.PaymentJournalSourceID != "" || reversed.Bill.ReconciledAt != nil ||
		reversed.Bill.ReconciledBy != "" || reversed.Bill.ReconciliationReference != "" {
		t.Fatalf("reversed managed bill did not reopen cleanly: %+v", reversed.Bill)
	}
	if len(reversed.Bill.PaymentAttempts) != 1 {
		t.Fatalf("archived payment attempts = %+v, want one", reversed.Bill.PaymentAttempts)
	}
	attempt := reversed.Bill.PaymentAttempts[0]
	if attempt.ID != paid.Bill.PaymentJournalSourceID ||
		attempt.PaymentReference != "pix-success-original" ||
		attempt.PaymentJournalSourceID != paid.Bill.PaymentJournalSourceID ||
		attempt.PaidBy != "payer-success" || attempt.ReconciledBy != "reconciler-success" ||
		attempt.ReconciledAt == nil || attempt.ReconciliationReference != "statement-line-success" ||
		attempt.ReversalReference != "bank-return-success" || attempt.ReversalJournalSourceID == "" ||
		attempt.ReversalProofReference != "https://proof.example/reversal-success" ||
		attempt.ReversedBy != "controller-success" ||
		attempt.ReversalReason != "beneficiary account rejected the payment" {
		t.Fatalf("archived payment attempt lost provenance: %+v", attempt)
	}

	persistedTransfer, err := transferRepo.GetByID(ctx, tenantID, successIDs.transferID)
	if err != nil {
		t.Fatalf("reload transfer after reversal: %v", err)
	}
	if persistedTransfer.OwnerBalance == nil || persistedTransfer.OwnerBalance.TotalCents != 560_000 ||
		persistedTransfer.OwnerBalance.ReservedCents != 100_000 ||
		persistedTransfer.OwnerBalance.AvailableCents() != 460_000 ||
		len(persistedTransfer.OwnerBalanceMovements) != 2 {
		t.Fatalf("owner balance was not restored exactly once: %+v", persistedTransfer)
	}

	reloadedOriginal, err := journalService.GetBySourceEventID(ctx, tenantID, attempt.PaymentJournalSourceID)
	if err != nil || reloadedOriginal == nil {
		t.Fatalf("reload reversed original journal = (%+v, %v)", reloadedOriginal, err)
	}
	if reloadedOriginal.Status != journal.StatusReversed || reloadedOriginal.ReconciliationDate == nil {
		t.Fatalf("original payment journal was not marked REVERSED: %+v", reloadedOriginal)
	}
	mirror, err := journalService.GetBySourceEventID(ctx, tenantID, attempt.ReversalJournalSourceID)
	if err != nil || mirror == nil {
		t.Fatalf("load mirror reversal journal = (%+v, %v)", mirror, err)
	}
	assertManagedBillPaymentMirror(t, reloadedOriginal, mirror, successBill.ID, "controller-success", attempt.ReversedAt)
	assertManagedBillJournalRows(t, ctx, db, tenantID, successBill.ID, 2)

	// The exact retry carries the pre-reversal version and must return the
	// canonical APPROVED aggregate without another movement, attempt or journal.
	retried, err := service.ReversePayment(
		ctx, tenantID, successBill.ID, "controller-success", reconciled.Bill.Version,
		"bank-return-success", "beneficiary account rejected the payment",
		"https://proof.example/reversal-success", time.Time{},
	)
	if err != nil {
		t.Fatalf("retry exact payment reversal: %v", err)
	}
	if retried.Bill.Version != reversed.Bill.Version || len(retried.Bill.PaymentAttempts) != 1 {
		t.Fatalf("exact reversal retry mutated bill: %+v", retried.Bill)
	}
	persistedTransfer, err = transferRepo.GetByID(ctx, tenantID, successIDs.transferID)
	if err != nil || len(persistedTransfer.OwnerBalanceMovements) != 2 {
		t.Fatalf("exact reversal retry duplicated movement: transfer=%+v err=%v", persistedTransfer, err)
	}
	assertManagedBillJournalRows(t, ctx, db, tenantID, successBill.ID, 2)

	repaid, err := service.Pay(
		ctx, tenantID, successBill.ID, "payer-success-2", reversed.Bill.Version,
		"pix-success-retry", "https://proof.example/payment-success-2", time.Time{},
	)
	if err != nil {
		t.Fatalf("pay managed bill with a new reference after reversal: %v", err)
	}
	if repaid.Bill.Status != managed_bill.StatusPaid || repaid.Bill.PaymentReference != "pix-success-retry" ||
		repaid.Bill.PaymentJournalSourceID == attempt.PaymentJournalSourceID ||
		len(repaid.Bill.PaymentAttempts) != 1 {
		t.Fatalf("new payment attempt was not activated: %+v", repaid.Bill)
	}
	assertManagedBillBalance(t, repaid.BalanceBefore, 560_000, 100_000, 0, 460_000)
	assertManagedBillBalance(t, repaid.BalanceAfter, 490_000, 30_000, 0, 460_000)
	assertManagedBillJournalRows(t, ctx, db, tenantID, successBill.ID, 3)

	foreignCtx := pkgcontext.WithTenantID(context.Background(), foreignTenantID)
	if _, err := service.ReversePayment(
		foreignCtx, foreignTenantID, successBill.ID, "foreign-controller", repaid.Bill.Version,
		"foreign-return", "cross-tenant probe", "", time.Time{},
	); !errors.Is(err, managed_bill.ErrNotFound) {
		t.Fatalf("cross-tenant reversal error = %v, want ErrNotFound", err)
	}

	rollbackApproved, err := service.Approve(ctx, tenantID, rollbackBill.ID, "approver-rollback", rollbackBill.Version)
	if err != nil {
		t.Fatalf("approve rollback fixture: %v", err)
	}
	rollbackPaid, err := service.Pay(
		ctx, tenantID, rollbackBill.ID, "payer-rollback", rollbackApproved.Bill.Version,
		"pix-rollback-original", "", time.Time{},
	)
	if err != nil {
		t.Fatalf("pay rollback fixture: %v", err)
	}
	rollbackOriginal, err := journalService.GetBySourceEventID(ctx, tenantID, rollbackPaid.Bill.PaymentJournalSourceID)
	if err != nil || rollbackOriginal == nil {
		t.Fatalf("load rollback fixture payment journal = (%+v, %v)", rollbackOriginal, err)
	}
	rollbackMirrorSourceID := managedBillPaymentReversalJournalSourceID(tenantID, rollbackOriginal.ID)
	wantRollback := errors.New("force managed-bill reversal projection rollback")
	failingRepo := &failingManagedBillPaymentReversalRepository{
		Repository: billRepo,
		targetID:   rollbackBill.ID,
		err:        wantRollback,
	}
	failingService := NewManagedBillService(failingRepo, transferRepo).
		WithDB(db).
		WithJournalService(journalService)
	if _, err := failingService.ReversePayment(
		ctx, tenantID, rollbackBill.ID, "controller-rollback", rollbackPaid.Bill.Version,
		"bank-return-rollback", "forced rollback after journal and transfer writes", "", time.Time{},
	); !errors.Is(err, wantRollback) {
		t.Fatalf("forced reversal failure = %v, want rollback sentinel", err)
	}

	rolledBackBill, err := billRepo.GetByID(ctx, tenantID, rollbackBill.ID)
	if err != nil {
		t.Fatalf("reload rolled-back managed bill: %v", err)
	}
	if rolledBackBill.Status != managed_bill.StatusPaid || rolledBackBill.Version != rollbackPaid.Bill.Version ||
		rolledBackBill.PaymentReference != "pix-rollback-original" ||
		rolledBackBill.PaymentJournalSourceID != rollbackPaid.Bill.PaymentJournalSourceID ||
		len(rolledBackBill.PaymentAttempts) != 0 {
		t.Fatalf("managed-bill status escaped failed reversal: %+v", rolledBackBill)
	}
	rolledBackTransfer, err := transferRepo.GetByID(ctx, tenantID, rollbackIDs.transferID)
	if err != nil {
		t.Fatalf("reload transfer after failed reversal: %v", err)
	}
	if rolledBackTransfer.OwnerBalance == nil || rolledBackTransfer.OwnerBalance.TotalCents != 490_000 ||
		rolledBackTransfer.OwnerBalance.ReservedCents != 30_000 ||
		rolledBackTransfer.OwnerBalance.AvailableCents() != 460_000 ||
		len(rolledBackTransfer.OwnerBalanceMovements) != 1 {
		t.Fatalf("owner-balance movement escaped failed reversal: %+v", rolledBackTransfer)
	}
	rolledBackOriginal, err := journalService.GetBySourceEventID(ctx, tenantID, rollbackPaid.Bill.PaymentJournalSourceID)
	if err != nil || rolledBackOriginal == nil || rolledBackOriginal.Status != journal.StatusPosted {
		t.Fatalf("original journal status escaped failed reversal: entry=%+v err=%v", rolledBackOriginal, err)
	}
	rolledBackMirror, err := journalService.GetBySourceEventID(ctx, tenantID, rollbackMirrorSourceID)
	if err != nil || rolledBackMirror != nil {
		t.Fatalf("mirror journal escaped failed reversal: entry=%+v err=%v", rolledBackMirror, err)
	}
	assertManagedBillJournalRows(t, ctx, db, tenantID, rollbackBill.ID, 1)

	assertManagedBillPaymentReversalFixtureRows(
		t, ctx, db, tenantID, successIDs, rollbackIDs, 4,
	)
}

type managedBillPaymentReversalPostgresIDs struct {
	chargeID      string
	transferID    string
	billID        string
	contractID    string
	tenantPartyID string
	ownerID       string
	propertyID    string
	agencyID      string
	paymentID     string
}

func newManagedBillPaymentReversalPostgresIDs() managedBillPaymentReversalPostgresIDs {
	return managedBillPaymentReversalPostgresIDs{
		chargeID:      uuid.NewString(),
		transferID:    uuid.NewString(),
		billID:        uuid.NewString(),
		contractID:    uuid.NewString(),
		tenantPartyID: uuid.NewString(),
		ownerID:       uuid.NewString(),
		propertyID:    uuid.NewString(),
		agencyID:      uuid.NewString(),
		paymentID:     uuid.NewString(),
	}
}

func persistManagedBillPaymentReversalPostgresFixture(
	t *testing.T,
	ctx context.Context,
	tenantID string,
	ids managedBillPaymentReversalPostgresIDs,
	chargeRepo charge.RepositoryInterface,
	transferRepo transfer.RepositoryInterface,
	billRepo managed_bill.Repository,
) *managed_bill.ManagedBill {
	t.Helper()
	dueDate := time.Now().UTC().AddDate(0, 0, 5)
	receivable, err := charge.NewWithLineItems(
		ids.contractID, ids.tenantPartyID, ids.ownerID, ids.propertyID, tenantID,
		charge.TypeRent, "Managed-bill payment reversal PostgreSQL fixture", "2026-08", 6_000, dueDate,
		[]charge.LineItem{
			{Description: "Aluguel", AmountCents: 500_000, SourceType: "postgres_fixture", SourceID: uuid.NewString(), SettlementMode: charge.SettlementModeCollection},
			{Description: "Condominio", AmountCents: 70_000, SourceType: "postgres_fixture", SourceID: uuid.NewString(), SettlementMode: charge.SettlementModeDirectLiquidation},
			{Description: "IPTU", AmountCents: 30_000, SourceType: "postgres_fixture", SourceID: uuid.NewString(), SettlementMode: charge.SettlementModeDirectLiquidation},
		},
	)
	if err != nil {
		t.Fatalf("build allocation-aware charge fixture: %v", err)
	}
	receivable.ID = ids.chargeID
	if err := chargeRepo.Create(ctx, receivable); err != nil {
		t.Fatalf("persist charge fixture: %v", err)
	}

	allocationSpecs := []struct {
		itemIndex   int
		beneficiary string
		kind        charge.AllocationType
		amountCents int64
	}{
		{itemIndex: 0, beneficiary: ids.agencyID, kind: charge.AllocationTypeRevenue, amountCents: 40_000},
		{itemIndex: 0, beneficiary: ids.ownerID, kind: charge.AllocationTypeOwnerPayable, amountCents: 460_000},
		{itemIndex: 1, beneficiary: ids.ownerID, kind: charge.AllocationTypeOwnerPayable, amountCents: 70_000},
		{itemIndex: 2, beneficiary: ids.ownerID, kind: charge.AllocationTypeOwnerPayable, amountCents: 30_000},
	}
	allocations := make([]charge.Allocation, 0, len(allocationSpecs))
	for _, spec := range allocationSpecs {
		item := receivable.LineItems[spec.itemIndex]
		allocation, err := charge.NewAllocation(
			ids.paymentID, item.ID, spec.beneficiary, spec.kind, spec.amountCents, item.RuleVersionID,
		)
		if err != nil {
			t.Fatalf("build charge payment allocation: %v", err)
		}
		allocations = append(allocations, *allocation)
	}
	if changed, err := receivable.ApplyPaymentWithAllocations(
		ids.paymentID, 6_000, "PIX", time.Now().UTC(), charge.AllocationMethodProportionalV1, allocations,
	); err != nil || !changed {
		t.Fatalf("apply allocation-aware charge payment: changed=%v err=%v", changed, err)
	}
	if err := chargeRepo.Update(ctx, receivable); err != nil {
		t.Fatalf("persist allocation-aware charge payment: %v", err)
	}

	ownerTransfer := transfer.New(
		ids.contractID, ids.ownerID, ids.propertyID, tenantID, "2026-08",
		6_000, 0, 0, 0, nil, "allocation-aware managed-bill reversal fixture",
	)
	ownerTransfer.ID = ids.transferID
	ownerTransfer.SourceChargeID = ids.chargeID
	ownerBalance, err := transfer.NewOwnerBalance(560_000, 100_000, 0)
	if err != nil {
		t.Fatalf("build allocation-aware owner balance: %v", err)
	}
	if err := ownerTransfer.SetAllocatedOwnerBalance(ownerBalance, 40_000); err != nil {
		t.Fatalf("set allocation-aware owner balance: %v", err)
	}
	if err := transferRepo.Create(ctx, ownerTransfer); err != nil {
		t.Fatalf("persist allocation-aware transfer: %v", err)
	}

	bill, err := managed_bill.NewReserved(managed_bill.NewReservedInput{
		TenantID:           tenantID,
		SourceKey:          "managed-bill-payment-reversal:" + ids.billID,
		SourceTransferID:   ids.transferID,
		SourceChargeID:     ids.chargeID,
		SourceChargeItemID: receivable.LineItems[1].ID,
		ContractID:         ids.contractID,
		PropertyID:         ids.propertyID,
		OwnerID:            ids.ownerID,
		Component:          "CONDOMINIO",
		Creditor:           "Condominio PostgreSQL",
		Description:        "Condominio administrado para teste de estorno",
		AmountCents:        70_000,
		DueDate:            dueDate,
		CompetenceMonth:    "2026-08",
		ReservedBy:         "system:postgres-fixture",
		ReservedAt:         time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("build managed-bill fixture: %v", err)
	}
	bill.ID = ids.billID
	if err := billRepo.Create(ctx, bill); err != nil {
		t.Fatalf("persist managed-bill fixture: %v", err)
	}
	return bill
}

type failingManagedBillPaymentReversalRepository struct {
	managed_bill.Repository
	targetID string
	err      error
}

func (r *failingManagedBillPaymentReversalRepository) UpdateIfVersion(
	ctx context.Context,
	bill *managed_bill.ManagedBill,
	expectedVersion int64,
) (bool, error) {
	if bill != nil && bill.ID == r.targetID && bill.Status == managed_bill.StatusApproved && len(bill.PaymentAttempts) > 0 {
		return false, r.err
	}
	return r.Repository.UpdateIfVersion(ctx, bill, expectedVersion)
}

func managedBillPaymentReversalJournalSourceID(tenantID, originalJournalID string) string {
	return events.DeriveNotificationID("journal.reversal", tenantID, originalJournalID)
}

func assertManagedBillPaymentMirror(
	t *testing.T,
	original, mirror *journal.Entry,
	managedBillID, reversedBy string,
	reversedAt time.Time,
) {
	t.Helper()
	if original == nil || mirror == nil || mirror.Status != journal.StatusPosted || mirror.ReversalOf != original.ID ||
		mirror.ReversedBy != reversedBy || !mirror.Date.Equal(reversedAt) ||
		mirror.OperationID != original.OperationID || len(mirror.Legs) != len(original.Legs) {
		t.Fatalf("invalid managed-bill payment mirror: original=%+v mirror=%+v", original, mirror)
	}
	for i := range original.Legs {
		originalLeg := original.Legs[i]
		mirrorLeg := mirror.Legs[i]
		wantDirection := journal.Debit
		if originalLeg.Direction == journal.Debit {
			wantDirection = journal.Credit
		}
		if mirrorLeg.Account != originalLeg.Account || mirrorLeg.Direction != wantDirection ||
			mirrorLeg.AmountCents != originalLeg.AmountCents || mirrorLeg.ManagedBillID != managedBillID {
			t.Fatalf("mirror leg %d does not compensate original: original=%+v mirror=%+v", i, originalLeg, mirrorLeg)
		}
	}
}

func assertManagedBillJournalRows(
	t *testing.T,
	ctx context.Context,
	db *database.PostgresDB,
	tenantID, billID string,
	want int,
) {
	t.Helper()
	var got int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM financial.journal_entries
		WHERE tenant_id = $1::uuid
		  AND EXISTS (
			SELECT 1
			FROM jsonb_array_elements(COALESCE(metadata->'legs', '[]'::jsonb)) AS leg
			WHERE leg->>'managed_bill_id' = $2
		  )`, tenantID, billID).Scan(&got); err != nil {
		t.Fatalf("count managed-bill journal rows: %v", err)
	}
	if got != want {
		t.Fatalf("managed-bill journal rows = %d, want %d", got, want)
	}
}

func assertManagedBillPaymentReversalFixtureRows(
	t *testing.T,
	ctx context.Context,
	db *database.PostgresDB,
	tenantID string,
	successIDs, rollbackIDs managedBillPaymentReversalPostgresIDs,
	wantJournals int,
) {
	t.Helper()
	var charges, transfers, bills, journals int
	if err := db.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM financial.charges
		 WHERE tenant_id = $1::uuid AND id IN ($2::uuid, $3::uuid)),
		(SELECT COUNT(*) FROM financial.transfers
		 WHERE tenant_id = $1::uuid AND id IN ($4::uuid, $5::uuid)),
		(SELECT COUNT(*) FROM financial.managed_bills
		 WHERE tenant_id = $1::uuid AND id IN ($6::uuid, $7::uuid)),
		(SELECT COUNT(*) FROM financial.journal_entries
		 WHERE tenant_id = $1::uuid
		   AND EXISTS (
			 SELECT 1
			 FROM jsonb_array_elements(COALESCE(metadata->'legs', '[]'::jsonb)) AS leg
			 WHERE leg->>'managed_bill_id' IN ($6::text, $7::text)
		   ))`,
		tenantID,
		successIDs.chargeID, rollbackIDs.chargeID,
		successIDs.transferID, rollbackIDs.transferID,
		successIDs.billID, rollbackIDs.billID,
	).Scan(&charges, &transfers, &bills, &journals); err != nil {
		t.Fatalf("count exact managed-bill reversal fixture rows: %v", err)
	}
	if charges != 2 || transfers != 2 || bills != 2 || journals != wantJournals {
		t.Fatalf("exact fixture rows charges/transfers/bills/journals = %d/%d/%d/%d, want 2/2/2/%d",
			charges, transfers, bills, journals, wantJournals)
	}
}
