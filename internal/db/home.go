package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type HomeStore struct{ db *sqlx.DB }

func NewHomeStore(db *sqlx.DB) *HomeStore { return &HomeStore{db: db} }

type UpsertHomeParams struct {
	AccountID                uuid.UUID
	UserID                   uuid.UUID
	Address                  string
	InterestRate             float64
	LoanTermYears            int
	OriginalLoanValue        float64
	CurrentLoanBalance       float64
	MortgageStartDate        *time.Time
	ExtraMonthlyPayment      float64
	TrackMortgageAsLiability bool
	IncludeInRetirementCalcs bool
	PropertyTaxAnnual        float64
	HomeInsuranceAnnual      float64
	HOAMonthly               float64
	PMIMonthly               float64
	PurchasePrice            float64
	// BalanceIsCurrent marks CurrentLoanBalance as today's balance even when
	// it hasn't changed (a Plaid sync); otherwise its as-of date only moves
	// when the balance does.
	BalanceIsCurrent bool
}

func (s *HomeStore) Get(ctx context.Context, accountID, userID uuid.UUID) (*models.HomeDetails, error) {
	var d models.HomeDetails
	err := s.db.QueryRowxContext(ctx,
		`SELECT * FROM home_details WHERE account_id = $1 AND user_id = $2`,
		accountID, userID,
	).StructScan(&d)
	if err != nil {
		return nil, fmt.Errorf("get home details: %w", err)
	}
	return &d, nil
}

// ListByUser returns home details for every home account owned by the user.
func (s *HomeStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]models.HomeDetails, error) {
	var rows []models.HomeDetails
	err := s.db.SelectContext(ctx, &rows,
		`SELECT * FROM home_details WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list home details: %w", err)
	}
	return rows, nil
}

func (s *HomeStore) Upsert(ctx context.Context, p UpsertHomeParams) (*models.HomeDetails, error) {
	var d models.HomeDetails
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO home_details (account_id, user_id, address, interest_rate, loan_term_years, original_loan_value, current_loan_balance, mortgage_start_date, extra_monthly_payment, track_mortgage_as_liability, include_in_retirement_calcs,
		                          property_tax_annual, home_insurance_annual, hoa_monthly, pmi_monthly, purchase_price, loan_balance_as_of)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, CURRENT_DATE)
		ON CONFLICT (account_id) DO UPDATE
			SET address                      = EXCLUDED.address,
			    interest_rate                = EXCLUDED.interest_rate,
			    loan_term_years              = EXCLUDED.loan_term_years,
			    original_loan_value          = EXCLUDED.original_loan_value,
			    current_loan_balance         = EXCLUDED.current_loan_balance,
			    mortgage_start_date          = EXCLUDED.mortgage_start_date,
			    extra_monthly_payment        = EXCLUDED.extra_monthly_payment,
			    track_mortgage_as_liability  = EXCLUDED.track_mortgage_as_liability,
			    include_in_retirement_calcs  = EXCLUDED.include_in_retirement_calcs,
			    property_tax_annual          = EXCLUDED.property_tax_annual,
			    home_insurance_annual        = EXCLUDED.home_insurance_annual,
			    hoa_monthly                  = EXCLUDED.hoa_monthly,
			    pmi_monthly                  = EXCLUDED.pmi_monthly,
			    purchase_price               = EXCLUDED.purchase_price,
			    loan_balance_as_of           = CASE
			        WHEN $17 OR home_details.current_loan_balance IS DISTINCT FROM EXCLUDED.current_loan_balance
			        THEN CURRENT_DATE ELSE home_details.loan_balance_as_of END,
			    updated_at                   = NOW()
		RETURNING *`,
		p.AccountID, p.UserID, p.Address, p.InterestRate, p.LoanTermYears, p.OriginalLoanValue, p.CurrentLoanBalance, p.MortgageStartDate, p.ExtraMonthlyPayment, p.TrackMortgageAsLiability, p.IncludeInRetirementCalcs,
		p.PropertyTaxAnnual, p.HomeInsuranceAnnual, p.HOAMonthly, p.PMIMonthly, p.PurchasePrice, p.BalanceIsCurrent,
	).StructScan(&d)
	if err != nil {
		return nil, fmt.Errorf("upsert home details: %w", err)
	}
	return &d, nil
}

func (s *HomeStore) UpdateZestimate(ctx context.Context, accountID uuid.UUID, value float64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE home_details
		SET last_zestimate = $2, last_zestimate_at = NOW(), updated_at = NOW()
		WHERE account_id = $1`,
		accountID, value,
	)
	if err != nil {
		return fmt.Errorf("update zestimate: %w", err)
	}
	return nil
}
