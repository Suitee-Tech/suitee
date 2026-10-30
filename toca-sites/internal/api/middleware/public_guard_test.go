package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func guardEngine(perMinute float64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	guard := NewPublicGuard("https", "imoveis.suitee.com.br", "https://app.example.com", perMinute)
	r := gin.New()
	r.POST("/views", guard.Middleware(), func(c *gin.Context) {
		c.JSON(http.StatusAccepted, gin.H{"ok": true})
	})
	return r
}

func doPOST(r *gin.Engine, body, origin, host string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/views", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if host != "" {
		req.Header.Set("X-Site-Host", host)
	}
	req.RemoteAddr = "203.0.113.9:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestPublicGuardRequiresJSONContentType(t *testing.T) {
	r := guardEngine(100)
	req := httptest.NewRequest(http.MethodPost, "/views", strings.NewReader("a=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "203.0.113.9:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestPublicGuardAllowsTenantOriginOnly(t *testing.T) {
	r := guardEngine(100)
	if w := doPOST(r, `{}`, "https://Exemplo.imoveis.suitee.com.br", "Exemplo.imoveis.suitee.com.br"); w.Code != http.StatusAccepted {
		t.Fatalf("origem da zona pública deveria passar, status=%d", w.Code)
	}
	if w := doPOST(r, `{}`, "https://evil.example", "Exemplo.imoveis.suitee.com.br"); w.Code != http.StatusForbidden {
		t.Fatalf("origem estranha deveria ser 403, status=%d", w.Code)
	}
	if w := doPOST(r, `{}`, "https://app.example.com", "Exemplo.imoveis.suitee.com.br"); w.Code != http.StatusAccepted {
		t.Fatalf("origem extra configurada deveria passar, status=%d", w.Code)
	}
}

func TestPublicGuardRateLimitsPerHostAndIP(t *testing.T) {
	r := guardEngine(2)
	for i := 0; i < 2; i++ {
		if w := doPOST(r, `{}`, "", "Exemplo.imoveis.suitee.com.br"); w.Code != http.StatusAccepted {
			t.Fatalf("request %d status=%d", i, w.Code)
		}
	}
	w := doPOST(r, `{}`, "", "Exemplo.imoveis.suitee.com.br")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("terceira request deveria ser 429, veio %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("429 precisa de Retry-After")
	}
}
