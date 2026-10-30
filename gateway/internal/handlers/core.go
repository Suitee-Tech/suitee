package handlers

import (
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	"github.com/oginoino/toca/gateway/internal/services/proxy"

	"github.com/gin-gonic/gin"
)

func RegisterCore(r *gin.Engine, cfg gwconfig.Config, pxy *proxy.Service) {
	r.Any("/api/v1/properties", pxy.ReverseProxy(cfg.CoreServiceURL))
	r.Any("/api/v1/properties/*path", pxy.ReverseProxy(cfg.CoreServiceURL))
	r.Any("/api/v1/captacao", pxy.ReverseProxy(cfg.CoreServiceURL))
	r.Any("/api/v1/captacao/*path", pxy.ReverseProxy(cfg.CoreServiceURL))
	r.Any("/api/v1/partnerships", pxy.ReverseProxy(cfg.CoreServiceURL))
	// The /partnerships subtree (lifecycle + shared/properties) is owned by
	// toca-core, EXCEPT the aggregated shared-leads endpoint
	// (/partnerships/shared/leads), which is owned by toca-crm. gin won't let
	// a static route coexist with this catch-all wildcard, so dispatch by path
	// here rather than registering a second, conflicting route.
	corePartnerships := pxy.ReverseProxy(cfg.CoreServiceURL)
	crmSharedLeads := pxy.ReverseProxy(cfg.CRMServiceURL)
	r.Any("/api/v1/partnerships/*path", func(c *gin.Context) {
		if c.Param("path") == "/shared/leads" {
			crmSharedLeads(c)
			return
		}
		corePartnerships(c)
	})
	r.Any("/api/v1/company-settings", pxy.ReverseProxy(cfg.CoreServiceURL))
	r.Any("/api/v1/company-settings/*path", pxy.ReverseProxy(cfg.CoreServiceURL))
	r.Any("/api/v1/me/permissions", pxy.ReverseProxy(cfg.CoreServiceURL))
	r.Any("/api/v1/me/permissions/*path", pxy.ReverseProxy(cfg.CoreServiceURL))
	r.Any("/api/v1/health", pxy.ReverseProxy(cfg.CoreServiceURL))
}
