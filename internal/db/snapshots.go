package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type SnapshotStore struct{ db *sqlx.DB }

func NewSnapshotStore(db *sqlx.DB) *SnapshotStore { return &SnapshotStore{db: db} }

// List returns all snapshots for an account owned by the requesting user.
func (s *SnapshotStore) List(ctx context.Context, accountID, userID uuid.UUID) ([]models.BalanceSnapshot, error) {
	var snaps []models.BalanceSnapshot
	err := s.db.SelectContext(ctx, &snaps, `
		SELECT bs.* FROM balance_snapshots bs
		JOIN accounts a ON a.id = bs.account_id
		WHERE bs.account_id = $1
		  AND a.user_id = $2
		ORDER BY bs.snapshot_date DESC`,
		accountID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	return snaps, nil
}

// LatestForAccount returns the single most recent snapshot (any source) for
// one account, or nil if none exists.
func (s *SnapshotStore) LatestForAccount(ctx context.Context, accountID, userID uuid.UUID) (*models.BalanceSnapshot, error) {
	var snap models.BalanceSnapshot
	err := s.db.QueryRowxContext(ctx, `
		SELECT * FROM balance_snapshots
		WHERE account_id = $1 AND user_id = $2
		ORDER BY snapshot_date DESC
		LIMIT 1`,
		accountID, userID,
	).StructScan(&snap)
	if err != nil {
		return nil, nil // not found is not an error here
	}
	return &snap, nil
}

// ListByUser returns the most recent snapshot per account for a user (for net worth computation).
func (s *SnapshotStore) LatestPerAccount(ctx context.Context, userID uuid.UUID) ([]models.BalanceSnapshot, error) {
	var snaps []models.BalanceSnapshot
	err := s.db.SelectContext(ctx, &snaps, `
		SELECT DISTINCT ON (account_id) *
		FROM balance_snapshots
		WHERE user_id = $1
		ORDER BY account_id, snapshot_date DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("latest per account: %w", err)
	}
	return snaps, nil
}

// AccountBucketBalance is one account's carried-forward balance within a
// single chart bucket, used so callers can apply per-account logic (like
// splitting a home account's equity into property value + mortgage
// liability) before aggregating into a HistoryPoint.
type AccountBucketBalance struct {
	Bucket    time.Time `db:"bucket"`
	AccountID uuid.UUID `db:"account_id"`
	Balance   float64   `db:"balance"`
}

// HistoryByAccount returns each account's carried-forward balance per date
// bucket for chart rendering. interval: 'day', 'week', 'month', 'year'
//
// For each bucket, we carry forward each account's most recent snapshot value
// so that buckets with no new snapshot for an account still include that
// account's last known balance (preventing artificial downspikes).
func (s *SnapshotStore) HistoryByAccount(ctx context.Context, userID uuid.UUID, from, to time.Time, interval string) ([]AccountBucketBalance, error) {
	query := `
		WITH
		-- One row per (account, bucket): the snapshot with the latest snapshot_date in that bucket
		snapshots_per_bucket AS (
			SELECT DISTINCT ON (account_id, snap_bucket)
				account_id,
				balance,
				snap_bucket
			FROM (
				SELECT
					account_id,
					balance,
					snapshot_date,
					date_trunc($4, snapshot_date::timestamp) AS snap_bucket
				FROM balance_snapshots
				WHERE user_id = $1 AND snapshot_date <= $3
			) sub
			ORDER BY account_id, snap_bucket, snapshot_date DESC
		),
		-- Every bucket in the requested range
		buckets AS (
			SELECT generate_series(
				date_trunc($4, $2::timestamp),
				date_trunc($4, $3::timestamp),
				('1 ' || $4)::interval
			) AS bucket
		),
		-- Most recent pre-range balance per account, seeds carry-forward for the first bucket
		seed_balances AS (
			SELECT DISTINCT ON (account_id)
				account_id,
				balance
			FROM snapshots_per_bucket
			WHERE snap_bucket < date_trunc($4, $2::timestamp)
			ORDER BY account_id, snap_bucket DESC
		),
		-- For each account × bucket, carry forward the most recent known balance.
		-- Uses the COUNT/MAX window pattern to avoid a correlated subquery per row:
		--   COUNT(non-null balance) increments the group number each time a real value
		--   appears, so consecutive NULLs share the same group; MAX within that group
		--   fills them with the preceding non-null value.
		account_buckets AS (
			SELECT
				bucket,
				account_id,
				MAX(raw_balance) OVER (PARTITION BY account_id, grp) AS balance
			FROM (
				SELECT
					bucket,
					account_id,
					raw_balance,
					COUNT(raw_balance) OVER (
						PARTITION BY account_id
						ORDER BY bucket
					) AS grp
				FROM (
					SELECT
						b.bucket,
						a.account_id,
						COALESCE(
							spb.balance,
							CASE WHEN b.bucket = date_trunc($4, $2::timestamp) THEN seed.balance END
						) AS raw_balance
					FROM buckets b
					CROSS JOIN (SELECT DISTINCT account_id FROM snapshots_per_bucket) a
					LEFT JOIN snapshots_per_bucket spb
						ON spb.account_id = a.account_id
						AND spb.snap_bucket = b.bucket
					LEFT JOIN seed_balances seed ON seed.account_id = a.account_id
				) raw
			) with_grp
		)
		SELECT bucket, account_id, balance
		FROM account_buckets
		WHERE balance IS NOT NULL
		ORDER BY bucket`

	var rows []AccountBucketBalance
	err := s.db.SelectContext(ctx, &rows, query, userID, from, to, interval)
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	return rows, nil
}

type HistoryPoint struct {
	Date        time.Time `db:"date"        json:"date"`
	Assets      float64   `db:"assets"      json:"assets"`
	Liabilities float64   `db:"liabilities" json:"liabilities"`
	NetWorth    float64   `db:"net_worth"   json:"netWorth"`
}

type UpsertSnapshotParams struct {
	AccountID    uuid.UUID
	UserID       uuid.UUID
	SnapshotDate time.Time
	Balance      float64
	Miles        *int
	Notes        *string
	Source       string
}

func (s *SnapshotStore) Upsert(ctx context.Context, p UpsertSnapshotParams) (*models.BalanceSnapshot, error) {
	var snap models.BalanceSnapshot
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO balance_snapshots (account_id, user_id, snapshot_date, balance, miles, notes, source)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (account_id, snapshot_date)
		DO UPDATE SET balance = EXCLUDED.balance, miles = EXCLUDED.miles, notes = EXCLUDED.notes, updated_at = NOW()
		RETURNING *`,
		p.AccountID, p.UserID, p.SnapshotDate, p.Balance, p.Miles, p.Notes, p.Source,
	).StructScan(&snap)
	if err != nil {
		return nil, fmt.Errorf("upsert snapshot: %w", err)
	}
	return &snap, nil
}

type UpdateSnapshotParams struct {
	ID           uuid.UUID
	AccountID    uuid.UUID
	UserID       uuid.UUID
	SnapshotDate time.Time
	Balance      float64
	Miles        *int
	Notes        *string
}

// Update modifies an existing snapshot owned by the requesting user.
func (s *SnapshotStore) Update(ctx context.Context, p UpdateSnapshotParams) (*models.BalanceSnapshot, error) {
	var snap models.BalanceSnapshot
	err := s.db.QueryRowxContext(ctx, `
		UPDATE balance_snapshots
		SET snapshot_date = $4, balance = $5, miles = $6, notes = $7, updated_at = NOW()
		WHERE id = $1 AND account_id = $2 AND user_id = $3
		RETURNING *`,
		p.ID, p.AccountID, p.UserID, p.SnapshotDate, p.Balance, p.Miles, p.Notes,
	).StructScan(&snap)
	if err != nil {
		return nil, fmt.Errorf("update snapshot: %w", err)
	}
	return &snap, nil
}

// Delete removes a snapshot owned by the requesting user.
func (s *SnapshotStore) Delete(ctx context.Context, id, userID uuid.UUID) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM balance_snapshots
		WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete snapshot: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("snapshot not found")
	}
	return nil
}

