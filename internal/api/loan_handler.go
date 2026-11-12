package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

func (s *Server) handleGetLoanDetails(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}

	d, err := s.loans.Get(r.Context(), accountID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, &models.LoanDetails{AccountID: accountID, UserID: userID})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// handleUpsertLoanDetails saves the interest rate and monthly payment the
// forecast uses to project a loan's payoff.
func (s *Server) handleUpsertLoanDetails(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	acc, err := s.accounts.Get(r.Context(), accountID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "account not found")
		return
	}
	if acc.AccountType != "loan" && acc.AccountType != "other_liability" {
		writeError(w, http.StatusBadRequest, "VALIDATION", "loan details only apply to loan accounts")
		return
	}

	var body struct {
		APRPercentage  *float64 `json:"aprPercentage,omitempty"`
		MonthlyPayment *float64 `json:"monthlyPayment,omitempty"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.APRPercentage != nil && (*body.APRPercentage < 0 || *body.APRPercentage > 100) {
		writeError(w, http.StatusBadRequest, "VALIDATION", "interest rate must be between 0% and 100%")
		return
	}
	if body.MonthlyPayment != nil && *body.MonthlyPayment < 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION", "monthly payment can't be negative")
		return
	}

	d, err := s.loans.Upsert(r.Context(), db.UpsertLoanParams{
		AccountID:      accountID,
		UserID:         userID,
		APRPercentage:  body.APRPercentage,
		MonthlyPayment: body.MonthlyPayment,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}
