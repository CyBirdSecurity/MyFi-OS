package api

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// resolveTransactionAccount validates that accountIDStr (if non-empty) is a real
// account owned by userID, returning its id and current name (used as the
// denormalized account_label so display code never needs a live join). An
// empty accountIDStr means "no account" and returns (nil, nil, true). Writes
// its own error response and returns ok=false on an invalid or foreign id.
func (s *Server) resolveTransactionAccount(w http.ResponseWriter, r *http.Request, userID uuid.UUID, accountIDStr string) (accountID *uuid.UUID, accountLabel *string, ok bool) {
	if accountIDStr == "" {
		return nil, nil, true
	}
	id, err := uuid.Parse(accountIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION", "invalid accountId")
		return nil, nil, false
	}
	acc, err := s.accounts.Get(r.Context(), id, userID)
	if err != nil || acc == nil {
		writeError(w, http.StatusBadRequest, "VALIDATION", "account not found")
		return nil, nil, false
	}
	return &id, &acc.Name, true
}

func (s *Server) handleListBudgetCategories(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	cats, err := s.budgetCategories.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cats)
}

func (s *Server) handleCreateBudgetCategory(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	var body struct {
		Name string `json:"name"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION", "name is required")
		return
	}
	cat, err := s.budgetCategories.Create(r.Context(), userID, body.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, cat)
}

// handleBudgetImportPreview parses an uploaded CSV and returns classified transactions
// without persisting them.
func (s *Server) handleBudgetImportPreview(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "failed to parse multipart form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "file field required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "failed to read file")
		return
	}

	parsed, err := s.csvImport.Parse(r.Context(), userIDFromCtx(r), data, header.Filename)
	if err != nil {
		writeError(w, http.StatusBadRequest, "PARSE_ERROR", err.Error())
		return
	}

	type previewRow struct {
		TransDate      string  `json:"transDate"`
		Merchant       string  `json:"merchant"`
		RawDescription string  `json:"rawDescription"`
		Amount         float64 `json:"amount"`
		SourceCategory string  `json:"sourceCategory"`
		CategoryID     *string `json:"categoryId"`
		CategoryName   string  `json:"categoryName"`
		NeedsReview    bool    `json:"needsReview"`
	}

	rows := make([]previewRow, len(parsed))
	for i, p := range parsed {
		var catIDStr *string
		if p.CategoryID != nil {
			v := p.CategoryID.String()
			catIDStr = &v
		}
		rows[i] = previewRow{
			TransDate:      p.TransDate.Format("2006-01-02"),
			Merchant:       p.Merchant,
			RawDescription: p.RawDescription,
			Amount:         p.Amount,
			SourceCategory: p.SourceCategory,
			CategoryID:     catIDStr,
			CategoryName:   p.CategoryName,
			NeedsReview:    p.NeedsReview,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"transactions": rows,
		"filename":     header.Filename,
	})
}

// handleBudgetImportConfirm saves the (possibly user-adjusted) transactions from a preview.
func (s *Server) handleBudgetImportConfirm(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)

	var body struct {
		Filename     string `json:"filename"`
		AccountID    string `json:"accountId"`
		Transactions []struct {
			TransDate      string  `json:"transDate"`
			Merchant       string  `json:"merchant"`
			RawDescription string  `json:"rawDescription"`
			Amount         float64 `json:"amount"`
			CategoryID     *string `json:"categoryId"`
			NeedsReview    bool    `json:"needsReview"`
		} `json:"transactions"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}

	accountID, accountLabel, ok := s.resolveTransactionAccount(w, r, userID, body.AccountID)
	if !ok {
		return
	}

	txns := make([]models.Transaction, 0, len(body.Transactions))
	for _, t := range body.Transactions {
		date, err := time.Parse("2006-01-02", t.TransDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", fmt.Sprintf("invalid date: %s", t.TransDate))
			return
		}
		var catID *uuid.UUID
		if t.CategoryID != nil {
			id, err := uuid.Parse(*t.CategoryID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "VALIDATION", fmt.Sprintf("invalid categoryId: %s", *t.CategoryID))
				return
			}
			catID = &id
		}
		fn := body.Filename
		txns = append(txns, models.Transaction{
			UserID:         userID,
			TransDate:      date,
			Merchant:       t.Merchant,
			RawDescription: t.RawDescription,
			Amount:         t.Amount,
			CategoryID:     catID,
			AccountID:      accountID,
			AccountLabel:   accountLabel,
			SourceFile:     &fn,
			NeedsReview:    t.NeedsReview,
		})
	}

	result, err := s.transactions.BulkImport(r.Context(), txns)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	if result.Inserted > 0 {
		s.notifyAutoCategorize()
	}

	status := "success"
	if result.Inserted == 0 {
		status = "failed"
	} else if result.Skipped > 0 || result.Failed > 0 {
		status = "partial"
	}
	_ = s.importLogs.Create(r.Context(), db.CreateImportLogParams{
		UserID:     userID,
		ImportType: "csv_transactions",
		Filename:   body.Filename,
		Status:     status,
		RowCount:   result.Inserted,
		Skipped:    result.Skipped,
		ErrorCount: result.Failed,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"inserted": result.Inserted,
		"skipped":  result.Skipped,
		"failed":   result.Failed,
	})
}

