package services

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

func hasOverlapFlag(flags []ConcentrationFlag, a, b string) *ConcentrationFlag {
	for i := range flags {
		f := &flags[i]
		if f.Type != "sector_overlap" || len(f.Tickers) != 2 {
			continue
		}
		if (f.Tickers[0] == a && f.Tickers[1] == b) || (f.Tickers[0] == b && f.Tickers[1] == a) {
			return f
		}
	}
	return nil
}

func hasFlag(flags []ConcentrationFlag, flagType, severity, ticker string) bool {
	for _, f := range flags {
		if f.Type == flagType && f.Severity == severity && (ticker == "" || f.Ticker == ticker) {
			return true
		}
	}
	return false
}

func TestSingleTickerAndRedundancyFlags_Concentration(t *testing.T) {
	acct1, acct2 := uuid.New(), uuid.New()

	byTicker := map[string]*tickerAggregate{
		"AAPL": {assetClass: "equity", totalValue: 35_000, accounts: map[uuid.UUID]string{acct1: "Brokerage"}},
		"VTI":  {assetClass: "equity", totalValue: 25_000, accounts: map[uuid.UUID]string{acct1: "Brokerage", acct2: "401k"}},
		"BND":  {assetClass: "bond", totalValue: 10_000, accounts: map[uuid.UUID]string{acct2: "401k"}},
		"CASH": {assetClass: "cash_equivalent", totalValue: 30_000, accounts: map[uuid.UUID]string{acct1: "Checking"}},
	}
	totalValue := 100_000.0 // 35k + 25k + 10k + 30k

	flags := singleTickerAndRedundancyFlags(byTicker, totalValue)

	if !hasFlag(flags, "single_ticker", "critical", "AAPL") {
		t.Errorf("expected AAPL (35%%) to be flagged single_ticker/critical, got %+v", flags)
	}
	if !hasFlag(flags, "single_ticker", "warning", "VTI") {
		t.Errorf("expected VTI (25%%) to be flagged single_ticker/warning, got %+v", flags)
	}
	if hasFlag(flags, "single_ticker", "", "BND") {
		t.Errorf("did not expect BND (10%%) to be flagged single_ticker, got %+v", flags)
	}
	if hasFlag(flags, "single_ticker", "", "CASH") {
		t.Errorf("CASH bucket should never be single_ticker-flagged, got %+v", flags)
	}
	if !hasFlag(flags, "redundant_ticker", "info", "VTI") {
		t.Errorf("expected VTI to be flagged redundant_ticker (held in 2 accounts), got %+v", flags)
	}
	if hasFlag(flags, "redundant_ticker", "", "AAPL") {
		t.Errorf("did not expect AAPL (1 account) to be flagged redundant_ticker, got %+v", flags)
	}
}

func TestAssetClassDriftFlags_Concentration(t *testing.T) {
	byTicker := map[string]*tickerAggregate{
		"VTI": {assetClass: "equity", totalValue: 90_000},
		"BND": {assetClass: "bond", totalValue: 10_000},
	}
	totalValue := 100_000.0

	// Target: 70% equity / 30% bond. Actual: 90% equity / 10% bond.
	// Equity is +20pp over target (critical, >10pp), bond is -20pp under (critical).
	targets := &models.AllocationTarget{EquityPct: 70, BondPct: 30}

	flags := assetClassDriftFlags(targets, byTicker, totalValue)

	if !hasFlag(flags, "asset_class_drift", "critical", "") {
		t.Fatalf("expected at least one critical asset_class_drift flag, got %+v", flags)
	}
	equityFlags := 0
	for _, f := range flags {
		if f.AssetClass == "equity" {
			equityFlags++
			if f.Severity != "critical" {
				t.Errorf("expected equity drift to be critical, got %s", f.Severity)
			}
		}
	}
	if equityFlags != 1 {
		t.Errorf("expected exactly one equity drift flag, got %d", equityFlags)
	}
}

func TestAssetClassDriftFlags_WithinTolerance(t *testing.T) {
	byTicker := map[string]*tickerAggregate{
		"VTI": {assetClass: "equity", totalValue: 72_000},
		"BND": {assetClass: "bond", totalValue: 28_000},
	}
	totalValue := 100_000.0

	// Target 70/30, actual 72/28 — within the ±5pp tolerance, should produce no flags.
	targets := &models.AllocationTarget{EquityPct: 70, BondPct: 30}

	flags := assetClassDriftFlags(targets, byTicker, totalValue)
	if len(flags) != 0 {
		t.Errorf("expected no drift flags within tolerance, got %+v", flags)
	}
}

// TestFundOverlapFlags_HighOverlapWarns asserts that two broad-market funds
// with near-identical sector composition (VTI and VOO both track ~the whole
// U.S. equity market) are flagged, since holding both adds little
// diversification.
func TestFundOverlapFlags_HighOverlapWarns(t *testing.T) {
	byTicker := map[string]*tickerAggregate{
		"VTI": {assetClass: "equity", totalValue: 50_000},
		"VOO": {assetClass: "equity", totalValue: 50_000},
	}
	totalValue := 100_000.0

	flags := fundOverlapFlags(byTicker, totalValue)

	flag := hasOverlapFlag(flags, "VTI", "VOO")
	if flag == nil {
		t.Fatalf("expected a sector_overlap flag between VTI and VOO, got %+v", flags)
	}
	if flag.Severity != "warning" {
		t.Errorf("expected VTI/VOO overlap to be severity warning (broad-market funds overlap heavily), got %s (pct=%.1f)", flag.Severity, flag.Pct)
	}
}

