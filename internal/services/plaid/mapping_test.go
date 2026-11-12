package plaid

import "testing"

func TestSubtypeToAccountType(t *testing.T) {
	tests := []struct {
		plaidType, subtype string
		wantType, wantTax  string
	}{
		{"investment", "401k", "traditional_401k", "tax_deferred"},
		{"investment", "roth 401k", "roth_401k", "tax_free"},
		{"investment", "roth", "roth_ira", "tax_free"},
		{"investment", "ira", "traditional_ira", "tax_deferred"},
		{"investment", "hsa", "hsa", "tax_free"},
		{"investment", "529", "529", "tax_free"},
		{"investment", "brokerage", "brokerage", "taxable"},
		{"investment", "something new", "brokerage", "taxable"}, // investment fallback
		{"depository", "checking", "checking", "taxable"},
		{"depository", "savings", "savings", "taxable"},
		{"depository", "money market", "money_market", "taxable"},
		{"depository", "cd", "savings", "taxable"},
		{"credit", "credit card", "credit_card", "taxable"},
		{"loan", "mortgage", "home", "taxable"},
		{"loan", "home equity", "home", "taxable"},
		{"loan", "auto", "loan", "taxable"},
		{"credit", "", "credit_card", "taxable"}, // type fallback
		{"other", "", "other_asset", "taxable"},
	}
	for _, tc := range tests {
		gotType, gotTax := SubtypeToAccountType(tc.plaidType, tc.subtype)
		if gotType != tc.wantType || gotTax != tc.wantTax {
			t.Errorf("SubtypeToAccountType(%q, %q) = (%q, %q), want (%q, %q)",
				tc.plaidType, tc.subtype, gotType, gotTax, tc.wantType, tc.wantTax)
		}
	}
}

func TestSecurityTypeToAssetClass(t *testing.T) {
	tests := map[string]string{
		"equity":         "equity",
		"etf":            "equity",
		"mutual fund":    "equity",
		"fixed income":   "bond",
		"cash":           "cash_equivalent",
		"cryptocurrency": "crypto",
		"derivative":     "other",
		"loan":           "other",
		"":               "other",
	}
	for in, want := range tests {
		if got := SecurityTypeToAssetClass(in); got != want {
			t.Errorf("SecurityTypeToAssetClass(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsLiability(t *testing.T) {
	for plaidType, want := range map[string]bool{
		"credit": true, "loan": true,
		"depository": false, "investment": false, "brokerage": false, "other": false,
	} {
		if got := IsLiability(plaidType); got != want {
			t.Errorf("IsLiability(%q) = %v, want %v", plaidType, got, want)
		}
	}
}

func TestResolveTicker(t *testing.T) {
	tests := []struct {
		name       string
		sec        Security
		wantTicker string
		wantManual bool
	}{
		{"symbol", Security{Ticker: "vti"}, "VTI", false},
		{"symbol with stars", Security{Ticker: "VMFXX**"}, "VMFXX", false},
		{"cusip fallback", Security{CUSIP: "922908769"}, "922908769", true},
		{"security id fallback", Security{SecurityID: "abcdef1234567890"}, "PLAID-ABCDEF12", true},
		{"short security id", Security{SecurityID: "ab12"}, "PLAID-AB12", true},
	}
	for _, tc := range tests {
		ticker, manual := ResolveTicker(tc.sec)
		if ticker != tc.wantTicker || manual != tc.wantManual {
			t.Errorf("%s: ResolveTicker = (%q, %v), want (%q, %v)", tc.name, ticker, manual, tc.wantTicker, tc.wantManual)
		}
	}
}
