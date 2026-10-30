package services

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
	"github.com/stretchr/testify/require"
)

// TestSaleWizardPaidChargeRepairPostgres reproduces the exact operator
// scenario of the financeiro wizard bug: a SALE plan/charge created without
// deal_id (legacy wizard) was paid with NO allocations. After the plan+charge
// were backfilled with deal_id/owner_id/property_id, a re-reconcile with a
// NEW idempotency key must:
//
//  1. materialize the frozen commission snapshot (ledger + Commission +
//     payout orders) from the deal (wizard path, contract never signed),
//  2. apply the sale economic composition to the paid charge,
//  3. rebuild the allocations of the existing zero-allocation payment,
//  4. create the OwnerTransfer (repasse) and the allocated payment journal.
//
// The test is self-contained against TOCA_TEST_DATABASE_URI + RabbitMQ; it
// reuses the REAL wiring of main.go/router.go (production services).
func TestSaleWizardPaidChargeRepairPostgres(t *testing.T) {
	databaseURI := os.Getenv("TOCA_TEST_DATABASE_URI")
	if databaseURI == "" {
		t.Skip("TOCA_TEST_DATABASE_URI is not configured")
	}
	rabbitURI := os.Getenv("TOCA_TEST_RABBITMQ_URI")
	if rabbitURI == "" {
		t.Skip("TOCA_TEST_RABBITMQ_URI is not configured")
	}
	if os.Getenv("DEAL_SERVICE_URL") == "" {
		t.Skip("DEAL_SERVICE_URL is not configured")
	}

	// The user's real rows (from the financeiro wizard bug).
	const (
		tenantID   = "b6c50a56-d5f1-42e7-87ad-fdd2e924e0e0"
		contractID = "2cd10776-e43c-4a83-ace0-74eb98a0e23c"
		dealID     = "148c04fe-94dc-4690-9da0-545b2018e0c3"
		chargeID   = "4af4a6c5-d949-4a55-ba5b-7dbdecc91609"
	)

	ctx, cancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), tenantID), 90*time.Second)
	defer cancel()

	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{
		URI:         databaseURI,
		MaxPoolSize: 12,
	})
	require.NoError(t, err)
	defer db.Close()

	rb, err := events.NewRabbitMQEventBusPG(rabbitURI, db)
	require.NoError(t, err)
	defer rb.Close()

	// ── Repositories ────────────────────────────────────────────────────
	chargeRepo := repositories.NewChargePostgresRepository(db)
	journalRepo := repositories.NewJournalPostgresRepository(db)
	invRepo := repositories.NewInvoicePostgresRepository(db)
	commRepo := repositories.NewCommissionPostgresRepository(db)
	payoutRepo := repositories.NewPayoutOrderPostgresRepository(db)
	transferRepo := repositories.NewTransferPostgresRepository(db)
	managedBillRepo := repositories.NewManagedBillPostgresRepository(db)
	txRepo := repositories.NewTransactionPostgresRepository(db)
	rentalConfigRepo := repositories.NewRentalChargeConfigPostgresRepository(db)
	rentalRepo := repositories.NewRentalPostgresRepository(db)
	saleBillingPlanRepo := repositories.NewSaleBillingPlanPostgresRepository(db)
	saleBillingPlanDraftRepo := repositories.NewSaleBillingPlanDraftPostgresRepository(db)
	collectionRepo := repositories.NewCollectionPostgresRepository(db)
	tenantSettingsRepo := repositories.NewTenantFinancialSettingsPostgresRepository(db)
	commissionRulesRepo := repositories.NewTenantCommissionRulesPostgresRepository(db)
	crossRepo := repositories.NewCrossPostgresRepository(db)
	partnerRoyaltyRepo := repositories.NewPartnerRoyaltyPostgresRepository(db)

	// ── Services (mirrors main.go / router.go) ──────────────────────────
	dealClient := clients.NewDealClient()
	journalService := NewJournalService(journalRepo).WithDB(db)
	tenantSettingsService := NewTenantFinancialSettingsService(tenantSettingsRepo)
	commissionRulesService := NewCommissionRulesService(commissionRulesRepo)
	commService := NewCommissionService(commRepo).WithDB(db)
	txService := NewTransactionService(txRepo).WithDB(db)
	transferService := NewTransferService(transferRepo).WithDB(db).WithJournalService(journalService)
	managedBillService := NewManagedBillService(managedBillRepo, transferRepo).WithDB(db).WithJournalService(journalService)
	rentalConfigService := NewRentalChargeConfigService(rentalConfigRepo, chargeRepo, dealClient)
	rentalService := NewRentalService(rentalRepo, rb).WithDB(db)

	chargeService := NewChargeService(chargeRepo, rentalConfigRepo).
		WithDB(db).
		WithEventBus(rb).
		WithInvoiceReservations(invRepo)
	chargeService.SetPaymentJournal(journalService)
	chargeService.SetTransferHook(NewTransferChargeHook(transferService, tenantSettingsService).
		WithManagedBillService(managedBillService))
	chargeService.SetTxService(txService)
	chargeService.SetCollectionResolver(NewDelinquencyService(chargeRepo, collectionRepo, rb, DefaultDelinquencyConfig()))
	chargeService.SetPaymentAllocationResolver(
		NewEconomicAllocationService(tenantSettingsService),
	)

	saleBillingPlanService := NewSaleBillingPlanService(saleBillingPlanRepo, chargeService).
		WithDraftRepo(saleBillingPlanDraftRepo)
	chargeService.WithSaleCommissionFunding(
		NewSaleCommissionFundingService(commService, payoutRepo).RequirePaymentTransaction(),
	)
	payoutOrchestration := NewPayoutOrchestrationService(payoutRepo, chargeRepo, dealClient)
	payoutOrchestration.SetCommissionService(commService)
	rentalIntermediationService := NewRentalIntermediationService(rentalConfigRepo, commService, payoutRepo).
		RequirePaymentTransaction()
	payoutOrchestration.SetRentalIntermediation(rentalIntermediationService)
	payoutOrchestration.SetRoyaltyLookup(partnerRoyaltyRepo)

	financialPreview := NewFinancialPreviewService(dealClient, rentalConfigService, saleBillingPlanService, commissionRulesService, tenantSettingsService)
	financialPreview.SetCrossCompanyLookup(crossRepo)
	chargeService.WithSalePlanResolver(financialPreview)

	materializerChain := NewContractSignedChain(db, chargeService, transferService, txService, rentalConfigService, rentalService, commService, crossRepo, nil, saleBillingPlanService, commissionRulesService, tenantSettingsService, payoutOrchestration)
	planMaterializer := NewSalePlanMaterializationService(materializerChain, dealClient, crossRepo, tenantSettingsService)
	chargeService.WithSalePlanMaterializer(planMaterializer)

	// ── Preconditions (the repaired bug shape) ──────────────────────────
	before, err := chargeService.GetByID(ctx, tenantID, chargeID)
	require.NoError(t, err)
	require.NotNil(t, before)
	require.Equal(t, charge.StatusPaid, before.Status)
	require.Equal(t, dealID, before.DealID, "precondition: charge must carry the backfilled deal_id")
	require.Len(t, before.Payments, 1)
	// Reproduzível: reseta a charge para o formato do bug (sem composição
	// econômica nem alocação) a cada execução do teste.
	if len(before.LineItems) > 0 || len(before.Payments[0].Allocations) > 0 {
		if err := before.SetLineItems(nil); err != nil {
			t.Fatal(err)
		}
		before.Payments[0].Allocations = nil
		before.Payments[0].AllocationMethod = ""
		if err := chargeRepo.Update(ctx, before); err != nil {
			t.Fatalf("reset charge to bug shape: %v", err)
		}
		before, err = chargeService.GetByID(ctx, tenantID, chargeID)
		require.NoError(t, err)
	}
	require.Empty(t, before.LineItems)
	require.Empty(t, before.Payments[0].Allocations)

	// ── Trigger: re-reconcile with a NEW idempotency key (UI does this) ──
	paymentID := "repair-e2e-" + time.Now().UTC().Format("20060102150405.000000000")
	updated, err := chargeService.MarkPaidIdempotent(ctx, tenantID, chargeID, "manual", before.TotalAmount.Float(), paymentID, time.Now().UTC())
	require.NoError(t, err, "re-reconcile of a paid charge with zero allocations must repair, not error")

	// 1. Economic composition frozen on the charge.
	require.NotEmpty(t, updated.LineItems, "charge must carry the sale economic composition after repair")

	// 2. Existing payment allocation rebuilt (the new paymentID must NOT be
	//    appended — the charge was already fully paid).
	require.Len(t, updated.Payments, 1, "repair must not add a second payment")
	require.NotEmpty(t, updated.Payments[0].Allocations, "existing payment must have allocations after repair")
	var ownerAllocated, commissionAllocated, revenueAllocated int64
	for _, a := range updated.Payments[0].Allocations {
		switch {
		case a.AllocationType == charge.AllocationTypeOwnerPayable:
			ownerAllocated += a.AmountCents
		case a.Purpose == charge.AllocationPurposeSaleCommissionBroker:
			// O funding de venda reescreve o pool de comissão como recibos
			// de receita vinculados à Commission do corretor (modelo
			// bruto: receita integral − custo acruado = margem).
			commissionAllocated += a.AmountCents
		case a.AllocationType == charge.AllocationTypeRevenue:
			revenueAllocated += a.AmountCents
		}
	}
	require.Greater(t, ownerAllocated, int64(0), "owner proceeds must be allocated")
	require.Greater(t, commissionAllocated, int64(0), "broker commissions must be allocated")
	require.Greater(t, revenueAllocated, int64(0), "agency retention must be allocated")
	require.Equal(t, before.TotalAmount.Cents(), ownerAllocated+commissionAllocated+revenueAllocated,
		"allocations must close to the paid amount")

	// 3. Frozen commission snapshot materialized from the deal.
	commissions, err := commService.ListByDeal(ctx, tenantID, dealID)
	require.NoError(t, err)
	require.NotEmpty(t, commissions, "wizard-created sale plan must materialize Commission entities")
	for _, c := range commissions {
		require.Equal(t, contractID, c.ContractID)
		require.NotEmpty(t, c.PlanHash)
		require.NotEqual(t, commission.StatusReversed, c.Status)
	}

	// 4. Broker payout orders provisioned at the same plan hash.
	payoutOrders, _, err := payoutRepo.List(ctx, tenantID, payout_order.ListFilter{
		ContractID: contractID,
		Limit:      200,
		Offset:     0,
	})
	require.NoError(t, err)
	require.NotEmpty(t, payoutOrders, "signing payout orders must exist for the contract")

	// 5. OwnerTransfer (repasse) born from the paid charge.
	transfers, _, err := transferRepo.List(ctx, tenantID, "", "", "", "", 200, 0)
	require.NoError(t, err)
	var repasseFound bool
	for _, tr := range transfers {
		if tr.SourceChargeID == chargeID {
			repasseFound = true
			require.Equal(t, transfer.StatusPending, tr.Status)
			require.NotEmpty(t, tr.OwnerID)
		}
	}
	require.True(t, repasseFound, "a paid SALE charge must generate an OwnerTransfer")

	// 6. Once repaired, the charge behaves like any fully paid charge: a new
	//    payment command is rejected (double payment), it never re-applies.
	_, err = chargeService.MarkPaidIdempotent(ctx, tenantID, chargeID, "manual", before.TotalAmount.Float(), paymentID+"-again", time.Now().UTC())
	require.Error(t, err, "second payment on a repaired paid charge must be rejected")
	afterAgain, getErr := chargeService.GetByID(ctx, tenantID, chargeID)
	require.NoError(t, getErr)
	require.Len(t, afterAgain.Payments, 1, "second payment attempt must not add money")
	require.Len(t, afterAgain.Payments[0].Allocations, len(updated.Payments[0].Allocations))
}

