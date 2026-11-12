package api

import (
	"net/http"
)

func (s *Server) handleGetForecast(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	result, version := s.forecastCache.get(userID)
	if result == nil {
		var err error
		result, err = s.forecast.Calculate(r.Context(), userID)
		if err != nil {
			writeForecastError(w, err, http.StatusInternalServerError, "FORECAST_ERROR")
			return
		}
		s.forecastCache.set(userID, version, result)
	}
	writeJSON(w, http.StatusOK, result)
}
