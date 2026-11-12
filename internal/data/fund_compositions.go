package data

// FundComposition is a curated snapshot of one fund's sector weights, used to
// detect sector-overlap concentration risk that isn't visible from ticker
// symbols alone (e.g. holding both VTI and VOO, or QQQ and VGT, adds far less
// diversification than two different tickers might suggest).
//
// This is a small, manually curated table — not a live feed. MyFi's price
// provider (internal/prices/yahoo.go) uses Yahoo's v8/finance/chart endpoint,
// but fund composition data requires Yahoo's v10/finance/quoteSummary
// endpoint, which (confirmed as of this writing) requires a cookie/crumb
// handshake rather than a simple unauthenticated request. Rather than depend
// on that fragile, undocumented flow, sector weights are sourced directly
// from each issuer's own public fact sheet — see Source/AsOf per entry — and
// refreshed manually. Fund sector composition drifts slowly, so staleness on
// the order of months is an acceptable tradeoff.
type FundComposition struct {
	Name          string
	AsOf          string             // fact sheet date, "YYYY-MM-DD"
	Source        string             // issuer fact sheet URL
	SectorWeights map[string]float64 // GICS-ish sector name -> % of fund (0-100)
}

// FundCompositions covers ~15-20 of the most commonly held U.S.-listed ETFs
// across the largest issuers (Vanguard, State Street, iShares, Invesco,
// Schwab). Sector names are normalized to a common set across issuers, since
// fact sheets use inconsistent labels (e.g. Vanguard's ICB-based sheets say
// "Technology"/"Basic Materials"/"Telecommunications" where GICS-based sheets
// say "Information Technology"/"Materials"/"Communication Services").
var FundCompositions = map[string]FundComposition{
	"VTI": {
		Name: "Vanguard Total Stock Market ETF", AsOf: "2026-03-31",
		Source: "https://fund-docs.vanguard.com/F0970.pdf",
		SectorWeights: map[string]float64{
			"Information Technology": 36.3, "Consumer Discretionary": 13.6, "Industrials": 12.6,
			"Financials": 10.7, "Health Care": 9.7, "Energy": 4.3, "Consumer Staples": 3.7,
			"Utilities": 2.8, "Real Estate": 2.4, "Communication Services": 2.1, "Materials": 1.8,
		},
	},
	"VOO": {
		Name: "Vanguard S&P 500 ETF", AsOf: "2026-03-31",
		Source: "https://fund-docs.vanguard.com/F0968.pdf",
		SectorWeights: map[string]float64{
			"Information Technology": 32.9, "Financials": 12.6, "Communication Services": 10.3,
			"Consumer Discretionary": 9.9, "Health Care": 9.5, "Industrials": 9.0, "Consumer Staples": 5.3,
			"Energy": 4.0, "Utilities": 2.5, "Materials": 2.1, "Real Estate": 2.0,
		},
	},
	"IVV": {
		Name: "iShares Core S&P 500 ETF", AsOf: "2026-03-31",
		Source: "https://www.ishares.com/us/literature/fact-sheet/ivv-ishares-core-s-p-500-etf-fund-fact-sheet-en-us.pdf",
		SectorWeights: map[string]float64{
			"Information Technology": 32.84, "Financials": 12.57, "Communication Services": 10.26,
			"Consumer Discretionary": 9.84, "Health Care": 9.45, "Industrials": 9.00, "Consumer Staples": 5.24,
			"Energy": 4.01, "Utilities": 2.53, "Materials": 2.09,
		},
	},
	"SPY": {
		Name: "SPDR S&P 500 ETF Trust", AsOf: "2026-07-02",
		Source: "https://www.ssga.com/us/en/intermediary/etfs/spdr-sp-500-etf-trust-spy",
		SectorWeights: map[string]float64{
			"Information Technology": 36.87, "Financials": 12.22, "Communication Services": 9.87,
			"Consumer Discretionary": 9.33, "Health Care": 9.19, "Industrials": 8.88, "Consumer Staples": 4.67,
			"Energy": 2.99, "Utilities": 2.22, "Materials": 1.88, "Real Estate": 1.86,
		},
	},
	"ITOT": {
		Name: "iShares Core S&P Total U.S. Stock Market ETF", AsOf: "2026-03-31",
		Source: "https://www.ishares.com/us/literature/fact-sheet/itot-ishares-core-s-p-total-u-s-stock-market-etf-fund-fact-sheet-en-us.pdf",
		SectorWeights: map[string]float64{
			"Information Technology": 30.93, "Financials": 12.89, "Industrials": 10.28, "Health Care": 9.97,
			"Consumer Discretionary": 9.84, "Communication Services": 9.48, "Consumer Staples": 4.97,
			"Energy": 4.16, "Utilities": 2.49, "Materials": 2.40,
		},
	},
	"IWM": {
		Name: "iShares Russell 2000 ETF", AsOf: "2026-03-31",
		Source: "https://www.ishares.com/us/literature/fact-sheet/iwm-ishares-russell-2000-etf-fund-fact-sheet-en-us.pdf",
		SectorWeights: map[string]float64{
			"Industrials": 18.15, "Health Care": 17.69, "Financials": 17.01, "Information Technology": 14.09,
			"Consumer Discretionary": 8.38, "Energy": 6.71, "Real Estate": 5.31, "Materials": 4.91,
			"Utilities": 3.06, "Communication Services": 2.57,
		},
	},
	"QQQ": {
		Name: "Invesco QQQ Trust", AsOf: "2025-12-22",
		Source: "https://www.schwab.wallst.com/schwab/Prospect/research/etfs/reports/reportRetrieve.asp?reportType=etfrc&symbol=QQQ",
		SectorWeights: map[string]float64{
			"Information Technology": 58.9, "Communication Services": 13.5, "Consumer Discretionary": 11.4,
			"Consumer Staples": 6.4, "Health Care": 3.7, "Industrials": 3.2, "Utilities": 1.2,
			"Materials": 1.0, "Energy": 0.5, "Financials": 0.2,
		},
	},
	"VGT": {
		Name: "Vanguard Information Technology ETF", AsOf: "2026-03-31",
		Source: "https://fund-docs.vanguard.com/F0958.pdf",
		// Single-sector fund by mandate (tracks the MSCI US IMI/Information
		// Technology 25/50 Index) — 100% Information Technology is definitional,
		// not a fact-sheet-reported figure.
		SectorWeights: map[string]float64{
			"Information Technology": 100.0,
		},
	},
	"SCHD": {
		Name: "Schwab U.S. Dividend Equity ETF", AsOf: "2026-07-02",
		Source: "https://www.schwab.wallst.com/schwab/Prospect/research/etfs/reports/reportRetrieve.asp?reportType=etfrc&symbol=SCHD",
		SectorWeights: map[string]float64{
			"Health Care": 20.8, "Consumer Staples": 20.6, "Energy": 13.8, "Industrials": 11.9,
			"Financials": 10.1, "Information Technology": 9.0, "Consumer Discretionary": 7.6,
			"Communication Services": 6.0, "Utilities": 0.1,
		},
	},
	"VUG": {
		Name: "Vanguard Growth ETF", AsOf: "2026-03-31",
		Source: "https://fund-docs.vanguard.com/F0967.pdf",
		SectorWeights: map[string]float64{
			"Information Technology": 65.9, "Consumer Discretionary": 16.2, "Industrials": 7.7,
			"Health Care": 5.3, "Financials": 1.6, "Real Estate": 1.1, "Communication Services": 1.1,
			"Materials": 0.4, "Energy": 0.4, "Consumer Staples": 0.2, "Utilities": 0.2,
		},
	},
	"VTV": {
		Name: "Vanguard Value ETF", AsOf: "2026-03-31",
		Source: "https://fund-docs.vanguard.com/F0966.pdf",
		SectorWeights: map[string]float64{
			"Financials": 21.4, "Industrials": 16.4, "Health Care": 14.2, "Energy": 8.7,
			"Consumer Discretionary": 8.6, "Consumer Staples": 8.1, "Information Technology": 8.0,
			"Utilities": 6.0, "Communication Services": 3.4, "Real Estate": 2.7, "Materials": 2.4,
		},
	},
	"VIG": {
		Name: "Vanguard Dividend Appreciation ETF", AsOf: "2026-03-31",
		Source: "https://fund-docs.vanguard.com/F0920.pdf",
		SectorWeights: map[string]float64{
			"Information Technology": 23.3, "Financials": 21.0, "Health Care": 17.5, "Industrials": 12.1,
			"Consumer Staples": 10.6, "Consumer Discretionary": 4.8, "Energy": 3.9, "Materials": 3.5,
			"Utilities": 3.4,
		},
	},
	"VXUS": {
		Name: "Vanguard Total International Stock ETF", AsOf: "2026-03-31",
		Source: "https://fund-docs.vanguard.com/F3369.pdf",
		SectorWeights: map[string]float64{
			"Financials": 22.6, "Industrials": 15.7, "Information Technology": 14.6, "Consumer Discretionary": 9.9,
			"Materials": 7.7, "Health Care": 7.5, "Energy": 5.9, "Consumer Staples": 5.2,
			"Communication Services": 4.6, "Utilities": 3.6, "Real Estate": 2.6,
		},
	},
	"VEA": {
		Name: "Vanguard FTSE Developed Markets ETF", AsOf: "2026-03-31",
		Source: "https://fund-docs.vanguard.com/F0936.pdf",
		SectorWeights: map[string]float64{
			"Financials": 23.2, "Industrials": 18.2, "Consumer Discretionary": 9.7, "Information Technology": 9.2,
			"Health Care": 8.8, "Materials": 7.6, "Energy": 6.2, "Consumer Staples": 5.7,
			"Communication Services": 4.8, "Utilities": 3.8, "Real Estate": 2.6,
		},
	},
	"VWO": {
		Name: "Vanguard FTSE Emerging Markets ETF", AsOf: "2026-03-31",
		Source: "https://fund-docs.vanguard.com/F0964.pdf",
		SectorWeights: map[string]float64{
			"Information Technology": 29.7, "Financials": 20.7, "Consumer Discretionary": 10.3, "Industrials": 9.0,
			"Materials": 7.7, "Energy": 5.1, "Communication Services": 4.2, "Consumer Staples": 3.9,
			"Health Care": 3.9, "Utilities": 3.1, "Real Estate": 2.4,
		},
	},
	"VT": {
		Name: "Vanguard Total World Stock ETF", AsOf: "2026-03-31",
		Source: "https://fund-docs.vanguard.com/F3141.pdf",
		SectorWeights: map[string]float64{
			"Information Technology": 28.1, "Financials": 15.3, "Industrials": 13.8, "Consumer Discretionary": 12.2,
			"Health Care": 8.6, "Energy": 4.9, "Consumer Staples": 4.3, "Materials": 4.1,
			"Communication Services": 3.1, "Utilities": 3.1, "Real Estate": 2.4,
		},
	},
}
