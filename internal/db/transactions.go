package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type TransactionStore struct{ db *sqlx.DB }

func NewTransactionStore(db *sqlx.DB) *TransactionStore {
	return &TransactionStore{db: db}
}

type ImportTransactionsResult struct {
	Inserted int
	Skipped  int
	Failed   int
}

type CreateTransactionParams struct {
	UserID       uuid.UUID
	TransDate    time.Time
	Merchant     string
	Amount       float64
	CategoryID   *uuid.UUID
	AccountID    *uuid.UUID
	AccountLabel *string // denormalized display name for AccountID; caller resolves it
	Notes        *string
}

func (s *TransactionStore) Create(ctx context.Context, p CreateTransactionParams) (*models.Transaction, error) {
	type row struct {
		models.Transaction
		CatName *string `db:"cat_name"`
	}
	var r row
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO transactions
			(user_id, trans_date, merchant, amount, category_id, account_id, account_label, raw_description, notes, needs_review)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $3, $8, false)
		RETURNING transactions.*,
			(SELECT name FROM budget_categories WHERE id = $5) AS cat_name`,
		p.UserID, p.TransDate, p.Merchant, p.Amount, p.CategoryID, p.AccountID, p.AccountLabel, p.Notes,
	).StructScan(&r)
	if err != nil {
		return nil, fmt.Errorf("create transaction: %w", err)
	}
	t := r.Transaction
	if r.CatName != nil {
		t.CategoryName = *r.CatName
	}
	return &t, nil
}

// BulkImport inserts transactions, skipping exact duplicates (user_id, trans_date, raw_description, amount).
// Rows that fail for other reasons are counted in Failed and processing continues.
func (s *TransactionStore) BulkImport(ctx context.Context, txns []models.Transaction) (ImportTransactionsResult, error) {
	var result ImportTransactionsResult
	var firstErr error
	for _, t := range txns {
		res, err := s.db.ExecContext(ctx, `
			INSERT INTO transactions
				(user_id, trans_date, merchant, amount, category_id, account_id, account_label, source_file, raw_description, needs_review)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (user_id, trans_date, raw_description, amount) DO NOTHING`,
			t.UserID, t.TransDate, t.Merchant, t.Amount, t.CategoryID, t.AccountID,
			t.AccountLabel, t.SourceFile, t.RawDescription, t.NeedsReview,
		)
		if err != nil {
			result.Failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			result.Inserted++
		} else {
			result.Skipped++
		}
	}
	if firstErr != nil {
		return result, fmt.Errorf("import had %d error(s), first: %w", result.Failed, firstErr)
	}
	return result, nil
}

// UpsertExternalParams describes one provider-sourced (e.g. Plaid) transaction.
// Category/NeedsReview apply only to the initial insert — an existing row's
// categorisation is preserved on update so a user's manual re-categorisation
// survives later provider "modified" events (which mostly finalise amounts).
type UpsertExternalParams struct {
	UserID                uuid.UUID
	ExternalTransactionID string
	ExternalProvider      string
	TransDate             time.Time
	Merchant              string
	Amount                float64
	CategoryID            *uuid.UUID
	AccountID             *uuid.UUID
	AccountLabel          *string
	RawDescription        string
	NeedsReview           bool
}

// UpsertExternal inserts a provider-sourced transaction, or updates the
// financial fields of an existing one (matched on user_id +
// external_transaction_id). Returns true when a new row was inserted.
func (s *TransactionStore) UpsertExternal(ctx context.Context, p UpsertExternalParams) (bool, error) {
	// Re-key path: Plaid is not guaranteed to keep transaction_id stable across
	// every Item-level operation — notably, widening transactions.days_requested
	// via a Link update can cause an already-synced transaction to reappear as
	// "added" under a brand-new id. That collides with the pre-existing
	// (user_id, trans_date, raw_description, amount) natural-key constraint
	// (kept for CSV/manual dedup) in a way the INSERT's ON CONFLICT below can't
	// catch, since Postgres only de-dupes against the named conflict target —
	// a collision against a *different* unique constraint is a hard error, not
	// a conflict it can route to DO UPDATE. Detect an exact natural-key match
	// first and re-key that row instead of attempting a doomed INSERT.
	var rekeyed bool
	err := s.db.QueryRowContext(ctx, `
		UPDATE transactions SET
			external_transaction_id = $5,
			external_provider       = $6,
			account_id              = $7,
			account_label           = $8,
			updated_at              = NOW()
		WHERE user_id = $1 AND trans_date = $2 AND raw_description = $3 AND amount = $4
		  AND external_transaction_id IS DISTINCT FROM $5
		RETURNING true`,
		p.UserID, p.TransDate, p.RawDescription, p.Amount,
		p.ExternalTransactionID, p.ExternalProvider, p.AccountID, p.AccountLabel,
	).Scan(&rekeyed)
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("rekey transaction: %w", err)
	}
	if rekeyed {
		return false, nil // updated an existing row in place, not a new insert
	}

	var inserted bool
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO transactions
			(user_id, trans_date, merchant, amount, category_id, account_id,
			 account_label, raw_description, external_transaction_id, external_provider, needs_review)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (user_id, external_transaction_id) WHERE external_transaction_id IS NOT NULL
		DO UPDATE SET
			trans_date     = EXCLUDED.trans_date,
			merchant       = EXCLUDED.merchant,
			amount         = EXCLUDED.amount,
			account_id     = EXCLUDED.account_id,
			account_label  = EXCLUDED.account_label,
			raw_description = EXCLUDED.raw_description,
			updated_at     = NOW()
		RETURNING (xmax = 0) AS inserted`,
		p.UserID, p.TransDate, p.Merchant, p.Amount, p.CategoryID, p.AccountID,
		p.AccountLabel, p.RawDescription, p.ExternalTransactionID, p.ExternalProvider, p.NeedsReview,
	).Scan(&inserted)
	if err != nil {
		return false, fmt.Errorf("upsert external transaction: %w", err)
	}
	return inserted, nil
}

