package handlers

import (
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	"github.com/oginoino/toca/gateway/internal/services/proxy"

	"github.com/gin-gonic/gin"
)

func RegisterComm(r *gin.Engine, cfg gwconfig.Config, pxy *proxy.Service) {
	r.Any("/api/v1/messages", pxy.ReverseProxy(cfg.CommServiceURL))
	r.Any("/api/v1/messages/*path", pxy.ReverseProxy(cfg.CommServiceURL))
	r.Any("/api/v1/channels", pxy.ReverseProxy(cfg.CommServiceURL))
	r.Any("/api/v1/channels/*path", pxy.ReverseProxy(cfg.CommServiceURL))
	r.Any("/api/v1/templates", pxy.ReverseProxy(cfg.CommServiceURL))
	r.Any("/api/v1/templates/*path", pxy.ReverseProxy(cfg.CommServiceURL))
	r.Any("/api/v1/metrics", pxy.ReverseProxy(cfg.CommServiceURL))
	r.Any("/api/v1/metrics/*path", pxy.ReverseProxy(cfg.CommServiceURL))
	r.Any("/api/v1/notifications", pxy.ReverseProxy(cfg.CommServiceURL))
	r.Any("/api/v1/notifications/*path", pxy.ReverseProxy(cfg.CommServiceURL))
	r.Any("/api/v1/ws", pxy.ReverseProxy(cfg.CommServiceURL))
	r.Any("/uploads/*path", pxy.ReverseProxy(cfg.CommServiceURL))
}
