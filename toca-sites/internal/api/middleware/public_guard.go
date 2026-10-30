// Package middleware contém as guardas específicas da borda pública do site.
// A autenticação de painel continua em pkg/auth; aqui só vive o que é único da
// vitrine: rate-limit por Host e checagem de origem.
package middleware

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// PublicGuard aplica:
//   - Content-Type application/json nos POST públicos (form cross-site não
//     consegue mandar JSON simples sem preflight);
//   - Origin permitido quando presente (a zona pública é
//     `{marca}.{suffix}`; a API pode estar em outro host, então a lista de
//     origens é configurada);
//   - body limit de 64 KiB (lead/mensagem não precisa de mais);
//   - rate limit por Host+IP.
type PublicGuard struct {
	scheme      string
	suffix      string
	extraOrigin string
	limiter     *tokenBucketLimiter
	maxBody     int64
}

func NewPublicGuard(scheme, suffix, extraOrigin string, perMinute float64) *PublicGuard {
	if perMinute <= 0 {
		perMinute = 120
	}
	return &PublicGuard{
		scheme:      strings.TrimSpace(scheme),
		suffix:      strings.Trim(strings.TrimSpace(suffix), "."),
		extraOrigin: strings.TrimRight(strings.TrimSpace(extraOrigin), "/"),
		limiter:     newTokenBucketLimiter(perMinute),
		maxBody:     64 << 10,
	}
}

func (g *PublicGuard) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodPost {
			ct := strings.ToLower(c.GetHeader("Content-Type"))
			if !strings.HasPrefix(ct, "application/json") {
				c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "content-type must be application/json"})
				c.Abort()
				return
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, g.maxBody)
		}

		if origin := strings.TrimSpace(c.GetHeader("Origin")); origin != "" && !g.originAllowed(origin) {
			c.JSON(http.StatusForbidden, gin.H{"error": "origin not allowed"})
			c.Abort()
			return
		}

		host := publicHost(c)
		key := host + "|" + c.ClientIP()
		if !g.limiter.allow(key) {
			c.Header("Retry-After", strconv.Itoa(60))
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (g *PublicGuard) originAllowed(origin string) bool {
	if g.extraOrigin != "" && origin == g.extraOrigin {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	scheme := g.scheme
	if scheme == "" {
		scheme = "https"
	}
	if u.Scheme != scheme {
		return false
	}
	host := strings.ToLower(u.Hostname())
	suffix := strings.ToLower(g.suffix)
	if suffix == "" || !strings.HasSuffix(host, "."+suffix) {
		return false
	}
	slug := strings.TrimSuffix(host, "."+suffix)
	return slug != "" && !strings.Contains(slug, ".")
}

// publicHost prefere o header encaminhado pelo Next (server-side) ao Host da
// conexão, que atrás do proxy é sempre o do serviço.
func publicHost(c *gin.Context) string {
	if h := strings.TrimSpace(c.GetHeader("X-Site-Host")); h != "" {
		return strings.ToLower(h)
	}
	return strings.ToLower(c.Request.Host)
}

// tokenBucketLimiter é um token bucket em processo, com limpeza preguiçosa.
// O limite real de borda continua sendo o do gateway; este protege o Postgres
// e o CRM de rajadas por host/IP mesmo quando a chamada vem da rede interna.
type tokenBucketLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	capacity float64
	refill   float64
	lastGC   time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newTokenBucketLimiter(perMinute float64) *tokenBucketLimiter {
	return &tokenBucketLimiter{
		buckets:  make(map[string]*bucket),
		capacity: perMinute,
		refill:   perMinute / 60.0,
		lastGC:   time.Now(),
	}
}

func (l *tokenBucketLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if now.Sub(l.lastGC) > 10*time.Minute {
		for k, b := range l.buckets {
			if now.Sub(b.last) > 10*time.Minute {
				delete(l.buckets, k)
			}
		}
		l.lastGC = now
	}

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.capacity, last: now}
		l.buckets[key] = b
	}
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens += elapsed * l.refill
		if b.tokens > l.capacity {
			b.tokens = l.capacity
		}
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}
