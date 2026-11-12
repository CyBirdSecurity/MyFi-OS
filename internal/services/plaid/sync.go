package plaid

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// SyncResult summarises one sync run.
type SyncResult struct {
	AccountsSynced     int      `json:"accountsSynced"`
	HoldingsReplaced   int      `json:"holdingsReplaced"`
	SnapshotsWritten   int      `json:"snapshotsWritten"`
	TransactionsSynced int      `json:"transactionsSynced"`
	Errors             []string `json:"errors"`
}

func (r *SyncResult) merge(other *SyncResult) {
	if other == nil {
		return
	}
	r.AccountsSynced += other.AccountsSynced
	r.HoldingsReplaced += other.HoldingsReplaced
	r.SnapshotsWritten += other.SnapshotsWritten
	r.TransactionsSynced += other.TransactionsSynced
	r.Errors = append(r.Errors, other.Errors...)
}

// SyncItem refreshes holdings and balances for every MyFi account linked to
// the given Plaid Item, writing a source='plaid' snapshot per account dated
// today. Manual snapshots are never overwritten. It fetches real-time
// balances (/accounts/balance/get, billed per call), so it is for
// user-initiated syncs; the daily scheduler uses SyncItemCached.
func (s *Service) SyncItem(ctx context.Context, item models.PlaidItem) (*SyncResult, error) {
	return s.syncItem(ctx, item, true)
}

// SyncItemCached is SyncItem using Plaid's cached balances (/accounts/get,
// not billed per call) instead of a real-time balance fetch. Balances can lag
// the bank until Plaid next refreshes the Item, which is fine for the daily
// background sync.
func (s *Service) SyncItemCached(ctx context.Context, item models.PlaidItem) (*SyncResult, error) {
	return s.syncItem(ctx, item, false)
}

// syncItem runs one Item's sync and logs every failure — both the fatal error
// and the per-account errors collected in result.Errors, which otherwise only
// reach the UI on a manual sync and are lost entirely for the daily one.
func (s *Service) syncItem(ctx context.Context, item models.PlaidItem, realtimeBalances bool) (*SyncResult, error) {
	result, err := s.runItemSync(ctx, item, realtimeBalances)
	if err != nil {
		log.Printf("plaid: item %s (%s) sync failed: %v", item.ItemID, displayName(item), err)
	}
	for _, e := range result.Errors {
		log.Printf("plaid: item %s (%s) sync error: %s", item.ItemID, displayName(item), e)
	}
	return result, err
}

