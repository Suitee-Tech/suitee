package handlers

import (
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	gwmiddleware "github.com/oginoino/toca/gateway/internal/middleware"
	"github.com/oginoino/toca/gateway/internal/services/proxy"

	"github.com/gin-gonic/gin"
)

func RegisterSignature(r *gin.Engine, cfg gwconfig.Config, pxy *proxy.Service) {
	access := gwmiddleware.RequirePlanFeature(cfg.FinancialServiceURL, "electronic_signature", cfg.PlanAccessEnabled, true)
	r.Any("/api/v1/signature", access, pxy.ReverseProxy(cfg.SignatureServiceURL))
	r.Any("/api/v1/signature/*path", access, pxy.ReverseProxy(cfg.SignatureServiceURL))
	r.Any("/webhooks/clicksign", pxy.ReverseProxy(cfg.SignatureServiceURL))
}
