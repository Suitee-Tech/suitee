package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/apidocs"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/pkg/validation"
	_ "github.com/oginoino/toca/toca-portal/docs"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func SetupRouter(
	cfg config.BaseConfig,
	integHandler *IntegrationHandler,
	feedHandler *FeedHandler,
	webhookHandler *WebhookHandler,
	syncHandler *SyncHandler,
	statsHandler *StatsHandler,
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
	r.Use(gin.Recovery())
	r.Use(gin.Logger())
	r.Use(metrics.GinMiddleware("toca-portal"))

	r.GET("/metrics", metrics.Handler())

	// Health check (no auth)
	r.Any("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "toca-portal"})
	})

	// ===== PUBLIC ROUTES (no JWT) =====
	// These are accessed by portal crawlers and webhook senders
	public := r.Group("/api/v1/portals")
	{
		// XML Feed endpoint - crawled by portal bots (ZAP, OLX, VivaReal)
		public.GET("/feed/:token", feedHandler.ServeFeed)

		// Webhook receiver for incoming portal leads
		public.POST("/webhook/:integrationID", webhookHandler.ReceiveLead)
	}

	// Swagger — P2/M16: só fora de release (pkg/apidocs). Antes era registrado
	// fora do grupo autenticado e sem gate de ambiente.
	apidocs.Register(r.Group("/api/v1"), "/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// ===== AUTHENTICATED ROUTES (JWT required) =====
	v1 := r.Group("/api/v1")
	v1.Use(auth.InternalOrMiddleware(auth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}), auth.RejectFunctionalRole("cliente"))

	// ABAC/RBAC: as rotas de integração exigem a permission
	// settings:manage_integrations (mesmo padrão do toca-signature).
	// O feed público e o webhook público NÃO passam por aqui (seguem sem auth).
	permissionProvider := auth.NewCorePermissionProvider(clients.NewCoreClient())
	requireManageIntegrations := auth.RequirePermission(permissionProvider, "settings:manage_integrations")

	// Integration management
	integrations := v1.Group("/integrations")
	{
		integrations.POST("", requireManageIntegrations, integHandler.Create)
		integrations.GET("", requireManageIntegrations, integHandler.List)
		integrations.GET("/:id", requireManageIntegrations, integHandler.GetByID)
		integrations.PUT("/:id", requireManageIntegrations, integHandler.Update)
		integrations.DELETE("/:id", requireManageIntegrations, integHandler.Delete)
		integrations.POST("/:id/sync", requireManageIntegrations, syncHandler.TriggerSync)
		integrations.GET("/:id/syncs", requireManageIntegrations, syncHandler.ListSyncs)
		// P1/A8: a leitura dos leads de uma integração estava isenta do gate com a
		// justificativa de que "bloquearia telas de papéis não-admin". A superfície
		// foi conferida: o ÚNICO consumidor é a aba de integrações das
		// configurações (`IntegrationsTab` → `portalIntegrationService`), que o
		// cliente já gateia por `settings:manage_integrations` (a mesma permissão
		// do resto do grupo). Manter a rota aberta só servia a quem chamasse a API
		// direto.
		integrations.GET("/:id/leads", requireManageIntegrations, statsHandler.LeadsByIntegration)
	}

	// Portal stats dashboard — P1/A8: nenhum consumidor no frontend; são
	// agregados do módulo de portais, então valem a mesma permissão de gestão de
	// integrações (divergência de default registrada em
	// docs/features/ROLES-MATRIX.md §Reconciliações: a matriz concede integrações
	// a operacoes/marketing e o catálogo só a admin — conceder é decisão de
	// produto, não de endurecimento).
	stats := v1.Group("/portal-stats")
	{
		stats.GET("/dashboard", requireManageIntegrations, statsHandler.Dashboard)
		stats.GET("/leads", requireManageIntegrations, statsHandler.PortalLeads)
	}

	return r
}
