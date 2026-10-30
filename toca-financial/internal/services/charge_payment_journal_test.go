package services

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
)

func TestAllocatedPaymentJournalUsesTheSameEconomicAllocations(t *testing.T) {
	c := canonicalAllocatedRentCharge(t)
	allocator := NewEconomicAllocationService(fixedAllocationSettings{adminFeePct: 8})
	method, allocations, err := allocator.BuildPaymentAllocations(context.Background(), c, "payment-journal", 600000)
	if err != nil {
		t.Fatal(err)
	}
	paidAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	if _, err := c.ApplyPaymentWithAllocations("payment-journal", 6000, "PIX", paidAt, method, allocations); err != nil {
		t.Fatal(err)
	}

	entry, err := newAllocatedPaymentJournalEntry(c, &c.Payments[0])
	if err != nil {
		t.Fatalf("newAllocatedPaymentJournalEntry() error = %v", err)
	}
	if !entry.Balanced() || entry.OperationID != "payment-journal" || !entry.Date.Equal(paidAt) {
		t.Fatalf("unexpected journal header: %+v", entry)
	}
	if len(entry.Legs) != 5 {
		t.Fatalf("legs = %d, want cash + four allocation legs", len(entry.Legs))
	}
	if entry.Legs[0].Account != journal.AccountCash || entry.Legs[0].Direction != journal.Debit || entry.Legs[0].AmountCents != 600000 {
		t.Fatalf("cash leg = %+v", entry.Legs[0])
	}

	var ownerPayable, revenue int64
	for _, leg := range entry.Legs[1:] {
		if leg.Direction != journal.Credit || leg.AllocationID == "" || leg.ChargeItemID == "" || leg.BeneficiaryPartyID == "" || leg.RuleVersionID == "" {
			t.Fatalf("credit leg lost allocation traceability: %+v", leg)
		}
		switch leg.Account {
		case journal.AccountOwnerPayable:
			ownerPayable += leg.AmountCents
		case journal.AccountRevenue:
			revenue += leg.AmountCents
		default:
			t.Fatalf("unexpected account in canonical entry: %s", leg.Account)
		}
	}
	if ownerPayable != 560000 || revenue != 40000 {
		t.Fatalf("journal credits = owner %d revenue %d", ownerPayable, revenue)
	}
}

func TestJournalAccountForAllocationCoversEveryAllocationType(t *testing.T) {
	tests := map[charge.AllocationType]journal.Account{
		charge.AllocationTypeRevenue:           journal.AccountRevenue,
		charge.AllocationTypeOwnerPayable:      journal.AccountOwnerPayable,
		charge.AllocationTypeCommissionPayable: journal.AccountCommissionPayable,
		charge.AllocationTypePartnerPayable:    journal.AccountPartnerPayable,
		charge.AllocationTypeRoyaltyPayable:    journal.AccountRoyaltyPayable,
		charge.AllocationTypeTaxWithheld:       journal.AccountTaxWithheld,
	}
	for allocationType, want := range tests {
		got, err := journalAccountForAllocation(allocationType)
		if err != nil || got != want {
			t.Errorf("journalAccountForAllocation(%q) = (%q, %v), want %q", allocationType, got, err, want)
		}
	}
}

func TestAllocatedPaymentJournalPreservesRentalFundingPurposeAndCommissionLink(t *testing.T) {
	c, err := charge.NewWithLineItems(
		"contract-rent", "payer-rent", "owner-rent", "property-rent", "tenant-rent",
		charge.TypeRent, "Aluguel", "2026-08", 1000, time.Now(),
		[]charge.LineItem{{
			Description: "Aluguel", AmountCents: 100000, SourceType: "rental_base",
			EconomicOwnerID: "owner-rent", SettlementMode: charge.SettlementModeCollection,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	itemID := c.LineItems[0].ID
	allocation, err := charge.NewAllocationWithPurpose(
		"payment-rent", itemID, c.TenantID, charge.AllocationTypeRevenue, 100000,
		"rule-rent", charge.AllocationPurposeRentalIntermediationBroker, "commission-rent",
	)
	if err != nil {
		t.Fatal(err)
	}
	payment := &charge.PaymentEntry{
		ID: "payment-rent", AmountCents: 100000, Method: "PIX", PaidAt: time.Now(),
		AllocationMethod: charge.AllocationMethodProportionalV1,
		Allocations:      []charge.Allocation{*allocation},
	}
	entry, err := newAllocatedPaymentJournalEntry(c, payment)
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Legs) != 2 || entry.Legs[1].AllocationPurpose != string(charge.AllocationPurposeRentalIntermediationBroker) || entry.Legs[1].RelatedEntityID != "commission-rent" {
		t.Fatalf("journal lost rental commission traceability: %+v", entry.Legs)
	}
}
