package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type PlaidItemStore struct{ db *sqlx.DB }

func NewPlaidItemStore(db *sqlx.DB) *PlaidItemStore { return &PlaidItemStore{db: db} }

type CreatePlaidItemParams struct {
	UserID               uuid.UUID
	ItemID               string
	AccessTokenEncrypted string
	InstitutionID        *string
	InstitutionName      *string
}

func (s *PlaidItemStore) Create(ctx context.Context, p CreatePlaidItemParams) (*models.PlaidItem, error) {
	var item models.PlaidItem
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO plaid_items (user_id, item_id, access_token_encrypted, institution_id, institution_name)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING *`,
		p.UserID, p.ItemID, p.AccessTokenEncrypted, p.InstitutionID, p.InstitutionName,
	).StructScan(&item)
	if err != nil {
		return nil, fmt.Errorf("create plaid item: %w", err)
	}
	return &item, nil
}

// GetByItemID returns the item with the given Plaid item_id owned by userID.
func (s *PlaidItemStore) GetByItemID(ctx context.Context, userID uuid.UUID, itemID string) (*models.PlaidItem, error) {
	var item models.PlaidItem
	err := s.db.QueryRowxContext(ctx,
		`SELECT * FROM plaid_items WHERE user_id = $1 AND item_id = $2`,
		userID, itemID,
	).StructScan(&item)
	if err != nil {
		return nil, fmt.Errorf("get plaid item: %w", err)
	}
	return &item, nil
}

func (s *PlaidItemStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]models.PlaidItem, error) {
	var items []models.PlaidItem
	err := s.db.SelectContext(ctx, &items,
		`SELECT * FROM plaid_items WHERE user_id = $1 ORDER BY created_at`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list plaid items: %w", err)
	}
	return items, nil
}

// ListUnlinked returns Items older than minAge that have no linked MyFi
// account. These are connections whose account-mapping step was abandoned or
// failed; they serve no purpose but still count against the Plaid plan's limit.
func (s *PlaidItemStore) ListUnlinked(ctx context.Context, minAge time.Duration) ([]models.PlaidItem, error) {
	var items []models.PlaidItem
	err := s.db.SelectContext(ctx, &items, `
		SELECT i.* FROM plaid_items i
		WHERE i.created_at < NOW() - make_interval(secs => $1)
		  AND NOT EXISTS (
		      SELECT 1 FROM accounts a
		      WHERE a.user_id = i.user_id
		        AND a.external_item_id = i.item_id
		        AND a.deleted_at IS NULL)
		ORDER BY i.created_at`, minAge.Seconds())
	if err != nil {
		return nil, fmt.Errorf("list unlinked plaid items: %w", err)
	}
	return items, nil
}

// ListAllActive returns every active item across all users, for the daily
// sync scheduler.
func (s *PlaidItemStore) ListAllActive(ctx context.Context) ([]models.PlaidItem, error) {
	var items []models.PlaidItem
	err := s.db.SelectContext(ctx, &items,
		`SELECT * FROM plaid_items WHERE status = 'active' ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("list active plaid items: %w", err)
	}
	return items, nil
}

// RecordSyncResult stamps last_synced_at and records the sync outcome.
// A nil syncErr marks the item active and clears any previous error.
func (s *PlaidItemStore) RecordSyncResult(ctx context.Context, id uuid.UUID, syncErr *string) error {
	var err error
	if syncErr == nil {
		_, err = s.db.ExecContext(ctx, `
			UPDATE plaid_items
			SET last_synced_at = NOW(), last_sync_error = NULL,
			    status = 'active', error_code = NULL, updated_at = NOW()
			WHERE id = $1`, id)
	} else {
		_, err = s.db.ExecContext(ctx, `
			UPDATE plaid_items
			SET last_synced_at = NOW(), last_sync_error = $2, status = 'error', updated_at = NOW()
			WHERE id = $1`, id, *syncErr)
	}
	if err != nil {
		return fmt.Errorf("record sync result: %w", err)
	}
	return nil
}

// SetStatus updates the item's status and error code (e.g. login_required on
// ITEM_LOGIN_REQUIRED).
func (s *PlaidItemStore) SetStatus(ctx context.Context, id uuid.UUID, status, errorCode string) error {
	var ec *string
	if errorCode != "" {
		ec = &errorCode
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE plaid_items SET status = $2, error_code = $3, updated_at = NOW() WHERE id = $1`,
		id, status, ec,
	)
	if err != nil {
		return fmt.Errorf("set plaid item status: %w", err)
	}
	return nil
}

// SetBackfillRequested marks that a historical backfill was kicked off for
// this item, distinguishing "in progress" from "never requested" while
// backfilled_at is still null.
func (s *PlaidItemStore) SetBackfillRequested(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE plaid_items SET backfill_requested = TRUE, updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("set backfill requested: %w", err)
	}
	return nil
}

// SetBackfilledAt marks the historical backfill as completed.
func (s *PlaidItemStore) SetBackfilledAt(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE plaid_items SET backfilled_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("set backfilled_at: %w", err)
	}
	return nil
}

// SetTransactionsCursor persists the /transactions/sync cursor after a
// completed transaction sync so the next run resumes from where it stopped.
func (s *PlaidItemStore) SetTransactionsCursor(ctx context.Context, id uuid.UUID, cursor string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE plaid_items SET transactions_cursor = $2, updated_at = NOW() WHERE id = $1`,
		id, cursor,
	)
	if err != nil {
		return fmt.Errorf("set transactions cursor: %w", err)
	}
	return nil
}

func (s *PlaidItemStore) Delete(ctx context.Context, id, userID uuid.UUID) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM plaid_items WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete plaid item: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("plaid item not found")
	}
	return nil
}

// ClaimInvestmentsRefresh atomically records an investments refresh for the
// Item unless one was already claimed within window. It reports whether the
// caller may go ahead; /investments/refresh is billed, so only the winner of
// the claim should call it.
func (s *PlaidItemStore) ClaimInvestmentsRefresh(ctx context.Context, id uuid.UUID, window time.Duration) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE plaid_items SET investments_refresh_at = NOW()
		WHERE id = $1 AND (investments_refresh_at IS NULL OR investments_refresh_at <= NOW() - make_interval(secs => $2))`,
		id, window.Seconds())
	if err != nil {
		return false, fmt.Errorf("claim investments refresh: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("claim investments refresh: %w", err)
	}
	return n == 1, nil
}
