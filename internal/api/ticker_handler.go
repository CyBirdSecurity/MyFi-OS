package api

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// tickerAccountRow represents one account's position in a given ticker.
type tickerAccountRow struct {
	AccountID     string   `json:"accountId"`
	AccountName   string   `json:"accountName"`
	AccountType   string   `json:"accountType"`
	AssetClass    string   `json:"assetClass"`
	Shares        float64  `json:"shares"`
	PricePerShare *float64 `json:"pricePerShare"`
	TotalValue    float64  `json:"totalValue"`
}

// tickerDetailResponse is the full response for GET /holdings/ticker/{ticker}.
type tickerDetailResponse struct {
	Ticker        string             `json:"ticker"`
	Name          string             `json:"name"`
	Positions     []tickerAccountRow `json:"positions"`
	TotalShares   float64            `json:"totalShares"`
	TotalValue    float64            `json:"totalValue"`
	PricePerShare *float64           `json:"pricePerShare"`
}

// pricePoint is a single date/close pair for the history chart.
type pricePoint struct {
	Date  string  `json:"date"`
	Close float64 `json:"close"`
}

func (s *Server) handleGetTickerDetail(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	ticker := strings.ToUpper(chi.URLParam(r, "ticker"))
	if ticker == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "ticker is required")
		return
	}

	// Use HoldingsWithPrices to get live prices applied.
	allHoldings, err := s.networth.HoldingsWithPrices(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}

	// Get all accounts for name/type lookup.
	accounts, err := s.accounts.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVER_ERROR", err.Error())
		return
	}
	accountMap := make(map[string]struct {
		Name string
		Type string
	}, len(accounts))
	for _, a := range accounts {
		accountMap[a.ID.String()] = struct {
			Name string
			Type string
		}{Name: a.Name, Type: a.AccountType}
	}

	var positions []tickerAccountRow
	var displayName string
	var totalShares, totalValue float64
	var pricePerShare *float64

	for _, h := range allHoldings {
		if h.Ticker != ticker {
			continue
		}
		if h.Name != nil && displayName == "" {
			displayName = *h.Name
		}
		if pricePerShare == nil && h.PricePerShare != nil {
			pricePerShare = h.PricePerShare
		}
		acc := accountMap[h.AccountID.String()]
		row := tickerAccountRow{
			AccountID:     h.AccountID.String(),
			AccountName:   acc.Name,
			AccountType:   acc.Type,
			AssetClass:    h.AssetClass,
			Shares:        h.Shares,
			PricePerShare: h.PricePerShare,
			TotalValue:    h.TotalValue,
		}
		positions = append(positions, row)
		totalShares += h.Shares
		totalValue += h.TotalValue
	}

	if len(positions) == 0 {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "ticker not found in your holdings")
		return
	}

	writeJSON(w, http.StatusOK, tickerDetailResponse{
		Ticker:        ticker,
		Name:          displayName,
		Positions:     positions,
		TotalShares:   totalShares,
		TotalValue:    totalValue,
		PricePerShare: pricePerShare,
	})
}

func (s *Server) handleGetTickerHistory(w http.ResponseWriter, r *http.Request) {
	ticker := strings.ToUpper(chi.URLParam(r, "ticker"))
	if ticker == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "ticker is required")
		return
	}

	// Default to 1 year of history.
	period := r.URL.Query().Get("period")
	var fromTime time.Time
	now := time.Now()
	switch period {
	case "5y":
		fromTime = now.AddDate(-5, 0, 0)
	case "3y":
		fromTime = now.AddDate(-3, 0, 0)
	case "6m":
		fromTime = now.AddDate(0, -6, 0)
	case "3m":
		fromTime = now.AddDate(0, -3, 0)
	default:
		fromTime = now.AddDate(-1, 0, 0)
	}

	// Chart the symbol the holding is priced under (a coin in a crypto
	// account is looked up as "BTC-USD", not "BTC").
	symbol := ticker
	if holdings, err := s.holdings.ListByUser(r.Context(), userIDFromCtx(r)); err == nil {
		for _, h := range holdings {
			if h.Ticker == ticker {
				symbol = h.PriceTicker()
				break
			}
		}
	}

	candles, available, err := s.priceService.GetCandles(r.Context(), symbol, fromTime, now)
	if err != nil {
		// Log the error but return an empty-but-available response so the
		// frontend shows "no data" rather than a misleading error message.
		log.Printf("warn: GetCandles %s: %v", ticker, err)
		writeJSON(w, http.StatusOK, map[string]any{
			"available": true,
			"points":    []pricePoint{},
		})
		return
	}
	if !available {
		// No price provider configured — return empty with a flag.
		writeJSON(w, http.StatusOK, map[string]any{
			"available": false,
			"points":    []pricePoint{},
		})
		return
	}

	points := make([]pricePoint, len(candles))
	for i, c := range candles {
		points[i] = pricePoint{
			Date:  c.Time.Format("2006-01-02"),
			Close: c.Close,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available": true,
		"points":    points,
	})
}
