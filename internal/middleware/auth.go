package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/ebachmann/go-gin-agent/internal/model"
)

// Auth returns a middleware that validates JWT access tokens from the
// Authorization header. It extracts tenant_id, user_id, and role into
// the Gin context for downstream handlers.
//
// Security: error messages are intentionally generic to avoid leaking
// information about whether a token is expired vs. malformed vs. missing.
func Auth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			abortUnauthorized(c)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			abortUnauthorized(c)
			return
		}

		tokenString := parts[1]
		claims := &model.Claims{}

		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			// Ensure the signing method is HMAC (prevent algorithm substitution attacks)
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			abortUnauthorized(c)
			return
		}

		// Ensure this is an access token, not a refresh token
		if claims.TokenType != model.TokenTypeAccess {
			abortUnauthorized(c)
			return
		}

		// Inject identity into context
		c.Set("tenant_id", claims.TenantID)
		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)

		c.Next()
	}
}

// abortUnauthorized returns a generic 401 to avoid info leakage.
func abortUnauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": "unauthorized",
	})
}
