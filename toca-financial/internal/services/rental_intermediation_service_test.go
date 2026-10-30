package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
)

type rentalFundingConfigRepo struct {
	rental_charge_config.Repository
	item *rental_charge_config.RentalChargeConfig
}

func (r *rentalFundingConfigRepo) GetByContractID(_ context.Context, tenantID, contractID string) (*rental_charge_config.RentalChargeConfig, error) {
	if r.item == nil || r.item.TenantID != tenantID || r.item.ContractID != contractID {
		return nil, nil
	}
	return r.item, nil
}

func (r *rentalFundingConfigRepo) Update(_ context.Context, item *rental_charge_config.RentalChargeConfig) error {
	r.item = item
	return nil
}

type rentalFundingCommissionRepo struct {
	CommissionRepository
	item            *commission.Commission
	createErr       error
	transitionCalls int
}

func (r *rentalFundingCommissionRepo) Create(_ context.Context, item *commission.Commission) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.item = item
	return nil
}

func (r *rentalFundingCommissionRepo) GetBySourceEventID(_ context.Context, tenantID, sourceEventID string) (*commission.Commission, error) {
	if r.item == nil || r.item.TenantID != tenantID || r.item.SourceEventID != sourceEventID {
		return nil, nil
	}
	return r.item, nil
}

func (r *rentalFundingCommissionRepo) GetByID(_ context.Context, tenantID, id string) (*commission.Commission, error) {
	if r.item == nil || r.item.TenantID != tenantID || r.item.ID != id {
		return nil, nil
	}
	return r.item, nil
}

func (r *rentalFundingCommissionRepo) Update(_ context.Context, item *commission.Commission) error {
	r.item = item
	return nil
}

func (r *rentalFundingCommissionRepo) UpdateIfStatus(_ context.Context, item *commission.Commission, _ commission.Status, _ time.Time) error {
	r.item = item
	r.transitionCalls++
	return nil
}

type rentalFundingPayoutRepo struct {
	payout_order.Repository
	item            *payout_order.PayoutOrder
	transitionCalls int
}

func (r *rentalFundingPayoutRepo) GetByID(_ context.Context, tenantID, id string) (*payout_order.PayoutOrder, error) {
	if r.item == nil || r.item.TenantID != tenantID || r.item.ID != id {
		return nil, nil
	}
	return r.item, nil
}

func (r *rentalFundingPayoutRepo) UpdateIfStatus(_ context.Context, item *payout_order.PayoutOrder, _ payout_order.Status, _ time.Time) error {
	r.item = item
	r.transitionCalls++
	return nil
}

func TestLegacyRentalObligationsGainFundingGateOnSafeRedrive(t *testing.T) {
	t.Parallel()

	legacyCommission := commission.NewWithRole("deal-legacy", "broker-legacy", "contract-legacy", "fechamento", 100, 5000, "tenant-legacy")
	legacyCommission.SourceEventID = "event-legacy"
	commissionRepo := &rentalFundingCommissionRepo{
		item: legacyCommission, createErr: commission.ErrCommissionAlreadyExists,
	}
	commissionService := NewCommissionService(commissionRepo)
	upgraded, err := commissionService.CreateRentalWithRoleFromEvent(
		context.Background(), "event-legacy", "deal-legacy", "broker-legacy", "contract-legacy", "fechamento",
		100, 5000, "tenant-legacy", "owner-legacy", "customer-legacy", "property-legacy", "plan-legacy", 500000,
	)
	if err != nil {
		t.Fatal(err)
	}
	if upgraded.FundingRequiredCents != 500000 || commissionRepo.transitionCalls != 1 {
		t.Fatalf("commission redrive did not persist gate: commission=%+v transitions=%d", upgraded, commissionRepo.transitionCalls)
	}

	legacyPayout, err := payout_order.New(
		"tenant-legacy", payout_order.SourceManual, "contract-legacy", payout_order.RecipientBroker,
		"broker-legacy", "Corretor", 500000, "system",
	)
	if err != nil {
		t.Fatal(err)
	}
	payoutRepo := &rentalFundingPayoutRepo{item: legacyPayout}
	payoutService := NewPayoutOrchestrationService(payoutRepo, nil, nil)
	upgradedPayout, err := payoutService.ensurePayoutFundingGate(context.Background(), legacyPayout, 500000)
	if err != nil {
		t.Fatal(err)
	}
	if upgradedPayout.Status != payout_order.StatusDraft || upgradedPayout.FundingRequiredCents != 500000 || payoutRepo.transitionCalls != 1 {
		t.Fatalf("payout redrive did not persist gate: payout=%+v transitions=%d", upgradedPayout, payoutRepo.transitionCalls)
	}
}

func TestLegacyRentalRedriveRejectsAlreadyApprovedObligation(t *testing.T) {
	t.Parallel()

	legacy := commission.NewWithRole("deal-legacy", "broker-legacy", "contract-legacy", "fechamento", 100, 5000, "tenant-legacy")
	legacy.SourceEventID = "event-legacy"
	legacy.Status = commission.StatusApproved
	repo := &rentalFundingCommissionRepo{item: legacy, createErr: commission.ErrCommissionAlreadyExists}
	_, err := NewCommissionService(repo).CreateRentalWithRoleFromEvent(
		context.Background(), "event-legacy", "deal-legacy", "broker-legacy", "contract-legacy", "fechamento",
		100, 5000, "tenant-legacy", "", "", "", "plan-legacy", 500000,
	)
	if !errors.Is(err, commission.ErrFundingGateConflict) {
		t.Fatalf("approved legacy redrive = %v, want ErrFundingGateConflict", err)
	}
	if repo.transitionCalls != 0 || legacy.FundingRequiredCents != 0 {
		t.Fatalf("unsafe redrive mutated approved obligation: commission=%+v transitions=%d", legacy, repo.transitionCalls)
	}
}

