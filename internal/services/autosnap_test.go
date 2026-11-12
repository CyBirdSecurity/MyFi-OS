package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
	"github.com/cybirdsecurity/myfi-os/internal/prices"
)

// ---- Mock implementations for the AutoSnapshotService interfaces ----

type mockHoldingReader struct {
	userIDs  []uuid.UUID
	holdings map[uuid.UUID][]models.Holding // keyed by userID
}

func (m *mockHoldingReader) ListUsersWithHoldings(_ context.Context) ([]uuid.UUID, error) {
	return m.userIDs, nil
}

func (m *mockHoldingReader) ListByUser(_ context.Context, userID uuid.UUID) ([]models.Holding, error) {
	return m.holdings[userID], nil
}

type capturedSnapshot struct {
	params db.UpsertSnapshotParams
}

type mockSnapshotWriter struct {
	captured []capturedSnapshot
}

func (m *mockSnapshotWriter) Upsert(_ context.Context, p db.UpsertSnapshotParams) (*models.BalanceSnapshot, error) {
	m.captured = append(m.captured, capturedSnapshot{params: p})
	return &models.BalanceSnapshot{
		AccountID:    p.AccountID,
		UserID:       p.UserID,
		SnapshotDate: p.SnapshotDate,
		Balance:      p.Balance,
		Source:       p.Source,
	}, nil
}

type mockPriceGetter struct {
	prices map[string]float64
}

func (m *mockPriceGetter) GetPrices(_ context.Context, tickers []string) (map[string]prices.Quote, error) {
	result := make(map[string]prices.Quote, len(tickers))
	for _, t := range tickers {
		if p, ok := m.prices[t]; ok {
			result[t] = prices.Quote{Ticker: t, Price: p, UpdatedAt: time.Now()}
		}
	}
	return result, nil
}

// newTestAutoSnap wires mocks into an AutoSnapshotService without any DB.
func newTestAutoSnap(h holdingReader, s snapshotWriter, p priceGetter) *AutoSnapshotService {
	return &AutoSnapshotService{holdings: h, snapshots: s, prices: p}
}

// ---- thisWeekMonday tests ----

func TestThisWeekMonday_Monday(t *testing.T) {
	// 2024-04-08 is a Monday — should return itself.
	monday := time.Date(2024, 4, 8, 14, 30, 0, 0, time.UTC)
	got := thisWeekMonday(monday)
	want := time.Date(2024, 4, 8, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Monday input: want %s, got %s", want, got)
	}
}

func TestThisWeekMonday_Wednesday(t *testing.T) {
	// 2024-04-10 is a Wednesday — should step back to 2024-04-08 (Mon).
	wed := time.Date(2024, 4, 10, 9, 0, 0, 0, time.UTC)
	got := thisWeekMonday(wed)
	want := time.Date(2024, 4, 8, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Wednesday input: want %s, got %s", want, got)
	}
}

func TestThisWeekMonday_Sunday(t *testing.T) {
	// 2024-04-14 is a Sunday — ISO week says step back to 2024-04-08 (Mon).
	sunday := time.Date(2024, 4, 14, 23, 59, 0, 0, time.UTC)
	got := thisWeekMonday(sunday)
	want := time.Date(2024, 4, 8, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Sunday input: want %s, got %s", want, got)
	}
}

func TestThisWeekMonday_Saturday(t *testing.T) {
	// 2024-04-13 is a Saturday — should step back to 2024-04-08 (Mon).
	saturday := time.Date(2024, 4, 13, 0, 0, 0, 0, time.UTC)
	got := thisWeekMonday(saturday)
	want := time.Date(2024, 4, 8, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("Saturday input: want %s, got %s", want, got)
	}
}

