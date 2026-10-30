package services

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
)

func TestManualRentReceiptFundsFrozenIntermediation(t *testing.T) {
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

	commissionRepo := &rentalFundingCommissionRepo{item: comm}
	payoutRepo := &rentalFundingPayoutRepo{item: order}
	intermediation := NewRentalIntermediationService(
		&rentalFundingConfigRepo{item: cfg},
		NewCommissionService(commissionRepo),
		payoutRepo,
	)

	svc := &ManualReceiptSettlementService{rentalIntermediation: intermediation}

	paidAt := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	receipt := &transaction.Transaction{
		ID: "receipt-rent", TenantID: tenantID, ContractID: contractID, DealID: "deal-rent",
		Date: paidAt, PaymentDate: &paidAt,
		EconomicBreakdown: &transaction.EconomicBreakdown{
			Flow: "rent", GrossCents: 500000, AdminFeeCents: 50000, OwnerPayableCents: 450000,
		},
	}

	if err := svc.fundRentIntermediation(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}

	// available = 500000 - 50000 (10% admin) = 450000, all of which funds the
	// 500000 target.
	if commissionRepo.item.FundedAmountCents != 450000 {
		t.Fatalf("commission funded = %d, want 450000", commissionRepo.item.FundedAmountCents)
	}
	if payoutRepo.item.FundedAmountCents != 450000 {
		t.Fatalf("payout funded = %d, want 450000", payoutRepo.item.FundedAmountCents)
	}

	b := receipt.EconomicBreakdown
	if b.OwnerPayableCents != 0 {
		t.Fatalf("owner payable = %d, want 0", b.OwnerPayableCents)
	}
	if b.AgencyRetainedCents != 0 {
		t.Fatalf("agency retained = %d, want 0", b.AgencyRetainedCents)
	}
	if b.CommissionPoolCents != 450000 {
		t.Fatalf("commission pool = %d, want 450000", b.CommissionPoolCents)
	}
	if b.CommissionPayableCents != 450000 || b.CommissionFundedCents != 450000 || !b.CommissionFundingKnown {
		t.Fatalf("commission payable = %d, want 450000", b.CommissionPayableCents)
	}
	if b.AdminFeeCents != 50000 {
		t.Fatalf("admin fee = %d, want 50000", b.AdminFeeCents)
	}
	// A retry starts from the already funded composition. It must not
	// subtract the same intermediation from the owner's balance twice.
	want := *receipt.EconomicBreakdown
	if err := svc.fundRentIntermediation(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, *receipt.EconomicBreakdown) || len(cfg.IntermediationPlan.FundingEntries) != 1 {
		t.Fatalf("retry changed economic composition: before=%+v after=%+v", want, receipt.EconomicBreakdown)
	}
}

func TestManualRentFundingExcludesDiscountedCash(t *testing.T) {
	cfg := &rental_charge_config.RentalChargeConfig{ID: "config", TenantID: "tenant", ContractID: "contract", OwnerID: "owner"}
	if _, err := cfg.FreezeIntermediationPlan(rental_charge_config.IntermediationPlan{PlanHash: "plan", TotalAmountCents: 500000}); err != nil {
		t.Fatal(err)
	}
	svc := &ManualReceiptSettlementService{rentalIntermediation: NewRentalIntermediationService(&rentalFundingConfigRepo{item: cfg}, NewCommissionService(&rentalFundingCommissionRepo{}), &rentalFundingPayoutRepo{})}
	receipt := &transaction.Transaction{
		ID: "discounted-receipt", TenantID: "tenant", ContractID: "contract", DealID: "deal", Date: time.Now(),
		EconomicBreakdown: &transaction.EconomicBreakdown{
			Flow: "rent", GrossCents: 500000, DiscountCents: 10000, AdminFeeCents: 50000, OwnerPayableCents: 440000,
		},
	}
	if err := svc.fundRentIntermediation(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.EconomicBreakdown.CommissionPoolCents != 440000 || receipt.EconomicBreakdown.OwnerPayableCents != 0 {
		t.Fatalf("funding used cash that was not received: %+v", receipt.EconomicBreakdown)
	}
	if err := validateManualReceiptBreakdown(receipt); err != nil {
		t.Fatal(err)
	}
}

func TestManualRentReceiptWithoutFrozenPlanKeepsAdminFeeOnly(t *testing.T) {
	// A contract without a frozen intermediation plan degrades to the Phase A
	// shape: owner keeps gross - admin fee and nothing is funded.
	svc := &ManualReceiptSettlementService{rentalIntermediation: NewRentalIntermediationService(&rentalFundingConfigRepo{item: nil}, nil, nil)}

	paidAt := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	receipt := &transaction.Transaction{
		ID: "receipt-rent", TenantID: "tenant-rent", ContractID: "contract-rent", DealID: "deal-rent",
		Date: paidAt, PaymentDate: &paidAt,
		EconomicBreakdown: &transaction.EconomicBreakdown{
			Flow: "rent", GrossCents: 500000, AdminFeeCents: 50000, OwnerPayableCents: 450000,
		},
	}

	if err := svc.fundRentIntermediation(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	b := receipt.EconomicBreakdown
	if b.OwnerPayableCents != 450000 || b.AgencyRetainedCents != 0 || b.CommissionPoolCents != 0 {
		t.Fatalf("breakdown drifted without a frozen plan: %+v", b)
	}
}
