package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/gin-gonic/gin"
)

const VisitorIDKey = "visitor_id"

func VisitorID() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		if authHeader := c.GetHeader("Authorization"); authHeader != "" {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token != authHeader {
				hash := sha256.Sum256([]byte(token + ip))
				visitorID := hex.EncodeToString(hash[:16])
				c.Set(VisitorIDKey, visitorID)
				c.Next()
				return
			}
		}

		visitorID := generateVisitorID(ip)
		c.Set(VisitorIDKey, visitorID)
		c.Next()
	}
}

func generateVisitorID(ip string) string {
	return "pg_" + ip
}