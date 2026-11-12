package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type HoldingStore struct{ db *sqlx.DB }

func NewHoldingStore(db *sqlx.DB) *HoldingStore { return &HoldingStore{db: db} }

func (s *HoldingStore) ListByAccount(ctx context.Context, accountID, userID uuid.UUID) ([]models.Holding, error) {
	var h []models.Holding
	err := s.db.SelectContext(ctx, &h,
		`SELECT * FROM holdings WHERE account_id = $1 AND user_id = $2 ORDER BY ticker`,
		accountID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list holdings by account: %w", err)
	}
	return h, nil
}

func (s *HoldingStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]models.Holding, error) {
	var h []models.Holding
	err := s.db.SelectContext(ctx, &h,
		`SELECT h.*, a.account_type FROM holdings h
		 JOIN accounts a ON a.id = h.account_id
		 WHERE h.user_id = $1 ORDER BY h.ticker`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list holdings by user: %w", err)
	}
	return h, nil
}

type CreateHoldingParams struct {
	AccountID     uuid.UUID
	UserID        uuid.UUID
	Ticker        string
	Name          *string
	AssetClass    string
	Shares        float64
	CostBasis     *float64
	ManualPrice   bool
	PricePerShare *float64
}

func (s *HoldingStore) Create(ctx context.Context, p CreateHoldingParams) (*models.Holding, error) {
	var h models.Holding
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO holdings (account_id, user_id, ticker, name, asset_class, shares, cost_basis, manual_price, price_per_share)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING *`,
		p.AccountID, p.UserID, p.Ticker, p.Name, p.AssetClass, p.Shares, p.CostBasis, p.ManualPrice, p.PricePerShare,
	).StructScan(&h)
	if err != nil {
		return nil, fmt.Errorf("create holding: %w", err)
	}
	return &h, nil
}

type UpdateHoldingParams struct {
	ID            uuid.UUID
	AccountID     uuid.UUID
	UserID        uuid.UUID
	Ticker        string
	Name          *string
	AssetClass    string
	Shares        float64
	CostBasis     *float64
	ManualPrice   bool
	PricePerShare *float64
	// DripEnabled is a pointer so omitting it from a request (older clients
	// that predate the DRIP feature) leaves the holding's existing value
	// untouched instead of silently disabling reinvestment.
	DripEnabled *bool
}

func (s *HoldingStore) Update(ctx context.Context, p UpdateHoldingParams) (*models.Holding, error) {
	var h models.Holding
	err := s.db.QueryRowxContext(ctx, `
		UPDATE holdings
		SET ticker = $4, name = $5, asset_class = $6, shares = $7, cost_basis = $8,
		    manual_price = $9, price_per_share = CASE WHEN $9 THEN $10 ELSE price_per_share END,
		    drip_enabled = COALESCE($11, drip_enabled)
		WHERE id = $1 AND account_id = $2 AND user_id = $3
		RETURNING *`,
		p.ID, p.AccountID, p.UserID, p.Ticker, p.Name, p.AssetClass, p.Shares, p.CostBasis,
		p.ManualPrice, p.PricePerShare, p.DripEnabled,
	).StructScan(&h)
	if err != nil {
		return nil, fmt.Errorf("update holding: %w", err)
	}
	return &h, nil
}

func (s *HoldingStore) Delete(ctx context.Context, id, accountID, userID uuid.UUID) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM holdings WHERE id = $1 AND account_id = $2 AND user_id = $3`,
		id, accountID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete holding: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("holding not found")
	}
	return nil
}

func (s *HoldingStore) UpdatePrice(ctx context.Context, id uuid.UUID, price float64, fetchedAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE holdings SET price_per_share = $2, price_fetched_at = $3 WHERE id = $1`,
		id, price, fetchedAt,
	)
	return err
}

// ReplaceForTickers deletes existing holdings matching the given tickers for an account,
// then bulk-inserts the replacement set — all inside a single transaction.
// Only the tickers present in params are touched; other holdings are left as-is.
func (s *HoldingStore) ReplaceForTickers(ctx context.Context, accountID, userID uuid.UUID, params []CreateHoldingParams) (int, error) {
	if len(params) == 0 {
		return 0, nil
	}

	tickers := make([]string, len(params))
	for i, p := range params {
		tickers[i] = p.Ticker
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.ExecContext(ctx,
		`DELETE FROM holdings WHERE account_id = $1 AND user_id = $2 AND ticker = ANY($3)`,
		accountID, userID, tickers,
	)
	if err != nil {
		return 0, fmt.Errorf("delete holdings: %w", err)
	}

	inserted := 0
	for _, p := range params {
		if p.Shares <= 0 {
			continue // delete-only sentinel — the DELETE above already removed it
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO holdings (account_id, user_id, ticker, name, asset_class, shares, cost_basis, manual_price, price_per_share)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			accountID, userID, p.Ticker, p.Name, p.AssetClass, p.Shares, p.CostBasis, p.ManualPrice, p.PricePerShare,
		)
		if err != nil {
			return 0, fmt.Errorf("insert holding %s: %w", p.Ticker, err)
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return inserted, nil
}

type CostBasisUpdate struct {
	ID        uuid.UUID
	CostBasis *float64
}

// BulkUpdateCostBasis updates cost_basis for a set of holdings belonging to the given user.
func (s *HoldingStore) BulkUpdateCostBasis(ctx context.Context, userID uuid.UUID, updates []CostBasisUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	for _, u := range updates {
		if _, err = tx.ExecContext(ctx,
			`UPDATE holdings SET cost_basis = $1, updated_at = NOW() WHERE id = $2 AND user_id = $3`,
			u.CostBasis, u.ID, userID,
		); err != nil {
			return fmt.Errorf("update cost basis %s: %w", u.ID, err)
		}
	}
	return tx.Commit()
}

// AddShares increments a holding's share count by delta. Used by
// AutoDividendService to apply DRIP reinvestment.
func (s *HoldingStore) AddShares(ctx context.Context, id uuid.UUID, delta float64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE holdings SET shares = shares + $2, updated_at = NOW() WHERE id = $1`,
		id, delta,
	)
	if err != nil {
		return fmt.Errorf("add shares: %w", err)
	}
	return nil
}

// ListUsersWithHoldings returns the distinct user IDs that have at least one holding.
// Used by the auto-snapshot scheduler to know which users to process.
func (s *HoldingStore) ListUsersWithHoldings(ctx context.Context) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := s.db.SelectContext(ctx, &ids, `SELECT DISTINCT user_id FROM holdings`)
	if err != nil {
		return nil, fmt.Errorf("list users with holdings: %w", err)
	}
	return ids, nil
}
