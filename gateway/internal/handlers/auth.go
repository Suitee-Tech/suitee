package handlers

import (
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	"github.com/oginoino/toca/gateway/internal/services/proxy"

	"github.com/gin-gonic/gin"
)

func RegisterAuth(r *gin.Engine, cfg gwconfig.Config, pxy *proxy.Service) {
	r.Any("/api/v1/auth", pxy.ReverseProxy(cfg.AuthServiceURL))
	r.Any("/api/v1/auth/*path", pxy.ReverseProxy(cfg.AuthServiceURL))
	r.Any("/api/v1/companies", pxy.ReverseProxy(cfg.AuthServiceURL))
	r.Any("/api/v1/companies/*path", pxy.ReverseProxy(cfg.AuthServiceURL))
	r.Any("/api/v1/settings", pxy.ReverseProxy(cfg.AuthServiceURL))
	r.Any("/api/v1/settings/*path", pxy.ReverseProxy(cfg.AuthServiceURL))
	r.Any("/api/v1/lgpd", pxy.ReverseProxy(cfg.AuthServiceURL))
	r.Any("/api/v1/lgpd/*path", pxy.ReverseProxy(cfg.AuthServiceURL))
	r.Any("/api/v1/audit-trail", pxy.ReverseProxy(cfg.AuthServiceURL))
	r.Any("/api/v1/audit-trail/*path", pxy.ReverseProxy(cfg.AuthServiceURL))
}
