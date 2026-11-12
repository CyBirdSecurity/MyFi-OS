package api

import (
	"net/http"
	"strconv"
)

func (s *Server) handleGetRetirementPlan(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)

	monthlySpending := 0.0
	if raw := r.URL.Query().Get("monthlySpending"); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v < 0 {
			writeError(w, http.StatusBadRequest, "VALIDATION", "monthlySpending must be a non-negative number")
			return
		}
		monthlySpending = v
	}

	plan, err := s.forecast.ComputeRetirementPlan(r.Context(), userID, monthlySpending)
	if err != nil {
		writeForecastError(w, err, http.StatusInternalServerError, "SERVER_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, plan)
}
