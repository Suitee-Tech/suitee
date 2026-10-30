package middleware

import (
	"net/url"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/config"
)

// SetupCORS returns a gin middleware that applies the project's standard
// CORS posture from a config.CORSConfig.
//
// The wildcard nuance: gin-contrib/cors refuses to combine AllowOrigins
// and AllowAllOrigins. So when origins is exactly ["*"] we flip the
// AllowAllOrigins flag and clear AllowOrigins — every caller used to
// reimplement this check, so it lives here once.
//
// MaxAge defaults to 12h (the longest preflight cache value browsers
// universally honor). pkg/config validates the wildcard+credentials
// combo at boot, so this factory trusts its input.
func SetupCORS(cfg config.CORSConfig) gin.HandlerFunc {
	c := cors.DefaultConfig()
	c.AllowOrigins = cfg.AllowedOrigins
	c.AllowMethods = cfg.AllowedMethods
	c.AllowHeaders = append(append([]string(nil), cfg.AllowedHeaders...), RequestIDHeader)
	c.ExposeHeaders = []string{RequestIDHeader}
	c.AllowCredentials = cfg.AllowCredentials
	c.MaxAge = 12 * time.Hour

	if len(cfg.AllowedOrigins) == 1 && cfg.AllowedOrigins[0] == "*" {
		c.AllowAllOrigins = true
		c.AllowOrigins = nil
	} else if HasWildcardOrigin(cfg.AllowedOrigins) {
		// Zona pública multi-tenant (ex.: https://*.imoveis.suitee.com.br):
		// a origem muda por imobiliária, então a comparação exata do
		// gin-contrib/cors não serve. AllowOriginFunc é a única forma de
		// aceitar um padrão sem abrir mão de credentials (aqui desligadas).
		allowed := append([]string(nil), cfg.AllowedOrigins...)
		c.AllowOriginFunc = func(origin string) bool { return OriginAllowed(origin, allowed) }
		c.AllowOrigins = nil
	}

	return cors.New(c)
}

// HasWildcardOrigin informa se a lista tem um padrão de subdomínio (`*`).
func HasWildcardOrigin(origins []string) bool {
	for _, o := range origins {
		if strings.Contains(o, "*") {
			return true
		}
	}
	return false
}

// OriginAllowed compara a origem do browser com uma allowlist que pode conter
// exatos e padrões de subdomínio (`https://*.imoveis.suitee.com.br`).
//
// O padrão é ancorado no scheme E no domínio: `*.imoveis.suitee.com.br` aceita
// `a.imoveis.suitee.com.br` e `a.b.imoveis.suitee.com.br`, mas NÃO aceita
// `imoveis.suitee.com.br` (sem subdomínio), `evil-imoveis.suitee.com.br` nem
// `a.imoveis.suitee.com.br.evil.com`.
func OriginAllowed(origin string, allowed []string) bool {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	if origin == "" {
		return false
	}
	for _, pattern := range allowed {
		pattern = strings.TrimRight(strings.TrimSpace(pattern), "/")
		if pattern == "" {
			continue
		}
		if pattern == "*" || pattern == origin {
			return true
		}
		if !strings.Contains(pattern, "*") {
			continue
		}
		if wildcardOriginMatch(origin, pattern) {
			return true
		}
	}
	return false
}

func wildcardOriginMatch(origin, pattern string) bool {
	ou, err := url.Parse(origin)
	if err != nil || ou.Scheme == "" || ou.Host == "" {
		return false
	}
	pu, err := url.Parse(pattern)
	if err != nil || pu.Scheme == "" || pu.Host == "" {
		return false
	}
	if !strings.EqualFold(ou.Scheme, pu.Scheme) {
		return false
	}
	host := strings.ToLower(ou.Hostname())
	phost := strings.ToLower(pu.Hostname())
	parts := strings.SplitN(phost, "*", 2)
	if len(parts) != 2 {
		return false
	}
	prefix, suffix := parts[0], parts[1]
	if !strings.HasPrefix(host, prefix) || !strings.HasSuffix(host, suffix) {
		return false
	}
	// O `*` precisa casar pelo menos um caractere: o apex do domínio não é
	// uma vitrine válida.
	return len(host) > len(prefix)+len(suffix)
}
