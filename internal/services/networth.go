package services

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

type NetWorthService struct {
	accounts  *db.AccountStore
	snapshots *db.SnapshotStore
	holdings  *db.HoldingStore
	prices    *PriceService
	home      *db.HomeStore
}

func NewNetWorthService(
	accounts *db.AccountStore,
	snapshots *db.SnapshotStore,
	holdings *db.HoldingStore,
	prices *PriceService,
	home *db.HomeStore,
) *NetWorthService {
	return &NetWorthService{accounts: accounts, snapshots: snapshots, holdings: holdings, prices: prices, home: home}
}

type AccountBalance struct {
	AccountID      uuid.UUID `json:"accountId"`
	Name           string    `json:"name"`
	AccountType    string    `json:"accountType"`
	TaxTreatment   string    `json:"taxTreatment"`
	Balance        float64   `json:"balance"`
	DisplayBalance *float64  `json:"displayBalance,omitempty"` // if set, used by UI instead of Balance
}

type TaxTreatmentBreakdown struct {
	TaxTreatment    string  `json:"taxTreatment"`
	Total           float64 `json:"total"`
	PercentOfAssets float64 `json:"percentOfAssets"`
}

type AccountTypeBreakdown struct {
	AccountType     string  `json:"accountType"`
	TaxTreatment    string  `json:"taxTreatment"`
	Total           float64 `json:"total"`
	PercentOfAssets float64 `json:"percentOfAssets"`
}

type CurrentNetWorth struct {
	TotalNetWorth    float64                 `json:"totalNetWorth"`
	TotalAssets      float64                 `json:"totalAssets"`
	TotalLiabilities float64                 `json:"totalLiabilities"`
	ByTaxTreatment   []TaxTreatmentBreakdown `json:"byTaxTreatment"`
	ByAccountType    []AccountTypeBreakdown  `json:"byAccountType"`
	Accounts         []AccountBalance        `json:"accounts"`
	AsOf             time.Time               `json:"asOf"`
}

