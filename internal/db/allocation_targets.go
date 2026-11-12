package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type AllocationTargetStore struct{ db *sqlx.DB }

func NewAllocationTargetStore(db *sqlx.DB) *AllocationTargetStore {
	return &AllocationTargetStore{db: db}
}

func (s *AllocationTargetStore) Get(ctx context.Context, userID uuid.UUID) (*models.AllocationTarget, error) {
	var t models.AllocationTarget
	err := s.db.QueryRowxContext(ctx,
		`SELECT * FROM allocation_targets WHERE user_id = $1`, userID,
	).StructScan(&t)
	if err != nil {
		return nil, nil // no targets yet
	}
	return &t, nil
}

type UpsertAllocationTargetParams struct {
	UserID    uuid.UUID
	EquityPct float64
	BondPct   float64
	CryptoPct float64
	CashPct   float64
	OtherPct  float64
}

func (s *AllocationTargetStore) Upsert(ctx context.Context, p UpsertAllocationTargetParams) (*models.AllocationTarget, error) {
	var out models.AllocationTarget
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO allocation_targets (user_id, equity_pct, bond_pct, crypto_pct, cash_pct, other_pct)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id) DO UPDATE SET
			equity_pct = EXCLUDED.equity_pct,
			bond_pct   = EXCLUDED.bond_pct,
			crypto_pct = EXCLUDED.crypto_pct,
			cash_pct   = EXCLUDED.cash_pct,
			other_pct  = EXCLUDED.other_pct,
			updated_at = now()
		RETURNING *`,
		p.UserID, p.EquityPct, p.BondPct, p.CryptoPct, p.CashPct, p.OtherPct,
	).StructScan(&out)
	if err != nil {
		return nil, fmt.Errorf("upsert allocation targets: %w", err)
	}
	return &out, nil
}
