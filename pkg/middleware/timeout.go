package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/logger"
)

// DefaultRequestTimeout caps a single inbound HTTP request at 30 seconds
// when no explicit value is provided. Picked deliberately above the
// frontend axios client's 20s ceiling so the client always sees an
// explicit error before this kicks in. Services with heavier endpoints
// (PDF generation, batch jobs) can override via RequestTimeout(custom).
const DefaultRequestTimeout = 30 * time.Second

// RequestTimeout wraps each request's context.Context with a deadline. When
// the deadline expires the context cancels — downstream SQL queries and HTTP
// clients that honor ctx.Done() abort — and the handler can return promptly.
// If the handler returns without having written a response, the middleware
// emits HTTP 504 with the request_id so operators can correlate the timeout
// with logs.
//
// Pass 0 (or a negative duration) to fall back to DefaultRequestTimeout.
//
// Why this exists: a single hung downstream (slow upstream API, Postgres
// row lock, stuck RabbitMQ publish) otherwise holds a pool connection forever.
// Bounded contexts let cooperative dependencies stop their work. Handlers run
// synchronously because gin.Context and its ResponseWriter are not safe for
// concurrent use; racing a timeout response against c.Next() can crash the
// whole process with "concurrent map writes".
func RequestTimeout(timeout time.Duration) gin.HandlerFunc {
	return requestTimeout(timeout, nil)
}

// RequestTimeoutWithSkip applies RequestTimeout unless shouldSkip returns true.
// It is intended for explicitly long-lived routes such as SSE and WebSocket
// endpoints, which must be governed by connection lifecycle rather than a
// normal request deadline.
func RequestTimeoutWithSkip(timeout time.Duration, shouldSkip func(*gin.Context) bool) gin.HandlerFunc {
	return requestTimeout(timeout, shouldSkip)
}

func requestTimeout(timeout time.Duration, shouldSkip func(*gin.Context) bool) gin.HandlerFunc {
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	return func(c *gin.Context) {
		if shouldSkip != nil && shouldSkip(c) {
			c.Next()
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		if ctx.Err() == context.DeadlineExceeded {
			reqID := pkgctx.GetRequestID(ctx)
			logger.WarnContext(ctx, "request timeout",
				"timeout", timeout.String(),
				"path", c.Request.URL.Path,
				"method", c.Request.Method,
				"request_id", reqID,
			)
			if !c.Writer.Written() {
				c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{
					"error":      "request timeout",
					"timeout":    timeout.String(),
					"request_id": reqID,
				})
			}
		}
	}
}
