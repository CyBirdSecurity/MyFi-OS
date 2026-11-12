package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
)

func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	snaps, err := s.snapshots.List(r.Context(), accountID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snaps)
}

func (s *Server) handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	// Verify the requesting user has household access to this account.
	if _, err := s.accounts.Get(r.Context(), accountID, userID); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "account not found")
		return
	}
	var body struct {
		SnapshotDate string  `json:"snapshotDate"`
		Balance      float64 `json:"balance"`
		Miles        *int    `json:"miles"`
		Notes        *string `json:"notes"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	date, err := time.Parse("2006-01-02", body.SnapshotDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION", "snapshotDate must be YYYY-MM-DD")
		return
	}

	snap, err := s.snapshots.Upsert(r.Context(), db.UpsertSnapshotParams{
		AccountID:    accountID,
		UserID:       userID,
		SnapshotDate: date,
		Balance:      body.Balance,
		Miles:        body.Miles,
		Notes:        body.Notes,
		Source:       "manual",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, snap)
}

func (s *Server) handleUpdateSnapshot(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	snapID, err := uuid.Parse(chi.URLParam(r, "snapId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid snapshot id")
		return
	}
	var body struct {
		SnapshotDate string  `json:"snapshotDate"`
		Balance      float64 `json:"balance"`
		Miles        *int    `json:"miles"`
		Notes        *string `json:"notes"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	date, err := time.Parse("2006-01-02", body.SnapshotDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION", "snapshotDate must be YYYY-MM-DD")
		return
	}

	snap, err := s.snapshots.Update(r.Context(), db.UpdateSnapshotParams{
		ID:           snapID,
		AccountID:    accountID,
		UserID:       userID,
		SnapshotDate: date,
		Balance:      body.Balance,
		Miles:        body.Miles,
		Notes:        body.Notes,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// handleTriggerUserSnapshot lets the current user manually snapshot their own
// accounts on demand, instead of waiting for the weekly auto-snapshot job.
func (s *Server) handleTriggerUserSnapshot(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	count, err := s.autosnap.SnapshotUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"accountsUpdated": count})
}

func (s *Server) handleDeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	snapID, err := uuid.Parse(chi.URLParam(r, "snapId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid snapshot id")
		return
	}
	if err := s.snapshots.Delete(r.Context(), snapID, userID); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "snapshot not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
