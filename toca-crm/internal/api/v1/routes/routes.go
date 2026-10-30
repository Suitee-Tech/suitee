package routes

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/apidocs"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/exports"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	pkgmw "github.com/oginoino/toca/pkg/middleware"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/pkg/validation"
	_ "github.com/oginoino/toca/toca-crm/docs"
	"github.com/oginoino/toca/toca-crm/internal/api/v1/handlers"
	"github.com/oginoino/toca/toca-crm/internal/repository"
	"github.com/oginoino/toca/toca-crm/internal/service"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// SetupRouter wires the toca-crm HTTP router. The workerCtx parameter
// controls the lifetime of long-running background workers (currently the
// SLA scanner) — cancel it during graceful shutdown to let them exit
// cleanly instead of leaking goroutines.
func SetupRouter(workerCtx context.Context, cfg config.BaseConfig, pgDB *database.PostgresDB, eventBus events.EventBus, storageSvc storage.Service) *gin.Engine {
	// Initialize JWKS for Supabase JWT validation
	if err := auth.InitJWKS(cfg.JWT.SupabaseJWKSURL); err != nil {
		logger.Error("Failed to initialize JWKS", map[string]interface{}{"error": err.Error()})
	}

	// P1/A2: registra no boot se o serviço está no modelo antigo (chave
	// compartilhada, sem allowlist de tenant) — em produção isso é alarme.
	auth.LogServiceKeyRollout()

	_ = validation.InitGin()

	r := gin.New()
	// Base middleware stack (request id, structured logger, request
	// timeout, panic recovery) consolidated in pkg/middleware. CORS is
	// intentionally NOT applied here — the gateway terminates CORS for
	// the whole platform.
	r.Use(pkgmw.Setup(pkgmw.StackConfig{})...)
	r.Use(metrics.GinMiddleware("toca-crm"))

	r.GET("/metrics", metrics.Handler())

	// Health check (no auth)
	r.Any("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "toca-crm"})
	})

	// Dependencies
	leadRepo := repository.NewLeadPostgresRepository(pgDB)
	rouletteRepo := repository.NewRoulettePostgresRepository(pgDB)
	historyRepo := repository.NewRouletteHistoryPostgresRepository(pgDB)
	auditRepo := repository.NewRouletteAuditPostgresRepository(pgDB)
	brokerRepo := repository.NewBrokerPostgresRepository(pgDB)
	leadService := service.NewLeadService(leadRepo, rouletteRepo, historyRepo, brokerRepo, eventBus, pgDB)
	// Issue #415 — wire CoreClient so MatchProperties pode listar imóveis
	// disponíveis. Quando CORE_SERVICE_URL não está setado, o client
	// devolve erro descritivo no Search; o handler GET /leads/:id/matched-properties
	// responde 500 com a causa.
	leadService.WithPropertyClient(clients.NewCoreClient())
	// Agenda: a visita do lead vira um evento real no calendário do corretor (toca-calendar).
	leadService.WithCalendarClient(clients.NewCalendarClient())
	leadService.WithCommClient(clients.NewCommClient())
	// crossRepo reads core.partnerships so SharedLeadsForCompany can resolve
	// which partner companies the caller shares leads with (cross-tenant,
	// gated by the partnership permission check inside the service).
	leadService.WithCrossRepo(repository.NewCrossPostgresRepository(pgDB))
	rouletteService := service.NewLeadRouletteService(leadRepo, rouletteRepo, historyRepo, brokerRepo, eventBus, auditRepo)
	slaService := service.NewLeadSLAService(leadRepo, rouletteRepo, brokerRepo, eventBus, auditRepo)
	leadHandler := handlers.NewLeadHandler(leadService, rouletteService, slaService)
	historyHandler := handlers.NewRouletteHistoryHandler(historyRepo)
	auditHandler := handlers.NewRouletteAuditHandler(auditRepo)
	customerHandler := handlers.NewCustomerHandler(leadRepo)

	// Roleta por evento: ao receber lead.created, atribui corretor automaticamente (round-robin).
	// Em RabbitMQ, usamos fila nomeada para ser durável e permitir múltiplas instâncias.
	if eventBus != nil {
		_ = eventBus.SubscribeWithQueue("toca-crm.lead.created.assign", "lead.created", rouletteService.HandleLeadCreatedAssign)
		_ = eventBus.SubscribeWithQueue("toca-crm.deal.contract.signed.convert", "deal.contract.signed", leadService.HandleDealContractSignedConvert)
		// #409 — property.captured promove owner_lead para cliente
		// automaticamente. Idempotente; pula leads já convertidos.
		_ = eventBus.SubscribeWithQueue("toca-crm.property.captured.convert", "property.captured", leadService.HandlePropertyCapturedConvert)
		// Visita agendada pelo OPERADOR na agenda vira visit_request do lead:
		// sem isto o agendamento fica invisível na home do portal do cliente
		// quando o lead não tem imóvel recomendado nem contrato.
		_ = eventBus.SubscribeWithQueue("toca-crm.appointment.created.visit_sync", "appointment.created", service.NewAppointmentVisitSyncHandler(leadRepo))
	}

	// Worker de SLA (redirecionamentos automáticos). Vinculado ao workerCtx
	// para que o graceful shutdown encerre a goroutine em vez de vazar.
	go leadService.RunVisitRecoveryWorker(workerCtx)
	go slaService.RunSLAWorker(workerCtx)

	// Worker de SLA Primeiro Contato (#427). Goroutine independente
	// porque varre uma query diferente (sem filtro por status PENDING_
	// ACCEPTANCE da roleta) — não dá pra reusar o mesmo loop. Mesmo
	// shutdown context.
	go slaService.RunFirstContactSLAWorker(workerCtx)

	brokerHandler := handlers.NewBrokerHandler(brokerRepo)
	// Wire the event bus so PUT /brokers/:id emits broker.updated for
	// downstream snapshot consumers (Deal.BrokerParticipations etc.).
	if eventBus != nil {
		brokerHandler.SetEventBus(eventBus)
	}
	permissionProvider := auth.NewCorePermissionProvider(clients.NewCoreClient())

	// P1/A10: o handler de leads decide posse na leitura por recurso
	// (admin/gestor/operacoes leem qualquer lead; os demais, só o próprio ou o
	// pool não atribuído), então precisa do catálogo.
	leadHandler.WithPermissionProvider(permissionProvider)

	// Issue #474 — Async export. Registry global do pkg/exports recebe
	// o generator de Lead+CSV; handler + worker compartilham o mesmo
	// Service. Worker roda em goroutine paralela aos demais workers.
	exportRepo := exports.NewPostgresRepository(pgDB)
	exportService := exports.NewService(exportRepo)
	exports.Register(service.LeadEntityType, exports.FormatCSV, service.NewLeadCSVGenerator(leadRepo))
	exportHandler := handlers.NewExportHandler(exportService)

	if storageSvc != nil {
		// Bucket configurável; default "exports". Pre-assinatura de
		// 7 dias (suficiente pro usuário baixar, mais que isso = pedir
		// novo export — não queremos URLs longas circulando).
		exportBucket := os.Getenv("EXPORTS_BUCKET")
		if exportBucket == "" {
			exportBucket = "exports"
		}
		uploader := func(ctx context.Context, bucket, filename, contentType string, content []byte) (string, error) {
			if _, err := storageSvc.Upload(ctx, bucket, filename, contentType, bytes.NewReader(content)); err != nil {
				return "", err
			}
			return storageSvc.GetPresignedURL(ctx, bucket, filename, 7*24*time.Hour)
		}
		worker := exports.NewWorker(exportService, exportRepo, exportBucket, uploader)
		go worker.Run(workerCtx)
		logger.Info("export worker started", "bucket", exportBucket)
	} else {
		logger.Warn("storage not configured — export worker NOT started; jobs will queue but never process")
	}

	// API V1
	v1 := r.Group("/api/v1")

	// Internal-only routes (X-Internal-Key auth). Used by toca-financial when
	// rendering the transfer receipt PDF and any other service that needs to
	// resolve a lead by ID without holding a user JWT. Mounted BEFORE the
	// global auth middleware so it never sees the JWT path.
	// P1/A3: aceita chave de serviço OU JWT de usuário, e o tenant é resolvido
	// pela credencial (antes o `?tenant_id=` era validado e ignorado).
	internal := r.Group("/api/v1/crm/internal")
	internal.Use(auth.InternalOrUserMiddleware(auth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}))
	{
		internal.GET("/leads/:id", leadHandler.GetByIDInternal)
	}

	// Swagger — P2/M16: só fora de release (pkg/apidocs).
	apidocs.Register(v1, "/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Apply Auth Middleware
	v1.Use(auth.InternalOrMiddleware(auth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}), auth.RejectFunctionalRole("cliente"))

	leads := v1.Group("/leads")
	{
		// ÚNICA rota aprovada para tráfego inter-serviço via X-Internal-Key.
		leads.POST("", auth.RequirePermissionOrInternalKey(permissionProvider, "leads:write"), leadHandler.Create)

		// Demais rotas: exigem JWT (não aceitam internal key)
		leadsProtected := leads.Group("", auth.RejectFunctionalRole("service"))

		leadsProtected.GET("/roulette", auth.RequireAnyPermission(permissionProvider, "leads:view_roulette", "leads:configure_roulette"), leadHandler.GetRouletteConfig)
		leadsProtected.GET("/roulette/unassigned", auth.RequireAnyPermission(permissionProvider, "leads:view_roulette", "leads:configure_roulette"), leadHandler.GetRouletteUnassignedStatus)
		leadsProtected.PUT("/roulette", auth.RequirePermission(permissionProvider, "leads:configure_roulette"), leadHandler.UpdateRouletteConfig)
		leadsProtected.POST("/roulette/simulate", auth.RequirePermission(permissionProvider, "leads:configure_roulette"), leadHandler.SimulateRoulette)
		leadsProtected.GET("/roulette/history", auth.RequireAnyPermission(permissionProvider, "leads:view_roulette", "leads:configure_roulette"), historyHandler.ListRouletteHistory)
		leadsProtected.GET("/roulette/audit", auth.RequirePermission(permissionProvider, "system:view_audit_trail"), auditHandler.ListRouletteAudit)

		// P1/A10: permissão EXPLÍCITA. Antes estas rotas usavam
		// `RequirePermission(provider, "")`, que só exige "estar autenticado com
		// papel funcional" — qualquer papel (inclusive `financeiro`/`parceiro`,
		// que não têm `leads:read` em catálogo algum) lia a base de leads com
		// PII completa, e as buscas por e-mail/telefone permitiam harvesting.
		leadsProtected.GET("", auth.RequirePermission(permissionProvider, "leads:read"), leadHandler.List)
		leadsProtected.GET("/by-email/:email", auth.RequirePermission(permissionProvider, "leads:read"), leadHandler.GetByEmail)
		leadsProtected.GET("/by-phone/:phone", auth.RequirePermission(permissionProvider, "leads:read"), leadHandler.GetByPhone)
		leadsProtected.GET("/by-display-id/:display_id", auth.RequirePermission(permissionProvider, "leads:read"), leadHandler.GetByDisplayID)
		leadsProtected.PUT("/:id", auth.RequirePermission(permissionProvider, "leads:write"), leadHandler.RequireLeadOwnership(), leadHandler.Update)
		leadsProtected.PATCH("/:id", auth.RequirePermission(permissionProvider, "leads:write"), leadHandler.RequireLeadOwnership(), leadHandler.Patch)
		leadsProtected.GET("/stats", auth.RequirePermission(permissionProvider, "leads:read"), leadHandler.GetStats)
		// P1/A10: `leads:read` + posse verificada no handler (o corretor só lê
		// lead atribuído a ele ou ainda não atribuído — a mesma regra que a UI
		// aplicava sozinha).
		leadsProtected.GET("/:id", auth.RequirePermission(permissionProvider, "leads:read"), leadHandler.GetByID)
		leadsProtected.POST("/:id/notes", auth.RequirePermission(permissionProvider, "leads:write"), leadHandler.RequireLeadOwnership(), leadHandler.AddNote)
		// Corretor recomenda um imóvel ao cliente (sugestão exibida no portal).
		leadsProtected.PUT("/:id/recommended-property", auth.RequirePermission(permissionProvider, "leads:write"), leadHandler.RequireLeadOwnership(), leadHandler.SetRecommendedProperty)
		// Corretor confirma a visita solicitada pelo cliente.
		leadsProtected.PUT("/:id/visit/confirm", auth.RequirePermission(permissionProvider, "leads:write"), leadHandler.RequireLeadOwnership(), leadHandler.ConfirmLeadVisit)
		// Issue #427 — corretor registra primeiro contato. Idempotente.
		// leads:write porque transita status; mesma permission usada em
		// outras ações de qualificação.
		leadsProtected.POST("/:id/first-contact", auth.RequirePermission(permissionProvider, "leads:write"), leadHandler.RequireLeadOwnership(), leadHandler.RegisterFirstContact)
		// Issue #429 — corretor desqualifica lead sem potencial com motivo
		// obrigatório. leads:write — ação de qualificação como as demais.
		leadsProtected.POST("/:id/disqualify", auth.RequirePermission(permissionProvider, "leads:write"), leadHandler.RequireLeadOwnership(), leadHandler.Disqualify)

		// Issue #474 — Async export de leads em CSV/PDF. Permission
		// "leads:read" porque é leitura agregada — quem pode listar
		// pode exportar.
		// P0/A22: era `leads:read` ("quem pode listar pode exportar"); exportar
		// é a capacidade própria `leads:import_export`.
		leadsProtected.POST("/export", auth.RequirePermission(permissionProvider, "leads:import_export"), exportHandler.CreateLeadExport)
		// Lead → Client conversion (single source of truth: same ID, new role)
		leadsProtected.POST("/:id/convert", auth.RequirePermission(permissionProvider, "leads:write"), leadHandler.RequireLeadOwnership(), leadHandler.ConvertToClient)
		leadsProtected.POST("/:id/invite", auth.RequirePermission(permissionProvider, "leads:write"), leadHandler.RequireLeadOwnership(), leadHandler.InviteClient)

		// Roleta — aceite/rejeição da atribuição pelo corretor (#425/#426).
		// Permission "leads:write" é satisfeita por todos os perfis que
		// recebem leads pela roleta (corretor, gestor). O service valida
		// que JWT.account_id == lead.assigned_broker_id (impede que um
		// corretor aceite lead de outro).
		leadsProtected.POST("/:id/accept-assignment", auth.RequirePermission(permissionProvider, "leads:write"), leadHandler.AcceptAssignment)
		leadsProtected.POST("/:id/reject-assignment", auth.RequirePermission(permissionProvider, "leads:write"), leadHandler.RejectAssignment)

		// Match Properties / Cruzamento Inteligente (#415). Devolve
		// imóveis disponíveis rankeados por compatibilidade com o
		// property_interest do lead. Permission leads:read porque o
		// caller só precisa ler o lead — a chamada interna para
		// /properties já é gated por properties:read no toca-core via
		// JWT herdado.
		leadsProtected.GET("/:id/matched-properties", auth.RequirePermission(permissionProvider, "leads:read"), leadHandler.GetMatchedProperties)
	}

	// Client portal — overview (lead + imóveis recomendados) para o cliente
	// convidado (functional_role "cliente"). O gateway roteia /client-portal/overview
	// aqui. Registrado no root (não em v1) para não herdar o
	// RejectFunctionalRole("cliente") do grupo de operador.
	clientPortal := r.Group("/api/v1/client-portal")
	clientPortal.Use(auth.InternalOrMiddleware(auth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}), auth.RequireFunctionalRole("cliente"))
	{
		clientPortal.GET("/overview", leadHandler.GetClientPortalOverview)
		// Vitrine do cliente SEM negócio aberto: imóveis disponíveis das
		// imobiliárias com quem ele se relaciona. A busca do portal é
		// `deal-scoped`; este é o caminho de quem só tem lead.
		clientPortal.GET("/properties", leadHandler.GetClientPortalProperties)
		clientPortal.POST("/leads/:id/visit-request", leadHandler.RequestLeadVisit)
		clientPortal.POST("/leads/:id/channel", leadHandler.OpenDirectChannel)
		clientPortal.PUT("/leads/:id/visit", leadHandler.UpdateLeadVisit)
		clientPortal.DELETE("/leads/:id/visit", leadHandler.CancelLeadVisit)
		clientPortal.GET("/leads/:id/visit/calendar.ics", leadHandler.DownloadLeadVisitCalendar)
	}

	// Partnerships — shared-content surface owned by toca-crm. Only the
	// aggregated shared-leads endpoint lives here; the partnership lifecycle
	// itself is owned by toca-core. Gated by `shared_content:read`; the
	// service additionally enforces the per-partnership `leads` view scope.
	partnerships := v1.Group("/partnerships", auth.RejectFunctionalRole("service"))
	{
		partnerships.GET("/shared/leads", auth.RequirePermission(permissionProvider, "shared_content:read"), leadHandler.SharedLeads)
	}

	// Brokers: single registration, reused across properties, leads, and deals
	brokers := v1.Group("/brokers", auth.RejectFunctionalRole("service"))
	{
		brokers.POST("", auth.RequirePermission(permissionProvider, "brokers:write"), brokerHandler.Create)
		// Everyone who is logged in should be able to read brokers for communication/mentions
		// A LISTA de corretores é dado de referência usado por seletores em
		// telas de VÁRIOS papéis (captação de imóvel, nova comissão, filtros de
		// relatório). Exigir `brokers:read` (admin/gestor/operacoes) esvaziaria
		// esses seletores para corretor e financeiro — regressão funcional que o
		// achado A10 não pedia. O que vazava PII era a leitura POR ID, que agora
		// exige `brokers:read` e é tenant-scoped no SQL (A19).
		brokers.GET("", auth.RequirePermission(permissionProvider, ""), brokerHandler.List)
		brokers.GET("/:id", auth.RequirePermission(permissionProvider, "brokers:read"), brokerHandler.GetByID)
		brokers.GET("/:id/performance", auth.RequirePermission(permissionProvider, "brokers:read"), brokerHandler.GetPerformance)
		// Auto-serviço: o corretor atualiza a PRÓPRIA qualificação. Mantém-se sem
		// permissão de catálogo de propósito (não há `brokers:self_write`), mas o
		// handler resolve a linha pelo account id DO CHAMADOR dentro do tenant do
		// token — não há como atingir o registro de outro corretor.
		brokers.PATCH("/me/qualification", auth.RequirePermission(permissionProvider, ""), brokerHandler.UpdateMyQualification)
		brokers.PUT("/:id", auth.RequirePermission(permissionProvider, "brokers:write"), brokerHandler.Update)
		brokers.DELETE("/:id", auth.RequirePermission(permissionProvider, "brokers:delete"), brokerHandler.Delete)
	}

	customers := v1.Group("/customers", auth.RejectFunctionalRole("service"))
	{
		customers.GET("/:id", auth.RequirePermission(permissionProvider, "customers:read"), customerHandler.GetProfile)
		customers.GET("/:id/summary", auth.RequirePermission(permissionProvider, "customers:read"), customerHandler.GetSummary)
		customers.GET("/:id/leads", auth.RequirePermission(permissionProvider, "customers:read"), customerHandler.ListLeads)
		customers.GET("/:id/deals", auth.RequirePermission(permissionProvider, "customers:read"), auth.RequirePermission(permissionProvider, "deals:read"), customerHandler.ListDeals)
		customers.GET("/:id/charges", auth.RequirePermission(permissionProvider, "customers:read"), auth.RequirePermission(permissionProvider, "charges:read"), customerHandler.ListCharges)
		customers.GET("/:id/appointments", auth.RequirePermission(permissionProvider, "customers:read"), auth.RequirePermission(permissionProvider, "leads:read"), customerHandler.ListAppointments)
		customers.GET("/:id/tickets", auth.RequirePermission(permissionProvider, "customers:read"), auth.RequirePermission(permissionProvider, "tickets:read"), customerHandler.ListTickets)
	}

	// Issue #474 — endpoints comuns de export (status + listagem).
	// POST /leads/export está sob /leads acima; GET /exports/* fica
	// aqui porque é transversal (futuro: deals/export, charges/export
	// vão escrever no mesmo registry e ler do mesmo endpoint).
	// P0/A21 + A22: export é extração em MASSA de PII, não leitura comum.
	// Antes o grupo não tinha permissão NEM rejeição de identidade de serviço
	// (a chave interna alcançava a rota com `X-Tenant-Id` arbitrário), e a
	// criação do job era gated por `leads:read` — quem podia listar podia
	// despejar a base inteira em CSV com URL pré-assinada de 7 dias.
	// `leads:import_export` é a capacidade documentada (ROLES-MATRIX:43).
	exportsGroup := v1.Group("/exports")
	exportsGroup.Use(auth.RejectFunctionalRole("service"))
	{
		exportsGroup.GET("/mine", auth.RequirePermission(permissionProvider, "leads:import_export"), exportHandler.ListMyExports)
		exportsGroup.GET("/:id", auth.RequirePermission(permissionProvider, "leads:import_export"), exportHandler.GetExport)
	}

	return r
}