func (s *NetWorthService) Current(ctx context.Context, userID uuid.UUID) (*CurrentNetWorth, error) {
	accs, err := s.accounts.List(ctx, userID)
	if err != nil {
		return nil, err
	}

	latestSnaps, err := s.snapshots.LatestPerAccount(ctx, userID)
	if err != nil {
		return nil, err
	}
	snapByAccount := make(map[uuid.UUID]float64, len(latestSnaps))
	for _, snap := range latestSnaps {
		snapByAccount[snap.AccountID] = snap.Balance
	}

	// Collect tickers from investment/retirement/brokerage/crypto accounts.
	allHoldings, err := s.holdings.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	tickers := make([]string, 0, len(allHoldings))
	seen := map[string]bool{}
	for _, h := range allHoldings {
		// Cash holdings and manually-priced holdings don't need a live price fetch.
		if h.AssetClass == "cash_equivalent" || h.ManualPrice {
			continue
		}
		if t := h.PriceTicker(); !seen[t] {
			tickers = append(tickers, t)
			seen[t] = true
		}
	}

	priceMap, _ := s.prices.GetPricesAllowStale(ctx, tickers)

	// Compute holding value per account.
	holdingValueByAccount := make(map[uuid.UUID]float64)
	for i := range allHoldings {
		h := &allHoldings[i]
		if h.AssetClass == "cash_equivalent" {
			// Shares field stores the dollar amount directly for cash positions.
			h.TotalValue = h.Shares
		} else if !h.ManualPrice {
			if q, ok := priceMap[h.PriceTicker()]; ok {
				h.PricePerShare = &q.Price
				h.TotalValue = h.Shares * q.Price
			} else if h.PricePerShare != nil {
				h.TotalValue = h.Shares * *h.PricePerShare
			}
		} else if h.PricePerShare != nil {
			h.TotalValue = h.Shares * *h.PricePerShare
		}
		holdingValueByAccount[h.AccountID] += h.TotalValue
	}

	// Load home details for any home accounts so we can honour TrackMortgageAsLiability.
	homeDetailsByAccount := make(map[uuid.UUID]*models.HomeDetails)
	for _, acc := range accs {
		if acc.AccountType == "home" {
			if d, err := s.home.Get(ctx, acc.ID, userID); err == nil {
				RollForwardLoanBalance(d, time.Now())
				homeDetailsByAccount[acc.ID] = d
			}
		}
	}

	// Build per-account balances.
	taxTreatmentTotals := map[string]float64{}
	accountTypeTotals := map[string]struct {
		tax   string
		total float64
	}{}

	balances := make([]AccountBalance, 0, len(accs))
	totalAssets := 0.0
	totalLiabilities := 0.0

	addBalance := func(b AccountBalance) {
		balances = append(balances, b)
		taxTreatmentTotals[b.TaxTreatment] += b.Balance
		if entry, ok := accountTypeTotals[b.AccountType]; ok {
			entry.total += b.Balance
			accountTypeTotals[b.AccountType] = entry
		} else {
			accountTypeTotals[b.AccountType] = struct {
				tax   string
				total float64
			}{tax: b.TaxTreatment, total: b.Balance}
		}
		if b.Balance > 0 {
			totalAssets += b.Balance
		} else {
			totalLiabilities += b.Balance
		}
	}

	for _, acc := range accs {
		var balance float64
		// Investment-type accounts: use live holding values if available.
		if hv, ok := holdingValueByAccount[acc.ID]; ok && hv != 0 {
			balance = hv
		} else if b, ok := snapByAccount[acc.ID]; ok {
			balance = b
		}
		// A home with a known value shows live equity, so it grows as the
		// rolled-forward mortgage balance falls between saved snapshots.
		if hd, ok := homeDetailsByAccount[acc.ID]; ok && hd.LastZestimate != nil && *hd.LastZestimate > 0 {
			balance = *hd.LastZestimate - hd.CurrentLoanBalance
		}

		// Home accounts with TrackMortgageAsLiability: show full property value as
		// asset and the outstanding loan balance as a separate liability entry.
		// The account card shows equity (DisplayBalance) while totals use propValue/mortgage.
		if hd, ok := homeDetailsByAccount[acc.ID]; ok && hd.TrackMortgageAsLiability && hd.CurrentLoanBalance > 0 {
			// Reconstruct property value: zestimate if available, else equity + loan.
			propValue := balance + hd.CurrentLoanBalance
			if hd.LastZestimate != nil && *hd.LastZestimate > 0 {
				propValue = *hd.LastZestimate
			}
			equity := propValue - hd.CurrentLoanBalance
			addBalance(AccountBalance{
				AccountID:      acc.ID,
				Name:           acc.Name,
				AccountType:    acc.AccountType,
				TaxTreatment:   acc.TaxTreatment,
				Balance:        propValue, // used for total assets
				DisplayBalance: &equity,   // shown on the account card
			})
			// Use a synthetic ID so the mortgage entry doesn't overwrite the home
			// account in the frontend balance map (which is keyed by accountId).
			mortgageID := mortgageSeriesID(acc.ID)
			addBalance(AccountBalance{
				AccountID:    mortgageID,
				Name:         acc.Name + " (Mortgage)",
				AccountType:  "mortgage",
				TaxTreatment: acc.TaxTreatment,
				Balance:      -hd.CurrentLoanBalance,
			})
			continue
		}

		addBalance(AccountBalance{
			AccountID:    acc.ID,
			Name:         acc.Name,
			AccountType:  acc.AccountType,
			TaxTreatment: acc.TaxTreatment,
			Balance:      balance,
		})
	}

	// Build breakdowns.
	byTax := make([]TaxTreatmentBreakdown, 0, 3)
	for _, tt := range []string{"taxable", "tax_deferred", "tax_free"} {
		total := taxTreatmentTotals[tt]
		pct := 0.0
		if totalAssets > 0 && total > 0 {
			pct = (total / totalAssets) * 100
		}
		byTax = append(byTax, TaxTreatmentBreakdown{
			TaxTreatment:    tt,
			Total:           total,
			PercentOfAssets: pct,
		})
	}

	byType := make([]AccountTypeBreakdown, 0, len(accountTypeTotals))
	for at, entry := range accountTypeTotals {
		pct := 0.0
		if totalAssets > 0 && entry.total > 0 {
			pct = (entry.total / totalAssets) * 100
		}
		byType = append(byType, AccountTypeBreakdown{
			AccountType:     at,
			TaxTreatment:    entry.tax,
			Total:           entry.total,
			PercentOfAssets: pct,
		})
	}

	return &CurrentNetWorth{
		TotalNetWorth:    totalAssets + totalLiabilities,
		TotalAssets:      totalAssets,
		TotalLiabilities: totalLiabilities,
		ByTaxTreatment:   byTax,
		ByAccountType:    byType,
		Accounts:         balances,
		AsOf:             time.Now(),
	}, nil
}