func (s *Service) runItemSync(ctx context.Context, item models.PlaidItem, realtimeBalances bool) (*SyncResult, error) {
	result := &SyncResult{}

	token, err := s.cipher.Decrypt(item.AccessTokenEncrypted)
	if err != nil {
		return result, fmt.Errorf("decrypt access token: %w", err)
	}
	api, err := s.apiForItem(ctx, item)
	if err != nil {
		return result, err
	}

	linked, err := s.accounts.ListByExternalItem(ctx, item.UserID, "plaid", item.ItemID)
	if err != nil {
		return result, err
	}
	if len(linked) == 0 {
		msg := "no linked accounts"
		_ = s.items.RecordSyncResult(ctx, item.ID, &msg)
		result.Errors = append(result.Errors, msg)
		return result, nil
	}
	linkedByPlaidID := make(map[string]models.Account, len(linked))
	for _, acc := range linked {
		if acc.ExternalAccountID != nil {
			linkedByPlaidID[*acc.ExternalAccountID] = acc
		}
	}

	fetchAccounts := api.GetAccounts
	if realtimeBalances {
		fetchAccounts = api.GetBalances
	}
	plaidAccounts, err := fetchAccounts(ctx, token)
	if err != nil {
		s.recordSyncError(ctx, item, err)
		return result, err
	}

	// Holdings sync for investment accounts — one /investments/holdings/get
	// call covers the whole Item.
	needHoldings := false
	for _, pa := range plaidAccounts {
		if _, ok := linkedByPlaidID[pa.AccountID]; ok && IsInvestmentType(pa.Type) {
			needHoldings = true
			break
		}
	}
	if needHoldings {
		holdings, securities, err := api.GetHoldings(ctx, token)
		if err != nil {
			s.recordSyncError(ctx, item, err)
			return result, err
		}
		holdings, securities = s.refreshEmptyInvestments(ctx, api, item, token, plaidAccounts, linkedByPlaidID, holdings, securities, result)
		secByID := make(map[string]Security, len(securities))
		for _, sec := range securities {
			secByID[sec.SecurityID] = sec
		}
		for _, pa := range plaidAccounts {
			acc, ok := linkedByPlaidID[pa.AccountID]
			if !ok || !IsInvestmentType(pa.Type) {
				continue
			}
			n, err := s.syncAccountHoldings(ctx, acc, pa.AccountID, holdings, secByID)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", acc.Name, err))
				continue
			}
			result.HoldingsReplaced += n
		}

		// Recover any gap in weekly history left by the app being stopped —
		// e.g. a self-hosted instance that isn't always running — by
		// reconstructing just the missing weeks from investment transactions.
		s.fillRecentGaps(ctx, api, token, linked, holdings, secByID, result)

		s.syncContributions(ctx, api, item, token, linkedByPlaidID, plaidAccounts, result)
	}

	// Balance snapshots for every linked account, dated today (UTC).
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var snaps []db.UpsertSnapshotParams
	for _, pa := range plaidAccounts {
		acc, ok := linkedByPlaidID[pa.AccountID]
		if !ok || pa.CurrentBalance == nil {
			continue
		}
		if acc.AccountType == "home" {
			// A home account's balance must be equity (zestimate - loan
			// balance), never the raw mortgage liability amount Plaid reports
			// here. syncLiabilities (below) writes the equity snapshot instead.
			continue
		}
		if pa.Currency != "" && pa.Currency != acc.Currency {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: currency mismatch (plaid %s, account %s)", acc.Name, pa.Currency, acc.Currency))
			continue
		}
		balance := *pa.CurrentBalance
		if IsLiability(pa.Type) {
			// Plaid reports amount owed as positive; MyFi stores liabilities negative.
			balance = -balance
		}
		snaps = append(snaps, db.UpsertSnapshotParams{
			AccountID:    acc.ID,
			UserID:       item.UserID,
			SnapshotDate: today,
			Balance:      balance,
			Source:       "plaid",
		})
		if err := s.accounts.TouchSynced(ctx, acc.ID, item.UserID); err != nil {
			log.Printf("plaid: touch synced %s: %v", acc.ID, err)
		}
		result.AccountsSynced++
	}
	if len(snaps) > 0 {
		n, err := s.snapshots.BulkUpsertSkipManual(ctx, snaps)
		if err != nil {
			s.recordSyncError(ctx, item, err)
			return result, err
		}
		result.SnapshotsWritten += n
	}

	// Banking transaction sync for linked depository/credit accounts. Kept
	// non-fatal: a transactions failure (e.g. product not yet consented) must
	// not discard the holdings/balances we just synced above.
	s.syncItemTransactions(ctx, api, item, token, linkedByPlaidID, plaidAccounts, result)

	// Mortgage/credit-card liabilities sync for linked home/credit_card
	// accounts. Also kept non-fatal — an Item linked without liabilities
	// consent must not discard everything synced above.
	s.syncLiabilities(ctx, api, item, token, linkedByPlaidID, plaidAccounts, result)

	_ = s.items.RecordSyncResult(ctx, item.ID, nil)
	return result, nil
}

