package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type BudgetCategoryStore struct{ db *sqlx.DB }

func NewBudgetCategoryStore(db *sqlx.DB) *BudgetCategoryStore {
	return &BudgetCategoryStore{db: db}
}

// List returns system categories plus the user's custom categories, ordered by sort_order then name.
func (s *BudgetCategoryStore) List(ctx context.Context, userID uuid.UUID) ([]models.BudgetCategory, error) {
	var cats []models.BudgetCategory
	err := s.db.SelectContext(ctx, &cats, `
		SELECT * FROM budget_categories
		WHERE user_id IS NULL OR user_id = $1
		ORDER BY sort_order, name`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list budget categories: %w", err)
	}
	return cats, nil
}

// Get returns a category the user can see (a system one or their own), or
// sql.ErrNoRows-wrapped error when there isn't one.
func (s *BudgetCategoryStore) Get(ctx context.Context, userID, id uuid.UUID) (*models.BudgetCategory, error) {
	var cat models.BudgetCategory
	err := s.db.GetContext(ctx, &cat, `
		SELECT * FROM budget_categories
		WHERE id = $1 AND (user_id IS NULL OR user_id = $2)`, id, userID)
	if err != nil {
		return nil, fmt.Errorf("get budget category: %w", err)
	}
	return &cat, nil
}

// LoadVendorRules loads all vendor rules into memory for use by the classifier.
func (s *BudgetCategoryStore) LoadVendorRules(ctx context.Context) ([]models.VendorRule, error) {
	var rules []models.VendorRule
	err := s.db.SelectContext(ctx, &rules, `SELECT id, keyword, category_id FROM vendor_rules`)
	if err != nil {
		return nil, fmt.Errorf("load vendor rules: %w", err)
	}
	return rules, nil
}

func (s *BudgetCategoryStore) Create(ctx context.Context, userID uuid.UUID, name string) (*models.BudgetCategory, error) {
	var cat models.BudgetCategory
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO budget_categories (user_id, name, is_system, sort_order)
		VALUES ($1, $2, false, 100)
		RETURNING *`,
		userID, name,
	).StructScan(&cat)
	if err != nil {
		return nil, fmt.Errorf("create budget category: %w", err)
	}
	return &cat, nil
}
