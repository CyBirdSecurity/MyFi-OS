package api

import (
	"net/http"

	"github.com/cybirdsecurity/myfi-os/internal/db"
)

func (s *Server) handleGetAllocationTargets(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	targets, err := s.allocationTargets.Get(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	if targets == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"equityPct": 0,
			"bondPct":   0,
			"cryptoPct": 0,
			"cashPct":   0,
			"otherPct":  0,
		})
		return
	}
	writeJSON(w, http.StatusOK, targets)
}

func (s *Server) handleUpsertAllocationTargets(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)

	var body struct {
		EquityPct *float64 `json:"equityPct"`
		BondPct   *float64 `json:"bondPct"`
		CryptoPct *float64 `json:"cryptoPct"`
		CashPct   *float64 `json:"cashPct"`
		OtherPct  *float64 `json:"otherPct"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}

	existing, _ := s.allocationTargets.Get(r.Context(), userID)

	params := db.UpsertAllocationTargetParams{UserID: userID}
	if existing != nil {
		params.EquityPct = existing.EquityPct
		params.BondPct = existing.BondPct
		params.CryptoPct = existing.CryptoPct
		params.CashPct = existing.CashPct
		params.OtherPct = existing.OtherPct
	}
	if body.EquityPct != nil {
		params.EquityPct = *body.EquityPct
	}
	if body.BondPct != nil {
		params.BondPct = *body.BondPct
	}
	if body.CryptoPct != nil {
		params.CryptoPct = *body.CryptoPct
	}
	if body.CashPct != nil {
		params.CashPct = *body.CashPct
	}
	if body.OtherPct != nil {
		params.OtherPct = *body.OtherPct
	}

	out, err := s.allocationTargets.Upsert(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}
