package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/logger"
	"golang.org/x/time/rate"
)

type client struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type IPRateLimiter struct {
	clients map[string]*client
	mu      sync.RWMutex
	r       rate.Limit
	b       int
}

func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	i := &IPRateLimiter{
		clients: make(map[string]*client),
		r:       r,
		b:       b,
	}

	go i.cleanup()

	return i
}

func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	c, exists := i.clients[ip]
	if !exists {
		limiter := rate.NewLimiter(i.r, i.b)
		i.clients[ip] = &client{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}

	c.lastSeen = time.Now()
	return c.limiter
}

func (i *IPRateLimiter) cleanup() {
	for {
		time.Sleep(time.Minute)

		i.mu.Lock()
		for ip, client := range i.clients {
			if time.Since(client.lastSeen) > 3*time.Minute {
				delete(i.clients, ip)
			}
		}
		i.mu.Unlock()
	}
}

func RateLimitMiddleware(rps float64, burst int) gin.HandlerFunc {
	limiter := NewIPRateLimiter(rate.Limit(rps), burst)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !limiter.GetLimiter(ip).Allow() {
			logger.Warn("Rate limit exceeded", "ip", ip)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "Too Many Requests",
				"message": "You have exceeded the rate limit. Please try again later.",
			})
			return
		}
		c.Next()
	}
}
