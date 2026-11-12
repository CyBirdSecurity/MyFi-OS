package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type BudgetLimitStore struct{ db *sqlx.DB }

func NewBudgetLimitStore(db *sqlx.DB) *BudgetLimitStore { return &BudgetLimitStore{db: db} }

// List returns the user's monthly limits. It is never nil, so it serializes as
// an empty array rather than null.
func (s *BudgetLimitStore) List(ctx context.Context, userID uuid.UUID) ([]models.BudgetLimit, error) {
	limits := []models.BudgetLimit{}
	err := s.db.SelectContext(ctx, &limits, `
		SELECT category_id, monthly_limit
		FROM budget_limits
		WHERE user_id = $1
		ORDER BY category_id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list budget limits: %w", err)
	}
	return limits, nil
}

// Upsert sets the user's monthly limit for a category, replacing any earlier one.
func (s *BudgetLimitStore) Upsert(ctx context.Context, userID, categoryID uuid.UUID, monthlyLimit float64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO budget_limits (user_id, category_id, monthly_limit)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, category_id) DO UPDATE SET
			monthly_limit = EXCLUDED.monthly_limit,
			updated_at    = NOW()`,
		userID, categoryID, monthlyLimit)
	if err != nil {
		return fmt.Errorf("set budget limit: %w", err)
	}
	return nil
}

// Delete removes the user's limit for a category. Removing one that isn't set is not an error.
func (s *BudgetLimitStore) Delete(ctx context.Context, userID, categoryID uuid.UUID) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM budget_limits WHERE user_id = $1 AND category_id = $2`, userID, categoryID); err != nil {
		return fmt.Errorf("delete budget limit: %w", err)
	}
	return nil
}
