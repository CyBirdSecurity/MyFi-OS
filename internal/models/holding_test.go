package models

import "testing"

func TestHoldingPriceTicker(t *testing.T) {
	cases := []struct {
		name        string
		ticker      string
		accountType string
		want        string
	}{
		{"coin in crypto account", "BTC", "crypto", "BTC-USD"},
		{"coin already quoted in a currency", "BTC-USD", "crypto", "BTC-USD"},
		{"crypto ETF in brokerage stays as entered", "FBTC", "brokerage", "FBTC"},
		{"bare BTC in brokerage is the ETF", "BTC", "brokerage", "BTC"},
		{"account type unknown", "AAPL", "", "AAPL"},
	}
	for _, tc := range cases {
		h := Holding{Ticker: tc.ticker, AccountType: tc.accountType}
		if got := h.PriceTicker(); got != tc.want {
			t.Errorf("%s: PriceTicker() = %q, want %q", tc.name, got, tc.want)
		}
	}
}
