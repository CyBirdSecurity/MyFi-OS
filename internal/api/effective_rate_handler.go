package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// handleGetAccountEffectiveRate returns the computed effective return rate for one account.
func (s *Server) handleGetAccountEffectiveRate(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}

	result, err := s.forecast.EffectiveRateForAccount(r.Context(), accountID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
