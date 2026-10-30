package routes

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/metrics"
	pkgmw "github.com/oginoino/toca/pkg/middleware"
	"github.com/oginoino/toca/pkg/validation"

	"github.com/oginoino/toca/toca-sites/internal/api/handlers"
	"github.com/oginoino/toca/toca-sites/internal/api/middleware"
)

// Deps agrupa as dependências HTTP do serviço.
type Deps struct {
	Cfg           config.BaseConfig
	SiteHandler   *handlers.SiteHandler
	PublicHandler *handlers.PublicHandler
	PublicGuard   *middleware.PublicGuard
	// PublicOriginPattern é a origem da zona pública aceita por CORS
	// (ex.: https://*.imoveis.suitee.com.br).
	PublicOriginPattern string
}

// SetupRouter monta o roteador. As rotas públicas são registradas ANTES do
// grupo autenticado e resolvem tenant só pelo Host; o painel exige JWT +
// permissões `sites:read`/`sites:write`.
func SetupRouter(deps Deps) *gin.Engine {
	if err := auth.InitJWKS(deps.Cfg.JWT.SupabaseJWKSURL); err != nil {
		logger.Error("Failed to initialize JWKS", map[string]interface{}{"error": err.Error()})
	}
	auth.LogServiceKeyRollout()
	_ = validation.InitGin()

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())
	r.Use(metrics.GinMiddleware("toca-sites"))

	r.GET("/metrics", metrics.Handler())

	// CORS: a zona pública tem origem por tenant, então a lista aceita o
	// padrão com wildcard (`https://*.imoveis.suitee.com.br`). O gateway
	// também aplica CORS, mas chamadas server-side/diretas em dev precisam
	// funcionar sem depender do proxy.
	origins := append([]string{}, deps.Cfg.CORS.AllowedOrigins...)
	if pattern := strings.TrimSpace(deps.PublicOriginPattern); pattern != "" {
		origins = append(origins, pattern)
	}
	r.Use(pkgmw.SetupCORS(config.CORSConfig{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   append(append([]string{}, deps.Cfg.CORS.AllowedHeaders...), "X-Site-Host", "X-Request-ID"),
		AllowCredentials: false,
	}))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "toca-sites"})
	})

	// ===== PÚBLICO (sem JWT; tenant resolvido pelo Host) =====
	public := r.Group("/api/v1/sites/public")
	if deps.PublicGuard != nil {
		public.Use(deps.PublicGuard.Middleware())
	}
	{
		public.POST("/views", deps.PublicHandler.CreateView)
		public.POST("/leads", deps.PublicHandler.CreateLead)
	}

	// ===== AUTENTICADO =====
	v1 := r.Group("/api/v1")
	v1.Use(auth.InternalOrMiddleware(auth.Config{SupabaseJWKSURL: deps.Cfg.JWT.SupabaseJWKSURL}))

	// Portal do cliente: só precisa estar autenticado no tenant; a URL
	// devolvida é sempre da própria imobiliária (tenant do JWT). Não exige
	// sites:read porque o cliente final não tem permissões de gestão.
	portal := v1.Group("/sites/portal")
	{
		portal.GET("/listing-url", deps.SiteHandler.PortalListingURL)
		portal.GET("/site", deps.SiteHandler.PortalSite)
	}

	// Painel da imobiliária: rejeita o papel `cliente` e exige a permissão
	// granular. `service` também é rejeitado pelo RequirePermission — rotas
	// internas usariam RequireInternalKey, e aqui não há nenhuma.
	manage := v1.Group("/sites")
	manage.Use(auth.RejectFunctionalRole("cliente"))
	permissionProvider := auth.NewCorePermissionProvider(clients.NewCoreClient())
	requireRead := auth.RequirePermission(permissionProvider, "sites:read")
	requireWrite := auth.RequirePermission(permissionProvider, "sites:write")
	// A URL canônica é informação pública do anúncio e o link aparece no
	// detalhe do imóvel — exigi-la por `sites:read` esconderia o "Ver no site"
	// de quem tem `properties:read` mas não administra a vitrine.
	requirePropertiesRead := auth.RequirePermission(permissionProvider, "properties:read")
	{
		manage.GET("/me", requireRead, deps.SiteHandler.GetMe)
		manage.GET("/me/readiness", requireRead, deps.SiteHandler.Readiness)
		manage.PUT("/me", requireWrite, deps.SiteHandler.SaveMe)
		manage.POST("/me/reconcile", requireWrite, deps.SiteHandler.ReconcileMe)
		manage.GET("/pages", requireRead, deps.SiteHandler.ListPages)
		manage.PUT("/pages/:key", requireWrite, deps.SiteHandler.UpdatePage)
		manage.GET("/listings/:propertyId/url", requirePropertiesRead, deps.SiteHandler.ListingURL)
		manage.GET("/listings/:propertyId/stats", requireRead, deps.SiteHandler.ListingStats)
		manage.POST("/listings/:propertyId/resync", requireWrite, deps.SiteHandler.Resync)
		manage.GET("/stats", requireRead, deps.SiteHandler.Stats)
		manage.GET("/leads", requireRead, deps.SiteHandler.Leads)
	}

	return r
}
