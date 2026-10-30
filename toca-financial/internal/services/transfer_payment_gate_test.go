package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/managed_bill"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type paymentGateTransferRepo struct {
	existing    *transfer.OwnerTransfer
	createCalls int
	lookupCalls int
}

type chargeIdentityTransferRepo struct {
	items                 []*transfer.OwnerTransfer
	deductionClaims       map[string]string
	deductionClaimDetails map[string]transfer.Deduction
}

func cloneOwnerTransfer(item *transfer.OwnerTransfer) *transfer.OwnerTransfer {
	if item == nil {
		return nil
	}
	clone := *item
	if item.OwnerBalance != nil {
		balance := *item.OwnerBalance
		clone.OwnerBalance = &balance
	}
	if item.EconomicBreakdown != nil {
		breakdown := *item.EconomicBreakdown
		clone.EconomicBreakdown = &breakdown
	}
	clone.DeductionDetails = append([]transfer.Deduction(nil), item.DeductionDetails...)
	clone.OwnerBalanceMovements = append([]transfer.OwnerBalanceMovement(nil), item.OwnerBalanceMovements...)
	clone.History = append([]transfer.HistoryEntry(nil), item.History...)
	return &clone
}

func (r *chargeIdentityTransferRepo) Create(_ context.Context, item *transfer.OwnerTransfer) error {
	for _, existing := range r.items {
		if item.SourceChargeID != "" && existing.TenantID == item.TenantID && existing.SourceChargeID == item.SourceChargeID {
			return transfer.ErrTransferAlreadyExists
		}
		if item.SourceChargeID == "" && item.SourceTransactionID == "" && existing.SourceChargeID == "" && existing.SourceTransactionID == "" && existing.TenantID == item.TenantID && existing.ContractID == item.ContractID && existing.ReferenceMonth == item.ReferenceMonth {
			return transfer.ErrTransferAlreadyExists
		}
	}
	r.items = append(r.items, cloneOwnerTransfer(item))
	return nil
}

func (r *chargeIdentityTransferRepo) GetByID(_ context.Context, tenantID, id string) (*transfer.OwnerTransfer, error) {
	for _, item := range r.items {
		if item.TenantID == tenantID && item.ID == id {
			return cloneOwnerTransfer(item), nil
		}
	}
	return nil, nil
}

func (r *chargeIdentityTransferRepo) GetByContractMonth(_ context.Context, tenantID, contractID, referenceMonth string) (*transfer.OwnerTransfer, error) {
	for _, item := range r.items {
		if item.TenantID == tenantID && item.ContractID == contractID && item.ReferenceMonth == referenceMonth {
			return cloneOwnerTransfer(item), nil
		}
	}
	return nil, nil
}

func (r *chargeIdentityTransferRepo) GetBySourceChargeID(_ context.Context, tenantID, sourceChargeID string) (*transfer.OwnerTransfer, error) {
	for _, item := range r.items {
		if item.TenantID == tenantID && item.SourceChargeID == sourceChargeID {
			return cloneOwnerTransfer(item), nil
		}
	}
	return nil, nil
}

func (r *chargeIdentityTransferRepo) GetBySourceTransactionID(_ context.Context, tenantID, sourceTransactionID string) (*transfer.OwnerTransfer, error) {
	for _, item := range r.items {
		if item.TenantID == tenantID && item.SourceTransactionID == sourceTransactionID {
			return cloneOwnerTransfer(item), nil
		}
	}
	return nil, nil
}

func (r *chargeIdentityTransferRepo) GetLegacyByContractMonth(_ context.Context, tenantID, contractID, referenceMonth string) (*transfer.OwnerTransfer, error) {
	for _, item := range r.items {
		if item.SourceChargeID == "" && item.SourceTransactionID == "" && item.TenantID == tenantID && item.ContractID == contractID && item.ReferenceMonth == referenceMonth {
			return cloneOwnerTransfer(item), nil
		}
	}
	return nil, nil
}

func (r *chargeIdentityTransferRepo) ClaimLegacyForCharge(_ context.Context, claimed *transfer.OwnerTransfer) (bool, error) {
	for index, item := range r.items {
		if item.ID == claimed.ID && item.TenantID == claimed.TenantID && item.SourceChargeID == "" && item.SourceTransactionID == "" {
			r.items[index] = cloneOwnerTransfer(claimed)
			return true, nil
		}
	}
	return false, nil
}

func (r *chargeIdentityTransferRepo) ApplyDeductionIdempotent(
	_ context.Context,
	tenantID, contractID, referenceMonth string,
	detail transfer.Deduction,
) (*transfer.OwnerTransfer, bool, error) {
	if r.deductionClaims == nil {
		r.deductionClaims = make(map[string]string)
		r.deductionClaimDetails = make(map[string]transfer.Deduction)
	}
	claimKey := tenantID + "\x00" + detail.ReferenceID
	if transferID := r.deductionClaims[claimKey]; transferID != "" {
		claimed := r.deductionClaimDetails[claimKey]
		if claimed.Type != detail.Type || claimed.Amount != detail.Amount {
			return nil, false, fmt.Errorf("deduction reference %s reused with divergent payload", detail.ReferenceID)
		}
		for _, item := range r.items {
			if item.TenantID == tenantID && item.ID == transferID {
				return cloneOwnerTransfer(item), false, nil
			}
		}
	}
	for _, item := range r.items {
		if item.TenantID != tenantID {
			continue
		}
		for _, existing := range item.DeductionDetails {
			if existing.ReferenceID == detail.ReferenceID {
				if existing.Type != detail.Type || existing.Amount != detail.Amount {
					return nil, false, fmt.Errorf("deduction reference %s reused with divergent payload", detail.ReferenceID)
				}
				r.deductionClaims[claimKey] = item.ID
				r.deductionClaimDetails[claimKey] = existing
				return cloneOwnerTransfer(item), false, nil
			}
		}
	}

	selected := -1
	for index, item := range r.items {
		if item.TenantID != tenantID || item.ContractID != contractID || item.ReferenceMonth != referenceMonth || item.Status != transfer.StatusPending {
			continue
		}
		if selected < 0 || item.CreatedAt.Before(r.items[selected].CreatedAt) || (item.CreatedAt.Equal(r.items[selected].CreatedAt) && item.ID < r.items[selected].ID) {
			selected = index
		}
	}
	if selected < 0 {
		return nil, false, nil
	}
	chosen := cloneOwnerTransfer(r.items[selected])
	applyTransferDeduction(chosen, detail)
	r.items[selected] = cloneOwnerTransfer(chosen)
	r.deductionClaims[claimKey] = chosen.ID
	r.deductionClaimDetails[claimKey] = detail
	return chosen, true, nil
}

func (r *chargeIdentityTransferRepo) Update(_ context.Context, item *transfer.OwnerTransfer) error {
	for index, existing := range r.items {
		if existing.ID == item.ID && existing.TenantID == item.TenantID {
			r.items[index] = cloneOwnerTransfer(item)
			return nil
		}
	}
	return nil
}

func (r *chargeIdentityTransferRepo) UpdateIfVersion(_ context.Context, item *transfer.OwnerTransfer, expected time.Time) (bool, error) {
	for index, existing := range r.items {
		if existing.ID == item.ID && existing.TenantID == item.TenantID {
			if !existing.UpdatedAt.Equal(expected) {
				return false, nil
			}
			r.items[index] = cloneOwnerTransfer(item)
			return true, nil
		}
	}
	return false, nil
}

