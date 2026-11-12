package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type ReturnRateStore struct{ db *sqlx.DB }

func NewReturnRateStore(db *sqlx.DB) *ReturnRateStore { return &ReturnRateStore{db: db} }

// ListByUser returns all custom return rate overrides for a user.
func (s *ReturnRateStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]models.UserReturnRate, error) {
	var out []models.UserReturnRate
	err := s.db.SelectContext(ctx, &out,
		`SELECT * FROM user_return_rates WHERE user_id = $1 ORDER BY asset_class`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list return rates: %w", err)
	}
	return out, nil
}

// Upsert creates or updates a custom return rate for a user/asset_class pair.
func (s *ReturnRateStore) Upsert(ctx context.Context, userID uuid.UUID, assetClass string, returnRate float64) (*models.UserReturnRate, error) {
	var out models.UserReturnRate
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO user_return_rates (user_id, asset_class, return_rate)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, asset_class) DO UPDATE SET return_rate = EXCLUDED.return_rate
		RETURNING *`,
		userID, assetClass, returnRate,
	).StructScan(&out)
	if err != nil {
		return nil, fmt.Errorf("upsert return rate: %w", err)
	}
	return &out, nil
}

// Delete removes a custom return rate (falling back to system default).
func (s *ReturnRateStore) Delete(ctx context.Context, userID uuid.UUID, assetClass string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM user_return_rates WHERE user_id = $1 AND asset_class = $2`,
		userID, assetClass,
	)
	if err != nil {
		return fmt.Errorf("delete return rate: %w", err)
	}
	return nil
}

// SetAccountOverride sets or clears a per-account return rate override.
func (s *ReturnRateStore) SetAccountOverride(ctx context.Context, accountID, userID uuid.UUID, rate *float64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET return_rate_override = $3 WHERE id = $1 AND user_id = $2`,
		accountID, userID, rate,
	)
	if err != nil {
		return fmt.Errorf("set account return rate override: %w", err)
	}
	return nil
}
