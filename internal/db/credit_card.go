package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type CreditCardStore struct{ db *sqlx.DB }

func NewCreditCardStore(db *sqlx.DB) *CreditCardStore { return &CreditCardStore{db: db} }

func (s *CreditCardStore) Get(ctx context.Context, accountID, userID uuid.UUID) (*models.CreditCardDetails, error) {
	var d models.CreditCardDetails
	err := s.db.QueryRowxContext(ctx,
		`SELECT * FROM credit_card_details WHERE account_id = $1 AND user_id = $2`,
		accountID, userID,
	).StructScan(&d)
	if err != nil {
		return nil, fmt.Errorf("get credit card details: %w", err)
	}
	return &d, nil
}

type UpsertCreditCardParams struct {
	AccountID              uuid.UUID
	UserID                 uuid.UUID
	APRPercentage          *float64
	CreditLimit            *float64
	LastStatementBalance   *float64
	LastStatementIssueDate *time.Time
	MinimumPaymentAmount   *float64
	NextPaymentDueDate     *time.Time
	LastPaymentAmount      *float64
	LastPaymentDate        *time.Time
	IsOverdue              bool
}

func (s *CreditCardStore) Upsert(ctx context.Context, p UpsertCreditCardParams) (*models.CreditCardDetails, error) {
	var d models.CreditCardDetails
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO credit_card_details (account_id, user_id, apr_percentage, credit_limit, last_statement_balance, last_statement_issue_date, minimum_payment_amount, next_payment_due_date, last_payment_amount, last_payment_date, is_overdue)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (account_id) DO UPDATE
			SET apr_percentage             = EXCLUDED.apr_percentage,
			    credit_limit               = EXCLUDED.credit_limit,
			    last_statement_balance     = EXCLUDED.last_statement_balance,
			    last_statement_issue_date  = EXCLUDED.last_statement_issue_date,
			    minimum_payment_amount     = EXCLUDED.minimum_payment_amount,
			    next_payment_due_date      = EXCLUDED.next_payment_due_date,
			    last_payment_amount        = EXCLUDED.last_payment_amount,
			    last_payment_date          = EXCLUDED.last_payment_date,
			    is_overdue                 = EXCLUDED.is_overdue,
			    updated_at                 = NOW()
		RETURNING *`,
		p.AccountID, p.UserID, p.APRPercentage, p.CreditLimit, p.LastStatementBalance, p.LastStatementIssueDate, p.MinimumPaymentAmount, p.NextPaymentDueDate, p.LastPaymentAmount, p.LastPaymentDate, p.IsOverdue,
	).StructScan(&d)
	if err != nil {
		return nil, fmt.Errorf("upsert credit card details: %w", err)
	}
	return &d, nil
}
