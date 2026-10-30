package services

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/managed_bill"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

type fakeManagedBillRepo struct {
	byID     map[string]*managed_bill.ManagedBill
	bySource map[string]*managed_bill.ManagedBill
}

func newFakeManagedBillRepo() *fakeManagedBillRepo {
	return &fakeManagedBillRepo{
		byID:     make(map[string]*managed_bill.ManagedBill),
		bySource: make(map[string]*managed_bill.ManagedBill),
	}
}

func managedBillTenantKey(tenantID, value string) string { return tenantID + "\x00" + value }

func cloneManagedBill(bill *managed_bill.ManagedBill) *managed_bill.ManagedBill {
	if bill == nil {
		return nil
	}
	clone := *bill
	clone.Evidence = append([]managed_bill.Evidence(nil), bill.Evidence...)
	clone.History = append([]managed_bill.HistoryEntry(nil), bill.History...)
	clone.PaymentAttempts = append([]managed_bill.PaymentAttempt(nil), bill.PaymentAttempts...)
	return &clone
}

func (r *fakeManagedBillRepo) Create(_ context.Context, bill *managed_bill.ManagedBill) error {
	key := managedBillTenantKey(bill.TenantID, bill.SourceKey)
	if _, exists := r.bySource[key]; exists {
		return managed_bill.ErrAlreadyExists
	}
	for _, existing := range r.byID {
		if existing.TenantID == bill.TenantID && existing.SourceTransferID == bill.SourceTransferID &&
			existing.SourceChargeItemID == bill.SourceChargeItemID {
			return managed_bill.ErrAlreadyExists
		}
	}
	stored := cloneManagedBill(bill)
	r.byID[managedBillTenantKey(bill.TenantID, bill.ID)] = stored
	r.bySource[key] = stored
	return nil
}

func (r *fakeManagedBillRepo) GetByID(_ context.Context, tenantID, id string) (*managed_bill.ManagedBill, error) {
	return cloneManagedBill(r.byID[managedBillTenantKey(tenantID, id)]), nil
}

func (r *fakeManagedBillRepo) GetBySourceKey(_ context.Context, tenantID, sourceKey string) (*managed_bill.ManagedBill, error) {
	return cloneManagedBill(r.bySource[managedBillTenantKey(tenantID, sourceKey)]), nil
}

func (r *fakeManagedBillRepo) SumActiveReservedCentsByTransfer(_ context.Context, tenantID, transferID string) (int64, error) {
	var total int64
	for _, bill := range r.byID {
		if bill.TenantID == tenantID && bill.SourceTransferID == transferID &&
			(bill.Status == managed_bill.StatusReserved || bill.Status == managed_bill.StatusApproved) {
			total += bill.AmountCents
		}
	}
	return total, nil
}

func (r *fakeManagedBillRepo) UpdateIfVersion(_ context.Context, bill *managed_bill.ManagedBill, expectedVersion int64) (bool, error) {
	key := managedBillTenantKey(bill.TenantID, bill.ID)
	existing := r.byID[key]
	if existing == nil || existing.Version != expectedVersion || bill.Version != expectedVersion+1 {
		return false, managed_bill.ErrConcurrentModification
	}
	stored := cloneManagedBill(bill)
	r.byID[key] = stored
	r.bySource[managedBillTenantKey(bill.TenantID, bill.SourceKey)] = stored
	return true, nil
}

