package handlers

import (
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	"github.com/oginoino/toca/gateway/internal/services/proxy"

	"github.com/gin-gonic/gin"
)

func RegisterOps(r *gin.Engine, cfg gwconfig.Config, pxy *proxy.Service) {
	r.Any("/api/v1/suppliers", pxy.ReverseProxy(cfg.OpsServiceURL))
	r.Any("/api/v1/suppliers/*path", pxy.ReverseProxy(cfg.OpsServiceURL))
	r.Any("/api/v1/maintenance", pxy.ReverseProxy(cfg.OpsServiceURL))
	r.Any("/api/v1/maintenance/*path", pxy.ReverseProxy(cfg.OpsServiceURL))
	r.Any("/api/v1/tickets", pxy.ReverseProxy(cfg.OpsServiceURL))
	r.Any("/api/v1/tickets/*path", pxy.ReverseProxy(cfg.OpsServiceURL))
	r.Any("/api/v1/keys", pxy.ReverseProxy(cfg.OpsServiceURL))
	r.Any("/api/v1/keys/*path", pxy.ReverseProxy(cfg.OpsServiceURL))
	r.Any("/api/v1/inspections", pxy.ReverseProxy(cfg.OpsServiceURL))
	r.Any("/api/v1/inspections/*path", pxy.ReverseProxy(cfg.OpsServiceURL))
}
