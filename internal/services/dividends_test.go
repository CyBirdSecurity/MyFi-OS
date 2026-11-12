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

// ---- Mock implementations for the AutoDividendService interfaces ----

type mockDividendHoldingReader struct {
	userIDs  []uuid.UUID
	holdings map[uuid.UUID][]models.Holding
}

func (m *mockDividendHoldingReader) ListUsersWithHoldings(_ context.Context) ([]uuid.UUID, error) {
	return m.userIDs, nil
}

func (m *mockDividendHoldingReader) ListByUser(_ context.Context, userID uuid.UUID) ([]models.Holding, error) {
	return m.holdings[userID], nil
}

type addSharesCall struct {
	id    uuid.UUID
	delta float64
}

type mockHoldingShareStore struct {
	calls []addSharesCall
}

func (m *mockHoldingShareStore) AddShares(_ context.Context, id uuid.UUID, delta float64) error {
	m.calls = append(m.calls, addSharesCall{id: id, delta: delta})
	return nil
}

type mockDividendRecorder struct {
	lastApplied map[uuid.UUID]*time.Time
	created     []db.CreateDividendParams
	// existing simulates rows that already exist (holdingID+exDate), so
	// Create returns (nil, nil) for them — exercising the idempotency path.
	existing map[uuid.UUID]map[time.Time]bool
}

func (m *mockDividendRecorder) LastAppliedDate(_ context.Context, holdingID uuid.UUID) (*time.Time, error) {
	return m.lastApplied[holdingID], nil
}

func (m *mockDividendRecorder) Create(_ context.Context, p db.CreateDividendParams) (*models.HoldingDividend, error) {
	if m.existing != nil && m.existing[p.HoldingID] != nil && m.existing[p.HoldingID][p.ExDate] {
		return nil, nil
	}
	m.created = append(m.created, p)
	return &models.HoldingDividend{HoldingID: p.HoldingID, ExDate: p.ExDate}, nil
}

type mockDividendGetter struct {
	events map[string][]prices.DividendEvent
	ok     bool
}

func (m *mockDividendGetter) GetDividends(_ context.Context, ticker string, _, _ time.Time) ([]prices.DividendEvent, bool, error) {
	return m.events[ticker], m.ok, nil
}

func newTestAutoDividend(
	h dividendHoldingReader,
	shares holdingShareStore,
	d dividendRecorder,
	p dividendGetter,
) *AutoDividendService {
	return &AutoDividendService{holdings: h, holdingShares: shares, dividends: d, prices: p}
}

func floatPtr(f float64) *float64 { return &f }

// ---- ProcessDividends / applyDividend tests ----

// TestProcessDividends_DripEnabledAddsShares verifies that when a holding has
// DRIP enabled and a live price, a paid dividend adds shares proportional to
// the dividend amount divided by price.
func TestProcessDividends_DripEnabledAddsShares(t *testing.T) {
	userID := uuid.New()
	holdingID := uuid.New()
	exDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)

	holding := models.Holding{
		ID:            holdingID,
		UserID:        userID,
		Ticker:        "SCHD",
		Shares:        100,
		DripEnabled:   true,
		PricePerShare: floatPtr(25.0),
	}

	shares := &mockHoldingShareStore{}
	recorder := &mockDividendRecorder{lastApplied: map[uuid.UUID]*time.Time{}}
	getter := &mockDividendGetter{
		ok: true,
		events: map[string][]prices.DividendEvent{
			"SCHD": {{Ticker: "SCHD", ExDate: exDate, AmountPerShare: 0.20}},
		},
	}

	svc := newTestAutoDividend(
		&mockDividendHoldingReader{
			userIDs:  []uuid.UUID{userID},
			holdings: map[uuid.UUID][]models.Holding{userID: {holding}},
		},
		shares, recorder, getter,
	)

	windowStart := exDate.AddDate(0, -1, 0)
	if err := svc.ProcessDividends(context.Background(), windowStart); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recorder.created) != 1 {
		t.Fatalf("expected 1 dividend record, got %d", len(recorder.created))
	}
	rec := recorder.created[0]
	wantAmount := 100.0 * 0.20 // 20.00
	if rec.DividendAmount != wantAmount {
		t.Errorf("dividend amount: want %.4f, got %.4f", wantAmount, rec.DividendAmount)
	}
	if !rec.DripApplied {
		t.Errorf("expected DripApplied=true")
	}

	if len(shares.calls) != 1 {
		t.Fatalf("expected 1 AddShares call, got %d", len(shares.calls))
	}
	wantDelta := wantAmount / 25.0 // 0.8 shares
	if shares.calls[0].id != holdingID || shares.calls[0].delta != wantDelta {
		t.Errorf("AddShares: want (%s, %.4f), got (%s, %.4f)",
			holdingID, wantDelta, shares.calls[0].id, shares.calls[0].delta)
	}
}

