package model

import "time"

// User represents a registered user in the system.
type User struct {
	ID                  string     `json:"id" db:"id"`
	TenantID            string     `json:"tenant_id" db:"tenant_id"`
	Email               string     `json:"email" db:"email"`
	PasswordHash        string     `json:"-" db:"password_hash"` // never serialized
	Name                string     `json:"name" db:"name"`
	Role                string     `json:"role" db:"role"` // "admin" | "user"
	IsActive            bool       `json:"is_active" db:"is_active"`
	FailedLoginAttempts int        `json:"-" db:"failed_login_attempts"`
	LockedUntil         *time.Time `json:"-" db:"locked_until"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at" db:"updated_at"`
}

// IsLocked returns true if the user account is currently locked out.
func (u *User) IsLocked() bool {
	if u.LockedUntil == nil {
		return false
	}
	return time.Now().Before(*u.LockedUntil)
}

// TokenPair holds an access/refresh token pair returned on successful login.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"` // access token expiry
}

// RegisterRequest is the payload for POST /api/v1/auth/register.
type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,min=8,max=128"`
	Name     string `json:"name" validate:"required,min=2,max=100"`
}

// LoginRequest is the payload for POST /api/v1/auth/login.
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// RefreshRequest is the payload for POST /api/v1/auth/refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}
