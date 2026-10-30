package middleware

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TestRedisCacheBypassesAuthenticatedAndInternalReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{"bearer", "query token", "internal key"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int64
			rdb := redis.NewClient(&redis.Options{Addr: "unused:6379", Dialer: func(context.Context, string, string) (net.Conn, error) {
				calls.Add(1)
				return nil, errors.New("Redis must not be contacted")
			}})
			t.Cleanup(func() { _ = rdb.Close() })
			r := gin.New()
			r.Use(RedisCache(rdb))
			revision := 0
			r.GET("/api/v1/client-portal/overview", func(c *gin.Context) { revision++; c.JSON(200, gin.H{"revision": revision}) })
			for i := 1; i <= 2; i++ {
				path := "/api/v1/client-portal/overview"
				if mode == "query token" {
					path += "?token=test-jwt"
				}
				req := httptest.NewRequest(http.MethodGet, path, nil)
				if mode == "bearer" {
					req.Header.Set("Authorization", "Bearer test-jwt")
				}
				if mode == "internal key" {
					req.Header.Set("X-Internal-Key", "test-internal-key")
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if w.Code != 200 || w.Header().Get("Cache-Control") != "private, no-store" || revision != i {
					t.Fatalf("read not delegated: %d %s", w.Code, w.Body.String())
				}
			}
			if calls.Load() != 0 {
				t.Fatalf("authenticated read contacted Redis %d times", calls.Load())
			}
		})
	}
}
