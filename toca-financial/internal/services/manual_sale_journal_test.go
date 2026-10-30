package services

import (
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
)

func TestNewManualSaleReceiptJournalEntryClosesEconomicOwnership(t *testing.T) {
	paidAt := time.Date(2026, time.August, 29, 12, 34, 56, 789123456, time.FixedZone("BRT", -3*60*60))
	receipt := &transaction.Transaction{
		ID:          "receipt-1",
		TenantID:    "tenant-1",
		ContractID:  "contract-1",
		DealID:      "deal-1",
		Date:        paidAt.Add(-24 * time.Hour),
		PaymentDate: &paidAt,
		EconomicBreakdown: &transaction.EconomicBreakdown{
			Flow:                   "sale",
			GrossCents:             98_000_000,
			CommissionPoolCents:    5_880_000,
			CommissionPayableCents: 3_528_000,
			AgencyRetainedCents:    2_352_000,
			OwnerPayableCents:      92_120_000,
		},
	}

	entry, err := newManualSaleReceiptJournalEntry(receipt, "owner-1")
	if err != nil {
		t.Fatalf("new manual sale receipt journal: %v", err)
	}
	if !entry.Balanced() {
		t.Fatal("receipt journal must be balanced")
	}
	if entry.OperationID != receipt.ID {
		t.Fatalf("operation id = %q, want %q", entry.OperationID, receipt.ID)
	}
	wantDate := paidAt.UTC().Truncate(time.Microsecond)
	if !entry.Date.Equal(wantDate) {
		t.Fatalf("date = %s, want %s", entry.Date, wantDate)
	}

	assertJournalLeg(t, entry, journal.AccountCash, journal.Debit, 98_000_000, "")
	assertJournalLeg(t, entry, journal.AccountOwnerPayable, journal.Credit, 92_120_000, "owner-1")
	// GROSS revenue: broker share (3_528_000) + agency retention (2_352_000).
	// The broker share is expensed separately via the accrual entry, so it is
	// no longer credited to commission_payable here.
	assertJournalLeg(t, entry, journal.AccountRevenue, journal.Credit, 5_880_000, "tenant-1")
}

func TestNewManualCommissionSettlementJournalEntryConsumesPayableAndSeparatesDeductions(t *testing.T) {
	paidAt := time.Date(2026, time.August, 29, 22, 18, 0, 987654321, time.UTC)
	ledger := &transaction.Transaction{
		ID:          "ledger-1",
		TenantID:    "tenant-1",
		ContractID:  "contract-1",
		Date:        paidAt.Add(-time.Hour),
		PaymentDate: &paidAt,
	}
	c := &commission.Commission{
		ID:          "commission-1",
		TenantID:    "tenant-1",
		ContractID:  "contract-1",
		BrokerID:    "broker-1",
		Amount:      1_000,
		GrossAmount: 1_000,
		NetAmount:   850,
		PaidAt:      &paidAt,
		Deductions: []commission.Deduction{
			{ID: "irrf-1", Kind: commission.DeductionIRRF, Amount: 100},
			{ID: "other-1", Kind: commission.DeductionOutros, Amount: 50},
		},
	}

	entry, err := newManualCommissionSettlementJournalEntry(ledger, c)
	if err != nil {
		t.Fatalf("new manual commission settlement journal: %v", err)
	}
	if !entry.Balanced() {
		t.Fatal("commission settlement journal must be balanced")
	}
	if entry.OperationID != c.ID {
		t.Fatalf("operation id = %q, want %q", entry.OperationID, c.ID)
	}
	assertJournalLeg(t, entry, journal.AccountCommissionPayable, journal.Debit, 100_000, "broker-1")
	assertJournalLeg(t, entry, journal.AccountCash, journal.Credit, 85_000, "broker-1")
	assertJournalLeg(t, entry, journal.AccountTaxWithheld, journal.Credit, 10_000, "")
	assertJournalLeg(t, entry, journal.AccountOtherPayable, journal.Credit, 5_000, "")
}

func assertJournalLeg(t *testing.T, entry *journal.Entry, account journal.Account, direction journal.Direction, amountCents int64, beneficiary string) {
	t.Helper()
	for _, leg := range entry.Legs {
		if leg.Account == account && leg.Direction == direction && leg.AmountCents == amountCents && leg.BeneficiaryPartyID == beneficiary {
			return
		}
	}
	t.Fatalf("journal leg not found: account=%s direction=%s amount=%d beneficiary=%q; legs=%+v", account, direction, amountCents, beneficiary, entry.Legs)
}
