package services

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
	"github.com/cybirdsecurity/myfi-os/internal/prices"
)

// dividendHoldingReader is the subset of db.HoldingStore used to enumerate holdings.
type dividendHoldingReader interface {
	ListUsersWithHoldings(ctx context.Context) ([]uuid.UUID, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]models.Holding, error)
}

// holdingShareStore is the subset of db.HoldingStore used to apply DRIP reinvestment.
type holdingShareStore interface {
	AddShares(ctx context.Context, id uuid.UUID, delta float64) error
}

// dividendRecorder is the subset of db.DividendStore used by AutoDividendService.
type dividendRecorder interface {
	LastAppliedDate(ctx context.Context, holdingID uuid.UUID) (*time.Time, error)
	Create(ctx context.Context, p db.CreateDividendParams) (*models.HoldingDividend, error)
}

// dividendGetter is the subset of PriceService used by AutoDividendService.
type dividendGetter interface {
	GetDividends(ctx context.Context, ticker string, from, to time.Time) ([]prices.DividendEvent, bool, error)
}

// AutoDividendStatus is returned by AutoDividendService.Status().
type AutoDividendStatus struct {
	LastRunAt  *time.Time `json:"lastRunAt"`
	LastRunErr string     `json:"lastRunErr"`
	RunCount   int        `json:"runCount"`
}

// AutoDividendService checks each holding's ticker for real dividend
// payments and, for holdings with drip_enabled, reinvests the payment by
// adding shares. This removes the need for users to manually bump share
// counts every time an ETF or stock pays a dividend.
//
// It runs on two monthly checkpoints — the last calendar day of the month,
// and the first Monday of the following month — so a weekend/holiday delay
// in Yahoo posting a dividend around month-end doesn't cause it to be missed.
// Each run only looks for dividends posted since the service's last run, and
// a holding's very first check never backfills historical dividends.
type AutoDividendService struct {
	holdings      dividendHoldingReader
	holdingShares holdingShareStore
	dividends     dividendRecorder
	prices        dividendGetter

	mu         sync.Mutex
	lastRunAt  *time.Time
	lastRunErr string
	runCount   int
}

func NewAutoDividendService(
	holdings *db.HoldingStore,
	dividends *db.DividendStore,
	prices *PriceService,
) *AutoDividendService {
	return &AutoDividendService{
		holdings:      holdings,
		holdingShares: holdings,
		dividends:     dividends,
		prices:        prices,
	}
}

// Status returns the current state of the dividend scheduler.
func (s *AutoDividendService) Status() AutoDividendStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return AutoDividendStatus{
		LastRunAt:  s.lastRunAt,
		LastRunErr: s.lastRunErr,
		RunCount:   s.runCount,
	}
}

// Trigger starts a dividend check in a new goroutine (non-blocking).
func (s *AutoDividendService) Trigger() {
	go s.runAndRecord(context.Background())
}

func (s *AutoDividendService) runAndRecord(ctx context.Context) {
	// The window for "new since last check" is captured before the run so
	// that the next run's window starts exactly where this one left off,
	// regardless of how long ProcessDividends takes.
	s.mu.Lock()
	windowStart := time.Now().UTC()
	if s.lastRunAt != nil {
		windowStart = *s.lastRunAt
	}
	s.mu.Unlock()

	err := s.ProcessDividends(ctx, windowStart)
	now := time.Now().UTC()
	s.mu.Lock()
	s.lastRunAt = &now
	s.runCount++
	if err != nil {
		s.lastRunErr = err.Error()
	} else {
		s.lastRunErr = ""
	}
	s.mu.Unlock()
}

// Run starts the dividend-check scheduler and blocks until ctx is cancelled.
// It fires once immediately on startup (establishing the baseline so nothing
// backfills), then re-fires on each subsequent monthly checkpoint.
func (s *AutoDividendService) Run(ctx context.Context) {
	log.Println("dividends: starting dividend scheduler")
	s.runAndRecord(ctx)

	for {
		next := nextDividendCheck(time.Now().UTC())
		timer := time.NewTimer(time.Until(next))
		select {
		case <-timer.C:
			s.runAndRecord(ctx)
		case <-ctx.Done():
			timer.Stop()
			log.Println("dividends: scheduler stopped")
			return
		}
	}
}

