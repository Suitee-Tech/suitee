package middleware

import (
	"github.com/gin-gonic/gin"
)

// SecurityHeaders attaches the standard OWASP-recommended response
// headers to every reply. Safe defaults out of the box; callers can
// override via SecurityHeadersConfig when a single service needs to
// relax a policy (e.g. an embed-friendly portal).
//
// Notes:
//   - HSTS is only emitted when the connection is HTTPS (direct TLS
//     or X-Forwarded-Proto from the proxy). Emitting it over HTTP
//     would bake the wrong policy into caches.
//   - Content-Security-Policy defaults to a moderate policy
//     ("default-src 'self'; img-src 'self' data: https:; ..."), which
//     is safe for the SaaS dashboard but breaks if the frontend pulls
//     scripts from a CDN. Override via Config.CSP when needed.
func SecurityHeaders() gin.HandlerFunc {
	return SecurityHeadersWith(SecurityHeadersConfig{})
}

type SecurityHeadersConfig struct {
	// CSP overrides the default Content-Security-Policy. Empty means
	// "use the default" (see code). Pass "off" to disable CSP entirely
	// (not recommended outside ad-hoc debugging).
	CSP string
	// FrameOption controls X-Frame-Options. Empty = "DENY". Set to
	// "SAMEORIGIN" or "" if the service needs to be embeddable.
	FrameOption string
}

func SecurityHeadersWith(cfg SecurityHeadersConfig) gin.HandlerFunc {
	csp := cfg.CSP
	if csp == "" {
		csp = "default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self' https:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
	}
	frame := cfg.FrameOption
	if frame == "" {
		frame = "DENY"
	}
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", frame)
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		if csp != "off" {
			c.Header("Content-Security-Policy", csp)
		}
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}