// TestPlainEntradaSaidaSettlePostsCashJournalPostgres prova, contra o Postgres
// real, que a baixa de uma entrada (receita) e de uma saída (despesa) avulsas
// registra o movimento no caixa do plano de contas e persiste o comprovante.
func TestPlainEntradaSaidaSettlePostsCashJournalPostgres(t *testing.T) {
	databaseURI := os.Getenv("TOCA_TEST_DATABASE_URI")
	if databaseURI == "" {
		t.Skip("TOCA_TEST_DATABASE_URI is not configured")
	}
	ctx, cancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), "e2e-plain-cash"), 60*time.Second)
	defer cancel()

	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: databaseURI, MaxPoolSize: 12})
	require.NoError(t, err)
	defer db.Close()

	tenantID := uuid.NewString()
	txRepo := repositories.NewTransactionPostgresRepository(db)
	journalRepo := repositories.NewJournalPostgresRepository(db)
	txService := NewTransactionService(txRepo).WithDB(db).WithJournalService(NewJournalService(journalRepo))
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.Exec(cleanupCtx, "DELETE FROM financial.journal_entries WHERE tenant_id = $1", tenantID)
		_, _ = db.Exec(cleanupCtx, "DELETE FROM financial.transactions WHERE tenant_id = $1", tenantID)
	})

	// Entrada avulsa: baixa → cash D / revenue C + comprovante persistido.
	receipt := transaction.New("contract-e2e", transaction.TypeReceita, "Entrada avulsa", 8_000, time.Now().UTC(), tenantID)
	require.NoError(t, txService.Create(ctx, receipt))
	paidAt := time.Now().UTC()
	got, err := txService.SettleWithProof(ctx, receipt.ID, tenantID, &paidAt, "PIX", "https://files.example/entrada-e2e.pdf", "PDF conferido")
	require.NoError(t, err)
	require.Equal(t, "https://files.example/entrada-e2e.pdf", got.ProofURL)
	require.Equal(t, "PDF conferido", got.Note)

	entry, err := journalRepo.GetBySourceEventID(ctx, tenantID, events.DeriveNotificationID("transaction.cash.journal", tenantID, receipt.ID))
	require.NoError(t, err)
	require.NotNil(t, entry, "entrada settle must post the cash journal")
	require.Len(t, entry.Legs, 2)
	require.Equal(t, journal.AccountCash, entry.Legs[0].Account)
	require.Equal(t, int64(800_000), entry.Legs[0].AmountCents)

	// Saída avulsa: baixa → expense D / cash C.
	expense := transaction.New("contract-e2e", transaction.TypeDespesa, "Saída avulsa", 2_400, time.Now().UTC(), tenantID)
	require.NoError(t, txService.Create(ctx, expense))
	_, err = txService.SettleWithProof(ctx, expense.ID, tenantID, &paidAt, "TED", "", "")
	require.NoError(t, err)
	expenseEntry, err := journalRepo.GetBySourceEventID(ctx, tenantID, events.DeriveNotificationID("transaction.cash.journal", tenantID, expense.ID))
	require.NoError(t, err)
	require.NotNil(t, expenseEntry)
	require.Len(t, expenseEntry.Legs, 2)
	require.Equal(t, journal.AccountExpense, expenseEntry.Legs[0].Account)
	require.Equal(t, journal.AccountCash, expenseEntry.Legs[1].Account)
	require.Equal(t, int64(240_000), expenseEntry.Legs[0].AmountCents)

	// Idempotência: re-baixar a entrada (já paga) com nova evidência apenas
	// anexa o comprovante — nunca duplica o journal.
	_, err = txService.SettleWithProof(ctx, receipt.ID, tenantID, &paidAt, "PIX", "https://files.example/entrada-e2e-v2.pdf", "refeito")
	require.NoError(t, err)
	entries, _, err := journalRepo.List(ctx, tenantID, "", 100, 0)
	require.NoError(t, err)
	var cashEntries int
	for _, e := range entries {
		if e.SourceEventID == events.DeriveNotificationID("transaction.cash.journal", tenantID, receipt.ID) {
			cashEntries++
		}
	}
	require.Equal(t, 1, cashEntries, "re-settle must not duplicate the cash journal")
}

