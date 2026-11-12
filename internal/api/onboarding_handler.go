package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// pageGuides are the first-visit guide IDs the frontend may dismiss, plus the
// dashboard's Getting Started checklist.
var pageGuides = map[string]bool{
	"dashboard": true, "accounts": true, "holdings": true, "forecast": true, "retirement": true,
	"scenarios": true, "goals": true, "budget": true, "taxes": true, "import": true,
	"getting-started": true,
}

// handleMarkOnboarded records that the user finished or skipped the welcome screen.
func (s *Server) handleMarkOnboarded(w http.ResponseWriter, r *http.Request) {
	if err := s.users.MarkOnboarded(r.Context(), userIDFromCtx(r)); err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not update onboarding status")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDismissGuide stops a page's first-visit guide from opening automatically.
func (s *Server) handleDismissGuide(w http.ResponseWriter, r *http.Request) {
	guide := chi.URLParam(r, "guide")
	if !pageGuides[guide] {
		writeError(w, http.StatusBadRequest, "VALIDATION", "unknown guide")
		return
	}
	if err := s.users.DismissGuide(r.Context(), userIDFromCtx(r), guide); err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not dismiss guide")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
