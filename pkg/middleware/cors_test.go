package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// SetupCORS must echo the configured origin for non-wildcard configs.
// Regression: if a service accidentally passed a wildcard credentials
// combo, gin-contrib/cors would silently emit "*" — the boot-time
// validator in pkg/config blocks that; this test makes sure we don't
// rely on the library doing the right thing.
func TestSetupCORS_ExplicitOrigin(t *testing.T) {
	r := gin.New()
	r.Use(SetupCORS(config.CORSConfig{
		AllowedOrigins:   []string{"https://app.toca.dev"},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://app.toca.dev")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.toca.dev" {
		t.Fatalf("Access-Control-Allow-Origin: want %q, got %q", "https://app.toca.dev", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Access-Control-Allow-Credentials: want %q, got %q", "true", got)
	}
}

// Wildcard origin must trigger AllowAllOrigins mode. The library
// errors out at setup time if both AllowOrigins=["*"] and any other
// origin config are set together, so the factory handles the swap.
func TestSetupCORS_Wildcard(t *testing.T) {
	r := gin.New()
	r.Use(SetupCORS(config.CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: false,
	}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://random.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("wildcard origin: want %q, got %q", "*", got)
	}
}