func (r *chargeIdentityTransferRepo) List(_ context.Context, _, _, _, _, _ string, _, _ int64) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}

func (r *chargeIdentityTransferRepo) ListByOwnerLeadIDs(_ context.Context, _ string, _ []string, _ transfer.PortalListFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}

func (r *chargeIdentityTransferRepo) UpdateStatus(_ context.Context, _, _ string, _ transfer.Status) error {
	return nil
}

func (r *chargeIdentityTransferRepo) Search(_ context.Context, _ string, _ transfer.SearchFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}

func (r *paymentGateTransferRepo) Create(_ context.Context, item *transfer.OwnerTransfer) error {
	r.createCalls++
	r.existing = item
	return nil
}

func (r *paymentGateTransferRepo) GetByID(_ context.Context, tenantID, id string) (*transfer.OwnerTransfer, error) {
	if r.existing != nil && r.existing.TenantID == tenantID && r.existing.ID == id {
		return r.existing, nil
	}
	return nil, nil
}

func (r *paymentGateTransferRepo) GetByContractMonth(_ context.Context, tenantID, contractID, referenceMonth string) (*transfer.OwnerTransfer, error) {
	r.lookupCalls++
	if r.existing != nil && r.existing.TenantID == tenantID && r.existing.ContractID == contractID && r.existing.ReferenceMonth == referenceMonth {
		return r.existing, nil
	}
	return nil, nil
}

func (r *paymentGateTransferRepo) Update(_ context.Context, item *transfer.OwnerTransfer) error {
	r.existing = item
	return nil
}

func (r *paymentGateTransferRepo) List(_ context.Context, _, _, _, _, _ string, _, _ int64) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}

func (r *paymentGateTransferRepo) ListByOwnerLeadIDs(_ context.Context, _ string, _ []string, _ transfer.PortalListFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}

func (r *paymentGateTransferRepo) UpdateStatus(_ context.Context, _, _ string, _ transfer.Status) error {
	return nil
}

func (r *paymentGateTransferRepo) Search(_ context.Context, _ string, _ transfer.SearchFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}

func newTransferGateCharge(t *testing.T, status charge.Status, paidCents *int64) *charge.Charge {
	t.Helper()
	c, err := charge.New(
		"contract-transfer",
		"payer-transfer",
		"owner-transfer",
		"property-transfer",
		"tenant-transfer",
		charge.TypeRent,
		"Aluguel",
		"2026-08",
		100,
		time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("create charge fixture: %v", err)
	}
	c.ID = "charge-transfer"
	c.Status = status
	if paidCents != nil {
		paid := valueobjects.NewMoney(*paidCents)
		c.PaidAmount = &paid
	}
	return c
}

func centsPtr(value int64) *int64 { return &value }

func newAllocatedPaidRentCharge(
	t *testing.T,
	id string,
	totalCents int64,
	items []charge.LineItem,
	allocations []charge.Allocation,
) *charge.Charge {
	t.Helper()
	c, err := charge.New(
		"contract-transfer",
		"payer-transfer",
		"owner-transfer",
		"property-transfer",
		"tenant-transfer",
		charge.TypeRent,
		"Aluguel + Encargos",
		"2026-08",
		float64(totalCents)/100,
		time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("create allocated charge fixture: %v", err)
	}
	c.ID = id
	c.LineItems = append([]charge.LineItem(nil), items...)
	c.Payments = []charge.PaymentEntry{{
		ID:          "payment-allocated",
		AmountCents: totalCents,
		Method:      "PIX",
		PaidAt:      time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC),
		Allocations: append([]charge.Allocation(nil), allocations...),
	}}
	c.Status = charge.StatusPaid
	paid := valueobjects.NewMoney(totalCents)
	c.PaidAmount = &paid
	return c
}

func allocatedSlice(id, itemID string, amountCents int64, allocationType charge.AllocationType, beneficiary string) charge.Allocation {
	return allocatedSliceForPayment(id, "payment-allocated", itemID, amountCents, allocationType, beneficiary)
}

func allocatedSliceForPayment(id, paymentID, itemID string, amountCents int64, allocationType charge.AllocationType, beneficiary string) charge.Allocation {
	return charge.Allocation{
		ID:                 id,
		PaymentID:          paymentID,
		ChargeItemID:       itemID,
		AmountCents:        amountCents,
		AllocationType:     allocationType,
		BeneficiaryPartyID: beneficiary,
		RuleVersionID:      "rule-v1",
	}
}

func TestTransferServiceRejectsEveryNonFullyPaidCharge(t *testing.T) {
	tests := []struct {
		name      string
		status    charge.Status
		paidCents *int64
	}{
		{name: "pending", status: charge.StatusPending},
		{name: "partial state", status: charge.StatusPartiallyPaid, paidCents: centsPtr(4000)},
		{name: "paid state with short balance", status: charge.StatusPaid, paidCents: centsPtr(4000)},
		{name: "paid state without balance", status: charge.StatusPaid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &paymentGateTransferRepo{}
			svc := NewTransferService(repo)
			c := newTransferGateCharge(t, tt.status, tt.paidCents)

			created, wasCreated, err := svc.EnsureMonthlyForChargePaid(context.Background(), c, 10)

			if err == nil {
				t.Fatal("expected fully-paid gate error")
			}
			if created != nil || wasCreated {
				t.Fatalf("non-fully-paid charge produced transfer: transfer=%+v created=%v", created, wasCreated)
			}
			if repo.lookupCalls != 0 || repo.createCalls != 0 {
				t.Fatalf("gate must reject before repository access, got lookups=%d creates=%d", repo.lookupCalls, repo.createCalls)
			}
		})
	}
}

func TestTransferChargeHookCreatesExactlyOnceOnlyAfterFullSettlement(t *testing.T) {
	repo := &paymentGateTransferRepo{}
	hook := NewTransferChargeHook(NewTransferService(repo), nil)

	partial := newTransferGateCharge(t, charge.StatusPartiallyPaid, centsPtr(4000))
	if err := hook.EnsureMonthlyForChargePaid(context.Background(), partial); err != nil {
		t.Fatalf("partial payment should be an intentional hook no-op: %v", err)
	}
	if repo.lookupCalls != 0 || repo.createCalls != 0 {
		t.Fatalf("partial payment reached transfer repository: lookups=%d creates=%d", repo.lookupCalls, repo.createCalls)
	}

	fullyPaid := newTransferGateCharge(t, charge.StatusPaid, centsPtr(10000))
	for attempt := 0; attempt < 2; attempt++ {
		if err := hook.EnsureMonthlyForChargePaid(context.Background(), fullyPaid); err != nil {
			t.Fatalf("fully-paid hook attempt %d: %v", attempt+1, err)
		}
	}

	if repo.createCalls != 1 {
		t.Fatalf("expected exactly one transfer across retry, got %d", repo.createCalls)
	}
	if repo.existing == nil {
		t.Fatal("expected fully-paid charge to create transfer")
	}
	if repo.existing.GrossRent != 100 || repo.existing.NetAmount != 100 {
		t.Fatalf("transfer must use the fully settled amount, got gross=%.2f net=%.2f", repo.existing.GrossRent, repo.existing.NetAmount)
	}
}

