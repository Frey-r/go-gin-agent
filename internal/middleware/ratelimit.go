package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter provides per-key token bucket rate limiting.
type RateLimiter struct {
	limiters sync.Map
	rpm      int
}

// NewRateLimiter creates a rate limiter with the given requests-per-minute limit.
func NewRateLimiter(rpm int) *RateLimiter {
	return &RateLimiter{rpm: rpm}
}

// getLimiter returns (or creates) a rate limiter for the given key.
func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	if v, ok := rl.limiters.Load(key); ok {
		return v.(*rate.Limiter)
	}
	// tokens per second = rpm / 60; burst = rpm (allow short bursts)
	limiter := rate.NewLimiter(rate.Limit(float64(rl.rpm)/60.0), rl.rpm)
	actual, _ := rl.limiters.LoadOrStore(key, limiter)
	return actual.(*rate.Limiter)
}

// ByTenant returns a middleware that rate-limits by tenant_id (from JWT auth context).
// Must be placed AFTER the Auth middleware in the chain.
func (rl *RateLimiter) ByTenant() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.GetString("tenant_id")
		if tenantID == "" {
			tenantID = "anonymous"
		}

		limiter := rl.getLimiter("tenant:" + tenantID)
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate limit exceeded",
				"retry_after": "60s",
			})
			return
		}
		c.Next()
	}
}

// ByIP returns a middleware that rate-limits by client IP address.
// Useful for public endpoints like login/register.
func (rl *RateLimiter) ByIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		limiter := rl.getLimiter("ip:" + ip)
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "too many requests",
				"retry_after": "60s",
			})
			return
		}
		c.Next()
	}
}
