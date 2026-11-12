package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type UserStore struct{ db *sqlx.DB }

func NewUserStore(db *sqlx.DB) *UserStore { return &UserStore{db: db} }

// Create inserts the instance's user. The schema allows only one row in users,
// so a second call fails.
func (s *UserStore) Create(ctx context.Context, email, passwordHash, displayName string) (*models.User, error) {
	var u models.User
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ($1, $2, $3)
		RETURNING *`,
		email, passwordHash, displayName,
	).StructScan(&u)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &u, nil
}

// Exists reports whether the instance's user has been created yet.
func (s *UserStore) Exists(ctx context.Context) (bool, error) {
	var exists bool
	if err := s.db.GetContext(ctx, &exists, `SELECT EXISTS (SELECT 1 FROM users)`); err != nil {
		return false, fmt.Errorf("check user exists: %w", err)
	}
	return exists, nil
}

func (s *UserStore) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := s.db.QueryRowxContext(ctx, `SELECT * FROM users WHERE email = $1`, email).StructScan(&u)
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &u, nil
}

func (s *UserStore) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var u models.User
	err := s.db.QueryRowxContext(ctx, `SELECT * FROM users WHERE id = $1`, id).StructScan(&u)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

// SessionState reports whether the user exists, and the time before which
// their sign-in tokens are no longer valid (nil if none have been revoked). The
// auth middleware calls it on every request so changing the password ends
// other sessions immediately instead of when their 30-day token expires.
func (s *UserStore) SessionState(ctx context.Context, id uuid.UUID) (active bool, validAfter *time.Time, err error) {
	var row struct {
		SessionsValidAfter *time.Time `db:"sessions_valid_after"`
	}
	err = s.db.GetContext(ctx, &row,
		`SELECT sessions_valid_after FROM users WHERE id = $1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("check user session: %w", err)
	}
	return true, row.SessionsValidAfter, nil
}

// MarkOnboarded records that the user finished (or skipped) the welcome screen.
func (s *UserStore) MarkOnboarded(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET onboarded_at = NOW() WHERE id = $1 AND onboarded_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("mark onboarded: %w", err)
	}
	return nil
}

// DismissGuide adds a page guide ID to the user's dismissed list (idempotent).
func (s *UserStore) DismissGuide(ctx context.Context, id uuid.UUID, guide string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET dismissed_guides = array_append(dismissed_guides, $2)
		WHERE id = $1 AND NOT ($2 = ANY(dismissed_guides))`,
		id, guide)
	if err != nil {
		return fmt.Errorf("dismiss guide: %w", err)
	}
	return nil
}

func (s *UserStore) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = $1, sessions_valid_after = NOW(), updated_at = NOW() WHERE id = $2`,
		passwordHash, id,
	)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// ResetPassword sets a new password for the instance's user, clears any login
// lockout and signs out every session. It returns the account's email, or
// sql.ErrNoRows when setup hasn't created the account yet.
func (s *UserStore) ResetPassword(ctx context.Context, passwordHash string) (string, error) {
	var email string
	err := s.db.GetContext(ctx, &email, `
		UPDATE users SET
			password_hash = $1,
			sessions_valid_after = NOW(),
			failed_login_attempts = 0,
			locked_until = NULL,
			updated_at = NOW()
		RETURNING email`,
		passwordHash,
	)
	if err != nil {
		return "", fmt.Errorf("reset password: %w", err)
	}
	return email, nil
}

// IncrementFailedAttempts increments the failed login counter. When the counter
// reaches maxAttempts, locked_until is set to now + lockDuration.
func (s *UserStore) IncrementFailedAttempts(ctx context.Context, id uuid.UUID, maxAttempts int, lockDuration time.Duration) error {
	lockUntil := time.Now().Add(lockDuration)
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET
			failed_login_attempts = failed_login_attempts + 1,
			locked_until = CASE
				WHEN failed_login_attempts + 1 >= $2 THEN $3
				ELSE locked_until
			END,
			updated_at = NOW()
		WHERE id = $1`,
		id, maxAttempts, lockUntil,
	)
	if err != nil {
		return fmt.Errorf("increment failed attempts: %w", err)
	}
	return nil
}

// ResetFailedAttempts clears the lockout state on successful login.
func (s *UserStore) ResetFailedAttempts(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET failed_login_attempts = 0, locked_until = NULL, updated_at = NOW() WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("reset failed attempts: %w", err)
	}
	return nil
}
