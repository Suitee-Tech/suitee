package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/health"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/pkg/tracing"
	"github.com/oginoino/toca/toca-financial/internal/handlers"
	repository "github.com/oginoino/toca/toca-financial/internal/repositories"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// @title           Toca Financial API
// @version         1.0
// @description     Financial service for Toca ecosystem
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8085
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

func main() {

	// Initialize OpenTelemetry
	shutdown, err := tracing.InitProvider("toca-financial")
	if err == nil {
		defer func() { _ = shutdown(context.Background()) }()
	} else {
		log.Printf("Failed to initialize OpenTelemetry: %v", err)
	}
	// Load configuration
	cfg := config.LoadBaseConfig()

	// Setup Logger
	logger.Setup(logger.Config{
		Level:  "info",
		Format: "json",
	})
	logger.Info("Starting financial service", "port", cfg.Port)

	// Fuso da imobiliária (Configurações → "Fuso horário") manda nos dias civis
	// dos lançamentos e relatórios. O resolvedor lê o valor do toca-core por
	// rota interna; sem configuração ou sem core no ar, o timeutil usa o padrão
	// do ambiente (America/Sao_Paulo).
	timeutil.SetTenantResolver(clients.NewCoreClient().TenantLocationResolver())

	// Context with cancellation
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	pgDB, err := database.ConnectPostgres(context.Background(), cfg.Database)
	if err != nil {
		logger.Error("Failed to connect to Postgres", "error", err)
		os.Exit(1)
	}
	defer pgDB.Close()
	logger.Info("Connected to Postgres database", "name", cfg.Database.Name)

	// Audit recorder for Rental + Invoice mutations.
	audit.SetDefault(audit.NewRecorder(pgDB))

	// Setup Services (needed for event handler)
	commRepo := repository.NewCommissionPostgresRepository(pgDB)
	commService := service.NewCommissionService(commRepo).WithDB(pgDB).WithContractLookup(clients.NewDealClient())

	chargeRepo := repository.NewChargePostgresRepository(pgDB)
	rentalConfigRepo := repository.NewRentalChargeConfigPostgresRepository(pgDB)
	rentalConfigService := service.NewRentalChargeConfigService(rentalConfigRepo, chargeRepo, clients.NewDealClient())
	rentalConfigService.SetCrmClient(clients.NewCrmClient())

	// Phase 2.6 — consolidated Rental aggregate. Wired through to the
	// contract.signed handler so a Rental row is created next to the
	// existing rental_charge_config when a RENT contract is signed. The
	// service is built inside the rb branch below once the event bus
	// exists, so we only declare the repo here.
	rentalRepo := repository.NewRentalPostgresRepository(pgDB)

	chargeService := service.NewChargeService(chargeRepo, rentalConfigRepo).WithDB(pgDB)
	journalService := service.NewJournalService(repository.NewJournalPostgresRepository(pgDB)).WithDB(pgDB)
	chargeService.SetPaymentJournal(journalService)

	// End-to-end billing feature (sale plans + payout orchestration):
	//   - SaleBillingPlanService pre-generates the SALE installments at
	//     contract.signed.
	//   - PayoutOrderService is the operator-driven CRUD over manual
	//     splits/repasses.
	//   - PayoutOrchestrationService materializes payout_orders when a
	//     charge transitions to PAID (wired into the charge.paid handler).
	saleBillingPlanRepo := repository.NewSaleBillingPlanPostgresRepository(pgDB)
	saleBillingPlanDraftRepo := repository.NewSaleBillingPlanDraftPostgresRepository(pgDB)
	saleBillingPlanService := service.NewSaleBillingPlanService(saleBillingPlanRepo, chargeService).
		WithDraftRepo(saleBillingPlanDraftRepo)
	payoutOrderRepo := repository.NewPayoutOrderPostgresRepository(pgDB)
	chargeService.WithSaleCommissionFunding(
		service.NewSaleCommissionFundingService(commService, payoutOrderRepo).RequirePaymentTransaction(),
	)
	// PayoutOrderService is consumed by the HTTP layer (built in
	// routes.go); the orchestration service below is what the event
	// handlers use to materialize payout_orders.
	payoutOrchestration := service.NewPayoutOrchestrationService(payoutOrderRepo, chargeRepo, clients.NewDealClient())
	payoutOrchestration.SetCommissionService(commService)
	rentalIntermediationService := service.NewRentalIntermediationService(rentalConfigRepo, commService, payoutOrderRepo).
		RequirePaymentTransaction()
	payoutOrchestration.SetRentalIntermediation(rentalIntermediationService)

	transferRepo := repository.NewTransferPostgresRepository(pgDB)
	transferService := service.NewTransferService(transferRepo).
		WithDB(pgDB).
		WithJournalService(journalService)
	managedBillRepo := repository.NewManagedBillPostgresRepository(pgDB)
	managedBillService := service.NewManagedBillService(managedBillRepo, transferRepo).
		WithDB(pgDB).
		WithJournalService(journalService)

	txRepo := repository.NewTransactionPostgresRepository(pgDB)
	txService := service.NewTransactionService(txRepo).
		WithDB(pgDB).
		WithManualCommissionService(commService).
		WithJournalService(journalService).
		WithManualReceiptSettlement(
			service.NewManualReceiptSettlementService(transferService, clients.NewDealClient()).
				WithJournalService(journalService).
				WithRentalIntermediation(rentalIntermediationService),
		)
	commService.WithManualTransactionService(txService)

	// Cross-company partnership financials — crossRepo reads core.properties
	// and core.partnerships cross-schema; partnerRoyaltyService persists
	// franchise royalty payables in financial.partner_royalties.
	crossRepo := repository.NewCrossPostgresRepository(pgDB)
	partnerRoyaltyRepo := repository.NewPartnerRoyaltyPostgresRepository(pgDB)
	partnerRoyaltyService := service.NewPartnerRoyaltyService(partnerRoyaltyRepo)
	// Let the orchestration provision a royalty_settlement payout per franchise
	// royalty at contract.signed (one-time accrued amount), settleable via the
	// payout rails and visible in the inbox's royalty bucket.
	payoutOrchestration.SetRoyaltyLookup(partnerRoyaltyRepo)

	// Phase 6 (end-to-end financial) — tenant defaults + commission rules.
	// commissionRulesService injetado no contract.signed handler para
	// resolver per-tenant overrides (fecha o TODO #420 em event_handler).
	// tenantFinancialSettingsService injetado em delinquency + rental config
	// para fallback chain contract → tenant → global.
	tenantSettingsRepo := repository.NewTenantFinancialSettingsPostgresRepository(pgDB)
	tenantSettingsService := service.NewTenantFinancialSettingsService(tenantSettingsRepo)
	txService.WithManualReceiptSettings(tenantSettingsService)
	chargeService.SetPaymentAllocationResolver(
		service.NewEconomicAllocationService(tenantSettingsService).
			WithRentalIntermediation(rentalIntermediationService),
	)
	commissionRulesRepo := repository.NewTenantCommissionRulesPostgresRepository(pgDB)
	commissionRulesService := service.NewCommissionRulesService(commissionRulesRepo)
	// owner_statement_service é instanciado pelo router (consome transferRepo
	// e crossRepo) — não precisa de wire global aqui.
	_ = transferRepo

	// FASE D — Hook que cria o OwnerTransfer mensal ao pagar uma
	// charge. Sem ele, repasse continua manual via Finance modal.
	chargeService.SetTransferHook(
		service.NewTransferChargeHook(transferService, tenantSettingsService).
			WithManagedBillService(managedBillService),
	)

	// Sincroniza Transaction → PAID quando uma Charge é paga, independentemente
	// de a origem ser manual, invoice ou futura integração PSP.
	chargeService.SetTxService(txService)

	// RabbitMQ & Events
	var eventBus events.EventBus
	var rb *events.RabbitMQEventBus
	// SalePlanMaterializationService materializa o snapshot congelado de
	// comissões quando o wizard grava o plano de venda antes da assinatura
	// (POST /sale-billing-plans/contracts/:id). Nil quando o broker de
	// eventos está desligado — os call sites degradam para a composição
	// aplicada na baixa.
	var planMaterializer *service.SalePlanMaterializationService
	if cfg.RabbitMQ.URI != "" {
		rb, err = events.NewRabbitMQEventBusPG(cfg.RabbitMQ.URI, pgDB)
		if err != nil {
			logger.Error("Failed to connect to RabbitMQ", "error", err)
		} else {
			defer rb.Close()
			logger.Info("Connected to RabbitMQ")
			eventBus = rb

			// FASE D — fecha o ciclo do MarkPaid: ChargeService publica
			// charge.paid somente na quitação integral. InvoiceService publica
			// invoice.paid separadamente para a invoice real.
			chargeService.WithEventBus(rb)

			// Register Handlers
			// Comissões financeiras nascem exclusivamente de
			// deal.contract.signed. O antigo deal.converted criava uma
			// segunda comissão fixa de 6% sem conhecer contrato nem split.

			// Build the rental service now that we have an event bus to
			// publish RentalCreatedEvent on. WithDB enables the atomic
			// publish+commit path: the rental row and the rental.created
			// outbox row land or roll back together. See audit P0.1
			// follow-up.
			rentalService := service.NewRentalService(rentalRepo, rb).WithDB(pgDB)
			// Issue #414 — pass commService so SALE and RENT contracts persist
			// Commission domain entities per broker (PENDING → APPROVED →
			// PAID lifecycle). RENT also gates settlement on collected cash.
			//
			// crossRepo + partnerRoyaltyService enable the cross-company
			// step: when a deal closes on a property captured by a partner
			// company, the commission is split per the partnership model and
			// (for franchises) a royalty payable is recorded.
			// Every repository reached by this pipeline honors database.WithTx,
			// including the rental/outbox subflow, whose nested RunInTx reuses the
			// transaction carried by the context. Keep all financial obligations
			// from one delivery atomic (FIN-COM-08).
			contractSignedHandler := service.NewContractSignedEventHandler(pgDB, chargeService, transferService, txService, rentalConfigService, rentalService, commService, crossRepo, partnerRoyaltyService, saleBillingPlanService, commissionRulesService, tenantSettingsService, payoutOrchestration)
			// MESMA chain para o caminho manual (wizard commit
			// pré-assinatura): o snapshot de comissões congelado no
			// contract.signed e o congelado no wizard compartilham steps,
			// transação e chaves de idempotência.
			materializerChain := service.NewContractSignedChain(pgDB, chargeService, transferService, txService, rentalConfigService, rentalService, commService, crossRepo, partnerRoyaltyService, saleBillingPlanService, commissionRulesService, tenantSettingsService, payoutOrchestration)
			planMaterializer = service.NewSalePlanMaterializationService(materializerChain, clients.NewDealClient(), crossRepo, tenantSettingsService)
			// IMPORTANT: the signed event is published as "deal.contract.signed"
			// (pkg/events/contracts/deal_events.go) — same name crm/comm/ops bind
			// to. A previous binding to "contract.signed" never matched on the
			// topic exchange, so the whole provisioning pipeline silently never
			// fired. The queue name is also "deal.contract.signed" to get a clean
			// binding + fresh dedup ledger.
			if err := rb.SubscribeWithQueue("toca-financial.deal.contract.signed", "deal.contract.signed", contractSignedHandler); err != nil {
				logger.Error("Failed to subscribe to deal.contract.signed events", "error", err)
			} else {
				logger.Info("Subscribed to deal.contract.signed")
			}

			chargePaidHandler := service.NewChargePaidEventHandler(payoutOrchestration)
			if err := rb.SubscribeWithQueue("toca-financial.charge.paid", "charge.paid", chargePaidHandler); err != nil {
				logger.Error("Failed to subscribe to charge.paid events", "error", err)
			} else {
				logger.Info("Subscribed to charge.paid")
			}

			// Keep the historical queue during rolling deployments. The adapter
			// accepts only schema_version=0 events once emitted by ChargeService;
			// schema_version=2 events from InvoiceService never trigger payouts.
			legacyInvoicePaidHandler := service.NewLegacyInvoicePaidEventHandler(payoutOrchestration)
			if err := rb.SubscribeWithQueue("toca-financial.invoice.paid", "invoice.paid", legacyInvoicePaidHandler); err != nil {
				logger.Error("Failed to subscribe to invoice.paid events", "error", err)
			} else {
				logger.Info("Subscribed to legacy invoice.paid compatibility events")
			}

			// Phase 3.2 — auto-charge for maintenance tickets per tenant
			// policy. The handler is a no-op when payer_role is empty, so
			// existing tenants without a configured policy keep working.
			// O custo da manutenção entra no próximo documento do responsável:
			// na próxima cobrança em aberto do inquilino ou no próximo repasse
			// pendente do proprietário.
			maintenanceBilling := service.NewMaintenanceBillingService(chargeService, chargeRepo, transferRepo, transferService).
				WithJournal(journalService).
				WithTransactions(txService)
			maintenanceHandler := service.NewMaintenanceRequestedEventHandler(maintenanceBilling)
			if err := rb.SubscribeWithQueue("toca-financial.maintenance.requested", "maintenance.requested", maintenanceHandler); err != nil {
				logger.Error("Failed to subscribe to maintenance.requested events", "error", err)
			} else {
				logger.Info("Subscribed to maintenance.requested")
			}

			// FASE 1 (integração total) — reconcile actual_cost on completion.
			// Creates an extra charge for overruns, applies a discount on the
			// original PENDING charge for underruns, or registers a credit
			// transaction when the original was already PAID.
			maintenanceCompletedHandler := service.NewMaintenanceCompletedEventHandler(chargeService, chargeRepo, txService, maintenanceBilling)
			if err := rb.SubscribeWithQueue("toca-financial.maintenance.completed", "maintenance.completed", maintenanceCompletedHandler); err != nil {
				logger.Error("Failed to subscribe to maintenance.completed events", "error", err)
			} else {
				logger.Info("Subscribed to maintenance.completed")
			}

			// FASE 4 (integração total) — EXIT-inspection deductions become
			// Charge type=OTHER against the locatário so the
			// security-deposit balance reflects each line before rescisão.
			deductionsHandler := service.NewInspectionDeductionsHandler(chargeService)
			if err := rb.SubscribeWithQueue("toca-financial.inspection.deductions_recorded", "inspection.deductions_recorded", deductionsHandler); err != nil {
				logger.Error("Failed to subscribe to inspection.deductions_recorded events", "error", err)
			} else {
				logger.Info("Subscribed to inspection.deductions_recorded")
			}

			// Rental management Phase 1 — react to contract termination
			// (cancel future charges, mark rental rescinded, generate fee
			// charge + caução acerto) and to contract readjustment
			// (update RentalChargeConfig.MonthlyAmount for future generations).
			terminatedHandler := service.NewContractTerminatedEventHandler(chargeService, chargeRepo, transferService, txService, rentalRepo)
			if err := rb.SubscribeWithQueue("toca-financial.contract.terminated", "contract.terminated", terminatedHandler); err != nil {
				logger.Error("Failed to subscribe to contract.terminated events", "error", err)
			} else {
				logger.Info("Subscribed to contract.terminated")
			}
			readjustedHandler := service.NewContractReadjustedEventHandler(rentalConfigService)
			if err := rb.SubscribeWithQueue("toca-financial.contract.readjusted", "contract.readjusted", readjustedHandler); err != nil {
				logger.Error("Failed to subscribe to contract.readjusted events", "error", err)
			} else {
				logger.Info("Subscribed to contract.readjusted")
			}

			rb.StartDLQConsumers()
		}
	}

	collectionRepo := repository.NewCollectionPostgresRepository(pgDB)
	// Fase 2: o serviço de cobrança ganha o acordo (parcelas, baixa e custas) e o
	// motor avisa quando as parcelas fecham, para o acordo virar FULFILLED.
	agreementRepo := repository.NewCollectionAgreementPostgresRepository(pgDB)
	collectionService := service.NewCollectionService(collectionRepo).
		WithChargeRepository(chargeRepo).
		WithAgreementRepository(agreementRepo).
		WithChargeService(chargeService).
		WithTransactionService(txService).
		WithApprovalThreshold(service.AdjustmentApprovalThresholdBRL())
	delinquencyService := service.NewDelinquencyService(chargeRepo, collectionRepo, eventBus, service.DefaultDelinquencyConfig()).
		WithAgreementFulfiller(collectionService.ReconcileAgreement)
	chargeService.SetCollectionResolver(delinquencyService)
	// Wire the deal client so the engine can read per-contract multa/juros
	// overrides; falls back to the global config when nothing is set on the
	// contract. Safe to call even when DEAL_SERVICE_URL isn't configured —
	// the lookup just returns an error and the global default applies.
	delinquencyService.SetDealClient(clients.NewDealClient())
	// Phase 6 — tenant defaults entram no meio da fallback chain (contract
	// → tenant → global). Sem isso, contratos sem override caem direto no
	// config global.
	delinquencyService.SetTenantSettings(tenantSettingsService)
	rentalConfigService.SetTenantSettings(tenantSettingsService)
	// Wire o DB para o distributed lock (#445). Em deploy multi-pod só
	// um pod processa cada tick — outros pulam silenciosamente.
	delinquencyService.WithDB(pgDB)
	delinquencyService.Start()
	defer delinquencyService.Stop()

	// Reminder engine ("Régua de Comunicação"). Built and started in main
	// so the daily ticker survives even if the HTTP router is rebuilt.
	// SetChargeService is what lets it dispatch — without it the engine is
	// inert. ChargeService here needs the same comm/CRM clients the routes
	// wire (already set on `chargeService` above).
	chargeService.SetCommClient(clients.NewCommClient())
	chargeService.SetCrmClient(clients.NewCrmClient())
	reminderRuleRepo := repository.NewReminderRulePostgresRepository(pgDB)
	reminderRuleService := service.NewReminderRuleService(reminderRuleRepo, chargeRepo)
	reminderRuleService.SetChargeService(chargeService)
	// Issue #446 — wire the collection repository so the engine pauses
	// reminders for charges whose Collection já está em NEGOTIATED /
	// JUDICIAL / GUARANTEE_ACTIVATED. Optional: when nil the engine
	// behaves exactly like before (covers tenants that ainda não
	// onboardaram a delinquency engine).
	reminderRuleService.SetCollectionRepo(collectionRepo)
	// F1.1: lock distribuído da régua — em deploy multi-réplica só uma dispara a
	// régua do dia (o dedup diário é read-modify-write em RemindersSent, sem CAS).
	reminderRuleService.WithDB(pgDB)
	reminderRuleService.Start()
	defer reminderRuleService.Stop()

	// Setup Router
	r := handlers.SetupRouter(cfg, pgDB, eventBus, delinquencyService, planMaterializer)
	if rb != nil {
		events.RegisterDLQAdminRoutes(r, rb)
	}
	readiness := health.NewReadinessChecker(health.NewPostgresChecker(pgDB.DB.DB))
	if rb != nil {
		readiness.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterGin(r, "toca-financial", readiness)
	deep := health.NewReadinessChecker(health.NewPostgresDeepChecker(pgDB.DB.DB))
	if rb != nil {
		deep.Add(health.NewDynamicRabbitMQChecker(rb))
	}
	health.RegisterDeepGin(r, "toca-financial", deep)

	// Start Server
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       90 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	go func() {
		logger.Info("Starting financial service", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen failed", "error", err)
		}
	}()

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown:", "error", err)
	}

	logger.Info("Server exiting")
}
