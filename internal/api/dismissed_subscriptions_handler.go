package api

import (
	"net/http"
	"strings"
)

func (s *Server) handleListDismissedSubscriptions(w http.ResponseWriter, r *http.Request) {
	keys, err := s.dismissedSubs.List(r.Context(), userIDFromCtx(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not load dismissed subscriptions")
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

// subscriptionKey reads {"key": "..."} — a merchant key from the Subscriptions tab.
func subscriptionKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Key string `json:"key"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return "", false
	}
	key := strings.TrimSpace(body.Key)
	if key == "" || len(key) > 200 {
		writeError(w, http.StatusBadRequest, "VALIDATION", "key is required")
		return "", false
	}
	return key, true
}

func (s *Server) handleDismissSubscription(w http.ResponseWriter, r *http.Request) {
	key, ok := subscriptionKey(w, r)
	if !ok {
		return
	}
	if err := s.dismissedSubs.Dismiss(r.Context(), userIDFromCtx(r), key); err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not dismiss the subscription")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRestoreSubscription(w http.ResponseWriter, r *http.Request) {
	key, ok := subscriptionKey(w, r)
	if !ok {
		return
	}
	if err := s.dismissedSubs.Restore(r.Context(), userIDFromCtx(r), key); err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not restore the subscription")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
