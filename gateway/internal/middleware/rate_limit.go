package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/metrics"
)

const (
	defaultGeneralRateLimit = 3000
	authRateLimit           = 20
	// aiRateLimit é o teto de chamadas ao MODELO por janela. O painel do
	// assistente é conversacional: abrir a aba consome `/ai/tools` e
	// `/ai/conversations`, cada mensagem consome `/ai/chat` e a lista de
	// sugestões pode ser refeita a cada troca de tela. Com 30 por HORA (o valor
	// anterior) uma sessão normal de uso esgotava o bucket em minutos e o
	// painel passava a responder 429 — que a interface mostrava como "não
	// consegui concluir essa solicitação" / "resposta interrompida".
	aiRateLimit    = 120
	aiRefillWindow = 5 * time.Minute
	refillInterval = time.Minute
)

// generalRateLimit is resolved once at startup so the env var is read
// before any middleware handler runs (avoiding per-request os.Getenv).
var generalRateLimit = resolveGeneralRateLimit()

// aiRateLimitPerWindow permite ajustar o teto do assistente por ambiente
// (custo de modelo varia por provedor/plano) sem recompilar.
var aiRateLimitPerWindow = resolveAiRateLimit()

func resolveAiRateLimit() int {
	if v := strings.TrimSpace(os.Getenv("GATEWAY_AI_RATE_LIMIT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return aiRateLimit
}

func resolveGeneralRateLimit() int {
	if v := strings.TrimSpace(os.Getenv("GATEWAY_RATE_LIMIT_PER_MIN")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultGeneralRateLimit
}

type IPRateLimiter struct {
	mu          sync.Mutex
	visitors    map[string]*visitorEntry
	cleanupOnce sync.Once
}

type visitorEntry struct {
	tokens   int
	lastSeen time.Time
}

func NewIPRateLimiter() *IPRateLimiter {
	return &IPRateLimiter{visitors: make(map[string]*visitorEntry)}
}

func (l *IPRateLimiter) Middleware() gin.HandlerFunc {
	l.cleanupOnce.Do(func() {
		go func() {
			for {
				time.Sleep(5 * time.Minute)
				l.mu.Lock()
				for ip, v := range l.visitors {
					if time.Since(v.lastSeen) > 2*time.Hour {
						delete(l.visitors, ip)
					}
				}
				l.mu.Unlock()
			}
		}()
	})

	return func(c *gin.Context) {
		// SECURITY (achado A16 da revisão de permissões de 2026-09-19): a
		// identidade do bucket é o IP resolvido pela política de proxies
		// confiáveis (pkg/middleware.ApplyTrustedProxies) e NADA mais. Até o P2
		// a chave era `ip + "|" + X-Tenant-Id`, ou seja, dois cabeçalhos do
		// cliente: bastava rotacionar o tenant para ganhar um bucket novo por
		// requisição e derrubar o limite de 20/min de /auth/login.
		ip := c.ClientIP()
		path := c.Request.URL.Path

		if !strings.HasPrefix(path, "/api/") {
			c.Next()
			return
		}

		limit := generalRateLimit
		interval := refillInterval
		endpointGroup := "general"
		// P2/M14: além das rotas de credencial, entram no bucket estrito as
		// consultas de disponibilidade e o pedido de OTP — são as que permitem
		// enumerar contato e gerar custo de envio. O limite por CONTATO vive no
		// toca-auth (OTP_RATE_LIMIT_PER_CONTACT); aqui é a borda por IP.
		if strings.HasPrefix(path, "/api/v1/auth/login") ||
			strings.HasPrefix(path, "/api/v1/auth/register") ||
			strings.HasPrefix(path, "/api/v1/auth/forgot-password") ||
			strings.HasPrefix(path, "/api/v1/auth/check-email") ||
			strings.HasPrefix(path, "/api/v1/auth/check-phone") ||
			strings.HasPrefix(path, "/api/v1/auth/otp/") {
			limit = authRateLimit
			endpointGroup = "auth"
		}
		// O bucket estrito cobre as rotas que CHAMAM O MODELO. As demais rotas
		// do assistente (`/ai/tools`, `/ai/conversations`, `/ai/health`) são
		// leitura barata e ficavam consumindo o mesmo teto — abrir o painel
		// gastava cota de conversa sem nenhuma inferência.
		if path == "/api/v1/ai/chat" || strings.HasPrefix(path, "/api/v1/ai/suggestions") {
			limit = aiRateLimitPerWindow
			interval = aiRefillWindow
			endpointGroup = "ai"
		}

		if path == "/api/v1/client-errors" {
			limit = 20
			endpointGroup = "diagnostics"
		}

		// Chave do bucket: identidade (IP confiável) + grupo do endpoint. Nunca
		// um cabeçalho do cliente.
		key := ip + ":" + endpointGroup

		l.mu.Lock()
		v, exists := l.visitors[key]
		if !exists {
			l.visitors[key] = &visitorEntry{tokens: limit, lastSeen: time.Now()}
			c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
			c.Header("X-RateLimit-Remaining", strconv.Itoa(limit-1))
			l.visitors[key].tokens-- // consume one token for this request
			l.mu.Unlock()
			c.Next()
			return
		}

		// Refill before consuming so newly added tokens are available
		// immediately. This is especially important for concurrent requests
		// arriving within the same sub-interval (e.g. dashboard page load).
		elapsed := time.Since(v.lastSeen)
		refill := int(elapsed / (interval / time.Duration(limit)))
		if refill > 0 {
			v.tokens += refill
			if v.tokens > limit {
				v.tokens = limit
			}
		}
		v.lastSeen = time.Now()

		if v.tokens <= 0 {
			// `Retry-After` é o tempo até o PRÓXIMO token (interval/limit), não
			// um 60 fixo: com janela de uma hora o valor mentia por até ~59min.
			espera := int((interval / time.Duration(limit)).Seconds())
			if espera < 1 {
				espera = 1
			}
			c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("Retry-After", strconv.Itoa(espera))
			l.mu.Unlock()
			// A métrica recebe um identificador derivado (não o IP cru e nunca
			// um valor escolhido pelo cliente): cardinalidade alta em label de
			// métrica é vetor de exaustão de memória no Prometheus.
			metrics.IncRateLimitRejected("ip:"+identityFingerprint(ip), endpointGroup)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":          "rate limit exceeded, please try again later",
				"endpoint_group": endpointGroup,
				"retry_after_s":  espera,
			})
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(v.tokens-1))
		v.tokens--
		l.mu.Unlock()
		c.Next()
	}
}

// identityFingerprint devolve um prefixo estável de SHA-256 do IP para uso como
// label de métrica: correlaciona ocorrências do mesmo cliente sem publicar o IP
// cru e sem deixar o cliente escolher o valor do label.
func identityFingerprint(ip string) string {
	if ip == "" {
		return "unknown"
	}
	sum := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(sum[:4])
}
