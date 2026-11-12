package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
	"github.com/cybirdsecurity/myfi-os/internal/services"
)

// handleListAllContributions returns all active contributions for the authenticated user
// across all their accounts. Used by the forecast page to display model context.
func (s *Server) handleListAllContributions(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	contribs, err := s.contributions.ListByUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, contribs)
}

func (s *Server) handleListContributions(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	// Verify household access to this account.
	// Verify household access to this account (owner or accepted partner).
	if _, err := s.accounts.Get(r.Context(), accountID, userID); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "account not found")
		return
	}
	contribs, err := s.contributions.ListByAccount(r.Context(), accountID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, contribs)
}

func (s *Server) handleCreateContribution(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	// Verify household access to this account (owner or accepted partner).
	acc, err := s.accounts.Get(r.Context(), accountID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "account not found")
		return
	}

	var body struct {
		Label             *string           `json:"label"`
		Amount            float64           `json:"amount"`
		Frequency         string            `json:"frequency"`
		AnnualIncreasePct float64           `json:"annualIncreasePct"`
		AmountType        string            `json:"amountType"`
		IncomePct         float64           `json:"incomePct"`
		MatchTiers        models.MatchTiers `json:"matchTiers"`
		NextPayDate       *string           `json:"nextPayDate"`
		StartDate         *string           `json:"startDate"`
		EndDate           *string           `json:"endDate"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.Amount < 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION", "amount must be >= 0")
		return
	}
	if body.Frequency == "" {
		body.Frequency = "monthly"
	}
	if body.Frequency != "monthly" && body.Frequency != "annually" && body.Frequency != "biweekly" {
		writeError(w, http.StatusBadRequest, "VALIDATION", "frequency must be 'monthly', 'annually', or 'biweekly'")
		return
	}
	if body.Frequency == "biweekly" && body.NextPayDate == nil {
		writeError(w, http.StatusBadRequest, "VALIDATION", "nextPayDate is required for biweekly frequency")
		return
	}
	var nextPayDate *time.Time
	if body.NextPayDate != nil {
		t, err := time.Parse("2006-01-02", *body.NextPayDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "nextPayDate must be YYYY-MM-DD")
			return
		}
		nextPayDate = &t
	}
	var startDate, endDate *time.Time
	if body.StartDate != nil {
		t, err := time.Parse("2006-01-02", *body.StartDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "startDate must be YYYY-MM-DD")
			return
		}
		startDate = &t
	}
	if body.EndDate != nil {
		t, err := time.Parse("2006-01-02", *body.EndDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "endDate must be YYYY-MM-DD")
			return
		}
		endDate = &t
	}
	if startDate != nil && endDate != nil && !endDate.After(*startDate) {
		writeError(w, http.StatusBadRequest, "VALIDATION", "endDate must be after startDate")
		return
	}
	if body.AmountType == "" {
		body.AmountType = "fixed"
	}
	if msg := s.validateContributionIncome(r, *acc, models.Contribution{
		AmountType: body.AmountType, IncomePct: body.IncomePct, MatchTiers: body.MatchTiers,
	}); msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION", msg)
		return
	}

	c, err := s.contributions.Create(r.Context(), db.CreateContributionParams{
		AccountID:         accountID,
		UserID:            userID,
		Label:             body.Label,
		Amount:            body.Amount,
		Frequency:         body.Frequency,
		AnnualIncreasePct: body.AnnualIncreasePct,
		AmountType:        body.AmountType,
		IncomePct:         body.IncomePct,
		MatchTiers:        body.MatchTiers,
		NextPayDate:       nextPayDate,
		StartDate:         startDate,
		EndDate:           endDate,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleUpdateContribution(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	cID, err := uuid.Parse(chi.URLParam(r, "cId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid contribution id")
		return
	}
	// Verify household access to this account (owner or accepted partner).
	acc, err := s.accounts.Get(r.Context(), accountID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "account not found")
		return
	}

	var body struct {
		Label             *string           `json:"label"`
		Amount            float64           `json:"amount"`
		Frequency         string            `json:"frequency"`
		AnnualIncreasePct float64           `json:"annualIncreasePct"`
		AmountType        string            `json:"amountType"`
		IncomePct         float64           `json:"incomePct"`
		MatchTiers        models.MatchTiers `json:"matchTiers"`
		IsActive          bool              `json:"isActive"`
		NextPayDate       *string           `json:"nextPayDate"`
		StartDate         *string           `json:"startDate"`
		EndDate           *string           `json:"endDate"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.Frequency == "" {
		body.Frequency = "monthly"
	}
	var nextPayDate *time.Time
	if body.NextPayDate != nil {
		t, err := time.Parse("2006-01-02", *body.NextPayDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "nextPayDate must be YYYY-MM-DD")
			return
		}
		nextPayDate = &t
	}
	var startDate, endDate *time.Time
	if body.StartDate != nil {
		t, err := time.Parse("2006-01-02", *body.StartDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "startDate must be YYYY-MM-DD")
			return
		}
		startDate = &t
	}
	if body.EndDate != nil {
		t, err := time.Parse("2006-01-02", *body.EndDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION", "endDate must be YYYY-MM-DD")
			return
		}
		endDate = &t
	}
	if startDate != nil && endDate != nil && !endDate.After(*startDate) {
		writeError(w, http.StatusBadRequest, "VALIDATION", "endDate must be after startDate")
		return
	}
	if body.AmountType == "" {
		body.AmountType = "fixed"
	}
	if msg := s.validateContributionIncome(r, *acc, models.Contribution{
		AmountType: body.AmountType, IncomePct: body.IncomePct, MatchTiers: body.MatchTiers,
	}); msg != "" {
		writeError(w, http.StatusBadRequest, "VALIDATION", msg)
		return
	}

	c, err := s.contributions.Update(r.Context(), db.UpdateContributionParams{
		ID:                cID,
		AccountID:         accountID,
		UserID:            userID,
		Label:             body.Label,
		Amount:            body.Amount,
		Frequency:         body.Frequency,
		AnnualIncreasePct: body.AnnualIncreasePct,
		AmountType:        body.AmountType,
		IncomePct:         body.IncomePct,
		MatchTiers:        body.MatchTiers,
		IsActive:          body.IsActive,
		NextPayDate:       nextPayDate,
		StartDate:         startDate,
		EndDate:           endDate,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteContribution(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	cID, err := uuid.Parse(chi.URLParam(r, "cId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid contribution id")
		return
	}
	// Verify household access to this account (owner or accepted partner).
	if _, err := s.accounts.Get(r.Context(), accountID, userID); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "account not found")
		return
	}
	if err := s.contributions.Delete(r.Context(), cID, accountID, userID); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "contribution not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validateContributionIncome checks the amount type and match tiers, and that
// a contribution based on salary has one to use: the account must belong to
// one person and that person needs an income. Returns "" when valid.
func (s *Server) validateContributionIncome(r *http.Request, acc models.Account, c models.Contribution) string {
	switch c.AmountType {
	case "fixed":
	case "percent":
		if c.IncomePct <= 0 || c.IncomePct > 1 {
			return "incomePct must be between 0 and 1"
		}
	default:
		return "amountType must be 'fixed' or 'percent'"
	}
	for i, t := range c.MatchTiers {
		if t.MatchPct < 0 || t.MatchPct > 2 {
			return "each match tier's matchPct must be between 0 and 2"
		}
		if t.UpToPct == nil && i != len(c.MatchTiers)-1 {
			return "only the last match tier can be uncapped"
		}
		if t.UpToPct != nil && (*t.UpToPct <= 0 || *t.UpToPct > 1) {
			return "each match tier's upToPct must be between 0 and 1"
		}
	}
	if !services.NeedsIncome(c) {
		return ""
	}
	if acc.IsJoint {
		return "Joint accounts don't have one salary to base this on. Assign the account to one person to use a percent of income or a capped match."
	}
	members, err := s.householdMembers.ListByUser(r.Context(), acc.UserID)
	if err != nil {
		return "couldn't load household income"
	}
	owner := services.AccountOwner(acc, members)
	if services.AnnualSalary(owner) <= 0 {
		name := "your"
		if owner != nil && owner.Relationship == "partner" && owner.Name != "" {
			name = owner.Name + "'s"
		}
		return "Add " + name + " annual income in Settings to use a percent of income or a capped match."
	}
	return ""
}