// TryAdoptExternalMatch converts a single pre-existing manual/CSV transaction
// into a provider-sourced one by stamping the external identity and account
// onto it, when exactly one unmatched row matches (user, date, amount). This
// prevents a Plaid sync from inserting a second copy of a transaction the user
// already imported by CSV, while preserving that row's existing category.
//
// It deliberately adopts only when there's exactly one candidate: with two or
// more same-day/same-amount unmatched rows there's no safe way to know which is
// the true duplicate, so the caller inserts normally (any resulting duplicate
// is surfaced by the cleanup tool rather than risking a wrong merge).
// Returns true when a row was adopted, in which case the caller skips its insert.
func (s *TransactionStore) TryAdoptExternalMatch(ctx context.Context, p UpsertExternalParams) (bool, error) {
	// If this external transaction is already stored, there's nothing to adopt —
	// the caller's upsert will update it in place. Guards against a re-sync
	// stamping the same external id onto a second row (unique-index violation).
	var existing int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM transactions WHERE user_id = $1 AND external_transaction_id = $2`,
		p.UserID, p.ExternalTransactionID,
	).Scan(&existing); err != nil {
		return false, fmt.Errorf("check existing external id: %w", err)
	}
	if existing > 0 {
		return false, nil
	}

	var ids []uuid.UUID
	err := s.db.SelectContext(ctx, &ids, `
		SELECT id FROM transactions
		WHERE user_id = $1 AND trans_date = $2 AND amount = $3
		  AND external_transaction_id IS NULL
		LIMIT 2`,
		p.UserID, p.TransDate, p.Amount,
	)
	if err != nil {
		return false, fmt.Errorf("find adopt candidate: %w", err)
	}
	if len(ids) != 1 {
		return false, nil // no candidate, or ambiguous — don't guess
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE transactions SET
			external_transaction_id = $2,
			external_provider       = $3,
			account_id              = $4,
			account_label           = $5,
			updated_at              = NOW()
		WHERE id = $1`,
		ids[0], p.ExternalTransactionID, p.ExternalProvider, p.AccountID, p.AccountLabel,
	)
	if err != nil {
		return false, fmt.Errorf("adopt manual transaction: %w", err)
	}
	return true, nil
}

