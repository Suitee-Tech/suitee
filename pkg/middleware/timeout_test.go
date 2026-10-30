package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequestTimeout_WaitsForHandlerToStopBeforeWritingTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var handlerFinished atomic.Bool
	router := gin.New()
	router.Use(RequestTimeout(5 * time.Millisecond))
	router.GET("/slow", func(c *gin.Context) {
		time.Sleep(25 * time.Millisecond)
		handlerFinished.Store(true)
	})

	started := time.Now()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/slow", nil))

	if !handlerFinished.Load() {
		t.Fatal("timeout middleware returned while the handler was still using gin.Context")
	}
	if elapsed := time.Since(started); elapsed < 20*time.Millisecond {
		t.Fatalf("timeout middleware returned before handler stopped: %s", elapsed)
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status: want %d, got %d", http.StatusGatewayTimeout, recorder.Code)
	}
}

func TestRequestTimeoutWithSkip_LeavesLongLivedRouteUnbounded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestTimeoutWithSkip(5*time.Millisecond, func(c *gin.Context) bool {
		return c.Request.URL.Path == "/stream"
	}))
	router.GET("/stream", func(c *gin.Context) {
		select {
		case <-time.After(20 * time.Millisecond):
			c.Status(http.StatusNoContent)
		case <-c.Request.Context().Done():
			c.Status(http.StatusGatewayTimeout)
		}
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/stream", nil))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status: want %d, got %d", http.StatusNoContent, recorder.Code)
	}
}
