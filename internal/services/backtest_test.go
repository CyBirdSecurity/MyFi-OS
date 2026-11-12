package services

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/data"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

func TestComputeAssetClassWeights_HoldingsWeighted(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	price := 100.0

	holdings := []models.Holding{
		{AccountID: acc.ID, Ticker: "VTI", AssetClass: "equity", Shares: 60, PricePerShare: &price}, // $6,000
		{AccountID: acc.ID, Ticker: "BND", AssetClass: "bond", Shares: 40, PricePerShare: &price},   // $4,000
	}

	weights := computeAssetClassWeights([]models.Account{acc}, holdings)

	w := weights[acc.ID]
	if !approxEqual(w["equity"]*100, 60) {
		t.Errorf("expected equity weight 0.60, got %.4f", w["equity"])
	}
	if !approxEqual(w["bond"]*100, 40) {
		t.Errorf("expected bond weight 0.40, got %.4f", w["bond"])
	}
}

func TestComputeAssetClassWeights_AccountTypeFallback(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "roth_ira") // no holdings — should fall back to account-type default

	weights := computeAssetClassWeights([]models.Account{acc}, nil)

	w := weights[acc.ID]
	defaultClass := models.AccountTypeDefaultAssetClass["roth_ira"]
	if w[defaultClass] != 1.0 {
		t.Errorf("expected 100%% weight in account-type default class %q, got %+v", defaultClass, w)
	}
}

// TestRunHistoricalBacktestCore_RateMatchesEmbeddedDataset asserts that, for a
// single-year horizon, every eligible start year's return comes directly from
// the embedded dataset rather than some derived/blended value.
func TestRunHistoricalBacktestCore_RateMatchesEmbeddedDataset(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "roth_ira") // no holdings -> 100% equity default
	profile := newProfile(65, 0)       // retire far in the future; no withdrawals

	currentYear := time.Now().Year() - 1 // a past year avoids current-year proration
	startingBalances := map[uuid.UUID]float64{acc.ID: 100_000}

	result := svc().runHistoricalBacktestCore(
		[]models.Account{acc}, nil, startingBalances, nil,
		nil, profile, nil,
		currentYear, currentYear, // 1-year horizon
		profile.Birthday.Year(), nil, nil, nil,
	)

	numData := len(data.HistoricalReturns)
	if len(result.Runs) != numData {
		t.Fatalf("expected %d eligible start years for a 1-year horizon, got %d", numData, len(result.Runs))
	}

	first := result.Runs[0]
	if first.StartYear != data.HistoricalReturns[0].Year {
		t.Errorf("expected first run to start in %d, got %d", data.HistoricalReturns[0].Year, first.StartYear)
	}
	wantTotal := 100_000 * (1 + data.HistoricalReturns[0].EquityReturn)
	if !approxEqual(first.Years[0].Total, wantTotal) {
		t.Errorf("expected %d's ending balance to use that year's embedded equity return: want %.2f, got %.2f",
			data.HistoricalReturns[0].Year, wantTotal, first.Years[0].Total)
	}

	if result.EarliestDataYear != data.HistoricalReturns[0].Year {
		t.Errorf("expected EarliestDataYear %d, got %d", data.HistoricalReturns[0].Year, result.EarliestDataYear)
	}
	if result.LatestDataYear != data.HistoricalReturns[numData-1].Year {
		t.Errorf("expected LatestDataYear %d, got %d", data.HistoricalReturns[numData-1].Year, result.LatestDataYear)
	}
}

// TestRunHistoricalBacktestCore_TruncatesStartYearsTooCloseToPresent asserts
// that start years without enough remaining historical data to cover the full
// projection horizon are excluded (truncated), not wrapped to an earlier year.
func TestRunHistoricalBacktestCore_TruncatesStartYearsTooCloseToPresent(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "roth_ira")
	profile := newProfile(65, 0)
	startingBalances := map[uuid.UUID]float64{acc.ID: 100_000}

	numData := len(data.HistoricalReturns)
	currentYear := time.Now().Year() - 1

	// Horizon spans exactly the whole embedded dataset: only the earliest
	// start year has enough remaining data to run to completion.
	endYear := currentYear + numData - 1
	result := svc().runHistoricalBacktestCore(
		[]models.Account{acc}, nil, startingBalances, nil,
		nil, profile, nil,
		currentYear, endYear,
		profile.Birthday.Year(), nil, nil, nil,
	)
	if len(result.Runs) != 1 {
		t.Fatalf("expected exactly 1 eligible start year when horizon spans the full dataset, got %d", len(result.Runs))
	}
	if result.Runs[0].StartYear != data.HistoricalReturns[0].Year {
		t.Errorf("expected the only eligible start year to be %d, got %d", data.HistoricalReturns[0].Year, result.Runs[0].StartYear)
	}

	// One year longer than the dataset can cover: no start year survives truncation.
	result = svc().runHistoricalBacktestCore(
		[]models.Account{acc}, nil, startingBalances, nil,
		nil, profile, nil,
		currentYear, endYear+1,
		profile.Birthday.Year(), nil, nil, nil,
	)
	if len(result.Runs) != 0 {
		t.Errorf("expected 0 eligible start years once the horizon exceeds the dataset, got %d", len(result.Runs))
	}
}
