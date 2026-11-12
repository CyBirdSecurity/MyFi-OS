package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type ContributionStore struct{ db *sqlx.DB }

func NewContributionStore(db *sqlx.DB) *ContributionStore { return &ContributionStore{db: db} }

// ListByAccount returns all contributions for an account owned by the requesting user.
func (s *ContributionStore) ListByAccount(ctx context.Context, accountID, userID uuid.UUID) ([]models.Contribution, error) {
	var out []models.Contribution
	err := s.db.SelectContext(ctx, &out, `
		SELECT c.* FROM contributions c
		JOIN accounts a ON a.id = c.account_id
		WHERE c.account_id = $1
		  AND a.user_id = $2
		ORDER BY c.created_at`,
		accountID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list contributions: %w", err)
	}
	return out, nil
}

// ListByUser returns all active contributions for a user across all accounts.
func (s *ContributionStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]models.Contribution, error) {
	var out []models.Contribution
	err := s.db.SelectContext(ctx, &out,
		`SELECT * FROM contributions WHERE user_id = $1 AND is_active = TRUE ORDER BY account_id, created_at`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list user contributions: %w", err)
	}
	return out, nil
}

type CreateContributionParams struct {
	AccountID         uuid.UUID
	UserID            uuid.UUID
	Label             *string
	Amount            float64
	Frequency         string
	AnnualIncreasePct float64
	AmountType        string
	IncomePct         float64
	MatchTiers        models.MatchTiers
	NextPayDate       *time.Time
	StartDate         *time.Time
	EndDate           *time.Time
}

func (s *ContributionStore) Create(ctx context.Context, p CreateContributionParams) (*models.Contribution, error) {
	var out models.Contribution
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO contributions (account_id, user_id, label, amount, frequency, annual_increase_pct, amount_type, income_pct, match_tiers, next_pay_date, start_date, end_date)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING *`,
		p.AccountID, p.UserID, p.Label, p.Amount, p.Frequency, p.AnnualIncreasePct, p.AmountType, p.IncomePct, p.MatchTiers, p.NextPayDate, p.StartDate, p.EndDate,
	).StructScan(&out)
	if err != nil {
		return nil, fmt.Errorf("create contribution: %w", err)
	}
	return &out, nil
}

type UpdateContributionParams struct {
	ID                uuid.UUID
	AccountID         uuid.UUID
	UserID            uuid.UUID
	Label             *string
	Amount            float64
	Frequency         string
	AnnualIncreasePct float64
	AmountType        string
	IncomePct         float64
	MatchTiers        models.MatchTiers
	IsActive          bool
	NextPayDate       *time.Time
	StartDate         *time.Time
	EndDate           *time.Time
}

func (s *ContributionStore) Update(ctx context.Context, p UpdateContributionParams) (*models.Contribution, error) {
	var out models.Contribution
	err := s.db.QueryRowxContext(ctx, `
		UPDATE contributions
		SET label = $4, amount = $5, frequency = $6,
		    annual_increase_pct = $7, amount_type = $8, income_pct = $9, match_tiers = $10, is_active = $11,
		    next_pay_date = $12, start_date = $13, end_date = $14
		WHERE id = $1 AND account_id = $2 AND user_id = $3
		RETURNING *`,
		p.ID, p.AccountID, p.UserID,
		p.Label, p.Amount, p.Frequency,
		p.AnnualIncreasePct, p.AmountType, p.IncomePct, p.MatchTiers, p.IsActive,
		p.NextPayDate, p.StartDate, p.EndDate,
	).StructScan(&out)
	if err != nil {
		return nil, fmt.Errorf("update contribution: %w", err)
	}
	return &out, nil
}

func (s *ContributionStore) Delete(ctx context.Context, id, accountID, userID uuid.UUID) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM contributions
		WHERE id = $1 AND account_id = $2 AND user_id = $3`,
		id, accountID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete contribution: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("contribution not found")
	}
	return nil
}
