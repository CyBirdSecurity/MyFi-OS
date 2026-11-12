package api

import (
	"database/sql"
	"errors"
	"math"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// maxBudgetLimit bounds a monthly limit to something a typo can't push past.
const maxBudgetLimit = 10_000_000

// validBudgetLimit returns the limit rounded to cents, or false when it isn't a
// usable amount: it must be a real, positive number that is still positive after
// rounding, and no larger than maxBudgetLimit.
func validBudgetLimit(v float64) (float64, bool) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v > maxBudgetLimit {
		return 0, false
	}
	rounded := math.Round(v*100) / 100
	if rounded <= 0 {
		return 0, false
	}
	return rounded, true
}

// handleListBudgetLimits returns the user's monthly category limits.
//
// GET /budget/limits
func (s *Server) handleListBudgetLimits(w http.ResponseWriter, r *http.Request) {
	limits, err := s.budgetLimits.List(r.Context(), userIDFromCtx(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not load budget limits")
		return
	}
	writeJSON(w, http.StatusOK, limits)
}

// handleSetBudgetLimit sets (or replaces) the monthly limit for a category.
//
// PUT /budget/limits/{categoryId}
// Body: {"monthlyLimit": 450}
func (s *Server) handleSetBudgetLimit(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	categoryID, err := uuid.Parse(chi.URLParam(r, "categoryId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid category id")
		return
	}
	var body struct {
		MonthlyLimit float64 `json:"monthlyLimit"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	limit, ok := validBudgetLimit(body.MonthlyLimit)
	if !ok {
		writeError(w, http.StatusBadRequest, "VALIDATION", "monthlyLimit must be a positive amount")
		return
	}

	cat, err := s.budgetCategories.Get(r.Context(), userID, categoryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "category not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not check the category")
		return
	}
	// Transfers and income aren't spending, so a spending limit on them means nothing.
	if cat.IsSystem && (cat.Name == "Transfer" || cat.Name == "Income") {
		writeError(w, http.StatusBadRequest, "VALIDATION", "limits apply to spending categories")
		return
	}

	if err := s.budgetLimits.Upsert(r.Context(), userID, categoryID, limit); err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not save the limit")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categoryId": categoryID, "monthlyLimit": limit})
}

// handleDeleteBudgetLimit removes the monthly limit for a category.
//
// DELETE /budget/limits/{categoryId}
func (s *Server) handleDeleteBudgetLimit(w http.ResponseWriter, r *http.Request) {
	categoryID, err := uuid.Parse(chi.URLParam(r, "categoryId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid category id")
		return
	}
	if err := s.budgetLimits.Delete(r.Context(), userIDFromCtx(r), categoryID); err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not remove the limit")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
