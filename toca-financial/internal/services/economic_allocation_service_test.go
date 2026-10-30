package services

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
)

type fixedAllocationSettings struct {
	adminFeePct float64
	err         error
}

type fixedRentalIntermediationAllocator struct {
	entry             rental_charge_config.IntermediationFundingEntry
	availableReceived int64
}

func (f *fixedRentalIntermediationAllocator) FundPayment(
	_ context.Context,
	_, _, _, _, _ string,
	availableCents int64,
	_ time.Time,
) (rental_charge_config.IntermediationFundingEntry, error) {
	f.availableReceived = availableCents
	return f.entry, nil
}

func (s fixedAllocationSettings) Get(_ context.Context, tenantID string) (*tenant_financial_settings.Settings, error) {
	if s.err != nil {
		return nil, s.err
	}
	settings := tenant_financial_settings.New(tenantID)
	settings.AdminFeePct = s.adminFeePct
	return settings, nil
}

func canonicalAllocatedRentCharge(t *testing.T) *charge.Charge {
	t.Helper()
	c, err := charge.NewWithLineItems(
		"contract-1", "payer-1", "owner-1", "property-1", "tenant-1",
		charge.TypeRent, "Aluguel + Encargos", "2026-08", 6000,
		time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		[]charge.LineItem{
			{Description: "Aluguel", AmountCents: 500000, SourceType: "rental_base", SourceID: "cfg-1", EconomicOwnerID: "owner-1", SettlementMode: charge.SettlementModeCollection, RuleVersionID: "rule-1"},
			{Description: "Condomínio", AmountCents: 70000, SourceType: "rental_bundled_item", SourceID: "condominio", EconomicOwnerID: "owner-1", SettlementMode: charge.SettlementModeDirectLiquidation, RuleVersionID: "rule-1"},
			{Description: "IPTU", AmountCents: 30000, SourceType: "rental_bundled_item", SourceID: "iptu", EconomicOwnerID: "owner-1", SettlementMode: charge.SettlementModeDirectLiquidation, RuleVersionID: "rule-1"},
		},
	)
	if err != nil {
		t.Fatalf("NewWithLineItems() error = %v", err)
	}
	return c
}

func TestEconomicAllocationServiceCanonicalRentCase(t *testing.T) {
	c := canonicalAllocatedRentCharge(t)
	svc := NewEconomicAllocationService(fixedAllocationSettings{adminFeePct: 8})

	method, allocations, err := svc.BuildPaymentAllocations(context.Background(), c, "payment-1", 600000)
	if err != nil {
		t.Fatalf("BuildPaymentAllocations() error = %v", err)
	}
	if method != charge.AllocationMethodProportionalV1 {
		t.Fatalf("method = %q", method)
	}

	type allocationKey struct {
		itemID         string
		allocationType charge.AllocationType
		beneficiary    string
	}
	got := make(map[allocationKey]int64, len(allocations))
	for _, allocation := range allocations {
		got[allocationKey{allocation.ChargeItemID, allocation.AllocationType, allocation.BeneficiaryPartyID}] += allocation.AmountCents
		if allocation.ID == "" || allocation.RuleVersionID == "" {
			t.Fatalf("allocation lacks audit identity: %+v", allocation)
		}
	}
	baseID := c.LineItems[0].ID
	condoID := c.LineItems[1].ID
	iptuID := c.LineItems[2].ID
	want := map[allocationKey]int64{
		{baseID, charge.AllocationTypeOwnerPayable, "owner-1"}:  460000,
		{baseID, charge.AllocationTypeRevenue, "tenant-1"}:      40000,
		{condoID, charge.AllocationTypeOwnerPayable, "owner-1"}: 70000,
		{iptuID, charge.AllocationTypeOwnerPayable, "owner-1"}:  30000,
	}
	if len(got) != len(want) {
		t.Fatalf("allocation groups = %+v, want %+v", got, want)
	}
	for key, amount := range want {
		if got[key] != amount {
			t.Errorf("allocation %+v = %d, want %d", key, got[key], amount)
		}
	}

	changed, err := c.ApplyPaymentWithAllocations("payment-1", 6000, "PIX", time.Now(), method, allocations)
	if err != nil || !changed {
		t.Fatalf("ApplyPaymentWithAllocations() = (%v, %v)", changed, err)
	}
	if c.Status != charge.StatusPaid || len(c.Payments) != 1 || len(c.Payments[0].Allocations) != 4 {
		t.Fatalf("paid charge did not retain allocations: %+v", c)
	}
}