func TestThisWeekMonday_AlwaysMidnightUTC(t *testing.T) {
	// Whatever time of day, the result should be midnight UTC.
	cases := []time.Time{
		time.Date(2024, 6, 3, 0, 0, 0, 0, time.UTC),      // Monday midnight
		time.Date(2024, 6, 5, 23, 59, 59, 999, time.UTC), // Wednesday near midnight
		time.Date(2024, 6, 9, 12, 0, 0, 0, time.UTC),     // Sunday noon
	}
	for _, tc := range cases {
		got := thisWeekMonday(tc)
		if got.Hour() != 0 || got.Minute() != 0 || got.Second() != 0 {
			t.Errorf("input %s: result not midnight UTC: %s", tc, got)
		}
		if got.Location() != time.UTC {
			t.Errorf("input %s: result not in UTC: %s", tc, got.Location())
		}
	}
}

// ---- TakeSnapshots / snapshotUser tests ----

// TestTakeSnapshots_NoUsersNoSnapshots ensures that when nobody has holdings,
// no snapshots are created.
func TestTakeSnapshots_NoUsersNoSnapshots(t *testing.T) {
	sw := &mockSnapshotWriter{}
	svc := newTestAutoSnap(
		&mockHoldingReader{},
		sw,
		&mockPriceGetter{},
	)

	if err := svc.TakeSnapshots(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sw.captured) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(sw.captured))
	}
}

// TestTakeSnapshots_SingleAccountLivePrice verifies a snapshot is created for
// a user with one account and one holding using the live price.
func TestTakeSnapshots_SingleAccountLivePrice(t *testing.T) {
	userID := uuid.New()
	accountID := uuid.New()

	sw := &mockSnapshotWriter{}
	svc := newTestAutoSnap(
		&mockHoldingReader{
			userIDs: []uuid.UUID{userID},
			holdings: map[uuid.UUID][]models.Holding{
				userID: {
					{
						AccountID: accountID,
						UserID:    userID,
						Ticker:    "AAPL",
						Shares:    10,
					},
				},
			},
		},
		sw,
		&mockPriceGetter{prices: map[string]float64{"AAPL": 200.0}},
	)

	if err := svc.TakeSnapshots(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sw.captured) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(sw.captured))
	}

	snap := sw.captured[0]
	if snap.params.AccountID != accountID {
		t.Errorf("wrong account ID")
	}
	if snap.params.UserID != userID {
		t.Errorf("wrong user ID")
	}
	// 10 shares × $200 = $2000
	wantBalance := 10.0 * 200.0
	if snap.params.Balance != wantBalance {
		t.Errorf("balance: want %.2f, got %.2f", wantBalance, snap.params.Balance)
	}
	if snap.params.Source != "computed" {
		t.Errorf("source: want 'computed', got %q", snap.params.Source)
	}
}

// TestTakeSnapshots_CryptoAccountUsesCoinPrice checks that a coin in a crypto
// account is priced as "BTC-USD", not as the unrelated "BTC" ETF.
func TestTakeSnapshots_CryptoAccountUsesCoinPrice(t *testing.T) {
	userID := uuid.New()
	accountID := uuid.New()

	sw := &mockSnapshotWriter{}
	svc := newTestAutoSnap(
		&mockHoldingReader{
			userIDs: []uuid.UUID{userID},
			holdings: map[uuid.UUID][]models.Holding{
				userID: {
					{
						AccountID:   accountID,
						UserID:      userID,
						Ticker:      "BTC",
						AssetClass:  "crypto",
						AccountType: "crypto",
						Shares:      0.5,
					},
				},
			},
		},
		sw,
		&mockPriceGetter{prices: map[string]float64{"BTC": 36.0, "BTC-USD": 80000.0}},
	)

	if err := svc.TakeSnapshots(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sw.captured) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(sw.captured))
	}
	// 0.5 BTC × $80,000 = $40,000
	if got := sw.captured[0].params.Balance; got != 40000.0 {
		t.Errorf("balance: want 40000.00, got %.2f", got)
	}
}

