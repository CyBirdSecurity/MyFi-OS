package plaid

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// backfillMonths is how far back the historical reconstruction reaches —
// Plaid returns at most 24 months of investment transactions.
const backfillMonths = 24

// snapshotChunkSize bounds one bulk insert statement.
const snapshotChunkSize = 500

// BackfillItem reconstructs weekly historical values for every investment
// account linked to the item and writes them as source='plaid' snapshots
// dated to Mondays. Cash/credit accounts are skipped — their history can't be
// reconstructed from the investments product.
//
// Method: start from current holdings, then walk backward one week at a time,
// undoing investment transactions to recover the holdings (and a cash delta)
// as of each Monday, valuing positions with historical weekly closes.
// Snapshots the user entered manually are never overwritten.
func (s *Service) BackfillItem(ctx context.Context, item models.PlaidItem) error {
	token, err := s.cipher.Decrypt(item.AccessTokenEncrypted)
	if err != nil {
		return fmt.Errorf("decrypt access token: %w", err)
	}
	api, err := s.apiForItem(ctx, item)
	if err != nil {
		return err
	}

	linked, err := s.accounts.ListByExternalItem(ctx, item.UserID, "plaid", item.ItemID)
	if err != nil {
		return err
	}

	holdings, securities, err := api.GetHoldings(ctx, token)
	if err != nil {
		return err
	}
	secByID := make(map[string]Security, len(securities))
	for _, sec := range securities {
		secByID[sec.SecurityID] = sec
	}

	now := time.Now().UTC()
	start := now.AddDate(0, -backfillMonths, 0)

	txns, txnSecurities, err := api.GetInvestmentTransactions(ctx, token, start, now, nil)
	if err != nil {
		return err
	}
	for _, sec := range txnSecurities {
		if _, ok := secByID[sec.SecurityID]; !ok {
			secByID[sec.SecurityID] = sec
		}
	}

	// Fetch weekly close history once per distinct live-priced ticker.
	closes := s.fetchWeeklyCloses(ctx, holdings, secByID, start, now)

	for _, acc := range linked {
		if acc.ExternalAccountID == nil {
			continue
		}
		plaidAccountID := *acc.ExternalAccountID
		if !accountHasHoldings(plaidAccountID, holdings) {
			continue // balance-only account: nothing to reconstruct
		}
		snaps := reconstructWeekly(acc, plaidAccountID, holdings, txns, secByID, closes, start, now, true)
		for i := 0; i < len(snaps); i += snapshotChunkSize {
			end := min(i+snapshotChunkSize, len(snaps))
			if _, err := s.snapshots.BulkUpsertSkipManual(ctx, snaps[i:end]); err != nil {
				return fmt.Errorf("backfill snapshots for %s: %w", acc.Name, err)
			}
		}
		log.Printf("plaid: backfilled %d weekly snapshot(s) for account %s", len(snaps), acc.Name)
	}

	return s.items.SetBackfilledAt(ctx, item.ID)
}

// fetchWeeklyCloses loads historical candles for each distinct resolvable
// ticker held in the item and indexes the most recent close on or before each
// Monday in the range. Manual-priced securities (no real symbol) are skipped —
// they are valued at the current institution price.
func (s *Service) fetchWeeklyCloses(
	ctx context.Context,
	holdings []Holding,
	secByID map[string]Security,
	from, to time.Time,
) map[string]map[time.Time]float64 {
	closes := map[string]map[time.Time]float64{}
	seen := map[string]bool{}
	for _, h := range holdings {
		sec := secByID[h.SecurityID]
		ticker, manualPrice := ResolveTicker(sec)
		if manualPrice || SecurityTypeToAssetClass(sec.Type) == "cash_equivalent" || seen[ticker] {
			continue
		}
		seen[ticker] = true

		// Plaid's type is authoritative here: a coin's history is listed
		// against a currency ("BTC-USD"); a bare "BTC" is an unrelated ETF.
		symbol := ticker
		if SecurityTypeToAssetClass(sec.Type) == "crypto" && !strings.Contains(ticker, "-") {
			symbol = ticker + "-USD"
		}
		candles, err := s.candles.GetCandles(ctx, symbol, from.AddDate(0, 0, -14), to)
		if err != nil || len(candles) == 0 {
			log.Printf("plaid: no candle history for %s (falling back to current price): %v", ticker, err)
			continue
		}
		sort.Slice(candles, func(i, j int) bool { return candles[i].Time.Before(candles[j].Time) })

		weekly := map[time.Time]float64{}
		ci := 0
		lastClose := 0.0
		for monday := thisWeekMonday(from); !monday.After(to); monday = monday.AddDate(0, 0, 7) {
			for ci < len(candles) && !candles[ci].Time.After(monday) {
				lastClose = candles[ci].Close
				ci++
			}
			if lastClose > 0 {
				weekly[monday] = lastClose
			}
		}
		closes[ticker] = weekly
	}
	return closes
}

