package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// handleListReturnRates returns the user's custom return rate overrides merged with system defaults.
func (s *Server) handleListReturnRates(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	custom, err := s.returnRates.ListByUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}

	// Build a merged map: start with system defaults, overlay user overrides.
	customMap := make(map[string]float64, len(custom))
	for _, rr := range custom {
		customMap[rr.AssetClass] = rr.ReturnRate
	}

	type rateEntry struct {
		AssetClass    string   `json:"assetClass"`
		DefaultRate   float64  `json:"defaultRate"`
		CustomRate    *float64 `json:"customRate"`
		EffectiveRate float64  `json:"effectiveRate"`
	}

	assetClasses := []string{"equity", "bond", "crypto", "cash_equivalent", "other"}
	result := make([]rateEntry, 0, len(assetClasses))
	for _, ac := range assetClasses {
		def := models.DefaultAssetClassReturnRates[ac]
		entry := rateEntry{
			AssetClass:    ac,
			DefaultRate:   def,
			EffectiveRate: def,
		}
		if cr, ok := customMap[ac]; ok {
			entry.CustomRate = &cr
			entry.EffectiveRate = cr
		}
		result = append(result, entry)
	}

	writeJSON(w, http.StatusOK, result)
}

// handleUpsertReturnRate sets a custom return rate for an asset class.
func (s *Server) handleUpsertReturnRate(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	assetClass := chi.URLParam(r, "assetClass")

	validClasses := map[string]bool{
		"equity": true, "bond": true,
		"crypto": true, "cash_equivalent": true, "other": true,
	}
	if !validClasses[assetClass] {
		writeError(w, http.StatusBadRequest, "VALIDATION", "unknown asset class")
		return
	}

	var body struct {
		ReturnRate float64 `json:"returnRate"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.ReturnRate < 0 || body.ReturnRate > 1 {
		writeError(w, http.StatusBadRequest, "VALIDATION", "returnRate must be between 0 and 1 (e.g., 0.07 for 7%)")
		return
	}

	rr, err := s.returnRates.Upsert(r.Context(), userID, assetClass, body.ReturnRate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rr)
}

// handleDeleteReturnRate removes a custom return rate, reverting to the system default.
func (s *Server) handleDeleteReturnRate(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	assetClass := chi.URLParam(r, "assetClass")

	if err := s.returnRates.Delete(r.Context(), userID, assetClass); err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSetAccountReturnRate sets or clears a per-account return rate override.
func (s *Server) handleSetAccountReturnRate(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	if _, err := s.accounts.Get(r.Context(), accountID, userID); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "account not found")
		return
	}

	var body struct {
		ReturnRate *float64 `json:"returnRate"` // null to clear override
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.ReturnRate != nil && (*body.ReturnRate < 0 || *body.ReturnRate > 1) {
		writeError(w, http.StatusBadRequest, "VALIDATION", "returnRate must be between 0 and 1")
		return
	}

	if err := s.returnRates.SetAccountOverride(r.Context(), accountID, userID, body.ReturnRate); err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}

	acc, _ := s.accounts.Get(r.Context(), accountID, userID)
	writeJSON(w, http.StatusOK, acc)
}
