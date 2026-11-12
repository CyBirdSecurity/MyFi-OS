package api

import (
	"net/http"

	"github.com/cybirdsecurity/myfi-os/internal/services"
)

func (s *Server) handleSolveScenario(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)

	var body services.SolveParams
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}

	result, err := s.forecast.SolveScenario(r.Context(), userID, body)
	if err != nil {
		writeForecastError(w, err, http.StatusBadRequest, "VALIDATION")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