// TestFundOverlapFlags_LowOverlapNoFlag asserts that two funds with
// distinctly different sector tilts (VGT is 100% technology; SCHD is a
// dividend-value fund with only ~9% technology) are not flagged.
func TestFundOverlapFlags_LowOverlapNoFlag(t *testing.T) {
	byTicker := map[string]*tickerAggregate{
		"VGT":  {assetClass: "equity", totalValue: 50_000},
		"SCHD": {assetClass: "equity", totalValue: 50_000},
	}
	totalValue := 100_000.0

	flags := fundOverlapFlags(byTicker, totalValue)

	if flag := hasOverlapFlag(flags, "VGT", "SCHD"); flag != nil {
		t.Errorf("did not expect VGT/SCHD to be flagged as overlapping (distinct sector tilts), got %+v", flag)
	}
}

// TestFundOverlapFlags_SkipsUnknownTickers asserts that tickers absent from
// the curated FundCompositions table are silently excluded rather than
// erroring or being compared against.
func TestFundOverlapFlags_SkipsUnknownTickers(t *testing.T) {
	byTicker := map[string]*tickerAggregate{
		"VTI":                 {assetClass: "equity", totalValue: 50_000},
		"SOME_OBSCURE_TICKER": {assetClass: "equity", totalValue: 50_000},
	}
	totalValue := 100_000.0

	flags := fundOverlapFlags(byTicker, totalValue)
	if len(flags) != 0 {
		t.Errorf("expected no overlap flags when only one ticker has known composition data, got %+v", flags)
	}
}

func TestHoldingLabel(t *testing.T) {
	name := "Fidelity Freedom 2050 Commingled Pool"
	for _, tc := range []struct {
		ticker string
		name   *string
		want   string
	}{
		{"PLAID-RJ6M41MY", &name, name},
		{"31617E471", &name, name}, // CUSIP stand-in
		{"VTI", &name, "VTI"},
		{"PLAID-RJ6M41MY", nil, "PLAID-RJ6M41MY"},
		{"BRK.B", &name, "BRK.B"},
	} {
		if got := holdingLabel(tc.ticker, tc.name); got != tc.want {
			t.Errorf("holdingLabel(%q) = %q, want %q", tc.ticker, got, tc.want)
		}
	}
}

func TestSingleTickerFlagUsesFundNameForStandInTicker(t *testing.T) {
	byTicker := map[string]*tickerAggregate{
		"PLAID-RJ6M41MY": {assetClass: "equity", totalValue: 40, accounts: map[uuid.UUID]string{}, label: "Target Date 2050 Trust"},
		"VTI":            {assetClass: "equity", totalValue: 60, accounts: map[uuid.UUID]string{}},
	}
	flags := singleTickerAndRedundancyFlags(byTicker, 100)
	var msgs []string
	for _, f := range flags {
		msgs = append(msgs, f.Message)
	}
	joined := strings.Join(msgs, "\n")
	if !strings.Contains(joined, "Target Date 2050 Trust makes up 40.0%") || strings.Contains(joined, "PLAID-") {
		t.Errorf("messages = %q, want the fund name instead of the stand-in ticker", joined)
	}
	if !strings.Contains(joined, "VTI makes up 60.0%") {
		t.Errorf("messages = %q, want VTI by ticker", joined)
	}
}

func TestIsDiversifiedFund(t *testing.T) {
	str := func(s string) *string { return &s }
	for _, tc := range []struct {
		h    models.Holding
		want bool
	}{
		{models.Holding{Ticker: "VTI", InstrumentType: "ETF"}, true},
		{models.Holding{Ticker: "FXAIX", InstrumentType: "MUTUALFUND"}, true},
		{models.Holding{Ticker: "AAPL", InstrumentType: "EQUITY"}, false},
		{models.Holding{Ticker: "AAPL"}, false}, // no quote: treat as a single position
		{models.Holding{Ticker: "PLAID-RJ6M41MY", Name: str("Target Date 2050 Trust")}, true},
		{models.Holding{Ticker: "PLAID-AB12CD34"}, true},
		{models.Holding{Ticker: "31617E471", Name: str("Stable Value Fund")}, true},
		{models.Holding{Ticker: "912828YK0", Name: str("US Treasury Note 1.5% 2029")}, false},
	} {
		if got := isDiversifiedFund(tc.h); got != tc.want {
			t.Errorf("isDiversifiedFund(%s) = %v, want %v", tc.h.Ticker, got, tc.want)
		}
	}
}

func TestSingleTickerFlagsSkipFunds(t *testing.T) {
	byTicker := map[string]*tickerAggregate{
		"VTI":  {assetClass: "equity", totalValue: 70, accounts: map[uuid.UUID]string{}, diversified: true},
		"NVDA": {assetClass: "equity", totalValue: 30, accounts: map[uuid.UUID]string{}},
	}
	flags := singleTickerAndRedundancyFlags(byTicker, 100)
	if len(flags) != 1 || flags[0].Ticker != "NVDA" {
		t.Fatalf("flags = %+v, want only the NVDA stock flagged", flags)
	}
}