// handleBudgetImportQuick parses an uploaded CSV using rules-only classification (no LLM),
// saves the transactions immediately, and returns import stats + the new total review count.
func (s *Server) handleBudgetImportQuick(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "failed to parse multipart form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "file field required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "failed to read file")
		return
	}

	userID := userIDFromCtx(r)

	accountID, accountLabel, ok := s.resolveTransactionAccount(w, r, userID, r.FormValue("accountId"))
	if !ok {
		return
	}
	label := ""
	if accountLabel != nil {
		label = *accountLabel
	}

	parsed, err := s.csvImport.ParseQuick(r.Context(), userID, data, header.Filename)
	if err != nil {
		writeError(w, http.StatusBadRequest, "PARSE_ERROR", err.Error())
		return
	}

	txns := s.csvImport.ToTransactions(parsed, userID, header.Filename, accountID, label)
	result, err := s.transactions.BulkImport(r.Context(), txns)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	if result.Inserted > 0 {
		s.notifyAutoCategorize()
	}

	status := "success"
	if result.Inserted == 0 {
		status = "failed"
	} else if result.Skipped > 0 || result.Failed > 0 {
		status = "partial"
	}
	_ = s.importLogs.Create(r.Context(), db.CreateImportLogParams{
		UserID:     userID,
		ImportType: "csv_transactions",
		Filename:   header.Filename,
		Status:     status,
		RowCount:   result.Inserted,
		Skipped:    result.Skipped,
		ErrorCount: result.Failed,
	})

	reviewCount, _ := s.transactions.NeedsReviewCount(r.Context(), userID)
	writeJSON(w, http.StatusOK, map[string]any{
		"inserted":    result.Inserted,
		"skipped":     result.Skipped,
		"failed":      result.Failed,
		"reviewCount": reviewCount,
	})
}