// TestTransactionCASMicrosecondPrecisionPostgres é o regressão do 400 em
// /payout-orders/:id/mark-paid ("transaction was modified concurrently"):
// a comparação CAS com timestamp do metadata arredondava de um lado (parse
// do texto, micros) e truncava do outro (param do driver), falhando para
// rows com updated_at de 9 dígitos. A versão esperada agora viaja como texto
// RFC3339 (mesmo padrão de charge/transfer).
func TestTransactionCASMicrosecondPrecisionPostgres(t *testing.T) {
	databaseURI := os.Getenv("TOCA_TEST_DATABASE_URI")
	if databaseURI == "" {
		t.Skip("TOCA_TEST_DATABASE_URI is not configured")
	}
	ctx, cancel := context.WithTimeout(pkgcontext.WithTenantID(context.Background(), "e2e-cas-regression"), 60*time.Second)
	defer cancel()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: databaseURI, MaxPoolSize: 12})
	require.NoError(t, err)
	defer db.Close()

	tenantID := uuid.NewString()
	txRepo := repositories.NewTransactionPostgresRepository(db)
	tx := transaction.New("contract-cas", transaction.TypeReceita, "CAS regression", 100, time.Now().UTC(), tenantID)
	require.NoError(t, txRepo.Create(ctx, tx))

	loaded, err := txRepo.GetByID(ctx, tenantID, tx.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// updated_at com 9 dígitos (produzido por time.Now()) — a comparação CAS
	// precisa casar com a versão lida mesmo com o arredondamento do Postgres.
	versioned, ok := txRepo.(interface {
		UpdateIfVersion(ctx context.Context, t *transaction.Transaction, expected time.Time) (bool, error)
	})
	require.True(t, ok, "transaction repository must expose the versioned CAS")
	loaded.Description = "CAS regression updated"
	updated, err := versioned.UpdateIfVersion(ctx, loaded, loaded.UpdatedAt)
	require.NoError(t, err)
	require.True(t, updated, "CAS must match the read revision regardless of microsecond rounding")

	// Versão obsoleta deve falhar (nunca aplicar duas escritas).
	updated, err = versioned.UpdateIfVersion(ctx, loaded, loaded.UpdatedAt.Add(-time.Millisecond))
	require.NoError(t, err)
	require.False(t, updated, "stale revision must be rejected")

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.Exec(cleanupCtx, "DELETE FROM financial.transactions WHERE tenant_id = $1", tenantID)
	})
}
