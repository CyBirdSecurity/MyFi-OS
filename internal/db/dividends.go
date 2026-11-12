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

type DividendStore struct{ db *sqlx.DB }

func NewDividendStore(db *sqlx.DB) *DividendStore { return &DividendStore{db: db} }

type CreateDividendParams struct {
	HoldingID        uuid.UUID
	UserID           uuid.UUID
	ExDate           time.Time
	DividendPerShare float64
	SharesBefore     float64
	DividendAmount   float64
	DripApplied      bool
	SharesAdded      *float64
	PriceUsed        *float64
}

// Create inserts a dividend record. Returns (nil, nil) if the
// (holding_id, ex_date) pair already exists — callers should treat that as
// "already applied, nothing to do".
func (s *DividendStore) Create(ctx context.Context, p CreateDividendParams) (*models.HoldingDividend, error) {
	var d models.HoldingDividend
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO holding_dividends
			(holding_id, user_id, ex_date, dividend_per_share, shares_before, dividend_amount, drip_applied, shares_added, price_used)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (holding_id, ex_date) DO NOTHING
		RETURNING *`,
		p.HoldingID, p.UserID, p.ExDate, p.DividendPerShare, p.SharesBefore, p.DividendAmount,
		p.DripApplied, p.SharesAdded, p.PriceUsed,
	).StructScan(&d)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("create dividend: %w", err)
	}
	return &d, nil
}

// LastAppliedDate returns the most recent ex_date already recorded for a
// holding, or nil if the holding has never been checked before.
func (s *DividendStore) LastAppliedDate(ctx context.Context, holdingID uuid.UUID) (*time.Time, error) {
	var exDate sql.NullTime
	err := s.db.GetContext(ctx, &exDate,
		`SELECT MAX(ex_date) FROM holding_dividends WHERE holding_id = $1`,
		holdingID,
	)
	if err != nil {
		return nil, fmt.Errorf("last applied date: %w", err)
	}
	if !exDate.Valid {
		return nil, nil
	}
	return &exDate.Time, nil
}

// ListByHolding returns dividend history for a holding, most recent first.
func (s *DividendStore) ListByHolding(ctx context.Context, holdingID, userID uuid.UUID) ([]models.HoldingDividend, error) {
	var divs []models.HoldingDividend
	err := s.db.SelectContext(ctx, &divs,
		`SELECT * FROM holding_dividends WHERE holding_id = $1 AND user_id = $2 ORDER BY ex_date DESC`,
		holdingID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list dividends by holding: %w", err)
	}
	return divs, nil
}
