package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/metrics"
)

// RegisterInfra monta os endpoints de infraestrutura da engine PÚBLICA.
//
// SECURITY (achado B1 da revisão de permissões de 2026-09-19): `/metrics` NÃO é
// registrado aqui. Ele era servido na mesma engine que a borda publica, e em
// beta o router do Traefik para `/metrics` ficava sem os middlewares de
// basic-auth/IP-allowlist do stack de produção — as métricas respondiam 200
// para qualquer um. O scrape acontece no listener interno
// (RegisterMetrics / GATEWAY_METRICS_PORT), que não é publicado pela borda.
func RegisterInfra(r *gin.Engine) {
	r.Any("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "gateway",
		})
	})
}

// RegisterMetrics monta o endpoint de scrape na engine interna de métricas.
func RegisterMetrics(r *gin.Engine) {
	r.GET("/metrics", metrics.Handler())
}
