package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/clients"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
)

// TestManualSaleFinancialJourneyPostgresClosesCashAndLiabilities exercises the
// operator journey against the migrated PostgreSQL boundary: post a sale,
// settle the receipt, approve/pay broker commissions, approve/process the
// seller transfer, and retry every idempotent money-moving command.
func TestManualSaleFinancialJourneyPostgresClosesCashAndLiabilities(t *testing.T) {
	db := openFinancialConsistencyPostgres(t)
	tenantID := uuid.NewString()
	dealID := uuid.NewString()
	contractID := uuid.NewString()
	ownerID := uuid.NewString()
	propertyID := uuid.NewString()
	brokerIDs := []string{uuid.NewString(), uuid.NewString()}
	operationKey := "manual-sale-journey:" + uuid.NewString()
	ctx, cancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 45*time.Second)
	defer cancel()

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 15*time.Second)
		defer cleanupCancel()
		if _, err := deleteJournalFixturesInTest(cleanupCtx, db, `SELECT id::text
			FROM financial.journal_entries WHERE tenant_id = $1`, tenantID); err != nil {
			t.Errorf("cleanup manual sale journal entries: %v", err)
		}
		for _, table := range []string{"transfers", "commissions", "transactions"} {
			if _, err := db.Exec(cleanupCtx, fmt.Sprintf("DELETE FROM financial.%s WHERE tenant_id = $1", table), tenantID); err != nil {
				t.Errorf("cleanup manual sale %s: %v", table, err)
			}
		}
		var residual int
		if err := db.QueryRow(cleanupCtx, `SELECT
			(SELECT COUNT(*) FROM financial.transactions WHERE tenant_id = $1) +
			(SELECT COUNT(*) FROM financial.commissions WHERE tenant_id = $1) +
			(SELECT COUNT(*) FROM financial.transfers WHERE tenant_id = $1) +
			(SELECT COUNT(*) FROM financial.journal_entries WHERE tenant_id = $1)`, tenantID).Scan(&residual); err != nil {
			t.Errorf("verify manual sale cleanup: %v", err)
		} else if residual != 0 {
			t.Errorf("manual sale cleanup left %d rows for tenant %s", residual, tenantID)
		}
	})

	txRepo := repositories.NewTransactionPostgresRepository(db)
	commissionRepo := repositories.NewCommissionPostgresRepository(db)
	transferRepo := repositories.NewTransferPostgresRepository(db)
	journalRepo := repositories.NewJournalPostgresRepository(db)
	journalService := NewJournalService(journalRepo).WithDB(db)
	transferService := NewTransferService(transferRepo).
		WithDB(db).
		WithJournalService(journalService)
	commissionService := NewCommissionService(commissionRepo).WithDB(db)
	transactionService := NewTransactionService(txRepo).
		WithDB(db).
		WithManualCommissionService(commissionService).
		WithJournalService(journalService)
	transactionService.WithManualReceiptSettlement(
		NewManualReceiptSettlementService(transferService, fixedManualReceiptDealLookup{deal: &clients.DealDTO{
			ID: dealID, TenantID: tenantID, Type: "SALE", Value: 980_000,
			OwnerID: ownerID, PropertyID: propertyID,
		}}).WithJournalService(journalService),
	)
	commissionService.WithManualTransactionService(transactionService)

	items := []BatchCreateInput{
		{ContractID: contractID, DealID: dealID, Type: string(transaction.TypeReceita), Description: "Recebimento - Venda", Value: 980_000, DateStr: "2026-08-29", Flow: "sale"},
		{ContractID: contractID, Type: string(transaction.TypeComissao), Description: "Comissão - captador", Value: 11_760, DateStr: "2026-08-29", BrokerID: brokerIDs[0], Role: "captador"},
		{ContractID: contractID, Type: string(transaction.TypeComissao), Description: "Comissão - fechamento", Value: 23_520, DateStr: "2026-08-29", BrokerID: brokerIDs[1], Role: "fechamento"},
	}
	commissionInputs := []ManualCommissionInput{
		{DealID: dealID, ContractID: contractID, BrokerID: brokerIDs[0], Role: "captador", Percentage: 20, Amount: 11_760},
		{DealID: dealID, ContractID: contractID, BrokerID: brokerIDs[1], Role: "fechamento", Percentage: 40, Amount: 23_520},
	}
	retention := &ManualCommissionRetentionInput{Percentage: 40, Amount: 23_520}
	outputs, err := transactionService.BatchCreatePosting(ctx, tenantID, operationKey, items, commissionInputs, retention)
	if err != nil {
		t.Fatalf("post manual sale: %v", err)
	}
	if len(outputs) != 3 || outputs[0].Transaction == nil {
		t.Fatalf("manual sale outputs = %#v", outputs)
	}
	receipt := outputs[0].Transaction
	provisioned, err := journalService.AccountBalanceByFlow(ctx, tenantID, journal.AccountCommissionPayable)
	if err != nil || provisioned["sale"] != -3_528_000 {
		t.Fatalf("sale commission provision by flow = %+v, err=%v", provisioned, err)
	}
	assertJournalAccountBalance(t, ctx, journalService, tenantID, journal.AccountCash, 0)
	if receipt.EconomicBreakdown == nil || receipt.EconomicBreakdown.OwnerPayableCents != 92_120_000 ||
		receipt.EconomicBreakdown.CommissionPayableCents != 3_528_000 || receipt.EconomicBreakdown.AgencyRetainedCents != 2_352_000 {
		t.Fatalf("manual sale waterfall = %+v", receipt.EconomicBreakdown)
	}

	// Rewrites only the textual timezone representation. The old string CAS
	// returned a false 409 here; the timestamptz CAS must accept this revision.
	rewriteJSONAggregateUpdatedAtAsUTCOffset(t, ctx, db, "transactions", tenantID, receipt.ID)
	paidAt := time.Date(2026, time.August, 29, 18, 30, 0, 123456000, time.UTC)
	settledReceipt, err := transactionService.Settle(ctx, receipt.ID, tenantID, &paidAt, "PIX")
	if err != nil {
		t.Fatalf("settle manual sale receipt: %v", err)
	}
	if transaction.CanonicalStatus(settledReceipt.Status) != transaction.StatusPaid {
		t.Fatalf("receipt status = %s, want PAID", settledReceipt.Status)
	}

	identityRepo, ok := transferRepo.(transferTransactionIdentityRepository)
	if !ok {
		t.Fatal("postgres transfer repository does not expose source transaction identity")
	}
	ownerTransfer, err := identityRepo.GetBySourceTransactionID(ctx, tenantID, receipt.ID)
	if err != nil {
		t.Fatalf("load owner transfer: %v", err)
	}
	if ownerTransfer == nil || ownerTransfer.Status != transfer.StatusPending || ownerTransfer.NetAmount != 921_200 {
		t.Fatalf("owner obligation after receipt = %+v", ownerTransfer)
	}

	paidCommissions := make([]*commission.Commission, 0, len(commissionInputs))
	for i := range commissionInputs {
		sourceID := events.DeriveNotificationID(operationKey, "manual_commission", tenantID, fmt.Sprint(i))
		created, err := commissionRepo.GetBySourceEventID(ctx, tenantID, sourceID)
		if err != nil || created == nil {
			t.Fatalf("load manual commission %d: commission=%+v err=%v", i, created, err)
		}
		if i == 0 {
			for _, deduction := range []commission.Deduction{
				{Kind: commission.DeductionIRRF, Description: "IRRF retido", Amount: 100},
				{Kind: commission.DeductionOutros, Description: "Adiantamento", Amount: 50},
			} {
				created, err = commissionService.AddDeduction(ctx, tenantID, created.ID, deduction)
				if err != nil {
					t.Fatalf("add commission deduction %s: %v", deduction.Kind, err)
				}
			}
			if created.NetAmount != 11_610 {
				t.Fatalf("commission net after deductions = %.2f, want 11610.00", created.NetAmount)
			}
		}
		if _, err := commissionService.Approve(ctx, tenantID, created.ID, "approver-journey"); err != nil {
			t.Fatalf("approve manual commission %d: %v", i, err)
		}
		paid, err := commissionService.MarkPaidWithProof(ctx, tenantID, created.ID, "/api/v1/proofs/content/integration-proof", "Pagamento confirmado", "payer-journey")
		if err != nil {
			t.Fatalf("pay manual commission %d: %v", i, err)
		}
		if paid.Status != commission.StatusPaid || paid.ProofURL == "" {
			t.Fatalf("paid manual commission %d = %+v", i, paid)
		}
		ledger, err := txRepo.GetByID(ctx, tenantID, paid.LedgerTransactionID)
		if err != nil || transaction.CanonicalStatus(ledger.Status) != transaction.StatusPaid {
			t.Fatalf("linked ledger %d not settled: ledger=%+v err=%v", i, ledger, err)
		}
		// Approval and payment retries are no-ops that still validate/restore the
		// canonical linked accounting entry.
		if _, err := commissionService.Approve(ctx, tenantID, created.ID, "approver-journey"); err != nil {
			t.Fatalf("retry commission approval %d: %v", i, err)
		}
		if _, err := commissionService.MarkPaidWithProof(ctx, tenantID, created.ID, paid.ProofURL, paid.Note, "payer-journey"); err != nil {
			t.Fatalf("retry commission payment %d: %v", i, err)
		}
		paidCommissions = append(paidCommissions, paid)
	}

	rewriteJSONAggregateUpdatedAtAsUTCOffset(t, ctx, db, "transfers", tenantID, ownerTransfer.ID)
	if err := transferService.Approve(ctx, tenantID, ownerTransfer.ID, "approver-journey", "Aprovador"); err != nil {
		t.Fatalf("approve owner transfer: %v", err)
	}
	if err := transferService.Approve(ctx, tenantID, ownerTransfer.ID, "approver-journey", "Aprovador"); err != nil {
		t.Fatalf("retry owner transfer approval: %v", err)
	}
	if err := transferService.Process(ctx, tenantID, ownerTransfer.ID, "payer-journey", "Pagador"); err != nil {
		t.Fatalf("process owner transfer: %v", err)
	}
	if err := transferService.Process(ctx, tenantID, ownerTransfer.ID, "payer-journey", "Pagador"); err != nil {
		t.Fatalf("retry owner transfer processing: %v", err)
	}
	processed, err := transferRepo.GetByID(ctx, tenantID, ownerTransfer.ID)
	if err != nil || processed == nil || processed.Status != transfer.StatusProcessed {
		t.Fatalf("processed owner transfer = %+v err=%v", processed, err)
	}
	if countTransferHistoryAction(processed, "approved") != 1 || countTransferHistoryAction(processed, "processed") != 1 {
		t.Fatalf("owner transfer retry duplicated history: %+v", processed.History)
	}

	// A receipt retry after all downstream payments must validate the same
	// canonical waterfall without creating a second transfer or journal.
	if _, err := transactionService.Settle(ctx, receipt.ID, tenantID, &paidAt, "PIX"); err != nil {
		t.Fatalf("retry manual sale receipt settlement: %v", err)
	}

	wantSources := []string{
		manualSaleReceiptJournalSourceID(settledReceipt),
		manualCommissionPaymentJournalSourceID(paidCommissions[0]),
		manualCommissionPaymentJournalSourceID(paidCommissions[1]),
		ownerTransferSettlementSourceEventID(tenantID, ownerTransfer.ID),
	}
	var journalCount int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM financial.journal_entries
		WHERE tenant_id = $1 AND deleted_at IS NULL
		  AND (metadata->>'source_event_id') IN ($2, $3, $4, $5)`,
		tenantID, wantSources[0], wantSources[1], wantSources[2], wantSources[3]).Scan(&journalCount); err != nil {
		t.Fatalf("count manual sale journals: %v", err)
	}
	if journalCount != 4 {
		t.Fatalf("manual sale journal count = %d, want 4", journalCount)
	}
	assertJournalAccountBalance(t, ctx, journalService, tenantID, journal.AccountCash, 2_367_000)
	assertJournalAccountBalance(t, ctx, journalService, tenantID, journal.AccountOwnerPayable, 0)
	assertJournalAccountBalance(t, ctx, journalService, tenantID, journal.AccountCommissionPayable, 0)
	remainingByFlow, err := journalService.AccountBalanceByFlow(ctx, tenantID, journal.AccountCommissionPayable)
	if err != nil || remainingByFlow["sale"] != 0 {
		t.Fatalf("settled sale commission provision by flow = %+v, err=%v", remainingByFlow, err)
	}
	assertJournalAccountBalance(t, ctx, journalService, tenantID, journal.AccountTaxWithheld, -10_000)
	assertJournalAccountBalance(t, ctx, journalService, tenantID, journal.AccountOtherPayable, -5_000)
	// GROSS model: the receipt recognizes the full commission as revenue
	// (3_528_000 brokers + 2_352_000 retention) and the broker share is expensed.
	assertJournalAccountBalance(t, ctx, journalService, tenantID, journal.AccountRevenue, -5_880_000)
	assertJournalAccountBalance(t, ctx, journalService, tenantID, journal.AccountCommissionExpense, 3_528_000)

	// The realized report must use the same gross model as the posted journal:
	// broker expenses count once, while the seller's 921,200 stays out of DRE.
	reports := repositories.NewFinancialReportPostgresRepository(db)
	dre, err := NewFinancialReportService(reports).DRE(ctx, tenantID, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("manual sale DRE: %v", err)
	}
	var revenueCents, expenseCents int64
	for _, category := range dre.Revenue {
		revenueCents += category.TotalCents
	}
	for _, category := range dre.Expense {
		expenseCents += category.TotalCents
	}
	if revenueCents != 5_880_000 || expenseCents != 3_528_000 || dre.NetCents != 2_352_000 {
		t.Fatalf("manual sale DRE: revenue=%d expense=%d net=%d", revenueCents, expenseCents, dre.NetCents)
	}
	cashflow, err := reports.CashFlowByDay(ctx, tenantID, dre.From, dre.To)
	if err != nil {
		t.Fatalf("manual sale cash flow: %v", err)
	}
	var cashNet int64
	for _, day := range cashflow {
		cashNet += day.InflowCents - day.OutflowCents
	}
	if cashNet != 2_367_000 {
		t.Fatalf("cash flow net = %d, want 2367000 excluding unpaid withholdings", cashNet)
	}

	position, err := repositories.NewFinancialReportPostgresRepository(db).KpisSnapshot(ctx, tenantID)
	if err != nil {
		t.Fatalf("financial position after manual sale journey: %v", err)
	}
	if position.CashBalanceCents != 2_367_000 || position.ThirdPartyObligationsCents != 0 ||
		position.OtherCommitmentsCents != 15_000 || position.CommittedFundsCents != 15_000 ||
		position.AgencyAvailableCashCents != 2_352_000 || position.CoverageGapCents != 0 ||
		position.UnexpectedPayableDebitCents != 0 {
		t.Fatalf("financial position after manual sale journey = %#v", position)
	}
}

type fixedManualReceiptDealLookup struct {
	deal *clients.DealDTO
}

func (f fixedManualReceiptDealLookup) GetDeal(_ context.Context, tenantID, dealID string) (*clients.DealDTO, error) {
	if f.deal == nil || f.deal.TenantID != tenantID || f.deal.ID != dealID {
		return nil, fmt.Errorf("deal %s not found for tenant %s", dealID, tenantID)
	}
	copy := *f.deal
	return &copy, nil
}

func rewriteJSONAggregateUpdatedAtAsUTCOffset(t *testing.T, ctx context.Context, db *database.PostgresDB, table, tenantID, id string) {
	t.Helper()
	if table != "transactions" && table != "transfers" {
		t.Fatalf("unsupported aggregate table %q", table)
	}
	query := fmt.Sprintf(`UPDATE financial.%s
		SET metadata = jsonb_set(
			metadata,
			'{updated_at}',
			to_jsonb(to_char((metadata->>'updated_at')::timestamptz AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US') || '+00:00'),
			false
		)
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
		RETURNING metadata->>'updated_at'`, table)
	var rewritten string
	if err := db.QueryRow(ctx, query, id, tenantID).Scan(&rewritten); err != nil {
		t.Fatalf("rewrite %s timestamp offset: %v", table, err)
	}
	if !strings.HasSuffix(rewritten, "+00:00") {
		t.Fatalf("rewritten %s timestamp = %q, want +00:00", table, rewritten)
	}
}

func assertJournalAccountBalance(t *testing.T, ctx context.Context, service *JournalService, tenantID string, account journal.Account, want int64) {
	t.Helper()
	got, err := service.AccountBalance(ctx, tenantID, account)
	if err != nil {
		t.Fatalf("account balance %s: %v", account, err)
	}
	if got != want {
		t.Fatalf("account balance %s = %d, want %d", account, got, want)
	}
}