func TestTransferChargeHookSkipsChargeWithoutOwner(t *testing.T) {
	repo := &paymentGateTransferRepo{}
	hook := NewTransferChargeHook(NewTransferService(repo), nil)

	fullyPaid := newTransferGateCharge(t, charge.StatusPaid, centsPtr(10000))
	fullyPaid.OwnerID = ""

	if err := hook.EnsureMonthlyForChargePaid(context.Background(), fullyPaid); err != nil {
		t.Fatalf("charge without owner must not fail the payment: %v", err)
	}
	if repo.createCalls != 0 || repo.lookupCalls != 0 {
		t.Fatalf("charge without owner must skip the transfer: creates=%d lookups=%d", repo.createCalls, repo.lookupCalls)
	}
}

func TestTransferServiceBuildsOwnerBalanceFromAccumulatedAllocations(t *testing.T) {
	items := []charge.LineItem{
		{
			ID: "rent-item", Description: "Aluguel", AmountCents: 500000,
			SourceType: "rental_base", SourceID: "rent-config", EconomicOwnerID: "owner-transfer",
			SettlementMode: charge.SettlementModeCollection, RuleVersionID: "rule-v1",
		},
		{
			ID: "condo-item", Description: "Condomínio", AmountCents: 70000,
			SourceType: "rental_bundled_item", SourceID: "condominio", EconomicOwnerID: "owner-transfer",
			SettlementMode: charge.SettlementModeDirectLiquidation, RuleVersionID: "rule-v1",
		},
		{
			ID: "iptu-item", Description: "IPTU", AmountCents: 30000,
			SourceType: "rental_bundled_item", SourceID: "iptu", EconomicOwnerID: "owner-transfer",
			SettlementMode: charge.SettlementModeDirectLiquidation, RuleVersionID: "rule-v1",
		},
	}
	allocations := []charge.Allocation{
		allocatedSlice("allocation-owner-rent", "rent-item", 460000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
		allocatedSlice("allocation-admin-fee", "rent-item", 40000, charge.AllocationTypeRevenue, "tenant-transfer"),
		allocatedSlice("allocation-owner-condo", "condo-item", 70000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
		allocatedSlice("allocation-owner-iptu", "iptu-item", 30000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
	}
	c := newAllocatedPaidRentCharge(t, "charge-allocated", 600000, items, allocations)
	// Two partial payments exercise the accumulated-allocation path. Their
	// economic slices add up to the canonical full settlement.
	c.Payments = []charge.PaymentEntry{
		{
			ID: "payment-partial-1", AmountCents: 300000, Method: "PIX", PaidAt: time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC),
			Allocations: []charge.Allocation{
				allocatedSliceForPayment("a1-owner-rent", "payment-partial-1", "rent-item", 230000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
				allocatedSliceForPayment("a1-admin-fee", "payment-partial-1", "rent-item", 20000, charge.AllocationTypeRevenue, "tenant-transfer"),
				allocatedSliceForPayment("a1-owner-condo", "payment-partial-1", "condo-item", 35000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
				allocatedSliceForPayment("a1-owner-iptu", "payment-partial-1", "iptu-item", 15000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
			},
		},
		{
			ID: "payment-partial-2", AmountCents: 300000, Method: "PIX", PaidAt: time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC),
			Allocations: []charge.Allocation{
				allocatedSliceForPayment("a2-owner-rent", "payment-partial-2", "rent-item", 230000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
				allocatedSliceForPayment("a2-admin-fee", "payment-partial-2", "rent-item", 20000, charge.AllocationTypeRevenue, "tenant-transfer"),
				allocatedSliceForPayment("a2-owner-condo", "payment-partial-2", "condo-item", 35000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
				allocatedSliceForPayment("a2-owner-iptu", "payment-partial-2", "iptu-item", 15000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
			},
		},
	}
	repo := &paymentGateTransferRepo{}
	svc := NewTransferService(repo)

	created, wasCreated, err := svc.EnsureMonthlyForChargePaid(context.Background(), c, 8)
	if err != nil || !wasCreated {
		t.Fatalf("allocation-aware transfer: transfer=%+v created=%v err=%v", created, wasCreated, err)
	}
	if created.GrossRent != 5000 || created.AdminFeePercent != 8 || created.AdminFeeAmount != 400 {
		t.Fatalf("wrong rental base/admin fee: gross=%.2f pct=%.4f fee=%.2f", created.GrossRent, created.AdminFeePercent, created.AdminFeeAmount)
	}
	if created.OwnerBalance == nil {
		t.Fatal("allocation-aware transfer must persist owner balance")
	}
	if created.OwnerBalance.TotalCents != 560000 || created.OwnerBalance.ReservedCents != 100000 || created.OwnerBalance.BlockedCents != 0 {
		t.Fatalf("wrong owner payable buckets: %+v", created.OwnerBalance)
	}
	if created.OwnerBalance.AvailableCents() != 460000 || created.NetAmount != 4600 {
		t.Fatalf("wrong available transfer: balance=%+v net=%.2f", created.OwnerBalance, created.NetAmount)
	}

	retry, wasCreated, err := svc.EnsureMonthlyForChargePaid(context.Background(), c, 8)
	if err != nil || wasCreated || retry.ID != created.ID || repo.createCalls != 1 {
		t.Fatalf("allocation-aware retry was not idempotent: retry=%+v created=%v calls=%d err=%v", retry, wasCreated, repo.createCalls, err)
	}
}

func TestTransferServiceAutomaticallySettlesChargeWithNoOwnerResidual(t *testing.T) {
	items := []charge.LineItem{{
		ID: "rent-item", Description: "Primeiro aluguel", AmountCents: 320000,
		SourceType: "rental_base", EconomicOwnerID: "owner-transfer",
		SettlementMode: charge.SettlementModeCollection, RuleVersionID: "rule-v1",
	}}
	admin := allocatedSlice("allocation-admin", "rent-item", 32000, charge.AllocationTypeRevenue, "tenant-transfer")
	admin.Purpose = charge.AllocationPurposeAdminFee
	broker := allocatedSlice("allocation-broker", "rent-item", 288000, charge.AllocationTypeRevenue, "tenant-transfer")
	broker.Purpose = charge.AllocationPurposeRentalIntermediationBroker
	c := newAllocatedPaidRentCharge(t, "charge-zero-owner-residual", 320000, items, []charge.Allocation{admin, broker})
	repo := &chargeIdentityTransferRepo{}

	created, wasCreated, err := NewTransferService(repo).EnsureMonthlyForChargePaid(context.Background(), c, 10)
	require.NoError(t, err)
	require.True(t, wasCreated)
	require.NotNil(t, created.OwnerBalance)
	assert.Zero(t, created.OwnerBalance.TotalCents)
	assert.Zero(t, created.NetAmount)
	assert.Equal(t, transfer.StatusProcessed, created.Status)
	assert.Equal(t, "system", created.ProcessedBy)
	require.NotNil(t, created.ProcessedAt)
	assert.Equal(t, []string{"created", "processed"}, []string{created.History[0].Action, created.History[1].Action})

	retry, wasCreated, err := NewTransferService(repo).EnsureMonthlyForChargePaid(context.Background(), c, 10)
	require.NoError(t, err)
	require.False(t, wasCreated)
	assert.Equal(t, created.ID, retry.ID)
	assert.Equal(t, transfer.StatusProcessed, retry.Status)
	assert.Len(t, retry.History, 2)
}

func TestTransferServiceBuildsSaleSettlementFromEconomicAllocations(t *testing.T) {
	items := []charge.LineItem{
		{ID: "owner", Description: "Saldo do proprietário", AmountCents: 92120000, SourceType: "sale_owner_proceeds", EconomicOwnerID: "owner-transfer", SettlementMode: charge.SettlementModeCollection, AllocationType: charge.AllocationTypeOwnerPayable, RuleVersionID: "rule-v1"},
		{ID: "brokers", Description: "Comissões a corretores", AmountCents: 3528000, SourceType: "sale_commission_payable", EconomicOwnerID: "tenant-transfer", SettlementMode: charge.SettlementModeCollection, AllocationType: charge.AllocationTypeCommissionPayable, RuleVersionID: "rule-v1"},
		{ID: "agency", Description: "Retenção da imobiliária", AmountCents: 2352000, SourceType: "sale_agency_retained", EconomicOwnerID: "tenant-transfer", SettlementMode: charge.SettlementModeCollection, AllocationType: charge.AllocationTypeRevenue, RuleVersionID: "rule-v1"},
	}
	allocations := []charge.Allocation{
		allocatedSlice("a-owner", "owner", 92120000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
		allocatedSlice("a-brokers", "brokers", 3528000, charge.AllocationTypeCommissionPayable, "tenant-transfer"),
		allocatedSlice("a-agency", "agency", 2352000, charge.AllocationTypeRevenue, "tenant-transfer"),
	}
	c := newAllocatedPaidRentCharge(t, "charge-sale-waterfall", 98000000, items, allocations)
	c.Type = charge.TypeSale

	created, wasCreated, err := NewTransferService(&paymentGateTransferRepo{}).EnsureMonthlyForChargePaid(context.Background(), c, 0)
	require.NoError(t, err)
	require.True(t, wasCreated)
	require.NotNil(t, created.OwnerBalance)
	require.NotNil(t, created.EconomicBreakdown)
	assert.Equal(t, "sale", created.SettlementType)
	assert.Equal(t, int64(92120000), created.OwnerBalance.TotalCents)
	assert.Equal(t, int64(92120000), created.OwnerBalance.AvailableCents())
	assert.Equal(t, float64(921200), created.NetAmount)
	assert.Equal(t, int64(98000000), created.EconomicBreakdown.GrossCents)
	assert.Equal(t, int64(3528000), created.EconomicBreakdown.CommissionPayableCents)
	assert.Equal(t, int64(2352000), created.EconomicBreakdown.AgencyRetainedCents)
	assert.Equal(t, int64(0), created.EconomicBreakdown.DiscountCents)
}

func TestTransferServiceExplainsRentalAdministrationIntermediationAndOwnerBalance(t *testing.T) {
	items := []charge.LineItem{{
		ID: "rent", Description: "Primeiro aluguel proporcional", AmountCents: 300000,
		SourceType: "rental_base", EconomicOwnerID: "owner-transfer",
		SettlementMode: charge.SettlementModeCollection, RuleVersionID: "rule-v2",
	}}
	owner := allocatedSlice("a-owner", "rent", 150000, charge.AllocationTypeOwnerPayable, "owner-transfer")
	owner.Purpose = charge.AllocationPurposeOwnerRent
	admin := allocatedSlice("a-admin", "rent", 30000, charge.AllocationTypeRevenue, "tenant-transfer")
	admin.Purpose = charge.AllocationPurposeAdminFee
	brokerFunding := allocatedSlice("a-broker", "rent", 100000, charge.AllocationTypeRevenue, "tenant-transfer")
	brokerFunding.Purpose = charge.AllocationPurposeRentalIntermediationBroker
	brokerFunding.RelatedEntityID = "commission-1"
	agencyIntermediation := allocatedSlice("a-agency", "rent", 20000, charge.AllocationTypeRevenue, "tenant-transfer")
	agencyIntermediation.Purpose = charge.AllocationPurposeRentalIntermediationAgency
	c := newAllocatedPaidRentCharge(t, "charge-rent-waterfall", 300000, items, []charge.Allocation{
		owner, admin, brokerFunding, agencyIntermediation,
	})

	created, wasCreated, err := NewTransferService(&paymentGateTransferRepo{}).EnsureMonthlyForChargePaid(context.Background(), c, 10)
	require.NoError(t, err)
	require.True(t, wasCreated)
	require.NotNil(t, created.OwnerBalance)
	require.NotNil(t, created.EconomicBreakdown)
	assert.Equal(t, float64(3000), created.GrossRent)
	assert.Equal(t, float64(300), created.AdminFeeAmount)
	assert.Equal(t, float64(1500), created.NetAmount)
	assert.Equal(t, int64(150000), created.OwnerBalance.AvailableCents())
	assert.Equal(t, int64(30000), created.EconomicBreakdown.AdminFeeCents)
	assert.Equal(t, int64(120000), created.EconomicBreakdown.IntermediationRevenueCents)
	assert.Equal(t, int64(100000), created.EconomicBreakdown.CommissionFundedCents)
	assert.Equal(t, int64(150000), created.EconomicBreakdown.OwnerPayableCents)
	assert.Equal(t, int64(50000), created.EconomicBreakdown.AgencyRetainedCents)
	assert.Equal(t, int64(150000), created.EconomicBreakdown.AgencyRevenueCents)
	assert.Equal(t, int64(150000), created.EconomicBreakdown.DeductionsCents)
}

func TestTransferServiceCreatesOneOwnerTransferForManualSaleReceipt(t *testing.T) {
	repo := &chargeIdentityTransferRepo{}
	svc := NewTransferService(repo)
	receipt := transaction.New("contract-sale", transaction.TypeReceita, "Recebimento - Venda", 980_000, time.Date(2026, time.August, 29, 0, 0, 0, 0, time.UTC), "tenant-sale")
	receipt.ID = "receipt-sale"
	receipt.DealID = "deal-sale"
	receipt.EconomicBreakdown = &transaction.EconomicBreakdown{
		Flow:                   "sale",
		GrossCents:             98_000_000,
		CommissionPoolCents:    5_880_000,
		CommissionPayableCents: 3_528_000,
		AgencyRetainedCents:    2_352_000,
		OwnerPayableCents:      92_120_000,
	}
	paymentDate := time.Date(2026, time.August, 29, 17, 30, 0, 0, time.UTC)
	require.NoError(t, receipt.MarkPaid(paymentDate, "PIX"))

	created, wasCreated, err := svc.EnsureForManualSaleReceipt(context.Background(), receipt, "owner-sale", "property-sale")
	require.NoError(t, err)
	require.True(t, wasCreated)
	require.NotNil(t, created.OwnerBalance)
	require.NotNil(t, created.EconomicBreakdown)
	assert.Equal(t, receipt.ID, created.SourceTransactionID)
	assert.Equal(t, "sale", created.SettlementType)
	assert.Equal(t, int64(92_120_000), created.OwnerBalance.TotalCents)
	assert.Equal(t, int64(92_120_000), created.OwnerBalance.AvailableCents())
	assert.Equal(t, float64(921_200), created.NetAmount)
	assert.Equal(t, int64(3_528_000), created.EconomicBreakdown.CommissionPayableCents)
	assert.Equal(t, int64(2_352_000), created.EconomicBreakdown.AgencyRetainedCents)

	retry, wasCreated, err := svc.EnsureForManualSaleReceipt(context.Background(), receipt, "owner-sale", "property-sale")
	require.NoError(t, err)
	require.False(t, wasCreated)
	assert.Equal(t, created.ID, retry.ID)
	assert.Len(t, repo.items, 1)
}

func TestTransferServiceNeverClaimsManualReceiptTransferAsChargeLegacy(t *testing.T) {
	repo := &chargeIdentityTransferRepo{}
	manual := transfer.New(
		"contract-transfer",
		"owner-transfer",
		"property-transfer",
		"tenant-transfer",
		"2026-08",
		100,
		0,
		0,
		0,
		nil,
		"manual sale receipt",
	)
	manual.SourceTransactionID = "receipt-existing"
	require.NoError(t, repo.Create(context.Background(), manual))

	paid := newTransferGateCharge(t, charge.StatusPaid, centsPtr(10000))
	created, wasCreated, err := NewTransferService(repo).EnsureMonthlyForChargePaid(context.Background(), paid, 10)
	require.NoError(t, err)
	require.True(t, wasCreated)
	assert.Equal(t, paid.ID, created.SourceChargeID)
	assert.Empty(t, created.SourceTransactionID)
	assert.Len(t, repo.items, 2)
	assert.Equal(t, "receipt-existing", repo.items[0].SourceTransactionID)
	assert.Empty(t, repo.items[0].SourceChargeID)
}

func TestTransferServiceMakesSaleDiscountExplicit(t *testing.T) {
	items := []charge.LineItem{
		{ID: "owner", Description: "Saldo do proprietário", AmountCents: 9000, SourceType: "sale_owner_proceeds", EconomicOwnerID: "owner-transfer", SettlementMode: charge.SettlementModeCollection, AllocationType: charge.AllocationTypeOwnerPayable, RuleVersionID: "rule-v1"},
		{ID: "agency", Description: "Retenção da imobiliária", AmountCents: 1000, SourceType: "sale_agency_retained", EconomicOwnerID: "tenant-transfer", SettlementMode: charge.SettlementModeCollection, AllocationType: charge.AllocationTypeRevenue, RuleVersionID: "rule-v1"},
	}
	allocations := []charge.Allocation{
		allocatedSlice("a-owner-discount", "owner", 8100, charge.AllocationTypeOwnerPayable, "owner-transfer"),
		allocatedSlice("a-agency-discount", "agency", 900, charge.AllocationTypeRevenue, "tenant-transfer"),
	}
	c := newAllocatedPaidRentCharge(t, "charge-sale-discount", 10000, items, allocations)
	c.Type = charge.TypeSale
	c.Discount = valueobjects.NewMoney(1000)
	c.CalculateTotal()
	c.Payments[0].AmountCents = 9000
	paid := valueobjects.NewMoney(9000)
	c.PaidAmount = &paid

	created, _, err := NewTransferService(&paymentGateTransferRepo{}).EnsureMonthlyForChargePaid(context.Background(), c, 0)
	require.NoError(t, err)
	require.NotNil(t, created.EconomicBreakdown)
	assert.Equal(t, int64(10000), created.EconomicBreakdown.GrossCents)
	assert.Equal(t, int64(1000), created.EconomicBreakdown.DiscountCents)
	assert.Equal(t, int64(1000), created.EconomicBreakdown.DeductionsCents)
	assert.Equal(t, int64(8100), created.EconomicBreakdown.OwnerPayableCents)
	assert.Equal(t, int64(900), created.EconomicBreakdown.AgencyRetainedCents)
}

func TestTransferChargeHookEnsuresManagedBillsAndRetryPreservesConsumedReserve(t *testing.T) {
	items := []charge.LineItem{
		{
			ID: "rent-item", Description: "Aluguel", AmountCents: 500000,
			SourceType: "rental_base", SourceID: "rent-config", EconomicOwnerID: "owner-transfer",
			SettlementMode: charge.SettlementModeCollection, RuleVersionID: "rule-v1",
		},
		{
			ID: "condo-item", Description: "Condominio", AmountCents: 70000,
			SourceType: "rental_bundled_item", SourceID: "condominio", EconomicOwnerID: "owner-transfer",
			SettlementMode: charge.SettlementModeDirectLiquidation, RuleVersionID: "rule-v1",
		},
		{
			ID: "iptu-item", Description: "IPTU", AmountCents: 30000,
			SourceType: "rental_bundled_item", SourceID: "iptu", EconomicOwnerID: "owner-transfer",
			SettlementMode: charge.SettlementModeDirectLiquidation, RuleVersionID: "rule-v1",
		},
	}
	allocations := []charge.Allocation{
		allocatedSlice("allocation-owner-rent", "rent-item", 460000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
		allocatedSlice("allocation-admin-fee", "rent-item", 40000, charge.AllocationTypeRevenue, "tenant-transfer"),
		allocatedSlice("allocation-owner-condo", "condo-item", 70000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
		allocatedSlice("allocation-owner-iptu", "iptu-item", 30000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
	}
	c := newAllocatedPaidRentCharge(t, "charge-managed-bills", 600000, items, allocations)
	transferRepo := &chargeIdentityTransferRepo{}
	billRepo := newFakeManagedBillRepo()
	journalRepo := newFakeJournalRepo()
	billService := NewManagedBillService(billRepo, transferRepo).WithJournalService(NewJournalService(journalRepo))
	hook := NewTransferChargeHook(NewTransferService(transferRepo), nil).WithManagedBillService(billService)
	ctx := context.Background()

	if err := hook.EnsureMonthlyForChargePaid(ctx, c); err != nil {
		t.Fatalf("initial charge hook: %v", err)
	}
	ownerTransfer, err := transferRepo.GetBySourceChargeID(ctx, c.TenantID, c.ID)
	if err != nil || ownerTransfer == nil {
		t.Fatalf("load transfer: transfer=%+v err=%v", ownerTransfer, err)
	}
	if ownerTransfer.OwnerBalance == nil || ownerTransfer.OwnerBalance.TotalCents != 560000 ||
		ownerTransfer.OwnerBalance.ReservedCents != 100000 || ownerTransfer.OwnerBalance.AvailableCents() != 460000 {
		t.Fatalf("initial owner balance: %+v", ownerTransfer.OwnerBalance)
	}
	if len(billRepo.byID) != 2 {
		t.Fatalf("hook created %d managed bills, want 2", len(billRepo.byID))
	}
	var condoBill *managed_bill.ManagedBill
	for _, bill := range billRepo.byID {
		if bill.SourceChargeItemID == "condo-item" {
			condoBill = cloneManagedBill(bill)
		}
	}
	if condoBill == nil || condoBill.AmountCents != 70000 {
		t.Fatalf("condo managed bill not derived from allocations: %+v", condoBill)
	}
	approved, err := billService.Approve(ctx, c.TenantID, condoBill.ID, "approver-1", condoBill.Version)
	if err != nil {
		t.Fatalf("approve condo: %v", err)
	}
	if _, err := billService.Pay(ctx, c.TenantID, condoBill.ID, "payer-1", approved.Bill.Version, "pix-condo", "", time.Time{}); err != nil {
		t.Fatalf("pay condo: %v", err)
	}

	if err := hook.EnsureMonthlyForChargePaid(ctx, c); err != nil {
		t.Fatalf("charge hook retry after managed-bill payment: %v", err)
	}
	retryTransfer, err := transferRepo.GetBySourceChargeID(ctx, c.TenantID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retryTransfer.OwnerBalance.TotalCents != 490000 || retryTransfer.OwnerBalance.ReservedCents != 30000 ||
		retryTransfer.OwnerBalance.AvailableCents() != 460000 || len(retryTransfer.OwnerBalanceMovements) != 1 {
		t.Fatalf("charge retry restored consumed reserve: transfer=%+v balance=%+v", retryTransfer, retryTransfer.OwnerBalance)
	}
	if retryTransfer.OwnerBalanceBaseTotalCents != 560000 || retryTransfer.OwnerBalanceBaseReservedCents != 100000 || len(billRepo.byID) != 2 {
		t.Fatalf("retry changed economic basis or duplicated bills: transfer=%+v bills=%d", retryTransfer, len(billRepo.byID))
	}
}

func TestPaidChargeTransferCompositionRejectsRentOrFeeBaseChangeAfterManagedBillMovement(t *testing.T) {
	item := transfer.New("contract-1", "owner-1", "property-1", "tenant-1", "2026-08", 5000, 8, 0, 0, nil, "")
	original := paidChargeTransferComposition{
		grossRentCents: 500000, adminFeePercent: 8, adminFeeCents: 40000,
		ownerPayableCents: 560000, ownerReservedCents: 100000, allocationAware: true,
	}
	if err := original.applyTo(item); err != nil {
		t.Fatal(err)
	}
	if _, err := item.ApplyOwnerBalanceMovement(transfer.OwnerBalanceMovement{
		ID: "payment-1", ManagedBillID: "bill-1", Type: transfer.OwnerBalanceMovementConsumeReserved,
		AmountCents: 70000, At: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	changedBase := original
	changedBase.grossRentCents = 510000
	changedBase.adminFeeCents = 50000
	changedBase.adminFeePercent = float64(changedBase.adminFeeCents) * 100 / float64(changedBase.grossRentCents)
	if _, err := changedBase.matches(item); !errors.Is(err, transfer.ErrOwnerBalanceBaseChanged) {
		t.Fatalf("changed rent/admin base: got %v, want ErrOwnerBalanceBaseChanged", err)
	}
}

func TestTransferServiceUsesAllOwnerSettlementBuckets(t *testing.T) {
	items := []charge.LineItem{
		{ID: "base", Description: "Aluguel", AmountCents: 10000, SourceType: "rental_base", EconomicOwnerID: "owner-transfer", SettlementMode: charge.SettlementModeCollection, RuleVersionID: "rule-v1"},
		{ID: "refund", Description: "Reembolso", AmountCents: 1000, SourceType: "rental_bundled_item", EconomicOwnerID: "owner-transfer", SettlementMode: charge.SettlementModeReimbursement, RuleVersionID: "rule-v1"},
		{ID: "direct", Description: "Liquidação", AmountCents: 1000, SourceType: "rental_bundled_item", EconomicOwnerID: "owner-transfer", SettlementMode: charge.SettlementModeDirectLiquidation, RuleVersionID: "rule-v1"},
		{ID: "offset", Description: "Compensação", AmountCents: 1000, SourceType: "rental_bundled_item", EconomicOwnerID: "owner-transfer", SettlementMode: charge.SettlementModeOffset, RuleVersionID: "rule-v1"},
	}
	allocations := []charge.Allocation{
		allocatedSlice("a-base", "base", 10000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
		allocatedSlice("a-refund", "refund", 1000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
		allocatedSlice("a-direct", "direct", 1000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
		allocatedSlice("a-offset", "offset", 1000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
	}
	c := newAllocatedPaidRentCharge(t, "charge-settlement-modes", 13000, items, allocations)

	created, _, err := NewTransferService(&paymentGateTransferRepo{}).EnsureMonthlyForChargePaid(context.Background(), c, 8)
	if err != nil {
		t.Fatalf("settlement-mode transfer: %v", err)
	}
	if created.GrossRent != 100 || created.AdminFeeAmount != 0 || created.OwnerBalance == nil {
		t.Fatalf("unexpected transfer composition: %+v", created)
	}
	if created.OwnerBalance.TotalCents != 13000 || created.OwnerBalance.ReservedCents != 2000 || created.OwnerBalance.AvailableCents() != 11000 || created.NetAmount != 110 {
		t.Fatalf("settlement modes were not bucketed correctly: transfer=%+v balance=%+v", created, created.OwnerBalance)
	}
}

func TestTransferServiceExplicitTenantAdminFeeDoesNotDoubleRetain(t *testing.T) {
	items := []charge.LineItem{
		{ID: "rent", Description: "Aluguel", AmountCents: 500000, SourceType: "rental_base", EconomicOwnerID: "owner-transfer", SettlementMode: charge.SettlementModeCollection, RuleVersionID: "rule-v1"},
		{ID: "admin", Description: "Taxa de administração", AmountCents: 40000, SourceType: "rental_bundled_item", SourceID: "admin_fee", EconomicOwnerID: "tenant-transfer", SettlementMode: charge.SettlementModeCollection, RuleVersionID: "rule-v1"},
	}
	allocations := []charge.Allocation{
		allocatedSlice("a-owner", "rent", 500000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
		allocatedSlice("a-explicit-fee", "admin", 40000, charge.AllocationTypeRevenue, "tenant-transfer"),
	}
	c := newAllocatedPaidRentCharge(t, "charge-explicit-admin", 540000, items, allocations)
	c.SourceEventID = "rental.config-extra.month.2026-08.r0"

	created, _, err := NewTransferService(&paymentGateTransferRepo{}).EnsureMonthlyForChargePaid(context.Background(), c, 8)
	if err != nil {
		t.Fatalf("explicit admin-fee transfer: %v", err)
	}
	if created.GrossRent != 5000 || created.AdminFeePercent != 0 || created.AdminFeeAmount != 0 || created.NetAmount != 5000 {
		t.Fatalf("explicit admin fee was retained twice: %+v", created)
	}
	if created.OwnerBalance == nil || created.OwnerBalance.TotalCents != 500000 || created.OwnerBalance.AvailableCents() != 500000 {
		t.Fatalf("wrong explicit-fee owner balance: %+v", created.OwnerBalance)
	}
}

func TestTransferServiceExplainsIncludedAdministrationFromGeneratedRentalCharge(t *testing.T) {
	for _, intermediation := range []bool{false, true} {
		t.Run(fmt.Sprintf("intermediation_%t", intermediation), func(t *testing.T) {
			cfg := &rental_charge_config.RentalChargeConfig{
				ID: "config-included-admin", TenantID: "tenant-transfer", OwnerID: "owner-transfer",
				MonthlyAmount: 320000, AdminFeePercent: 10,
				BundledItems: []rental_charge_config.BundledItem{
					{Key: "condominio", Description: "Condomínio", AmountCents: 65000, Enabled: true},
					{Key: "iptu", Description: "IPTU", AmountCents: 18000, Enabled: true},
					{Key: "seguro_fianca", Description: "Seguro fiança", AmountCents: 9800, Enabled: true},
				},
			}
			c, err := charge.NewWithLineItems("contract-transfer", "payer-transfer", cfg.OwnerID, "property-transfer", cfg.TenantID,
				charge.TypeRent, "Aluguel + Encargos", "2026-10", 4128, time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC),
				buildBundledLineItems(cfg, 412800, 412800, cfg.AdminFeePercent))
			require.NoError(t, err)
			c.SourceEventID = "rental.config-included-admin.month.2026-10.r0"
			method, allocations, err := NewEconomicAllocationService(nil).BuildPaymentAllocations(context.Background(), c, "payment-allocated", 412800)
			require.NoError(t, err)
			fundedCents, retainedIntermediationCents := int64(0), int64(0)
			if intermediation {
				fundedCents, retainedIntermediationCents = 100000, 20000
				baseID := c.LineItems[0].ID
				for i := range allocations {
					if allocations[i].ChargeItemID == baseID {
						allocations[i].AmountCents -= fundedCents + retainedIntermediationCents
					}
				}
				broker := allocatedSlice("included-broker", baseID, fundedCents, charge.AllocationTypeRevenue, cfg.TenantID)
				broker.Purpose, broker.RelatedEntityID = charge.AllocationPurposeRentalIntermediationBroker, "commission-included"
				agency := allocatedSlice("included-agency", baseID, retainedIntermediationCents, charge.AllocationTypeRevenue, cfg.TenantID)
				agency.Purpose = charge.AllocationPurposeRentalIntermediationAgency
				allocations = append(allocations, broker, agency)
			}
			_, err = c.ApplyPaymentWithAllocations("payment-allocated", 4128, "PIX", time.Now(), method, allocations)
			require.NoError(t, err)
			repo := &paymentGateTransferRepo{}
			svc := NewTransferService(repo)
			created, wasCreated, err := svc.EnsureMonthlyForChargePaid(context.Background(), c, 10)
			require.NoError(t, err)
			require.True(t, wasCreated)
			require.NotNil(t, created.OwnerBalance)
			require.NotNil(t, created.EconomicBreakdown)
			assert.Equal(t, float64(3200), created.GrossRent)
			assert.Equal(t, float64(10), created.AdminFeePercent)
			assert.Equal(t, float64(320), created.AdminFeeAmount)
			assert.Equal(t, int64(320000), created.EconomicBreakdown.GrossCents)
			assert.Equal(t, int64(32000), created.EconomicBreakdown.AdminFeeCents)
			assert.Equal(t, int64(32000)+retainedIntermediationCents, created.EconomicBreakdown.AgencyRetainedCents)
			assert.Equal(t, fundedCents, created.EconomicBreakdown.CommissionFundedCents)
			assert.Equal(t, int64(32000)+fundedCents+retainedIntermediationCents, created.EconomicBreakdown.AgencyRevenueCents)
			assert.Equal(t, int64(380800)-fundedCents-retainedIntermediationCents, created.OwnerBalance.TotalCents)
			assert.Equal(t, int64(83000), created.OwnerBalance.ReservedCents)
			assert.Equal(t, int64(297800)-fundedCents-retainedIntermediationCents, created.OwnerBalance.AvailableCents())
			retry, wasCreated, err := svc.EnsureMonthlyForChargePaid(context.Background(), c, 10)
			require.NoError(t, err)
			assert.False(t, wasCreated)
			assert.Equal(t, created.ID, retry.ID)
			assert.Equal(t, 1, repo.createCalls)
		})
	}
}

func TestTransferServiceRejectsPartialAllocationCoverage(t *testing.T) {
	items := []charge.LineItem{
		{ID: "rent", Description: "Aluguel", AmountCents: 10000, SourceType: "rental_base", EconomicOwnerID: "owner-transfer", SettlementMode: charge.SettlementModeCollection, RuleVersionID: "rule-v1"},
	}
	allocations := []charge.Allocation{
		allocatedSlice("a-owner", "rent", 9000, charge.AllocationTypeOwnerPayable, "owner-transfer"),
	}
	c := newAllocatedPaidRentCharge(t, "charge-partial-allocation", 10000, items, allocations)
	repo := &paymentGateTransferRepo{}

	created, wasCreated, err := NewTransferService(repo).EnsureMonthlyForChargePaid(context.Background(), c, 8)
	if err == nil || created != nil || wasCreated {
		t.Fatalf("partial allocation coverage must fail closed: transfer=%+v created=%v err=%v", created, wasCreated, err)
	}
	if repo.lookupCalls != 0 || repo.createCalls != 0 {
		t.Fatalf("invalid allocations reached repository: lookups=%d creates=%d", repo.lookupCalls, repo.createCalls)
	}
}

func TestTransferServicePreservesLegacyRentFallback(t *testing.T) {
	c := newTransferGateCharge(t, charge.StatusPaid, centsPtr(10000))
	repo := &paymentGateTransferRepo{}

	created, _, err := NewTransferService(repo).EnsureMonthlyForChargePaid(context.Background(), c, 8)
	if err != nil {
		t.Fatalf("legacy fallback: %v", err)
	}
	if created.OwnerBalance != nil || created.GrossRent != 100 || created.AdminFeePercent != 8 || created.AdminFeeAmount != 8 || created.NetAmount != 92 {
		t.Fatalf("legacy transfer semantics changed: %+v", created)
	}
}

func newPaidSaleTransferCharge(t *testing.T, id string, amount float64) *charge.Charge {
	t.Helper()
	c, err := charge.New(
		"sale-contract",
		"sale-payer",
		"sale-owner",
		"sale-property",
		"sale-tenant",
		charge.TypeSale,
		"Parcela da venda",
		"2026-01",
		amount,
		time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("create SALE charge fixture: %v", err)
	}
	c.ID = id
	c.Status = charge.StatusPaid
	paid := c.TotalAmount
	c.PaidAmount = &paid
	return c
}

func TestTransferServiceKeepsDistinctSameMonthSaleInstallmentsIdempotent(t *testing.T) {
	repo := &chargeIdentityTransferRepo{}
	svc := NewTransferService(repo)
	firstCharge := newPaidSaleTransferCharge(t, "sale-charge-1", 50)
	secondCharge := newPaidSaleTransferCharge(t, "sale-charge-2", 75)

	first, created, err := svc.EnsureMonthlyForChargePaid(context.Background(), firstCharge, 10)
	if err != nil || !created {
		t.Fatalf("first installment: transfer=%+v created=%v err=%v", first, created, err)
	}
	firstRetry, created, err := svc.EnsureMonthlyForChargePaid(context.Background(), firstCharge, 10)
	if err != nil || created || firstRetry.ID != first.ID {
		t.Fatalf("first retry was not canonical: first=%s retry=%+v created=%v err=%v", first.ID, firstRetry, created, err)
	}

	second, created, err := svc.EnsureMonthlyForChargePaid(context.Background(), secondCharge, 10)
	if err != nil || !created {
		t.Fatalf("second installment: transfer=%+v created=%v err=%v", second, created, err)
	}
	secondRetry, created, err := svc.EnsureMonthlyForChargePaid(context.Background(), secondCharge, 10)
	if err != nil || created || secondRetry.ID != second.ID {
		t.Fatalf("second retry was not canonical: second=%s retry=%+v created=%v err=%v", second.ID, secondRetry, created, err)
	}

	if len(repo.items) != 2 {
		t.Fatalf("same-month SALE installments collapsed: got %d transfers, want 2", len(repo.items))
	}
	if first.SourceChargeID != firstCharge.ID || second.SourceChargeID != secondCharge.ID {
		t.Fatalf("wrong source identities: first=%q second=%q", first.SourceChargeID, second.SourceChargeID)
	}
	if first.ReferenceMonth != second.ReferenceMonth || first.GrossRent != 50 || second.GrossRent != 75 {
		t.Fatalf("installment values/month diverged: first=%+v second=%+v", first, second)
	}
	if first.AdminFeePercent != 0 || first.AdminFeeAmount != 0 || first.NetAmount != 50 || first.OwnerBalance != nil ||
		second.AdminFeePercent != 0 || second.AdminFeeAmount != 0 || second.NetAmount != 75 || second.OwnerBalance != nil {
		t.Fatalf("SALE installments must not retain the rental admin fee: first=%+v second=%+v", first, second)
	}
}

func TestTransferServiceClaimsLegacyMonthlyTransferForRentRetry(t *testing.T) {
	legacy := transfer.New(
		"contract-transfer",
		"owner-transfer",
		"property-transfer",
		"tenant-transfer",
		"2026-08",
		100,
		10,
		0,
		0,
		nil,
		"legacy monthly transfer",
	)
	repo := &chargeIdentityTransferRepo{items: []*transfer.OwnerTransfer{cloneOwnerTransfer(legacy)}}
	svc := NewTransferService(repo)
	c := newTransferGateCharge(t, charge.StatusPaid, centsPtr(10000))

	claimed, created, err := svc.EnsureMonthlyForChargePaid(context.Background(), c, 10)
	if err != nil || created {
		t.Fatalf("legacy claim: transfer=%+v created=%v err=%v", claimed, created, err)
	}
	if claimed.ID != legacy.ID || claimed.SourceChargeID != c.ID || len(repo.items) != 1 {
		t.Fatalf("legacy row was not backfilled in place: claimed=%+v rows=%d", claimed, len(repo.items))
	}
	retry, created, err := svc.EnsureMonthlyForChargePaid(context.Background(), c, 10)
	if err != nil || created || retry.ID != legacy.ID || len(repo.items) != 1 {
		t.Fatalf("legacy retry was not canonical: retry=%+v created=%v err=%v rows=%d", retry, created, err, len(repo.items))
	}
}

func TestTransferServiceAppliesDeductionOnceToDeterministicSameMonthTransfer(t *testing.T) {
	baseTime := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	later := transfer.New("sale-contract", "sale-owner", "sale-property", "sale-tenant", "2026-01", 75, 0, 0, 0, nil, "later installment")
	later.ID = "transfer-later"
	later.SourceChargeID = "sale-charge-2"
	later.CreatedAt = baseTime.Add(time.Hour)
	earlier := transfer.New("sale-contract", "sale-owner", "sale-property", "sale-tenant", "2026-01", 50, 0, 0, 0, nil, "earlier installment")
	earlier.ID = "transfer-earlier"
	earlier.SourceChargeID = "sale-charge-1"
	earlier.CreatedAt = baseTime
	// Deliberately reverse storage order: selection must use created_at/id,
	// never whichever row the database happens to return first.
	repo := &chargeIdentityTransferRepo{items: []*transfer.OwnerTransfer{cloneOwnerTransfer(later), cloneOwnerTransfer(earlier)}}
	svc := NewTransferService(repo)
	ctx := context.Background()

	if err := svc.ApplyDeduction(ctx, "sale-tenant", "sale-contract", "2026-01", "UTILITY", "Energia", "utility.ref-1", 12.50); err != nil {
		t.Fatalf("first deduction: %v", err)
	}
	storedEarlier, _ := repo.GetByID(ctx, "sale-tenant", earlier.ID)
	storedLater, _ := repo.GetByID(ctx, "sale-tenant", later.ID)
	if storedEarlier.OtherDeductions != 12.50 || len(storedEarlier.DeductionDetails) != 1 {
		t.Fatalf("oldest pending transfer did not receive deduction exactly once: %+v", storedEarlier)
	}
	if storedLater.OtherDeductions != 0 || len(storedLater.DeductionDetails) != 0 {
		t.Fatalf("later same-month installment was mutated: %+v", storedLater)
	}

	// Even after the canonical transfer advances, the same reference must not
	// migrate to the other pending installment on retry.
	for index, item := range repo.items {
		if item.ID == earlier.ID {
			repo.items[index].Status = transfer.StatusProcessed
		}
	}
	if err := svc.ApplyDeduction(ctx, "sale-tenant", "sale-contract", "2026-01", "UTILITY", "Energia retry", "utility.ref-1", 12.50); err != nil {
		t.Fatalf("idempotent retry after lifecycle advance: %v", err)
	}
	storedEarlier, _ = repo.GetByID(ctx, "sale-tenant", earlier.ID)
	storedLater, _ = repo.GetByID(ctx, "sale-tenant", later.ID)
	if storedEarlier.OtherDeductions != 12.50 || len(storedEarlier.DeductionDetails) != 1 || storedLater.OtherDeductions != 0 {
		t.Fatalf("retry duplicated or moved deduction: earlier=%+v later=%+v", storedEarlier, storedLater)
	}
	if err := svc.ApplyDeduction(ctx, "sale-tenant", "sale-contract", "2026-01", "UTILITY", "payload divergente", "utility.ref-1", 99); err == nil {
		t.Fatal("reusing deduction reference_id with a different amount must fail")
	}

	// A genuinely new reference safely selects the remaining PENDING row.
	if err := svc.ApplyDeduction(ctx, "sale-tenant", "sale-contract", "2026-01", "MAINTENANCE", "Reparo", "maintenance.ref-2", 7.50); err != nil {
		t.Fatalf("second distinct deduction: %v", err)
	}
	storedLater, _ = repo.GetByID(ctx, "sale-tenant", later.ID)
	if storedLater.MaintenanceDeductions != 7.50 || len(storedLater.DeductionDetails) != 1 {
		t.Fatalf("new reference did not select remaining pending transfer: %+v", storedLater)
	}
}

func TestTransferServiceDeductionKeepsLegacyRentCompatibility(t *testing.T) {
	legacy := transfer.New("rent-contract", "rent-owner", "rent-property", "rent-tenant", "2026-08", 100, 10, 0, 0, nil, "legacy RENT")
	legacy.ID = "legacy-rent-transfer"
	repo := &chargeIdentityTransferRepo{items: []*transfer.OwnerTransfer{cloneOwnerTransfer(legacy)}}
	svc := NewTransferService(repo)

	for attempt := 0; attempt < 2; attempt++ {
		if err := svc.ApplyDeduction(context.Background(), "rent-tenant", "rent-contract", "2026-08", "UTILITY", "Água", "utility.rent-1", 10); err != nil {
			t.Fatalf("legacy RENT attempt %d: %v", attempt+1, err)
		}
	}
	stored, _ := repo.GetByID(context.Background(), "rent-tenant", legacy.ID)
	if stored.SourceChargeID != "" || stored.OtherDeductions != 10 || len(stored.DeductionDetails) != 1 {
		t.Fatalf("legacy RENT deduction lost compatibility/idempotency: %+v", stored)
	}
}
