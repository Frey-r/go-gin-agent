package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Logger returns a Gin middleware that logs every request using zerolog
// with structured JSON output. It includes request_id, tenant_id, method,
// path, status, latency, and client IP.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()

		if raw != "" {
			path = path + "?" + raw
		}

		var event *zerolog.Event
		switch {
		case status >= 500:
			event = log.Error()
		case status >= 400:
			event = log.Warn()
		default:
			event = log.Info()
		}

		event.
			Str("request_id", c.GetString(RequestIDKey)).
			Str("tenant_id", c.GetString("tenant_id")).
			Str("method", c.Request.Method).
			Str("path", path).
			Int("status", status).
			Dur("latency", latency).
			Str("ip", clientIP).
			Int("body_size", c.Writer.Size()).
			Msg("request")
	}
}
