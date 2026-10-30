package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func RedisCache(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rdb == nil || c.Request.Method != http.MethodGet {
			c.Next()
			return
		}

		if !strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Next()
			return
		}

		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/ai/") {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			authHeader = c.Query("token")
		}
		// Mutable authenticated state must reach its owning service on each
		// read: a token-specific cache cannot observe another actor's write or
		// re-evaluate authorization. Internal-key responses must never seed a
		// public cache entry either.
		if authHeader != "" || c.GetHeader("X-Internal-Key") != "" {
			c.Header("Cache-Control", "private, no-store")
			c.Next()
			return
		}

		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/auth") {
			c.Next()
			return
		}

		tenantHeader := c.GetHeader("X-Tenant-Id")
		rawKey := c.Request.URL.Path + "?" + c.Request.URL.RawQuery + "|" + tenantHeader + "|" + authHeader
		hash := sha256.Sum256([]byte(rawKey))
		cacheKey := "gateway:cache:" + hex.EncodeToString(hash[:])

		ctx := context.Background()
		cachedResp, err := rdb.Get(ctx, cacheKey).Result()
		if err == nil && cachedResp != "" {
			c.Header("X-Cache", "HIT")
			c.Header("Content-Type", "application/json")
			c.String(http.StatusOK, cachedResp)
			c.Abort()
			return
		}

		w := &responseBodyWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = w

		c.Next()

		if c.Writer.Status() == http.StatusOK {
			rdb.Set(ctx, cacheKey, w.body.String(), 10*time.Second)
		}
	}
}