const (
	// investmentsRefreshWindow is the minimum gap between automatic
	// /investments/refresh calls for one Item (the call is billed).
	investmentsRefreshWindow = 24 * time.Hour
	// investmentsRefreshWait is how long Plaid is given to process a refresh
	// before holdings are fetched again. If the positions aren't there yet,
	// the next sync (which fetches holdings but doesn't refresh again) gets them.
	investmentsRefreshWait = 5 * time.Second
)

// refreshEmptyInvestments handles institutions (Fidelity) that return
// balances at link time but no positions until /investments/refresh is
// called. When a linked investment account has a balance but Plaid returned no
// holdings for it, it asks Plaid to refresh the Item — at most once per
// investmentsRefreshWindow, since the call is billed — and fetches holdings
// again. Failures are non-fatal: the original (empty) holdings are returned
// and the error is added to result.
func (s *Service) refreshEmptyInvestments(
	ctx context.Context,
	api API,
	item models.PlaidItem,
	token string,
	plaidAccounts []Account,
	linkedByPlaidID map[string]models.Account,
	holdings []Holding,
	securities []Security,
	result *SyncResult,
) ([]Holding, []Security) {
	empty := 0
	for _, pa := range plaidAccounts {
		if _, ok := linkedByPlaidID[pa.AccountID]; !ok || !IsInvestmentType(pa.Type) {
			continue
		}
		if pa.CurrentBalance != nil && *pa.CurrentBalance > 0 && !accountHasHoldings(pa.AccountID, holdings) {
			empty++
		}
	}
	if empty == 0 {
		return holdings, securities
	}

	ok, err := s.items.ClaimInvestmentsRefresh(ctx, item.ID, investmentsRefreshWindow)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("%s: investments refresh: %v", displayName(item), err))
		return holdings, securities
	}
	if !ok {
		return holdings, securities
	}

	log.Printf("plaid: item %s (%s): %d investment account(s) have a balance but no holdings; requesting an investments refresh",
		item.ItemID, displayName(item), empty)
	if err := api.RefreshInvestments(ctx, token); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("%s: investments refresh: %v", displayName(item), err))
		return holdings, securities
	}
	if s.refreshWait > 0 {
		select {
		case <-time.After(s.refreshWait):
		case <-ctx.Done():
			return holdings, securities
		}
	}
	h, sec, err := api.GetHoldings(ctx, token)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("%s: holdings after refresh: %v", displayName(item), err))
		return holdings, securities
	}
	return h, sec
}