// PlaidDateRange reports the earliest transaction date and total count of
// Plaid-synced transactions for one account, so the UI can answer "how far
// back does my synced history actually go" — distinguishing a sync that
// succeeded but only found recent data from one that's silently behind.
// A nil EarliestDate means no Plaid transactions have synced for this account
// yet.
type PlaidDateRange struct {
	EarliestDate *time.Time `json:"earliestDate"`
	Count        int        `json:"count"`
}

func (s *TransactionStore) PlaidDateRange(ctx context.Context, userID, accountID uuid.UUID) (*PlaidDateRange, error) {
	var r PlaidDateRange
	err := s.db.QueryRowContext(ctx, `
		SELECT MIN(trans_date), COUNT(*)
		FROM transactions
		WHERE user_id = $1 AND account_id = $2 AND external_provider = 'plaid'`,
		userID, accountID,
	).Scan(&r.EarliestDate, &r.Count)
	if err != nil {
		return nil, fmt.Errorf("plaid date range: %w", err)
	}
	return &r, nil
}

// ListManualDuplicatesOfPlaid returns manual/CSV transactions (no external id)
// that have a Plaid-sourced counterpart with the same date and amount — i.e.
// the same real transaction imported twice from two sources. Used by the
// preview-first duplicate cleanup; the Plaid copy is authoritative and kept.
func (s *TransactionStore) ListManualDuplicatesOfPlaid(ctx context.Context, userID uuid.UUID) ([]models.Transaction, error) {
	type row struct {
		models.Transaction
		CatName *string `db:"cat_name"`
	}
	var rows []row
	err := s.db.SelectContext(ctx, &rows, `
		SELECT t.*, c.name AS cat_name
		FROM transactions t
		LEFT JOIN budget_categories c ON c.id = t.category_id
		WHERE t.user_id = $1
		  AND t.external_transaction_id IS NULL
		  AND EXISTS (
		      SELECT 1 FROM transactions p
		      WHERE p.user_id = t.user_id
		        AND p.external_provider = 'plaid'
		        AND p.trans_date = t.trans_date
		        AND p.amount = t.amount
		        AND p.id <> t.id
		  )
		ORDER BY t.trans_date DESC, t.merchant`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list manual duplicates: %w", err)
	}
	out := make([]models.Transaction, len(rows))
	for i, r := range rows {
		out[i] = r.Transaction
		if r.CatName != nil {
			out[i].CategoryName = *r.CatName
		}
	}
	return out, nil
}

