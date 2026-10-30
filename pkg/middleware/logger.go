package middleware

import (
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/logger"
)

// redactedQueryParams são parâmetros que NÃO podem aparecer no log de acesso.
//
// O webhook da AbacatePay chega com `?webhookSecret=<segredo>` (é assim que o
// provedor entrega o segredo combinado), e o log registrava o path completo: o
// segredo ficava em texto claro no Loki/Grafana, em toda entrega de evento. O
// valor continua sendo validado pelo handler — só não é mais escrito.
var redactedQueryParams = []string{"webhooksecret", "secret", "token", "apikey", "api_key", "access_token"}

// safePath devolve o path com os parâmetros sensíveis substituídos por
// "REDACTED", preservando os demais.
func safePath(path, rawQuery string) string {
	if rawQuery == "" {
		return path
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		// Query que não dá para interpretar: registra só o path, nunca o cru.
		return path
	}
	changed := false
	for chave := range values {
		for _, sensivel := range redactedQueryParams {
			if strings.EqualFold(chave, sensivel) {
				values.Set(chave, "REDACTED")
				changed = true
			}
		}
	}
	if !changed {
		return path + "?" + rawQuery
	}
	return path + "?" + values.Encode()
}

// SetupLogger returns a gin middleware that emits one structured log
// line per request via pkg/logger. Fields: status, latency, ip, method,
// path. Private gin.Errors collected during the request are flattened
// into an "error" attribute when present and the line is upgraded to
// Error level.
//
// Uses the *Context logger methods so trace_id / request_id / tenant_id
// / user_id attached upstream (pkg/middleware.RequestID, the auth
// middleware, OTEL spans) flow into every log line automatically via
// pkg/logger's ContextHandler.
func SetupLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		errorMessage := c.Errors.ByType(gin.ErrorTypePrivate).String()

		path = safePath(path, raw)

		fields := []any{
			"status", statusCode,
			"latency", latency,
			"ip", c.ClientIP(),
			"method", c.Request.Method,
			"path", path,
		}

		ctx := c.Request.Context()
		if errorMessage != "" {
			fields = append(fields, "error", errorMessage)
			logger.ErrorContext(ctx, "Request", fields...)
		} else {
			logger.InfoContext(ctx, "Request", fields...)
		}
	}
}
