package plaid

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/crypto"
	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
	"github.com/cybirdsecurity/myfi-os/internal/prices"
)

const testHexKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// fixedAPI resolves every user to the same client.
func fixedAPI(api API) func(context.Context, uuid.UUID) (API, error) {
	return func(context.Context, uuid.UUID) (API, error) { return api, nil }
}

// fakeOf returns the *fakeAPI a test Service resolves for every user.
func fakeOf(svc *Service) *fakeAPI {
	api, _ := svc.clientFor(context.Background(), uuid.Nil)
	return api.(*fakeAPI)
}

// fakeAPI implements API with canned data.
type fakeAPI struct {
	accounts   []Account
	holdings   []Holding
	securities []Security
	txns       []InvestmentTransaction
	// txnPages are returned in order by successive SyncTransactions calls; the
	// last page should have HasMore=false. syncCalls records how many times
	// SyncTransactions was invoked and with what cursor.
	txnPages  []TransactionSyncPage
	syncCalls []string
	// refreshCalls counts RefreshInvestments calls. Once refreshed, GetHoldings
	// returns afterRefresh* instead (when set), like Plaid filling in positions.
	refreshCalls           int
	refreshErr             error
	holdingsAfterRefresh   []Holding
	securitiesAfterRefresh []Security
}

func (f *fakeAPI) CreateLinkToken(context.Context, string, []string, []string, []string, []string, string) (string, error) {
	return "link-token", nil
}
func (f *fakeAPI) ExchangePublicToken(context.Context, string) (string, string, error) {
	return "access-token", "item-1", nil
}
func (f *fakeAPI) GetAccounts(context.Context, string) ([]Account, error) { return f.accounts, nil }
func (f *fakeAPI) GetBalances(context.Context, string) ([]Account, error) { return f.accounts, nil }
func (f *fakeAPI) GetHoldings(context.Context, string) ([]Holding, []Security, error) {
	if f.refreshCalls > 0 && f.holdingsAfterRefresh != nil {
		return f.holdingsAfterRefresh, f.securitiesAfterRefresh, nil
	}
	return f.holdings, f.securities, nil
}
func (f *fakeAPI) RefreshInvestments(context.Context, string) error {
	f.refreshCalls++
	return f.refreshErr
}
func (f *fakeAPI) GetInvestmentTransactions(context.Context, string, time.Time, time.Time, []string) ([]InvestmentTransaction, []Security, error) {
	return f.txns, f.securities, nil
}
func (f *fakeAPI) SyncTransactions(_ context.Context, _, cursor string) (*TransactionSyncPage, error) {
	f.syncCalls = append(f.syncCalls, cursor)
	idx := len(f.syncCalls) - 1
	if idx < len(f.txnPages) {
		p := f.txnPages[idx]
		return &p, nil
	}
	return &TransactionSyncPage{}, nil
}
func (f *fakeAPI) RemoveItem(context.Context, string) error { return nil }
func (f *fakeAPI) GetLiabilities(context.Context, string) ([]MortgageLiability, []CreditCardLiability, error) {
	return nil, nil, nil
}

// fakeAccountStore returns fixed linked accounts and records sync touches.
type fakeAccountStore struct {
	linked []models.Account
}

func (f *fakeAccountStore) Get(_ context.Context, id, _ uuid.UUID) (*models.Account, error) {
	for _, a := range f.linked {
		if a.ID == id {
			return &a, nil
		}
	}
	return nil, nil
}
func (f *fakeAccountStore) Create(context.Context, db.CreateAccountParams) (*models.Account, error) {
	return nil, nil
}
func (f *fakeAccountStore) LinkPlaid(context.Context, uuid.UUID, uuid.UUID, string, string) error {
	return nil
}
func (f *fakeAccountStore) UnlinkExternal(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeAccountStore) ListByExternalItem(context.Context, uuid.UUID, string, string) ([]models.Account, error) {
	return f.linked, nil
}
func (f *fakeAccountStore) FindByExternalID(context.Context, uuid.UUID, string, string) (*models.Account, error) {
	return nil, nil
}
func (f *fakeAccountStore) TouchSynced(context.Context, uuid.UUID, uuid.UUID) error { return nil }

// fakeHoldingStore captures ReplaceForTickers calls.
type fakeHoldingStore struct {
	existing []models.Holding
	replaced map[uuid.UUID][]db.CreateHoldingParams
}

func (f *fakeHoldingStore) ListByAccount(_ context.Context, accountID, _ uuid.UUID) ([]models.Holding, error) {
	var out []models.Holding
	for _, h := range f.existing {
		if h.AccountID == accountID {
			out = append(out, h)
		}
	}
	return out, nil
}
func (f *fakeHoldingStore) ReplaceForTickers(_ context.Context, accountID, _ uuid.UUID, params []db.CreateHoldingParams) (int, error) {
	if f.replaced == nil {
		f.replaced = map[uuid.UUID][]db.CreateHoldingParams{}
	}
	f.replaced[accountID] = params
	n := 0
	for _, p := range params {
		if p.Shares > 0 {
			n++
		}
	}
	return n, nil
}

// fakeSnapshotStore captures BulkUpsertSkipManual calls.
type fakeSnapshotStore struct {
	written []db.UpsertSnapshotParams
	latest  map[uuid.UUID]*models.BalanceSnapshot
}

func (f *fakeSnapshotStore) BulkUpsertSkipManual(_ context.Context, snaps []db.UpsertSnapshotParams) (int, error) {
	f.written = append(f.written, snaps...)
	return len(snaps), nil
}

