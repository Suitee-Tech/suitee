package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/config"
)

// StackConfig configures the standard middleware stack returned by
// Setup. Zero-valued fields fall back to safe defaults so most callers
// only need to populate the bits they want to override.
type StackConfig struct {
	// CORS — when nil, no CORS middleware is registered. Internal
	// services typically leave this nil because the gateway terminates
	// CORS. Set to &cfg.CORS for the edge or for services that need
	// their own CORS posture.
	CORS *config.CORSConfig

	// SecurityHeaders — when true, attaches the OWASP-recommended
	// header set (X-Content-Type-Options, X-Frame-Options, CSP, HSTS
	// when TLS is detected, etc.). Default false because intermediate
	// services don't need them; the edge (gateway) sets this to true.
	SecurityHeaders bool

	// SecurityHeadersConfig — overrides for SecurityHeaders (CSP,
	// FrameOption). Only consulted when SecurityHeaders is true.
	SecurityHeadersConfig SecurityHeadersConfig

	// RequestTimeout — per-request deadline. Zero falls back to
	// DefaultRequestTimeout (30s); the timeout middleware is always
	// installed so a hung downstream can't pin a goroutine forever.
	RequestTimeout time.Duration
}

// Setup returns the ordered list of middlewares that every Toca HTTP
// service applies in front of its routes. Callers spread the slice
// into r.Use:
//
//	r := gin.New()
//	r.Use(middleware.Setup(middleware.StackConfig{CORS: &cfg.CORS})...)
//
// Order (each middleware sees requests top-to-bottom; responses
// bottom-to-top):
//
//  1. RequestID         — assigns/echoes X-Request-ID, populates ctx
//  2. Logger            — structured request log line (sees final status
//     because Recovery rewrites the response before this resumes)
//  3. CORS              — only when StackConfig.CORS != nil
//  4. SecurityHeaders   — only when StackConfig.SecurityHeaders is true
//  5. RequestTimeout    — adds a cancellable deadline to the request
//     context while keeping the handler chain in the request goroutine
//  6. Recovery          — converts handler panics to a JSON 500 response
//
// Tracing (pkg/tracing.Middleware) and metrics (pkg/metrics.GinMiddleware)
// are not part of this stack because they depend on the service name;
// callers wire them right after the stack returned here.
func Setup(cfg StackConfig) []gin.HandlerFunc {
	stack := []gin.HandlerFunc{
		RequestID(),
		SetupLogger(),
	}
	if cfg.CORS != nil {
		stack = append(stack, SetupCORS(*cfg.CORS))
	}
	if cfg.SecurityHeaders {
		stack = append(stack, SecurityHeadersWith(cfg.SecurityHeadersConfig))
	}
	stack = append(stack, RequestTimeout(cfg.RequestTimeout))
	stack = append(stack, SetupRecovery())
	return stack
}
