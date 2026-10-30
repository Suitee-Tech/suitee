package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	"github.com/oginoino/toca/gateway/internal/services/proxy"
)

// B1 — a engine que a borda publica não pode servir `/metrics`. Antes do P2 a
// rota estava registrada nela (`handlers.RegisterInfra`) e o router do Traefik
// para `/metrics` chegava sem credencial em beta: `curl http://host/metrics`
// devolvia 200 com todas as métricas.
func TestPublicEngineDoesNotServeMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := buildPublicEngine(testConfig(), proxy.New(), nil)

	if code := statusOf(t, engine, http.MethodGet, "/metrics"); code != http.StatusNotFound {
		t.Fatalf("GET /metrics na engine pública = %d, want 404 (scrape é no listener interno)", code)
	}
	if code := statusOf(t, engine, http.MethodGet, "/health"); code != http.StatusOK {
		t.Fatalf("GET /health na engine pública = %d, want 200", code)
	}
}

// O listener interno continua servindo o scrape (o Prometheus aponta para ele).
func TestMetricsEngineServesMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := buildMetricsEngine()

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /metrics no listener interno = %d, want 200", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "http_requests_total") {
		t.Fatalf("corpo do scrape não contém as métricas esperadas: %.120s", body)
	}
	// O listener interno é só de métricas: nada de API nele.
	if code := statusOf(t, engine, http.MethodGet, "/api/v1/auth/me"); code != http.StatusNotFound {
		t.Fatalf("GET /api/v1/auth/me no listener interno = %d, want 404", code)
	}
}

// A16 — a engine pública resolve a identidade pela política de proxies
// confiáveis: sem TRUSTED_PROXIES o X-Forwarded-For do cliente é ignorado.
func TestPublicEngineIgnoresUntrustedForwardedFor(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_MODE", "release")

	gin.SetMode(gin.TestMode)
	engine := buildPublicEngine(testConfig(), proxy.New(), nil)
	engine.GET("/ip-probe", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/ip-probe", nil)
	req.RemoteAddr = "198.51.100.77:1234"
	req.Header.Set("X-Forwarded-For", "10.9.9.9")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if got := recorder.Body.String(); got != "198.51.100.77" {
		t.Fatalf("ClientIP = %q, want o par TCP (cabeçalho do cliente ignorado)", got)
	}
}

func TestMetricsPortFromEnv(t *testing.T) {
	t.Setenv("GATEWAY_METRICS_PORT", "")
	if got := metricsPortFromEnv(); got != defaultMetricsPort {
		t.Fatalf("porta padrão = %q, want %q", got, defaultMetricsPort)
	}
	t.Setenv("GATEWAY_METRICS_PORT", "0")
	if got := metricsPortFromEnv(); got != "0" {
		t.Fatalf("porta desligada = %q, want \"0\"", got)
	}
	t.Setenv("GATEWAY_METRICS_PORT", "9199")
	if got := metricsPortFromEnv(); got != "9199" {
		t.Fatalf("porta configurada = %q, want 9199", got)
	}
}

func statusOf(t *testing.T, engine *gin.Engine, method, path string) int {
	t.Helper()
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder.Code
}

func testConfig() gwconfig.Config {
	return gwconfig.Config{
		AppEnv:          "development",
		AppMode:         "debug",
		FrontendURL:     "http://localhost:5173",
		FrontendCORSURL: "http://localhost:5173",
	}
}

// Regressão de 30/09/2026 — "marcar cobrança como paga" falhava com
// "Request header field idempotency-key is not allowed by Access-Control-Allow-
// Headers in preflight response". O axios manda `Idempotency-Key` (grafia
// canônica) e a lista de CORS só tinha `X-Idempotency-Key`; o backend lê as
// duas. O browser bloqueia a requisição no preflight, então o erro não aparece
// em log nenhum do servidor.
func TestPublicEngineAllowsIdempotencyKeyInPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := buildPublicEngine(testConfig(), proxy.New(), nil)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/charges/7e0b667e-652b-4777-85eb-8e57c4b9b26b/reconcile-manual", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "idempotency-key,content-type,authorization")
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent && recorder.Code != http.StatusOK {
		t.Fatalf("preflight = %d, want 204/200; body=%s", recorder.Code, recorder.Body.String())
	}
	allow := strings.ToLower(recorder.Header().Get("Access-Control-Allow-Headers"))
	if !strings.Contains(allow, "idempotency-key") {
		t.Fatalf("Access-Control-Allow-Headers = %q, quero idempotency-key (o browser bloqueia a requisição antes de sair)", allow)
	}
}
