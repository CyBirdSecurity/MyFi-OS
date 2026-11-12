package services

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
)

type ImportResult struct {
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors"`
}

// ImportCSV parses a CSV and bulk-upserts balance snapshots.
//
// Expected CSV format (header row required):
//
//	account_name, snapshot_date, balance [, notes]
//
// account_name must match an existing account name for the user (case-insensitive).
// snapshot_date format: YYYY-MM-DD
// balance: numeric (negative for liabilities)
type ImporterService struct {
	accounts  *db.AccountStore
	snapshots *db.SnapshotStore
}

func NewImporterService(accounts *db.AccountStore, snapshots *db.SnapshotStore) *ImporterService {
	return &ImporterService{accounts: accounts, snapshots: snapshots}
}

func (s *ImporterService) ImportCSV(ctx context.Context, userID uuid.UUID, r io.Reader) (*ImportResult, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true

	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read csv header: %w", err)
	}

	// Normalize headers.
	colIdx := make(map[string]int)
	for i, h := range headers {
		colIdx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, required := range []string{"account_name", "snapshot_date", "balance"} {
		if _, ok := colIdx[required]; !ok {
			return nil, fmt.Errorf("missing required column: %s", required)
		}
	}

	// Build account name → ID map.
	accs, err := s.accounts.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	accByName := make(map[string]uuid.UUID, len(accs))
	for _, a := range accs {
		accByName[strings.ToLower(a.Name)] = a.ID
	}

	result := &ImportResult{Errors: []string{}}
	var toInsert []db.UpsertSnapshotParams

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read csv records: %w", err)
	}

	for i, row := range records {
		lineNum := i + 2 // 1-based, accounting for header

		accountName := strings.TrimSpace(row[colIdx["account_name"]])
		accountID, ok := accByName[strings.ToLower(accountName)]
		if !ok {
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: unknown account %q", lineNum, accountName))
			result.Skipped++
			continue
		}

		dateStr := strings.TrimSpace(row[colIdx["snapshot_date"]])
		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: invalid date %q (want YYYY-MM-DD)", lineNum, dateStr))
			result.Skipped++
			continue
		}

		balanceStr := strings.TrimSpace(row[colIdx["balance"]])
		balanceStr = strings.ReplaceAll(balanceStr, ",", "")
		balanceStr = strings.TrimPrefix(balanceStr, "$")
		balance, err := strconv.ParseFloat(balanceStr, 64)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: invalid balance %q", lineNum, balanceStr))
			result.Skipped++
			continue
		}

		var notes *string
		if idx, ok := colIdx["notes"]; ok && idx < len(row) {
			if n := strings.TrimSpace(row[idx]); n != "" {
				notes = &n
			}
		}

		toInsert = append(toInsert, db.UpsertSnapshotParams{
			AccountID:    accountID,
			UserID:       userID,
			SnapshotDate: date,
			Balance:      balance,
			Notes:        notes,
			Source:       "import",
		})
	}

	if len(toInsert) > 0 {
		count, err := s.snapshots.BulkUpsert(ctx, toInsert)
		if err != nil {
			return nil, fmt.Errorf("bulk upsert: %w", err)
		}
		result.Imported = count
	}

	return result, nil
}
