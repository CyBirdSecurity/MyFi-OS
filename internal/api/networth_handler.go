package api

import (
	"net/http"
	"time"
)

func (s *Server) handleNetWorthCurrent(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	nw, err := s.networth.Current(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, nw)
}

// parseHistoryRange reads the from/to/interval query parameters shared by the
// history endpoints, defaulting to the last year by month. On bad input it
// writes the 400 itself and returns ok=false.
func parseHistoryRange(w http.ResponseWriter, r *http.Request) (from, to time.Time, interval string, ok bool) {
	q := r.URL.Query()
	fromStr, toStr, interval := q.Get("from"), q.Get("to"), q.Get("interval")

	if fromStr == "" {
		fromStr = time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
	}
	if toStr == "" {
		toStr = time.Now().Format("2006-01-02")
	}
	if interval == "" {
		interval = "month"
	}

	switch interval {
	case "day", "week", "month", "year":
	default:
		writeError(w, http.StatusBadRequest, "VALIDATION", "interval must be day, week, month, or year")
		return
	}

	var err error
	if from, err = time.Parse("2006-01-02", fromStr); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION", "from must be YYYY-MM-DD")
		return
	}
	if to, err = time.Parse("2006-01-02", toStr); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION", "to must be YYYY-MM-DD")
		return
	}
	return from, to, interval, true
}

func (s *Server) handleNetWorthHistory(w http.ResponseWriter, r *http.Request) {
	from, to, interval, ok := parseHistoryRange(w, r)
	if !ok {
		return
	}
	history, err := s.networth.History(r.Context(), userIDFromCtx(r), from, to, interval)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, history)
}

// handleNetWorthAccountHistory returns net worth over time broken down by
// account, for the dashboard's detailed net worth chart.
//
// GET /networth/history/accounts?from=&to=&interval=
func (s *Server) handleNetWorthAccountHistory(w http.ResponseWriter, r *http.Request) {
	from, to, interval, ok := parseHistoryRange(w, r)
	if !ok {
		return
	}
	history, err := s.networth.AccountHistory(r.Context(), userIDFromCtx(r), from, to, interval)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, history)
}
