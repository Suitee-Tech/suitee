package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/logger"
)

// M17 — erros internos (SQL) devolvidos ao cliente.
//
// A revisão de permissões de 2026-09-19 registrou que caminhos de 500 devolviam
// `err.Error()` direto ao cliente: nome de tabela/coluna, violação de constraint,
// host/porta do banco, mensagem do driver. Isso é vazamento de detalhe de
// implementação — útil só para quem está atacando — e não ajuda o usuário, que
// não tem o que fazer com "pq: duplicate key value violates unique constraint
// \"accounts_email_key\"".
//
// Contrato: `InternalError` responde 500 com mensagem GENÉRICA (e um
// `error_id` estável para correlacionar com o log/observabilidade) enquanto o
// detalhe é registrado no log estruturado com rota, método, request_id e
// tenant. Quem precisa de mensagem específica para o usuário tem que tratar o
// erro antes (validação → 400, autorização → 403, inexistente → 404).
func InternalError(c *gin.Context, err error) {
	detail := ""
	if err != nil {
		detail = err.Error()
	}

	requestID := ""
	tenantID := ""
	path := ""
	method := ""
	if c != nil {
		requestID = c.GetString("request_id")
		tenantID = c.GetString("tenant_id")
		if c.Request != nil {
			path = c.Request.URL.Path
			method = c.Request.Method
			if requestID == "" {
				requestID = c.GetHeader("X-Request-ID")
			}
		}
	}

	logger.Error("http.internal_error",
		"event", "http.internal_error",
		"error", detail,
		"path", path,
		"method", method,
		"request_id", requestID,
		"tenant_id", tenantID,
	)

	Error(c, http.StatusInternalServerError, "Ocorreu um erro interno. Tente novamente ou informe o suporte com o identificador da requisição.")
}
