package handlers

import (
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	gwmiddleware "github.com/oginoino/toca/gateway/internal/middleware"
	"github.com/oginoino/toca/gateway/internal/services/proxy"
	"strings"

	"github.com/gin-gonic/gin"
)

func RegisterCRM(r *gin.Engine, cfg gwconfig.Config, pxy *proxy.Service) {
	r.Any("/api/v1/leads", pxy.ReverseProxy(cfg.CRMServiceURL))
	leadsProxy := pxy.ReverseProxy(cfg.CRMServiceURL)
	portalInviteAccess := gwmiddleware.RequirePlanFeature(cfg.FinancialServiceURL, "client_portal", cfg.PlanAccessEnabled, false)
	r.Any("/api/v1/leads/*path", func(c *gin.Context) {
		if c.Request.Method == "POST" && strings.HasSuffix(c.Param("path"), "/invite") {
			portalInviteAccess(c)
			if c.IsAborted() {
				return
			}
		}
		leadsProxy(c)
	})
	r.Any("/api/v1/brokers", pxy.ReverseProxy(cfg.CRMServiceURL))
	r.Any("/api/v1/brokers/*path", pxy.ReverseProxy(cfg.CRMServiceURL))
	r.Any("/api/v1/customers", pxy.ReverseProxy(cfg.CRMServiceURL))
	r.Any("/api/v1/customers/*path", pxy.ReverseProxy(cfg.CRMServiceURL))
	r.Any("/api/v1/exports", pxy.ReverseProxy(cfg.CRMServiceURL))
	r.Any("/api/v1/exports/*path", pxy.ReverseProxy(cfg.CRMServiceURL))
}