// TestProcessDividends_DripDisabledRecordsButNoShares verifies that a
// dividend is still recorded for history when DRIP is off, but no shares are
// added to the holding.
func TestProcessDividends_DripDisabledRecordsButNoShares(t *testing.T) {
	userID := uuid.New()
	holdingID := uuid.New()
	exDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)

	holding := models.Holding{
		ID:            holdingID,
		UserID:        userID,
		Ticker:        "SCHD",
		Shares:        100,
		DripEnabled:   false,
		PricePerShare: floatPtr(25.0),
	}

	shares := &mockHoldingShareStore{}
	recorder := &mockDividendRecorder{lastApplied: map[uuid.UUID]*time.Time{}}
	getter := &mockDividendGetter{
		ok: true,
		events: map[string][]prices.DividendEvent{
			"SCHD": {{Ticker: "SCHD", ExDate: exDate, AmountPerShare: 0.20}},
		},
	}

	svc := newTestAutoDividend(
		&mockDividendHoldingReader{
			userIDs:  []uuid.UUID{userID},
			holdings: map[uuid.UUID][]models.Holding{userID: {holding}},
		},
		shares, recorder, getter,
	)

	windowStart := exDate.AddDate(0, -1, 0)
	if err := svc.ProcessDividends(context.Background(), windowStart); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recorder.created) != 1 {
		t.Fatalf("expected 1 dividend record, got %d", len(recorder.created))
	}
	if recorder.created[0].DripApplied {
		t.Errorf("expected DripApplied=false")
	}
	if len(shares.calls) != 0 {
		t.Errorf("expected no AddShares calls, got %d", len(shares.calls))
	}
}

// TestProcessDividends_AlreadyAppliedSkipsShareUpdate verifies idempotency:
// if the dividend record already exists (Create returns nil), shares are not
// added again even though DRIP is enabled.
func TestProcessDividends_AlreadyAppliedSkipsShareUpdate(t *testing.T) {
	userID := uuid.New()
	holdingID := uuid.New()
	exDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)

	holding := models.Holding{
		ID:            holdingID,
		UserID:        userID,
		Ticker:        "SCHD",
		Shares:        100,
		DripEnabled:   true,
		PricePerShare: floatPtr(25.0),
	}

	shares := &mockHoldingShareStore{}
	recorder := &mockDividendRecorder{
		lastApplied: map[uuid.UUID]*time.Time{},
		existing: map[uuid.UUID]map[time.Time]bool{
			holdingID: {exDate: true},
		},
	}
	getter := &mockDividendGetter{
		ok: true,
		events: map[string][]prices.DividendEvent{
			"SCHD": {{Ticker: "SCHD", ExDate: exDate, AmountPerShare: 0.20}},
		},
	}

	svc := newTestAutoDividend(
		&mockDividendHoldingReader{
			userIDs:  []uuid.UUID{userID},
			holdings: map[uuid.UUID][]models.Holding{userID: {holding}},
		},
		shares, recorder, getter,
	)

	windowStart := exDate.AddDate(0, -1, 0)
	if err := svc.ProcessDividends(context.Background(), windowStart); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recorder.created) != 0 {
		t.Errorf("expected no new dividend records (already applied), got %d", len(recorder.created))
	}
	if len(shares.calls) != 0 {
		t.Errorf("expected no AddShares calls for an already-applied dividend, got %d", len(shares.calls))
	}
}

