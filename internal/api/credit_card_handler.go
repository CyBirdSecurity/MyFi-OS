package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

func (s *Server) handleGetCreditCardDetails(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}

	d, err := s.creditCards.Get(r.Context(), accountID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, &models.CreditCardDetails{AccountID: accountID, UserID: userID})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// handleUpsertCreditCardDetails supports manual entry for non-Plaid-linked
// cards. Once the account is Plaid-linked, the next sync overwrites these
// fields (same "Plaid always wins" precedent as mortgage/holdings data).
func (s *Server) handleUpsertCreditCardDetails(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}

	var body struct {
		APRPercentage          *float64 `json:"aprPercentage,omitempty"`
		CreditLimit            *float64 `json:"creditLimit,omitempty"`
		LastStatementBalance   *float64 `json:"lastStatementBalance,omitempty"`
		LastStatementIssueDate string   `json:"lastStatementIssueDate,omitempty"`
		MinimumPaymentAmount   *float64 `json:"minimumPaymentAmount,omitempty"`
		NextPaymentDueDate     string   `json:"nextPaymentDueDate,omitempty"`
		LastPaymentAmount      *float64 `json:"lastPaymentAmount,omitempty"`
		LastPaymentDate        string   `json:"lastPaymentDate,omitempty"`
		IsOverdue              bool     `json:"isOverdue"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}

	d, err := s.creditCards.Upsert(r.Context(), db.UpsertCreditCardParams{
		AccountID:              accountID,
		UserID:                 userID,
		APRPercentage:          body.APRPercentage,
		CreditLimit:            body.CreditLimit,
		LastStatementBalance:   body.LastStatementBalance,
		LastStatementIssueDate: parseDatePtr(body.LastStatementIssueDate),
		MinimumPaymentAmount:   body.MinimumPaymentAmount,
		NextPaymentDueDate:     parseDatePtr(body.NextPaymentDueDate),
		LastPaymentAmount:      body.LastPaymentAmount,
		LastPaymentDate:        parseDatePtr(body.LastPaymentDate),
		IsOverdue:              body.IsOverdue,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func parseDatePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}
