package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildManualReceiptBreakdownRentAppliesAdminFee(t *testing.T) {
	items := []BatchCreateInput{{
		ContractID:  "contract-rent",
		DealID:      "deal-rent",
		Type:        "receita",
		Description: "Recebimento - Locação",
		Value:       100_000,
		Flow:        "rent",
	}}
	commissions := []ManualCommissionInput{{
		DealID: "deal-rent", BrokerID: "broker-1", Role: "captador", Percentage: 60, Amount: 6_000,
	}}
	retention := &ManualCommissionRetentionInput{Percentage: 40, Amount: 4_000}

	breakdown, dealID, err := buildManualReceiptBreakdown(items, commissions, retention, 10)
	require.NoError(t, err)
	require.Equal(t, "deal-rent", dealID)
	require.NotNil(t, breakdown)
	assert.Equal(t, "rent", breakdown.Flow)
	assert.Equal(t, int64(10_000_000), breakdown.GrossCents)
	assert.Equal(t, int64(1_000_000), breakdown.CommissionPoolCents)
	assert.Equal(t, int64(600_000), breakdown.CommissionPayableCents)
	assert.Equal(t, int64(400_000), breakdown.AgencyRetainedCents)
	assert.Equal(t, int64(1_000_000), breakdown.AdminFeeCents)
	assert.Equal(t, int64(8_000_000), breakdown.OwnerPayableCents)
}

func TestBuildManualReceiptBreakdownRentNoCommission(t *testing.T) {
	items := []BatchCreateInput{{
		ContractID:  "contract-rent",
		DealID:      "deal-rent",
		Type:        "receita",
		Description: "Recebimento - Locação",
		Value:       100_000,
		Flow:        "rent",
	}}

	breakdown, dealID, err := buildManualReceiptBreakdown(items, nil, nil, 10)
	require.NoError(t, err)
	require.Equal(t, "deal-rent", dealID)
	require.NotNil(t, breakdown)
	assert.Equal(t, "rent", breakdown.Flow)
	assert.Equal(t, int64(10_000_000), breakdown.GrossCents)
	assert.Equal(t, int64(0), breakdown.CommissionPoolCents)
	assert.Equal(t, int64(0), breakdown.CommissionPayableCents)
	assert.Equal(t, int64(0), breakdown.AgencyRetainedCents)
	assert.Equal(t, int64(1_000_000), breakdown.AdminFeeCents)
	assert.Equal(t, int64(9_000_000), breakdown.OwnerPayableCents)
}

func TestValidateManualReceiptBreakdownRentClosure(t *testing.T) {
	valid := &transaction.Transaction{
		ID:     "receipt-rent",
		DealID: "deal-rent",
		EconomicBreakdown: &transaction.EconomicBreakdown{
			Flow:                   "rent",
			GrossCents:             10_000_000,
			CommissionPoolCents:    1_000_000,
			CommissionPayableCents: 600_000,
			AgencyRetainedCents:    400_000,
			AdminFeeCents:          1_000_000,
			OwnerPayableCents:      8_000_000,
		},
	}
	require.NoError(t, validateManualReceiptBreakdown(valid))

	// Owner split that keeps the admin fee but forgets to deduct it must not close.
	invalid := *valid
	invalid.EconomicBreakdown = &transaction.EconomicBreakdown{
		Flow:                   "rent",
		GrossCents:             10_000_000,
		CommissionPoolCents:    1_000_000,
		CommissionPayableCents: 600_000,
		AgencyRetainedCents:    400_000,
		AdminFeeCents:          1_000_000,
		OwnerPayableCents:      9_000_000, // admin fee double-counted into owner
	}
	require.Error(t, validateManualReceiptBreakdown(&invalid))
}

func TestNewManualRentReceiptJournalEntryClosesEconomicOwnership(t *testing.T) {
	paidAt := time.Date(2026, time.August, 29, 12, 34, 56, 789123456, time.UTC)
	receipt := &transaction.Transaction{
		ID:          "receipt-rent",
		TenantID:    "tenant-1",
		ContractID:  "contract-rent",
		DealID:      "deal-rent",
		Date:        paidAt.Add(-24 * time.Hour),
		PaymentDate: &paidAt,
		EconomicBreakdown: &transaction.EconomicBreakdown{
			Flow:                   "rent",
			GrossCents:             10_000_000,
			CommissionPoolCents:    1_000_000,
			CommissionPayableCents: 0,
			AgencyRetainedCents:    1_000_000, // intermediation funding
			AdminFeeCents:          1_000_000,
			OwnerPayableCents:      8_000_000,
		},
	}

	entry, err := newManualReceiptJournalEntry(receipt, "owner-1")
	require.NoError(t, err)
	require.True(t, entry.Balanced())
	assert.Equal(t, receipt.ID, entry.OperationID)

	assertJournalLeg(t, entry, journal.AccountCash, journal.Debit, 10_000_000, "")
	assertJournalLeg(t, entry, journal.AccountOwnerPayable, journal.Credit, 8_000_000, "owner-1")
	// GROSS revenue = administration fee (1_000_000) + intermediation funding (1_000_000).
	assertJournalLeg(t, entry, journal.AccountRevenue, journal.Credit, 2_000_000, "tenant-1")
}