func (s *NetWorthService) History(ctx context.Context, userID uuid.UUID, from, to time.Time, interval string) ([]db.HistoryPoint, error) {
	rows, err := s.snapshots.HistoryByAccount(ctx, userID, from, to, interval)
	if err != nil {
		return nil, err
	}

	// Home accounts with TrackMortgageAsLiability store equity (property value
	// minus loan balance) as their snapshot balance. To retroactively show the
	// mortgage as a liability for the account's entire history (not just months
	// after the flag was enabled), reconstruct the loan balance as of each
	// bucket's date from the original loan terms and split the bucket's equity
	// back into property value (asset) + loan balance (liability). Net worth is
	// unaffected since propertyValue - loanBalance == equity either way.
	homeDetails, err := s.home.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	trackedHomes := make(map[uuid.UUID]*models.HomeDetails, len(homeDetails))
	for i := range homeDetails {
		if homeDetails[i].TrackMortgageAsLiability {
			trackedHomes[homeDetails[i].AccountID] = &homeDetails[i]
		}
	}

	buckets := make(map[time.Time]*db.HistoryPoint)
	var order []time.Time
	for _, r := range rows {
		hp, ok := buckets[r.Bucket]
		if !ok {
			hp = &db.HistoryPoint{Date: r.Bucket}
			buckets[r.Bucket] = hp
			order = append(order, r.Bucket)
		}

		if hd, tracked := trackedHomes[r.AccountID]; tracked {
			loanBalance := LoanBalanceAsOf(hd, r.Bucket)
			propertyValue := r.Balance + loanBalance
			hp.Assets += propertyValue
			hp.Liabilities -= loanBalance
			hp.NetWorth += r.Balance
			continue
		}

		if r.Balance > 0 {
			hp.Assets += r.Balance
		} else {
			hp.Liabilities += r.Balance
		}
		hp.NetWorth += r.Balance
	}

	points := make([]db.HistoryPoint, 0, len(order))
	for _, bucket := range order {
		points = append(points, *buckets[bucket])
	}
	return points, nil
}

// RefreshHoldingPrices fetches live prices for all of a user's holdings and persists them.
func (s *NetWorthService) RefreshHoldingPrices(ctx context.Context, userID uuid.UUID) (int, error) {
	holdings, err := s.holdings.ListByUser(ctx, userID)
	if err != nil {
		return 0, err
	}

	tickers := make([]string, 0, len(holdings))
	seen := map[string]bool{}
	for _, h := range holdings {
		// Cash holdings store a dollar amount in Shares, not a share count —
		// their ticker (e.g. "CASH") must never be looked up as a security,
		// since that symbol may coincidentally match a real tradeable stock.
		if h.AssetClass == "cash_equivalent" || h.ManualPrice {
			continue
		}
		if t := h.PriceTicker(); !seen[t] {
			tickers = append(tickers, t)
			seen[t] = true
		}
	}

	s.prices.Invalidate(tickers...)
	priceMap, err := s.prices.GetPrices(ctx, tickers)
	if err != nil {
		return 0, err
	}

	count := 0
	now := time.Now()
	for _, h := range holdings {
		if h.AssetClass == "cash_equivalent" || h.ManualPrice {
			continue
		}
		if q, ok := priceMap[h.PriceTicker()]; ok {
			if err := s.holdings.UpdatePrice(ctx, h.ID, q.Price, now); err == nil {
				count++
			}
		}
	}
	return count, nil
}

// HoldingsWithPrices returns all holdings for a user with live prices applied.
func (s *NetWorthService) HoldingsWithPrices(ctx context.Context, userID uuid.UUID) ([]models.Holding, error) {
	holdings, err := s.holdings.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	tickers := make([]string, 0, len(holdings))
	seen := map[string]bool{}
	for _, h := range holdings {
		// Cash and manually-priced holdings don't need a live price fetch.
		if h.AssetClass == "cash_equivalent" || h.ManualPrice {
			continue
		}
		if t := h.PriceTicker(); !seen[t] {
			tickers = append(tickers, t)
			seen[t] = true
		}
	}

	priceMap, _ := s.prices.GetPricesAllowStale(ctx, tickers)
	now := time.Now()
	for i := range holdings {
		if holdings[i].AssetClass == "cash_equivalent" {
			// Shares field stores the dollar amount directly for cash positions.
			holdings[i].TotalValue = holdings[i].Shares
			continue
		}
		if holdings[i].ManualPrice {
			if holdings[i].PricePerShare != nil {
				holdings[i].TotalValue = holdings[i].Shares * *holdings[i].PricePerShare
			}
			continue
		}
		if q, ok := priceMap[holdings[i].PriceTicker()]; ok {
			holdings[i].InstrumentType = q.Type
			holdings[i].QuoteName = q.Name
			p := q.Price
			holdings[i].PricePerShare = &p
			holdings[i].PriceFetchedAt = &now
			holdings[i].TotalValue = holdings[i].Shares * p
			// Persist the live price so it's available as a fallback on future requests
			// when Yahoo Finance is unavailable. Errors are non-fatal.
			_ = s.holdings.UpdatePrice(ctx, holdings[i].ID, p, now)
		} else if holdings[i].PricePerShare != nil {
			holdings[i].TotalValue = holdings[i].Shares * *holdings[i].PricePerShare
		}
	}
	return holdings, nil
}
