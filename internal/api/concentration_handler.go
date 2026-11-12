package api

import "net/http"

func (s *Server) handleGetConcentration(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	result, err := s.concentration.Compute(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
