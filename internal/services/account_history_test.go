package services

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

var (
	jan = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	feb = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
)

func acct(name, typ string) models.Account {
	return models.Account{ID: uuid.New(), Name: name, AccountType: typ}
}

func row(b time.Time, a models.Account, bal float64) db.AccountBucketBalance {
	return db.AccountBucketBalance{Bucket: b, AccountID: a.ID, Balance: bal}
}

func noLoans(*models.HomeDetails, time.Time) float64 { return 0 }

// total adds a point's balances: net worth at that date.
func total(p AccountHistoryPoint) float64 {
	sum := 0.0
	for _, v := range p.Balances {
		sum += v
	}
	return sum
}

func TestBuildAccountHistory_OneSeriesPerAccountSummingToNetWorth(t *testing.T) {
	savings, card := acct("Savings", "savings"), acct("Visa", "credit_card")
	h := buildAccountHistory(
		[]db.AccountBucketBalance{row(jan, savings, 1000), row(jan, card, -250), row(feb, savings, 1200), row(feb, card, -100)},
		[]models.Account{savings, card}, nil, noLoans,
	)

	if len(h.Series) != 2 || h.Series[0].Name != "Savings" || h.Series[1].Name != "Visa" {
		t.Fatalf("series = %+v, want Savings then Visa", h.Series)
	}
	if len(h.Points) != 2 || h.Points[0].Date != jan || h.Points[1].Date != feb {
		t.Fatalf("points = %+v, want Jan then Feb", h.Points)
	}
	if got := total(h.Points[0]); got != 750 {
		t.Errorf("Jan net worth = %v, want 750", got)
	}
	if got := total(h.Points[1]); got != 1100 {
		t.Errorf("Feb net worth = %v, want 1100", got)
	}
	if h.Points[0].Balances[card.ID.String()] != -250 {
		t.Errorf("a liability should stay negative, got %v", h.Points[0].Balances[card.ID.String()])
	}
}

func TestBuildAccountHistory_FoldsDeletedAccountsIntoOneSeries(t *testing.T) {
	live := acct("Checking", "checking")
	gone1, gone2 := acct("Old A", "savings"), acct("Old B", "savings")
	// Only `live` still exists.
	h := buildAccountHistory(
		[]db.AccountBucketBalance{row(jan, live, 500), row(jan, gone1, 300), row(jan, gone2, 200)},
		[]models.Account{live}, nil, noLoans,
	)

	var closed *AccountSeries
	for i := range h.Series {
		if h.Series[i].ID == closedAccountsSeriesID {
			closed = &h.Series[i]
		}
	}
	if closed == nil || closed.Name != "Closed accounts" {
		t.Fatalf("expected a Closed accounts series, got %+v", h.Series)
	}
	if got := h.Points[0].Balances[closedAccountsSeriesID]; got != 500 {
		t.Errorf("closed accounts balance = %v, want 500 (300 + 200)", got)
	}
	if got := total(h.Points[0]); got != 1000 {
		t.Errorf("net worth = %v, want 1000: deleted accounts still count", got)
	}
}

func TestBuildAccountHistory_SplitsATrackedHomeIntoPropertyAndMortgage(t *testing.T) {
	home := acct("House", "home")
	hd := &models.HomeDetails{AccountID: home.ID}
	// The snapshot stores equity (100k); the loan reconstructs to 300k at Jan, 290k at Feb.
	loan := func(_ *models.HomeDetails, at time.Time) float64 {
		if at.Equal(jan) {
			return 300000
		}
		return 290000
	}
	h := buildAccountHistory(
		[]db.AccountBucketBalance{row(jan, home, 100000), row(feb, home, 110000)},
		[]models.Account{home}, map[uuid.UUID]*models.HomeDetails{home.ID: hd}, loan,
	)

	mortgageID := mortgageSeriesID(home.ID).String()
	if len(h.Series) != 2 {
		t.Fatalf("series = %+v, want the home and its mortgage", h.Series)
	}
	jPoint, fPoint := h.Points[0], h.Points[1]
	if jPoint.Balances[home.ID.String()] != 400000 || jPoint.Balances[mortgageID] != -300000 {
		t.Errorf("Jan = %+v, want property 400000 and mortgage -300000", jPoint.Balances)
	}
	if fPoint.Balances[home.ID.String()] != 400000 || fPoint.Balances[mortgageID] != -290000 {
		t.Errorf("Feb = %+v, want property 400000 and mortgage -290000", fPoint.Balances)
	}
	// The split must not change net worth: it is still the stored equity.
	if total(jPoint) != 100000 || total(fPoint) != 110000 {
		t.Errorf("net worth = %v, %v; want 100000, 110000", total(jPoint), total(fPoint))
	}
	for _, s := range h.Series {
		if s.ID == mortgageID && s.Name != "House (Mortgage)" {
			t.Errorf("mortgage series name = %q", s.Name)
		}
	}
}

func TestBuildAccountHistory_OnlyListsSeriesThatAppearAndIsStable(t *testing.T) {
	a, b, unused := acct("Brokerage", "brokerage"), acct("Checking", "checking"), acct("Never used", "savings")
	rows := []db.AccountBucketBalance{row(jan, b, 1), row(jan, a, 2)}
	first := buildAccountHistory(rows, []models.Account{a, b, unused}, nil, noLoans)
	second := buildAccountHistory(rows, []models.Account{unused, b, a}, nil, noLoans)

	if len(first.Series) != 2 {
		t.Fatalf("series = %+v, want only the two with data", first.Series)
	}
	for i := range first.Series {
		if first.Series[i] != second.Series[i] {
			t.Errorf("series order changed between calls: %+v vs %+v", first.Series, second.Series)
		}
	}
}

func TestBuildAccountHistory_EmptyIsEmptyNotNull(t *testing.T) {
	h := buildAccountHistory(nil, nil, nil, noLoans)
	if h.Series == nil || h.Points == nil {
		t.Errorf("empty history must serialize as [] not null: %+v", h)
	}
}

func TestMortgageSeriesID_MatchesNetWorthsSyntheticID(t *testing.T) {
	id := uuid.MustParse("11111111-2222-3333-4444-555555555554")
	got := mortgageSeriesID(id)
	if got == id || got[15] != id[15]^0x01 {
		t.Errorf("mortgageSeriesID = %v, want the last byte flipped", got)
	}
	if mortgageSeriesID(got) != id {
		t.Error("flipping twice should return the original id")
	}
}
