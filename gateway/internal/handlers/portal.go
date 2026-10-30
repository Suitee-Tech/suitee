package handlers

import (
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	"github.com/oginoino/toca/gateway/internal/services/proxy"

	"github.com/gin-gonic/gin"
)

func RegisterPortal(r *gin.Engine, cfg gwconfig.Config, pxy *proxy.Service) {
	r.Any("/api/v1/portals", pxy.ReverseProxy(cfg.PortalServiceURL))
	r.Any("/api/v1/portals/*path", pxy.ReverseProxy(cfg.PortalServiceURL))
	r.Any("/api/v1/integrations", pxy.ReverseProxy(cfg.PortalServiceURL))
	r.Any("/api/v1/integrations/*path", pxy.ReverseProxy(cfg.PortalServiceURL))
	r.Any("/api/v1/portal-stats", pxy.ReverseProxy(cfg.PortalServiceURL))
	r.Any("/api/v1/portal-stats/*path", pxy.ReverseProxy(cfg.PortalServiceURL))
}
