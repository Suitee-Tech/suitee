package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/config"
)

// Setup with the minimum config — no CORS, no security headers —
// returns the always-on middlewares: RequestID, Recovery, Logger,
// RequestTimeout. The exact count is part of the contract so a future
// refactor that drops or duplicates a middleware fails this test.
func TestSetup_DefaultStackLength(t *testing.T) {
	stack := Setup(StackConfig{})
	if want, got := 4, len(stack); want != got {
		t.Fatalf("default stack length: want %d, got %d", want, got)
	}
}

// Adding CORS bumps the stack by one.
func TestSetup_WithCORS(t *testing.T) {
	stack := Setup(StackConfig{
		CORS: &config.CORSConfig{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET"},
		},
	})
	if want, got := 5, len(stack); want != got {
		t.Fatalf("stack with CORS: want %d, got %d", want, got)
	}
}

// Adding SecurityHeaders also bumps the stack by one. Both together → 6.
func TestSetup_WithCORSAndSecurityHeaders(t *testing.T) {
	stack := Setup(StackConfig{
		CORS: &config.CORSConfig{
			AllowedOrigins: []string{"*"},
		},
		SecurityHeaders: true,
	})
	if want, got := 6, len(stack); want != got {
		t.Fatalf("stack with CORS + security headers: want %d, got %d", want, got)
	}
}

// Smoke test: a router built with the default stack must serve a
// normal request and echo the X-Request-ID header (proving RequestID
// is in the chain and runs before the handler).
func TestSetup_RequestIDEchoed(t *testing.T) {
	r := gin.New()
	r.Use(Setup(StackConfig{})...)
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "test-fixed-id")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: want %d, got %d", http.StatusOK, w.Code)
	}
	if got := w.Header().Get(RequestIDHeader); got != "test-fixed-id" {
		t.Fatalf("X-Request-ID: want %q, got %q", "test-fixed-id", got)
	}
}

// A panic inside a handler must surface as 500 even when wrapped by
// the full Setup stack — proves Recovery is ordered ahead of the
// handler (and not, say, swallowed by a downstream middleware).
func TestSetup_RecoversPanic(t *testing.T) {
	r := gin.New()
	r.Use(Setup(StackConfig{})...)
	r.GET("/boom", func(c *gin.Context) { panic("kaboom") })

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status after panic: want %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

// RequestTimeout=0 must fall back to DefaultRequestTimeout (30s) rather
// than zero-duration which would immediately cancel every request.
func TestSetup_DefaultTimeoutIsNotZero(t *testing.T) {
	r := gin.New()
	r.Use(Setup(StackConfig{RequestTimeout: 0})...)
	r.GET("/slow", func(c *gin.Context) {
		// A 50ms sleep would never complete if zero meant "instant cancel".
		time.Sleep(50 * time.Millisecond)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: want %d, got %d (zero-timeout fallback broken?)", http.StatusOK, w.Code)
	}
}
