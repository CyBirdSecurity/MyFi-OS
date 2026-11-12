package services

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/data"
	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

const (
	singleTickerWarnPct     = 20.0
	singleTickerCriticalPct = 30.0
	assetClassDriftWarnPct  = 5.0
	assetClassDriftCritPct  = 10.0
	sectorOverlapInfoPct    = 50.0
	sectorOverlapWarnPct    = 75.0
)

// ConcentrationFlag describes one thing worth the user's attention about how
// their portfolio is distributed.
type ConcentrationFlag struct {
	Type       string   `json:"type"`     // "single_ticker" | "asset_class_drift" | "redundant_ticker" | "sector_overlap"
	Severity   string   `json:"severity"` // "info" | "warning" | "critical"
	Ticker     string   `json:"ticker,omitempty"`
	Tickers    []string `json:"tickers,omitempty"` // the pair involved in a "sector_overlap" flag
	AssetClass string   `json:"assetClass,omitempty"`
	Pct        float64  `json:"pct"`
	Message    string   `json:"message"`
	Accounts   []string `json:"accounts,omitempty"`
}

type ConcentrationResult struct {
	Flags      []ConcentrationFlag `json:"flags"`
	TotalValue float64             `json:"totalValue"`
	// HasFundCompositionData is true when at least one held ticker has known
	// sector-composition data, so the frontend's experimental sector-overlap
	// sub-section has something to say (even if no overlap is found).
	HasFundCompositionData bool `json:"hasFundCompositionData"`
}

// ConcentrationService flags portfolio concentration risk from ticker value,
// asset class, and allocation targets, plus an experimental sector-overlap
// check (fundOverlapFlags) against a small curated table of fund sector
// compositions (internal/data/fund_compositions.go) for the ~15-20 most
// commonly held ETFs.
type ConcentrationService struct {
	networth          *NetWorthService
	allocationTargets *db.AllocationTargetStore
}

func NewConcentrationService(networth *NetWorthService, allocationTargets *db.AllocationTargetStore) *ConcentrationService {
	return &ConcentrationService{networth: networth, allocationTargets: allocationTargets}
}

type tickerAggregate struct {
	assetClass string
	totalValue float64
	accounts   map[uuid.UUID]string // accountID -> account name
	label      string               // what messages call this holding (see holdingLabel); empty = the ticker
	// diversified marks a fund (ETF, mutual fund, 401(k) trust or pool), which
	// holds many companies and so isn't flagged as a single position.
	diversified bool
}

func (a *tickerAggregate) labelOr(ticker string) string {
	if a.label != "" {
		return a.label
	}
	return ticker
}

// holdingLabel is how a holding is named in messages: its ticker, or its
// saved name when the ticker is a stand-in. Securities without a trading
// symbol (common for 401(k) collective trusts) are stored under their CUSIP
// or a made-up "PLAID-XXXXXXXX" id, which means nothing to the user.
func holdingLabel(ticker string, name *string) string {
	if name != nil && strings.TrimSpace(*name) != "" && isPlaceholderTicker(ticker) {
		return strings.TrimSpace(*name)
	}
	return ticker
}

// fundNameRe matches names that describe a pooled fund rather than one company.
var fundNameRe = regexp.MustCompile(`(?i)\b(fund|trust|pool|index|portfolio|target|collective|stable value|retirement)\b`)

// isDiversifiedFund reports whether a holding is a fund holding many companies:
// an ETF or mutual fund per its live quote, a security Plaid gave neither a
// ticker nor a CUSIP (almost always a 401(k) collective trust or pool), or a
// CUSIP-only security whose name says it's a fund. Anything unknown — a
// stock, a missing quote — is treated as a single position.
func isDiversifiedFund(h models.Holding) bool {
	switch h.InstrumentType {
	case "ETF", "MUTUALFUND":
		return true
	}
	if strings.HasPrefix(h.Ticker, "PLAID-") {
		return true
	}
	return isPlaceholderTicker(h.Ticker) && h.Name != nil && fundNameRe.MatchString(*h.Name)
}

