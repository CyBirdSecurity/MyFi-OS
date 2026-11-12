package api

import (
	"net/http"

	"github.com/cybirdsecurity/myfi-os/internal/services"
)

func (s *Server) handleRunScenarios(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)

	var body struct {
		Scenarios []services.ScenarioParams `json:"scenarios"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if len(body.Scenarios) > 3 {
		writeError(w, http.StatusBadRequest, "VALIDATION", "maximum 3 scenarios allowed")
		return
	}

	result, err := s.forecast.RunScenarios(r.Context(), userID, body.Scenarios)
	if err != nil {
		writeForecastError(w, err, http.StatusInternalServerError, "SERVER_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
