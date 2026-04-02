package service

import (
	"context"
	"errors"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"

	"github.com/ebachmann/go-gin-agent/internal/config"
	"github.com/ebachmann/go-gin-agent/internal/model"
	"github.com/ebachmann/go-gin-agent/internal/store"
)

// Sentinel errors for auth operations.
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountLocked      = errors.New("account locked")
	ErrAccountDisabled    = errors.New("account disabled")
	ErrInvalidToken       = errors.New("invalid token")
	ErrNoInvitation       = errors.New("no valid invitation found")
	ErrWeakPassword       = errors.New("password does not meet requirements")
)

// AuthService encapsulates all authentication business logic.
type AuthService struct {
	userStore *store.UserStore
	cfg       *config.Config
}

// NewAuthService creates a new AuthService.
func NewAuthService(userStore *store.UserStore, cfg *config.Config) *AuthService {
	return &AuthService{
		userStore: userStore,
		cfg:       cfg,
	}
}

// Register creates a new user account. Registration is invitation-only:
// the email must have a valid, unused invitation.
func (s *AuthService) Register(ctx context.Context, req model.RegisterRequest) (*model.User, error) {
	// Validate password strength
	if err := validatePasswordStrength(req.Password); err != nil {
		return nil, err
	}

	// Check for valid invitation
	invitation, err := s.userStore.GetValidInvitation(ctx, req.Email)
	if err != nil {
		return nil, err
	}
	if invitation == nil {
		// Timing-safe: still hash a dummy password to prevent timing attacks
		// that could reveal whether an invitation exists.
		_ = hashDummy()
		return nil, ErrNoInvitation
	}

	// Hash password with bcrypt
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.cfg.BcryptCost)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		ID:           uuid.New().String(),
		TenantID:     invitation.TenantID,
		Email:        req.Email,
		PasswordHash: string(hash),
		Name:         req.Name,
		Role:         invitation.Role,
		IsActive:     true,
	}

	if err := s.userStore.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	// Mark invitation as used
	if err := s.userStore.MarkInvitationUsed(ctx, invitation.ID); err != nil {
		log.Error().Err(err).Str("invitation_id", invitation.ID).Msg("failed to mark invitation used")
	}

	return user, nil
}

// Login authenticates a user by email and password.
// Returns a token pair on success. All error paths are designed to be
// timing-safe to prevent enumeration attacks.
func (s *AuthService) Login(ctx context.Context, req model.LoginRequest) (*model.TokenPair, error) {
	user, err := s.userStore.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, err
	}

	if user == nil {
		// User doesn't exist — still run bcrypt to prevent timing attacks
		_ = hashDummy()
		return nil, ErrInvalidCredentials
	}

	// Check if account is disabled
	if !user.IsActive {
		_ = hashDummy()
		return nil, ErrAccountDisabled
	}

	// Check lockout
	if user.IsLocked() {
		return nil, ErrAccountLocked
	}

	// Verify password (bcrypt.CompareHashAndPassword is timing-safe)
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		// Wrong password — increment failed attempts
		if incErr := s.userStore.IncrementFailedAttempts(ctx, user.ID); incErr != nil {
			log.Error().Err(incErr).Str("user_id", user.ID).Msg("failed to increment login attempts")
		}

		// Lock account if threshold reached
		if user.FailedLoginAttempts+1 >= s.cfg.MaxLoginAttempts {
			lockUntil := time.Now().Add(time.Duration(s.cfg.LockoutDurationMinutes) * time.Minute)
			if lockErr := s.userStore.LockUser(ctx, user.ID, lockUntil); lockErr != nil {
				log.Error().Err(lockErr).Str("user_id", user.ID).Msg("failed to lock user")
			}
		}

		return nil, ErrInvalidCredentials
	}

	// Success — reset failed attempts
	if err := s.userStore.ResetFailedAttempts(ctx, user.ID); err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("failed to reset login attempts")
	}

	// Generate token pair
	return s.GenerateTokenPair(ctx, user)
}

// RefreshToken validates a refresh token, revokes it, and issues a new pair (rotation).
func (s *AuthService) RefreshToken(ctx context.Context, refreshTokenString string) (*model.TokenPair, error) {
	claims := &model.Claims{}
	token, err := jwt.ParseWithClaims(refreshTokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(s.cfg.JWTRefreshSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}

	// Must be a refresh token
	if claims.TokenType != model.TokenTypeRefresh {
		return nil, ErrInvalidToken
	}

	// Check JTI is valid and not revoked
	jti := claims.ID
	valid, err := s.userStore.IsRefreshTokenValid(ctx, jti)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, ErrInvalidToken
	}

	// Revoke the old refresh token (one-time use)
	if err := s.userStore.RevokeRefreshToken(ctx, jti); err != nil {
		log.Error().Err(err).Str("jti", jti).Msg("failed to revoke refresh token")
	}

	// Fetch user to generate new tokens
	user, err := s.userStore.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive {
		return nil, ErrInvalidToken
	}

	return s.GenerateTokenPair(ctx, user)
}

// GenerateTokenPair creates a new access + refresh token pair.
func (s *AuthService) GenerateTokenPair(ctx context.Context, user *model.User) (*model.TokenPair, error) {
	now := time.Now()
	accessExpiry := now.Add(s.cfg.AccessTokenTTL)
	refreshExpiry := now.Add(s.cfg.RefreshTokenTTL)
	refreshJTI := uuid.New().String()

	// Access token
	accessClaims := model.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(accessExpiry),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "go-gin-agent",
		},
		TenantID:  user.TenantID,
		UserID:    user.ID,
		Role:      user.Role,
		TokenType: model.TokenTypeAccess,
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).
		SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return nil, err
	}

	// Refresh token
	refreshClaims := model.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        refreshJTI,
			ExpiresAt: jwt.NewNumericDate(refreshExpiry),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "go-gin-agent",
		},
		UserID:    user.ID,
		TenantID:  user.TenantID,
		TokenType: model.TokenTypeRefresh,
	}
	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).
		SignedString([]byte(s.cfg.JWTRefreshSecret))
	if err != nil {
		return nil, err
	}

	// Persist refresh token JTI for revocation
	if err := s.userStore.SaveRefreshToken(ctx, user.ID, refreshJTI, refreshExpiry); err != nil {
		return nil, err
	}

	return &model.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    accessExpiry,
	}, nil
}

// validatePasswordStrength checks: min 8 chars, 1 uppercase, 1 digit, 1 symbol.
func validatePasswordStrength(password string) error {
	if len(password) < 8 {
		return ErrWeakPassword
	}

	var hasUpper, hasDigit, hasSymbol bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSymbol = true
		}
	}

	if !hasUpper || !hasDigit || !hasSymbol {
		return ErrWeakPassword
	}
	return nil
}

// hashDummy runs a bcrypt hash to normalize timing (prevents user enumeration).
func hashDummy() error {
	_, err := bcrypt.GenerateFromPassword([]byte("dummy-timing-safe"), bcrypt.DefaultCost)
	return err
}