// reconstructWeekly walks holdings backward from today, undoing transactions
// week by week, and values the account at each Monday.
//
// boundToKnownTxns controls how far back the reconstruction is willing to go
// when there are no more transactions to undo:
//   - true (the multi-year initial backfill): stop at the oldest transaction
//     Plaid actually returned. An empty feed there is ambiguous — it may just
//     mean the institution's retention doesn't reach that far back — so we
//     refuse to guess further and risk fabricating history.
//   - false (a short, recent gap-fill): reconstruct every week back to start
//     regardless. Here an empty/short transaction list for that narrow, recent
//     window is authoritative — Plaid's retention easily covers it — so no
//     transactions genuinely means nothing changed, not "we have no data".
func reconstructWeekly(
	acc models.Account,
	plaidAccountID string,
	allHoldings []Holding,
	allTxns []InvestmentTransaction,
	secByID map[string]Security,
	closes map[string]map[time.Time]float64,
	start, now time.Time,
	boundToKnownTxns bool,
) []db.UpsertSnapshotParams {
	// Current state: share quantities per security + cash value.
	qty := map[string]float64{}
	cash := 0.0
	for _, h := range allHoldings {
		if h.AccountID != plaidAccountID {
			continue
		}
		sec := secByID[h.SecurityID]
		if SecurityTypeToAssetClass(sec.Type) == "cash_equivalent" {
			cash += h.InstitutionValue
		} else {
			qty[h.SecurityID] += h.Quantity
		}
	}

	// This account's transactions, newest first.
	var txns []InvestmentTransaction
	for _, t := range allTxns {
		if t.AccountID == plaidAccountID {
			txns = append(txns, t)
		}
	}
	sort.Slice(txns, func(i, j int) bool { return txns[i].Date.After(txns[j].Date) })

	earliest := thisWeekMonday(start)
	if boundToKnownTxns {
		// Don't reconstruct weeks before the account's known history begins.
		earliest = thisWeekMonday(now)
		if len(txns) > 0 {
			earliest = thisWeekMonday(txns[len(txns)-1].Date)
		}
		if s := thisWeekMonday(start); s.After(earliest) {
			earliest = s
		}
	}

	var snaps []db.UpsertSnapshotParams
	ti := 0
	notes := "backfilled from Plaid"
	for monday := thisWeekMonday(now); !monday.Before(earliest); monday = monday.AddDate(0, 0, -7) {
		// Undo every transaction that happened after this Monday.
		for ti < len(txns) && txns[ti].Date.After(monday) {
			applyReverse(txns[ti], qty, &cash)
			ti++
		}

		value := cash
		for secID, q := range qty {
			if q < 1e-9 {
				continue
			}
			sec := secByID[secID]
			ticker, manualPrice := ResolveTicker(sec)
			price := 0.0
			if !manualPrice {
				price = closes[ticker][monday]
			}
			if price == 0 {
				// Approximation: fall back to the current institution price.
				price = currentInstitutionPrice(secID, plaidAccountID, allHoldings)
			}
			value += q * price
		}

		if value > 0 {
			n := notes
			snaps = append(snaps, db.UpsertSnapshotParams{
				AccountID:    acc.ID,
				UserID:       acc.UserID,
				SnapshotDate: monday,
				Balance:      value,
				Notes:        &n,
				Source:       "plaid",
			})
		}
	}
	return snaps
}

