package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/clients"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
)

// A pro-rata manual receipt funds part of the signing-time commission. The
// following rent completes the frozen fee and releases only the remainder to
// the owner; a later rent has no intermediation deduction. Every persisted
// receipt must agree with its journal and transfer, including on old retries.
func TestManualRentFinancialJourneyPostgresPreservesFundingAndReceiptComposition(t *testing.T) {
	db := openFinancialConsistencyPostgres(t)
	tenantID, contractID, dealID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	ownerID, propertyID, brokerID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	ctx, cancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 45*time.Second)
	defer cancel()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 15*time.Second)
		defer cleanupCancel()
		if _, err := deleteJournalFixturesInTest(cleanupCtx, db, `SELECT id::text FROM financial.journal_entries WHERE tenant_id = $1`, tenantID); err != nil {
			t.Errorf("cleanup manual rent journals: %v", err)
		}
		for _, table := range []string{"transfers", "payout_orders", "commissions", "transactions", "rental_charge_configs", "journal_entries"} {
			if table != "journal_entries" {
				if _, err := db.Exec(cleanupCtx, fmt.Sprintf("DELETE FROM financial.%s WHERE tenant_id = $1", table), tenantID); err != nil {
					t.Errorf("cleanup manual rent %s: %v", table, err)
				}
			}
			var residual int
			if err := db.QueryRow(cleanupCtx, fmt.Sprintf("SELECT COUNT(*) FROM financial.%s WHERE tenant_id = $1", table), tenantID).Scan(&residual); err != nil {
				t.Errorf("verify manual rent %s cleanup: %v", table, err)
			} else if residual != 0 {
				t.Errorf("manual rent cleanup left %d %s rows", residual, table)
			}
		}
	})

	txRepo := repositories.NewTransactionPostgresRepository(db)
	commissionRepo := repositories.NewCommissionPostgresRepository(db)
	payoutRepo := repositories.NewPayoutOrderPostgresRepository(db)
	configRepo := repositories.NewRentalChargeConfigPostgresRepository(db)
	transferRepo := repositories.NewTransferPostgresRepository(db)
	journalRepo := repositories.NewJournalPostgresRepository(db)
	journalService := NewJournalService(journalRepo).WithDB(db)
	commissionService := NewCommissionService(commissionRepo).WithDB(db)
	transferService := NewTransferService(transferRepo).WithDB(db).WithJournalService(journalService)
	intermediation := NewRentalIntermediationService(configRepo, commissionService, payoutRepo).RequirePaymentTransaction()
	settlement := NewManualReceiptSettlementService(transferService, fixedManualReceiptDealLookup{deal: &clients.DealDTO{
		ID: dealID, TenantID: tenantID, Type: "RENT", Value: 5_000, OwnerID: ownerID, PropertyID: propertyID,
	}}).WithJournalService(journalService).WithRentalIntermediation(intermediation)
	transactionService := NewTransactionService(txRepo).WithDB(db).
		WithManualReceiptSettings(fixedAllocationSettings{adminFeePct: 10}).
		WithManualReceiptSettlement(settlement)

	// Seed the committed signing result, then exercise all receipts through the
	// production posting/settlement services and real PostgreSQL repositories.
	startAt := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	cfg, err := rental_charge_config.New(tenantID, propertyID, contractID, uuid.NewString(), ownerID,
		startAt, startAt.AddDate(1, 0, 0), startAt, rental_charge_config.SameMonthProRata, 10, 500_000)
	if err != nil {
		t.Fatal(err)
	}
	if err := configRepo.Create(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	plan := ResolvedCommissionPlan{
		PlanHash: "manual-rent-journey-plan", TotalAmountCents: 500_000,
		Shares: []ResolvedCommissionShare{{BrokerID: brokerID, Role: "fechamento", Percentage: 60, AmountCents: 300_000}},
	}
	if err := intermediation.FreezePlan(ctx, tenantID, contractID, dealID, plan); err != nil {
		t.Fatal(err)
	}
	comm, err := commissionService.CreateRentalWithRoleFromEvent(ctx, uuid.NewString(), dealID, brokerID,
		contractID, "fechamento", 60, 3_000, tenantID, ownerID, "", propertyID, plan.PlanHash, 300_000)
	if err != nil {
		t.Fatal(err)
	}
	order, err := payout_order.New(tenantID, payout_order.SourceManual, contractID,
		payout_order.RecipientBroker, brokerID, "Corretor", 300_000, "system")
	if err != nil {
		t.Fatal(err)
	}
	order.ContractID = contractID
	if _, err := order.EnableRentalFundingGate(300_000); err != nil {
		t.Fatal(err)
	}
	if err := payoutRepo.Create(ctx, order); err != nil {
		t.Fatal(err)
	}
	if err := intermediation.LinkCommission(ctx, tenantID, contractID, brokerID, "fechamento", comm.ID); err != nil {
		t.Fatal(err)
	}
	if err := intermediation.LinkPayout(ctx, tenantID, contractID, brokerID, "fechamento", order.ID); err != nil {
		t.Fatal(err)
	}
	accrual, err := journal.New(tenantID, contractID, "Comissão contratual da locação", startAt, []journal.Leg{
		{Account: journal.AccountCommissionExpense, Direction: journal.Debit, AmountCents: 300_000, Flow: "rent"},
		{Account: journal.AccountCommissionPayable, Direction: journal.Credit, AmountCents: 300_000, Flow: "rent", BeneficiaryPartyID: brokerID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := journalService.CreateWithSourceEventID(ctx, uuid.NewString(), accrual); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		grossCents, feeCents, appliedCents, ownerCents, brokerFundedCents int64
		fundingEntries                                                    int
		payoutStatus                                                      payout_order.Status
	}{
		{300_000, 30_000, 270_000, 0, 270_000, 1, payout_order.StatusDraft},
		{500_000, 50_000, 230_000, 220_000, 300_000, 2, payout_order.StatusPendingReview},
		{500_000, 50_000, 0, 450_000, 300_000, 2, payout_order.StatusPendingReview},
	}
	receipts := make([]*transaction.Transaction, 0, len(cases))
	for i, want := range cases {
		paidAt := startAt.AddDate(0, i, 0).Add(12 * time.Hour)
		outputs, err := transactionService.BatchCreatePosting(ctx, tenantID, "manual-rent-journey:"+uuid.NewString(), []BatchCreateInput{{
			ContractID: contractID, DealID: dealID, Type: string(transaction.TypeReceita), Flow: "rent",
			Description: "Recebimento - Locação", Value: float64(want.grossCents) / 100, DateStr: paidAt.Format("2006-01-02"),
		}}, nil, nil)
		if err != nil || len(outputs) != 1 || outputs[0].Transaction == nil {
			t.Fatalf("post rental receipt %d: outputs=%+v err=%v", i, outputs, err)
		}
		receipt := outputs[0].Transaction
		if i == 0 {
			// Fail after the config and commission writes to prove a downstream
			// payout CAS failure rolls back the complete settlement transaction.
			sentinel := errors.New("reject rental payout funding fixture")
			settlement.WithRentalIntermediation(NewRentalIntermediationService(configRepo, commissionService,
				rejectManualRentFundingPayoutRepository{Repository: payoutRepo, err: sentinel}).RequirePaymentTransaction())
			if _, err := transactionService.Settle(ctx, receipt.ID, tenantID, &paidAt, "PIX"); !errors.Is(err, sentinel) {
				t.Fatalf("failed settlement = %v, want injected payout failure", err)
			}
			settlement.WithRentalIntermediation(intermediation)
			pending, err := txRepo.GetByID(ctx, tenantID, receipt.ID)
			if err != nil || pending == nil || transaction.CanonicalStatus(pending.Status) == transaction.StatusPaid ||
				pending.EconomicBreakdown.CommissionPoolCents != 0 {
				t.Fatalf("rollback persisted paid receipt: receipt=%+v err=%v", pending, err)
			}
			rolledBack, err := configRepo.GetByContractID(ctx, tenantID, contractID)
			if err != nil || rolledBack == nil || len(rolledBack.IntermediationPlan.FundingEntries) != 0 ||
				rolledBack.IntermediationOutstandingCents() != 500_000 {
				t.Fatalf("rollback leaked plan funding: config=%+v err=%v", rolledBack, err)
			}
			unfunded, err := commissionRepo.GetByID(ctx, tenantID, comm.ID)
			if err != nil || unfunded == nil || unfunded.FundedAmountCents != 0 || len(unfunded.FundingEntries) != 0 {
				t.Fatalf("rollback leaked commission funding: commission=%+v err=%v", unfunded, err)
			}
		}
		settled, err := transactionService.Settle(ctx, receipt.ID, tenantID, &paidAt, "PIX")
		if err != nil {
			t.Fatalf("settle rental receipt %d: %v", i, err)
		}
		persisted, err := txRepo.GetByID(ctx, tenantID, receipt.ID)
		fundedThisReceipt := want.brokerFundedCents
		if i > 0 {
			fundedThisReceipt -= cases[i-1].brokerFundedCents
		}
		wantBreakdown := transaction.EconomicBreakdown{
			Flow: "rent", GrossCents: want.grossCents, AdminFeeCents: want.feeCents,
			CommissionPoolCents: want.appliedCents, AgencyRetainedCents: want.appliedCents - fundedThisReceipt, OwnerPayableCents: want.ownerCents,
			CommissionPayableCents: fundedThisReceipt, CommissionFundedCents: fundedThisReceipt, CommissionFundingKnown: true,
		}
		if err != nil || persisted == nil || persisted.EconomicBreakdown == nil ||
			transaction.CanonicalStatus(persisted.Status) != transaction.StatusPaid ||
			*persisted.EconomicBreakdown != wantBreakdown || *settled.EconomicBreakdown != wantBreakdown {
			t.Fatalf("persisted rental composition %d: receipt=%+v returned=%+v want=%+v err=%v", i, persisted, settled, wantBreakdown, err)
		}
		receipts = append(receipts, persisted)

		fundedCommission, err := commissionRepo.GetByID(ctx, tenantID, comm.ID)
		if err != nil || fundedCommission == nil || fundedCommission.FundedAmountCents != want.brokerFundedCents ||
			len(fundedCommission.FundingEntries) != want.fundingEntries {
			t.Fatalf("commission funding after receipt %d: commission=%+v err=%v", i, fundedCommission, err)
		}
		fundedOrder, err := payoutRepo.GetByID(ctx, tenantID, order.ID)
		if err != nil || fundedOrder == nil || fundedOrder.FundedAmountCents != want.brokerFundedCents ||
			fundedOrder.Status != want.payoutStatus || len(fundedOrder.FundingEntries) != want.fundingEntries {
			t.Fatalf("payout funding after receipt %d: payout=%+v err=%v", i, fundedOrder, err)
		}
		if i == 0 {
			if _, err := commissionService.Approve(ctx, tenantID, comm.ID, "approver-journey"); !errors.Is(err, commission.ErrFundingIncomplete) {
				t.Fatalf("approve partially funded commission = %v", err)
			}
			if err := fundedOrder.Approve("operator-1"); !errors.Is(err, payout_order.ErrFundingIncomplete) {
				t.Fatalf("approve partially funded payout = %v", err)
			}
		}
		ownerTransfer, err := transferRepo.(transferTransactionIdentityRepository).GetBySourceTransactionID(ctx, tenantID, receipt.ID)
		if err != nil || ownerTransfer == nil || ownerTransfer.Status != transfer.StatusPending ||
			ownerTransfer.NetAmount != float64(want.ownerCents)/100 || ownerTransfer.OwnerBalance == nil ||
			ownerTransfer.OwnerBalance.TotalCents != want.ownerCents || ownerTransfer.EconomicBreakdown == nil ||
			ownerTransfer.EconomicBreakdown.AgencyRevenueCents != want.feeCents+want.appliedCents {
			t.Fatalf("owner transfer for rental receipt %d: transfer=%+v err=%v", i, ownerTransfer, err)
		}
		entry, err := journalRepo.GetBySourceEventID(ctx, tenantID, manualReceiptJournalSourceID(persisted))
		if err != nil || entry == nil || entry.OperationID != receipt.ID {
			t.Fatalf("journal for rental receipt %d: entry=%+v err=%v", i, entry, err)
		}
		totals := map[journal.Account]int64{}
		for _, leg := range entry.Legs {
			if leg.Flow != "rent" {
				t.Fatalf("rental receipt %d journal has untagged leg: %+v", i, leg)
			}
			signed := leg.AmountCents
			if leg.Direction == journal.Credit {
				signed = -signed
			}
			totals[leg.Account] += signed
		}
		if totals[journal.AccountCash] != want.grossCents || totals[journal.AccountOwnerPayable] != -want.ownerCents ||
			totals[journal.AccountRevenue] != -want.feeCents-want.appliedCents || totals[journal.AccountCommissionPayable] != 0 {
			t.Fatalf("rental receipt %d journal composition = %+v", i, totals)
		}
		if want.ownerCents > 0 {
			if err := transferService.Approve(ctx, tenantID, ownerTransfer.ID, "approver-journey", "Aprovador"); err != nil {
				t.Fatalf("approve rental owner transfer %d: %v", i, err)
			}
			if err := transferService.Process(ctx, tenantID, ownerTransfer.ID, "payer-journey", "Pagador"); err != nil {
				t.Fatalf("process rental owner transfer %d: %v", i, err)
			}
		}
	}

	// Retry the oldest receipt only after the complete plan has been funded and
	// later owner transfers paid. The original allocation must remain canonical.
	for i, receipt := range receipts {
		retried, err := transactionService.Settle(ctx, receipt.ID, tenantID, receipt.PaymentDate, "PIX")
		if err != nil || retried == nil || retried.EconomicBreakdown == nil || *retried.EconomicBreakdown != *receipt.EconomicBreakdown {
			t.Fatalf("retry rental receipt %d changed its composition: receipt=%+v err=%v", i, retried, err)
		}
	}
	finalConfig, err := configRepo.GetByContractID(ctx, tenantID, contractID)
	if err != nil || finalConfig == nil || finalConfig.IntermediationOutstandingCents() != 0 ||
		len(finalConfig.IntermediationPlan.FundingEntries) != 2 || finalConfig.IntermediationPlan.AgencyRetainedFundedAmountCents != 200_000 {
		t.Fatalf("final rental funding plan: config=%+v err=%v", finalConfig, err)
	}
	for i, funding := range finalConfig.IntermediationPlan.FundingEntries {
		if funding.PaymentID != receipts[i].ID || funding.ChargeID != "" || funding.AppliedCents != cases[i].appliedCents ||
			funding.AvailableCents != cases[i].grossCents-cases[i].feeCents {
			t.Fatalf("manual receipt funding provenance %d = %+v", i, funding)
		}
	}
	for table, want := range map[string]int{"transactions": 3, "commissions": 1, "payout_orders": 1, "transfers": 3, "journal_entries": 6} {
		var count int
		if err := db.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM financial.%s WHERE tenant_id = $1 AND deleted_at IS NULL", table), tenantID).Scan(&count); err != nil {
			t.Fatalf("count rental %s: %v", table, err)
		} else if count != want {
			t.Fatalf("rental %s after retries = %d, want %d", table, count, want)
		}
	}
	assertJournalAccountBalance(t, ctx, journalService, tenantID, journal.AccountCash, 630_000)
	assertJournalAccountBalance(t, ctx, journalService, tenantID, journal.AccountOwnerPayable, 0)
	assertJournalAccountBalance(t, ctx, journalService, tenantID, journal.AccountRevenue, -630_000)
	assertJournalAccountBalance(t, ctx, journalService, tenantID, journal.AccountCommissionExpense, 300_000)
	assertJournalAccountBalance(t, ctx, journalService, tenantID, journal.AccountCommissionPayable, -300_000)

	// Gross revenue agrees with the journal: 5,000 of intermediation plus
	// 1,300 of administration. The broker obligation was accrued separately;
	// the owner's 6,700 must never enter agency revenue.
	revenue, err := repositories.NewFinancialReportPostgresRepository(db).RevenueByType(ctx, tenantID, startAt, startAt.AddDate(0, 3, 0))
	if err != nil {
		t.Fatalf("rental revenue report: %v", err)
	}
	var ownRevenueCents int64
	for _, bucket := range revenue {
		ownRevenueCents += bucket.TotalCents
	}
	if ownRevenueCents != 630_000 {
		t.Fatalf("rental gross revenue = %d, want 630000 including administration", ownRevenueCents)
	}
}

type rejectManualRentFundingPayoutRepository struct {
	payout_order.Repository
	err error
}

func (r rejectManualRentFundingPayoutRepository) UpdateIfStatus(context.Context, *payout_order.PayoutOrder, payout_order.Status, time.Time) error {
	return r.err
}
