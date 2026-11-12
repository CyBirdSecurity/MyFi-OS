package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// handleHealth is a public health-check endpoint.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	var one int
	dbOK := s.db.QueryRowContext(ctx, "SELECT 1").Scan(&one) == nil
	status := "ok"
	if !dbOK {
		status = "degraded"
	}
	uptime := time.Since(s.startedAt).Round(time.Second).String()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": status,
		"db":     dbOK,
		"uptime": uptime,
	})
}
