package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type ProfileStore struct{ db *sqlx.DB }

func NewProfileStore(db *sqlx.DB) *ProfileStore { return &ProfileStore{db: db} }

// Get returns the profile for a user, or nil if none exists yet.
func (s *ProfileStore) Get(ctx context.Context, userID uuid.UUID) (*models.UserProfile, error) {
	var p models.UserProfile
	err := s.db.QueryRowxContext(ctx,
		`SELECT id, user_id, use_inflation_adjustment, inflation_rate,
		        monthly_expenses, current_monthly_expenses, use_budget_for_expenses, withdrawal_rate,
		        filing_status, state_tax_rate, state_tax_code, dependents_count, created_at, updated_at
		 FROM user_profiles WHERE user_id = $1`, userID,
	).StructScan(&p)
	if err != nil {
		return nil, nil // no profile yet is not an error
	}
	return &p, nil
}

type UpsertProfileParams struct {
	UserID                 uuid.UUID
	UseInflationAdjustment bool
	InflationRate          float64
	MonthlyExpenses        float64
	CurrentMonthlyExpenses float64
	UseBudgetForExpenses   bool
	WithdrawalRate         *float64
	FilingStatus           string
	StateTaxRate           *float64
	StateTaxCode           *string
	DependentsCount        int
}

func (s *ProfileStore) Upsert(ctx context.Context, p UpsertProfileParams) (*models.UserProfile, error) {
	var out models.UserProfile
	filingStatus := p.FilingStatus
	if filingStatus == "" {
		filingStatus = "single"
	}
	if p.DependentsCount < 0 {
		p.DependentsCount = 0
	}
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO user_profiles (
			user_id, use_inflation_adjustment, inflation_rate,
			monthly_expenses, current_monthly_expenses, use_budget_for_expenses, withdrawal_rate,
			filing_status, state_tax_rate, state_tax_code, dependents_count
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (user_id) DO UPDATE SET
			use_inflation_adjustment = EXCLUDED.use_inflation_adjustment,
			inflation_rate           = EXCLUDED.inflation_rate,
			monthly_expenses         = EXCLUDED.monthly_expenses,
			current_monthly_expenses = EXCLUDED.current_monthly_expenses,
			use_budget_for_expenses  = EXCLUDED.use_budget_for_expenses,
			withdrawal_rate          = EXCLUDED.withdrawal_rate,
			filing_status            = EXCLUDED.filing_status,
			state_tax_rate           = EXCLUDED.state_tax_rate,
			state_tax_code           = EXCLUDED.state_tax_code,
			dependents_count         = EXCLUDED.dependents_count
		RETURNING id, user_id, use_inflation_adjustment, inflation_rate,
		          monthly_expenses, current_monthly_expenses, use_budget_for_expenses, withdrawal_rate,
		          filing_status, state_tax_rate, state_tax_code, dependents_count, created_at, updated_at`,
		p.UserID, p.UseInflationAdjustment, p.InflationRate,
		p.MonthlyExpenses, p.CurrentMonthlyExpenses, p.UseBudgetForExpenses, p.WithdrawalRate,
		filingStatus, p.StateTaxRate, p.StateTaxCode, p.DependentsCount,
	).StructScan(&out)
	if err != nil {
		return nil, fmt.Errorf("upsert profile: %w", err)
	}
	return &out, nil
}
