package handlers

import (
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	gwmiddleware "github.com/oginoino/toca/gateway/internal/middleware"
	"github.com/oginoino/toca/gateway/internal/services/proxy"

	"github.com/gin-gonic/gin"
)

func RegisterAI(r *gin.Engine, cfg gwconfig.Config, pxy *proxy.Service) {
	access := gwmiddleware.RequirePlanFeature(cfg.FinancialServiceURL, "intelligence", cfg.PlanAccessEnabled, false)
	mcpAccess := gwmiddleware.RequirePlanFeature(cfg.FinancialServiceURL, "mcp", cfg.PlanAccessEnabled, false)
	r.Any("/api/v1/ai/chat", access, pxy.StreamingReverseProxy(cfg.AIServiceURL))
	r.Any("/api/v1/ai/conversations", access, pxy.ReverseProxy(cfg.AIServiceURL))
	r.Any("/api/v1/ai/conversations/*path", access, pxy.ReverseProxy(cfg.AIServiceURL))
	r.Any("/api/v1/ai/suggestions", access, pxy.ReverseProxy(cfg.AIServiceURL))
	r.Any("/api/v1/ai/health", pxy.ReverseProxy(cfg.AIServiceURL))
	r.Any("/api/v1/ai/tools", mcpAccess, pxy.ReverseProxy(cfg.AIServiceURL))
}
