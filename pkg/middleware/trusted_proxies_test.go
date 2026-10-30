package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// A16 — a política de proxies confiáveis não pode ser "confie em todos": sem
// configuração o gin honra X-Forwarded-For de qualquer cliente.
func TestResolveTrustedProxyPolicy(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		release     bool
		wantTrust   bool
		wantProxie  []string
		wantIgnored int
		wantInvalid int
	}{
		{name: "vazio em release é falha fechada", raw: "", release: true, wantTrust: true},
		{name: "vazio em dev é falha fechada", raw: "   ", release: false, wantTrust: true},
		{name: "lista declarada", raw: "172.16.0.0/12, 10.0.0.1", release: true, wantProxie: []string{"172.16.0.0/12", "10.0.0.1"}},
		{name: "curinga recusado em release", raw: "0.0.0.0/0", release: true, wantTrust: true, wantIgnored: 1},
		{name: "curinga mantido em dev", raw: "0.0.0.0/0", release: false, wantProxie: []string{"0.0.0.0/0"}},
		{name: "curinga recusado e proxy válido mantido", raw: "::/0,172.16.0.0/12", release: true, wantProxie: []string{"172.16.0.0/12"}, wantIgnored: 1},
		{name: "entrada inválida descartada", raw: "nao-e-ip,172.16.0.0/12", release: true, wantProxie: []string{"172.16.0.0/12"}, wantInvalid: 1},
		{name: "só inválidas vira falha fechada", raw: "toca-gateway", release: true, wantTrust: true, wantInvalid: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := ResolveTrustedProxyPolicy(tt.raw, tt.release)
			if policy.TrustNone != tt.wantTrust {
				t.Fatalf("TrustNone = %v, want %v", policy.TrustNone, tt.wantTrust)
			}
			if len(policy.Proxies) != len(tt.wantProxie) {
				t.Fatalf("Proxies = %v, want %v", policy.Proxies, tt.wantProxie)
			}
			for i, want := range tt.wantProxie {
				if policy.Proxies[i] != want {
					t.Fatalf("Proxies[%d] = %q, want %q", i, policy.Proxies[i], want)
				}
			}
			if len(policy.WildcardsIgnored) != tt.wantIgnored {
				t.Fatalf("WildcardsIgnored = %v, want %d", policy.WildcardsIgnored, tt.wantIgnored)
			}
			if len(policy.Invalid) != tt.wantInvalid {
				t.Fatalf("Invalid = %v, want %d", policy.Invalid, tt.wantInvalid)
			}
		})
	}
}

// O comportamento que o achado A16 descreve: sem proxies confiáveis, o
// cabeçalho do cliente é ignorado e o IP resolvido é o par TCP.
func TestApplyTrustedProxiesIgnoresClientSuppliedForwardedFor(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_MODE", "release")

	engine := clientIPEngine(t)
	ApplyTrustedProxies(engine)

	if got := clientIP(t, engine, "198.51.100.9:4321", "1.2.3.4, 5.6.7.8"); got != "198.51.100.9" {
		t.Fatalf("ClientIP = %q, want o par TCP 198.51.100.9 (cabeçalho do cliente deve ser ignorado)", got)
	}
}

// Com o proxy de entrada declarado, o X-Forwarded-For que ELE acrescenta volta a
// valer (é assim que a borda enxerga o cliente atrás do Traefik/ALB).
func TestApplyTrustedProxiesHonoursConfiguredProxy(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "192.0.2.0/24")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_MODE", "release")

	engine := clientIPEngine(t)
	policy := ApplyTrustedProxies(engine)
	if policy.TrustNone {
		t.Fatalf("política não deveria ser falha fechada: %+v", policy)
	}

	if got := clientIP(t, engine, "192.0.2.10:4321", "203.0.113.7"); got != "203.0.113.7" {
		t.Fatalf("ClientIP = %q, want 203.0.113.7 (cliente atrás do proxy confiável)", got)
	}
	// O par TCP fora da lista continua sem poder forjar identidade.
	if got := clientIP(t, engine, "198.51.100.9:4321", "203.0.113.7"); got != "198.51.100.9" {
		t.Fatalf("ClientIP = %q, want 198.51.100.9 (par não confiável)", got)
	}
}

// Curinga em release não pode restaurar o comportamento antigo.
func TestApplyTrustedProxiesRejectsWildcardInRelease(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "0.0.0.0/0")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_MODE", "release")

	engine := clientIPEngine(t)
	if policy := ApplyTrustedProxies(engine); !policy.TrustNone {
		t.Fatalf("curinga em release deveria degradar para falha fechada: %+v", policy)
	}

	if got := clientIP(t, engine, "198.51.100.9:4321", "1.2.3.4"); got != "198.51.100.9" {
		t.Fatalf("ClientIP = %q, want 198.51.100.9", got)
	}
}

func clientIPEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
	return engine
}

func clientIP(t *testing.T, engine *gin.Engine, remoteAddr, forwardedFor string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = remoteAddr
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w.Body.String()
}
