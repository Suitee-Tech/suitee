package handlers

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	pkgauth "github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/pkg/validation"
)

func SetupRouter(cfg config.BaseConfig, chatHandler *ChatHandler, convHandler *ConversationHandler, suggestionsHandler *SuggestionsHandler, extractionHandler *ExtractionHandler) *gin.Engine {
	if err := pkgauth.InitJWKS(cfg.JWT.SupabaseJWKSURL); err != nil {
		logger.Error("Failed to initialize JWKS", map[string]interface{}{"error": err.Error()})
	}

	// P1/A2: registra no boot se o serviço está no modelo antigo (chave
	// compartilhada, sem allowlist de tenant) — em produção isso é alarme.
	pkgauth.LogServiceKeyRollout()

	_ = validation.InitGin()

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	appEnv := os.Getenv("APP_ENV")
	appMode := os.Getenv("APP_MODE")
	// P2/M16: critério único de release (pkg/config), sem cópias divergentes.
	isRelease := config.IsReleaseEnv(appEnv, appMode)
	allowedOrigins := os.Getenv("AI_ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		allowedOrigins = os.Getenv("CORS_ALLOWED_ORIGINS")
	}
	if allowedOrigins == "" {
		allowedOrigins = os.Getenv("ALLOWED_ORIGINS")
	}
	if allowedOrigins == "" {
		allowedOrigins = os.Getenv("FRONTEND_URL")
	}

	parsedOrigins := make([]string, 0)
	for _, o := range strings.Split(allowedOrigins, ",") {
		o = strings.TrimSpace(o)
		o = strings.TrimRight(o, "/")
		if o != "" {
			parsedOrigins = append(parsedOrigins, o)
		}
	}
	if !isRelease && len(parsedOrigins) == 0 {
		parsedOrigins = []string{
			"http://localhost:5173",
			"http://127.0.0.1:5173",
			"http://localhost:3000",
			"http://127.0.0.1:3000",
		}
	}
	if isRelease {
		if len(parsedOrigins) == 0 {
			logger.Error("CORS origins must be configured via AI_ALLOWED_ORIGINS/CORS_ALLOWED_ORIGINS/ALLOWED_ORIGINS/FRONTEND_URL in release mode")
			os.Exit(1)
		}
		for _, o := range parsedOrigins {
			if o == "*" {
				logger.Error("CORS wildcard origin is not allowed in release mode")
				os.Exit(1)
			}
		}
	}

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = parsedOrigins
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization", "X-Tenant-Id", "X-Debug"}
	corsConfig.AllowCredentials = true
	allowedSet := map[string]struct{}{}
	for _, o := range parsedOrigins {
		o = strings.TrimSpace(strings.TrimRight(o, "/"))
		if o != "" {
			allowedSet[o] = struct{}{}
		}
	}
	corsConfig.AllowOriginFunc = func(origin string) bool {
		origin = strings.TrimSpace(strings.TrimRight(origin, "/"))
		if origin == "" {
			return false
		}
		if strings.HasPrefix(origin, "http://localhost:") ||
			strings.HasPrefix(origin, "http://127.0.0.1:") ||
			strings.HasPrefix(origin, "https://localhost:") ||
			strings.HasPrefix(origin, "https://127.0.0.1:") ||
			origin == "http://localhost" ||
			origin == "http://127.0.0.1" ||
			origin == "https://localhost" ||
			origin == "https://127.0.0.1" {
			return true
		}
		_, ok := allowedSet[origin]
		return ok
	}
	corsMW := cors.New(corsConfig)
	r.Use(func(c *gin.Context) {
		if c.GetHeader("X-Forwarded-Host") != "" || c.GetHeader("X-Forwarded-For") != "" {
			c.Next()
			return
		}
		corsMW(c)
	})

	r.Use(metrics.GinMiddleware("toca-ai"))
	r.GET("/metrics", metrics.Handler())

	r.Any("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "toca-ai"})
	})

	v1 := r.Group("/api/v1")
	authMiddleware := pkgauth.Middleware(pkgauth.Config{SupabaseJWKSURL: cfg.JWT.SupabaseJWKSURL})
	v1.Use(authMiddleware, pkgauth.RejectFunctionalRole("cliente"))

	// P1/A8: as rotas do assistente não tinham NENHUMA verificação de permissão —
	// qualquer autenticado não-`cliente` conversava com o agente e listava/apagava
	// conversas. `ai:use` nomeia a capacidade (o default espelha a população que já
	// tem o painel no layout, então ninguém perde acesso) e passa a ser revogável
	// por tenant. A autorização POR TOOL continua pendente: cada tool do agente
	// toca um domínio (leads, deals, financeiro) e mereceria a permissão daquele
	// domínio — registrado em docs/reviews/evidence/p1/A8-ai-portal.md.
	permissionProvider := pkgauth.NewCorePermissionProvider(clients.NewCoreClient())
	requireAIUse := pkgauth.RequirePermission(permissionProvider, "ai:use")

	v1.GET("/ai/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "toca-ai"})
	})
	v1.GET("/ai/tools", requireAIUse, chatHandler.Tools)
	v1.POST("/ai/chat", requireAIUse, chatHandler.Chat)
	v1.GET("/ai/suggestions", requireAIUse, suggestionsHandler.Suggestions)

	ai := v1.Group("/ai")
	{
		ai.GET("/conversations", requireAIUse, convHandler.List)
		ai.DELETE("/conversations", requireAIUse, convHandler.Clear)
		ai.DELETE("/conversations/:id", requireAIUse, convHandler.Delete)
		ai.GET("/conversations/:id/messages", requireAIUse, convHandler.ListMessages)
	}

	// Rotas internas (serviço-a-serviço), autenticadas por X-Internal-Key em vez
	// de JWT — chamadas pelo toca-deal, não pelo browser. Por isso ficam fora do
	// grupo v1 (que exige JWT + RejectFunctionalRole).
	internal := r.Group("/api/v1/ai/internal")
	internal.Use(pkgauth.RequireInternalKey())
	internal.POST("/documents/extract", extractionHandler.Extract)

	return r
}
