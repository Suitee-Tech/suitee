package handlers

import (
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	"github.com/oginoino/toca/gateway/internal/services/proxy"

	"github.com/gin-gonic/gin"
)

func RegisterFrontend(r *gin.Engine, cfg gwconfig.Config, pxy *proxy.Service) {
	r.Any("/portal", pxy.ReverseProxy(cfg.FrontendURL))
	r.Any("/portal/*path", pxy.ReverseProxy(cfg.FrontendURL))
	r.Any("/assets/*path", pxy.ReverseProxy(cfg.FrontendURL))
}
