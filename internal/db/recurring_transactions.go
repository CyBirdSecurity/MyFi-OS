package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type CreateRecurringParams struct {
	UserID       uuid.UUID
	Merchant     string
	Amount       float64
	CategoryID   *uuid.UUID
	AccountID    *uuid.UUID
	AccountLabel *string
	Notes        *string
	Frequency    string
	StartDate    time.Time
}

func (s *TransactionStore) CreateRecurring(ctx context.Context, p CreateRecurringParams) (*models.RecurringTransaction, error) {
	var r models.RecurringTransaction
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO recurring_transactions
			(user_id, merchant, amount, category_id, account_id, account_label, notes, frequency, start_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING *`,
		p.UserID, p.Merchant, p.Amount, p.CategoryID, p.AccountID, p.AccountLabel, p.Notes, p.Frequency, p.StartDate,
	).StructScan(&r)
	if err != nil {
		return nil, fmt.Errorf("create recurring transaction: %w", err)
	}
	return &r, nil
}

func (s *TransactionStore) ListRecurring(ctx context.Context, userID uuid.UUID) ([]models.RecurringTransaction, error) {
	type row struct {
		models.RecurringTransaction
		CatName *string `db:"cat_name"`
	}
	var rows []row
	err := s.db.SelectContext(ctx, &rows, `
		SELECT r.*, bc.name AS cat_name
		FROM recurring_transactions r
		LEFT JOIN budget_categories bc ON bc.id = r.category_id
		WHERE r.user_id = $1
		ORDER BY r.created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("list recurring transactions: %w", err)
	}
	out := make([]models.RecurringTransaction, 0, len(rows))
	for _, rw := range rows {
		r := rw.RecurringTransaction
		if rw.CatName != nil {
			r.CategoryName = *rw.CatName
		}
		r.NextDate = r.Occurrence(r.GeneratedCount)
		out = append(out, r)
	}
	return out, nil
}

// DeleteRecurring stops future occurrences. Transactions already added stay.
func (s *TransactionStore) DeleteRecurring(ctx context.Context, id, userID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM recurring_transactions WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete recurring transaction: %w", err)
	}
	return nil
}

// GenerateDueRecurring adds every occurrence dated on or before today that
// hasn't been added yet, for one user (userID non-nil) or everyone. Rules are
// locked for the duration so concurrent runs can't add an occurrence twice.
// Returns the number of transactions inserted.
func (s *TransactionStore) GenerateDueRecurring(ctx context.Context, userID *uuid.UUID, today time.Time) (int, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	var rules []models.RecurringTransaction
	if userID != nil {
		err = tx.SelectContext(ctx, &rules, `SELECT * FROM recurring_transactions WHERE user_id = $1 FOR UPDATE`, *userID)
	} else {
		err = tx.SelectContext(ctx, &rules, `SELECT * FROM recurring_transactions FOR UPDATE`)
	}
	if err != nil {
		return 0, fmt.Errorf("load recurring transactions: %w", err)
	}

	inserted := 0
	for _, r := range rules {
		count := r.GeneratedCount
		for d := r.Occurrence(count); !d.After(today); d = r.Occurrence(count) {
			// ON CONFLICT: the same transaction already exists (e.g. entered by
			// hand); count the occurrence as done rather than duplicating it.
			res, err := tx.ExecContext(ctx, `
				INSERT INTO transactions
					(user_id, trans_date, merchant, amount, category_id, account_id, account_label, raw_description, notes, needs_review)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $3, $8, false)
				ON CONFLICT DO NOTHING`,
				r.UserID, d, r.Merchant, r.Amount, r.CategoryID, r.AccountID, r.AccountLabel, r.Notes)
			if err != nil {
				return 0, fmt.Errorf("insert recurring occurrence: %w", err)
			}
			n, _ := res.RowsAffected()
			inserted += int(n)
			count++
		}
		if count != r.GeneratedCount {
			if _, err := tx.ExecContext(ctx,
				`UPDATE recurring_transactions SET generated_count = $1 WHERE id = $2`, count, r.ID); err != nil {
				return 0, fmt.Errorf("update recurring progress: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return inserted, nil
}
