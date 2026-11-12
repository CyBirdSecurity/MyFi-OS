package prices

import (
	"context"
	"time"
)

// Quote holds current price data for a single ticker.
type Quote struct {
	Ticker    string
	Price     float64
	Name      string
	Type      string // Yahoo's instrumentType: EQUITY, ETF, MUTUALFUND, CRYPTOCURRENCY...
	UpdatedAt time.Time
}

// HistoricalCandle holds a single day's closing price for a ticker.
type HistoricalCandle struct {
	Time  time.Time
	Close float64
}

// DividendEvent holds a single real dividend payment for a ticker.
type DividendEvent struct {
	Ticker         string
	ExDate         time.Time
	AmountPerShare float64
}

// PriceProvider is the abstraction over any market data source.
// Implementations: YahooProvider.
// Partial results are acceptable — tickers that fail are omitted from the map.
type PriceProvider interface {
	GetQuotes(ctx context.Context, tickers []string) (map[string]Quote, error)
}

// HistoricalProvider extends PriceProvider with candle/historical data.
type HistoricalProvider interface {
	PriceProvider
	GetCandles(ctx context.Context, ticker string, from, to time.Time) ([]HistoricalCandle, error)
}

// DividendProvider extends PriceProvider with historical dividend payments.
type DividendProvider interface {
	PriceProvider
	GetDividends(ctx context.Context, ticker string, from, to time.Time) ([]DividendEvent, error)
}