func (r *fakeManagedBillRepo) List(_ context.Context, tenantID string, filter managed_bill.Filter) ([]*managed_bill.ManagedBill, int64, error) {
	result := make([]*managed_bill.ManagedBill, 0)
	for _, bill := range r.byID {
		if bill.TenantID != tenantID || (filter.SourceTransferID != "" && bill.SourceTransferID != filter.SourceTransferID) ||
			(filter.ContractID != "" && bill.ContractID != filter.ContractID) ||
			(filter.PropertyID != "" && bill.PropertyID != filter.PropertyID) ||
			(filter.OwnerID != "" && bill.OwnerID != filter.OwnerID) ||
			(filter.CompetenceMonth != "" && bill.CompetenceMonth != filter.CompetenceMonth) ||
			(filter.Status != "" && bill.Status != filter.Status) {
			continue
		}
		result = append(result, cloneManagedBill(bill))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, int64(len(result)), nil
}

type managedBillFixture struct {
	service       *ManagedBillService
	bills         *fakeManagedBillRepo
	transfers     *chargeIdentityTransferRepo
	journal       *fakeJournalRepo
	ownerTransfer *transfer.OwnerTransfer
}

func newManagedBillFixture(t *testing.T) *managedBillFixture {
	t.Helper()
	item := transfer.New(
		"contract-1", "owner-1", "property-1", "tenant-1", "2026-08",
		5000, 8, 0, 0, nil, "allocation-aware transfer",
	)
	item.ID = "transfer-1"
	item.SourceChargeID = "charge-1"
	item.CreatedAt = time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	item.UpdatedAt = item.CreatedAt
	balance, err := transfer.NewOwnerBalance(560000, 100000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := item.SetAllocatedOwnerBalance(balance, 40000); err != nil {
		t.Fatal(err)
	}
	bills := newFakeManagedBillRepo()
	transfers := &chargeIdentityTransferRepo{items: []*transfer.OwnerTransfer{cloneOwnerTransfer(item)}}
	journalRepo := newFakeJournalRepo()
	service := NewManagedBillService(bills, transfers).WithJournalService(NewJournalService(journalRepo))
	service.atomicTxRunner = func(ctx context.Context, work func(context.Context) error) error {
		return work(ctx)
	}
	return &managedBillFixture{service: service, bills: bills, transfers: transfers, journal: journalRepo, ownerTransfer: item}
}

func managedBillReserveInput(itemID, sourceKey, component, description string, amountCents int64) managed_bill.NewReservedInput {
	return managed_bill.NewReservedInput{
		TenantID: "tenant-1", SourceKey: sourceKey, SourceTransferID: "transfer-1", SourceChargeID: "charge-1",
		SourceChargeItemID: itemID, ContractID: "contract-1", PropertyID: "property-1", OwnerID: "owner-1",
		Component: component, Creditor: description, Description: description, AmountCents: amountCents,
		DueDate: time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC), CompetenceMonth: "2026-08",
		ReservedBy: "system:charge-paid", ReservedAt: time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC),
	}
}

