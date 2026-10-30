package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/response"
	services "github.com/oginoino/toca/toca-deal/internal/services"
)

// respondPortalError centraliza a tradução de erro das rotas `/client-portal/*`.
//
// Antes cada handler fazia `response.Forbidden(c, err.Error())`: QUALQUER erro
// virava "acesso negado" e o texto interno do serviço de destino ia para o
// navegador. O incidente de 2026-09-19 (tela Financeiro do portal respondendo
// 403) foi exatamente isso — o toca-financial devolvia 400 por falta de
// `tenant_id` na chamada interna e a cliente lia "deal not found or access
// denied".
//
// Regra: acesso negado é 403 (mensagem estática, sem detalhe interno); falha de
// serviço de destino é 503; o resto é erro interno já sanitizado pelo
// `response.InternalError` (M17).
func respondPortalError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrPortalAccessDenied),
		errors.Is(err, services.ErrPortalTicketAccessDenied):
		response.Forbidden(c, err.Error())
	case errors.Is(err, services.ErrPortalUpstreamUnavailable):
		response.Error(c, http.StatusServiceUnavailable, "upstream service unavailable")
	default:
		response.InternalError(c, err)
	}
}
