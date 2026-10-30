package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/metrics"
)

func TestDiagnosticsRateLimitDoesNotUseTenantHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(NewIPRateLimiter().Middleware())
	r.POST("/api/v1/client-errors", func(c *gin.Context) { c.Status(http.StatusAccepted) })
	for i := 0; i < 21; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/client-errors", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		req.Header.Set("X-Tenant-Id", fmt.Sprintf("untrusted-%d", i))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		want := http.StatusAccepted
		if i == 20 {
			want = http.StatusTooManyRequests
		}
		if w.Code != want {
			t.Fatalf("request %d: status %d, want %d", i, w.Code, want)
		}
	}
}

// A16 — o limite de 20/min de /auth/login não pode ser contornado rotacionando
// o cabeçalho `X-Tenant-Id`. Antes do P2 a chave do bucket era `ip + "|" +
// X-Tenant-Id`: vinte e uma tentativas de força bruta com vinte e um tenants
// diferentes passavam todas. Agora todas caem no mesmo bucket do IP.
func TestAuthRateLimitCannotBeRotatedByTenantHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.SetTrustedProxies(nil) // sem proxy confiável: X-Forwarded-For é ignorado
	engine.Use(NewIPRateLimiter().Middleware())
	engine.POST("/api/v1/auth/login", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 21; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "198.51.100.20:5555"
		req.Header.Set("X-Tenant-Id", fmt.Sprintf("tenant-%d", i))
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		want := http.StatusOK
		if i == 20 {
			want = http.StatusTooManyRequests
		}
		if w.Code != want {
			t.Fatalf("tentativa %d (tenant-%d): status %d, want %d", i, i, w.Code, want)
		}
	}
}

// A16 — a identidade também não pode ser rotacionada por `X-Forwarded-For`
// quando a requisição não vem de um proxy confiável: o bucket é o mesmo do par
// TCP, então o 21º login é recusado.
func TestAuthRateLimitCannotBeRotatedByForwardedFor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.SetTrustedProxies(nil)
	engine.Use(NewIPRateLimiter().Middleware())
	engine.POST("/api/v1/auth/login", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 21; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "198.51.100.21:5555"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", i+1))
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		want := http.StatusOK
		if i == 20 {
			want = http.StatusTooManyRequests
		}
		if w.Code != want {
			t.Fatalf("tentativa %d (XFF 203.0.113.%d): status %d, want %d", i, i+1, w.Code, want)
		}
	}
}

// O limite geral (3000/min em produção) não é o cenário do teste acima: aqui
// garantimos que dois IPs distintos continuam com buckets independentes, para
// que o endurecimento não vire um balde global.
func TestRateLimitKeepsPerIdentityBuckets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.SetTrustedProxies(nil)
	engine.Use(NewIPRateLimiter().Middleware())
	engine.POST("/api/v1/auth/login", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 40; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		if i%2 == 0 {
			req.RemoteAddr = "198.51.100.30:5555"
		} else {
			req.RemoteAddr = "198.51.100.31:5555"
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("requisição %d: status %d (buckets deveriam ser por identidade)", i, w.Code)
		}
	}
}

// A métrica de rejeição não pode carregar valor escolhido pelo cliente.
func TestRateLimitRejectionMetricKeyIsNotClientControlled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.SetTrustedProxies(nil)
	engine.Use(NewIPRateLimiter().Middleware())
	engine.POST("/api/v1/auth/login", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 21; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "198.51.100.40:5555"
		req.Header.Set("X-Tenant-Id", "tenant-escolhido-pelo-cliente")
		req.Header.Set("X-Forwarded-For", "meu-bucket-falso")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
	}

	body := scrapeMetrics(t)
	if strings.Contains(body, "tenant-escolhido-pelo-cliente") || strings.Contains(body, "meu-bucket-falso") {
		t.Fatalf("métrica de rate limit expôs valor controlado pelo cliente:\n%s", body)
	}
	if !strings.Contains(body, "key=\"ip:"+identityFingerprint("198.51.100.40")+"\"") {
		t.Fatalf("métrica de rate limit não registrou a identidade derivada:\n%s", body)
	}
}

