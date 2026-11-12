package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
)

func (s *Server) handleListHoldingsByAccount(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	holdings, err := s.holdings.ListByAccount(r.Context(), accountID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	for i := range holdings {
		if holdings[i].AssetClass == "cash_equivalent" {
			holdings[i].TotalValue = holdings[i].Shares
		} else if holdings[i].PricePerShare != nil {
			holdings[i].TotalValue = holdings[i].Shares * *holdings[i].PricePerShare
		}
	}
	writeJSON(w, http.StatusOK, holdings)
}

func (s *Server) handleCreateHolding(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	var body struct {
		Ticker        string   `json:"ticker"`
		Name          *string  `json:"name"`
		AssetClass    string   `json:"assetClass"`
		Shares        float64  `json:"shares"`
		CostBasis     *float64 `json:"costBasis"`
		ManualPrice   bool     `json:"manualPrice"`
		PricePerShare *float64 `json:"pricePerShare"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	if body.Ticker == "" || body.Shares <= 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION", "ticker and positive shares are required")
		return
	}
	body.Ticker = strings.ToUpper(strings.TrimSpace(body.Ticker))
	if body.AssetClass == "" {
		body.AssetClass = "equity"
	}

	h, err := s.holdings.Create(r.Context(), db.CreateHoldingParams{
		AccountID:     accountID,
		UserID:        userID,
		Ticker:        body.Ticker,
		Name:          body.Name,
		AssetClass:    body.AssetClass,
		Shares:        body.Shares,
		CostBasis:     body.CostBasis,
		ManualPrice:   body.ManualPrice,
		PricePerShare: body.PricePerShare,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, h)
}

func (s *Server) handleUpdateHolding(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	holdingID, err := uuid.Parse(chi.URLParam(r, "hId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid holding id")
		return
	}
	var body struct {
		Ticker        string   `json:"ticker"`
		Name          *string  `json:"name"`
		AssetClass    string   `json:"assetClass"`
		Shares        float64  `json:"shares"`
		CostBasis     *float64 `json:"costBasis"`
		ManualPrice   bool     `json:"manualPrice"`
		PricePerShare *float64 `json:"pricePerShare"`
		DripEnabled   *bool    `json:"dripEnabled"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON")
		return
	}
	body.Ticker = strings.ToUpper(strings.TrimSpace(body.Ticker))

	h, err := s.holdings.Update(r.Context(), db.UpdateHoldingParams{
		ID:            holdingID,
		AccountID:     accountID,
		UserID:        userID,
		Ticker:        body.Ticker,
		Name:          body.Name,
		AssetClass:    body.AssetClass,
		Shares:        body.Shares,
		CostBasis:     body.CostBasis,
		ManualPrice:   body.ManualPrice,
		PricePerShare: body.PricePerShare,
		DripEnabled:   body.DripEnabled,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) handleDeleteHolding(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid account id")
		return
	}
	holdingID, err := uuid.Parse(chi.URLParam(r, "hId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid holding id")
		return
	}
	if err := s.holdings.Delete(r.Context(), holdingID, accountID, userID); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "holding not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAllHoldings(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	holdings, err := s.networth.HoldingsWithPrices(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, holdings)
}

func (s *Server) handleRefreshPrices(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	count, err := s.networth.RefreshHoldingPrices(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"updated": count})
}