// handleBudgetReviewCount returns the count of transactions that need category review.
func (s *Server) handleBudgetReviewCount(w http.ResponseWriter, r *http.Request) {
	count, err := s.transactions.NeedsReviewCount(r.Context(), userIDFromCtx(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": count})
}

func (s *Server) handleCreateTransaction(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	var body struct {
		TransDate  string  `json:"transDate"`
		Merchant   string  `json:"merchant"`
		Amount     float64 `json:"amount"`
		CategoryID *string `json:"categoryId"`
		AccountID  string  `json:"accountId"`
		Notes      *string `json:"notes"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.Merchant == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION", "merchant is required")
		return
	}
	if body.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION", "amount must be positive")
		return
	}
	date, err := time.Parse("2006-01-02", body.TransDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION", "transDate must be YYYY-MM-DD")
		return
	}
	var catID *uuid.UUID
	if body.CategoryID != nil {
		id, err := uuid.Parse(*body.CategoryID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "invalid categoryId")
			return
		}
		catID = &id
	}
	accountID, accountLabel, ok := s.resolveTransactionAccount(w, r, userID, body.AccountID)
	if !ok {
		return
	}
	txn, err := s.transactions.Create(r.Context(), db.CreateTransactionParams{
		UserID:       userID,
		TransDate:    date,
		Merchant:     body.Merchant,
		Amount:       body.Amount,
		CategoryID:   catID,
		AccountID:    accountID,
		AccountLabel: accountLabel,
		Notes:        body.Notes,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, txn)
}

func (s *Server) handleListTransactions(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)

	params := db.ListTransactionsParams{UserID: userID}

	if raw := r.URL.Query().Get("accountId"); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			params.AccountID = &id
		}
	}
	if raw := r.URL.Query().Get("from"); raw != "" {
		if t, err := time.Parse("2006-01-02", raw); err == nil {
			params.DateFrom = &t
		}
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		if t, err := time.Parse("2006-01-02", raw); err == nil {
			params.DateTo = &t
		}
	}
	for _, raw := range r.URL.Query()["categoryId"] {
		if id, err := uuid.Parse(raw); err == nil {
			params.CategoryIDs = append(params.CategoryIDs, id)
		}
	}
	params.Search = r.URL.Query().Get("search")
	if raw := r.URL.Query().Get("needsReview"); raw == "true" {
		t := true
		params.NeedsReview = &t
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		params.Limit = n
	}

	txns, err := s.transactions.List(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, txns)
}

func (s *Server) handleUpdateTransaction(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid transaction id")
		return
	}
	var body struct {
		CategoryID    *string `json:"categoryId"`
		Notes         *string `json:"notes"`
		AccountID     string  `json:"accountId"`
		ApplyToVendor bool    `json:"applyToVendor"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	var catID *uuid.UUID
	if body.CategoryID != nil {
		parsed, err := uuid.Parse(*body.CategoryID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "invalid categoryId")
			return
		}
		catID = &parsed
	}
	accountID, accountLabel, ok := s.resolveTransactionAccount(w, r, userID, body.AccountID)
	if !ok {
		return
	}
	txn, err := s.transactions.Update(r.Context(), db.UpdateTransactionParams{
		ID:           id,
		UserID:       userID,
		CategoryID:   catID,
		Notes:        body.Notes,
		AccountID:    accountID,
		AccountLabel: accountLabel,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	if body.ApplyToVendor && catID != nil {
		s.transactions.ApplyVendorCategory(r.Context(), userID, txn.Merchant, id, *catID)
	}
	writeJSON(w, http.StatusOK, txn)
}

func (s *Server) handleDeleteTransaction(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid transaction id")
		return
	}
	if err := s.transactions.Delete(r.Context(), id, userID); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "transaction not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListDuplicateTransactions previews manual/CSV transactions that a Plaid
// sync has since duplicated (same date + amount). Read-only — deletes nothing.
//
// GET /budget/duplicates
func (s *Server) handleListDuplicateTransactions(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	dupes, err := s.transactions.ListManualDuplicatesOfPlaid(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dupes)
}

// handleBulkDeleteTransactions deletes the given transaction ids for the user.
// The duplicate-cleanup UI posts exactly the ids it previewed, so deletion is
// always over a set the user has reviewed.
//
// POST /budget/transactions/bulk-delete  Body: {"ids": ["...", "..."]}
func (s *Server) handleBulkDeleteTransactions(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	ids := make([]uuid.UUID, 0, len(body.IDs))
	for _, raw := range body.IDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "invalid transaction id: "+raw)
			return
		}
		ids = append(ids, id)
	}
	n, err := s.transactions.BulkDelete(r.Context(), userID, ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": n})
}

func (s *Server) handleBudgetSummary(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	now := time.Now()
	year := intParam(r, "year", now.Year())

	rows := []models.BudgetSummaryRow{}
	var monthlyIncome float64
	var err error
	if r.URL.Query().Get("annual") == "true" {
		rows, err = s.transactions.AnnualSummary(r.Context(), userID, year)
	} else {
		month := intParam(r, "month", int(now.Month()))
		rows, err = s.transactions.Summary(r.Context(), userID, year, month)
		if err == nil {
			monthlyIncome, err = s.transactions.MonthlyIncomeTotal(r.Context(), userID, year, month)
		}
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	reviewCount, err := s.transactions.NeedsReviewCount(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"categories":    rows,
		"reviewCount":   reviewCount,
		"monthlyIncome": monthlyIncome,
	})
}

func (s *Server) handleBudgetIncomeBreakdown(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	year := intParam(r, "year", time.Now().Year())
	rows, err := s.transactions.MonthlyIncomeBreakdown(r.Context(), userID, year)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleBudgetMonthlyBreakdown(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	year := intParam(r, "year", time.Now().Year())
	rows, err := s.transactions.MonthlyBreakdown(r.Context(), userID, year)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func intParam(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
