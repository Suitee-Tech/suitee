package handlers

import (
	stdctx "context"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/apidocs"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	pkgmw "github.com/oginoino/toca/pkg/middleware"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/pkg/tracing"
	"github.com/oginoino/toca/pkg/validation"
	_ "github.com/oginoino/toca/toca-financial/docs"
	"github.com/oginoino/toca/toca-financial/internal/providers"
	repository "github.com/oginoino/toca/toca-financial/internal/repositories"
	service "github.com/oginoino/toca/toca-financial/internal/services"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func SetupRouter(
	cfg config.BaseConfig,
	pgDB *database.PostgresDB,
	eventBus events.EventBus,
	delinquencyService *service.DelinquencyService,
	planMaterializer *service.SalePlanMaterializationService,
) *gin.Engine {
	// Initialize JWKS for Supabase JWT validation
	if err := auth.InitJWKS(cfg.JWT.SupabaseJWKSURL); err != nil {
		logger.Error("Failed to initialize JWKS", map[string]interface{}{"error": err.Error()})
	}

	// P1/A2: registra no boot se o serviço está no modelo antigo (chave
	// compartilhada, sem allowlist de tenant) — em produção isso é alarme.
	auth.LogServiceKeyRollout()

	_ = validation.InitGin()

	r := gin.New()
	// Global middleware must be registered before routes. Gin snapshots the
	// handler chain when a route is created, so adding these in main after
	// SetupRouter returned left all financial/fiscal routes without request
	// IDs, tracing and the 60-second outer deadline.
	r.Use(pkgmw.RequestID())
	r.Use(gin.Recovery())
	r.Use(pkgmw.SetupLogger())
	pkgmw.ApplyTrustedProxies(r)
	r.Use(metrics.GinMiddleware("toca-financial"))
	r.Use(tracing.Middleware("toca-financial"))
	r.Use(pkgmw.RequestTimeout(60 * time.Second))

	r.GET("/metrics", metrics.Handler())

	// Health check (no auth)
	r.Any("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "toca-financial"})
	})

	// Dependencies
	fiscalRepo := repository.NewFiscalPostgresRepository(pgDB)

	commRepo := repository.NewCommissionPostgresRepository(pgDB)
	commService := service.NewCommissionService(commRepo).WithDB(pgDB)
	// Tributos retidos a recolher: o desconto de IRRF/INSS/ISS na comissão vira
	// obrigação da imobiliária (competência, vencimento, baixa com comprovante e
	// lançamento contábil do recolhimento).
	brokerAdvanceRepo := repository.NewBrokerAdvancePostgresRepository(pgDB)
	brokerAdvanceService := service.NewBrokerAdvanceService(brokerAdvanceRepo)
	taxObligationRepo := repository.NewTaxObligationPostgresRepository(pgDB)
	taxObligationService := service.NewTaxObligationService(taxObligationRepo, commRepo)
	commHandler := NewCommissionHandler(commService)

	invRepo := repository.NewInvoicePostgresRepository(pgDB)

	txRepo := repository.NewTransactionPostgresRepository(pgDB)
	txService := service.NewTransactionService(txRepo).
		WithDB(pgDB).
		WithManualCommissionService(commService)
	commService.WithManualTransactionService(txService)
	txHandler := NewTransactionHandler(txService)

	journalRepo := repository.NewJournalPostgresRepository(pgDB)
	journalService := service.NewJournalService(journalRepo).WithDB(pgDB)
	// A baixa do tributo retido fecha o razão (débito em 2.1.05 / crédito caixa).
	taxObligationService.WithJournal(journalService)
	// O adiantamento também é fato de caixa: concessão escriturada no razão.
	brokerAdvanceService.WithJournal(journalService)
	commService.WithTaxObligationService(taxObligationService).WithBrokerAdvanceService(brokerAdvanceService)
	txService.WithJournalService(journalService)
	withdrawalRepo := repository.NewWithdrawalPostgresRepository(pgDB)
	withdrawalService := service.NewWithdrawalService(withdrawalRepo).
		WithDB(pgDB).
		WithJournalService(journalService)
	withdrawalHandler := NewWithdrawalHandler(withdrawalService)

	// Consolidated financial reports (read-only aggregates).
	financialReportRepo := repository.NewFinancialReportPostgresRepository(pgDB)
	financialReportService := service.NewFinancialReportService(financialReportRepo)
	financialReportHandler := NewFinancialReportHandler(financialReportService)

	// Reconciliation dry-run report (FIN-COM-14)
	reconciliationRepo := repository.NewReconciliationPostgresRepository(pgDB)
	reconciliationService := service.NewReconciliationService(reconciliationRepo)
	reconciliationHandler := NewReconciliationHandler(reconciliationService)

	chargeRepo := repository.NewChargePostgresRepository(pgDB)
	tenantIdentityRepo := repository.NewTenantIdentityPostgresRepository(pgDB)
	rentalConfigRepo := repository.NewRentalChargeConfigPostgresRepository(pgDB)
	rentalConfigService := service.NewRentalChargeConfigService(rentalConfigRepo, chargeRepo, clients.NewDealClient())
	rentalConfigService.SetCrmClient(clients.NewCrmClient())
	rentalConfigHandler := NewRentalChargeConfigHandler(rentalConfigService)

	chargeService := service.NewChargeService(chargeRepo, rentalConfigRepo).
		WithDB(pgDB).
		WithEventBus(eventBus).
		WithInvoiceReservations(invRepo)
	chargeService.SetPaymentJournal(journalService)
	// P1-05: provedor de pagamentos por ambiente. "sandbox" liga o link
	// determinístico (testes/staging); vazio mantém o endpoint de link
	// respondendo "não configurado" até o gateway real ser escolhido.
	if os.Getenv("PAYMENT_PROVIDER") == "sandbox" {
		chargeService.WithPaymentProvider(providers.NewSandboxPaymentProvider())
	}
	invService := service.NewInvoiceService(invRepo, eventBus).
		WithPaymentApplication(chargeService).
		WithDB(pgDB)
	invHandler := NewInvoiceHandler(invService)
	// Wire downstream clients for the Notify flow. Both are nil-safe inside
	// the service, so missing envs degrade gracefully instead of crashing.
	chargeService.SetCommClient(clients.NewCommClient())
	chargeService.SetCrmClient(clients.NewCrmClient())
	chargeHandler := NewChargeHandler(chargeService).WithTenantCompanyLookup(tenantIdentityRepo)

	// Phase 5.1 — Rental aggregate + portal-scoped handler. Reuses the same
	// rentalRepo/service the contract.signed event handler builds; this
	// router-time instance just wires the HTTP entrypoints.
	rentalRepo := repository.NewRentalPostgresRepository(pgDB)
	rentalService := service.NewRentalService(rentalRepo, eventBus)
	crossRepo := repository.NewCrossPostgresRepository(pgDB)
	portalRentalHandler := NewPortalRentalHandler(rentalService, crossRepo)
	// Phase 4 admin handler — powers /rentals + /rentals/:id used by
	// LeadDetails > Locações and the admin RentalDetails page.
	rentalHandler := NewRentalHandler(rentalService)

	transferRepo := repository.NewTransferPostgresRepository(pgDB)
	managedBillRepo := repository.NewManagedBillPostgresRepository(pgDB)
	managedBillService := service.NewManagedBillService(managedBillRepo, transferRepo).
		WithDB(pgDB).
		WithJournalService(journalService)
	managedBillHandler := NewManagedBillHandler(managedBillService)

	// FiscalService usa o repositório de transfers para reconstruir, a
	// partir dos repasses, as operações de locação do TXT da DIMOB (#418)
	// e os totais anuais por proprietário dos informes (#419);
	// WithCrmClient resolve o nome do proprietário.
	fiscalService := service.NewFiscalService(fiscalRepo).
		WithTransferLister(transferRepo).
		WithCrmClient(clients.NewCrmClient()).
		WithDealClient(clients.NewDealClient()).
		WithPropertyClient(clients.NewPropertyClient()).
		WithNfseProvider(providers.NewSandboxNfseProvider())
	fiscalHandler := NewFiscalHandler(fiscalService)

	// WithEventBus habilita publicação de transfer.processed (#461) —
	// trigger para toca-comm notificar o owner via portal/email/push.
	transferService := service.NewTransferService(transferRepo).
		WithDB(pgDB).
		WithJournalService(journalService).
		WithEventBus(eventBus)
	manualReceiptSettlement := service.NewManualReceiptSettlementService(transferService, clients.NewDealClient()).
		WithJournalService(journalService)
	txService.WithManualReceiptSettlement(manualReceiptSettlement)
	transferHandler := NewTransferHandler(transferService).WithTenantCompanyLookup(tenantIdentityRepo)
	// Reuses the same deal-client used by the delinquency engine to enrich
	// the receipt PDF with contract metadata. CRM client adds the owner
	// lead's name + documentation so the demonstrativo prints real names.
	transferHandler.SetDealClient(clients.NewDealClient())
	transferHandler.SetCrmClient(clients.NewCrmClient())

	// Portal-scoped variant — valida account_id antes de devolver transfer
	// ou renderizar receipt PDF (#455/#465 — LGPD guard). Compartilha o
	// mesmo service + clients do handler admin.
	portalTransferHandler := NewPortalTransferHandler(transferService, crossRepo)
	portalTransferHandler.SetDealClient(clients.NewDealClient())
	portalTransferHandler.SetCrmClient(clients.NewCrmClient())

	// Portal-scoped charges (#460) — gate por account_id via crossRepo.
	// Compartilha o chargeService usado pelo handler admin.
	portalChargeHandler := NewPortalChargeHandler(chargeService, crossRepo)

	// Portal-scoped informes (#419/#459) — owner consulta e baixa o
	// próprio Informe de Rendimentos. Mesmo guard LGPD por account_id.
	portalInformeHandler := NewPortalInformeHandler(fiscalService, crossRepo)

	collectionRepo := repository.NewCollectionPostgresRepository(pgDB)
	agreementRepo := repository.NewCollectionAgreementPostgresRepository(pgDB)
	// O repositório de cobranças entra para validar a criação manual do caso
	// (F0.7): sem ele, `POST /collections` aceitava charge_ids/total_debt livres.
	// Na Fase 2 o serviço também materializa as parcelas do acordo e dá baixa nas
	// cobranças (garantia/judicial), e lança as custas do processo como despesa.
	collectionService := service.NewCollectionService(collectionRepo).
		WithChargeRepository(chargeRepo).
		WithAgreementRepository(agreementRepo).
		WithChargeService(chargeService).
		WithTransactionService(txService).
		WithApprovalThreshold(adjustmentApprovalThresholdBRL())
	collectionHandler := NewCollectionHandler(collectionService)

	utilityAccountRepo := repository.NewUtilityAccountPostgresRepository(pgDB)
	utilityAccountService := service.NewUtilityAccountService(utilityAccountRepo)
	utilityAccountHandler := NewUtilityAccountHandler(utilityAccountService)
	utilityChargeHandler := NewUtilityChargeHandler(chargeService, utilityAccountService, transferService, txService)

	// End-to-end billing feature handlers.
	saleBillingPlanRepo := repository.NewSaleBillingPlanPostgresRepository(pgDB)
	saleBillingPlanDraftRepo := repository.NewSaleBillingPlanDraftPostgresRepository(pgDB)
	saleBillingPlanService := service.NewSaleBillingPlanService(saleBillingPlanRepo, chargeService).
		WithDraftRepo(saleBillingPlanDraftRepo)
	saleBillingPlanHandler := NewSaleBillingPlanHandler(saleBillingPlanService).
		WithDealClient(clients.NewDealClient())

	payoutOrderRepo := repository.NewPayoutOrderPostgresRepository(pgDB)
	chargeService.WithSaleCommissionFunding(
		service.NewSaleCommissionFundingService(commService, payoutOrderRepo).RequirePaymentTransaction(),
	)
	payoutOrderService := service.NewPayoutOrderService(payoutOrderRepo)
	payoutOrchestration := service.NewPayoutOrchestrationService(payoutOrderRepo, chargeRepo, clients.NewDealClient())
	payoutOrderService.SetCommissionService(commService)
	partnerRoyaltyRepo := repository.NewPartnerRoyaltyPostgresRepository(pgDB)
	payoutOrderService.SetPartnerRoyaltyService(service.NewPartnerRoyaltyService(partnerRoyaltyRepo))
	payoutOrchestration.SetCommissionService(commService)
	rentalIntermediationService := service.NewRentalIntermediationService(rentalConfigRepo, commService, payoutOrderRepo).
		RequirePaymentTransaction()
	payoutOrchestration.SetRentalIntermediation(rentalIntermediationService)
	manualReceiptSettlement.WithRentalIntermediation(rentalIntermediationService)
	payoutOrderHandler := NewPayoutOrderHandler(payoutOrderService, payoutOrchestration)
	taxObligationHandler := NewTaxObligationHandler(taxObligationService)
	brokerAdvanceHandler := NewBrokerAdvanceHandler(brokerAdvanceService)

	reminderRuleRepo := repository.NewReminderRulePostgresRepository(pgDB)
	reminderRuleService := service.NewReminderRuleService(reminderRuleRepo, chargeRepo)
	// Wire the charge service in so the engine can fire reminders. Using a
	// setter avoids the init-time cycle that would happen if the
	// constructors were chained.
	reminderRuleService.SetChargeService(chargeService)
	reminderRuleHandler := NewReminderRuleHandler(reminderRuleService)

	delinquencyHandler := NewDelinquencyHandler(delinquencyService)

	// Phase 6 (end-to-end financial) — handlers novos. Tenant settings
	// alimenta defaults financeiros; commission rules expõe a tabela por
	// role; proof upload serve o bucket de comprovantes; owner statement
	// agrega o demonstrativo mensal (admin + portal).
	tenantSettingsRepo := repository.NewTenantFinancialSettingsPostgresRepository(pgDB)
	tenantSettingsService := service.NewTenantFinancialSettingsService(tenantSettingsRepo)
	tenantSettingsHandler := NewTenantFinancialSettingsHandler(tenantSettingsService)
	txService.WithManualReceiptSettings(tenantSettingsService)
	// Política de comprovante das baixas manuais. Vale para os quatro caminhos
	// que confirmam dinheiro movido fora do sistema — comissão, repasse, baixa
	// de lançamento e ordem de pagamento — na borda HTTP, que é por onde a
	// confirmação do operador entra.
	commHandler.SetProofPolicy(tenantSettingsService)
	transferHandler.SetProofPolicy(tenantSettingsService)
	txHandler.SetProofPolicy(tenantSettingsService)
	payoutOrderHandler.SetProofPolicy(tenantSettingsService)
	rentalConfigService.SetTenantSettings(tenantSettingsService)
	chargeService.SetPaymentAllocationResolver(
		service.NewEconomicAllocationService(tenantSettingsService).
			WithRentalIntermediation(rentalIntermediationService),
	)

	// FASE D — TransferChargeHook: cada charge marcada paga dispara
	// a criação do OwnerTransfer mensal idempotente em
	// (tenant, contract, reference_month), fechando o ciclo
	// end-to-end sem depender do webhook do gateway. tenantSettings
	// é o source-of-truth do AdminFeePct (default 10%).
	transferChargeHook := service.NewTransferChargeHook(transferService, tenantSettingsService).
		WithManagedBillService(managedBillService)
	chargeService.SetTransferHook(transferChargeHook)
	chargeService.SetTxService(txService)
	// Retenção de repasse por inadimplência (política opcional do tenant): o
	// serviço de repasse lê a política e a dívida aberta do contrato, e o serviço
	// de cobrança avisa quando o caso fecha para a retenção ser liberada.
	transferService.
		WithDelinquencyPolicy(func(ctx stdctx.Context, tenantID string) service.DelinquencyPolicy {
			ts, err := tenantSettingsService.Get(ctx, tenantID)
			if err != nil || ts == nil {
				return service.DelinquencyPolicy{}
			}
			return service.DelinquencyPolicy{
				Enabled:     ts.HoldTransfersOnDelinquency,
				MinSharePct: ts.DelinquencyHoldMinSharePct,
			}
		}).
		WithOpenCaseDebt(collectionService.OpenCaseDebtForContract)
	collectionService.WithTransferHoldReleaser(transferService.ReleaseHoldsForContract)

	chargeService.SetCollectionResolver(delinquencyService)
	// O motor também libera a retenção quando o caso é resolvido pela quitação das
	// cobranças (o caminho do `POST /collections/:id` cobre a resolução manual).
	if delinquencyService != nil {
		delinquencyService.WithTransferHoldReleaser(func(ctx stdctx.Context, tenantID, contractID string) {
			if _, err := transferService.ReleaseHoldsForContract(ctx, tenantID, contractID, "caso de cobrança resolvido"); err != nil {
				log.Printf("[Router] failed to release transfer holds for contract %s: %v", contractID, err)
			}
		})
	}

	commissionRulesRepo := repository.NewTenantCommissionRulesPostgresRepository(pgDB)
	commissionRulesService := service.NewCommissionRulesService(commissionRulesRepo)
	commissionRulesHandler := NewCommissionRulesHandler(commissionRulesService)

	// O saldo de intermediação do demonstrativo vem do plano-base congelado da
	// locação (rentalConfigRepo): é o que explica por quanto tempo o repasse do
	// proprietário ainda fica menor.
	ownerStatementService := service.NewOwnerStatementService(transferRepo, managedBillRepo, journalRepo).
		WithContractLookup(clients.NewDealClient()).
		WithIntermediationLookup(rentalConfigRepo)
	ownerStatementHandler := NewOwnerStatementHandler(ownerStatementService, crossRepo).
		WithTenantCompanyLookup(tenantIdentityRepo).
		WithCRMClient(clients.NewCrmClient())

	// Financial preview engine — read-only, projeta o ciclo financeiro
	// (charges + commissions + transfers) antes da assinatura. Sem
	// efeito colateral; reusa BuildChargesPreview dos services já
	// construídos acima e ResolveCommissionPlan do pacote services.
	financialPreviewService := service.NewFinancialPreviewService(
		clients.NewDealClient(),
		rentalConfigService,
		saleBillingPlanService,
		commissionRulesService,
		tenantSettingsService,
	)
	financialPreviewService.SetCrossCompanyLookup(crossRepo)
	// Permite que a baixa de uma charge de venda sem composição econômica
	// (plano criado antes da assinatura) aplique a composição automaticamente.
	chargeService.WithSalePlanResolver(financialPreviewService)
	// Materializa o snapshot congelado de comissões do deal de venda na baixa
	// (obrigações Commission + payout orders com o mesmo plan hash) quando o
	// plano foi criado pelo wizard antes do contract.signed. Nil-safe.
	chargeService.WithSalePlanMaterializer(planMaterializer)
	financialPreviewHandler := NewFinancialPreviewHandler(financialPreviewService)

	// O provedor de comprovantes acompanha STORAGE_PROVIDER em cada ambiente
	// (Supabase em beta/producao; S3 ou local quando explicitamente configurado).
	// Em caso de configuracao invalida, o endpoint permanece registrado e
	// devolve 503 sem derrubar o restante do servico financeiro.
	proofUploadHandler := NewProofUploadHandler(nil)
	if proofStorage, sErr := storage.NewFromEnv(); sErr == nil {
		proofUploadHandler = NewProofUploadHandler(proofStorage)
	} else {
		logger.Warn("proof storage init failed; proofs upload disabled", "error", sErr.Error())
	}
	// Centralização de comprovantes: publica financial.proof_uploaded para o
	// toca-deal materializar o Document na Central de Documentos.
	proofUploadHandler.WithEventBus(eventBus)

	permissionProvider := auth.NewCorePermissionProvider(clients.NewCoreClient())
	// SaaS billing is independent of real-estate charge settlement.
	subscriptionEnabled := os.Getenv("PLATFORM_SUBSCRIPTIONS_ENABLED") == "true"
	subscriptionDev := os.Getenv("ABACATEPAY_DEV_MODE") == "true"
	frontendURL := strings.TrimRight(os.Getenv("PUBLIC_FRONTEND_URL"), "/")
	parsedURL, urlErr := url.Parse(frontendURL)
	if subscriptionEnabled && (os.Getenv("PAYMENT_GATEWAY_API_KEY") == "" || os.Getenv("ABACATEPAY_WEBHOOK_SECRET") == "" || urlErr != nil || parsedURL.Host == "" || parsedURL.Scheme != "https" || parsedURL.RawQuery != "" || parsedURL.Fragment != "" || parsedURL.User != nil || (cfg.AppEnv == "beta" && !subscriptionDev)) {
		logger.Error("invalid platform subscription configuration; checkout disabled")
		subscriptionEnabled = false
	}
	platformSubscriptionHandler := &PlatformSubscriptionHandler{
		Service: &service.PlatformSubscriptionService{Repo: repository.NewPlatformSubscriptionRepository(pgDB), Provider: providers.NewAbacateSubscriptionProvider(os.Getenv("PAYMENT_GATEWAY_API_KEY"), subscriptionDev), ReturnURL: frontendURL + "/assinatura", DevMode: subscriptionDev, Enabled: subscriptionEnabled, AccessControlEnabled: subscriptionEnabled && os.Getenv("PLATFORM_ACCESS_CONTROL_ENABLED") == "true"},
		Secret:  os.Getenv("ABACATEPAY_WEBHOOK_SECRET"),
	}
	for _, ip := range strings.Split(os.Getenv("ABACATEPAY_WEBHOOK_ALLOWED_IPS"), ",") {
		if strings.TrimSpace(ip) != "" {
			platformSubscriptionHandler.AllowedIPs = append(platformSubscriptionHandler.AllowedIPs, strings.TrimSpace(ip))
		}
	}
	r.POST("/webhooks/abacatepay", platformSubscriptionHandler.Webhook)

	// P2/M11 — gate de aprovação por valor em ajustes financeiros: os handlers
	// de cobrança e comissão avaliam `finance:approve_adjustment` in-handler
	// quando o ajuste passa do limiar.
	chargeHandler.WithPermissionProvider(permissionProvider)
	// Resolve o nome do devedor na listagem de cobranças (a cobrança guarda só o
	// id da parte): sem isso a tela mostra um UUID na coluna do inquilino.
	chargeHandler.WithCRMClient(clients.NewCrmClient())
	// Mesmo gate de alçada (M11) para o desconto do acordo de inadimplência.
	collectionHandler.WithPermissionProvider(permissionProvider)
	commHandler.WithPermissionProvider(permissionProvider)

	// API V1
	v1 := r.Group("/api/v1")

	// Swagger — P2/M16: só fora de release (pkg/apidocs); quando montado, mantém
	// a autenticação de operador que já tinha.
	apidocs.Register(v1, "/swagger/*any", auth.InternalOrMiddleware(auth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}), auth.RejectFunctionalRole("cliente"), ginSwagger.WrapHandler(swaggerFiles.Handler))

	// P1/A3: aceita chave de serviço OU JWT de usuário; o tenant deixa de vir cru
	// do chamador (antes esta rota não tinha predicado de tenant na consulta).
	internalFinancial := v1.Group("/financial/internal")
	internalFinancial.Use(auth.InternalOrUserMiddleware(auth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}))
	internalFinancial.Use(pkgmw.RequestTimeout(15 * time.Second))
	{
		internalFinancial.GET("/deals/:id/charges", chargeHandler.ListInternalCharges)
		// T-01: guard server-side do plano SALE pré-contrato — o toca-deal
		// consulta antes de gerar/enviar contrato de venda; sem draft ativo o
		// envio é recusado (o contract.signed nunca fabrica fallback à vista).
		internalFinancial.GET("/deals/:id/sale-plan-readiness", saleBillingPlanHandler.SalePlanReadiness)
		// Reparo operacional (F4 do financeiro de locação): re-deriva alocações
		// de uma cobrança quitada antes do plano de intermediação existir.
		// Credencial de serviço com allowlist de tenant ou JWT com finance:write.
		internalFinancial.POST("/charges/:id/repair-rental-allocations", chargeHandler.RepairRentalAllocations)
	}

	// Phase 5.1 — Client portal routes. Distinct from `secured` because
	// these endpoints are scoped to authenticated clients (functional_role=
	// cliente) and gate access by JWT account_id rather than admin
	// permissions. See PortalRentalHandler.ListMine for the resolution
	// flow (account_id → leads → rentals).
	clientPortal := v1.Group("/client-portal")
	// Credencial primeiro; depois o contexto do portal (tenant do vínculo do
	// cliente + pool de serviço) — ver portal_service_context.go.
	clientPortal.Use(
		auth.InternalOrMiddleware(auth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}),
		auth.RequireFunctionalRole("cliente"),
		portalServiceContext(pgDB, crossRepo),
	)

	clientPortalFast := clientPortal.Group("")
	clientPortalFast.Use(pkgmw.RequestTimeout(15 * time.Second))
	{
		clientPortalFast.GET("/rentals", portalRentalHandler.ListMine)

		// Portal owner — repasses próprios (gate por account_id).
		// LGPD: cada handler valida que o transfer pertence a um lead
		// do account do caller antes de retornar dados/PDF.
		clientPortalFast.GET("/transfers", portalTransferHandler.ListMine)
		clientPortalFast.GET("/transfers/:id", portalTransferHandler.GetMine)

		// Portal charges (#460) — tenant/owner consultam charges das próprias
		// locações. Filtragem por leads do account_id evita vazamento.
		clientPortalFast.GET("/charges", portalChargeHandler.ListMine)
		clientPortalFast.GET("/charges/:id", portalChargeHandler.GetMine)

		// Portal informes (#419/#459) — owner consulta e baixa o próprio
		// Informe de Rendimentos (declaração para IR).
		clientPortalFast.GET("/informes", portalInformeHandler.ListMine)

		// Phase 6 — demonstrativo mensal portal (proprietário consulta o
		// próprio mês). LGPD guard interno via crossRepo.LeadIDsForAccount.
		clientPortalFast.GET("/owner-statements", ownerStatementHandler.GetMine)
	}

	clientPortalSlow := clientPortal.Group("")
	clientPortalSlow.Use(pkgmw.RequestTimeout(60 * time.Second))
	{
		clientPortalSlow.GET("/transfers/:id/receipt", portalTransferHandler.ReceiptMine)
		clientPortalSlow.GET("/informes/:id/pdf", portalInformeHandler.DownloadMine)
		clientPortalSlow.GET("/charges/:id/receipt", portalChargeHandler.GetReceiptMine)
	}

	secured := v1.Group("")
	secured.Use(auth.InternalOrMiddleware(auth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}), auth.RejectFunctionalRole("cliente"))
	// Subscription access is a read for all authenticated staff. The admin-only
	// billing page and mutations remain below on the subscriptions group.
	secured.GET("/platform-subscription/access", platformSubscriptionHandler.Access)

	subscriptions := secured.Group("/platform-subscription")
	subscriptions.Use(auth.RequirePermission(permissionProvider, "module:admin"))
	{
		subscriptions.GET("", platformSubscriptionHandler.Get)
		subscriptions.GET("/payments/:id/receipt", platformSubscriptionHandler.GetPaymentReceipt)
		subscriptions.POST("/checkout", auth.RequirePermission(permissionProvider, "settings:manage_general"), platformSubscriptionHandler.Select)
		subscriptions.POST("/cancel", auth.RequirePermission(permissionProvider, "settings:manage_general"), platformSubscriptionHandler.Cancel)
		subscriptions.POST("/change/cancel", auth.RequirePermission(permissionProvider, "settings:manage_general"), platformSubscriptionHandler.RevertChange)
		subscriptions.POST("/payment-method", auth.RequirePermission(permissionProvider, "settings:manage_general"), platformSubscriptionHandler.ChangePaymentMethod)
	}

	securedFast := secured.Group("")
	securedFast.Use(pkgmw.RequestTimeout(15 * time.Second))

	// Financial preview — projeção do ciclo financeiro (charges +
	// commissions + transfers) sem persistir nada. Alimenta a aba
	// "Financeiro (Prévia)" da esteira antes do contract.signed.
	financialPreview := securedFast.Group("/financial-preview")
	{
		financialPreview.GET("/deals/:id", auth.RequirePermission(permissionProvider, "finance:read"), financialPreviewHandler.PreviewByDeal)
		financialPreview.GET("/contracts/:id", auth.RequirePermission(permissionProvider, "finance:read"), financialPreviewHandler.PreviewByContract)
	}

	commissions := securedFast.Group("/commissions")
	{
		commissions.POST("", auth.RequirePermission(permissionProvider, "finance:write"), commHandler.Create)
		commissions.GET("", auth.RequirePermission(permissionProvider, "finance:read"), commHandler.List)
		commissions.GET("/:id", auth.RequirePermission(permissionProvider, "finance:read"), commHandler.GetByID)
		// Phase 6 — fluxo CRUD completo de commission
		commissions.GET("/by-deal/:dealId", auth.RequirePermission(permissionProvider, "finance:read"), commHandler.ListByDeal)
		commissions.GET("/by-broker/:brokerId", auth.RequirePermission(permissionProvider, "finance:read"), commHandler.ListByBroker)
		commissions.POST("/:id/approve", auth.RequirePermission(permissionProvider, "finance:write"), commHandler.ApproveCommission)
		commissions.POST("/:id/pay", auth.RequirePermission(permissionProvider, "finance:write"), commHandler.MarkPaid)
		commissions.POST("/:id/cancel", auth.RequirePermission(permissionProvider, "finance:write"), commHandler.CancelCommission)
		commissions.POST("/:id/reverse", auth.RequirePermission(permissionProvider, "finance:write"), commHandler.ReverseCommission)
		commissions.POST("/:id/deductions", auth.RequirePermission(permissionProvider, "finance:write"), commHandler.AddDeduction)
		commissions.DELETE("/:id/deductions/:deductionId", auth.RequirePermission(permissionProvider, "finance:write"), commHandler.RemoveDeduction)
	}

	commissionsBatch := secured.Group("/commissions")
	commissionsBatch.Use(pkgmw.RequestTimeout(60 * time.Second))
	{
		commissionsBatch.POST("/batch", auth.RequirePermission(permissionProvider, "finance:write"), commHandler.CreateBatch)
	}

	// Phase 6 — tenant financial settings (defaults aplicados em runtime)
	financialSettings := securedFast.Group("/financial-settings")
	{
		// Leitura liberada também para quem personaliza comissão de negociação
		// (`commissions:write`, que inclui operações): sem isso o editor via os
		// padrões da imobiliária caírem no fallback local.
		financialSettings.GET("", auth.RequireAnyPermission(permissionProvider, "finance:read", "commissions:write"), tenantSettingsHandler.Get)
		financialSettings.PUT("", auth.RequirePermission(permissionProvider, "finance:write"), tenantSettingsHandler.Update)
	}

	// Phase 6 — tabela de comissão por role (substitui hardcoded 30/20/50)
	commissionRules := securedFast.Group("/commission-rules")
	{
		commissionRules.GET("", auth.RequireAnyPermission(permissionProvider, "finance:read", "commissions:write"), commissionRulesHandler.Get)
		commissionRules.PUT("", auth.RequirePermission(permissionProvider, "finance:write"), commissionRulesHandler.Update)
	}

	// Phase 6 — comprovantes financeiros (payout/charge/commission)
	proofs := secured.Group("/proofs")
	{
		proofs.POST("", auth.RequirePermission(permissionProvider, "finance:write"), proofUploadHandler.Upload)
		proofs.GET("/content/:token", auth.RequirePermission(permissionProvider, "finance:read"), proofUploadHandler.View)
	}

	// Phase 6 — demonstrativo mensal do proprietário (admin scope)
	ownerStatements := securedFast.Group("/owner-statements")
	{
		ownerStatements.GET("/:ownerLeadId", auth.RequirePermission(permissionProvider, "finance:read"), ownerStatementHandler.GetAdmin)
		// Saldo corrente do proprietário (widget fixo) — allocation-aware
		// com movimentações de contas administradas reaplicadas.
		ownerStatements.GET("/:ownerLeadId/balance", auth.RequirePermission(permissionProvider, "finance:read"), ownerStatementHandler.GetBalance)
	}

	reports := secured.Group("/reports")
	{
		reports.GET("/financial/summary", auth.RequireAnyPermission(permissionProvider, "reports:read", "finance:read"), financialReportHandler.Summary)
		// Phase 6 — dashboard (DRE + cashflow + KPIs)
		reports.GET("/financial/dre", auth.RequireAnyPermission(permissionProvider, "reports:read", "finance:read"), financialReportHandler.DRE)
		reports.GET("/financial/cashflow", auth.RequireAnyPermission(permissionProvider, "reports:read", "finance:read"), financialReportHandler.CashFlow)
		reports.GET("/financial/chart-of-accounts", auth.RequireAnyPermission(permissionProvider, "reports:read", "finance:read"), financialReportHandler.ChartOfAccounts)
		reports.GET("/financial/kpis", auth.RequireAnyPermission(permissionProvider, "reports:read", "finance:read"), financialReportHandler.KPIs)
		// FIN-COM-14: reconciliation dry-run report
		reports.GET("/financial/reconciliation", auth.RequireAnyPermission(permissionProvider, "reports:read", "finance:read"), reconciliationHandler.Run)
	}

	invoices := securedFast.Group("/invoices")
	{
		invoices.POST("", auth.RequirePermission(permissionProvider, "finance:write"), invHandler.Create)
		invoices.GET("", auth.RequirePermission(permissionProvider, "finance:read"), invHandler.List)
		invoices.GET("/:id", auth.RequirePermission(permissionProvider, "finance:read"), invHandler.GetByID)
		invoices.POST("/:id/pay", auth.RequirePermission(permissionProvider, "finance:write"), invHandler.Pay)
	}

	transactions := securedFast.Group("/transactions")
	{
		transactions.POST("", auth.RequirePermission(permissionProvider, "finance:write"), txHandler.Create)
		transactions.POST("/batch", auth.RequirePermission(permissionProvider, "finance:write"), txHandler.CreateBatch)
		transactions.GET("", auth.RequirePermission(permissionProvider, "finance:read"), txHandler.List)
		transactions.GET("/:id", auth.RequirePermission(permissionProvider, "finance:read"), txHandler.GetByID)
		transactions.PUT("/:id", auth.RequirePermission(permissionProvider, "finance:write"), txHandler.Update)
		transactions.POST("/:id/settle", auth.RequirePermission(permissionProvider, "finance:write"), txHandler.Settle)
		transactions.POST("/:id/cancel", auth.RequirePermission(permissionProvider, "finance:write"), txHandler.Cancel)
		transactions.POST("/:id/reverse", auth.RequirePermission(permissionProvider, "finance:write"), txHandler.Reverse)
		// P1/A7-M3: a rota exigia `finance:delete`, que NÃO existe no catálogo —
		// ou seja, `DELETE /transactions/:id` era inalcançável para todo papel
		// funcional (só perfil root passava). As demais transições da mesma
		// entidade (settle/cancel/reverse) usam `finance:write`, que é a
		// capacidade documentada na matriz ("Financeiro — editar").
		transactions.DELETE("/:id", auth.RequirePermission(permissionProvider, "finance:write"), txHandler.Delete)
	}

	charges := securedFast.Group("/charges")
	{
		charges.POST("", auth.RequirePermission(permissionProvider, "charges:write"), chargeHandler.Create)
		charges.GET("", auth.RequirePermission(permissionProvider, "charges:read"), chargeHandler.List)
		charges.GET("/overdue", auth.RequirePermission(permissionProvider, "charges:read"), chargeHandler.ListOverdue)
		charges.GET("/:id", auth.RequirePermission(permissionProvider, "charges:read"), chargeHandler.GetByID)
		charges.PUT("/:id", auth.RequirePermission(permissionProvider, "charges:write"), chargeHandler.Update)
		charges.POST("/:id/pay", auth.RequirePermission(permissionProvider, "charges:write"), chargeHandler.Pay)
		charges.POST("/:id/notify", auth.RequirePermission(permissionProvider, "charges:write"), chargeHandler.Notify)
		charges.POST("/:id/reconcile-manual", auth.RequirePermission(permissionProvider, "finance:write"), chargeHandler.ReconcileManual)
		// P1-05: link de cobrança via provedor de pagamentos (sandbox por
		// PAYMENT_PROVIDER; 409 enquanto o gateway real não for escolhido).
		charges.POST("/:id/payment-link", auth.RequirePermission(permissionProvider, "charges:write"), chargeHandler.GeneratePaymentLink)
		// Read-only: o painel só oferece o botão de reparo quando há o que
		// reparar (plano congelado com saldo + pagamento sem a intermediação).
		charges.GET("/:id/repair-rental-allocations/eligibility", auth.RequirePermission(permissionProvider, "charges:read"), chargeHandler.CheckRepairRentalAllocations)
		// Reparo de alocação de locação (comissão de intermediação não financiada
		// porque a baixa aconteceu antes do plano congelar). Operação auditada:
		// exige reason e estorna/regrava o lançamento do recebimento.
		// Permissão de aprovação de repasse: a ação estorna lançamento do ledger
		// e recalcula o repasse do proprietário (mesma família do estorno de
		// managed bill), então não fica no alcance de escrita comum.
		charges.POST("/:id/repair-rental-allocations", auth.RequirePermission(permissionProvider, "finance:approve_transfer"), chargeHandler.RepairRentalAllocations)
		charges.POST("/:id/cancel", auth.RequirePermission(permissionProvider, "finance:write"), chargeHandler.CancelCharge)
		// Estorno de cobrança PAGA (G30): o serviço existia sem rota, então não
		// havia como desfazer um recebimento pela API.
		charges.POST("/:id/reverse", auth.RequirePermission(permissionProvider, "finance:approve_transfer"), chargeHandler.ReverseCharge)
	}

	chargesReceipt := secured.Group("/charges")
	chargesReceipt.Use(pkgmw.RequestTimeout(60 * time.Second))
	{
		chargesReceipt.GET("/:id/receipt", auth.RequirePermission(permissionProvider, "charges:read"), chargeHandler.GetReceipt)
	}

	// FASE 4 (integração total) — security-deposit acerto. The front office
	// rescisão modal queries this endpoint to pre-fill devolução vs. dedução.
	contractsRoutes := securedFast.Group("/contracts")
	{
		contractsRoutes.GET("/:id/security-deposit-balance", auth.RequirePermission(permissionProvider, "charges:read"), chargeHandler.SecurityDepositBalance)
	}

	// Phase 4 admin rental endpoints. Somente LEITURA: a permissão é
	// `rentals:read`. Não existe rota de escrita de locação — as escritas que a
	// tela dispara (contas de consumo, régua, config de cobrança) são cobrança e
	// usam `charges:write` (P1/A7 removeu `rentals:write`, que não governava
	// nada).
	rentals := securedFast.Group("/rentals")
	{
		rentals.GET("", auth.RequirePermission(permissionProvider, "rentals:read"), rentalHandler.List)
		rentals.GET("/:id", auth.RequirePermission(permissionProvider, "rentals:read"), rentalHandler.GetByID)
	}

	utilityAccounts := securedFast.Group("/utility-accounts")
	{
		utilityAccounts.POST("", auth.RequirePermission(permissionProvider, "charges:write"), utilityAccountHandler.Create)
		utilityAccounts.GET("", auth.RequirePermission(permissionProvider, "charges:read"), utilityAccountHandler.List)
		utilityAccounts.GET("/:id", auth.RequirePermission(permissionProvider, "charges:read"), utilityAccountHandler.GetByID)
		utilityAccounts.PUT("/:id", auth.RequirePermission(permissionProvider, "charges:write"), utilityAccountHandler.Update)
		utilityAccounts.DELETE("/:id", auth.RequirePermission(permissionProvider, "charges:write"), utilityAccountHandler.Delete)
		// VARIABLE_SEPARATE accounts (água/luz por consumo): operator
		// posts the month's amount and the handler emits a standalone
		// TypeUtility charge.
		utilityAccounts.POST("/:id/charge", auth.RequirePermission(permissionProvider, "charges:write"), utilityChargeHandler.Create)
	}

	// Sale billing plan + payout orders — end-to-end billing feature.
	saleBillingPlans := securedFast.Group("/sale-billing-plans")
	{
		saleBillingPlans.POST("/contracts/:id", auth.RequirePermission(permissionProvider, "charges:write"), saleBillingPlanHandler.Create)
		saleBillingPlans.GET("/contracts/:id", auth.RequirePermission(permissionProvider, "charges:read"), saleBillingPlanHandler.GetByContract)
		// Drafts pré-assinatura: gravados pelo operador na esteira;
		// consumidos pelo contract.signed handler via CreateFromDraft.
		saleBillingPlans.POST("/drafts/deals/:id", auth.RequirePermission(permissionProvider, "charges:write"), saleBillingPlanHandler.UpsertDraft)
		saleBillingPlans.GET("/drafts/deals/:id", auth.RequirePermission(permissionProvider, "charges:read"), saleBillingPlanHandler.GetDraft)
	}

	payoutOrders := securedFast.Group("/payout-orders")
	{
		payoutOrders.GET("", auth.RequirePermission(permissionProvider, "finance:read"), payoutOrderHandler.List)
		// FIN-COM-13: totais server-side independentes da paginação.
		// Precisa vir antes de /:id para "totals" não ser capturado como :id.
		payoutOrders.GET("/totals", auth.RequirePermission(permissionProvider, "finance:read"), payoutOrderHandler.Totals)
		payoutOrders.GET("/:id", auth.RequirePermission(permissionProvider, "finance:read"), payoutOrderHandler.GetByID)
		payoutOrders.POST("/:id/approve", auth.RequirePermission(permissionProvider, "finance:approve_transfer"), payoutOrderHandler.Approve)
		payoutOrders.POST("/:id/mark-paid", auth.RequirePermission(permissionProvider, "finance:approve_transfer"), payoutOrderHandler.MarkPaid)
		payoutOrders.POST("/:id/cancel", auth.RequirePermission(permissionProvider, "finance:write"), payoutOrderHandler.Cancel)
		// Cancelar sem caminho de volta obrigava o operador a recriar a
		// obrigação por fora do sistema. Regerar recria a ordem a partir da
		// cancelada e reabre a comissão/lançamento vinculados.
		payoutOrders.POST("/:id/regenerate", auth.RequirePermission(permissionProvider, "finance:write"), payoutOrderHandler.Regenerate)
		payoutOrders.POST("/group", auth.RequirePermission(permissionProvider, "finance:write"), payoutOrderHandler.Group)
		// Admin-only "recompute splits" hook — used when commission rules
		// were corrected after the charge cleared.
		payoutOrders.POST("/charges/:id/recompute", auth.RequirePermission(permissionProvider, "finance:approve_transfer"), payoutOrderHandler.RecomputeForCharge)
	}

	reminderRules := securedFast.Group("/reminder-rules")
	{
		reminderRules.POST("", auth.RequirePermission(permissionProvider, "charges:write"), reminderRuleHandler.Create)
		reminderRules.GET("", auth.RequirePermission(permissionProvider, "charges:read"), reminderRuleHandler.List)
		reminderRules.GET("/:id", auth.RequirePermission(permissionProvider, "charges:read"), reminderRuleHandler.GetByID)
		reminderRules.PUT("/:id", auth.RequirePermission(permissionProvider, "charges:write"), reminderRuleHandler.Update)
		reminderRules.DELETE("/:id", auth.RequirePermission(permissionProvider, "charges:write"), reminderRuleHandler.Delete)
		reminderRules.POST("/run-now", auth.RequirePermission(permissionProvider, "charges:write"), reminderRuleHandler.RunNow)
	}

	rentalChargeConfigs := securedFast.Group("/rental-charge-configs")
	{
		rentalChargeConfigs.POST("", auth.RequirePermission(permissionProvider, "charges:write"), rentalConfigHandler.Create)
		rentalChargeConfigs.GET("/by-contract/:contract_id", auth.RequirePermission(permissionProvider, "charges:read"), rentalConfigHandler.GetByContractID)
		rentalChargeConfigs.GET("/:id", auth.RequirePermission(permissionProvider, "charges:read"), rentalConfigHandler.GetByID)
		rentalChargeConfigs.PUT("/:id", auth.RequirePermission(permissionProvider, "charges:write"), rentalConfigHandler.Update)
		rentalChargeConfigs.POST("/:id/generate-charges", auth.RequirePermission(permissionProvider, "charges:write"), rentalConfigHandler.GenerateCharges)
		rentalChargeConfigs.POST("/:id/bundled-items", auth.RequirePermission(permissionProvider, "charges:write"), rentalConfigHandler.UpdateBundledItems)
	}

	transfers := securedFast.Group("/transfers")
	{
		transfers.POST("", auth.RequirePermission(permissionProvider, "finance:write"), transferHandler.Create)
		transfers.GET("", auth.RequirePermission(permissionProvider, "finance:read"), transferHandler.List)
		transfers.GET("/:id", auth.RequirePermission(permissionProvider, "finance:read"), transferHandler.GetByID)
		transfers.POST("/:id/approve", auth.RequirePermission(permissionProvider, "finance:approve_transfer"), transferHandler.Approve)
		transfers.POST("/:id/process", auth.RequirePermission(permissionProvider, "finance:approve_transfer"), transferHandler.Process)
		// Liberação da retenção por inadimplência (política opcional do tenant):
		// quem aprova repasse é quem pode assumir o risco de liberar.
		transfers.POST("/:id/release-delinquency-hold", auth.RequirePermission(permissionProvider, "finance:approve_transfer"), transferHandler.ReleaseDelinquencyHold)
	}

	// Managed bills are concrete owner-paid obligations materialized from a
	// paid charge's reserved allocations. Approval uses the high-trust transfer
	// permission; payment, reconciliation and cancellation remain audited
	// finance writes with optimistic-version checks in the service.
	taxObligations := securedFast.Group("/tax-obligations")
	{
		taxObligations.GET("", auth.RequirePermission(permissionProvider, "finance:read"), taxObligationHandler.List)
		taxObligations.GET("/:id", auth.RequirePermission(permissionProvider, "finance:read"), taxObligationHandler.Get)
		// Recolhimento é movimento de dinheiro: mesma permissão dos outros pagamentos.
		taxObligations.POST("/:id/settle", auth.RequirePermission(permissionProvider, "finance:write"), taxObligationHandler.Settle)
		// Reparo: a obrigação é derivada da comissão, então uma sincronização
		// falha é corrigida reprocessando.
		taxObligations.POST("/recompute", auth.RequirePermission(permissionProvider, "finance:write"), taxObligationHandler.Recompute)
	}

	brokerAdvances := securedFast.Group("/broker-advances")
	{
		brokerAdvances.GET("", auth.RequirePermission(permissionProvider, "finance:read"), brokerAdvanceHandler.List)
		// Adiantar dinheiro é saída de caixa: mesma permissão das demais ações.
		brokerAdvances.POST("", auth.RequirePermission(permissionProvider, "finance:write"), brokerAdvanceHandler.Create)
		brokerAdvances.POST("/:id/cancel", auth.RequirePermission(permissionProvider, "finance:write"), brokerAdvanceHandler.Cancel)
	}

	managedBills := securedFast.Group("/managed-bills")
	{
		managedBills.GET("", auth.RequirePermission(permissionProvider, "finance:read"), managedBillHandler.List)
		managedBills.GET("/:id", auth.RequirePermission(permissionProvider, "finance:read"), managedBillHandler.GetByID)
		managedBills.POST("/:id/approve", auth.RequirePermission(permissionProvider, "finance:approve_transfer"), managedBillHandler.Approve)
		managedBills.POST("/:id/pay", auth.RequirePermission(permissionProvider, "finance:write"), managedBillHandler.Pay)
		managedBills.POST("/:id/reconcile", auth.RequirePermission(permissionProvider, "finance:write"), managedBillHandler.Reconcile)
		managedBills.POST("/:id/reverse-payment", auth.RequirePermission(permissionProvider, "finance:approve_transfer"), managedBillHandler.ReversePayment)
		managedBills.POST("/:id/cancel", auth.RequirePermission(permissionProvider, "finance:write"), managedBillHandler.Cancel)
	}

	transfersReceipt := secured.Group("/transfers")
	transfersReceipt.Use(pkgmw.RequestTimeout(60 * time.Second))
	{
		transfersReceipt.GET("/:id/receipt", auth.RequirePermission(permissionProvider, "finance:read"), transferHandler.Receipt)
	}

	// Agency withdrawals ("saque da imobiliária"). Money movement, so approve
	// and submit reuse the high-trust finance:approve_transfer permission.
	withdrawals := securedFast.Group("/withdrawals")
	{
		withdrawals.POST("", auth.RequirePermission(permissionProvider, "finance:write"), withdrawalHandler.Request)
		withdrawals.GET("", auth.RequirePermission(permissionProvider, "finance:read"), withdrawalHandler.List)
		withdrawals.GET("/:id", auth.RequirePermission(permissionProvider, "finance:read"), withdrawalHandler.GetByID)
		withdrawals.POST("/:id/approve", auth.RequirePermission(permissionProvider, "finance:approve_transfer"), withdrawalHandler.Approve)
		withdrawals.POST("/:id/submit", auth.RequirePermission(permissionProvider, "finance:approve_transfer"), withdrawalHandler.Submit)
		withdrawals.POST("/:id/cancel", auth.RequirePermission(permissionProvider, "finance:write"), withdrawalHandler.Cancel)
	}

	collections := securedFast.Group("/collections")
	{
		collections.POST("", auth.RequireAnyPermission(permissionProvider, "collections:write", "charges:write"), collectionHandler.Create)
		// Inadimplência: permissão dedicada OU a de cobrança (implantação aditiva —
		// ninguém perde acesso no deploy e a imobiliária ganha granularidade).
		collections.GET("", auth.RequireAnyPermission(permissionProvider, "collections:read", "charges:read"), collectionHandler.List)
		collections.GET("/:id", auth.RequireAnyPermission(permissionProvider, "collections:read", "charges:read"), collectionHandler.GetByID)
		collections.PUT("/:id", auth.RequireAnyPermission(permissionProvider, "collections:write", "charges:write"), collectionHandler.Update)
		collections.POST("/:id/actions", auth.RequireAnyPermission(permissionProvider, "collections:write", "charges:write"), collectionHandler.AddAction)
		collections.POST("/:id/judicial", auth.RequireAnyPermission(permissionProvider, "collections:judicial", "charges:write"), collectionHandler.SetJudicial)
		collections.GET("/:id/judicial", auth.RequireAnyPermission(permissionProvider, "collections:read", "charges:read"), collectionHandler.GetJudicial)
		collections.POST("/:id/guarantee", auth.RequireAnyPermission(permissionProvider, "collections:guarantee", "charges:write"), collectionHandler.ActivateGuarantee)
		collections.GET("/:id/guarantee", auth.RequireAnyPermission(permissionProvider, "collections:read", "charges:read"), collectionHandler.GetGuarantee)
		collections.PUT("/:id/guarantee/status", auth.RequireAnyPermission(permissionProvider, "collections:guarantee", "charges:write"), collectionHandler.UpdateGuaranteeStatus)
		// Fase 2 do plano de inadimplência: acordo como entidade (as parcelas
		// viram cobranças), desfecho judicial com custas e retomada da cobrança.
		collections.POST("/:id/agreement", auth.RequireAnyPermission(permissionProvider, "collections:negotiate", "charges:write"), collectionHandler.CreateAgreement)
		collections.GET("/:id/agreement", auth.RequireAnyPermission(permissionProvider, "collections:read", "charges:read"), collectionHandler.GetAgreement)
		collections.POST("/:id/agreement/break", auth.RequireAnyPermission(permissionProvider, "collections:negotiate", "charges:write"), collectionHandler.BreakAgreement)
		collections.POST("/:id/agreement/cancel", auth.RequireAnyPermission(permissionProvider, "collections:negotiate", "charges:write"), collectionHandler.CancelAgreement)
		collections.POST("/:id/judicial/outcome", auth.RequireAnyPermission(permissionProvider, "collections:judicial", "charges:write"), collectionHandler.RegisterJudicialOutcome)
	}

	delinquency := secured.Group("/delinquency")
	{
		delinquency.GET("/dashboard", auth.RequireAnyPermission(permissionProvider, "collections:read", "charges:read"), delinquencyHandler.GetDashboard)
		delinquency.GET("/aging", auth.RequireAnyPermission(permissionProvider, "collections:read", "charges:read"), delinquencyHandler.GetAgingReport)
		delinquency.GET("/stats", auth.RequireAnyPermission(permissionProvider, "collections:read", "charges:read"), delinquencyHandler.GetStats)
		delinquency.POST("/process", auth.RequireAnyPermission(permissionProvider, "delinquency:run", "charges:write"), delinquencyHandler.ProcessManual)
	}

	fiscal := secured.Group("/fiscal")
	{
		fiscal.GET("/calendar", auth.RequirePermission(permissionProvider, "fiscal:read"), fiscalHandler.GetCalendar)
		fiscal.GET("/dimob/:year", auth.RequirePermission(permissionProvider, "fiscal:read"), fiscalHandler.GetDimobStatus)
		fiscal.GET("/dimob/:year/file", auth.RequirePermission(permissionProvider, "fiscal:read"), fiscalHandler.DownloadDimob)
		fiscal.POST("/dimob/generate", auth.RequirePermission(permissionProvider, "fiscal:generate"), fiscalHandler.GenerateDimob)
		fiscal.GET("/nfse", auth.RequirePermission(permissionProvider, "fiscal:read"), fiscalHandler.ListNfse)
		fiscal.POST("/nfse", auth.RequirePermission(permissionProvider, "fiscal:generate"), fiscalHandler.IssueNfse)
		fiscal.GET("/informes", auth.RequirePermission(permissionProvider, "fiscal:read"), fiscalHandler.ListInformes)
		fiscal.GET("/informes/:id/pdf", auth.RequirePermission(permissionProvider, "fiscal:read"), fiscalHandler.DownloadInformePDF)
		fiscal.POST("/informes/generate", auth.RequirePermission(permissionProvider, "fiscal:generate"), fiscalHandler.GenerateInformes)
		fiscal.GET("/reports", auth.RequirePermission(permissionProvider, "fiscal:read"), fiscalHandler.ListReports)
		fiscal.GET("/reports/:id/download", auth.RequirePermission(permissionProvider, "fiscal:read"), fiscalHandler.DownloadReport)
		fiscal.DELETE("/reports/:id", auth.RequirePermission(permissionProvider, "fiscal:generate"), fiscalHandler.ArchiveReport)
	}

	return r
}