func TestRentalIntermediationWaterfallCarriesFirstProRataIntoNextMonth(t *testing.T) {
	t.Parallel()

	const (
		tenantID     = "tenant-rent"
		contractID   = "contract-rent"
		ownerID      = "owner-rent"
		commissionID = "commission-rent"
		payoutID     = "payout-rent"
	)
	cfg := &rental_charge_config.RentalChargeConfig{
		ID: "config-rent", TenantID: tenantID, ContractID: contractID, OwnerID: ownerID,
	}
	if _, err := cfg.FreezeIntermediationPlan(rental_charge_config.IntermediationPlan{
		DealID: "deal-rent", PlanHash: "plan-rent", TotalAmountCents: 500000,
		Shares: []rental_charge_config.IntermediationShare{{
			BrokerID: "broker-rent", Role: "fechamento", TargetAmountCents: 500000,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.LinkIntermediationCommission("broker-rent", "fechamento", commissionID); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.LinkIntermediationPayout("broker-rent", "fechamento", payoutID); err != nil {
		t.Fatal(err)
	}

	comm := commission.NewWithRole("deal-rent", "broker-rent", contractID, "fechamento", 100, 5000, tenantID)
	comm.ID = commissionID
	comm.FundingRequiredCents = 500000
	order, err := payout_order.New(tenantID, payout_order.SourceManual, contractID, payout_order.RecipientBroker, "broker-rent", "Corretor", 500000, "system")
	if err != nil {
		t.Fatal(err)
	}
	order.ID = payoutID
	order.FundingRequiredCents = 500000
	order.Status = payout_order.StatusDraft

	configRepo := &rentalFundingConfigRepo{item: cfg}
	commissionRepo := &rentalFundingCommissionRepo{item: comm}
	payoutRepo := &rentalFundingPayoutRepo{item: order}
	intermediation := NewRentalIntermediationService(
		configRepo,
		NewCommissionService(commissionRepo),
		payoutRepo,
	)
	allocator := NewEconomicAllocationService(fixedAllocationSettings{adminFeePct: 10}).WithRentalIntermediation(intermediation)

	first := newRentalBaseCharge(t, "charge-aug", contractID, tenantID, ownerID, "2026-08", 300000)
	_, firstAllocations, err := allocator.BuildPaymentAllocations(context.Background(), first, "payment-aug", 300000)
	if err != nil {
		t.Fatal(err)
	}
	firstPurposes := allocationTotalsByPurpose(firstAllocations)
	if firstPurposes[charge.AllocationPurposeAdminFee] != 30000 ||
		firstPurposes[charge.AllocationPurposeRentalIntermediationBroker] != 270000 ||
		firstPurposes[charge.AllocationPurposeOwnerRent] != 0 {
		t.Fatalf("first pro-rata waterfall = %+v", firstPurposes)
	}
	if commissionRepo.item.FundedAmountCents != 270000 || payoutRepo.item.FundedAmountCents != 270000 || payoutRepo.item.Status != payout_order.StatusDraft {
		t.Fatalf("first pro-rata aggregates: commission=%+v payout=%+v", commissionRepo.item, payoutRepo.item)
	}

	second := newRentalBaseCharge(t, "charge-sep", contractID, tenantID, ownerID, "2026-09", 500000)
	_, secondAllocations, err := allocator.BuildPaymentAllocations(context.Background(), second, "payment-sep", 500000)
	if err != nil {
		t.Fatal(err)
	}
	secondPurposes := allocationTotalsByPurpose(secondAllocations)
	if secondPurposes[charge.AllocationPurposeAdminFee] != 50000 ||
		secondPurposes[charge.AllocationPurposeRentalIntermediationBroker] != 230000 ||
		secondPurposes[charge.AllocationPurposeOwnerRent] != 220000 {
		t.Fatalf("second-month waterfall = %+v", secondPurposes)
	}
	if commissionRepo.item.FundedAmountCents != 500000 || commissionRepo.item.FundingOutstandingCents() != 0 {
		t.Fatalf("commission carry did not close: %+v", commissionRepo.item)
	}
	if payoutRepo.item.FundedAmountCents != 500000 || payoutRepo.item.Status != payout_order.StatusPendingReview {
		t.Fatalf("fully funded payout was not released for review: %+v", payoutRepo.item)
	}
	if configRepo.item.IntermediationOutstandingCents() != 0 || len(configRepo.item.IntermediationPlan.FundingEntries) != 2 {
		t.Fatalf("config funding provenance = %+v", configRepo.item.IntermediationPlan)
	}
}

func newRentalBaseCharge(t *testing.T, id, contractID, tenantID, ownerID, referenceMonth string, amountCents int64) *charge.Charge {
	t.Helper()
	c, err := charge.NewWithLineItems(
		contractID, "tenant-party", ownerID, "property-rent", tenantID,
		charge.TypeRent, "Aluguel", referenceMonth, float64(amountCents)/100,
		time.Date(2026, time.August, 20, 0, 0, 0, 0, time.UTC),
		[]charge.LineItem{{
			Description: "Aluguel", AmountCents: amountCents, SourceType: "rental_base",
			SourceID: "config-rent", EconomicOwnerID: ownerID, SettlementMode: charge.SettlementModeCollection,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	c.ID = id
	return c
}

func allocationTotalsByPurpose(allocations []charge.Allocation) map[charge.AllocationPurpose]int64 {
	totals := make(map[charge.AllocationPurpose]int64)
	for _, allocation := range allocations {
		totals[allocation.Purpose] += allocation.AmountCents
	}
	return totals
}
