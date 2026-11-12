package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type LoanStore struct{ db *sqlx.DB }

func NewLoanStore(db *sqlx.DB) *LoanStore { return &LoanStore{db: db} }

func (s *LoanStore) Get(ctx context.Context, accountID, userID uuid.UUID) (*models.LoanDetails, error) {
	var d models.LoanDetails
	err := s.db.QueryRowxContext(ctx,
		`SELECT * FROM loan_details WHERE account_id = $1 AND user_id = $2`,
		accountID, userID,
	).StructScan(&d)
	if err != nil {
		return nil, fmt.Errorf("get loan details: %w", err)
	}
	return &d, nil
}

type UpsertLoanParams struct {
	AccountID      uuid.UUID
	UserID         uuid.UUID
	APRPercentage  *float64
	MonthlyPayment *float64
}

func (s *LoanStore) Upsert(ctx context.Context, p UpsertLoanParams) (*models.LoanDetails, error) {
	var d models.LoanDetails
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO loan_details (account_id, user_id, apr_percentage, monthly_payment)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (account_id) DO UPDATE
			SET apr_percentage  = EXCLUDED.apr_percentage,
			    monthly_payment = EXCLUDED.monthly_payment,
			    updated_at      = NOW()
		RETURNING *`,
		p.AccountID, p.UserID, p.APRPercentage, p.MonthlyPayment,
	).StructScan(&d)
	if err != nil {
		return nil, fmt.Errorf("upsert loan details: %w", err)
	}
	return &d, nil
}
