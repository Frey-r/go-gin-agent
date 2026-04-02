package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// Security adds hardened HTTP response headers and panic recovery.
// Equivalent to helmet.js for Node servers.
func Security() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent MIME type sniffing
		c.Header("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")

		// XSS protection (legacy browsers)
		c.Header("X-XSS-Protection", "1; mode=block")

		// Force HTTPS (1 year, include subdomains)
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")

		// Control referrer information
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Restrict resource loading sources
		c.Header("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")

		// Disable browser features that could be abused
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")

		// Prevent caching of sensitive responses
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")

		c.Next()
	}
}

// Recovery catches panics and returns a generic 500 error without leaking
// internal details. The stack trace is logged server-side only.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				requestID := c.GetString(RequestIDKey)
				log.Error().
					Str("request_id", requestID).
					Interface("panic", err).
					Str("path", c.Request.URL.Path).
					Msg("panic recovered")

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error":      "internal server error",
					"request_id": requestID,
				})
			}
		}()
		c.Next()
	}
}
