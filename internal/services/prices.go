package services

import (
	"context"
	"log"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/cybirdsecurity/myfi-os/internal/prices"
)

const (
	priceCacheTTL      = 15 * time.Minute
	priceMaxConcurrent = 10
	// A ticker that failed to fetch isn't retried for this long, so a bad or
	// delisted ticker doesn't add a slow lookup to every page load.
	priceFailureTTL = 5 * time.Minute
	// Upper bound for a background refresh started by GetPricesAllowStale.
	priceRefreshTimeout = time.Minute
)

type cachedQuote struct {
	quote     prices.Quote
	expiresAt time.Time
}

type PriceService struct {
	provider    prices.PriceProvider
	cache       sync.Map // ticker -> cachedQuote
	failedUntil sync.Map // ticker -> time.Time
	refreshing  sync.Map // ticker -> struct{}, background refreshes in flight
	limiter     *rate.Limiter
}

func NewPriceService(provider prices.PriceProvider) *PriceService {
	return &PriceService{
		provider: provider,
		limiter:  rate.NewLimiter(rate.Every(time.Second), 10),
	}
}

// GetPrices returns quotes no older than priceCacheTTL, fetching expired or
// missing tickers before returning. Tickers that fail are left out.
func (s *PriceService) GetPrices(ctx context.Context, tickers []string) (map[string]prices.Quote, error) {
	result := make(map[string]prices.Quote, len(tickers))
	var missing []string
	for _, t := range tickers {
		if q, fresh, ok := s.cached(t); ok && fresh {
			result[t] = q
		} else {
			missing = append(missing, t)
		}
	}
	s.fetch(ctx, missing, result)
	return result, nil
}

// GetPricesAllowStale is GetPrices for page loads: an expired quote is
// returned straight away and refreshed in the background, so only tickers
// with no quote at all wait on the provider.
func (s *PriceService) GetPricesAllowStale(ctx context.Context, tickers []string) (map[string]prices.Quote, error) {
	result := make(map[string]prices.Quote, len(tickers))
	var missing, stale []string
	for _, t := range tickers {
		q, fresh, ok := s.cached(t)
		switch {
		case ok && fresh:
			result[t] = q
		case ok:
			result[t] = q
			stale = append(stale, t)
		default:
			missing = append(missing, t)
		}
	}
	s.refreshInBackground(stale)
	s.fetch(ctx, missing, result)
	return result, nil
}

func (s *PriceService) cached(t string) (q prices.Quote, fresh, ok bool) {
	v, ok := s.cache.Load(t)
	if !ok {
		return prices.Quote{}, false, false
	}
	c := v.(cachedQuote)
	return c.quote, time.Now().Before(c.expiresAt), true
}

func (s *PriceService) refreshInBackground(tickers []string) {
	var todo []string
	for _, t := range tickers {
		if _, busy := s.refreshing.LoadOrStore(t, struct{}{}); !busy {
			todo = append(todo, t)
		}
	}
	if len(todo) == 0 {
		return
	}
	go func() {
		defer func() {
			for _, t := range todo {
				s.refreshing.Delete(t)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), priceRefreshTimeout)
		defer cancel()
		s.fetch(ctx, todo, make(map[string]prices.Quote, len(todo)))
	}()
}

// fetch looks up tickers from the provider with bounded concurrency, caching
// and adding each quote to result. Recently failed tickers are skipped.
func (s *PriceService) fetch(ctx context.Context, tickers []string, result map[string]prices.Quote) {
	sem := make(chan struct{}, priceMaxConcurrent)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, ticker := range tickers {
		if v, ok := s.failedUntil.Load(ticker); ok && time.Now().Before(v.(time.Time)) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(t string) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := s.limiter.Wait(ctx); err != nil {
				return
			}
			quotes, err := s.provider.GetQuotes(ctx, []string{t})
			if err != nil {
				s.failedUntil.Store(t, time.Now().Add(priceFailureTTL))
				log.Printf("price fetch %s: %v", t, err)
				return
			}
			if q, ok := quotes[t]; ok {
				s.cache.Store(t, cachedQuote{quote: q, expiresAt: time.Now().Add(priceCacheTTL)})
				s.failedUntil.Delete(t)
				mu.Lock()
				result[t] = q
				mu.Unlock()
			} else {
				s.failedUntil.Store(t, time.Now().Add(priceFailureTTL))
				log.Printf("price fetch %s: ticker not found in Yahoo Finance response", t)
			}
		}(ticker)
	}
	wg.Wait()
}

// Invalidate clears the cache for specific tickers (e.g., after manual price
// update), including any recent failure, so the next lookup goes to the provider.
func (s *PriceService) Invalidate(tickers ...string) {
	for _, t := range tickers {
		s.cache.Delete(t)
		s.failedUntil.Delete(t)
	}
}

// GetCandles returns historical daily close prices if the underlying provider supports it.
// Returns (nil, false) when historical data is unavailable.
func (s *PriceService) GetCandles(ctx context.Context, ticker string, from, to time.Time) ([]prices.HistoricalCandle, bool, error) {
	hp, ok := s.provider.(prices.HistoricalProvider)
	if !ok {
		return nil, false, nil
	}
	candles, err := hp.GetCandles(ctx, ticker, from, to)
	if err != nil {
		return nil, true, err
	}
	return candles, true, nil
}

// GetDividends returns real historical dividend payments if the underlying
// provider supports it. Returns (nil, false) when dividend data is unavailable.
func (s *PriceService) GetDividends(ctx context.Context, ticker string, from, to time.Time) ([]prices.DividendEvent, bool, error) {
	dp, ok := s.provider.(prices.DividendProvider)
	if !ok {
		return nil, false, nil
	}
	events, err := dp.GetDividends(ctx, ticker, from, to)
	if err != nil {
		return nil, true, err
	}
	return events, true, nil
}
