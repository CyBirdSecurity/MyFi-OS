package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// handleListHoldingDividends returns the dividend application history for a
// single holding (audit trail for DRIP reinvestment).
func (s *Server) handleListHoldingDividends(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	holdingID, err := uuid.Parse(chi.URLParam(r, "hId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid holding id")
		return
	}
	divs, err := s.dividends.ListByHolding(r.Context(), holdingID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, divs)
}

// handleTriggerDividends lets the current user manually check their holdings
// for new dividends instead of waiting for the monthly auto-dividend job.
func (s *Server) handleTriggerDividends(w http.ResponseWriter, r *http.Request) {
	s.autoDividend.Trigger()
	writeJSON(w, http.StatusOK, map[string]string{"status": "triggered"})
}
