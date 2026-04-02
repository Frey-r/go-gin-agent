package model

import "github.com/golang-jwt/jwt/v5"

// TokenType distinguishes access tokens from refresh tokens in JWT claims.
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// Claims represents the JWT claims embedded in access and refresh tokens.
type Claims struct {
	jwt.RegisteredClaims
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	TokenType TokenType `json:"type"`
}
