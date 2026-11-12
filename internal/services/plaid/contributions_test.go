package plaid

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/crypto"
	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

func TestClassifyContribution(t *testing.T) {
	cases := []struct {
		name        string
		txn         InvestmentTransaction
		accountType string
		wantSource  string
		wantOK      bool
	}{
		{"payroll deferral", InvestmentTransaction{Type: "buy", Subtype: "contribution", Name: "EMPLOYEE DEFERRAL"}, "traditional_401k", "employee", true},
		{"employer match", InvestmentTransaction{Type: "buy", Subtype: "contribution", Name: "Employer Match"}, "traditional_401k", "employer", true},
		{"safe harbor", InvestmentTransaction{Type: "cash", Subtype: "contribution", Name: "SAFE HARBOR NONELECTIVE"}, "roth_401k", "employer", true},
		{"IRA deposit", InvestmentTransaction{Type: "cash", Subtype: "deposit", Name: "ELECTRONIC FUNDS TRANSFER RECEIVED"}, "roth_ira", "", false},
		{"IRA deposit, plain", InvestmentTransaction{Type: "cash", Subtype: "deposit", Name: "Deposit"}, "roth_ira", "employee", true},
		{"401k deposit isn't a contribution", InvestmentTransaction{Type: "cash", Subtype: "deposit", Name: "Deposit"}, "traditional_401k", "", false},
		{"rollover", InvestmentTransaction{Type: "cash", Subtype: "contribution", Name: "ROLLOVER CONTRIBUTION"}, "traditional_ira", "", false},
		{"Roth conversion", InvestmentTransaction{Type: "cash", Subtype: "deposit", Name: "Roth Conversion"}, "roth_ira", "", false},
		{"fund purchase", InvestmentTransaction{Type: "buy", Subtype: "buy", Name: "BUY VANGUARD TARGET 2055"}, "traditional_401k", "", false},
		{"dividend", InvestmentTransaction{Type: "cash", Subtype: "dividend", Name: "DIVIDEND"}, "traditional_ira", "", false},
	}
	for _, tc := range cases {
		source, ok := classifyContribution(tc.txn, tc.accountType)
		if source != tc.wantSource || ok != tc.wantOK {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, source, ok, tc.wantSource, tc.wantOK)
		}
	}
}

type fakeContribEntryStore struct{ upserted []db.ContributionEntryParams }

func (f *fakeContribEntryStore) UpsertSynced(_ context.Context, entries []db.ContributionEntryParams) error {
	f.upserted = append(f.upserted, entries...)
	return nil
}

func TestSyncItemRecordsRetirementContributions(t *testing.T) {
	cipher, _ := crypto.NewCipher(testHexKey)
	userID := uuid.New()
	k401ID, brokerageID := uuid.New(), uuid.New()
	k401Ext, brokerageExt := "p-401k", "p-brokerage"
	linked := []models.Account{
		{ID: k401ID, UserID: userID, Name: "401k", AccountType: "traditional_401k", Currency: "USD", ExternalAccountID: &k401Ext},
		{ID: brokerageID, UserID: userID, Name: "Brokerage", AccountType: "brokerage", Currency: "USD", ExternalAccountID: &brokerageExt},
	}
	bal := 1000.0
	day := time.Date(2026, 3, 13, 0, 0, 0, 0, time.UTC)
	api := &fakeAPI{
		accounts: []Account{
			{AccountID: "p-401k", Type: "investment", Subtype: "401k", CurrentBalance: &bal, Currency: "USD"},
			{AccountID: "p-brokerage", Type: "investment", Subtype: "brokerage", CurrentBalance: &bal, Currency: "USD"},
		},
		holdings:   []Holding{{AccountID: "p-401k", SecurityID: "s", Quantity: 1, InstitutionPrice: 1000, InstitutionValue: 1000}},
		securities: []Security{{SecurityID: "s", Ticker: "VFIAX", Type: "mutual fund"}},
		txns: []InvestmentTransaction{
			{ID: "t1", AccountID: "p-401k", Date: day, Type: "buy", Subtype: "contribution", Name: "Employee Deferral", Amount: 500},
			{ID: "t2", AccountID: "p-401k", Date: day, Type: "buy", Subtype: "contribution", Name: "Employer Match", Amount: 250},
			{ID: "t3", AccountID: "p-401k", Date: day, Type: "buy", Subtype: "buy", Name: "BUY", Amount: 750},
			{ID: "t4", AccountID: "p-brokerage", Date: day, Type: "cash", Subtype: "contribution", Name: "Deposit", Amount: -100},
		},
	}
	snapshots := &fakeSnapshotStore{latest: map[uuid.UUID]*models.BalanceSnapshot{
		k401ID:      {SnapshotDate: time.Now().UTC(), Source: "plaid"},
		brokerageID: {SnapshotDate: time.Now().UTC(), Source: "plaid"},
	}}
	entries := &fakeContribEntryStore{}
	svc := &Service{
		clientFor: fixedAPI(api), cipher: cipher, items: &fakeItemStore{},
		accounts: &fakeAccountStore{linked: linked}, holdings: &fakeHoldingStore{}, snapshots: snapshots,
		candles: fakeCandles{}, contribEntries: entries,
	}

	item := models.PlaidItem{ID: uuid.New(), UserID: userID, ItemID: "item-1", AccessTokenEncrypted: encryptedToken(t, cipher)}
	if _, err := svc.SyncItem(context.Background(), item); err != nil {
		t.Fatalf("SyncItem: %v", err)
	}

	// The fund purchase isn't a contribution, and the brokerage isn't a
	// retirement account.
	if len(entries.upserted) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries.upserted), entries.upserted)
	}
	got := map[string]db.ContributionEntryParams{}
	for _, e := range entries.upserted {
		got[*e.ExternalID] = e
	}
	if e := got["t1"]; e.Source != "employee" || e.Amount != 500 || e.AccountID != k401ID || e.UserID != userID {
		t.Errorf("deferral = %+v", e)
	}
	if e := got["t2"]; e.Source != "employer" || e.Amount != 250 {
		t.Errorf("match = %+v", e)
	}
}