func TestBatchCreatePostingPersistsDueDateAndPaymentPlan(t *testing.T) {
	txRepo := newStoringTxRepo()
	svc := NewTransactionService(txRepo)
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})

	items := []BatchCreateInput{{
		ContractID:  "contract-rent",
		DealID:      "deal-rent",
		Type:        "receita",
		Description: "Recebimento - Locação",
		Value:       100_000,
		DateStr:     "2026-08-29",
		Flow:        "rent",
		DueDateStr:  "2026-09-07",
		PaymentPlan: "Mensal (recorrente)",
	}}

	outputs, err := svc.BatchCreatePosting(ctx, "tenant-rent", "plan-provenance", items, nil, nil)
	if err != nil {
		t.Fatalf("post manual rent receipt with plan provenance: %v", err)
	}
	if len(outputs) != 1 || outputs[0].Transaction == nil {
		t.Fatalf("unexpected outputs: %#v", outputs)
	}
	tx := outputs[0].Transaction
	if tx.DueDate == nil || tx.DueDate.UTC().Format("2006-01-02") != "2026-09-07" {
		t.Fatalf("due date = %v, want 2026-09-07", tx.DueDate)
	}
	if tx.PaymentPlan != "Mensal (recorrente)" {
		t.Fatalf("payment plan = %q, want Mensal (recorrente)", tx.PaymentPlan)
	}
	if txRepo.byID[tx.ID] == nil || txRepo.byID[tx.ID].PaymentPlan != "Mensal (recorrente)" {
		t.Fatal("persisted transaction did not carry the payment plan")
	}
}

func planProvenanceItem(dueDate, plan string) []BatchCreateInput {
	return []BatchCreateInput{{
		ContractID:  "contract-rent",
		DealID:      "deal-rent",
		Type:        "receita",
		Description: "Recebimento - Locação",
		Value:       100_000,
		DateStr:     "2026-08-29",
		Flow:        "rent",
		DueDateStr:  dueDate,
		PaymentPlan: plan,
	}}
}

func TestBatchCreatePostingRejectsInvalidDueDate(t *testing.T) {
	svc := NewTransactionService(newStoringTxRepo())
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})

	_, err := svc.BatchCreatePosting(ctx, "tenant-rent", "invalid-due", planProvenanceItem("not-a-date", "Mensal"), nil, nil)
	if !errors.Is(err, transaction.ErrBatchValidation) || !strings.Contains(err.Error(), "invalid due_date") {
		t.Fatalf("invalid due date must fail batch validation, got %v", err)
	}
}

func TestBatchCreatePostingRejectsOversizedPaymentPlan(t *testing.T) {
	svc := NewTransactionService(newStoringTxRepo())
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})

	oversized := strings.Repeat("x", 513)
	_, err := svc.BatchCreatePosting(ctx, "tenant-rent", "oversized-plan", planProvenanceItem("2026-09-07", oversized), nil, nil)
	if !errors.Is(err, transaction.ErrBatchValidation) || !strings.Contains(err.Error(), "payment_plan") {
		t.Fatalf("oversized payment plan must fail batch validation, got %v", err)
	}
}

func TestBatchCreatePostingIdempotencyConflictOnDifferentPlan(t *testing.T) {
	svc := NewTransactionService(newStoringTxRepo())
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})

	if _, err := svc.BatchCreatePosting(ctx, "tenant-rent", "same-plan-key", planProvenanceItem("2026-09-07", "Entrada + 11x"), nil, nil); err != nil {
		t.Fatalf("first posting: %v", err)
	}
	_, err := svc.BatchCreatePosting(ctx, "tenant-rent", "same-plan-key", planProvenanceItem("2026-09-07", "Entrada + 24x"), nil, nil)
	if !errors.Is(err, transaction.ErrBatchIdempotencyConflict) {
		t.Fatalf("retry with a different payment plan must conflict, got %v", err)
	}
}