// syncAccountHoldings replaces one MyFi account's holdings with the Plaid
// positions for the corresponding Plaid account. Shares and cost basis always
// come from Plaid, but a holding the user has manually priced (manual_price
// = true, whether that started as our own CUSIP/no-ticker fallback or the
// user explicitly overrode a resolvable ticker) keeps its existing price
// across syncs instead of being reset to Plaid's institution_price.
func (s *Service) syncAccountHoldings(
	ctx context.Context,
	acc models.Account,
	plaidAccountID string,
	allHoldings []Holding,
	secByID map[string]Security,
) (int, error) {
	current, err := s.holdings.ListByAccount(ctx, acc.ID, acc.UserID)
	if err != nil {
		return 0, err
	}
	currentByTicker := make(map[string]models.Holding, len(current))
	for _, h := range current {
		currentByTicker[h.Ticker] = h
	}

	// Aggregate Plaid positions by resolved ticker (different security IDs can
	// map to the same symbol).
	type position struct {
		params db.CreateHoldingParams
		hasCB  bool
	}
	byTicker := map[string]*position{}
	order := []string{}

	for _, h := range allHoldings {
		if h.AccountID != plaidAccountID {
			continue
		}
		sec := secByID[h.SecurityID]
		ticker, manualPrice := ResolveTicker(sec)
		assetClass := SecurityTypeToAssetClass(sec.Type)

		shares := h.Quantity
		if assetClass == "cash_equivalent" {
			// MyFi convention: cash holdings store a dollar amount in Shares.
			shares = h.InstitutionValue
		}

		pos, ok := byTicker[ticker]
		if !ok {
			var name *string
			if sec.Name != "" {
				n := sec.Name
				name = &n
			}
			price := h.InstitutionPrice
			if existing, ok := currentByTicker[ticker]; ok && existing.ManualPrice {
				manualPrice = true
				if existing.PricePerShare != nil {
					price = *existing.PricePerShare
				}
			}
			pos = &position{params: db.CreateHoldingParams{
				AccountID:     acc.ID,
				UserID:        acc.UserID,
				Ticker:        ticker,
				Name:          name,
				AssetClass:    assetClass,
				ManualPrice:   manualPrice,
				PricePerShare: &price,
			}}
			byTicker[ticker] = pos
			order = append(order, ticker)
		}
		pos.params.Shares += shares
		if h.CostBasis != nil {
			if pos.params.CostBasis == nil {
				var zero float64
				pos.params.CostBasis = &zero
			}
			*pos.params.CostBasis += *h.CostBasis
			pos.hasCB = true
		}
	}

	if len(order) == 0 {
		// Not an error (an empty account is valid), but a balance with no
		// positions usually means the institution hasn't shared holdings yet.
		log.Printf("plaid: no holdings returned for investment account %s (%s, plaid account %s)", acc.Name, acc.ID, ShortID(plaidAccountID))
	}

	params := make([]db.CreateHoldingParams, 0, len(order))
	seen := make(map[string]bool, len(order))
	for _, t := range order {
		pos := byTicker[t]
		// Plaid reports cost basis per lot as a total dollar amount, and the
		// loop above sums those totals across lots — but Holding.CostBasis is
		// a per-share figure everywhere else in the app (manual entry, CSV
		// import, the weighted-average math on the Holdings page). Convert
		// before storing so a Plaid-synced holding's avg cost isn't off by a
		// factor of its share count.
		if pos.hasCB && pos.params.CostBasis != nil && pos.params.Shares > 0 {
			perShare := *pos.params.CostBasis / pos.params.Shares
			pos.params.CostBasis = &perShare
		}
		params = append(params, pos.params)
		seen[t] = true
	}

	// Tickers in the DB but gone from Plaid: zero-share sentinel deletes them.
	for _, h := range current {
		if !seen[h.Ticker] {
			params = append(params, db.CreateHoldingParams{
				AccountID:  acc.ID,
				UserID:     acc.UserID,
				Ticker:     h.Ticker,
				AssetClass: h.AssetClass,
				Shares:     0, // delete sentinel
			})
		}
	}

	if len(params) == 0 {
		return 0, nil
	}
	return s.holdings.ReplaceForTickers(ctx, acc.ID, acc.UserID, params)
}

// SyncUserItems syncs all of one user's items ("Sync all" button).
func (s *Service) SyncUserItems(ctx context.Context, userID uuid.UUID) (*SyncResult, error) {
	items, err := s.items.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := &SyncResult{}
	for _, item := range items {
		if item.Status == "login_required" {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: reconnection required", displayName(item)))
			continue
		}
		res, err := s.SyncItem(ctx, item)
		result.merge(res)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", displayName(item), err))
		}
	}
	return result, nil
}

// recordSyncError stores the failure on the item, marking login_required for
// re-auth errors so the scheduler skips the item until the user reconnects.
func (s *Service) recordSyncError(ctx context.Context, item models.PlaidItem, err error) {
	if code := ErrorCode(err); code == "ITEM_LOGIN_REQUIRED" {
		_ = s.items.SetStatus(ctx, item.ID, "login_required", code)
		return
	}
	msg := err.Error()
	_ = s.items.RecordSyncResult(ctx, item.ID, &msg)
}

func displayName(item models.PlaidItem) string {
	if item.InstitutionName != nil && *item.InstitutionName != "" {
		return *item.InstitutionName
	}
	return item.ItemID
}
