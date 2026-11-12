package prices

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// YahooProvider fetches live quotes and historical candles from Yahoo Finance.
// No API key is required.
type YahooProvider struct {
	client *http.Client
}

func NewYahooProvider() *YahooProvider {
	return &YahooProvider{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// GetQuotes fetches current prices for a set of tickers via Yahoo Finance v8
// chart API (one request per ticker). The v7 batch quote endpoint was blocked
// by Yahoo in 2025; the v8 chart API returns regularMarketPrice in its meta
// field and remains accessible without authentication.
func (p *YahooProvider) GetQuotes(ctx context.Context, tickers []string) (map[string]Quote, error) {
	if len(tickers) == 0 {
		return map[string]Quote{}, nil
	}

	results := make(map[string]Quote, len(tickers))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)

	for _, ticker := range tickers {
		wg.Add(1)
		sem <- struct{}{}
		go func(t string) {
			defer wg.Done()
			defer func() { <-sem }()

			q, err := p.getQuote(ctx, t)
			if err != nil || q == nil {
				return
			}
			mu.Lock()
			results[t] = *q
			mu.Unlock()
		}(ticker)
	}
	wg.Wait()
	return results, nil
}

// getQuote fetches current price for a single ticker using the v8 chart endpoint.
func (p *YahooProvider) getQuote(ctx context.Context, ticker string) (*Quote, error) {
	rawURL := fmt.Sprintf(
		"https://query2.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1d",
		url.PathEscape(ticker),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo finance chart quote: HTTP %d for %s", resp.StatusCode, ticker)
	}

	var data yahooChartResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("yahoo finance chart quote decode: %w", err)
	}
	if data.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo finance chart quote: %s", data.Chart.Error.Description)
	}
	if len(data.Chart.Result) == 0 {
		return nil, fmt.Errorf("yahoo finance chart quote: no data for %s", ticker)
	}

	meta := data.Chart.Result[0].Meta
	if meta.RegularMarketPrice == 0 {
		return nil, nil
	}

	name := html.UnescapeString(meta.LongName)
	if name == "" {
		name = html.UnescapeString(meta.ShortName)
	}
	return &Quote{
		Ticker:    ticker,
		Price:     meta.RegularMarketPrice,
		Name:      name,
		Type:      meta.InstrumentType,
		UpdatedAt: time.Now(),
	}, nil
}

// --- Historical Chart API (v8) ---

type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				ShortName          string  `json:"shortName"`
				LongName           string  `json:"longName"`
				InstrumentType     string  `json:"instrumentType"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				AdjClose []struct {
					AdjClose []float64 `json:"adjclose"`
				} `json:"adjclose"`
			} `json:"indicators"`
			Events struct {
				Dividends map[string]struct {
					Amount float64 `json:"amount"`
					Date   int64   `json:"date"`
				} `json:"dividends"`
			} `json:"events"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

// GetDividends fetches real historical dividend payments for a ticker from
// Yahoo Finance v8 using the events=div parameter on the chart endpoint.
func (p *YahooProvider) GetDividends(ctx context.Context, ticker string, from, to time.Time) ([]DividendEvent, error) {
	params := url.Values{}
	params.Set("period1", fmt.Sprintf("%d", from.Unix()))
	params.Set("period2", fmt.Sprintf("%d", to.Unix()))
	params.Set("interval", "1d")
	params.Set("events", "div")

	rawURL := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?%s",
		url.PathEscape(ticker), params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo finance chart dividends: HTTP %d for %s", resp.StatusCode, ticker)
	}

	var data yahooChartResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("yahoo finance chart dividends decode: %w", err)
	}
	if data.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo finance chart dividends: %s", data.Chart.Error.Description)
	}
	if len(data.Chart.Result) == 0 {
		return []DividendEvent{}, nil
	}

	dividends := data.Chart.Result[0].Events.Dividends
	events := make([]DividendEvent, 0, len(dividends))
	for _, d := range dividends {
		events = append(events, DividendEvent{
			Ticker:         ticker,
			ExDate:         time.Unix(d.Date, 0).UTC(),
			AmountPerShare: d.Amount,
		})
	}
	return events, nil
}

// GetCandles fetches daily adjusted-close prices for a ticker from Yahoo Finance v8.
func (p *YahooProvider) GetCandles(ctx context.Context, ticker string, from, to time.Time) ([]HistoricalCandle, error) {
	params := url.Values{}
	params.Set("period1", fmt.Sprintf("%d", from.Unix()))
	params.Set("period2", fmt.Sprintf("%d", to.Unix()))
	params.Set("interval", "1d")
	params.Set("includeAdjustedClose", "true")

	rawURL := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?%s",
		url.PathEscape(ticker), params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo finance chart: HTTP %d for %s", resp.StatusCode, ticker)
	}

	var data yahooChartResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("yahoo finance chart decode: %w", err)
	}
	if data.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo finance chart: %s", data.Chart.Error.Description)
	}
	if len(data.Chart.Result) == 0 {
		return nil, fmt.Errorf("yahoo finance chart: no data for %s", ticker)
	}

	result := data.Chart.Result[0]
	timestamps := result.Timestamp
	if len(timestamps) == 0 || len(result.Indicators.AdjClose) == 0 {
		return []HistoricalCandle{}, nil
	}

	closes := result.Indicators.AdjClose[0].AdjClose
	if len(closes) != len(timestamps) {
		return nil, fmt.Errorf("yahoo finance chart: mismatched data lengths for %s", ticker)
	}

	candles := make([]HistoricalCandle, 0, len(timestamps))
	for i, ts := range timestamps {
		if closes[i] == 0 {
			continue // skip days with no data (e.g. holidays with carry-forward = 0)
		}
		candles = append(candles, HistoricalCandle{
			Time:  time.Unix(ts, 0).UTC(),
			Close: closes[i],
		})
	}
	return candles, nil
}