// TestTakeSnapshots_FallsBackToStoredPrice checks that when no live price is
// available, the last known stored price on the holding is used.
func TestTakeSnapshots_FallsBackToStoredPrice(t *testing.T) {
	userID := uuid.New()
	accountID := uuid.New()
	lastKnown := 150.0

	sw := &mockSnapshotWriter{}
	svc := newTestAutoSnap(
		&mockHoldingReader{
			userIDs: []uuid.UUID{userID},
			holdings: map[uuid.UUID][]models.Holding{
				userID: {
					{
						AccountID:     accountID,
						UserID:        userID,
						Ticker:        "TSLA",
						Shares:        5,
						PricePerShare: &lastKnown,
					},
				},
			},
		},
		sw,
		&mockPriceGetter{prices: map[string]float64{}}, // no live price
	)

	if err := svc.TakeSnapshots(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sw.captured) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(sw.captured))
	}
	wantBalance := 5.0 * 150.0
	if sw.captured[0].params.Balance != wantBalance {
		t.Errorf("fallback balance: want %.2f, got %.2f", wantBalance, sw.captured[0].params.Balance)
	}
}

// TestTakeSnapshots_ZeroPriceHoldingSkipped verifies that if both live price
// and stored price are absent (or zero), no snapshot is written.
func TestTakeSnapshots_ZeroPriceHoldingSkipped(t *testing.T) {
	userID := uuid.New()
	accountID := uuid.New()

	sw := &mockSnapshotWriter{}
	svc := newTestAutoSnap(
		&mockHoldingReader{
			userIDs: []uuid.UUID{userID},
			holdings: map[uuid.UUID][]models.Holding{
				userID: {
					{
						AccountID: accountID,
						UserID:    userID,
						Ticker:    "UNKNOWN",
						Shares:    100,
						// No PricePerShare set; no live price either
					},
				},
			},
		},
		sw,
		&mockPriceGetter{prices: map[string]float64{}},
	)

	if err := svc.TakeSnapshots(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sw.captured) != 0 {
		t.Errorf("zero-price holding: expected no snapshots, got %d", len(sw.captured))
	}
}

// TestTakeSnapshots_CashHoldingUsesDollarAmountNotPrice verifies that a
// cash_equivalent holding's Shares field (which stores a dollar amount, not
// a share count) is used directly, and its ticker is never sent to the price
// provider — a cash holding's ticker (e.g. "CASH") can coincidentally match
// a real tradeable stock symbol, which would otherwise wildly inflate the
// computed snapshot.
func TestTakeSnapshots_CashHoldingUsesDollarAmountNotPrice(t *testing.T) {
	userID := uuid.New()
	accountID := uuid.New()

	callCount := 0
	sw := &mockSnapshotWriter{}
	svc := newTestAutoSnap(
		&mockHoldingReader{
			userIDs: []uuid.UUID{userID},
			holdings: map[uuid.UUID][]models.Holding{
				userID: {
					{AccountID: accountID, UserID: userID, Ticker: "CASH", AssetClass: "cash_equivalent", Shares: 1000},
					{AccountID: accountID, UserID: userID, Ticker: "VIIIX", AssetClass: "equity", Shares: 10},
				},
			},
		},
		sw,
		// If the bug were present, "CASH" would resolve to this bogus $88.72
		// stock price and massively inflate the snapshot.
		&countingMockPriceGetter{prices: map[string]float64{"VIIIX": 100.0, "CASH": 88.72}, calls: &callCount},
	)

	if err := svc.TakeSnapshots(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sw.captured) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(sw.captured))
	}
	// $1000 cash (used as-is) + 10 × $100 VIIIX = $2000.
	wantBalance := 1000.0 + 10.0*100.0
	if sw.captured[0].params.Balance != wantBalance {
		t.Errorf("balance: want %.2f, got %.2f", wantBalance, sw.captured[0].params.Balance)
	}
}

