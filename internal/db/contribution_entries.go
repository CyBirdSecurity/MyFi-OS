package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type ContributionEntryStore struct{ db *sqlx.DB }

func NewContributionEntryStore(db *sqlx.DB) *ContributionEntryStore {
	return &ContributionEntryStore{db: db}
}

func withSynced(entries []models.ContributionEntry) []models.ContributionEntry {
	for i := range entries {
		entries[i].IsSynced = entries[i].ExternalID != nil
	}
	return entries
}

// ListByAccount returns an account's entries dated on or after since, newest first.
func (s *ContributionEntryStore) ListByAccount(ctx context.Context, accountID, userID uuid.UUID, since time.Time) ([]models.ContributionEntry, error) {
	var out []models.ContributionEntry
	err := s.db.SelectContext(ctx, &out, `
		SELECT * FROM contribution_entries
		WHERE account_id = $1 AND user_id = $2 AND entry_date >= $3
		ORDER BY entry_date DESC, created_at DESC`,
		accountID, userID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("list contribution entries: %w", err)
	}
	return withSynced(out), nil
}

// ListByUser returns all of a user's entries dated on or after since.
func (s *ContributionEntryStore) ListByUser(ctx context.Context, userID uuid.UUID, since time.Time) ([]models.ContributionEntry, error) {
	var out []models.ContributionEntry
	err := s.db.SelectContext(ctx, &out, `
		SELECT * FROM contribution_entries
		WHERE user_id = $1 AND entry_date >= $2
		ORDER BY entry_date`,
		userID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("list user contribution entries: %w", err)
	}
	return withSynced(out), nil
}

type ContributionEntryParams struct {
	AccountID   uuid.UUID
	UserID      uuid.UUID
	EntryDate   time.Time
	Amount      float64
	Source      string
	Description *string
	ExternalID  *string
}

// CreateManual records a contribution the user logged by hand.
func (s *ContributionEntryStore) CreateManual(ctx context.Context, p ContributionEntryParams) (*models.ContributionEntry, error) {
	var out models.ContributionEntry
	err := s.db.QueryRowxContext(ctx, `
		INSERT INTO contribution_entries (account_id, user_id, entry_date, amount, source, description)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING *`,
		p.AccountID, p.UserID, p.EntryDate, p.Amount, p.Source, p.Description,
	).StructScan(&out)
	if err != nil {
		return nil, fmt.Errorf("create contribution entry: %w", err)
	}
	return &out, nil
}

// UpdateManual changes a hand-logged entry; synced entries are left alone.
func (s *ContributionEntryStore) UpdateManual(ctx context.Context, id uuid.UUID, p ContributionEntryParams) (*models.ContributionEntry, error) {
	var out models.ContributionEntry
	err := s.db.QueryRowxContext(ctx, `
		UPDATE contribution_entries
		SET entry_date = $4, amount = $5, source = $6, description = $7
		WHERE id = $1 AND account_id = $2 AND user_id = $3 AND external_id IS NULL
		RETURNING *`,
		id, p.AccountID, p.UserID, p.EntryDate, p.Amount, p.Source, p.Description,
	).StructScan(&out)
	if err != nil {
		return nil, fmt.Errorf("update contribution entry: %w", err)
	}
	return &out, nil
}

// SetSource corrects whose money a synced entry was, and keeps that
// correction through later syncs.
func (s *ContributionEntryStore) SetSource(ctx context.Context, id, accountID, userID uuid.UUID, source string) (*models.ContributionEntry, error) {
	var out models.ContributionEntry
	err := s.db.QueryRowxContext(ctx, `
		UPDATE contribution_entries
		SET source = $4, source_edited = TRUE
		WHERE id = $1 AND account_id = $2 AND user_id = $3
		RETURNING *`,
		id, accountID, userID, source,
	).StructScan(&out)
	if err != nil {
		return nil, fmt.Errorf("set contribution entry source: %w", err)
	}
	out.IsSynced = out.ExternalID != nil
	return &out, nil
}

// DeleteManual removes a hand-logged entry.
func (s *ContributionEntryStore) DeleteManual(ctx context.Context, id, accountID, userID uuid.UUID) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM contribution_entries
		WHERE id = $1 AND account_id = $2 AND user_id = $3 AND external_id IS NULL`,
		id, accountID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete contribution entry: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("contribution entry not found")
	}
	return nil
}

// UpsertSynced writes entries from Plaid, keyed by (account, external id). A
// source the user corrected is kept.
func (s *ContributionEntryStore) UpsertSynced(ctx context.Context, entries []ContributionEntryParams) error {
	for _, p := range entries {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO contribution_entries (account_id, user_id, entry_date, amount, source, description, external_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (account_id, external_id) WHERE external_id IS NOT NULL DO UPDATE SET
				entry_date  = EXCLUDED.entry_date,
				amount      = EXCLUDED.amount,
				description = EXCLUDED.description,
				source      = CASE WHEN contribution_entries.source_edited
				                   THEN contribution_entries.source ELSE EXCLUDED.source END`,
			p.AccountID, p.UserID, p.EntryDate, p.Amount, p.Source, p.Description, p.ExternalID,
		)
		if err != nil {
			return fmt.Errorf("upsert synced contribution entry: %w", err)
		}
	}
	return nil
}