func TestManualPostingFingerprintProvenanceConditional(t *testing.T) {
	due := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	makeTx := func(withDue bool, plan string) *transaction.Transaction {
		tx := transaction.New("contract-1", transaction.TypeReceita, "Recebimento", 100, time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC), "tenant-1")
		tx.ID = "tx-1"
		tx.PaymentPlan = plan
		if withDue {
			tx.DueDate = &due
		}
		return tx
	}
	empty := manualPostingFingerprint([]*transaction.Transaction{makeTx(false, "")}, nil, nil)
	withPlan := manualPostingFingerprint([]*transaction.Transaction{makeTx(false, "Mensal")}, nil, nil)
	withDue := manualPostingFingerprint([]*transaction.Transaction{makeTx(true, "")}, nil, nil)

	if empty == withPlan {
		t.Fatal("fingerprint must change when payment plan is present")
	}
	if empty == withDue {
		t.Fatal("fingerprint must change when due date is present")
	}
	if withPlan == withDue {
		t.Fatal("plan and due date must influence the fingerprint independently")
	}
	if empty != manualPostingFingerprint([]*transaction.Transaction{makeTx(false, "")}, nil, nil) {
		t.Fatal("empty provenance must remain determinant/stable for legacy retries")
	}
}

func TestBatchCreatePostingLegacyRetryWithoutProvenanceIsIdempotent(t *testing.T) {
	svc := NewTransactionService(newStoringTxRepo())
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})
	items := []BatchCreateInput{{
		ContractID:  "contract-rent",
		DealID:      "deal-rent",
		Type:        "receita",
		Description: "Recebimento - Locação",
		Value:       100_000,
		DateStr:     "2026-08-29",
		Flow:        "rent",
	}}

	if _, err := svc.BatchCreatePosting(ctx, "tenant-rent", "legacy-key", items, nil, nil); err != nil {
		t.Fatalf("first posting: %v", err)
	}
	// Retry com a mesma operação e sem proveniência (comando legado) permanece
	// idempotente após a introdução do due_date/payment_plan.
	if _, err := svc.BatchCreatePosting(ctx, "tenant-rent", "legacy-key", items, nil, nil); err != nil {
		t.Fatalf("legacy retry must stay idempotent, got %v", err)
	}
}

func cancelReverseTestSetup() (*TransactionService, *storingTxRepo, *storingManualCommissionRepo, *recordingManualReceiptSettlement, *transaction.Transaction) {
	txRepo := newStoringTxRepo()
	commissionRepo := newStoringManualCommissionRepo()
	commService := NewCommissionService(commissionRepo)
	settlement := &recordingManualReceiptSettlement{}
	svc := NewTransactionService(txRepo).
		WithManualCommissionService(commService).
		WithManualReceiptSettlement(settlement)
	commService.WithManualTransactionService(svc)

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
	outputs, err := svc.BatchCreatePosting(ctx, "tenant-sale", "cancel-reverse-setup", items, commissions, retention)
	if err != nil {
		panic(err)
	}
	var receipt *transaction.Transaction
	for _, output := range outputs {
		if output.Transaction != nil && output.Transaction.Type == transaction.TypeReceita {
			receipt = output.Transaction
			break
		}
	}
	return svc, txRepo, commissionRepo, settlement, receipt
}

func TestTransactionServiceCancelPendingDistributedReceipt(t *testing.T) {
	svc, txRepo, commissionRepo, _, receipt := cancelReverseTestSetup()
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})

	cancelled, err := svc.Cancel(ctx, receipt.ID, "tenant-sale", "Cancelamento do recebimento")
	if err != nil {
		t.Fatalf("cancel pending distributed receipt: %v", err)
	}
	if transaction.CanonicalStatus(cancelled.Status) != transaction.StatusCancelled {
		t.Fatalf("receipt status = %s, want CANCELLED", cancelled.Status)
	}
	for _, c := range commissionRepo.byID {
		if c.Status != commission.StatusCancelled {
			t.Fatalf("linked commission %s status = %s, want CANCELLED", c.ID, c.Status)
		}
	}
	if txRepo.byID[receipt.ID] == nil || transaction.CanonicalStatus(txRepo.byID[receipt.ID].Status) != transaction.StatusCancelled {
		t.Fatal("cancelled receipt was not persisted")
	}
}

func TestTransactionServiceCancelPaidReceiptRejected(t *testing.T) {
	svc, _, _, _, receipt := cancelReverseTestSetup()
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})
	paidAt := time.Date(2026, time.August, 29, 18, 30, 0, 0, time.UTC)

	if _, err := svc.Settle(ctx, receipt.ID, "tenant-sale", &paidAt, "PIX"); err != nil {
		t.Fatalf("settle receipt: %v", err)
	}
	_, err := svc.Cancel(ctx, receipt.ID, "tenant-sale", "Cancelar pago")
	if !errors.Is(err, transaction.ErrInvalidStatusTransition) {
		t.Fatalf("cancel of a paid receipt must fail, got %v", err)
	}
}

