package api

import "net/http"

func (s *Server) handleGetHistoricalBacktest(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	result, err := s.forecast.RunHistoricalBacktest(r.Context(), userID)
	if err != nil {
		writeForecastError(w, err, http.StatusBadRequest, "BACKTEST_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