// BulkUpsertSkipManual inserts multiple snapshots in a single query like
// BulkUpsert, but never overwrites a snapshot the user entered manually
// (source='manual'). Used by the Plaid sync and backfill so automated data
// can't clobber user-entered history. Unlike BulkUpsert it also updates
// source on conflict, so a 'computed' row is upgraded to 'plaid'.
// Callers must not pass duplicate (account_id, snapshot_date) pairs within
// one call — Postgres rejects updating the same row twice in one statement.
func (s *SnapshotStore) BulkUpsertSkipManual(ctx context.Context, snaps []UpsertSnapshotParams) (int, error) {
	if len(snaps) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(snaps))
	args := make([]any, 0, len(snaps)*7)
	for i, p := range snaps {
		base := i * 7
		placeholders[i] = fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6, base+7)
		args = append(args, p.AccountID, p.UserID, p.SnapshotDate, p.Balance, p.Miles, p.Notes, p.Source)
	}

	q := `INSERT INTO balance_snapshots (account_id, user_id, snapshot_date, balance, miles, notes, source)
		VALUES ` + strings.Join(placeholders, ", ") + `
		ON CONFLICT (account_id, snapshot_date)
		DO UPDATE SET balance = EXCLUDED.balance, miles = EXCLUDED.miles, notes = EXCLUDED.notes,
		              source = EXCLUDED.source, updated_at = NOW()
		WHERE balance_snapshots.source <> 'manual'`

	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("bulk upsert snapshots (skip manual): %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// BulkUpsert inserts multiple snapshots in a single query, used by the CSV importer.
func (s *SnapshotStore) BulkUpsert(ctx context.Context, snaps []UpsertSnapshotParams) (int, error) {
	if len(snaps) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(snaps))
	args := make([]any, 0, len(snaps)*7)
	for i, p := range snaps {
		base := i * 7
		placeholders[i] = fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6, base+7)
		args = append(args, p.AccountID, p.UserID, p.SnapshotDate, p.Balance, p.Miles, p.Notes, p.Source)
	}

	q := `INSERT INTO balance_snapshots (account_id, user_id, snapshot_date, balance, miles, notes, source)
		VALUES ` + strings.Join(placeholders, ", ") + `
		ON CONFLICT (account_id, snapshot_date)
		DO UPDATE SET balance = EXCLUDED.balance, miles = EXCLUDED.miles, notes = EXCLUDED.notes, updated_at = NOW()`

	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("bulk upsert snapshots: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