func TestEconomicAllocationServiceAppliesRentalWaterfallBeforeOwnerTransfer(t *testing.T) {
	c, err := charge.NewWithLineItems(
		"contract-rent", "payer-1", "owner-1", "property-1", "tenant-1",
		charge.TypeRent, "Primeiro aluguel proporcional", "2026-08", 3000,
		time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
		[]charge.LineItem{{
			Description: "Aluguel proporcional", AmountCents: 300000, SourceType: "rental_base",
			SourceID: "cfg-1", EconomicOwnerID: "owner-1", SettlementMode: charge.SettlementModeCollection,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	allocator := &fixedRentalIntermediationAllocator{entry: rental_charge_config.IntermediationFundingEntry{
		PaymentID: "payment-1", ChargeID: c.ID, ReferenceMonth: "2026-08", AppliedCents: 120000,
		BrokerFunding: []rental_charge_config.IntermediationFundingShare{{
			BrokerID: "broker-1", Role: "fechamento", CommissionID: "commission-1", AmountCents: 100000,
		}},
		AgencyRetainedCents: 20000, OutstandingAfterCents: 180000,
	}}
	svc := NewEconomicAllocationService(fixedAllocationSettings{adminFeePct: 10}).WithRentalIntermediation(allocator)

	_, allocations, err := svc.BuildPaymentAllocations(context.Background(), c, "payment-1", 300000)
	if err != nil {
		t.Fatal(err)
	}
	if allocator.availableReceived != 270000 {
		t.Fatalf("available after recurring admin fee = %d, want 270000", allocator.availableReceived)
	}
	got := map[charge.AllocationPurpose]int64{}
	var total int64
	for _, allocation := range allocations {
		got[allocation.Purpose] += allocation.AmountCents
		total += allocation.AmountCents
		if allocation.Purpose == charge.AllocationPurposeRentalIntermediationBroker && allocation.RelatedEntityID != "commission-1" {
			t.Fatalf("broker funding lost commission identity: %+v", allocation)
		}
	}
	want := map[charge.AllocationPurpose]int64{
		charge.AllocationPurposeAdminFee:                   30000,
		charge.AllocationPurposeRentalIntermediationBroker: 100000,
		charge.AllocationPurposeRentalIntermediationAgency: 20000,
		charge.AllocationPurposeOwnerRent:                  150000,
	}
	if total != 300000 || len(got) != len(want) {
		t.Fatalf("waterfall allocations total=%d got=%+v", total, got)
	}
	for purpose, amount := range want {
		if got[purpose] != amount {
			t.Errorf("purpose %s = %d, want %d", purpose, got[purpose], amount)
		}
	}
}

func TestEconomicAllocationServicePartialPaymentsConserveCentsAndFee(t *testing.T) {
	c := canonicalAllocatedRentCharge(t)
	svc := NewEconomicAllocationService(fixedAllocationSettings{adminFeePct: 8})

	method1, allocations1, err := svc.BuildPaymentAllocations(context.Background(), c, "payment-40", 240000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyPaymentWithAllocations("payment-40", 2400, "PIX", time.Now(), method1, allocations1); err != nil {
		t.Fatal(err)
	}
	method2, allocations2, err := svc.BuildPaymentAllocations(context.Background(), c, "payment-60", 360000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyPaymentWithAllocations("payment-60", 3600, "PIX", time.Now(), method2, allocations2); err != nil {
		t.Fatal(err)
	}

	var allocated, revenue, ownerPayable int64
	for _, payment := range c.Payments {
		var paymentAllocated int64
		for _, allocation := range payment.Allocations {
			allocated += allocation.AmountCents
			paymentAllocated += allocation.AmountCents
			switch allocation.AllocationType {
			case charge.AllocationTypeRevenue:
				revenue += allocation.AmountCents
			case charge.AllocationTypeOwnerPayable:
				ownerPayable += allocation.AmountCents
			}
		}
		if paymentAllocated != payment.AmountCents {
			t.Fatalf("payment %s allocated %d of %d cents", payment.ID, paymentAllocated, payment.AmountCents)
		}
	}
	if allocated != 600000 || revenue != 40000 || ownerPayable != 560000 {
		t.Fatalf("totals = allocated %d revenue %d owner %d", allocated, revenue, ownerPayable)
	}
	if c.Status != charge.StatusPaid {
		t.Fatalf("status = %s, want PAID", c.Status)
	}
}

func TestEconomicAllocationServiceUsesLargestRemainderAndDoesNotDoubleChargeExplicitAdminFee(t *testing.T) {
	other, err := charge.NewWithLineItems(
		"contract-2", "payer-1", "owner-1", "property-1", "tenant-1", charge.TypeOther,
		"Centavos", "2026-08", 0.03, time.Now(),
		[]charge.LineItem{
			{Description: "A", AmountCents: 1},
			{Description: "B", AmountCents: 1},
			{Description: "C", AmountCents: 1},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, allocations, err := NewEconomicAllocationService(nil).BuildPaymentAllocations(context.Background(), other, "payment-cents", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(allocations) != 2 || allocations[0].AmountCents != 1 || allocations[1].AmountCents != 1 || allocations[0].ChargeItemID != other.LineItems[0].ID || allocations[1].ChargeItemID != other.LineItems[1].ID {
		t.Fatalf("largest remainder was not deterministic: %+v", allocations)
	}

	rent, err := charge.NewWithLineItems(
		"contract-3", "payer-1", "owner-1", "property-1", "tenant-1", charge.TypeRent,
		"Aluguel + taxa", "2026-08", 5400, time.Now(),
		[]charge.LineItem{
			{Description: "Aluguel", AmountCents: 500000, SourceType: "rental_base", SourceID: "cfg-1", EconomicOwnerID: "owner-1", SettlementMode: charge.SettlementModeCollection},
			{Description: "Taxa", AmountCents: 40000, SourceType: "rental_bundled_item", SourceID: "admin_fee", EconomicOwnerID: "tenant-1", SettlementMode: charge.SettlementModeCollection},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, allocations, err = NewEconomicAllocationService(fixedAllocationSettings{adminFeePct: 8}).BuildPaymentAllocations(context.Background(), rent, "payment-admin", 540000)
	if err != nil {
		t.Fatal(err)
	}
	var revenue int64
	for _, allocation := range allocations {
		if allocation.AllocationType == charge.AllocationTypeRevenue {
			revenue += allocation.AmountCents
		}
	}
	if revenue != 40000 {
		t.Fatalf("explicit administration revenue = %d, want 40000 without a second fee", revenue)
	}
}

func TestEconomicAllocationServiceHonorsExplicitSaleDestinations(t *testing.T) {
	sale, err := charge.NewWithLineItems(
		"sale-contract", "buyer-1", "owner-1", "property-1", "tenant-1", charge.TypeSale,
		"Venda", "2026-08", 980000, time.Now(),
		[]charge.LineItem{
			{Description: "Saldo do proprietário", AmountCents: 92120000, SourceType: "sale_owner_proceeds", EconomicOwnerID: "owner-1", SettlementMode: charge.SettlementModeCollection, AllocationType: charge.AllocationTypeOwnerPayable},
			{Description: "Comissões", AmountCents: 3528000, SourceType: "sale_commission_payable", EconomicOwnerID: "tenant-1", SettlementMode: charge.SettlementModeCollection, AllocationType: charge.AllocationTypeCommissionPayable},
			{Description: "Retenção", AmountCents: 2352000, SourceType: "sale_agency_retained", EconomicOwnerID: "tenant-1", SettlementMode: charge.SettlementModeCollection, AllocationType: charge.AllocationTypeRevenue},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, allocations, err := NewEconomicAllocationService(nil).BuildPaymentAllocations(context.Background(), sale, "sale-payment", 98000000)
	if err != nil {
		t.Fatal(err)
	}
	got := map[charge.AllocationType]int64{}
	for _, allocation := range allocations {
		got[allocation.AllocationType] += allocation.AmountCents
	}
	if got[charge.AllocationTypeOwnerPayable] != 92120000 ||
		got[charge.AllocationTypeCommissionPayable] != 3528000 ||
		got[charge.AllocationTypeRevenue] != 2352000 {
		t.Fatalf("sale destinations were reclassified: %+v", got)
	}
}

func TestEconomicAllocationServiceRebuildPaymentAllocationsToleratesUnallocatedHistory(t *testing.T) {
	sale, err := charge.NewWithLineItems(
		"sale-contract", "buyer-1", "owner-1", "property-1", "tenant-1", charge.TypeSale,
		"Venda", "2026-08", 980000, time.Now(),
		[]charge.LineItem{
			{Description: "Saldo do proprietário", AmountCents: 92120000, SourceType: "sale_owner_proceeds", EconomicOwnerID: "owner-1", SettlementMode: charge.SettlementModeCollection, AllocationType: charge.AllocationTypeOwnerPayable},
			{Description: "Comissões", AmountCents: 3528000, SourceType: "sale_commission_payable", EconomicOwnerID: "tenant-1", SettlementMode: charge.SettlementModeCollection, AllocationType: charge.AllocationTypeCommissionPayable},
			{Description: "Retenção", AmountCents: 2352000, SourceType: "sale_agency_retained", EconomicOwnerID: "tenant-1", SettlementMode: charge.SettlementModeCollection, AllocationType: charge.AllocationTypeRevenue},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	// Pagamento histórico lançado antes da composição econômica existir.
	sale.Payments = append(sale.Payments, charge.PaymentEntry{
		ID: "legacy-payment", AmountCents: 98000000, Method: "manual", PaidAt: time.Now(),
	})
	svc := NewEconomicAllocationService(nil)

	// O builder normal recusa misturar caixa antigo não classificado…
	_, allocations, err := svc.BuildPaymentAllocations(context.Background(), sale, "new-payment", 98000000)
	if err != nil {
		t.Fatal(err)
	}
	if len(allocations) != 0 {
		t.Fatalf("regular builder must refuse to mix unallocated history, got %d allocations", len(allocations))
	}

	// …mas a variante de reparo deriva a divisão do pagamento desalocado.
	_, allocations, err = svc.RebuildPaymentAllocations(context.Background(), sale, "legacy-payment", 98000000)
	if err != nil {
		t.Fatal(err)
	}
	got := map[charge.AllocationType]int64{}
	for _, allocation := range allocations {
		got[allocation.AllocationType] += allocation.AmountCents
	}
	if got[charge.AllocationTypeOwnerPayable] != 92120000 ||
		got[charge.AllocationTypeCommissionPayable] != 3528000 ||
		got[charge.AllocationTypeRevenue] != 2352000 {
		t.Fatalf("rebuild misallocated the legacy payment: %+v", got)
	}
	if allocations[0].PaymentID != "legacy-payment" {
		t.Fatalf("rebuild must bind allocations to the legacy payment, got %s", allocations[0].PaymentID)
	}
}

func TestEconomicAllocationServiceRejectsUndefinedPositiveAdjustment(t *testing.T) {
	c := canonicalAllocatedRentCharge(t)
	c.TotalAmount = c.TotalAmount.Add(valueobjects.NewMoney(100))
	_, _, err := NewEconomicAllocationService(fixedAllocationSettings{adminFeePct: 8}).BuildPaymentAllocations(context.Background(), c, "payment-late", 600100)
	if err == nil {
		t.Fatal("expected an explicit-rule error for cash above the itemized principal")
	}
}

func TestEconomicAllocationServiceFailsClosedOnCorruptAllocationHistory(t *testing.T) {
	c := canonicalAllocatedRentCharge(t)
	svc := NewEconomicAllocationService(fixedAllocationSettings{adminFeePct: 8})
	method, allocations, err := svc.BuildPaymentAllocations(context.Background(), c, "payment-first", 300000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyPaymentWithAllocations("payment-first", 3000, "PIX", time.Now(), method, allocations); err != nil {
		t.Fatal(err)
	}
	c.Payments[0].Allocations[0].PaymentID = "another-payment"
	if _, _, err := svc.BuildPaymentAllocations(context.Background(), c, "payment-second", 300000); err == nil {
		t.Fatal("expected corrupt allocation history to fail closed")
	}
}