// BulkDelete removes the given transactions owned by userID and returns the
// number deleted. Ids not owned by the user are silently skipped.
func (s *TransactionStore) BulkDelete(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM transactions WHERE user_id = $1 AND id = ANY($2)`,
		userID, pq.Array(ids),
	)
	if err != nil {
		return 0, fmt.Errorf("bulk delete transactions: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// RelabelByAccount updates the denormalized account_label on transactions
// linked to accountID, but only rows whose label still matches oldLabel — so a
// user's manual per-transaction label override (see UpdateTransaction) is left
// intact. Used to propagate an account rename to its existing transactions.
//
// Also catches unlinked rows (account_id IS NULL) whose label matches oldLabel
// exactly: CSV/manual imports never set account_id (see BulkImport), only a
// free-text account_label the user typed at import time — so a row imported
// before this account was Plaid-linked, labeled with what was then the
// account's name, can never be reached via account_id alone. Matching by
// label text for the unlinked case is the only way to catch it; the small
// risk (two different accounts having shared the exact same historical name)
// is accepted as a labeling nicety, not a data-integrity concern.
func (s *TransactionStore) RelabelByAccount(ctx context.Context, userID, accountID uuid.UUID, oldLabel, newLabel string) (int, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE transactions
		SET account_label = $4, updated_at = NOW()
		WHERE user_id = $1
		  AND (account_id = $2 OR account_id IS NULL)
		  AND account_label IS NOT DISTINCT FROM $3`,
		userID, accountID, oldLabel, newLabel,
	)
	if err != nil {
		return 0, fmt.Errorf("relabel transactions by account: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// DeleteByExternalID removes a provider-sourced transaction. Missing rows are
// not an error: a Plaid "removed" event may reference a pending transaction we
// never stored.
func (s *TransactionStore) DeleteByExternalID(ctx context.Context, userID uuid.UUID, provider, externalID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM transactions
		WHERE user_id = $1 AND external_provider = $2 AND external_transaction_id = $3`,
		userID, provider, externalID,
	)
	if err != nil {
		return fmt.Errorf("delete external transaction: %w", err)
	}
	return nil
}

type ListTransactionsParams struct {
	UserID      uuid.UUID
	AccountID   *uuid.UUID
	DateFrom    *time.Time
	DateTo      *time.Time
	CategoryIDs []uuid.UUID
	Search      string
	NeedsReview *bool
	Limit       int // 0 = no limit
}

// List returns transactions for a given user, sorted recent-first, with optional date range,
// category, and search filters.
func (s *TransactionStore) List(ctx context.Context, p ListTransactionsParams) ([]models.Transaction, error) {
	type row struct {
		models.Transaction
		CatName *string `db:"cat_name"`
	}

	args := []any{p.UserID}
	where := []string{"t.user_id = $1"}

	if p.AccountID != nil {
		args = append(args, *p.AccountID)
		where = append(where, fmt.Sprintf("t.account_id = $%d", len(args)))
	}
	if p.DateFrom != nil {
		args = append(args, *p.DateFrom)
		where = append(where, fmt.Sprintf("t.trans_date >= $%d", len(args)))
	}
	if p.DateTo != nil {
		args = append(args, *p.DateTo)
		where = append(where, fmt.Sprintf("t.trans_date <= $%d", len(args)))
	}
	if len(p.CategoryIDs) > 0 {
		args = append(args, pq.Array(p.CategoryIDs))
		where = append(where, fmt.Sprintf("t.category_id = ANY($%d)", len(args)))
	}
	if p.Search != "" {
		args = append(args, "%"+strings.ToUpper(p.Search)+"%")
		where = append(where, fmt.Sprintf("UPPER(t.merchant) LIKE $%d", len(args)))
	}
	if p.NeedsReview != nil {
		args = append(args, *p.NeedsReview)
		where = append(where, fmt.Sprintf("t.needs_review = $%d", len(args)))
	}

	q := fmt.Sprintf(`
		SELECT t.*, c.name AS cat_name
		FROM transactions t
		LEFT JOIN budget_categories c ON c.id = t.category_id
		WHERE %s
		ORDER BY t.trans_date DESC, t.created_at DESC`,
		strings.Join(where, " AND "),
	)
	if p.Limit > 0 {
		args = append(args, p.Limit)
		q += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	var rows []row
	if err := s.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, fmt.Errorf("list transactions: %w", err)
	}

	out := make([]models.Transaction, len(rows))
	for i, r := range rows {
		out[i] = r.Transaction
		if r.CatName != nil {
			out[i].CategoryName = *r.CatName
		}
	}
	return out, nil
}

type UpdateTransactionParams struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	CategoryID   *uuid.UUID
	Notes        *string
	AccountID    *uuid.UUID
	AccountLabel *string
}

func (s *TransactionStore) Update(ctx context.Context, p UpdateTransactionParams) (*models.Transaction, error) {
	type row struct {
		models.Transaction
		CatName *string `db:"cat_name"`
	}
	var r row
	err := s.db.QueryRowxContext(ctx, `
		UPDATE transactions SET
			category_id   = $3,
			notes         = $4,
			account_id    = $5,
			account_label = $6,
			needs_review  = false,
			updated_at    = NOW()
		WHERE id = $1 AND user_id = $2
		RETURNING transactions.*,
			(SELECT name FROM budget_categories WHERE id = $3) AS cat_name`,
		p.ID, p.UserID, p.CategoryID, p.Notes, p.AccountID, p.AccountLabel,
	).StructScan(&r)
	if err != nil {
		return nil, fmt.Errorf("update transaction: %w", err)
	}
	t := r.Transaction
	if r.CatName != nil {
		t.CategoryName = *r.CatName
	}
	return &t, nil
}

func (s *TransactionStore) Delete(ctx context.Context, id, userID uuid.UUID) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM transactions WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete transaction: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("transaction not found")
	}
	return nil
}

// Summary returns per-category spending for the given month plus a 3-month rolling average.
func (s *TransactionStore) Summary(ctx context.Context, userID uuid.UUID, year, month int) ([]models.BudgetSummaryRow, error) {
	// Compute the 3-month window ending at (year, month).
	type rawRow struct {
		CategoryID   uuid.UUID `db:"category_id"`
		CategoryName string    `db:"category_name"`
		Total        float64   `db:"total"`
		Count        int       `db:"count"`
		IsSelected   bool      `db:"is_selected"`
	}
	var rows []rawRow
	err := s.db.SelectContext(ctx, &rows, `
		WITH months AS (
			SELECT generate_series(
				date_trunc('month', make_date($2, $3, 1)) - INTERVAL '2 months',
				date_trunc('month', make_date($2, $3, 1)),
				'1 month'
			)::date AS month_start
		),
		categorised AS (
			SELECT
				t.category_id,
				date_trunc('month', t.trans_date)::date AS month_start,
				SUM(t.amount)                            AS total,
				COUNT(*)                                  AS cnt
			FROM transactions t
			JOIN months m ON date_trunc('month', t.trans_date)::date = m.month_start
			WHERE t.user_id = $1
			  AND t.amount > 0
			  AND (t.category_id IS NULL OR t.category_id NOT IN (
			      SELECT id FROM budget_categories
			      WHERE name IN ('Transfer', 'Income') AND is_system = true
			  ))
			GROUP BY t.category_id, date_trunc('month', t.trans_date)::date
		),
		selected_month AS (
			SELECT make_date($2, $3, 1)::date AS start
		)
		SELECT
			COALESCE(c.category_id, gen_random_uuid())          AS category_id,
			COALESCE(bc.name, 'Other')                          AS category_name,
			COALESCE(SUM(CASE WHEN c.month_start = sm.start THEN c.total ELSE 0 END), 0) AS total,
			COUNT(DISTINCT c.month_start)::int                  AS count,
			(c.month_start = sm.start)                          AS is_selected
		FROM categorised c
		CROSS JOIN selected_month sm
		LEFT JOIN budget_categories bc ON bc.id = c.category_id
		GROUP BY c.category_id, bc.name, c.month_start = sm.start
		ORDER BY total DESC`,
		userID, year, month,
	)
	if err != nil {
		return nil, fmt.Errorf("budget summary: %w", err)
	}

	// Aggregate into per-category rows with 3-month average.
	type agg struct {
		name        string
		total       float64
		monthTotals []float64
	}
	byID := map[uuid.UUID]*agg{}
	for _, r := range rows {
		a, ok := byID[r.CategoryID]
		if !ok {
			a = &agg{name: r.CategoryName}
			byID[r.CategoryID] = a
		}
		if r.IsSelected {
			a.total = r.Total
		}
		if r.Total > 0 {
			a.monthTotals = append(a.monthTotals, r.Total)
		}
	}

	out := make([]models.BudgetSummaryRow, 0, len(byID))
	for id, a := range byID {
		var avg float64
		if len(a.monthTotals) > 0 {
			for _, v := range a.monthTotals {
				avg += v
			}
			avg /= float64(len(a.monthTotals))
		}
		out = append(out, models.BudgetSummaryRow{
			CategoryID:   id,
			CategoryName: a.name,
			Total:        a.total,
			MonthAvg:     avg,
			Count:        len(a.monthTotals),
		})
	}
	return out, nil
}

// FindCategoryByMerchant returns the most recently used category ID for a given
// normalized merchant name, or nil if the merchant has never been categorized.
func (s *TransactionStore) FindCategoryByMerchant(ctx context.Context, userID uuid.UUID, merchant string) *uuid.UUID {
	var catID uuid.UUID
	err := s.db.QueryRowContext(ctx, `
		SELECT category_id
		FROM transactions
		WHERE user_id = $1
		  AND UPPER(merchant) = $2
		  AND category_id IS NOT NULL
		  AND needs_review = false
		ORDER BY trans_date DESC
		LIMIT 1`,
		userID, strings.ToUpper(merchant),
	).Scan(&catID)
	if err != nil {
		return nil
	}
	return &catID
}

// MonthlyBreakdown returns per-month per-category spending totals for a full year,
// used to build the annual stacked bar chart.
func (s *TransactionStore) MonthlyBreakdown(ctx context.Context, userID uuid.UUID, year int) ([]models.MonthlyBreakdownRow, error) {
	rows := []models.MonthlyBreakdownRow{}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT
			EXTRACT(MONTH FROM t.trans_date)::int AS month,
			COALESCE(bc.name, 'Other')            AS category_name,
			SUM(t.amount)                         AS total
		FROM transactions t
		LEFT JOIN budget_categories bc ON bc.id = t.category_id
		WHERE t.user_id = $1
		  AND EXTRACT(YEAR FROM t.trans_date) = $2
		  AND t.amount > 0
		  AND (t.category_id IS NULL OR t.category_id NOT IN (
		      SELECT id FROM budget_categories
		      WHERE name IN ('Transfer', 'Income') AND is_system = true
		  ))
		GROUP BY EXTRACT(MONTH FROM t.trans_date), bc.name
		ORDER BY month, total DESC`,
		userID, year,
	)
	if err != nil {
		return nil, fmt.Errorf("monthly breakdown: %w", err)
	}
	return rows, nil
}

// AnnualSummary returns per-category spending totals for the full given year.
func (s *TransactionStore) AnnualSummary(ctx context.Context, userID uuid.UUID, year int) ([]models.BudgetSummaryRow, error) {
	type rawRow struct {
		CategoryID   *uuid.UUID `db:"category_id"`
		CategoryName string     `db:"category_name"`
		Total        float64    `db:"total"`
	}
	var rows []rawRow
	err := s.db.SelectContext(ctx, &rows, `
		SELECT
			t.category_id,
			COALESCE(bc.name, 'Other') AS category_name,
			SUM(t.amount)              AS total
		FROM transactions t
		LEFT JOIN budget_categories bc ON bc.id = t.category_id
		WHERE t.user_id = $1
		  AND EXTRACT(YEAR FROM t.trans_date) = $2
		  AND t.amount > 0
		  AND (t.category_id IS NULL OR t.category_id NOT IN (
		      SELECT id FROM budget_categories
		      WHERE name IN ('Transfer', 'Income') AND is_system = true
		  ))
		GROUP BY t.category_id, bc.name
		ORDER BY total DESC`,
		userID, year,
	)
	if err != nil {
		return nil, fmt.Errorf("annual budget summary: %w", err)
	}

	out := make([]models.BudgetSummaryRow, len(rows))
	for i, r := range rows {
		var id uuid.UUID
		if r.CategoryID != nil {
			id = *r.CategoryID
		}
		out[i] = models.BudgetSummaryRow{
			CategoryID:   id,
			CategoryName: r.CategoryName,
			Total:        r.Total,
		}
	}
	return out, nil
}

// MonthlyAvgExpenses returns the average monthly spending over the past 3 complete months.
// Used by the forecast service when use_budget_for_expenses is enabled.
func (s *TransactionStore) MonthlyAvgExpenses(ctx context.Context, userID uuid.UUID) (float64, error) {
	var avg float64
	err := s.db.QueryRowContext(ctx, `
		WITH recent AS (
			SELECT date_trunc('month', trans_date) AS m, SUM(amount) AS total
			FROM transactions
			WHERE user_id = $1
			  AND amount > 0
			  AND trans_date < date_trunc('month', NOW())
			  AND (category_id IS NULL OR category_id NOT IN (
			      SELECT id FROM budget_categories
			      WHERE name IN ('Transfer', 'Income') AND is_system = true
			  ))
			GROUP BY date_trunc('month', trans_date)
			ORDER BY m DESC
			LIMIT 3
		)
		SELECT COALESCE(AVG(total), 0) FROM recent`,
		userID,
	).Scan(&avg)
	if err != nil {
		return 0, fmt.Errorf("monthly avg expenses: %w", err)
	}
	return avg, nil
}

// MonthlyIncomeTotal returns the sum of Income-categorized transactions for a given month.
func (s *TransactionStore) MonthlyIncomeTotal(ctx context.Context, userID uuid.UUID, year, month int) (float64, error) {
	var total float64
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(t.amount), 0)
		FROM transactions t
		JOIN budget_categories bc ON bc.id = t.category_id
		WHERE t.user_id = $1
		  AND date_trunc('month', t.trans_date)::date = make_date($2, $3, 1)::date
		  AND bc.name = 'Income'
		  AND bc.is_system = true
		  AND t.amount > 0`,
		userID, year, month,
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("monthly income total: %w", err)
	}
	return total, nil
}

// MonthlyIncomePoint is one month's income total.
type MonthlyIncomePoint struct {
	Month int     `db:"month" json:"month"`
	Total float64 `db:"total" json:"total"`
}

// MonthlyIncomeBreakdown returns per-month income totals for a given year.
func (s *TransactionStore) MonthlyIncomeBreakdown(ctx context.Context, userID uuid.UUID, year int) ([]MonthlyIncomePoint, error) {
	rows, err := s.db.QueryxContext(ctx, `
		SELECT EXTRACT(MONTH FROM t.trans_date)::int AS month,
		       COALESCE(SUM(t.amount), 0)            AS total
		FROM transactions t
		JOIN budget_categories bc ON bc.id = t.category_id
		WHERE t.user_id = $1
		  AND EXTRACT(YEAR FROM t.trans_date) = $2
		  AND bc.name = 'Income'
		  AND bc.is_system = true
		  AND t.amount > 0
		GROUP BY month
		ORDER BY month`,
		userID, year,
	)
	if err != nil {
		return nil, fmt.Errorf("monthly income breakdown: %w", err)
	}
	defer rows.Close()
	out := make([]MonthlyIncomePoint, 0)
	for rows.Next() {
		var p MonthlyIncomePoint
		if err := rows.StructScan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// ApplyVendorCategory sets category_id and clears needs_review for all unreviewed transactions
// from the same merchant, except the one already explicitly updated.
func (s *TransactionStore) ApplyVendorCategory(ctx context.Context, userID uuid.UUID, merchant string, excludeID uuid.UUID, categoryID uuid.UUID) (int, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE transactions SET
			category_id  = $4,
			needs_review = false,
			updated_at   = NOW()
		WHERE user_id = $1
		  AND UPPER(merchant) = $2
		  AND id != $3
		  AND needs_review = true`,
		userID, strings.ToUpper(merchant), excludeID, categoryID,
	)
	if err != nil {
		return 0, fmt.Errorf("apply vendor category: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// NeedsReviewCount returns the count of transactions with needs_review = true for the given user.
func (s *TransactionStore) NeedsReviewCount(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM transactions WHERE user_id = $1 AND needs_review = true`,
		userID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("needs review count: %w", err)
	}
	return n, nil
}

// PendingAutoCategorize is a merchant with uncategorized, review-flagged
// transactions that the background categorizer hasn't attempted yet.
type PendingAutoCategorize struct {
	UserID   uuid.UUID `db:"user_id"`
	Merchant string    `db:"merchant"`
}

// ListPendingAutoCategorize returns up to limit distinct (user, merchant)
// pairs awaiting a background category suggestion.
func (s *TransactionStore) ListPendingAutoCategorize(ctx context.Context, limit int) ([]PendingAutoCategorize, error) {
	var rows []PendingAutoCategorize
	err := s.db.SelectContext(ctx, &rows, `
		SELECT DISTINCT user_id, merchant
		FROM transactions
		WHERE needs_review AND category_id IS NULL AND auto_categorized_at IS NULL
		ORDER BY user_id, merchant
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending auto-categorize: %w", err)
	}
	return rows, nil
}

// ApplyAutoCategory records a background suggestion for every still-
// uncategorized transaction from this merchant. categoryID may be nil when the
// LLM had no usable answer; the rows are then just marked as attempted.
// Transactions stay needs_review so the user confirms the suggestion, and rows
// the user categorized in the meantime are never touched.
func (s *TransactionStore) ApplyAutoCategory(ctx context.Context, userID uuid.UUID, merchant string, categoryID *uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE transactions
		SET category_id = $3, auto_categorized_at = NOW(), updated_at = NOW()
		WHERE user_id = $1 AND merchant = $2
		  AND needs_review AND category_id IS NULL AND auto_categorized_at IS NULL`,
		userID, merchant, categoryID)
	if err != nil {
		return fmt.Errorf("apply auto category: %w", err)
	}
	return nil
}
