package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/services"
)

func (s *Server) handleListRecurring(w http.ResponseWriter, r *http.Request) {
	rules, err := s.transactions.ListRecurring(r.Context(), userIDFromCtx(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

// handleCreateRecurring saves a recurring transaction and immediately adds any
// occurrences already due (a start date of today or earlier).
//
// POST /budget/recurring
func (s *Server) handleCreateRecurring(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	var body struct {
		StartDate  string  `json:"startDate"`
		Frequency  string  `json:"frequency"`
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
	if body.Frequency != "monthly" && body.Frequency != "biweekly" {
		writeError(w, http.StatusBadRequest, "VALIDATION", "frequency must be monthly or biweekly")
		return
	}
	start, err := time.Parse("2006-01-02", body.StartDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION", "startDate must be YYYY-MM-DD")
		return
	}
	// Past occurrences are back-filled on create; cap how far back that goes.
	if start.Before(services.Today().AddDate(-1, 0, 0)) {
		writeError(w, http.StatusBadRequest, "VALIDATION", "start date can't be more than a year ago")
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

	rule, err := s.transactions.CreateRecurring(r.Context(), db.CreateRecurringParams{
		UserID:       userID,
		Merchant:     body.Merchant,
		Amount:       body.Amount,
		CategoryID:   catID,
		AccountID:    accountID,
		AccountLabel: accountLabel,
		Notes:        body.Notes,
		Frequency:    body.Frequency,
		StartDate:    start,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	added, err := s.transactions.GenerateDueRecurring(r.Context(), &userID, services.Today())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": rule.ID, "added": added})
}

// handleDeleteRecurring stops a recurring transaction. Transactions it already
// added are kept.
func (s *Server) handleDeleteRecurring(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid recurring transaction id")
		return
	}
	if err := s.transactions.DeleteRecurring(r.Context(), id, userIDFromCtx(r)); err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
