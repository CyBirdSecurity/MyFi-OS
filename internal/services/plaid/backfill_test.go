package plaid

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestApplyReverse(t *testing.T) {
	qty := map[string]float64{"sec1": 15}
	cash := 100.0

	// Undo a buy of 5 shares that cost $500 (Plaid: positive amount = cash out).
	applyReverse(InvestmentTransaction{SecurityID: "sec1", Quantity: 5, Amount: 500}, qty, &cash)
	if qty["sec1"] != 10 {
		t.Errorf("after undoing buy: qty = %v, want 10", qty["sec1"])
	}
	if cash != 600 {
		t.Errorf("after undoing buy: cash = %v, want 600", cash)
	}

	// Undo a sell of 3 shares that credited $300 (negative amount = cash in).
	applyReverse(InvestmentTransaction{SecurityID: "sec1", Quantity: -3, Amount: -300}, qty, &cash)
	if qty["sec1"] != 13 {
		t.Errorf("after undoing sell: qty = %v, want 13", qty["sec1"])
	}
	if cash != 300 {
		t.Errorf("after undoing sell: cash = %v, want 300", cash)
	}

	// Undo a cash-only dividend of $50 (no security, negative amount = cash in).
	applyReverse(InvestmentTransaction{Amount: -50}, qty, &cash)
	if cash != 250 {
		t.Errorf("after undoing dividend: cash = %v, want 250", cash)
	}
}

func TestReconstructWeekly(t *testing.T) {
	// Fixed "now": Wednesday 2026-06-17 → current week Monday is 2026-06-15.
	now := date(2026, time.June, 17)
	start := now.AddDate(0, -1, 0) // one month of history

	acc := models.Account{ID: uuid.New(), UserID: uuid.New(), Name: "Brokerage"}
	const plaidAcc = "plaid-acc-1"

	secByID := map[string]Security{
		"sec-aapl": {SecurityID: "sec-aapl", Ticker: "AAPL", Type: "equity"},
		"sec-cash": {SecurityID: "sec-cash", Ticker: "CUR:USD", Type: "cash"},
	}

	// Today: 10 AAPL + $1000 cash.
	holdings := []Holding{
		{AccountID: plaidAcc, SecurityID: "sec-aapl", Quantity: 10, InstitutionPrice: 100, InstitutionValue: 1000},
		{AccountID: plaidAcc, SecurityID: "sec-cash", Quantity: 1000, InstitutionPrice: 1, InstitutionValue: 1000},
	}

	// One buy on 2026-06-10 (Wednesday of the prior week): bought 4 AAPL for $380.
	txns := []InvestmentTransaction{
		{AccountID: plaidAcc, SecurityID: "sec-aapl", Date: date(2026, time.June, 10), Type: "buy", Quantity: 4, Amount: 380},
	}

	// Weekly closes for every Monday in range.
	closes := map[string]map[time.Time]float64{"AAPL": {}}
	for monday := thisWeekMonday(start); !monday.After(now); monday = monday.AddDate(0, 0, 7) {
		closes["AAPL"][monday] = 100
	}

	snaps := reconstructWeekly(acc, plaidAcc, holdings, txns, secByID, closes, start, now, true)

	// Weeks span from the buy's week (2026-06-08) — earliest txn bounds history —
	// through the current week (2026-06-15): exactly 2 snapshots.
	if len(snaps) != 2 {
		t.Fatalf("got %d snapshots, want 2: %+v", len(snaps), snaps)
	}
	// snaps are emitted newest-first.
	newest, older := snaps[0], snaps[1]

	if !newest.SnapshotDate.Equal(date(2026, time.June, 15)) {
		t.Errorf("newest snapshot date = %v, want 2026-06-15", newest.SnapshotDate)
	}
	// Current week: 10 AAPL × $100 + $1000 cash = $2000.
	if math.Abs(newest.Balance-2000) > 0.01 {
		t.Errorf("newest balance = %v, want 2000", newest.Balance)
	}

	if !older.SnapshotDate.Equal(date(2026, time.June, 8)) {
		t.Errorf("older snapshot date = %v, want 2026-06-08", older.SnapshotDate)
	}
	// Before the buy: 6 AAPL × $100 + ($1000 + $380 not yet spent) = $1980.
	if math.Abs(older.Balance-1980) > 0.01 {
		t.Errorf("older balance = %v, want 1980", older.Balance)
	}

	for _, s := range snaps {
		if s.Source != "plaid" {
			t.Errorf("snapshot source = %q, want plaid", s.Source)
		}
		if s.AccountID != acc.ID || s.UserID != acc.UserID {
			t.Errorf("snapshot has wrong account/user IDs")
		}
	}
}

// TestReconstructWeeklyFallbackPrice covers securities with no candle history:
// they are valued at the current institution price.
func TestReconstructWeeklyFallbackPrice(t *testing.T) {
	now := date(2026, time.June, 17)
	start := now.AddDate(0, -1, 0)

	acc := models.Account{ID: uuid.New(), UserID: uuid.New()}
	const plaidAcc = "p1"

	secByID := map[string]Security{
		"sec-x": {SecurityID: "sec-x", Name: "Private Fund", Type: "equity"}, // no ticker → manual price
	}
	holdings := []Holding{
		{AccountID: plaidAcc, SecurityID: "sec-x", Quantity: 2, InstitutionPrice: 50, InstitutionValue: 100},
	}

	snaps := reconstructWeekly(acc, plaidAcc, holdings, nil, secByID, map[string]map[time.Time]float64{}, start, now, true)

	// No transactions → history is bounded to the current week only.
	if len(snaps) != 1 {
		t.Fatalf("got %d snapshots, want 1", len(snaps))
	}
	if math.Abs(snaps[0].Balance-100) > 0.01 {
		t.Errorf("balance = %v, want 100 (2 × institution price 50)", snaps[0].Balance)
	}
}

func TestThisWeekMonday(t *testing.T) {
	tests := []struct{ in, want time.Time }{
		{date(2026, time.June, 15), date(2026, time.June, 15)}, // Monday
		{date(2026, time.June, 17), date(2026, time.June, 15)}, // Wednesday
		{date(2026, time.June, 21), date(2026, time.June, 15)}, // Sunday → prior Monday
	}
	for _, tc := range tests {
		if got := thisWeekMonday(tc.in); !got.Equal(tc.want) {
			t.Errorf("thisWeekMonday(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