func (f *fakeSnapshotStore) Upsert(_ context.Context, p db.UpsertSnapshotParams) (*models.BalanceSnapshot, error) {
	f.written = append(f.written, p)
	return &models.BalanceSnapshot{AccountID: p.AccountID, UserID: p.UserID, SnapshotDate: p.SnapshotDate, Balance: p.Balance, Source: p.Source}, nil
}

// LatestForAccount reports no prior snapshot by default, so gap-fill sees a
// full gap — tests that care about a specific "already synced" state set
// latest explicitly.
func (f *fakeSnapshotStore) LatestForAccount(_ context.Context, accountID, _ uuid.UUID) (*models.BalanceSnapshot, error) {
	if f.latest == nil {
		return nil, nil
	}
	return f.latest[accountID], nil
}

// fakeCandles implements candleProvider with no historical data — exercises
// the "fall back to current institution price" path in fetchWeeklyCloses.
type fakeCandles struct{}

func (fakeCandles) GetCandles(context.Context, string, time.Time, time.Time) ([]prices.HistoricalCandle, error) {
	return nil, nil
}

// fakeItemStore records status transitions.
type fakeItemStore struct {
	items       []models.PlaidItem
	lastStatus  string
	syncErr     *string
	syncOK      bool
	savedCursor string
	cursorSet   bool
	created     []db.CreatePlaidItemParams
	unlinked    []models.PlaidItem
	deleted     []uuid.UUID
	// refreshClaims counts ClaimInvestmentsRefresh calls; refreshDenied makes
	// them report that a refresh was already claimed within the window.
	refreshClaims int
	refreshDenied bool
}

func (f *fakeItemStore) ListUnlinked(context.Context, time.Duration) ([]models.PlaidItem, error) {
	return f.unlinked, nil
}