func TestManagedBillService_CanonicalReservePayCancelAndReconcile(t *testing.T) {
	fixture := newManagedBillFixture(t)
	ctx := context.Background()
	condoInput := managedBillReserveInput("condo-item", "reserve:condo", "condominio", "Condominio", 70000)
	iptuInput := managedBillReserveInput("iptu-item", "reserve:iptu", "iptu", "Prefeitura", 30000)

	condo, err := fixture.service.EnsureReserved(ctx, condoInput)
	if err != nil {
		t.Fatalf("reserve condo: %v", err)
	}
	if condo.CoverageComplete {
		t.Fatal("R$700 bill must not fully cover a R$1,000 reserve")
	}
	iptu, err := fixture.service.EnsureReserved(ctx, iptuInput)
	if err != nil {
		t.Fatalf("reserve IPTU: %v", err)
	}
	if !iptu.CoverageComplete {
		t.Fatal("R$700 + R$300 bills must fully cover the R$1,000 reserve")
	}

	approvedCondo, err := fixture.service.Approve(ctx, "tenant-1", condo.Bill.ID, "approver-1", 1)
	if err != nil {
		t.Fatalf("approve condo: %v", err)
	}
	paidCondo, err := fixture.service.Pay(ctx, "tenant-1", condo.Bill.ID, "payer-1", approvedCondo.Bill.Version, "pix-condo-1", "proof://condo", time.Time{})
	if err != nil {
		t.Fatalf("pay condo: %v", err)
	}
	assertManagedBillBalance(t, paidCondo.BalanceBefore, 560000, 100000, 0, 460000)
	assertManagedBillBalance(t, paidCondo.BalanceAfter, 490000, 30000, 0, 460000)
	if !paidCondo.CoverageComplete || paidCondo.Bill.Status != managed_bill.StatusPaid {
		t.Fatalf("unexpected paid result: %+v", paidCondo)
	}
	posting := fixture.journal.bySource[paidCondo.Bill.PaymentJournalSourceID]
	if posting == nil || len(posting.Legs) != 2 ||
		posting.Legs[0].Account != journal.AccountOwnerPayable || posting.Legs[0].Direction != journal.Debit ||
		posting.Legs[1].Account != journal.AccountCash || posting.Legs[1].Direction != journal.Credit {
		t.Fatalf("wrong managed-bill payment posting: %+v", posting)
	}
	for _, leg := range posting.Legs {
		if leg.AmountCents != 70000 || leg.ManagedBillID != condo.Bill.ID {
			t.Fatalf("journal leg lost amount/bill identity: %+v", leg)
		}
	}

	// Exact payment replay is a no-op and cannot consume the owner balance twice.
	paidRetry, err := fixture.service.Pay(ctx, "tenant-1", condo.Bill.ID, "payer-1", approvedCondo.Bill.Version, "pix-condo-1", "proof://condo", time.Time{})
	if err != nil {
		t.Fatalf("payment retry: %v", err)
	}
	assertManagedBillBalance(t, paidRetry.BalanceAfter, 490000, 30000, 0, 460000)
	if len(fixture.journal.bySource) != 1 {
		t.Fatalf("payment retry duplicated journal: %d entries", len(fixture.journal.bySource))
	}

	reconciled, err := fixture.service.Reconcile(ctx, "tenant-1", condo.Bill.ID, "reconciler-1", paidCondo.Bill.Version, "statement-line-1", time.Time{})
	if err != nil {
		t.Fatalf("reconcile condo: %v", err)
	}
	if reconciled.Bill.Status != managed_bill.StatusReconciled || posting.ReconciliationDate == nil {
		t.Fatalf("payment journal/bill were not reconciled: bill=%+v posting=%+v", reconciled.Bill, posting)
	}

	reversed, err := fixture.service.ReversePayment(
		ctx, "tenant-1", condo.Bill.ID, "controller-1", reconciled.Bill.Version,
		"bank-return-condo-1", "beneficiary account rejected the transfer", "proof://return-condo", time.Time{},
	)
	if err != nil {
		t.Fatalf("reverse reconciled condo payment: %v", err)
	}
	assertManagedBillBalance(t, reversed.BalanceBefore, 490000, 30000, 0, 460000)
	assertManagedBillBalance(t, reversed.BalanceAfter, 560000, 100000, 0, 460000)
	if reversed.Bill.Status != managed_bill.StatusApproved || len(reversed.Bill.PaymentAttempts) != 1 {
		t.Fatalf("payment reversal did not reopen the approved bill with one archived attempt: %+v", reversed.Bill)
	}
	attempt := reversed.Bill.PaymentAttempts[0]
	if attempt.PaymentReference != "pix-condo-1" || attempt.ReconciliationReference != "statement-line-1" ||
		attempt.ReversalReference != "bank-return-condo-1" || attempt.ReversalJournalSourceID == "" ||
		attempt.ReversalProofReference != "proof://return-condo" {
		t.Fatalf("payment attempt lost reversal provenance: %+v", attempt)
	}
	if posting.Status != journal.StatusReversed {
		t.Fatalf("original payment journal was not marked reversed: %+v", posting)
	}
	reversal := fixture.journal.bySource[attempt.ReversalJournalSourceID]
	if reversal == nil || reversal.ReversalOf != posting.ID || reversal.ReversedBy != "controller-1" ||
		!reversal.Date.Equal(attempt.ReversedAt) {
		t.Fatalf("structured journal reversal not found: %+v", reversal)
	}

	// Exact reversal replay is a no-op even with the pre-reversal version.
	reversedRetry, err := fixture.service.ReversePayment(
		ctx, "tenant-1", condo.Bill.ID, "controller-1", reconciled.Bill.Version,
		"bank-return-condo-1", "beneficiary account rejected the transfer", "proof://return-condo", time.Time{},
	)
	if err != nil {
		t.Fatalf("payment reversal retry: %v", err)
	}
	assertManagedBillBalance(t, reversedRetry.BalanceAfter, 560000, 100000, 0, 460000)
	storedAfterRetry, _ := fixture.transfers.GetByID(ctx, "tenant-1", "transfer-1")
	if len(storedAfterRetry.OwnerBalanceMovements) != 2 || len(fixture.journal.bySource) != 2 {
		t.Fatalf("reversal retry duplicated economic writes: movements=%d journal=%d", len(storedAfterRetry.OwnerBalanceMovements), len(fixture.journal.bySource))
	}

	repaid, err := fixture.service.Pay(
		ctx, "tenant-1", condo.Bill.ID, "payer-2", reversed.Bill.Version,
		"pix-condo-2", "proof://condo-2", time.Time{},
	)
	if err != nil {
		t.Fatalf("pay condo after reversal: %v", err)
	}
	assertManagedBillBalance(t, repaid.BalanceBefore, 560000, 100000, 0, 460000)
	assertManagedBillBalance(t, repaid.BalanceAfter, 490000, 30000, 0, 460000)
	if repaid.Bill.Status != managed_bill.StatusPaid || len(repaid.Bill.PaymentAttempts) != 1 ||
		repaid.Bill.PaymentReference != "pix-condo-2" {
		t.Fatalf("new payment attempt was not activated: %+v", repaid.Bill)
	}

	approvedIPTU, err := fixture.service.Approve(ctx, "tenant-1", iptu.Bill.ID, "approver-1", 1)
	if err != nil {
		t.Fatalf("approve IPTU: %v", err)
	}
	cancelledIPTU, err := fixture.service.Cancel(ctx, "tenant-1", iptu.Bill.ID, "operator-1", approvedIPTU.Bill.Version, "owner pays IPTU directly")
	if err != nil {
		t.Fatalf("cancel IPTU: %v", err)
	}
	assertManagedBillBalance(t, cancelledIPTU.BalanceBefore, 490000, 30000, 0, 460000)
	assertManagedBillBalance(t, cancelledIPTU.BalanceAfter, 490000, 0, 0, 490000)
	if !cancelledIPTU.CoverageComplete || cancelledIPTU.Bill.Status != managed_bill.StatusCancelled {
		t.Fatalf("unexpected cancellation result: %+v", cancelledIPTU)
	}

	payload, err := json.Marshal(cancelledIPTU)
	if err != nil {
		t.Fatal(err)
	}
	var public map[string]json.RawMessage
	if err := json.Unmarshal(payload, &public); err != nil {
		t.Fatal(err)
	}
	if _, ok := public["owner_balance_before"]; !ok {
		t.Fatal("mutation JSON must expose owner_balance_before")
	}
	if _, ok := public["owner_balance_after"]; !ok {
		t.Fatal("mutation JSON must expose owner_balance_after")
	}
	if _, stale := public["balance_before"]; stale {
		t.Fatal("mutation JSON must not expose balance_before")
	}
}

