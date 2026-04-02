package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ebachmann/go-gin-agent/internal/model"
)

// UserStore handles all user-related database operations.
type UserStore struct {
	db *DB
}

// NewUserStore creates a new UserStore.
func NewUserStore(db *DB) *UserStore {
	return &UserStore{db: db}
}

// CreateUser inserts a new user into the database.
func (s *UserStore) CreateUser(ctx context.Context, user *model.User) error {
	query := `INSERT INTO users (id, tenant_id, email, password_hash, name, role, is_active)
			   VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.Conn.ExecContext(ctx, query,
		user.ID, user.TenantID, user.Email, user.PasswordHash, user.Name, user.Role, user.IsActive)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// GetUserByEmail retrieves a user by email address.
// Returns nil, nil if not found (caller must handle both cases identically for timing-safety).
func (s *UserStore) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `SELECT id, tenant_id, email, password_hash, name, role, is_active,
			         failed_login_attempts, locked_until, created_at, updated_at
			  FROM users WHERE email = ?`

	var user model.User
	err := s.db.Conn.QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.TenantID, &user.Email, &user.PasswordHash, &user.Name,
		&user.Role, &user.IsActive, &user.FailedLoginAttempts, &user.LockedUntil,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &user, nil
}

// GetUserByID retrieves a user by their UUID.
func (s *UserStore) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	query := `SELECT id, tenant_id, email, password_hash, name, role, is_active,
			         failed_login_attempts, locked_until, created_at, updated_at
			  FROM users WHERE id = ?`

	var user model.User
	err := s.db.Conn.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.TenantID, &user.Email, &user.PasswordHash, &user.Name,
		&user.Role, &user.IsActive, &user.FailedLoginAttempts, &user.LockedUntil,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &user, nil
}

// IncrementFailedAttempts atomically increments the failed login attempt counter.
func (s *UserStore) IncrementFailedAttempts(ctx context.Context, userID string) error {
	query := `UPDATE users SET failed_login_attempts = failed_login_attempts + 1,
			         updated_at = CURRENT_TIMESTAMP
			  WHERE id = ?`
	_, err := s.db.Conn.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("increment failed attempts: %w", err)
	}
	return nil
}

// ResetFailedAttempts resets the counter and clears the lockout.
func (s *UserStore) ResetFailedAttempts(ctx context.Context, userID string) error {
	query := `UPDATE users SET failed_login_attempts = 0, locked_until = NULL,
			         updated_at = CURRENT_TIMESTAMP
			  WHERE id = ?`
	_, err := s.db.Conn.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("reset failed attempts: %w", err)
	}
	return nil
}

// LockUser locks the user account until a specific time.
func (s *UserStore) LockUser(ctx context.Context, userID string, until time.Time) error {
	query := `UPDATE users SET locked_until = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.Conn.ExecContext(ctx, query, until, userID)
	if err != nil {
		return fmt.Errorf("lock user: %w", err)
	}
	return nil
}

// SaveRefreshToken persists a refresh token's JTI for revocation tracking.
func (s *UserStore) SaveRefreshToken(ctx context.Context, userID, jti string, expiresAt time.Time) error {
	query := `INSERT INTO refresh_tokens (jti, user_id, expires_at) VALUES (?, ?, ?)`
	_, err := s.db.Conn.ExecContext(ctx, query, jti, userID, expiresAt)
	if err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}
	return nil
}

// RevokeRefreshToken marks a refresh token as revoked.
func (s *UserStore) RevokeRefreshToken(ctx context.Context, jti string) error {
	query := `UPDATE refresh_tokens SET revoked = 1 WHERE jti = ?`
	_, err := s.db.Conn.ExecContext(ctx, query, jti)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

// IsRefreshTokenValid checks if a refresh token JTI exists and is not revoked/expired.
func (s *UserStore) IsRefreshTokenValid(ctx context.Context, jti string) (bool, error) {
	query := `SELECT COUNT(1) FROM refresh_tokens
			  WHERE jti = ? AND revoked = 0 AND expires_at > CURRENT_TIMESTAMP`
	var count int
	err := s.db.Conn.QueryRowContext(ctx, query, jti).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check refresh token: %w", err)
	}
	return count > 0, nil
}

// CleanupExpiredTokens removes expired refresh tokens (housekeeping).
func (s *UserStore) CleanupExpiredTokens(ctx context.Context) (int64, error) {
	query := `DELETE FROM refresh_tokens WHERE expires_at < CURRENT_TIMESTAMP`
	result, err := s.db.Conn.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("cleanup tokens: %w", err)
	}
	return result.RowsAffected()
}

// --- Invitation methods ---

// CreateInvitation creates a new invitation for email-based registration.
func (s *UserStore) CreateInvitation(ctx context.Context, id, email, tenantID, role, invitedBy string, expiresAt time.Time) error {
	query := `INSERT INTO invitations (id, email, tenant_id, role, invited_by, expires_at)
			  VALUES (?, ?, ?, ?, ?, ?)`
	_, err := s.db.Conn.ExecContext(ctx, query, id, email, tenantID, role, invitedBy, expiresAt)
	if err != nil {
		return fmt.Errorf("create invitation: %w", err)
	}
	return nil
}

// GetValidInvitation retrieves a valid (unused, not expired) invitation for an email.
func (s *UserStore) GetValidInvitation(ctx context.Context, email string) (*Invitation, error) {
	query := `SELECT id, email, tenant_id, role, invited_by, expires_at
			  FROM invitations
			  WHERE email = ? AND used = 0 AND expires_at > CURRENT_TIMESTAMP
			  ORDER BY created_at DESC LIMIT 1`

	var inv Invitation
	err := s.db.Conn.QueryRowContext(ctx, query, email).Scan(
		&inv.ID, &inv.Email, &inv.TenantID, &inv.Role, &inv.InvitedBy, &inv.ExpiresAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get invitation: %w", err)
	}
	return &inv, nil
}

// MarkInvitationUsed marks an invitation as used.
func (s *UserStore) MarkInvitationUsed(ctx context.Context, invitationID string) error {
	query := `UPDATE invitations SET used = 1 WHERE id = ?`
	_, err := s.db.Conn.ExecContext(ctx, query, invitationID)
	if err != nil {
		return fmt.Errorf("mark invitation used: %w", err)
	}
	return nil
}

// Invitation represents a pending registration invitation.
type Invitation struct {
	ID        string
	Email     string
	TenantID  string
	Role      string
	InvitedBy string
	ExpiresAt time.Time
}