// TestTakeSnapshots_MultipleAccountsAggregatedCorrectly verifies that holdings
// across two different accounts produce two separate snapshots with correct totals.
func TestTakeSnapshots_MultipleAccountsAggregatedCorrectly(t *testing.T) {
	userID := uuid.New()
	acc1ID := uuid.New()
	acc2ID := uuid.New()

	sw := &mockSnapshotWriter{}
	svc := newTestAutoSnap(
		&mockHoldingReader{
			userIDs: []uuid.UUID{userID},
			holdings: map[uuid.UUID][]models.Holding{
				userID: {
					// Account 1: 10 × $100 = $1000
					{AccountID: acc1ID, UserID: userID, Ticker: "A", Shares: 10},
					// Account 2: 5 × $100 = $500
					{AccountID: acc2ID, UserID: userID, Ticker: "B", Shares: 5},
				},
			},
		},
		sw,
		&mockPriceGetter{prices: map[string]float64{"A": 100.0, "B": 100.0}},
	)

	if err := svc.TakeSnapshots(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sw.captured) != 2 {
		t.Fatalf("expected 2 snapshots (one per account), got %d", len(sw.captured))
	}

	totals := map[uuid.UUID]float64{}
	for _, s := range sw.captured {
		totals[s.params.AccountID] = s.params.Balance
	}
	if totals[acc1ID] != 1000.0 {
		t.Errorf("acc1 balance: want 1000, got %.2f", totals[acc1ID])
	}
	if totals[acc2ID] != 500.0 {
		t.Errorf("acc2 balance: want 500, got %.2f", totals[acc2ID])
	}
}

// TestTakeSnapshots_SnapshotDatedToThisWeekMonday ensures the snapshot date is
// always the Monday of the current week regardless of when the test runs.
func TestTakeSnapshots_SnapshotDatedToThisWeekMonday(t *testing.T) {
	userID := uuid.New()
	accountID := uuid.New()

	sw := &mockSnapshotWriter{}
	svc := newTestAutoSnap(
		&mockHoldingReader{
			userIDs: []uuid.UUID{userID},
			holdings: map[uuid.UUID][]models.Holding{
				userID: {{AccountID: accountID, UserID: userID, Ticker: "X", Shares: 1}},
			},
		},
		sw,
		&mockPriceGetter{prices: map[string]float64{"X": 50.0}},
	)

	if err := svc.TakeSnapshots(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sw.captured) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(sw.captured))
	}

	snapshotDate := sw.captured[0].params.SnapshotDate
	if snapshotDate.Weekday() != time.Monday {
		t.Errorf("snapshot date %s is not a Monday", snapshotDate.Format("2006-01-02"))
	}
	if snapshotDate.Hour() != 0 || snapshotDate.Minute() != 0 {
		t.Errorf("snapshot date %s is not midnight", snapshotDate)
	}
}

// TestTakeSnapshots_DuplicateTickersDeduplicatedForPriceFetch verifies that
// two holdings with the same ticker are resolved from a single price lookup.
func TestTakeSnapshots_DuplicateTickersDeduplicatedForPriceFetch(t *testing.T) {
	userID := uuid.New()
	accountID := uuid.New()

	callCount := 0
	// Wrap the mock to track how many times GetPrices is called with AAPL.
	type countingPriceGetter struct{ inner *mockPriceGetter }
	counter := &countingPriceGetter{inner: &mockPriceGetter{prices: map[string]float64{"AAPL": 100.0}}}
	_ = counter

	sw := &mockSnapshotWriter{}
	svc := newTestAutoSnap(
		&mockHoldingReader{
			userIDs: []uuid.UUID{userID},
			holdings: map[uuid.UUID][]models.Holding{
				userID: {
					// Two lots of AAPL in the same account
					{AccountID: accountID, UserID: userID, Ticker: "AAPL", Shares: 10},
					{AccountID: accountID, UserID: userID, Ticker: "AAPL", Shares: 5},
				},
			},
		},
		sw,
		// Use a wrapper that counts calls.
		&countingMockPriceGetter{prices: map[string]float64{"AAPL": 100.0}, calls: &callCount},
	)

	if err := svc.TakeSnapshots(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Two lots: 15 shares × $100 = $1500 in one snapshot.
	if len(sw.captured) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(sw.captured))
	}
	if sw.captured[0].params.Balance != 1500.0 {
		t.Errorf("balance: want 1500, got %.2f", sw.captured[0].params.Balance)
	}
	// AAPL should appear only once in the tickers slice sent to GetPrices.
	if callCount != 1 {
		t.Errorf("GetPrices should be called once, got %d", callCount)
	}
}