func TestManagedBillService_RejectsOverallocationAndCrossTenantRead(t *testing.T) {
	fixture := newManagedBillFixture(t)
	input := managedBillReserveInput("too-large", "reserve:too-large", "iptu", "Prefeitura", 100001)
	if _, err := fixture.service.EnsureReserved(context.Background(), input); !errors.Is(err, ErrManagedBillReserveOverallocated) {
		t.Fatalf("overallocated reserve: got %v, want ErrManagedBillReserveOverallocated", err)
	}
	valid, err := fixture.service.EnsureReserved(context.Background(), managedBillReserveInput("condo", "reserve:condo", "condominio", "Condominio", 70000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.GetByID(context.Background(), "tenant-2", valid.Bill.ID); !errors.Is(err, managed_bill.ErrNotFound) {
		t.Fatalf("cross-tenant read: got %v, want ErrNotFound", err)
	}
}

func TestManagedBillService_CancelRequiresPendingTransferAndPayRequiresSegregatedActor(t *testing.T) {
	fixture := newManagedBillFixture(t)
	ctx := context.Background()
	reserved, err := fixture.service.EnsureReserved(ctx, managedBillReserveInput("condo", "reserve:condo", "condominio", "Condominio", 70000))
	if err != nil {
		t.Fatal(err)
	}
	approved, err := fixture.service.Approve(ctx, "tenant-1", reserved.Bill.ID, "operator-1", reserved.Bill.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.Pay(ctx, "tenant-1", reserved.Bill.ID, "operator-1", approved.Bill.Version, "pix-1", "", time.Time{}); !errors.Is(err, ErrManagedBillActorConflict) {
		t.Fatalf("same approver/payer: got %v, want ErrManagedBillActorConflict", err)
	}
	stored, _ := fixture.transfers.GetByID(ctx, "tenant-1", "transfer-1")
	if len(stored.OwnerBalanceMovements) != 0 {
		t.Fatalf("actor conflict mutated transfer: %+v", stored.OwnerBalanceMovements)
	}

	fixture.transfers.items[0].Status = transfer.StatusApproved
	if _, err := fixture.service.Cancel(ctx, "tenant-1", reserved.Bill.ID, "operator-2", approved.Bill.Version, "cancelled by owner"); !errors.Is(err, ErrManagedBillTransferNotPending) {
		t.Fatalf("cancel on approved transfer: got %v, want ErrManagedBillTransferNotPending", err)
	}
	stored, _ = fixture.transfers.GetByID(ctx, "tenant-1", "transfer-1")
	if len(stored.OwnerBalanceMovements) != 0 || stored.OwnerBalance.ReservedCents != 100000 {
		t.Fatalf("rejected cancellation mutated transfer: %+v", stored)
	}
}

func TestManagedBillService_ReversePaymentRequiresSegregatedActorAndExactRetry(t *testing.T) {
	fixture := newManagedBillFixture(t)
	ctx := context.Background()
	reserved, err := fixture.service.EnsureReserved(ctx, managedBillReserveInput("condo", "reserve:condo", "condominio", "Condominio", 70000))
	if err != nil {
		t.Fatal(err)
	}
	approved, err := fixture.service.Approve(ctx, "tenant-1", reserved.Bill.ID, "approver-1", reserved.Bill.Version)
	if err != nil {
		t.Fatal(err)
	}
	paid, err := fixture.service.Pay(ctx, "tenant-1", reserved.Bill.ID, "payer-1", approved.Bill.Version, "pix-1", "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	testUnitOfWork := fixture.service.atomicTxRunner
	fixture.service.atomicTxRunner = nil
	if _, err := fixture.service.ReversePayment(
		ctx, "tenant-1", reserved.Bill.ID, "controller-1", paid.Bill.Version,
		"return-atomic", "bank rejected", "", time.Time{},
	); !errors.Is(err, ErrManagedBillAtomicUnitOfWork) {
		t.Fatalf("reversal without atomic unit of work: got %v, want ErrManagedBillAtomicUnitOfWork", err)
	}
	fixture.service.atomicTxRunner = testUnitOfWork

	if _, err := fixture.service.ReversePayment(
		ctx, "tenant-1", reserved.Bill.ID, "payer-1", paid.Bill.Version,
		"return-1", "bank rejected", "", time.Time{},
	); !errors.Is(err, ErrManagedBillReversalActorConflict) {
		t.Fatalf("same payer/reverser: got %v, want ErrManagedBillReversalActorConflict", err)
	}
	stored, _ := fixture.transfers.GetByID(ctx, "tenant-1", "transfer-1")
	if len(stored.OwnerBalanceMovements) != 1 || len(fixture.journal.bySource) != 1 {
		t.Fatalf("rejected reversal mutated state: movements=%d journal=%d", len(stored.OwnerBalanceMovements), len(fixture.journal.bySource))
	}

	reversed, err := fixture.service.ReversePayment(
		ctx, "tenant-1", reserved.Bill.ID, "controller-1", paid.Bill.Version,
		"return-1", "bank rejected", "proof://return-1", time.Time{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.ReversePayment(
		ctx, "tenant-1", reserved.Bill.ID, "controller-1", paid.Bill.Version,
		"return-2", "different command", "", time.Time{},
	); !errors.Is(err, managed_bill.ErrConcurrentModification) {
		t.Fatalf("divergent stale reversal: got %v, want ErrConcurrentModification", err)
	}
	if _, err := fixture.service.ReversePayment(
		ctx, "tenant-1", reserved.Bill.ID, "controller-1", paid.Bill.Version,
		"return-1", "bank rejected", "proof://different", time.Time{},
	); !errors.Is(err, managed_bill.ErrConcurrentModification) {
		t.Fatalf("divergent reversal proof: got %v, want ErrConcurrentModification", err)
	}
	if _, err := fixture.service.ReversePayment(
		ctx, "tenant-1", reserved.Bill.ID, "controller-1", paid.Bill.Version,
		"return-1", "bank rejected", "proof://return-1", reversed.Bill.PaymentAttempts[0].ReversedAt.Add(time.Second),
	); !errors.Is(err, managed_bill.ErrConcurrentModification) {
		t.Fatalf("divergent reversal timestamp: got %v, want ErrConcurrentModification", err)
	}
	if reversed.Bill.Version != paid.Bill.Version+1 {
		t.Fatalf("reversal version = %d, want %d", reversed.Bill.Version, paid.Bill.Version+1)
	}
}

func TestManagedBillService_ReversePaymentRejectsDivergentPaymentJournal(t *testing.T) {
	fixture := newManagedBillFixture(t)
	ctx := context.Background()
	reserved, err := fixture.service.EnsureReserved(ctx, managedBillReserveInput("condo", "reserve:condo", "condominio", "Condominio", 70000))
	if err != nil {
		t.Fatal(err)
	}
	approved, err := fixture.service.Approve(ctx, "tenant-1", reserved.Bill.ID, "approver-1", reserved.Bill.Version)
	if err != nil {
		t.Fatal(err)
	}
	paid, err := fixture.service.Pay(ctx, "tenant-1", reserved.Bill.ID, "payer-1", approved.Bill.Version, "pix-1", "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	posting := fixture.journal.bySource[paid.Bill.PaymentJournalSourceID]
	posting.Legs[0].ManagedBillID = "another-bill"

	if _, err := fixture.service.ReversePayment(
		ctx, "tenant-1", reserved.Bill.ID, "controller-1", paid.Bill.Version,
		"return-1", "bank rejected", "", time.Time{},
	); !errors.Is(err, journal.ErrSourceEventConflict) {
		t.Fatalf("divergent payment journal: got %v, want ErrSourceEventConflict", err)
	}
	stored, _ := fixture.transfers.GetByID(ctx, "tenant-1", "transfer-1")
	if len(stored.OwnerBalanceMovements) != 1 || stored.OwnerBalance.TotalCents != 490000 || stored.OwnerBalance.ReservedCents != 30000 || len(fixture.journal.bySource) != 1 {
		t.Fatalf("rejected journal witness mutated state: transfer=%+v journal=%d", stored, len(fixture.journal.bySource))
	}
	storedBill, _ := fixture.bills.GetByID(ctx, "tenant-1", reserved.Bill.ID)
	if storedBill.Status != managed_bill.StatusPaid || len(storedBill.PaymentAttempts) != 0 {
		t.Fatalf("rejected journal witness mutated bill: %+v", storedBill)
	}
}

func assertManagedBillBalance(t *testing.T, got ManagedBillBalanceSnapshot, total, reserved, blocked, available int64) {
	t.Helper()
	if got.TotalCents != total || got.ReservedCents != reserved || got.BlockedCents != blocked || got.AvailableCents != available {
		t.Fatalf("balance = %+v, want total=%d reserved=%d blocked=%d available=%d", got, total, reserved, blocked, available)
	}
}
