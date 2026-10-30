package services

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
)

func TestChargeFlowMapsChargeType(t *testing.T) {
	cases := map[charge.ChargeType]string{
		charge.TypeSale:  "sale",
		charge.TypeRent:  "rent",
		charge.TypeOther: "",
		"MAINTENANCE":    "",
	}
	for chargeType, want := range cases {
		if got := chargeFlow(chargeType); got != want {
			t.Fatalf("chargeFlow(%q) = %q, want %q", chargeType, got, want)
		}
	}
}

func TestManualReceiptJournalCarriesFlow(t *testing.T) {
	paidAt := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)

	sale := &transaction.Transaction{
		ID: "receipt-sale", TenantID: "tenant-1", ContractID: "contract-sale", DealID: "deal-sale",
		Date: paidAt, PaymentDate: &paidAt,
		EconomicBreakdown: &transaction.EconomicBreakdown{
			Flow: "sale", GrossCents: 98_000_000, CommissionPoolCents: 5_880_000,
			CommissionPayableCents: 3_528_000, AgencyRetainedCents: 2_352_000, OwnerPayableCents: 92_120_000,
		},
	}
	saleEntry, err := newManualReceiptJournalEntry(sale, "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	for i, leg := range saleEntry.Legs {
		if leg.Flow != "sale" {
			t.Fatalf("sale receipt leg %d flow = %q, want sale", i, leg.Flow)
		}
	}

	rent := &transaction.Transaction{
		ID: "receipt-rent", TenantID: "tenant-1", ContractID: "contract-rent", DealID: "deal-rent",
		Date: paidAt, PaymentDate: &paidAt,
		EconomicBreakdown: &transaction.EconomicBreakdown{
			Flow: "rent", GrossCents: 10_000_000, CommissionPoolCents: 1_000_000,
			CommissionPayableCents: 0, AgencyRetainedCents: 1_000_000, AdminFeeCents: 1_000_000, OwnerPayableCents: 8_000_000,
		},
	}
	rentEntry, err := newManualReceiptJournalEntry(rent, "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	for i, leg := range rentEntry.Legs {
		if leg.Flow != "rent" {
			t.Fatalf("rent receipt leg %d flow = %q, want rent", i, leg.Flow)
		}
	}
}

func TestChargeAllocationJournalCarriesFlow(t *testing.T) {
	rentCharge := canonicalAllocatedRentCharge(t)
	allocator := NewEconomicAllocationService(fixedAllocationSettings{adminFeePct: 8})
	method, allocations, err := allocator.BuildPaymentAllocations(context.Background(), rentCharge, "payment-rent", 600000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rentCharge.ApplyPaymentWithAllocations("payment-rent", 6000, "PIX", time.Now(), method, allocations); err != nil {
		t.Fatal(err)
	}
	entry, err := newAllocatedPaymentJournalEntry(rentCharge, &rentCharge.Payments[0])
	if err != nil {
		t.Fatal(err)
	}
	for i, leg := range entry.Legs {
		if leg.Flow != "rent" {
			t.Fatalf("rent charge leg %d flow = %q, want rent", i, leg.Flow)
		}
	}

	saleCharge, err := charge.NewWithLineItems(
		"contract-sale", "payer-sale", "owner-sale", "property-sale", "tenant-sale",
		charge.TypeSale, "Venda", "2026-08", 1000, time.Now(),
		[]charge.LineItem{{
			Description: "Comissão", AmountCents: 100000, SourceType: "sale_base",
			EconomicOwnerID: "tenant-sale", SettlementMode: charge.SettlementModeCollection,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	itemID := saleCharge.LineItems[0].ID
	allocation, err := charge.NewAllocationWithPurpose("payment-sale", itemID, saleCharge.TenantID, charge.AllocationTypeRevenue, 100000, "rule-sale", charge.AllocationPurposeAdminFee, "")
	if err != nil {
		t.Fatal(err)
	}
	salePayment := &charge.PaymentEntry{
		ID: "payment-sale", AmountCents: 100000, Method: "PIX", PaidAt: time.Now(),
		AllocationMethod: charge.AllocationMethodProportionalV1, Allocations: []charge.Allocation{*allocation},
	}
	saleEntry, err := newAllocatedPaymentJournalEntry(saleCharge, salePayment)
	if err != nil {
		t.Fatal(err)
	}
	for i, leg := range saleEntry.Legs {
		if leg.Flow != "sale" {
			t.Fatalf("sale charge leg %d flow = %q, want sale", i, leg.Flow)
		}
	}
}
