package api

import (
	"net/http"
	"strconv"
)

func (s *Server) handleTaxAnalysis(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	var conversionOverride *float64
	if v := r.URL.Query().Get("conversion"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err == nil && f >= 0 {
			conversionOverride = &f
		}
	}
	analysis, err := s.taxService.Analyze(r.Context(), userID, conversionOverride)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, analysis)
}