// isPlaceholderTicker reports whether a ticker was made up from a Plaid
// security id or a CUSIP (nine characters, at least one digit) rather than
// being a real trading symbol.
func isPlaceholderTicker(ticker string) bool {
	if strings.HasPrefix(ticker, "PLAID-") {
		return true
	}
	if len(ticker) != 9 || !strings.ContainsAny(ticker, "0123456789") {
		return false
	}
	for _, r := range ticker {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

func (s *ConcentrationService) Compute(ctx context.Context, userID uuid.UUID) (*ConcentrationResult, error) {
	holdings, err := s.networth.HoldingsWithPrices(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load holdings: %w", err)
	}
	nw, err := s.networth.Current(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load net worth: %w", err)
	}

	accountNames := make(map[uuid.UUID]string, len(nw.Accounts))
	for _, a := range nw.Accounts {
		accountNames[a.AccountID] = a.Name
	}

	byTicker := map[string]*tickerAggregate{}
	for _, h := range holdings {
		agg, ok := byTicker[h.Ticker]
		if !ok {
			agg = &tickerAggregate{assetClass: h.AssetClass, accounts: map[uuid.UUID]string{}, label: holdingLabel(h.Ticker, h.Name)}
			byTicker[h.Ticker] = agg
		}
		if isDiversifiedFund(h) {
			agg.diversified = true
		}
		agg.totalValue += h.TotalValue
		if name, ok := accountNames[h.AccountID]; ok {
			agg.accounts[h.AccountID] = name
		}
	}

	// Fold checking/savings balances into a synthetic CASH bucket, mirroring
	// the aggregated view already shown on the Holdings page.
	for _, a := range nw.Accounts {
		if (a.AccountType != "checking" && a.AccountType != "savings") || a.Balance <= 0 {
			continue
		}
		agg, ok := byTicker["CASH"]
		if !ok {
			agg = &tickerAggregate{assetClass: "cash_equivalent", accounts: map[uuid.UUID]string{}}
			byTicker["CASH"] = agg
		}
		agg.totalValue += a.Balance
		agg.accounts[a.AccountID] = a.Name
	}

	var totalValue float64
	for _, agg := range byTicker {
		totalValue += agg.totalValue
	}

	hasCompositionData := false
	for ticker := range byTicker {
		if _, ok := data.FundCompositions[ticker]; ok {
			hasCompositionData = true
			break
		}
	}

	var flags []ConcentrationFlag
	if totalValue > 0 {
		flags = append(flags, singleTickerAndRedundancyFlags(byTicker, totalValue)...)
		flags = append(flags, fundOverlapFlags(byTicker, totalValue)...)

		targets, _ := s.allocationTargets.Get(ctx, userID)
		if targets != nil {
			flags = append(flags, assetClassDriftFlags(targets, byTicker, totalValue)...)
		}
	}

	severityRank := map[string]int{"critical": 0, "warning": 1, "info": 2}
	sort.SliceStable(flags, func(i, j int) bool { return severityRank[flags[i].Severity] < severityRank[flags[j].Severity] })

	return &ConcentrationResult{Flags: flags, TotalValue: totalValue, HasFundCompositionData: hasCompositionData}, nil
}

// fundOverlapFlags flags pairs of distinct held tickers whose underlying
// sector composition overlaps heavily — e.g. VTI and VOO, or QQQ and VGT —
// which add far less diversification than two different tickers might
// suggest. Only pairs where both tickers have known composition data
// (internal/data/fund_compositions.go) are considered.
func fundOverlapFlags(byTicker map[string]*tickerAggregate, totalValue float64) []ConcentrationFlag {
	tickers := make([]string, 0, len(byTicker))
	for t := range byTicker {
		if _, ok := data.FundCompositions[t]; ok {
			tickers = append(tickers, t)
		}
	}
	sort.Strings(tickers)

	var flags []ConcentrationFlag
	for i := 0; i < len(tickers); i++ {
		for j := i + 1; j < len(tickers); j++ {
			a, b := tickers[i], tickers[j]
			overlapPct := sectorOverlapPct(data.FundCompositions[a].SectorWeights, data.FundCompositions[b].SectorWeights)

			var severity string
			switch {
			case overlapPct > sectorOverlapWarnPct:
				severity = "warning"
			case overlapPct > sectorOverlapInfoPct:
				severity = "info"
			default:
				continue
			}

			combinedPct := ((byTicker[a].totalValue + byTicker[b].totalValue) / totalValue) * 100
			flags = append(flags, ConcentrationFlag{
				Type: "sector_overlap", Severity: severity, Tickers: []string{a, b}, Pct: overlapPct,
				Message: fmt.Sprintf("%s and %s share %.0f%% overlapping sector exposure (%.1f%% of your portfolio combined) — holding both adds limited diversification.", a, b, overlapPct, combinedPct),
			})
		}
	}
	return flags
}

// sectorOverlapPct returns the percentage of two funds' sector weightings
// that overlap — the sum, across sectors, of the smaller of the two funds'
// weights in that sector. Two identical funds score 100; two funds with no
// shared sector exposure score 0.
func sectorOverlapPct(a, b map[string]float64) float64 {
	overlap := 0.0
	for sector, weightA := range a {
		if weightB, ok := b[sector]; ok {
			overlap += math.Min(weightA, weightB)
		}
	}
	return overlap
}

func singleTickerAndRedundancyFlags(byTicker map[string]*tickerAggregate, totalValue float64) []ConcentrationFlag {
	tickers := make([]string, 0, len(byTicker))
	for t := range byTicker {
		tickers = append(tickers, t)
	}
	sort.Strings(tickers)

	var flags []ConcentrationFlag
	for _, ticker := range tickers {
		agg := byTicker[ticker]
		pct := (agg.totalValue / totalValue) * 100

		// The synthetic CASH bucket isn't a "position" in the concentration-risk
		// sense, and a fund already spreads its money across many companies, so
		// both are excluded from single-ticker flagging.
		if ticker != "CASH" && !agg.diversified {
			switch {
			case pct > singleTickerCriticalPct:
				flags = append(flags, ConcentrationFlag{
					Type: "single_ticker", Severity: "critical", Ticker: ticker, Pct: pct,
					Message: fmt.Sprintf("%s makes up %.1f%% of your portfolio — a decline in this single position would have an outsized impact.", agg.labelOr(ticker), pct),
				})
			case pct > singleTickerWarnPct:
				flags = append(flags, ConcentrationFlag{
					Type: "single_ticker", Severity: "warning", Ticker: ticker, Pct: pct,
					Message: fmt.Sprintf("%s makes up %.1f%% of your portfolio.", agg.labelOr(ticker), pct),
				})
			}
		}

		if len(agg.accounts) >= 2 {
			names := make([]string, 0, len(agg.accounts))
			for _, name := range agg.accounts {
				names = append(names, name)
			}
			sort.Strings(names)
			flags = append(flags, ConcentrationFlag{
				Type: "redundant_ticker", Severity: "info", Ticker: ticker, Pct: pct, Accounts: names,
				Message: fmt.Sprintf("%s is held across %d accounts: %s.", agg.labelOr(ticker), len(names), strings.Join(names, ", ")),
			})
		}
	}
	return flags
}

func assetClassDriftFlags(targets *models.AllocationTarget, byTicker map[string]*tickerAggregate, totalValue float64) []ConcentrationFlag {
	valueByClass := map[string]float64{}
	for _, agg := range byTicker {
		valueByClass[agg.assetClass] += agg.totalValue
	}

	classTargets := []struct {
		assetClass string
		label      string
		targetPct  float64
	}{
		{"equity", "Equity", targets.EquityPct},
		{"bond", "Bonds", targets.BondPct},
		{"crypto", "Crypto", targets.CryptoPct},
		{"cash_equivalent", "Cash", targets.CashPct},
		{"other", "Other", targets.OtherPct},
	}

	var flags []ConcentrationFlag
	for _, ct := range classTargets {
		if ct.targetPct <= 0 {
			continue
		}
		actualPct := (valueByClass[ct.assetClass] / totalValue) * 100
		deviation := actualPct - ct.targetPct
		abs := math.Abs(deviation)
		if abs <= assetClassDriftWarnPct {
			continue
		}
		severity := "warning"
		if abs > assetClassDriftCritPct {
			severity = "critical"
		}
		direction := "above"
		if deviation < 0 {
			direction = "below"
		}
		flags = append(flags, ConcentrationFlag{
			Type: "asset_class_drift", Severity: severity, AssetClass: ct.assetClass, Pct: deviation,
			Message: fmt.Sprintf("%s is %.1f%% %s target (%.1f%% actual vs %.0f%% target).", ct.label, abs, direction, actualPct, ct.targetPct),
		})
	}
	return flags
}