// TestProcessDividends_NoBackfillOnFirstCheck verifies that when the
// service-wide window start equals "now" (the very first run), a holding
// with no prior dividend record is skipped entirely rather than fetching
// historical dividends.
func TestProcessDividends_NoBackfillOnFirstCheck(t *testing.T) {
	userID := uuid.New()
	holdingID := uuid.New()

	holding := models.Holding{
		ID:            holdingID,
		UserID:        userID,
		Ticker:        "SCHD",
		Shares:        100,
		DripEnabled:   true,
		PricePerShare: floatPtr(25.0),
	}

	shares := &mockHoldingShareStore{}
	recorder := &mockDividendRecorder{lastApplied: map[uuid.UUID]*time.Time{}}
	getter := &mockDividendGetter{
		ok: true,
		events: map[string][]prices.DividendEvent{
			// Even if the provider "has" a very old event, it must not be
			// fetched/applied since the window never opens on a first check.
			"SCHD": {{Ticker: "SCHD", ExDate: time.Now().AddDate(-5, 0, 0), AmountPerShare: 0.20}},
		},
	}

	svc := newTestAutoDividend(
		&mockDividendHoldingReader{
			userIDs:  []uuid.UUID{userID},
			holdings: map[uuid.UUID][]models.Holding{userID: {holding}},
		},
		shares, recorder, getter,
	)

	// windowStart == now simulates the service's very first run.
	now := time.Now().UTC()
	if err := svc.ProcessDividends(context.Background(), now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recorder.created) != 0 {
		t.Errorf("expected no dividend records on first check (no backfill), got %d", len(recorder.created))
	}
	if len(shares.calls) != 0 {
		t.Errorf("expected no AddShares calls on first check, got %d", len(shares.calls))
	}
}

// TestProcessDividends_NoPriceSkipsDripButStillRecords verifies that a
// holding with DRIP enabled but no known price still gets its dividend
// recorded (for history) without attempting a share addition.
func TestProcessDividends_NoPriceSkipsDripButStillRecords(t *testing.T) {
	userID := uuid.New()
	holdingID := uuid.New()
	exDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)

	holding := models.Holding{
		ID:          holdingID,
		UserID:      userID,
		Ticker:      "SCHD",
		Shares:      100,
		DripEnabled: true,
		// No PricePerShare set.
	}

	shares := &mockHoldingShareStore{}
	recorder := &mockDividendRecorder{lastApplied: map[uuid.UUID]*time.Time{}}
	getter := &mockDividendGetter{
		ok: true,
		events: map[string][]prices.DividendEvent{
			"SCHD": {{Ticker: "SCHD", ExDate: exDate, AmountPerShare: 0.20}},
		},
	}

	svc := newTestAutoDividend(
		&mockDividendHoldingReader{
			userIDs:  []uuid.UUID{userID},
			holdings: map[uuid.UUID][]models.Holding{userID: {holding}},
		},
		shares, recorder, getter,
	)

	windowStart := exDate.AddDate(0, -1, 0)
	if err := svc.ProcessDividends(context.Background(), windowStart); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recorder.created) != 1 {
		t.Fatalf("expected 1 dividend record, got %d", len(recorder.created))
	}
	if recorder.created[0].DripApplied {
		t.Errorf("expected DripApplied=false when no price is available")
	}
	if len(shares.calls) != 0 {
		t.Errorf("expected no AddShares calls when no price is available, got %d", len(shares.calls))
	}
}

// ---- nextDividendCheck tests ----

func TestNextDividendCheck_MidMonthReturnsLastDayOfMonth(t *testing.T) {
	// June 2024 has 30 days.
	mid := time.Date(2024, 6, 10, 12, 0, 0, 0, time.UTC)
	got := nextDividendCheck(mid)
	want := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("want %s, got %s", want, got)
	}
}

func TestNextDividendCheck_OnLastDayReturnsFirstMondayNextMonth(t *testing.T) {
	// 2024-06-30 is itself the last day of June; the next checkpoint should
	// be the first Monday of July 2024 (2024-07-01, which is a Monday).
	lastDay := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)
	got := nextDividendCheck(lastDay)
	want := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("want %s, got %s", want, got)
	}
	if got.Weekday() != time.Monday {
		t.Errorf("expected a Monday, got %s", got.Weekday())
	}
}

func TestNextDividendCheck_AfterFirstMondayRollsToNextMonth(t *testing.T) {
	// After both July checkpoints have passed, the next one should be the
	// last day of August (July's first-Monday safety net already passed).
	afterJulyMonday := time.Date(2024, 7, 2, 0, 0, 0, 0, time.UTC)
	got := nextDividendCheck(afterJulyMonday)
	want := time.Date(2024, 7, 31, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("want %s, got %s", want, got)
	}
}

func TestNextDividendCheck_AlwaysStrictlyAfterInput(t *testing.T) {
	cases := []time.Time{
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), // leap day
		time.Date(2024, 12, 31, 23, 59, 0, 0, time.UTC),
	}
	for _, tc := range cases {
		got := nextDividendCheck(tc)
		if !got.After(tc) {
			t.Errorf("input %s: result %s is not strictly after input", tc, got)
		}
	}
}
