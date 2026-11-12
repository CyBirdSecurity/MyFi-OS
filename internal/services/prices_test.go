package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cybirdsecurity/myfi-os/internal/prices"
)

type fakeQuoteProvider struct {
	mu    sync.Mutex
	price float64
	fail  bool
	calls int
	delay time.Duration
}

func (f *fakeQuoteProvider) GetQuotes(_ context.Context, tickers []string) (map[string]prices.Quote, error) {
	time.Sleep(f.delay)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.fail {
		return nil, errors.New("boom")
	}
	return map[string]prices.Quote{tickers[0]: {Ticker: tickers[0], Price: f.price}}, nil
}

func (f *fakeQuoteProvider) set(price float64, fail bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.price, f.fail = price, fail
}

func (f *fakeQuoteProvider) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func expire(s *PriceService, ticker string) {
	v, _ := s.cache.Load(ticker)
	c := v.(cachedQuote)
	c.expiresAt = time.Now().Add(-time.Second)
	s.cache.Store(ticker, c)
}

func TestGetPricesAllowStale_ServesStaleAndRefreshes(t *testing.T) {
	ctx := context.Background()
	p := &fakeQuoteProvider{price: 100}
	s := NewPriceService(p)

	if got, _ := s.GetPricesAllowStale(ctx, []string{"VTI"}); got["VTI"].Price != 100 {
		t.Fatalf("first lookup = %v, want 100", got["VTI"].Price)
	}

	expire(s, "VTI")
	p.set(110, false)
	p.delay = 200 * time.Millisecond
	start := time.Now()
	got, _ := s.GetPricesAllowStale(ctx, []string{"VTI"})
	if got["VTI"].Price != 100 {
		t.Fatalf("stale lookup = %v, want the cached 100", got["VTI"].Price)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("stale lookup waited on the provider")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if q, fresh, _ := s.cached("VTI"); fresh && q.Price == 110 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background refresh never updated the cache")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestGetPrices_WaitsForExpiredQuote(t *testing.T) {
	ctx := context.Background()
	p := &fakeQuoteProvider{price: 100}
	s := NewPriceService(p)
	s.GetPrices(ctx, []string{"VTI"})
	expire(s, "VTI")
	p.set(110, false)
	if got, _ := s.GetPrices(ctx, []string{"VTI"}); got["VTI"].Price != 110 {
		t.Fatalf("GetPrices = %v, want the fresh 110", got["VTI"].Price)
	}
}

func TestGetPrices_SkipsRecentlyFailedTicker(t *testing.T) {
	ctx := context.Background()
	p := &fakeQuoteProvider{fail: true}
	s := NewPriceService(p)

	s.GetPrices(ctx, []string{"BAD"})
	s.GetPricesAllowStale(ctx, []string{"BAD"})
	if n := p.callCount(); n != 1 {
		t.Fatalf("provider called %d times, want 1 (failure should be remembered)", n)
	}

	// An explicit refresh retries straight away.
	p.set(5, false)
	s.Invalidate("BAD")
	if got, _ := s.GetPrices(ctx, []string{"BAD"}); got["BAD"].Price != 5 {
		t.Fatalf("after Invalidate = %v, want 5", got["BAD"].Price)
	}
}
