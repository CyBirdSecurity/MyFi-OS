package services

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// closedAccountsSeriesID collects the history of accounts that have since been
// deleted. Their snapshots are kept and still count toward net worth, so the
// breakdown must include them for its total to match the net worth line.
const closedAccountsSeriesID = "closed"

// mortgageSeriesID is the synthetic ID of the mortgage that goes with a home
// account tracked as a liability. Net worth uses the same ID for it, so the
// frontend's balance maps (keyed by account ID) don't clash with the home's.
func mortgageSeriesID(homeID uuid.UUID) uuid.UUID {
	id := homeID
	id[15] ^= 0x01
	return id
}

// AccountSeries names one stacked series of the per-account history.
type AccountSeries struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AccountType string `json:"accountType"`
}

// AccountHistoryPoint is every series' balance at one date bucket, keyed by series ID.
type AccountHistoryPoint struct {
	Date     time.Time          `json:"date"`
	Balances map[string]float64 `json:"balances"`
}

// AccountHistory is net worth over time broken down by account. At every date
// the balances sum to that date's net worth.
type AccountHistory struct {
	Series []AccountSeries       `json:"series"`
	Points []AccountHistoryPoint `json:"points"`
}

// buildAccountHistory turns per-account, per-bucket balances into stacked series.
//
// A home account tracked as a liability stores equity, so — as in the net worth
// total — it is split back into the property (an asset series under the home's
// name) and its mortgage (a liability series, "<home> (Mortgage)"), with the loan
// balance reconstructed for each bucket by loanBalance. Accounts that no longer
// exist are folded into one "Closed accounts" series. Series are sorted by name
// so the response is stable between requests.
func buildAccountHistory(
	rows []db.AccountBucketBalance,
	accounts []models.Account,
	trackedHomes map[uuid.UUID]*models.HomeDetails,
	loanBalance func(*models.HomeDetails, time.Time) float64,
) AccountHistory {
	byID := make(map[uuid.UUID]models.Account, len(accounts))
	for _, a := range accounts {
		byID[a.ID] = a
	}

	series := map[string]AccountSeries{}
	var order []time.Time
	points := map[time.Time]*AccountHistoryPoint{}

	add := func(bucket time.Time, s AccountSeries, value float64) {
		series[s.ID] = s
		p, ok := points[bucket]
		if !ok {
			p = &AccountHistoryPoint{Date: bucket, Balances: map[string]float64{}}
			points[bucket] = p
			order = append(order, bucket)
		}
		p.Balances[s.ID] += value
	}

	for _, r := range rows {
		acc, known := byID[r.AccountID]
		if !known {
			add(r.Bucket, AccountSeries{ID: closedAccountsSeriesID, Name: "Closed accounts", AccountType: "closed"}, r.Balance)
			continue
		}
		self := AccountSeries{ID: acc.ID.String(), Name: acc.Name, AccountType: acc.AccountType}
		if hd, tracked := trackedHomes[acc.ID]; tracked {
			loan := loanBalance(hd, r.Bucket)
			add(r.Bucket, self, r.Balance+loan)
			add(r.Bucket, AccountSeries{
				ID:          mortgageSeriesID(acc.ID).String(),
				Name:        acc.Name + " (Mortgage)",
				AccountType: "mortgage",
			}, -loan)
			continue
		}
		add(r.Bucket, self, r.Balance)
	}

	out := AccountHistory{Series: make([]AccountSeries, 0, len(series)), Points: make([]AccountHistoryPoint, 0, len(order))}
	for _, s := range series {
		out.Series = append(out.Series, s)
	}
	sort.Slice(out.Series, func(i, j int) bool {
		if out.Series[i].Name != out.Series[j].Name {
			return out.Series[i].Name < out.Series[j].Name
		}
		return out.Series[i].ID < out.Series[j].ID
	})
	for _, bucket := range order {
		out.Points = append(out.Points, *points[bucket])
	}
	return out
}

// AccountHistory returns net worth over time broken down by account.
func (s *NetWorthService) AccountHistory(ctx context.Context, userID uuid.UUID, from, to time.Time, interval string) (*AccountHistory, error) {
	rows, err := s.snapshots.HistoryByAccount(ctx, userID, from, to, interval)
	if err != nil {
		return nil, err
	}
	accounts, err := s.accounts.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	homeDetails, err := s.home.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	tracked := make(map[uuid.UUID]*models.HomeDetails, len(homeDetails))
	for i := range homeDetails {
		if homeDetails[i].TrackMortgageAsLiability {
			tracked[homeDetails[i].AccountID] = &homeDetails[i]
		}
	}
	h := buildAccountHistory(rows, accounts, tracked, LoanBalanceAsOf)
	return &h, nil
}