func TestTransactionServiceReverseDistributedReceipt(t *testing.T) {
	svc, _, commissionRepo, settlement, receipt := cancelReverseTestSetup()
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})
	paidAt := time.Date(2026, time.August, 29, 18, 30, 0, 0, time.UTC)

	if _, err := svc.Settle(ctx, receipt.ID, "tenant-sale", &paidAt, "PIX"); err != nil {
		t.Fatalf("settle receipt: %v", err)
	}
	reversed, err := svc.Reverse(ctx, receipt.ID, "tenant-sale", "Estorno do recebimento", "operator-1")
	if err != nil {
		t.Fatalf("reverse distributed receipt: %v", err)
	}
	if reversed.Type != transaction.TypeEstorno || reversed.Value >= 0 {
		t.Fatalf("estorno not created: %+v", reversed)
	}
	if transaction.CanonicalStatus(txRepoByID(t, svc, receipt.ID).Status) != transaction.StatusReversed {
		t.Fatal("original receipt was not marked REVERSED")
	}
	for _, c := range commissionRepo.byID {
		if c.Status != commission.StatusCancelled && c.Status != commission.StatusReversed {
			t.Fatalf("linked commission %s status = %s, want CANCELLED/REVERSED", c.ID, c.Status)
		}
	}
	if settlement.cancelCalls != 1 {
		t.Fatalf("owner transfer cancel calls = %d, want 1", settlement.cancelCalls)
	}
}

func txRepoByID(t *testing.T, svc *TransactionService, id string) *transaction.Transaction {
	t.Helper()
	repo, ok := svc.repo.(*storingTxRepo)
	if !ok {
		t.Fatal("svc repo is not storingTxRepo")
	}
	return repo.byID[id]
}

func TestTransferServiceCreatesOneOwnerTransferForManualRentReceipt(t *testing.T) {
	repo := &chargeIdentityTransferRepo{}
	svc := NewTransferService(repo)
	receipt := transaction.New("contract-rent", transaction.TypeReceita, "Recebimento - Locação", 100_000, time.Date(2026, time.August, 29, 0, 0, 0, 0, time.UTC), "tenant-rent")
	receipt.ID = "receipt-rent"
	receipt.DealID = "deal-rent"
	receipt.EconomicBreakdown = &transaction.EconomicBreakdown{
		Flow:                   "rent",
		GrossCents:             10_000_000,
		CommissionPoolCents:    1_000_000,
		CommissionPayableCents: 600_000,
		AgencyRetainedCents:    400_000,
		AdminFeeCents:          1_000_000,
		OwnerPayableCents:      8_000_000,
	}
	paymentDate := time.Date(2026, time.August, 29, 17, 30, 0, 0, time.UTC)
	require.NoError(t, receipt.MarkPaid(paymentDate, "PIX"))

	created, wasCreated, err := svc.EnsureForManualRentReceipt(context.Background(), receipt, "owner-rent", "property-rent")
	require.NoError(t, err)
	require.True(t, wasCreated)
	require.NotNil(t, created.OwnerBalance)
	require.NotNil(t, created.EconomicBreakdown)
	assert.Equal(t, receipt.ID, created.SourceTransactionID)
	assert.Equal(t, "rent", created.SettlementType)
	assert.Equal(t, int64(8_000_000), created.OwnerBalance.TotalCents)
	assert.Equal(t, int64(8_000_000), created.OwnerBalance.AvailableCents())
	assert.Equal(t, int64(1_000_000), created.EconomicBreakdown.AdminFeeCents)
	assert.Equal(t, int64(2_000_000), created.EconomicBreakdown.AgencyRevenueCents)
	assert.Equal(t, int64(600_000), created.EconomicBreakdown.CommissionPayableCents)
	assert.Equal(t, int64(400_000), created.EconomicBreakdown.AgencyRetainedCents)

	retry, wasCreated, err := svc.EnsureForManualRentReceipt(context.Background(), receipt, "owner-rent", "property-rent")
	require.NoError(t, err)
	require.False(t, wasCreated)
	assert.Equal(t, created.ID, retry.ID)
	assert.Len(t, repo.items, 1)

	// Compatibility with a transfer posted before broker funding was split
	// from retained revenue: its historical payload and identity stay intact.
	created.EconomicBreakdown.CommissionPayableCents = 0
	created.EconomicBreakdown.CommissionFundedCents = 0
	created.EconomicBreakdown.IntermediationRevenueCents = 0
	created.EconomicBreakdown.AgencyRetainedCents = receipt.EconomicBreakdown.CommissionPoolCents
	legacy := *created.EconomicBreakdown
	repo.items[0] = cloneOwnerTransfer(created)
	retry, wasCreated, err = svc.EnsureForManualRentReceipt(context.Background(), receipt, "owner-rent", "property-rent")
	require.NoError(t, err)
	require.False(t, wasCreated)
	assert.Equal(t, legacy, *retry.EconomicBreakdown)
	assert.Len(t, repo.items, 1)
}
