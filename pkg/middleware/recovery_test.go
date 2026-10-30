package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// SetupRecovery must convert a handler panic into a 500 with a JSON
// envelope, not let the connection drop. The error string in the body
// is intentionally generic — the panic detail goes to the structured
// log via pkg/logger, not to the client.
func TestSetupRecovery_PanicReturns500(t *testing.T) {
	r := gin.New()
	r.Use(SetupRecovery())
	r.GET("/boom", func(c *gin.Context) {
		panic("simulated explosion")
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status: want %d, got %d", http.StatusInternalServerError, w.Code)
	}
	if !strings.Contains(w.Body.String(), "Internal Server Error") {
		t.Fatalf("body should contain generic error message; got %q", w.Body.String())
	}
}

// When the panic happens after the handler has started writing, we
// can't replace the response — confirm we don't double-write headers
// (gin would log a warning and the test would still pass at the HTTP
// level, but the assertion guards the intent).
func TestSetupRecovery_PanicAfterWriteDoesNotDoubleWrite(t *testing.T) {
	r := gin.New()
	r.Use(SetupRecovery())
	r.GET("/partial", func(c *gin.Context) {
		c.String(http.StatusOK, "partial response")
		c.Writer.Flush()
		panic("after write")
	})

	req := httptest.NewRequest(http.MethodGet, "/partial", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Status was already 200 — recovery must NOT overwrite to 500.
	if w.Code != http.StatusOK {
		t.Fatalf("status after partial write: want %d, got %d", http.StatusOK, w.Code)
	}
}
