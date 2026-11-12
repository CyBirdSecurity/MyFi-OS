package services

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// csvTestService builds a CSV importer whose classifier knows a few system
// categories and vendor rules, with no database or LLM behind it.
func csvTestService() (*CSVImportService, map[string]uuid.UUID) {
	ids := map[string]uuid.UUID{}
	var cats []models.BudgetCategory
	for _, n := range []string{"Groceries", "Utilities", "Income", "Transfer", "Other"} {
		id := uuid.New()
		ids[n] = id
		cats = append(cats, models.BudgetCategory{ID: id, Name: n, IsSystem: true})
	}
	rules := []models.VendorRule{
		{Keyword: "WHOLE FOODS", CategoryID: ids["Groceries"]},
		{Keyword: "CITY WATER", CategoryID: ids["Utilities"]},
	}
	c := &ClassifierService{categories: cats, rules: rules}
	return NewCSVImportService(c, nil), ids
}

func parseTxCSV(t *testing.T, csv string) ([]ParsedTransaction, map[string]uuid.UUID) {
	t.Helper()
	s, ids := csvTestService()
	got, err := s.ParseQuick(context.Background(), uuid.Nil, []byte(csv), "test.csv")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return got, ids
}

func byMerchant(rows []ParsedTransaction) map[string]ParsedTransaction {
	m := map[string]ParsedTransaction{}
	for _, r := range rows {
		m[r.Merchant] = r
	}
	return m
}

// Credit-card exports list charges as positive amounts; payments and refunds
// are negative and are not spending.
func TestParse_PositiveChargesFile(t *testing.T) {
	got, ids := parseTxCSV(t, `Date,Description,Amount
11/01/2026,WHOLE FOODS MARKET,84.12
11/02/2026,CITY WATER UTIL,62.10
11/03/2026,ONLINE PAYMENT THANK YOU,-500.00
`)
	if len(got) != 2 {
		t.Fatalf("got %d rows, want the 2 charges: %+v", len(got), got)
	}
	m := byMerchant(got)
	if r := m["WHOLE FOODS MARKET"]; r.Amount != 84.12 || r.CategoryID == nil || *r.CategoryID != ids["Groceries"] {
		t.Errorf("groceries row = %+v", r)
	}
}

// Checking-account exports list spending as negative amounts and deposits as
// positive ones. Every purchase must be imported, not just the deposits.
func TestParse_NegativeSpendingFile(t *testing.T) {
	got, ids := parseTxCSV(t, `Date,Description,Amount
11/01/2026,WHOLE FOODS MARKET,-84.12
11/02/2026,CITY WATER UTIL,-62.10
11/03/2026,MYSTERY SHOP 123,-23.00
11/04/2026,PAYROLL ACME INC,3200.00
`)
	if len(got) != 4 {
		t.Fatalf("got %d rows, want all 4: %+v", len(got), got)
	}
	m := byMerchant(got)
	if r := m["WHOLE FOODS MARKET"]; r.Amount != 84.12 || r.CategoryID == nil || *r.CategoryID != ids["Groceries"] {
		t.Errorf("spending is stored as a positive amount in its category; got %+v", r)
	}
	if r := m["MYSTERY SHOP 123"]; r.Amount != 23.00 || !r.NeedsReview {
		t.Errorf("an unknown merchant is imported and flagged for review; got %+v", r)
	}
	if r := m["PAYROLL ACME INC"]; r.Amount != 3200 || r.CategoryID == nil || *r.CategoryID != ids["Income"] || r.NeedsReview {
		t.Errorf("a deposit is imported as income; got %+v", r)
	}
}

// Some card exports (e.g. Chase) show purchases as negative and the card
// payment as positive, with a Type column. A card payment is a transfer, not
// income, so it must not be imported.
func TestParse_NegativeSpendingCardWithPaymentType(t *testing.T) {
	got, _ := parseTxCSV(t, `Transaction Date,Description,Type,Amount
11/01/2026,WHOLE FOODS MARKET,Sale,-84.12
11/02/2026,CITY WATER UTIL,Sale,-62.10
11/05/2026,AUTOMATIC PAYMENT - THANK,Payment,146.22
`)
	if len(got) != 2 {
		t.Fatalf("got %d rows, want the 2 purchases only: %+v", len(got), got)
	}
	for _, r := range got {
		if r.Amount <= 0 {
			t.Errorf("purchase stored with non-positive amount: %+v", r)
		}
	}
}

// With no majority, the long-standing credit-card reading (positive is
// spending) is kept.
func TestParse_EvenSplitKeepsPositiveAsSpending(t *testing.T) {
	got, _ := parseTxCSV(t, `Date,Description,Amount
11/01/2026,WHOLE FOODS MARKET,84.12
11/03/2026,ONLINE PAYMENT THANK YOU,-84.12
`)
	if len(got) != 1 || got[0].Merchant != "WHOLE FOODS MARKET" {
		t.Fatalf("got %+v, want only the positive charge", got)
	}
}