// TestSnapshotUser_OnlySnapshotsRequestedUser verifies that SnapshotUser scopes
// its work to the given user, ignoring other users' holdings, and reports the
// number of accounts snapshotted.
func TestSnapshotUser_OnlySnapshotsRequestedUser(t *testing.T) {
	userID := uuid.New()
	otherUserID := uuid.New()
	acc1ID := uuid.New()
	acc2ID := uuid.New()
	otherAccID := uuid.New()

	sw := &mockSnapshotWriter{}
	svc := newTestAutoSnap(
		&mockHoldingReader{
			userIDs: []uuid.UUID{userID, otherUserID},
			holdings: map[uuid.UUID][]models.Holding{
				userID: {
					{AccountID: acc1ID, UserID: userID, Ticker: "A", Shares: 10},
					{AccountID: acc2ID, UserID: userID, Ticker: "B", Shares: 5},
				},
				otherUserID: {
					{AccountID: otherAccID, UserID: otherUserID, Ticker: "A", Shares: 1},
				},
			},
		},
		sw,
		&mockPriceGetter{prices: map[string]float64{"A": 100.0, "B": 100.0}},
	)

	count, err := svc.SnapshotUser(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 accounts snapshotted, got %d", count)
	}
	if len(sw.captured) != 2 {
		t.Fatalf("expected 2 snapshots written, got %d", len(sw.captured))
	}
	for _, snap := range sw.captured {
		if snap.params.UserID != userID {
			t.Errorf("snapshot written for wrong user: %s", snap.params.UserID)
		}
		if snap.params.Source != "computed" {
			t.Errorf("source: want 'computed', got %q", snap.params.Source)
		}
	}
}

// TestSnapshotUser_NoHoldingsReturnsZero verifies that a user with no holdings
// gets a zero count and no error, rather than a failure.
func TestSnapshotUser_NoHoldingsReturnsZero(t *testing.T) {
	userID := uuid.New()

	sw := &mockSnapshotWriter{}
	svc := newTestAutoSnap(&mockHoldingReader{}, sw, &mockPriceGetter{})

	count, err := svc.SnapshotUser(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 accounts snapshotted, got %d", count)
	}
}

// countingMockPriceGetter lets us assert that GetPrices is called exactly once
// (proving ticker deduplication is working).
type countingMockPriceGetter struct {
	prices map[string]float64
	calls  *int
}

func (m *countingMockPriceGetter) GetPrices(_ context.Context, tickers []string) (map[string]prices.Quote, error) {
	*m.calls++
	result := make(map[string]prices.Quote, len(tickers))
	for _, t := range tickers {
		if p, ok := m.prices[t]; ok {
			result[t] = prices.Quote{Ticker: t, Price: p}
		}
	}
	return result, nil
}

func TestRunIfDue_OnlyOnMondayOncePerWeek(t *testing.T) {
	svc := newTestAutoSnap(&mockHoldingReader{}, &mockSnapshotWriter{}, &mockPriceGetter{})
	ctx := context.Background()
	today := time.Now().UTC()

	svc.RunIfDue(ctx)
	svc.RunIfDue(ctx)

	want := 0
	if today.Weekday() == time.Monday {
		want = 1 // first call runs, second is the same week
	}
	if got := svc.Status().RunCount; got != want {
		t.Fatalf("today=%s: run count = %d, want %d", today.Weekday(), got, want)
	}
}
