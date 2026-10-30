package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca-core/internal/app"
	"github.com/oginoino/toca-core/internal/middleware"
	service "github.com/oginoino/toca-core/internal/services"
	"github.com/oginoino/toca/pkg/apidocs"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	pkgmw "github.com/oginoino/toca/pkg/middleware"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/pkg/validation"

	_ "github.com/oginoino/toca-core/docs"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func SetupRouter(
	cfg *app.Config,
	propertyService service.PropertyService,
	partnershipService *service.PartnershipService,
	settingsService *service.SettingsService,
	storageSvc storage.Service,
	// eventBus is used by the captação aggregator handler to emit
	// `property.captured`. Pass nil when RabbitMQ is not wired (the handler
	// degrades gracefully — the property is still persisted).
	eventBus events.EventBus,
) *gin.Engine {
	if cfg.AppMode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	_ = validation.InitGin()

	// Initialize JWKS for Supabase JWT validation
	if err := pkgAuth.InitJWKS(cfg.JWT.SupabaseJWKSURL); err != nil {
		logger.Error("Failed to initialize JWKS", map[string]interface{}{"error": err.Error()})
	}

	// P1/A2: registra no boot se o serviço está no modelo antigo (chave
	// compartilhada, sem allowlist de tenant) — em produção isso é alarme.
	pkgAuth.LogServiceKeyRollout()

	r := gin.New()
	r.MaxMultipartMemory = 12 << 20 // 12 MB

	// Base middleware stack (request id, structured logger, request
	// timeout, panic recovery) consolidated in pkg/middleware. CORS is
	// intentionally NOT applied here — the gateway terminates CORS for
	// the whole platform.
	r.Use(pkgmw.Setup(pkgmw.StackConfig{})...)
	r.Use(middleware.RateLimitMiddleware(cfg.RateLimit.RPS, cfg.RateLimit.Burst))
	r.Use(metrics.GinMiddleware("toca-core"))

	// Authentication Middleware
	// Applied globally but skips public endpoints via whitelist check inside the middleware
	// Alternatively, we can apply it to specific groups. Here we apply globally for simplicity and security by default.
	r.Use(pkgAuth.InternalOrMiddleware(pkgAuth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}), pkgAuth.RejectFunctionalRole("cliente"))

	// Dependencies
	propertyHandler := NewPropertyHandler(propertyService, storageSvc, eventBus)
	partnershipHandler := NewPartnershipHandler(partnershipService)
	healthHandler := NewHealthHandler()
	settingsHandler := NewSettingsHandler(settingsService)
	permissionsHandler := NewPermissionsHandler(settingsService)
	captacaoHandler := NewCaptacaoHandler(propertyService, eventBus)

	permissionAdapter := middleware.NewPermissionAdapter(settingsService)

	// API Versioning
	// Health check (no auth)
	r.Any("/health", healthHandler.HealthCheck)
	r.GET("/metrics", metrics.Handler())

	v1 := r.Group("/api/v1")
	{
		// Swagger — P2/M16: só fora de release (pkg/apidocs).
		apidocs.Register(v1, "/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

		// Health check (compatibilidade com Gateway: /api/v1/health)
		v1.Any("/health", healthHandler.HealthCheck)

		// This route must have auth middleware explicitly, or be inside a group with auth middleware if it relies on JWT context
		v1.GET("/me/permissions", pkgAuth.RequirePermission(permissionAdapter, ""), permissionsHandler.GetMyPermissions)

		properties := v1.Group("/properties")
		{
			// `Create` registers a brand-new property in the portfolio — that's
			// the captação act, so it requires `properties:capture`. Existing
			// edits (Update/UploadImage/etc.) keep `properties:write`. Adding
			// owners and capturing brokers to an already-created property are
			// also part of the captação flow and so are gated by the same
			// permission. See plan Phase 1 item 4.
			properties.POST("", pkgAuth.RequirePermission(permissionAdapter, "properties:capture"), propertyHandler.Create)
			properties.GET("/:id", pkgAuth.RequirePermission(permissionAdapter, "properties:read"), propertyHandler.GetByID)
			properties.GET("", pkgAuth.RequirePermission(permissionAdapter, "properties:read"), propertyHandler.GetAll)
			properties.PUT("/:id", pkgAuth.RequirePermission(permissionAdapter, "properties:write"), propertyHandler.Update)
			properties.DELETE("/:id", pkgAuth.RequirePermission(permissionAdapter, "properties:delete"), propertyHandler.Delete)
			properties.POST("/:id/media/images", pkgAuth.RequirePermission(permissionAdapter, "properties:write"), propertyHandler.UploadImage)
			properties.DELETE("/:id/media/images", pkgAuth.RequirePermission(permissionAdapter, "properties:write"), propertyHandler.DeleteImage)

			// Cross-entity linking — single source of truth
			properties.POST("/:id/interested-leads", pkgAuth.RequirePermission(permissionAdapter, "properties:write"), propertyHandler.AddInterestedLead)
			properties.DELETE("/:id/interested-leads/:leadId", pkgAuth.RequirePermission(permissionAdapter, "properties:write"), propertyHandler.RemoveInterestedLead)
			properties.POST("/:id/capturing-brokers", pkgAuth.RequirePermission(permissionAdapter, "properties:capture"), propertyHandler.AddCapturingBroker)
			properties.POST("/:id/owners", pkgAuth.RequirePermission(permissionAdapter, "properties:capture"), propertyHandler.AddOwner)
			properties.PATCH("/:id/active-deal", pkgAuth.RequirePermission(permissionAdapter, "properties:write"), propertyHandler.SetActiveDeal)
		}

		// Captação aggregator endpoint — Phase 3.4. Single POST creates the
		// Property, attaches owner-leads and capturing brokers, and publishes
		// `property.captured`. Same `properties:capture` permission as the
		// individual sub-endpoints (no extra permission needed).
		captacao := v1.Group("/captacao")
		{
			captacao.POST("", pkgAuth.RequirePermission(permissionAdapter, "properties:capture"), captacaoHandler.Capturar)
		}

		partnerships := v1.Group("/partnerships")
		{
			// Writes gate on `partnerships:write` (admin/gestor); reads gate
			// on `partnerships:read`. Migrated off `company:manage_members`
			// so partnership management is no longer coupled to blanket
			// company administration.
			partnerships.POST("", pkgAuth.RequirePermission(permissionAdapter, "partnerships:write"), partnershipHandler.Create)
			partnerships.GET("", pkgAuth.RequirePermission(permissionAdapter, "partnerships:read"), partnershipHandler.List)
			// Aggregated shared properties across every active partnership the
			// caller participates in. STATIC path — registered BEFORE the
			// `/:id` param routes so gin does not capture "shared" as an :id.
			partnerships.GET("/shared/properties", pkgAuth.RequirePermission(permissionAdapter, "shared_content:read"), partnershipHandler.AllSharedProperties)
			partnerships.GET("/:id", pkgAuth.RequirePermission(permissionAdapter, "partnerships:read"), partnershipHandler.GetByID)
			partnerships.PUT("/:id/agreement", pkgAuth.RequirePermission(permissionAdapter, "partnerships:write"), partnershipHandler.ChangeAgreement)
			partnerships.POST("/:id/accept", pkgAuth.RequirePermission(permissionAdapter, "partnerships:write"), partnershipHandler.LegacyMutation)
			partnerships.POST("/:id/reject", pkgAuth.RequirePermission(permissionAdapter, "partnerships:write"), partnershipHandler.LegacyMutation)
			partnerships.PUT("/:id/financial-config", pkgAuth.RequirePermission(permissionAdapter, "partnerships:write"), partnershipHandler.LegacyMutation)
			partnerships.PUT("/:id/permissions", pkgAuth.RequirePermission(permissionAdapter, "partnerships:write"), partnershipHandler.LegacyMutation)
			partnerships.PUT("/:id/sharing-config", pkgAuth.RequirePermission(permissionAdapter, "partnerships:write"), partnershipHandler.LegacyMutation)
			// Shared portfolio — returns the counterpart company's shared
			// properties. No extra permission middleware: the service
			// enforces the partnership-level shared_portfolio permission and
			// verifies the caller participates in the partnership.
			partnerships.GET("/:id/shared-portfolio", pkgAuth.RequirePermission(permissionAdapter, "shared_content:read"), partnershipHandler.SharedPortfolio)
		}

		// Rotas INTERNAS (serviço-a-serviço): chegam com a credencial interna e
		// sem JWT de usuário — é o caso dos fluxos que montam dias civis
		// (financeiro, negócios, agenda) sem usuário na ponta.
		coreInternal := v1.Group("/core/internal")
		coreInternal.Use(pkgAuth.RequireInternalKey())
		{
			coreInternal.GET("/tenant-settings", settingsHandler.GetTenantTimezoneInternal)
			// DIMOB: o financeiro lê o imóvel (endereço/CEP/UF) para o leiaute.
			coreInternal.GET("/properties", propertyHandler.ListInternal)
			coreInternal.GET("/properties/:id", propertyHandler.GetByIDInternal)
		}

		companySettings := v1.Group("/company-settings")
		{
			// P0/A1 — as leituras que expõem SEGREDO ou o MAPA DE AUTORIZAÇÃO
			// exigem permissão: a listagem de webhooks devolve o signing secret,
			// a de API keys devolve metadados de credencial e a matriz de
			// permissões orienta qualquer escalada. Antes só as escritas tinham
			// guarda e qualquer autenticado (inclusive `corretor`/`auditor`) lia
			// tudo isso.
			//
			// `GET ""` (dados gerais) e `GET /appearance` continuam legíveis por
			// qualquer membro do tenant de propósito: o `ThemeContext` do
			// frontend os carrega para TODO usuário autenticado
			// (toka/src/shared/context/ThemeContext.tsx:207-208) para aplicar o
			// white-label; guardá-los apagaria logo/cores para corretor,
			// financeiro e auditor (o `.catch` cai no tema padrão). Exposição
			// aceita: nome/CNPJ/endereço e tokens visuais da própria empresa —
			// nada disso é segredo nem matriz. O estado final desejado é uma
			// projeção pública/derivada só com os campos de branding (P1).
			companySettings.GET("", settingsHandler.GetCompanySettings)
			companySettings.PUT("", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_general"), settingsHandler.UpdateCompanySettings)
			companySettings.POST("/logo", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_appearance"), settingsHandler.UploadLogo)
			// Banner da tela de login (white label). Mesma permissão do logo:
			// `settings:manage_appearance` = admin e marketing. Voltar ao preset
			// não precisa de rota própria — a aba manda `loginBannerUrl: ""` no
			// PUT /appearance.
			companySettings.POST("/login-banner", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_appearance"), settingsHandler.UploadLoginBanner)

			companySettings.GET("/appearance", settingsHandler.GetAppearance)
			companySettings.PUT("/appearance", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_appearance"), settingsHandler.UpdateAppearance)

			companySettings.GET("/permissions", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_permissions"), settingsHandler.GetPermissions)
			companySettings.PUT("/permissions", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_permissions"), settingsHandler.UpdatePermissions)

			// Perfis de acesso personalizados — a unidade atribuível por um Grupo
			// de Usuários (toca-auth). Mesma permissão da matriz: quem define o
			// que um perfil pode também cria/edita/apaga o perfil.
			companySettings.GET("/access-profiles", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_permissions"), settingsHandler.ListAccessProfiles)
			companySettings.POST("/access-profiles", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_permissions"), settingsHandler.CreateAccessProfile)
			companySettings.PUT("/access-profiles/:profile_id", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_permissions"), settingsHandler.UpdateAccessProfile)
			companySettings.DELETE("/access-profiles/:profile_id", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_permissions"), settingsHandler.DeleteAccessProfile)

			// Sem consumidor global no frontend (só a aba de Notificações, que já
			// exige `settings:manage_notifications`) — guarda mantida.
			companySettings.GET("/notifications", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_notifications"), settingsHandler.GetNotifications)
			companySettings.PUT("/notifications", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_notifications"), settingsHandler.UpdateNotifications)

			companySettings.POST("/webhooks", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_integrations"), settingsHandler.CreateWebhook)
			companySettings.GET("/webhooks", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_integrations"), settingsHandler.ListWebhooks)
			companySettings.PUT("/webhooks/:id", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_integrations"), settingsHandler.UpdateWebhook)
			companySettings.DELETE("/webhooks/:id", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_integrations"), settingsHandler.DeleteWebhook)

			companySettings.POST("/api-keys", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_integrations"), settingsHandler.CreateAPIKey)
			companySettings.GET("/api-keys", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_integrations"), settingsHandler.ListAPIKeys)
			companySettings.DELETE("/api-keys/:id", pkgAuth.RequirePermission(permissionAdapter, "settings:manage_integrations"), settingsHandler.RevokeAPIKey)
		}
	}

	return r
}