func (f *fakeItemStore) Create(_ context.Context, p db.CreatePlaidItemParams) (*models.PlaidItem, error) {
	f.created = append(f.created, p)
	return &models.PlaidItem{}, nil
}
func (f *fakeItemStore) GetByItemID(context.Context, uuid.UUID, string) (*models.PlaidItem, error) {
	if len(f.items) > 0 {
		return &f.items[0], nil
	}
	return &models.PlaidItem{}, nil
}
func (f *fakeItemStore) ListByUser(context.Context, uuid.UUID) ([]models.PlaidItem, error) {
	return f.items, nil
}
func (f *fakeItemStore) ListAllActive(context.Context) ([]models.PlaidItem, error) {
	return f.items, nil
}
func (f *fakeItemStore) RecordSyncResult(_ context.Context, _ uuid.UUID, syncErr *string) error {
	f.syncErr = syncErr
	f.syncOK = syncErr == nil
	return nil
}
func (f *fakeItemStore) SetStatus(_ context.Context, _ uuid.UUID, status, _ string) error {
	f.lastStatus = status
	return nil
}
func (f *fakeItemStore) SetBackfillRequested(context.Context, uuid.UUID) error { return nil }
func (f *fakeItemStore) SetBackfilledAt(context.Context, uuid.UUID) error      { return nil }
func (f *fakeItemStore) SetTransactionsCursor(_ context.Context, _ uuid.UUID, cursor string) error {
	f.savedCursor = cursor
	f.cursorSet = true
	return nil
}
func (f *fakeItemStore) ClaimInvestmentsRefresh(context.Context, uuid.UUID, time.Duration) (bool, error) {
	f.refreshClaims++
	return !f.refreshDenied, nil
}
func (f *fakeItemStore) Delete(_ context.Context, id, _ uuid.UUID) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func encryptedToken(t *testing.T, c *crypto.Cipher) string {
	t.Helper()
	enc, err := c.Encrypt("access-token")
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func TestSyncItem(t *testing.T) {
	cipher, _ := crypto.NewCipher(testHexKey)
	userID := uuid.New()
	brokerageID, creditID := uuid.New(), uuid.New()

	brokerageExt, creditExt := "p-brokerage", "p-credit"
	linked := []models.Account{
		{ID: brokerageID, UserID: userID, Name: "Brokerage", Currency: "USD", ExternalAccountID: &brokerageExt},
		{ID: creditID, UserID: userID, Name: "Card", Currency: "USD", ExternalAccountID: &creditExt},
	}

	bal1, bal2 := 2500.0, 430.25
	cb1, cb2 := 900.0, 300.0
	api := &fakeAPI{
		accounts: []Account{
			{AccountID: "p-brokerage", Name: "Brokerage", Type: "investment", Subtype: "brokerage", CurrentBalance: &bal1, Currency: "USD"},
			{AccountID: "p-credit", Name: "Card", Type: "credit", Subtype: "credit card", CurrentBalance: &bal2, Currency: "USD"},
		},
		securities: []Security{
			{SecurityID: "s-vti", Ticker: "VTI", Name: "Vanguard Total Market", Type: "etf"},
			{SecurityID: "s-vti2", Ticker: "VTI", Name: "Vanguard Total Market", Type: "etf"}, // duplicate ticker
			{SecurityID: "s-cash", Ticker: "CUR:USD", Name: "US Dollar", Type: "cash"},
		},
		holdings: []Holding{
			{AccountID: "p-brokerage", SecurityID: "s-vti", Quantity: 5, InstitutionPrice: 200, InstitutionValue: 1000, CostBasis: &cb1},
			{AccountID: "p-brokerage", SecurityID: "s-vti2", Quantity: 2, InstitutionPrice: 200, InstitutionValue: 400, CostBasis: &cb2},
			{AccountID: "p-brokerage", SecurityID: "s-cash", Quantity: 1100, InstitutionPrice: 1, InstitutionValue: 1100},
		},
	}

	accounts := &fakeAccountStore{linked: linked}
	holdingsStore := &fakeHoldingStore{
		existing: []models.Holding{
			{AccountID: brokerageID, Ticker: "OLDX", AssetClass: "equity", Shares: 3}, // gone from Plaid → sentinel delete
		},
	}
	// Already synced this week — fillRecentGaps should see no gap and no-op,
	// keeping this test scoped to the holdings/balance sync it's actually for.
	snapshots := &fakeSnapshotStore{
		latest: map[uuid.UUID]*models.BalanceSnapshot{
			brokerageID: {SnapshotDate: time.Now().UTC(), Source: "plaid"},
		},
	}
	items := &fakeItemStore{}

	svc := &Service{clientFor: fixedAPI(api), cipher: cipher, items: items, accounts: accounts, holdings: holdingsStore, snapshots: snapshots, candles: fakeCandles{}}

	item := models.PlaidItem{ID: uuid.New(), UserID: userID, ItemID: "item-1", AccessTokenEncrypted: encryptedToken(t, cipher)}
	result, err := svc.SyncItem(context.Background(), item)
	if err != nil {
		t.Fatalf("SyncItem: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	// Holdings: VTI merged across duplicate security IDs, cash as dollar
	// amount in Shares, OLDX sentinel-deleted.
	params := holdingsStore.replaced[brokerageID]
	byTicker := map[string]db.CreateHoldingParams{}
	for _, p := range params {
		byTicker[p.Ticker] = p
	}
	vti := byTicker["VTI"]
	if vti.Shares != 7 {
		t.Errorf("VTI shares = %v, want 7 (merged)", vti.Shares)
	}
	// cb1=900 total for 5 shares + cb2=300 total for 2 shares = $1200 total
	// for 7 shares — stored per-share (Holding.CostBasis's convention
	// everywhere else in the app), so 1200/7 ≈ 171.43, not the raw $1200 sum.
	wantCB := 1200.0 / 7.0
	if vti.CostBasis == nil || math.Abs(*vti.CostBasis-wantCB) > 0.01 {
		t.Errorf("VTI cost basis = %v, want %.4f (per-share average)", vti.CostBasis, wantCB)
	}
	if vti.AssetClass != "equity" || vti.ManualPrice {
		t.Errorf("VTI asset class/manual price wrong: %+v", vti)
	}
	cash := byTicker["CUR:USD"]
	if cash.AssetClass != "cash_equivalent" || cash.Shares != 1100 {
		t.Errorf("cash holding = %+v, want cash_equivalent with 1100 dollars in Shares", cash)
	}
	oldx, ok := byTicker["OLDX"]
	if !ok || oldx.Shares != 0 {
		t.Errorf("OLDX = %+v, want zero-share delete sentinel", oldx)
	}

	// Snapshots: brokerage positive, credit negated, both source=plaid.
	if len(snapshots.written) != 2 {
		t.Fatalf("got %d snapshots, want 2", len(snapshots.written))
	}
	byAccount := map[uuid.UUID]db.UpsertSnapshotParams{}
	for _, s := range snapshots.written {
		byAccount[s.AccountID] = s
		if s.Source != "plaid" {
			t.Errorf("snapshot source = %q, want plaid", s.Source)
		}
	}
	if math.Abs(byAccount[brokerageID].Balance-2500) > 0.01 {
		t.Errorf("brokerage snapshot = %v, want 2500", byAccount[brokerageID].Balance)
	}
	if math.Abs(byAccount[creditID].Balance-(-430.25)) > 0.01 {
		t.Errorf("credit snapshot = %v, want -430.25 (negated)", byAccount[creditID].Balance)
	}

	if !items.syncOK {
		t.Error("expected successful sync to be recorded on the item")
	}
	if result.AccountsSynced != 2 || result.SnapshotsWritten != 2 {
		t.Errorf("result = %+v, want 2 accounts synced / 2 snapshots", result)
	}
}

// TestFillRecentGaps covers the resilience case: the app was stopped for a
// while, so an investment account's last snapshot is a few weeks stale.
// SyncItem should reconstruct the missing weeks from investment transactions,
// leaving the already-synced week and the current week (handled separately)
// untouched.
func TestFillRecentGaps(t *testing.T) {
	cipher, _ := crypto.NewCipher(testHexKey)
	userID := uuid.New()
	brokerageID := uuid.New()
	brokerageExt := "p-brokerage"

	now := time.Now().UTC()
	currentWeek := thisWeekMonday(now)
	week1Ago := currentWeek.AddDate(0, 0, -7)  // most recently completed week
	week2Ago := currentWeek.AddDate(0, 0, -14) // week before that
	week3Ago := currentWeek.AddDate(0, 0, -21) // already has a snapshot (last sync)

	bal := 1000.0
	api := &fakeAPI{
		accounts: []Account{
			{AccountID: "p-brokerage", Name: "Brokerage", Type: "investment", Subtype: "brokerage", CurrentBalance: &bal, Currency: "USD"},
		},
		securities: []Security{
			{SecurityID: "s-vti", Ticker: "VTI", Name: "Vanguard Total Market", Type: "etf"},
		},
		holdings: []Holding{
			{AccountID: "p-brokerage", SecurityID: "s-vti", Quantity: 10, InstitutionPrice: 100, InstitutionValue: 1000},
		},
		txns: []InvestmentTransaction{
			// Bought 4 shares at $80 (below today's $100 price) partway
			// through the most recently completed week.
			{AccountID: "p-brokerage", SecurityID: "s-vti", Date: week1Ago.AddDate(0, 0, 3), Quantity: 4, Amount: 320, Type: "buy"},
		},
	}

	linked := []models.Account{
		{ID: brokerageID, UserID: userID, Name: "Brokerage", Currency: "USD", ExternalAccountID: &brokerageExt},
	}
	accounts := &fakeAccountStore{linked: linked}
	holdingsStore := &fakeHoldingStore{}
	snapshots := &fakeSnapshotStore{
		latest: map[uuid.UUID]*models.BalanceSnapshot{
			brokerageID: {SnapshotDate: week3Ago, Source: "plaid"},
		},
	}
	items := &fakeItemStore{}

	svc := &Service{clientFor: fixedAPI(api), cipher: cipher, items: items, accounts: accounts, holdings: holdingsStore, snapshots: snapshots, candles: fakeCandles{}}

	item := models.PlaidItem{ID: uuid.New(), UserID: userID, ItemID: "item-1", AccessTokenEncrypted: encryptedToken(t, cipher)}
	result, err := svc.SyncItem(context.Background(), item)
	if err != nil {
		t.Fatalf("SyncItem: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	// 1 "today" balance snapshot from the regular sync + 2 gap-filled weekly
	// snapshots (week2Ago, week1Ago). week3Ago already had one (untouched,
	// not re-written) and the current week is covered by the regular sync.
	if len(snapshots.written) != 3 {
		t.Fatalf("got %d total snapshots written, want 3: %+v", len(snapshots.written), snapshots.written)
	}
	if result.SnapshotsWritten != 3 {
		t.Errorf("result.SnapshotsWritten = %d, want 3", result.SnapshotsWritten)
	}

	byDate := map[time.Time]db.UpsertSnapshotParams{}
	for _, s := range snapshots.written {
		byDate[s.SnapshotDate] = s
	}
	// Before the buy: 6 shares × $100 (current fallback price) + $320 cash
	// (the money that would not yet have been spent) = $920.
	for _, wk := range []time.Time{week1Ago, week2Ago} {
		snap, ok := byDate[wk]
		if !ok {
			t.Fatalf("missing gap-filled snapshot for %v: %+v", wk, snapshots.written)
		}
		if math.Abs(snap.Balance-920) > 0.01 {
			t.Errorf("%v balance = %v, want 920", wk, snap.Balance)
		}
		if snap.Source != "plaid" {
			t.Errorf("%v source = %q, want plaid", wk, snap.Source)
		}
	}
	if snap, ok := byDate[week3Ago]; ok {
		t.Errorf("week3Ago should not have been re-written by gap-fill, got %+v", snap)
	}
}

// TestSyncAccountHoldingsPreservesManualPrice covers the case where a user
// has manually corrected a holding's price (e.g. a CUSIP-only security, or a
// resolvable ticker they've overridden). Shares and cost basis should still
// update live from Plaid, but the price they set must survive the next sync
// instead of being reset to Plaid's institution_price.
func TestSyncAccountHoldingsPreservesManualPrice(t *testing.T) {
	userID := uuid.New()
	accountID := uuid.New()
	acc := models.Account{ID: accountID, UserID: userID, Name: "Brokerage"}

	existingPrice := 55.0
	holdingsStore := &fakeHoldingStore{
		existing: []models.Holding{
			{AccountID: accountID, Ticker: "VOOG", AssetClass: "equity", Shares: 3, ManualPrice: true, PricePerShare: &existingPrice},
		},
	}
	svc := &Service{holdings: holdingsStore}

	secByID := map[string]Security{
		"s-voog": {SecurityID: "s-voog", Ticker: "VOOG", Name: "Vanguard Growth ETF", Type: "etf"},
	}
	newCB := 300.0
	allHoldings := []Holding{
		{AccountID: "p-brokerage", SecurityID: "s-voog", Quantity: 5, InstitutionPrice: 60, InstitutionValue: 300, CostBasis: &newCB},
	}

	n, err := svc.syncAccountHoldings(context.Background(), acc, "p-brokerage", allHoldings, secByID)
	if err != nil {
		t.Fatalf("syncAccountHoldings: %v", err)
	}
	if n != 1 {
		t.Fatalf("got %d replaced, want 1", n)
	}

	params := holdingsStore.replaced[accountID]
	if len(params) != 1 {
		t.Fatalf("got %d params, want 1: %+v", len(params), params)
	}
	p := params[0]
	if p.Shares != 5 {
		t.Errorf("shares = %v, want 5 (updated live from Plaid)", p.Shares)
	}
	// newCB=300 is Plaid's total cost basis for 5 shares → $60/share stored.
	if p.CostBasis == nil || math.Abs(*p.CostBasis-60) > 0.01 {
		t.Errorf("cost basis = %v, want 60 (per-share, updated live from Plaid)", p.CostBasis)
	}
	if !p.ManualPrice {
		t.Error("expected manual_price to stay true")
	}
	if p.PricePerShare == nil || math.Abs(*p.PricePerShare-55) > 0.01 {
		t.Errorf("price per share = %v, want 55 (preserved, not Plaid's institution_price of 60)", p.PricePerShare)
	}
}

func TestSyncItemCurrencyMismatch(t *testing.T) {
	cipher, _ := crypto.NewCipher(testHexKey)
	userID := uuid.New()
	accID := uuid.New()
	ext := "p-eur"
	bal := 100.0

	api := &fakeAPI{accounts: []Account{
		{AccountID: "p-eur", Name: "EUR Acct", Type: "depository", Subtype: "checking", CurrentBalance: &bal, Currency: "EUR"},
	}}
	accounts := &fakeAccountStore{linked: []models.Account{
		{ID: accID, UserID: userID, Name: "Checking", Currency: "USD", ExternalAccountID: &ext},
	}}
	snapshots := &fakeSnapshotStore{}
	items := &fakeItemStore{}

	svc := &Service{clientFor: fixedAPI(api), cipher: cipher, items: items, accounts: accounts, holdings: &fakeHoldingStore{}, snapshots: snapshots}
	item := models.PlaidItem{ID: uuid.New(), UserID: userID, ItemID: "item-1", AccessTokenEncrypted: encryptedToken(t, cipher)}

	result, err := svc.SyncItem(context.Background(), item)
	if err != nil {
		t.Fatalf("SyncItem: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 currency-mismatch error, got %v", result.Errors)
	}
	if len(snapshots.written) != 0 {
		t.Errorf("expected no snapshots for mismatched currency, got %d", len(snapshots.written))
	}
}

// refreshFixture is a Fidelity-style Item: the investment account reports a
// balance but Plaid has no positions for it until a refresh is requested.
func refreshFixture(t *testing.T, balance float64, acctType string) (*Service, *fakeAPI, *fakeItemStore, *fakeHoldingStore, *fakeSnapshotStore, models.PlaidItem, uuid.UUID) {
	t.Helper()
	cipher, _ := crypto.NewCipher(testHexKey)
	userID, accID := uuid.New(), uuid.New()
	ext := "p-fid"

	api := &fakeAPI{
		accounts: []Account{
			{AccountID: "p-fid", Name: "Fidelity Brokerage", Type: acctType, Subtype: "brokerage", CurrentBalance: &balance, Currency: "USD"},
		},
		holdingsAfterRefresh: []Holding{
			{AccountID: "p-fid", SecurityID: "s-fnilx", Quantity: 10, InstitutionPrice: 27.7, InstitutionValue: 277},
		},
		securitiesAfterRefresh: []Security{
			{SecurityID: "s-fnilx", Ticker: "FNILX", Name: "Fidelity ZERO Large Cap", Type: "mutual fund"},
		},
	}
	items := &fakeItemStore{}
	holdings := &fakeHoldingStore{}
	snapshots := &fakeSnapshotStore{
		// Synced this week, so fillRecentGaps stays out of the way.
		latest: map[uuid.UUID]*models.BalanceSnapshot{accID: {SnapshotDate: time.Now().UTC(), Source: "plaid"}},
	}
	svc := &Service{
		clientFor: fixedAPI(api), cipher: cipher, items: items, holdings: holdings, snapshots: snapshots, candles: fakeCandles{},
		accounts: &fakeAccountStore{linked: []models.Account{{ID: accID, UserID: userID, Name: "Fidelity Brokerage", Currency: "USD", ExternalAccountID: &ext}}},
	}
	item := models.PlaidItem{ID: uuid.New(), UserID: userID, ItemID: "item-1", AccessTokenEncrypted: encryptedToken(t, cipher)}
	return svc, api, items, holdings, snapshots, item, accID
}

func TestSyncItemRefreshesEmptyInvestmentAccount(t *testing.T) {
	svc, api, items, holdings, _, item, accID := refreshFixture(t, 1000, "investment")

	result, err := svc.SyncItem(context.Background(), item)
	if err != nil {
		t.Fatalf("SyncItem: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if items.refreshClaims != 1 || api.refreshCalls != 1 {
		t.Errorf("claims=%d refreshes=%d, want exactly 1 each", items.refreshClaims, api.refreshCalls)
	}
	// The holdings fetched after the refresh are what get stored.
	got := holdings.replaced[accID]
	if len(got) != 1 || got[0].Ticker != "FNILX" || got[0].Shares != 10 {
		t.Errorf("stored holdings = %+v, want FNILX x10 from the post-refresh fetch", got)
	}
	if result.HoldingsReplaced != 1 {
		t.Errorf("HoldingsReplaced = %d, want 1", result.HoldingsReplaced)
	}
}

func TestSyncItemRefreshLimitedToOncePerWindow(t *testing.T) {
	svc, api, items, holdings, snapshots, item, accID := refreshFixture(t, 1000, "investment")
	items.refreshDenied = true // already refreshed within the last day

	result, err := svc.SyncItem(context.Background(), item)
	if err != nil {
		t.Fatalf("SyncItem: %v", err)
	}
	if items.refreshClaims != 1 {
		t.Errorf("claims = %d, want 1 attempt", items.refreshClaims)
	}
	if api.refreshCalls != 0 {
		t.Errorf("refreshCalls = %d, want 0 (claim denied, so no billed call)", api.refreshCalls)
	}
	if len(holdings.replaced[accID]) != 0 {
		t.Errorf("holdings stored = %+v, want none", holdings.replaced[accID])
	}
	if len(result.Errors) != 0 {
		t.Errorf("a denied claim is not an error, got %v", result.Errors)
	}
	// The balance sync still happens.
	if len(snapshots.written) != 1 {
		t.Errorf("snapshots written = %d, want 1", len(snapshots.written))
	}
}

func TestSyncItemRefreshFailureIsNonFatal(t *testing.T) {
	svc, api, _, _, snapshots, item, _ := refreshFixture(t, 1000, "investment")
	api.refreshErr = errors.New("PRODUCTS_NOT_READY")

	result, err := svc.SyncItem(context.Background(), item)
	if err != nil {
		t.Fatalf("SyncItem must not fail when the refresh does: %v", err)
	}
	if api.refreshCalls != 1 {
		t.Errorf("refreshCalls = %d, want 1", api.refreshCalls)
	}
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "investments refresh") {
		t.Errorf("errors = %v, want one investments refresh error", result.Errors)
	}
	if len(snapshots.written) != 1 {
		t.Errorf("snapshots written = %d, want 1 (balance sync continues)", len(snapshots.written))
	}
}

func TestSyncItemNoRefreshWhenNotNeeded(t *testing.T) {
	cases := []struct {
		name     string
		balance  float64
		acctType string
		holdings bool
	}{
		{"investment account already has holdings", 1000, "investment", true},
		{"investment account with zero balance", 0, "investment", false},
		{"non-investment account with a balance", 1000, "depository", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, api, items, _, _, item, _ := refreshFixture(t, tc.balance, tc.acctType)
			if tc.holdings {
				api.holdings = api.holdingsAfterRefresh
				api.securities = api.securitiesAfterRefresh
			}
			if _, err := svc.SyncItem(context.Background(), item); err != nil {
				t.Fatalf("SyncItem: %v", err)
			}
			if items.refreshClaims != 0 || api.refreshCalls != 0 {
				t.Errorf("claims=%d refreshes=%d, want none", items.refreshClaims, api.refreshCalls)
			}
		})
	}
}

// ---- Banking transaction sync (Phase 4) ----

// fakeTxnStore captures upserts and deletes.
type fakeTxnStore struct {
	upserts   []db.UpsertExternalParams
	deletedID []string
	// adoptIDs are external ids the store should report as adopted (matched a
	// pre-existing manual row) rather than inserted.
	adoptIDs map[string]bool
	adopted  []string
}

func (f *fakeTxnStore) UpsertExternal(_ context.Context, p db.UpsertExternalParams) (bool, error) {
	f.upserts = append(f.upserts, p)
	return true, nil
}
func (f *fakeTxnStore) TryAdoptExternalMatch(_ context.Context, p db.UpsertExternalParams) (bool, error) {
	if f.adoptIDs[p.ExternalTransactionID] {
		f.adopted = append(f.adopted, p.ExternalTransactionID)
		return true, nil
	}
	return false, nil
}
func (f *fakeTxnStore) DeleteByExternalID(_ context.Context, _ uuid.UUID, _, externalID string) error {
	f.deletedID = append(f.deletedID, externalID)
	return nil
}

// fakeClassifier resolves a fixed set of category names and never matches on
// rules/history (everything unresolved becomes needs_review), so tests assert
// PFC-hint and Income/Transfer routing deterministically.
type fakeClassifier struct {
	byName map[string]uuid.UUID
}

func (f *fakeClassifier) CategoryIDByName(name string) uuid.UUID { return f.byName[name] }
func (f *fakeClassifier) ClassifyBatchRulesOnly(_ context.Context, _ uuid.UUID, merchants, _ []string) ([]*uuid.UUID, []bool) {
	catIDs := make([]*uuid.UUID, len(merchants))
	needsReview := make([]bool, len(merchants))
	for i := range merchants {
		needsReview[i] = true // unresolved → needs review
	}
	return catIDs, needsReview
}

// txnSyncFixture builds a Service wired for transaction sync against one linked
// checking account, returning the pieces tests assert on.
func txnSyncFixture(t *testing.T, pages []TransactionSyncPage) (*Service, *fakeTxnStore, *fakeItemStore, models.PlaidItem, uuid.UUID, uuid.UUID) {
	t.Helper()
	cipher, _ := crypto.NewCipher(testHexKey)
	userID := uuid.New()
	acctID := uuid.New()
	ext := "p-checking"

	incomeID, transferID, diningID := uuid.New(), uuid.New(), uuid.New()

	api := &fakeAPI{
		accounts: []Account{{AccountID: ext, Name: "Checking", Type: "depository", Subtype: "checking", Currency: "USD"}},
		txnPages: pages,
	}
	accounts := &fakeAccountStore{linked: []models.Account{
		{ID: acctID, UserID: userID, Name: "Checking", Currency: "USD", ExternalAccountID: &ext},
	}}
	txns := &fakeTxnStore{}
	classifier := &fakeClassifier{byName: map[string]uuid.UUID{
		CategoryIncome:         incomeID,
		CategoryTransfer:       transferID,
		"Dining & Restaurants": diningID,
	}}
	items := &fakeItemStore{}

	// A depository balance with no CurrentBalance avoids snapshot noise.
	svc := &Service{clientFor: fixedAPI(api), cipher: cipher, items: items, accounts: accounts, holdings: &fakeHoldingStore{}, snapshots: &fakeSnapshotStore{}}
	svc.SetTransactionSync(txns, classifier)

	item := models.PlaidItem{ID: uuid.New(), UserID: userID, ItemID: "item-1", AccessTokenEncrypted: encryptedToken(t, cipher)}
	return svc, txns, items, item, incomeID, transferID
}

func TestSyncTransactions_SignAndCategoryRouting(t *testing.T) {
	pages := []TransactionSyncPage{{
		Added: []BankTransaction{
			{TransactionID: "t-dining", AccountID: "p-checking", Date: time.Now(), Name: "CHIPOTLE 123", MerchantName: "Chipotle", Amount: 12.50, PFCPrimary: "FOOD_AND_DRINK", PFCDetailed: "FOOD_AND_DRINK_RESTAURANT"},
			{TransactionID: "t-pay", AccountID: "p-checking", Date: time.Now(), Name: "ACME PAYROLL", Amount: -3000, PFCPrimary: "INCOME", PFCDetailed: "INCOME_WAGES"},
			{TransactionID: "t-xfer", AccountID: "p-checking", Date: time.Now(), Name: "CC PAYMENT", Amount: 500, PFCPrimary: "LOAN_PAYMENTS", PFCDetailed: "LOAN_PAYMENTS_CREDIT_CARD_PAYMENT"},
			{TransactionID: "t-unknown", AccountID: "p-checking", Date: time.Now(), Name: "MYSTERY LLC", Amount: 40, PFCPrimary: "GENERAL_SERVICES", PFCDetailed: "GENERAL_SERVICES_CONSULTING"},
		},
		NextCursor: "cursor-final",
		HasMore:    false,
	}}
	svc, txns, items, item, incomeID, transferID := txnSyncFixture(t, pages)

	result, err := svc.SyncItem(context.Background(), item)
	if err != nil {
		t.Fatalf("SyncItem: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if result.TransactionsSynced != 4 {
		t.Errorf("TransactionsSynced = %d, want 4", result.TransactionsSynced)
	}

	byID := map[string]db.UpsertExternalParams{}
	for _, u := range txns.upserts {
		byID[u.ExternalTransactionID] = u
	}

	// Dining: stored positive, categorised via PFC hint, not flagged.
	if d := byID["t-dining"]; d.Amount != 12.50 || d.CategoryID == nil || d.NeedsReview {
		t.Errorf("dining txn wrong: %+v", d)
	}
	// Income: Plaid inflow (-3000) stored positive under Income category.
	if p := byID["t-pay"]; p.Amount != 3000 || p.CategoryID == nil || *p.CategoryID != incomeID {
		t.Errorf("income txn wrong: amount=%v cat=%v", p.Amount, p.CategoryID)
	}
	// Credit-card payment: routed to Transfer, stored positive.
	if x := byID["t-xfer"]; x.Amount != 500 || x.CategoryID == nil || *x.CategoryID != transferID {
		t.Errorf("transfer txn wrong: amount=%v cat=%v", x.Amount, x.CategoryID)
	}
	// Unknown spend: no PFC hint, unresolved by rules → needs_review, no category.
	if u := byID["t-unknown"]; u.CategoryID != nil || !u.NeedsReview {
		t.Errorf("unknown txn should be needs_review with no category: %+v", u)
	}

	// Cursor persisted after the final page.
	if !items.cursorSet || items.savedCursor != "cursor-final" {
		t.Errorf("cursor not persisted correctly: set=%v value=%q", items.cursorSet, items.savedCursor)
	}
}

// TestSyncTransactions_HistoryBackfillAndRemoval covers the first sync of a
// fresh Item: with a nil stored cursor the loop starts from the empty cursor
// (Plaid's "start of history") and pages through HasMore until exhausted,
// which is exactly the transaction backfill. Also covers removals.
func TestSyncTransactions_HistoryBackfillAndRemoval(t *testing.T) {
	pages := []TransactionSyncPage{
		{
			Added:      []BankTransaction{{TransactionID: "t-1", AccountID: "p-checking", Date: time.Now(), Name: "STORE A", Amount: 10, PFCPrimary: "GENERAL_MERCHANDISE"}},
			NextCursor: "cursor-1",
			HasMore:    true,
		},
		{
			Added:      []BankTransaction{{TransactionID: "t-2", AccountID: "p-checking", Date: time.Now(), Name: "STORE B", Amount: 20, PFCPrimary: "GENERAL_MERCHANDISE"}},
			RemovedIDs: []string{"t-old"},
			NextCursor: "cursor-2",
			HasMore:    false,
		},
	}
	svc, txns, items, item, _, _ := txnSyncFixture(t, pages)
	api := fakeOf(svc)

	if _, err := svc.SyncItem(context.Background(), item); err != nil {
		t.Fatalf("SyncItem: %v", err)
	}
	// First call starts from the empty cursor (history), second resumes from the
	// page-1 cursor — proving pagination advances rather than refetching.
	if len(api.syncCalls) != 2 || api.syncCalls[0] != "" || api.syncCalls[1] != "cursor-1" {
		t.Errorf("cursor sequence wrong: %v (want [\"\", \"cursor-1\"])", api.syncCalls)
	}
	if len(txns.upserts) != 2 {
		t.Errorf("expected 2 upserts across pages, got %d", len(txns.upserts))
	}
	if len(txns.deletedID) != 1 || txns.deletedID[0] != "t-old" {
		t.Errorf("expected t-old removed, got %v", txns.deletedID)
	}
	if items.savedCursor != "cursor-2" {
		t.Errorf("cursor should advance to final page, got %q", items.savedCursor)
	}
}

// TestSyncTransactions_ResumesFromStoredCursor confirms an incremental sync
// starts from the Item's persisted cursor rather than re-pulling history.
func TestSyncTransactions_ResumesFromStoredCursor(t *testing.T) {
	pages := []TransactionSyncPage{{
		Added:      []BankTransaction{{TransactionID: "t-new", AccountID: "p-checking", Date: time.Now(), Name: "STORE C", Amount: 5, PFCPrimary: "GENERAL_MERCHANDISE"}},
		NextCursor: "cursor-next",
		HasMore:    false,
	}}
	svc, _, _, item, _, _ := txnSyncFixture(t, pages)
	api := fakeOf(svc)
	stored := "cursor-prev"
	item.TransactionsCursor = &stored

	if _, err := svc.SyncItem(context.Background(), item); err != nil {
		t.Fatalf("SyncItem: %v", err)
	}
	if len(api.syncCalls) != 1 || api.syncCalls[0] != "cursor-prev" {
		t.Errorf("incremental sync should start from stored cursor, got %v", api.syncCalls)
	}
}

// TestSyncTransactions_AdoptsCsvDuplicate verifies that a Plaid transaction
// matching a pre-existing manual/CSV row is adopted (external id stamped onto
// the existing row) rather than inserted as a second copy, while a
// non-matching transaction still inserts normally.
func TestSyncTransactions_AdoptsCsvDuplicate(t *testing.T) {
	pages := []TransactionSyncPage{{
		Added: []BankTransaction{
			{TransactionID: "t-dup", AccountID: "p-checking", Date: time.Now(), Name: "PAYROLL", Amount: 500, PFCPrimary: "INCOME", PFCDetailed: "INCOME_WAGES"},
			{TransactionID: "t-fresh", AccountID: "p-checking", Date: time.Now(), Name: "COFFEE", Amount: 4, PFCPrimary: "FOOD_AND_DRINK", PFCDetailed: "FOOD_AND_DRINK_RESTAURANT"},
		},
		NextCursor: "c",
		HasMore:    false,
	}}
	svc, txns, _, item, _, _ := txnSyncFixture(t, pages)
	txns.adoptIDs = map[string]bool{"t-dup": true} // a CSV row already exists for this one

	result, err := svc.SyncItem(context.Background(), item)
	if err != nil {
		t.Fatalf("SyncItem: %v", err)
	}
	// t-dup adopted (not inserted); t-fresh inserted.
	if len(txns.adopted) != 1 || txns.adopted[0] != "t-dup" {
		t.Errorf("expected t-dup adopted, got %v", txns.adopted)
	}
	if len(txns.upserts) != 1 || txns.upserts[0].ExternalTransactionID != "t-fresh" {
		t.Errorf("expected only t-fresh upserted, got %+v", txns.upserts)
	}
	// Both count toward the synced total (one adopted, one inserted).
	if result.TransactionsSynced != 2 {
		t.Errorf("TransactionsSynced = %d, want 2", result.TransactionsSynced)
	}
}

func TestSyncTransactions_SkipsPending(t *testing.T) {
	pages := []TransactionSyncPage{{
		Added: []BankTransaction{
			{TransactionID: "t-pending", AccountID: "p-checking", Date: time.Now(), Name: "PENDING COFFEE", Amount: 5, Pending: true, PFCPrimary: "FOOD_AND_DRINK"},
			{TransactionID: "t-posted", AccountID: "p-checking", Date: time.Now(), Name: "POSTED LUNCH", Amount: 15, PFCPrimary: "FOOD_AND_DRINK", PFCDetailed: "FOOD_AND_DRINK_RESTAURANT"},
		},
		NextCursor: "c",
		HasMore:    false,
	}}
	svc, txns, _, item, _, _ := txnSyncFixture(t, pages)

	if _, err := svc.SyncItem(context.Background(), item); err != nil {
		t.Fatalf("SyncItem: %v", err)
	}
	if len(txns.upserts) != 1 || txns.upserts[0].ExternalTransactionID != "t-posted" {
		t.Errorf("pending should be skipped, only posted stored; got %+v", txns.upserts)
	}
}

func TestSyncTransactions_SkippedWhenNoBankAccount(t *testing.T) {
	// Investment-only item: /transactions/sync must not be called at all.
	pages := []TransactionSyncPage{{Added: []BankTransaction{{TransactionID: "x", AccountID: "p-brk", Amount: 1}}}}
	svc, txns, items, item, _, _ := txnSyncFixture(t, pages)

	// Re-point the API + linked account to an investment account only.
	api := fakeOf(svc)
	api.accounts = []Account{{AccountID: "p-brk", Name: "Brokerage", Type: "investment", Subtype: "brokerage", Currency: "USD"}}
	ext := "p-brk"
	svc.accounts = &fakeAccountStore{linked: []models.Account{{ID: uuid.New(), UserID: item.UserID, Name: "Brk", Currency: "USD", ExternalAccountID: &ext}}}

	if _, err := svc.SyncItem(context.Background(), item); err != nil {
		t.Fatalf("SyncItem: %v", err)
	}
	if len(txns.upserts) != 0 {
		t.Errorf("no transactions expected for investment-only item, got %d", len(txns.upserts))
	}
	if len(api.syncCalls) != 0 {
		t.Errorf("SyncTransactions should not be called for investment-only item, got %d calls", len(api.syncCalls))
	}
	if items.cursorSet {
		t.Error("cursor should not be set when transaction sync is skipped")
	}
}