// ProcessDividends checks every user's holdings for dividend events posted
// since windowStart and applies DRIP reinvestment where enabled.
func (s *AutoDividendService) ProcessDividends(ctx context.Context, windowStart time.Time) error {
	userIDs, err := s.holdings.ListUsersWithHoldings(ctx)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	log.Printf("dividends: checking %d user(s) for dividends since %s", len(userIDs), windowStart.Format("2006-01-02"))

	for _, userID := range userIDs {
		holdings, err := s.holdings.ListByUser(ctx, userID)
		if err != nil {
			log.Printf("dividends: list holdings for user %s: %v", userID, err)
			continue
		}
		for _, h := range holdings {
			// Cash holdings store a dollar amount, not a share count, in a
			// ticker slot that may coincidentally match a real tradeable
			// stock (e.g. "CASH") — never treat them as a dividend payer.
			if h.AssetClass == "cash_equivalent" {
				continue
			}
			if err := s.processHolding(ctx, h, windowStart, now); err != nil {
				log.Printf("dividends: holding %s (%s): %v", h.ID, h.Ticker, err)
			}
		}
	}
	return nil
}

func (s *AutoDividendService) processHolding(ctx context.Context, h models.Holding, windowStart, now time.Time) error {
	last, err := s.dividends.LastAppliedDate(ctx, h.ID)
	if err != nil {
		return err
	}
	// A holding-specific checkpoint always wins over the service-wide window,
	// since it reflects exactly what's already been applied for this holding.
	if last != nil && last.After(windowStart) {
		windowStart = *last
	}
	if !windowStart.Before(now) {
		return nil
	}

	events, ok, err := s.prices.GetDividends(ctx, h.PriceTicker(), windowStart, now)
	if err != nil {
		return err
	}
	if !ok || len(events) == 0 {
		return nil
	}

	sort.Slice(events, func(i, j int) bool { return events[i].ExDate.Before(events[j].ExDate) })

	for _, ev := range events {
		if !ev.ExDate.After(windowStart) {
			continue // already covered by a previous check
		}
		if err := s.applyDividend(ctx, h, ev); err != nil {
			return err
		}
	}
	return nil
}

// applyDividend records a dividend event and, if DRIP reinvestment applies,
// adds the resulting shares to the holding. The dividend record is written
// first — its (holding_id, ex_date) uniqueness constraint is what makes
// re-running this safe, so shares are only added once record creation
// confirms this event hasn't been applied before.
func (s *AutoDividendService) applyDividend(ctx context.Context, h models.Holding, ev prices.DividendEvent) error {
	dividendAmount := h.Shares * ev.AmountPerShare

	var dripApplied bool
	var sharesAdded, priceUsed *float64
	if h.DripEnabled && h.PricePerShare != nil && *h.PricePerShare > 0 {
		price := *h.PricePerShare
		added := dividendAmount / price
		dripApplied = true
		sharesAdded = &added
		priceUsed = &price
	}

	created, err := s.dividends.Create(ctx, db.CreateDividendParams{
		HoldingID:        h.ID,
		UserID:           h.UserID,
		ExDate:           ev.ExDate,
		DividendPerShare: ev.AmountPerShare,
		SharesBefore:     h.Shares,
		DividendAmount:   dividendAmount,
		DripApplied:      dripApplied,
		SharesAdded:      sharesAdded,
		PriceUsed:        priceUsed,
	})
	if err != nil {
		return err
	}
	if created == nil {
		return nil // already applied by a previous run
	}

	if dripApplied {
		return s.holdingShares.AddShares(ctx, h.ID, *sharesAdded)
	}
	return nil
}

// nextDividendCheck returns the next of two monthly checkpoints strictly
// after t: the last calendar day of a month, or the first Monday of the
// following month (a safety net in case month-end postings lag over a
// weekend/holiday).
func nextDividendCheck(t time.Time) time.Time {
	cursor := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 24; i++ {
		nextMonthStart := cursor.AddDate(0, 1, 0)

		lastDay := nextMonthStart.AddDate(0, 0, -1)
		if lastDay.After(t) {
			return lastDay
		}

		monday := nextMonthStart
		for monday.Weekday() != time.Monday {
			monday = monday.AddDate(0, 0, 1)
		}
		if monday.After(t) {
			return monday
		}

		cursor = nextMonthStart
	}
	// Should never be reached, but guarantees termination.
	return t.AddDate(0, 1, 0)
}
