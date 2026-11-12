// Package data embeds reference datasets used by MyFi's services.
package data

import (
	_ "embed"
	"encoding/csv"
	"strconv"
	"strings"
)

// historical_returns.csv columns: year, equity_total_return, bond_total_return,
// tbill_total_return, cpi_inflation — annual U.S. figures, 1928–present.
//
// Sources: S&P 500 (with dividends), 10-year U.S. Treasury bond, and 3-month
// U.S. Treasury bill total returns are from Aswath Damodaran's "Historical
// Returns" dataset (NYU Stern, https://pages.stern.nyu.edu/~adamodar/New_Home_Page/data.html),
// downloaded from https://pages.stern.nyu.edu/~adamodar/pc/datasets/histretSP.xls.
// CPI inflation is the "Inflation Rate" sheet of that same workbook, itself
// sourced from FRED's CPIAUCNS series (Consumer Price Index for All Urban
// Consumers, not seasonally adjusted, percent change from year ago).
//
//go:embed historical_returns.csv
var historicalReturnsCSV string

// HistoricalYear holds one calendar year's annual total returns and inflation.
type HistoricalYear struct {
	Year         int
	EquityReturn float64
	BondReturn   float64
	TBillReturn  float64
	Inflation    float64
}

// HistoricalReturns is the parsed, year-ordered contents of historical_returns.csv.
var HistoricalReturns []HistoricalYear

func init() {
	r := csv.NewReader(strings.NewReader(historicalReturnsCSV))
	rows, err := r.ReadAll()
	if err != nil {
		panic("data: failed to parse historical_returns.csv: " + err.Error())
	}
	for _, row := range rows[1:] { // skip header
		year, err := strconv.Atoi(row[0])
		if err != nil {
			panic("data: bad year in historical_returns.csv: " + err.Error())
		}
		equity, _ := strconv.ParseFloat(row[1], 64)
		bond, _ := strconv.ParseFloat(row[2], 64)
		tbill, _ := strconv.ParseFloat(row[3], 64)
		inflation, _ := strconv.ParseFloat(row[4], 64)
		HistoricalReturns = append(HistoricalReturns, HistoricalYear{
			Year:         year,
			EquityReturn: equity,
			BondReturn:   bond,
			TBillReturn:  tbill,
			Inflation:    inflation,
		})
	}
}
