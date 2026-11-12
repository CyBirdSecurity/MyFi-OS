package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/data"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// computeAssetClassWeights returns, for each account, the fraction of that
// account's value attributed to each asset class — holdings-weighted when
// the account has priced holdings, falling back to the account-type default
// (100% weight in one class) otherwise. This mirrors the fallback logic in
// computeEffectiveRates, but returns a weight distribution instead of a
// single blended rate: a historical backtest needs to look up a separate
// return series per asset class rather than one blended number. Also used by
// runMonteCarlo, which draws one random shock per asset class per year and
// scales it by these weights so same-class accounts move together.
//
// This intentionally ignores Account.ReturnRateOverride — that's a rate
// override, not an asset-class reassignment, and has no meaning against a
// historical return series keyed by asset class.
//
// NOTE: mirrors computeEffectiveRates' holding filter (`PricePerShare != nil
// && > 0`) exactly, including its existing quirk of excluding cash_equivalent
// holdings from the holdings-weighted branch (those typically have no
// PricePerShare set, since Shares stores the dollar amount directly for cash
// positions).
func computeAssetClassWeights(accounts []models.Account, holdings []models.Holding) map[uuid.UUID]map[string]float64 {
	type acClassValue struct {
		assetClass string
		value      float64
	}
	holdingsByAccount := make(map[uuid.UUID][]acClassValue)
	for _, h := range holdings {
		if v, ok := holdingValue(h); ok && v > 0 {
			holdingsByAccount[h.AccountID] = append(holdingsByAccount[h.AccountID], acClassValue{h.AssetClass, v})
		}
	}

	weights := make(map[uuid.UUID]map[string]float64, len(accounts))
	for _, acc := range accounts {
		if hvs := holdingsByAccount[acc.ID]; len(hvs) > 0 {
			total := 0.0
			for _, hv := range hvs {
				total += hv.value
			}
			w := make(map[string]float64)
			if total > 0 {
				for _, hv := range hvs {
					w[hv.assetClass] += hv.value / total
				}
			}
			weights[acc.ID] = w
			continue
		}

		assetClass := models.AccountTypeDefaultAssetClass[acc.AccountType]
		weights[acc.ID] = map[string]float64{assetClass: 1.0}
	}

	return weights
}

// HistoricalRunResult is the outcome of running the plan starting from one
// real historical year.
type HistoricalRunResult struct {
	StartYear int            `json:"startYear"`
	Years     []ForecastYear `json:"years"`
	Survived  bool           `json:"survived"`
}

// BacktestResult summarizes replaying the plan across every eligible
// historical starting year.
type BacktestResult struct {
	Runs             []HistoricalRunResult `json:"runs"`
	SurvivalPct      float64               `json:"survivalPct"`
	DataCaveats      []string              `json:"dataCaveats"`
	EarliestDataYear int                   `json:"earliestDataYear"`
	LatestDataYear   int                   `json:"latestDataYear"`
}

// assetClassHistoricalReturn maps an asset class to its return for a given
// historical year. Crypto and "other" have no long historical series, so
// they're proxied with the equity series (surfaced via BacktestResult.DataCaveats
// rather than silently blended in).
func assetClassHistoricalReturn(hy data.HistoricalYear, assetClass string) float64 {
	switch assetClass {
	case "bond":
		return hy.BondReturn
	case "cash_equivalent":
		return hy.TBillReturn
	default: // equity, crypto, other
		return hy.EquityReturn
	}
}

// RunHistoricalBacktest replays the user's current plan against every real
// historical rolling window (1928-present) that has enough remaining data to
// cover the full projection horizon. Start years too close to the present are
// truncated rather than wrapped to an earlier year — wrapping e.g. 2023's
// horizon into 1928 would misrepresent what "surviving the Depression" means.
func (s *ForecastService) RunHistoricalBacktest(ctx context.Context, userID uuid.UUID) (*BacktestResult, error) {
	inp, err := s.loadForecastInputs(ctx, userID)
	if err != nil {
		return nil, err
	}

	contribsByAccount := make(map[uuid.UUID][]models.Contribution)
	for _, c := range inp.allContribs {
		if c.IsActive {
			contribsByAccount[c.AccountID] = append(contribsByAccount[c.AccountID], c)
		}
	}

	return s.runHistoricalBacktestCore(
		inp.allAccounts, inp.allHoldings, inp.startingBalances, contribsByAccount,
		inp.homeDetails, inp.profile, inp.spouseProfile,
		inp.currentYear, inp.endYear, inp.primaryBirthYear, inp.spouseBirthYear,
		inp.spendGoals, inp.liabilities,
	), nil
}

// runHistoricalBacktestCore is the data-independent backtest loop, split out
// from RunHistoricalBacktest so it can be tested without a database — mirrors
// the simulate/simulateCore split used elsewhere in this package.
func (s *ForecastService) runHistoricalBacktestCore(
	accounts []models.Account,
	holdings []models.Holding,
	startingBalances map[uuid.UUID]float64,
	contribsByAccount map[uuid.UUID][]models.Contribution,
	homeDetails map[uuid.UUID]*models.HomeDetails,
	profile *models.UserProfile,
	spouseProfile *models.UserProfile,
	currentYear, endYear int,
	primaryBirthYear int,
	spouseBirthYear *int,
	spendGoals []models.Goal,
	liabilities map[uuid.UUID]liabilityTerms,
) *BacktestResult {
	horizonYears := endYear - currentYear + 1
	numDataYears := len(data.HistoricalReturns)
	maxStartIdx := numDataYears - horizonYears
	if horizonYears <= 0 || maxStartIdx < 0 {
		return &BacktestResult{}
	}

	weights := computeAssetClassWeights(accounts, holdings)

	runs := make([]HistoricalRunResult, 0, maxStartIdx+1)
	survived := 0
	for startIdx := 0; startIdx <= maxStartIdx; startIdx++ {
		years := s.simulateCore(
			accounts, startingBalances, contribsByAccount,
			func(id uuid.UUID, yearIndex int) float64 {
				hy := data.HistoricalReturns[startIdx+yearIndex]
				rate := 0.0
				for assetClass, weight := range weights[id] {
					rate += weight * assetClassHistoricalReturn(hy, assetClass)
				}
				return rate
			},
			homeDetails, profile, spouseProfile,
			currentYear, endYear, primaryBirthYear, spouseBirthYear,
			spendGoals, liabilities,
		)

		ok := spendingMet(years)
		if ok {
			survived++
		}
		runs = append(runs, HistoricalRunResult{
			StartYear: data.HistoricalReturns[startIdx].Year,
			Years:     years,
			Survived:  ok,
		})
	}

	return &BacktestResult{
		Runs:        runs,
		SurvivalPct: float64(survived) / float64(len(runs)) * 100,
		DataCaveats: []string{
			`Crypto and "other" asset classes have no long historical return series and are modeled using U.S. equity returns as a proxy.`,
		},
		EarliestDataYear: runs[0].StartYear,
		LatestDataYear:   runs[len(runs)-1].StartYear,
	}
}