// scrapeMetrics lê o registry em processo pela rota real do handler.
func scrapeMetrics(t *testing.T) string {
	t.Helper()
	engine := gin.New()
	engine.GET("/metrics", metrics.Handler())
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return w.Body.String()
}

// M14 — as rotas de enumeração/OTP entram no bucket estrito da borda: o limite
// por contato vive no toca-auth, mas a borda precisa limitar o volume por IP
// (o atacante que troca de contato a cada requisição só é contido aqui).
func TestContactProbeRoutesUseStrictAuthBucket(t *testing.T) {
	for _, path := range []string{
		"/api/v1/auth/check-email",
		"/api/v1/auth/check-phone",
		"/api/v1/auth/otp/request",
	} {
		t.Run(path, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			engine := gin.New()
			engine.SetTrustedProxies(nil)
			engine.Use(NewIPRateLimiter().Middleware())
			engine.GET(path, func(c *gin.Context) { c.Status(http.StatusOK) })

			for i := 0; i < 21; i++ {
				req := httptest.NewRequest(http.MethodGet, path+"?email=probe%d@example.com", nil)
				req.RemoteAddr = "198.51.100.90:5555"
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, req)

				want := http.StatusOK
				if i == 20 {
					want = http.StatusTooManyRequests
				}
				if w.Code != want {
					t.Fatalf("%s requisição %d: status %d, want %d", path, i, w.Code, want)
				}
			}
		})
	}
}

// O teto do assistente conta apenas as rotas que CHAMAM O MODELO.
//
// Antes, TODO `/api/v1/ai/*` entrava no bucket estrito de 30 por HORA: abrir o
// painel (que consulta `/ai/tools` e `/ai/conversations`) consumia cota de
// conversa, e uma sessão normal de uso esgotava o bucket em minutos. O 429
// resultante chegava à interface como "não consegui concluir essa solicitação"
// e, com uma resposta parcial na tela, como "resposta interrompida".
func TestAIRateLimitOnlyCountsModelBackedRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.SetTrustedProxies(nil)
	engine.Use(NewIPRateLimiter().Middleware())
	engine.POST("/api/v1/ai/chat", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/api/v1/ai/tools", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/api/v1/ai/conversations", func(c *gin.Context) { c.Status(http.StatusOK) })

	call := func(method, path string) int {
		req := httptest.NewRequest(method, path, nil)
		req.RemoteAddr = "203.0.113.77:4321"
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w.Code
	}

	// Leitura barata não pode consumir o teto do modelo: 200 chamadas passam.
	for i := 0; i < 200; i++ {
		if code := call(http.MethodGet, "/api/v1/ai/tools"); code != http.StatusOK {
			t.Fatalf("chamada %d de /ai/tools: status %d, want 200 (não é rota de modelo)", i, code)
		}
		if code := call(http.MethodGet, "/api/v1/ai/conversations"); code != http.StatusOK {
			t.Fatalf("chamada %d de /ai/conversations: status %d, want 200", i, code)
		}
	}

	// O teto do modelo continua valendo, com a janela cheia disponível.
	for i := 0; i < aiRateLimitPerWindow; i++ {
		if code := call(http.MethodPost, "/api/v1/ai/chat"); code != http.StatusOK {
			t.Fatalf("conversa %d: status %d, want 200 (dentro do teto de %d)", i, code, aiRateLimitPerWindow)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat", nil)
	req.RemoteAddr = "203.0.113.77:4321"
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("conversa além do teto: status %d, want 429", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want 2 (janela de %s para %d tokens)", got, aiRefillWindow, aiRateLimitPerWindow)
	}
	if body := w.Body.String(); !strings.Contains(body, `"retry_after_s":2`) {
		t.Fatalf("corpo sem retry_after_s coerente: %s", body)
	}
}
