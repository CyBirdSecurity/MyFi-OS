package db

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type ImportLogStore struct{ db *sqlx.DB }

func NewImportLogStore(db *sqlx.DB) *ImportLogStore {
	return &ImportLogStore{db: db}
}

type CreateImportLogParams struct {
	UserID     uuid.UUID
	ImportType string // csv_snapshot | ofx_holdings | csv_transactions
	Filename   string
	Status     string // success | partial | failed
	RowCount   int
	Skipped    int
	ErrorCount int
}

func (s *ImportLogStore) Create(ctx context.Context, p CreateImportLogParams) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO import_logs (user_id, import_type, filename, status, row_count, skipped, error_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		p.UserID, p.ImportType, p.Filename, p.Status, p.RowCount, p.Skipped, p.ErrorCount,
	)
	return err
}

func (s *ImportLogStore) List(ctx context.Context, userID uuid.UUID) ([]models.ImportLog, error) {
	var logs []models.ImportLog
	err := s.db.SelectContext(ctx, &logs, `
		SELECT id, user_id, import_type, filename, status, row_count, skipped, error_count, imported_at
		FROM import_logs
		WHERE user_id = $1
		ORDER BY imported_at DESC
		LIMIT 50`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	return logs, nil
}