// applyReverse undoes one investment transaction against the running holdings
// state. Plaid conventions: Quantity is positive for buys and negative for
// sells; Amount is positive when cash leaves the account (purchases, fees,
// withdrawals) and negative when cash arrives (sales, dividends, deposits).
// Reversing therefore subtracts Quantity from the position and adds Amount
// back to cash.
func applyReverse(t InvestmentTransaction, qty map[string]float64, cash *float64) {
	if t.SecurityID != "" {
		qty[t.SecurityID] -= t.Quantity
	}
	*cash += t.Amount
}

// gapFillMaxWeeks bounds how far back a sync-time gap-fill will look. This is
// for recovering snapshots missed while the app was stopped (self-hosted
// deployments aren't always running 24/7), not a substitute for the one-time
// historical backfill run at link time.
const gapFillMaxWeeks = 26

// fillRecentGaps checks each investment account's most recent snapshot and,
// if there's a gap since then, reconstructs just the missing weeks from
// investment transactions — the same method as the initial backfill, bounded
// to a short recent window. Called at the end of every SyncItem so downtime
// self-heals on the next sync. Weeks that already have a snapshot (any
// source) are untouched; manual snapshots are never overwritten
// (BulkUpsertSkipManual). Failures are appended to result rather than
// aborting — the holdings/balance sync this runs after already succeeded.
func (s *Service) fillRecentGaps(
	ctx context.Context,
	api API,
	token string,
	linked []models.Account,
	holdings []Holding,
	secByID map[string]Security,
	result *SyncResult,
) {
	now := time.Now().UTC()
	currentWeek := thisWeekMonday(now)
	maxStart := now.AddDate(0, 0, -7*gapFillMaxWeeks)

	for _, acc := range linked {
		if acc.ExternalAccountID == nil {
			continue
		}
		plaidAccountID := *acc.ExternalAccountID
		if !accountHasHoldings(plaidAccountID, holdings) {
			continue // balance-only account: no transaction feed to reconstruct from
		}

		latest, err := s.snapshots.LatestForAccount(ctx, acc.ID, acc.UserID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: gap-fill lookup: %v", acc.Name, err))
			continue
		}
		gapStart := maxStart
		if latest != nil {
			gapStart = thisWeekMonday(latest.SnapshotDate).AddDate(0, 0, 7)
			if gapStart.Before(maxStart) {
				gapStart = maxStart
			}
		}
		if !gapStart.Before(currentWeek) {
			continue // no gap
		}

		txns, txnSecurities, err := api.GetInvestmentTransactions(ctx, token, gapStart, now, []string{plaidAccountID})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: gap-fill transactions: %v", acc.Name, err))
			continue
		}
		for _, sec := range txnSecurities {
			if _, ok := secByID[sec.SecurityID]; !ok {
				secByID[sec.SecurityID] = sec
			}
		}
		closes := s.fetchWeeklyCloses(ctx, holdings, secByID, gapStart, now)

		snaps := reconstructWeekly(acc, plaidAccountID, holdings, txns, secByID, closes, gapStart, now, false)
		var toWrite []db.UpsertSnapshotParams
		for _, snap := range snaps {
			// The current week is already handled by SyncItem's own
			// today-dated snapshot; don't double-write it here.
			if snap.SnapshotDate.Before(currentWeek) {
				toWrite = append(toWrite, snap)
			}
		}
		if len(toWrite) == 0 {
			continue
		}
		n, err := s.snapshots.BulkUpsertSkipManual(ctx, toWrite)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: gap-fill write: %v", acc.Name, err))
			continue
		}
		log.Printf("plaid: gap-filled %d weekly snapshot(s) for account %s", n, acc.Name)
		result.SnapshotsWritten += n
	}
}

func accountHasHoldings(plaidAccountID string, holdings []Holding) bool {
	for _, h := range holdings {
		if h.AccountID == plaidAccountID {
			return true
		}
	}
	return false
}

func currentInstitutionPrice(securityID, plaidAccountID string, holdings []Holding) float64 {
	for _, h := range holdings {
		if h.SecurityID == securityID && h.AccountID == plaidAccountID {
			return h.InstitutionPrice
		}
	}
	return 0
}

// thisWeekMonday returns midnight UTC of the Monday that starts the ISO week
// containing t (same helper as autosnap).
func thisWeekMonday(t time.Time) time.Time {
	weekday := t.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	daysBack := int(weekday - time.Monday)
	monday := t.AddDate(0, 0, -daysBack)
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
}
