// Package middleware hosts cross-cutting Gin middlewares shared by every
// service (request id, handler timeouts, security headers, etc.). Service
// owners wire them once in their cmd/server/main.go alongside the existing
// auth + metrics middlewares.
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	pkgctx "github.com/oginoino/toca/pkg/context"
)

const (
	// RequestIDHeader is the canonical name. Inbound requests carrying it
	// are trusted (gateway/proxy can set it); outbound responses always
	// echo it so curl / browser devtools can correlate.
	RequestIDHeader = "X-Request-ID"
)

// RequestID returns a Gin middleware that assigns a unique id to every
// inbound request, stores it on both the gin.Context (key "request_id")
// and the request's context.Context (via pkgctx.WithRequestID), and
// echoes it back via the X-Request-ID response header.
//
// Honors an inbound X-Request-ID when present so a request that already
// crossed an edge (gateway, mobile client) keeps the same id end-to-end.
// The ContextHandler in pkg/logger then pulls this id into every log line
// emitted with *Context methods.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}

		c.Set("request_id", id)
		c.Header(RequestIDHeader, id)
		// Keep the correlation id on the request itself as it crosses reverse
		// proxies. Storing it only in context made the gateway response
		// traceable, but downstream services received no X-Request-ID.
		c.Request.Header.Set(RequestIDHeader, id)

		ctx := pkgctx.WithRequestID(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
