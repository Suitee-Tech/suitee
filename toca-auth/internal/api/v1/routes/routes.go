package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	_ "github.com/oginoino/toca-auth/docs"
	"github.com/oginoino/toca-auth/internal/api/v1/handlers"
	"github.com/oginoino/toca-auth/internal/api/v1/middleware"
	"github.com/oginoino/toca-auth/internal/app"
	redisclient "github.com/oginoino/toca-auth/internal/infrastructure/redis"
	"github.com/oginoino/toca-auth/internal/infrastructure/supabase"
	"github.com/oginoino/toca-auth/internal/repository"
	"github.com/oginoino/toca-auth/internal/service"
	"github.com/oginoino/toca/pkg/apidocs"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	pkgmw "github.com/oginoino/toca/pkg/middleware"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/pkg/validation"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func SetupRouter(cfg *app.Config, pgDB *database.PostgresDB, storageService storage.Service, eventBus events.EventBus) *gin.Engine {
	if cfg.AppMode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	_ = validation.InitGin()

	// Initialize JWKS for Supabase JWT validation
	if err := pkgAuth.InitJWKS(cfg.JWT.SupabaseJWKSURL); err != nil {
		logger.Error("Failed to initialize JWKS", map[string]interface{}{"error": err.Error()})
	}

	r := gin.New()
	r.MaxMultipartMemory = 8 << 20 // 8 MB

	// Base middleware stack (request id, structured logger, request
	// timeout, panic recovery) consolidated in pkg/middleware. CORS is
	// intentionally NOT applied here — the gateway terminates CORS for
	// the whole platform.
	r.Use(pkgmw.Setup(pkgmw.StackConfig{})...)
	r.Use(middleware.RateLimitMiddleware(cfg.RateLimit.RPS, cfg.RateLimit.Burst))
	r.Use(metrics.GinMiddleware("toca-auth"))

	r.GET("/metrics", metrics.Handler())

	// Health check (no auth)
	r.Any("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "toca-auth"})
	})

	// Dependencies — Redis (optional). Used for OTP flows.
	rdb, err := redisclient.New(cfg.Redis.URI)
	if err != nil {
		logger.Warn("Redis unavailable", "error", err)
	} else {
		logger.Info("Redis connected")
	}

	accountRepo := repository.NewAccountPostgresRepository(pgDB)
	credentialRepo := repository.NewCredentialPostgresRepository(pgDB)
	invitationRepo := repository.NewInvitationPostgresRepository(pgDB)
	preferencesRepo := repository.NewPreferencesPostgresRepository(pgDB)

	// Service-role (BYPASSRLS) surfaces for pre-authentication flows
	// (register/login/OTP/e-mail availability and the public invite link) that
	// run before a JWT exists.
	accountServiceRepo := repository.NewAccountServicePostgresRepository(pgDB)
	credentialServiceRepo := repository.NewCredentialServicePostgresRepository(pgDB)
	invitationServiceRepo := repository.NewInvitationServicePostgresRepository(pgDB)

	// Como TenantSettings e Groups não foram inteiramente convertidos pro Postgres no piloto (precisam existir)
	// Vamos stubbá-los com nil ou criar implementações stub para compilar, pois a ordem foi focar na eliminação do Mongo.
	tenantSettingsRepo := repository.NewTenantSettingsPostgresRepository(pgDB)
	groupRepo := repository.NewGroupPostgresRepository(pgDB)

	supabaseClient, err := supabase.NewAuthClientFromEnv()
	if err != nil {
		logger.Error("Failed to initialize Supabase Auth client", map[string]interface{}{"error": err.Error()})
		panic(err)
	}

	authService := service.NewAuthService(accountRepo, credentialRepo, invitationRepo, preferencesRepo, tenantSettingsRepo, storageService, eventBus, cfg, rdb, supabaseClient, accountServiceRepo, credentialServiceRepo)
	approvalPolicyService := service.NewApprovalPolicyService(tenantSettingsRepo)

	regService := service.NewRegistrationService(
		accountServiceRepo,
		supabaseClient,
		eventBus,
		cfg,
	)

	// A lista de equipe e os avatares dos membros dos grupos precisam ler a
	// conta/credencial de OUTROS membros — o que a RLS nega ao pool de aplicação
	// (`id = auth.uid()`). Essas leituras passam pela pool de serviço.
	groupService := service.NewGroupService(groupRepo, accountRepo).
		WithAccountServiceRepo(accountServiceRepo)

	companyService := service.NewCompanyService(
		accountRepo,
		credentialRepo,
		groupRepo,
		groupService,
	).WithServiceRepos(accountServiceRepo, credentialServiceRepo)

	invitationService := service.NewInvitationService(
		accountRepo,
		credentialRepo,
		invitationRepo,
		authService,
		regService,
		eventBus,
		cfg,
		groupService,
		accountServiceRepo,
		credentialServiceRepo,
		invitationServiceRepo,
	)

	prefsService := service.NewPreferencesService(
		preferencesRepo,
	)

	passwordService := service.NewPasswordService(
		credentialRepo,
		authService,
		eventBus,
		cfg,
	)

	// Handlers
	authHandler := handlers.NewAuthHandler(authService, regService, companyService, invitationService, prefsService, passwordService, cfg)
	settingsHandler := handlers.NewSettingsHandler(authService, prefsService, passwordService)
	groupHandler := handlers.NewGroupHandler(groupService)
	customerHandler := handlers.NewCustomerHandler()
	auditHandler := handlers.NewAuditHandler()
	approvalPolicyHandler := handlers.NewApprovalPolicyHandler(approvalPolicyService)
	// LGPD handler — Issue #475 (Art. 9 direito de acesso). #476 (Art. 18
	// direito ao esquecimento) virá em PR separado.
	lgpdHandler := handlers.NewLGPDHandler(authService, prefsService)

	// Permission provider — Core fica em outro serviço, falamos com ele
	// via HTTP. Cache interno do CorePermissionProvider lida com a
	// latência. Issue #470: necessário pro gate de /audit-trail.
	permissionProvider := pkgAuth.NewCorePermissionProvider(clients.NewCoreClient())

	// API Versioning
	v1 := r.Group("/api/v1")
	{
		// Swagger — P2/M16: só fora de release (pkg/apidocs).
		apidocs.Register(v1, "/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.POST("/oauth/login", authHandler.OAuthLogin)
			auth.POST("/refresh", authHandler.RefreshToken)
			auth.POST("/forgot-password", authHandler.ForgotPassword)
			// `reset-password` recebe a sessão de recuperação (par
			// access/refresh do fragmento do link do Supabase) e aplica a senha
			// NO SERVIDOR. O fluxo local de token (P1/A14) segue removido — ele
			// nunca criava o token. Este caminho existe para o navegador não
			// precisar da URL/anon key do Supabase, que era o último motivo
			// para uma credencial aparecer em /env.js.
			auth.POST("/reset-password", authHandler.ResetPassword)
			auth.GET("/invitations/:token", authHandler.GetInvitation)
			auth.POST("/invitations/:token/accept", authHandler.AcceptPublicInvitation)
			auth.POST("/otp/request", authHandler.RequestOTP)
			auth.POST("/otp/verify", authHandler.VerifyOTP)

			// Portal isolated endpoints
			auth.POST("/portal/refresh", authHandler.PortalRefreshToken)

			// Internal endpoints.
			//
			// SECURITY (achado M8 da revisão de permissões de 2026-09-19): estas
			// duas rotas comparavam `X-Internal-Key` com `!=` (comparação não
			// constante), sem rate limit, sem auditoria e sem aceitar as chaves
			// NOMEADAS (INTERNAL_SERVICE_KEYS) — enquanto a rota vizinha já usava
			// `RequireInternalKey()`. Agora todas as internas usam a mesma guarda:
			// comparação em tempo constante, 60/min por IP, log de acesso/negativa
			// e vínculo de tenant quando a chave nomeada declara `nome:chave[:tenants]`.
			auth.GET("/internal/tenants/:tenant_id/approval-policy", pkgAuth.RequireInternalKey(), approvalPolicyHandler.GetInternal)

			auth.POST("/internal/accounts/ensure-client", pkgAuth.RequireInternalKey(), authHandler.EnsureClientAccount)

			auth.GET("/internal/companies/:id/members", pkgAuth.RequireInternalKey(), authHandler.ListCompanyMembersInternal)

			// Protected routes
			auth.Use(pkgAuth.InternalOrMiddleware(pkgAuth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}))
			{
				auth.GET("/me", authHandler.Me)
				auth.PATCH("/me", authHandler.UpdateMe)
				auth.GET("/companies", authHandler.ListCompanies)
				auth.POST("/switch-context", authHandler.SwitchContext)
				auth.POST("/accept-invite", authHandler.AcceptInvitation)
				auth.POST("/logout", authHandler.Logout)
				auth.POST("/logout-all", authHandler.LogoutAll)
			}
		}

		// Audit trail global — Issue #470. Cross-tenant quando o
		// tenant_id query é vazio; gate por system:view_audit_trail
		// (admin, auditor, operacoes por default). Sob /api/v1 fora do
		// /auth group para não confundir o leitor da rota
		// (audit-trail não é auth).
		auditGroup := v1.Group("/audit-trail")
		auditGroup.Use(pkgAuth.InternalOrMiddleware(pkgAuth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}))
		{
			auditGroup.GET("", pkgAuth.RequirePermission(permissionProvider, "system:view_audit_trail"), auditHandler.ListAuditTrail)
		}

		// LGPD endpoints — Issue #475 (direito de acesso) + #476
		// (esquecimento, follow-up). JWT-protected (qualquer usuário
		// autenticado acessa os PRÓPRIOS dados — não precisa de
		// permission especial, é direito constitucional).
		lgpdGroup := v1.Group("/lgpd")
		lgpdGroup.Use(pkgAuth.InternalOrMiddleware(pkgAuth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}))
		{
			lgpdGroup.GET("/me/data", lgpdHandler.GetMyData)
			// #476 — Art. 18 esquecimento. Irreversível: requer
			// confirmação textual ("EXCLUIR MINHA CONTA") no body.
			lgpdGroup.DELETE("/me/account", lgpdHandler.DeleteMyAccount)
		}

		// Bootstrap de tenant — `POST /api/v1/companies` cria a empresa DO
		// CHAMADOR e, com ela, o primeiro vínculo (Profile "owner",
		// FunctionalRole "admin"; company_service.go). É a única rota sob
		// `/companies` que NÃO pode exigir papel funcional: a conta recém-
		// cadastrada chega aqui sem vínculo e, portanto, com `functional_role`
		// vazio — e o fail-closed do achado M2 respondia
		// 403 "no functional role assigned" (`authz.denied` na auditoria),
		// travando o onboarding: sem empresa não há papel, e sem papel não há
		// empresa. Por isso ela vive em um grupo próprio, sem o
		// RejectFunctionalRole do grupo de operador logo abaixo — mas o papel
		// `cliente` continua barrado (P0/A11).
		onboarding := v1.Group("/companies")
		onboarding.Use(pkgAuth.InternalOrMiddleware(pkgAuth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}))
		{
			onboarding.POST("", pkgAuth.RejectFunctionalRoleAllowUnassigned("cliente"), authHandler.CreateCompany)
		}

		// P0/A11: gestão de empresas/membros/convites/grupos é superfície de
		// operador. O portal do cliente usa /auth/*, /settings/preferences e
		// /lgpd/* — esses grupos permanecem acessíveis ao `cliente`.
		// P1/A8: cada rota recebe a permissão do catálogo. Antes, qualquer
		// operador autenticado (a única barreira era `RejectFunctionalRole`) lia e
		// regravava vínculo, papel, convite e grupo de qualquer empresa do tenant.
		//
		//   ver        → `company:read_members` (todos os papéis de operador: a
		//                lista alimenta seletores e nomes no chat);
		//   gerenciar  → `company:manage_members` (admin/gestor/operacoes).
		//
		// A checagem de VÍNCULO do chamador com a empresa (P0/C1) continua no
		// serviço: permissão não substitui "só mexo na minha empresa".
		companies := v1.Group("/companies")
		companies.Use(pkgAuth.InternalOrMiddleware(pkgAuth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}), pkgAuth.RejectFunctionalRole("cliente"))
		{
			// Static segment registered before the /:id param route so gin
			// routes "/companies/lookup" to the lookup handler rather than
			// capturing "lookup" as an :id.
			companies.GET("/lookup", authHandler.LookupCompanyByTaxID)
			companies.GET("/:id", authHandler.GetCompany)
			companies.PUT("/:id", pkgAuth.RequirePermission(permissionProvider, "company:manage_members"), authHandler.UpdateCompany)
			companies.POST("/:id/users", pkgAuth.RequirePermission(permissionProvider, "company:manage_members"), authHandler.AddUserToCompany)
			companies.GET("/:id/members", pkgAuth.RequirePermission(permissionProvider, "company:read_members"), authHandler.ListCompanyMembers)
			companies.DELETE("/:id/members/:member_id", pkgAuth.RequirePermission(permissionProvider, "company:manage_members"), authHandler.RemoveMemberFromCompany)
			// P2/M6+M7: alterar papel/perfil e suspender/reativar um vínculo
			// existente. Antes disso não havia endpoint: o papel só mudava por
			// efeito colateral de grupo/convite, e não havia como bloquear alguém
			// sem removê-lo da empresa.
			companies.PATCH("/:id/members/:member_id", pkgAuth.RequirePermission(permissionProvider, "company:manage_members"), authHandler.UpdateCompanyMember)
			companies.POST("/:id/invite", pkgAuth.RequirePermission(permissionProvider, "company:manage_members"), authHandler.CreateInvitation)
			companies.GET("/:id/invitations", pkgAuth.RequirePermission(permissionProvider, "company:manage_members"), authHandler.ListInvitations)
			companies.POST("/:id/invitations/:invitation_id/resend", pkgAuth.RequirePermission(permissionProvider, "company:manage_members"), authHandler.ResendInvitation)
			companies.POST("/:id/invitations/:invitation_id/cancel", pkgAuth.RequirePermission(permissionProvider, "company:manage_members"), authHandler.CancelInvitation)

			// Groups — mesma separação ver/gerenciar. `MoveMember` muda papel de
			// membro (é escrita, não leitura).
			companies.POST("/:id/groups", pkgAuth.RequirePermission(permissionProvider, "company:manage_members"), groupHandler.CreateGroup)
			companies.GET("/:id/groups", pkgAuth.RequirePermission(permissionProvider, "company:read_members"), groupHandler.ListGroups)
			companies.GET("/:id/groups/:group_id", pkgAuth.RequirePermission(permissionProvider, "company:read_members"), groupHandler.GetGroup)
			companies.PUT("/:id/groups/:group_id", pkgAuth.RequirePermission(permissionProvider, "company:manage_members"), groupHandler.UpdateGroup)
			companies.DELETE("/:id/groups/:group_id", pkgAuth.RequirePermission(permissionProvider, "company:manage_members"), groupHandler.DeleteGroup)
			companies.POST("/:id/groups/:group_id/members", pkgAuth.RequirePermission(permissionProvider, "company:manage_members"), groupHandler.AddMember)
			companies.DELETE("/:id/groups/:group_id/members/:member_id", pkgAuth.RequirePermission(permissionProvider, "company:manage_members"), groupHandler.RemoveMember)
			companies.POST("/:id/groups/:group_id/members/:member_id/move", pkgAuth.RequirePermission(permissionProvider, "company:manage_members"), groupHandler.MoveMember)
		}

		settings := v1.Group("/settings")
		settings.Use(pkgAuth.InternalOrMiddleware(pkgAuth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}))
		{
			settings.PUT("/profile", settingsHandler.UpdateProfile)
			settings.POST("/profile/avatar", settingsHandler.UploadAvatar)
			settings.POST("/change-password", settingsHandler.ChangePassword)
			settings.GET("/preferences", settingsHandler.GetPreferences)
			// PUT continua aceitando merge (semantics legacy). PATCH é
			// o canônico — o handler é idêntico (UpdatePreferences já
			// faz merge via PatchPreferences). Ambos apontam para o
			// mesmo método. Issue #478.
			settings.PUT("/preferences", settingsHandler.UpdatePreferences)
			settings.PATCH("/preferences", settingsHandler.UpdatePreferences)
			settings.GET("/approval-policy", pkgAuth.RequirePermission(permissionProvider, "settings:manage_general"), approvalPolicyHandler.GetCurrent)
			settings.PUT("/approval-policy", pkgAuth.RequirePermission(permissionProvider, "settings:manage_general"), approvalPolicyHandler.UpdateCurrent)
		}

		customers := v1.Group("/customers")
		customers.Use(pkgAuth.InternalOrMiddleware(pkgAuth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL}), pkgAuth.RejectFunctionalRole("cliente"))
		{
			customers.GET("/:accountId/summary", customerHandler.GetSummary)
		}
	}

	return r
}
