package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// DismissedSubscriptionStore remembers recurring charges a user has marked as
// not being subscriptions, keyed by the Subscriptions tab's merchant key.
type DismissedSubscriptionStore struct{ db *sqlx.DB }

func NewDismissedSubscriptionStore(db *sqlx.DB) *DismissedSubscriptionStore {
	return &DismissedSubscriptionStore{db: db}
}

// List returns the user's dismissed merchant keys. It is never nil, so it
// serializes as an empty array rather than null.
func (s *DismissedSubscriptionStore) List(ctx context.Context, userID uuid.UUID) ([]string, error) {
	keys := []string{}
	if err := s.db.SelectContext(ctx, &keys,
		`SELECT merchant_key FROM dismissed_subscriptions WHERE user_id = $1 ORDER BY merchant_key`, userID,
	); err != nil {
		return nil, fmt.Errorf("list dismissed subscriptions: %w", err)
	}
	return keys, nil
}

func (s *DismissedSubscriptionStore) Dismiss(ctx context.Context, userID uuid.UUID, key string) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO dismissed_subscriptions (user_id, merchant_key) VALUES ($1, $2)
		ON CONFLICT (user_id, merchant_key) DO NOTHING`, userID, key,
	); err != nil {
		return fmt.Errorf("dismiss subscription: %w", err)
	}
	return nil
}

func (s *DismissedSubscriptionStore) Restore(ctx context.Context, userID uuid.UUID, key string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM dismissed_subscriptions WHERE user_id = $1 AND merchant_key = $2`, userID, key,
	); err != nil {
		return fmt.Errorf("restore subscription: %w", err)
	}
	return nil
}
