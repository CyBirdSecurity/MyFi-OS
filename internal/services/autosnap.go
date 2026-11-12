package services

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
	"github.com/cybirdsecurity/myfi-os/internal/prices"
)

// holdingReader is the subset of db.HoldingStore used by AutoSnapshotService.
type holdingReader interface {
	ListUsersWithHoldings(ctx context.Context) ([]uuid.UUID, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]models.Holding, error)
}

// snapshotWriter is the subset of db.SnapshotStore used by AutoSnapshotService.
type snapshotWriter interface {
	Upsert(ctx context.Context, p db.UpsertSnapshotParams) (*models.BalanceSnapshot, error)
}

// priceGetter is the subset of PriceService used by AutoSnapshotService.
type priceGetter interface {
	GetPrices(ctx context.Context, tickers []string) (map[string]prices.Quote, error)
}

// AutoSnapStatus is returned by AutoSnapshotService.Status().
type AutoSnapStatus struct {
	LastRunAt  *time.Time `json:"lastRunAt"`
	LastRunErr string     `json:"lastRunErr"`
	RunCount   int        `json:"runCount"`
}

// AutoSnapshotService creates weekly balance snapshots derived from live holding
// values. This removes the need for users to manually log snapshots on
// investment-type accounts — every Monday the service computes the total market
// value of each account's holdings and upserts a snapshot dated to that Monday.
//
// Snapshots written by this service have source='computed'. Manual snapshots
// written by the user use source='manual' and are never touched by this service.
type AutoSnapshotService struct {
	holdings  holdingReader
	snapshots snapshotWriter
	prices    priceGetter

	mu         sync.Mutex
	lastRunAt  *time.Time
	lastRunErr string
	runCount   int
}

func NewAutoSnapshotService(
	holdings *db.HoldingStore,
	snapshots *db.SnapshotStore,
	prices *PriceService,
) *AutoSnapshotService {
	return &AutoSnapshotService{holdings: holdings, snapshots: snapshots, prices: prices}
}

// Status returns the current state of the snapshot scheduler.
func (s *AutoSnapshotService) Status() AutoSnapStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return AutoSnapStatus{
		LastRunAt:  s.lastRunAt,
		LastRunErr: s.lastRunErr,
		RunCount:   s.runCount,
	}
}

// runAndRecord calls TakeSnapshots and records the outcome for Status().
func (s *AutoSnapshotService) runAndRecord(ctx context.Context) {
	err := s.TakeSnapshots(ctx)
	now := time.Now()
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

// RunIfDue takes the weekly snapshot on Mondays (UTC), once per week. It is
// called daily — by the Plaid scheduler after each sync, so the snapshot is
// built from freshly pulled holdings, or by Run's own daily tick when Plaid is
// off. Nothing runs at startup: a restart on any other day waits for Monday.
func (s *AutoSnapshotService) RunIfDue(ctx context.Context) {
	now := time.Now().UTC()
	if now.Weekday() != time.Monday {
		return
	}
	s.mu.Lock()
	due := s.lastRunAt == nil || s.lastRunAt.Before(thisWeekMonday(now))
	s.mu.Unlock()
	if due {
		s.runAndRecord(ctx)
	}
}

// Run starts the daily check for the weekly auto-snapshot and blocks until ctx
// is cancelled. It does not snapshot at startup; see RunIfDue.
func (s *AutoSnapshotService) Run(ctx context.Context) {
	log.Println("autosnap: starting weekly snapshot scheduler")

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.RunIfDue(ctx)
		case <-ctx.Done():
			log.Println("autosnap: scheduler stopped")
			return
		}
	}
}

// TakeSnapshots computes the current holding value per account for every user
// that owns at least one holding, then upserts a 'computed' snapshot dated to
// the Monday of the current week. Calling this multiple times in the same week
// will simply overwrite the snapshot for that week (upsert on account+date).
func (s *AutoSnapshotService) TakeSnapshots(ctx context.Context) error {
	userIDs, err := s.holdings.ListUsersWithHoldings(ctx)
	if err != nil {
		return err
	}

	weekStart := thisWeekMonday(time.Now().UTC())
	log.Printf("autosnap: taking snapshots for %d user(s) — week of %s", len(userIDs), weekStart.Format("2006-01-02"))

	for _, userID := range userIDs {
		if _, err := s.snapshotUser(ctx, userID, weekStart); err != nil {
			log.Printf("autosnap: user %s failed: %v", userID, err)
			// Continue processing remaining users even if one fails.
		}
	}
	return nil
}

// SnapshotUser runs the same computed-snapshot logic as TakeSnapshots but
// scoped to a single user, synchronously. It lets a user manually snapshot
// their own accounts on demand (e.g. after updating holdings) rather than
// waiting for the weekly job. Unlike the weekly job, it dates the snapshot to
// today rather than the current week's Monday, so repeated manual triggers
// add new history points instead of overwriting the week's existing one. It
// returns the number of accounts snapshotted.
func (s *AutoSnapshotService) SnapshotUser(ctx context.Context, userID uuid.UUID) (int, error) {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return s.snapshotUser(ctx, userID, today)
}

func (s *AutoSnapshotService) snapshotUser(ctx context.Context, userID uuid.UUID, weekStart time.Time) (int, error) {
	holdings, err := s.holdings.ListByUser(ctx, userID)
	if err != nil {
		return 0, err
	}
	if len(holdings) == 0 {
		return 0, nil
	}

	// Deduplicate tickers for the price fetch — skip manually-priced holdings
	// and cash. Cash holdings store a dollar amount in Shares, not a share
	// count, so their ticker (e.g. "CASH") must never be looked up as a
	// security — that symbol may coincidentally match a real tradeable stock.
	seen := map[string]bool{}
	tickers := make([]string, 0, len(holdings))
	for _, h := range holdings {
		if h.AssetClass == "cash_equivalent" || h.ManualPrice {
			continue
		}
		if t := h.PriceTicker(); !seen[t] {
			tickers = append(tickers, t)
			seen[t] = true
		}
	}

	// Fetch live prices; fall back to last known price if a ticker fails.
	priceMap, _ := s.prices.GetPrices(ctx, tickers)

	// Sum holding value per account.
	accountValue := make(map[uuid.UUID]float64)
	for _, h := range holdings {
		if h.AssetClass == "cash_equivalent" {
			accountValue[h.AccountID] += h.Shares
			continue
		}
		var pricePerShare float64
		if h.ManualPrice {
			if h.PricePerShare != nil {
				pricePerShare = *h.PricePerShare
			}
		} else if q, ok := priceMap[h.PriceTicker()]; ok {
			pricePerShare = q.Price
		} else if h.PricePerShare != nil {
			pricePerShare = *h.PricePerShare
		}
		if pricePerShare > 0 {
			accountValue[h.AccountID] += h.Shares * pricePerShare
		}
	}

	// Upsert one computed snapshot per account.
	count := 0
	for accountID, value := range accountValue {
		if value <= 0 {
			continue
		}
		_, err := s.snapshots.Upsert(ctx, db.UpsertSnapshotParams{
			AccountID:    accountID,
			UserID:       userID,
			SnapshotDate: weekStart,
			Balance:      value,
			Source:       "computed",
		})
		if err != nil {
			log.Printf("autosnap: upsert snapshot for account %s: %v", accountID, err)
			continue
		}
		count++
	}
	return count, nil
}

// thisWeekMonday returns midnight UTC of the Monday that starts the ISO week
// containing t.
func thisWeekMonday(t time.Time) time.Time {
	weekday := t.Weekday()
	if weekday == time.Sunday {
		// Sunday is day 0 in Go but day 7 in ISO weeks — step back 6 days.
		weekday = 7
	}
	daysBack := int(weekday - time.Monday)
	monday := t.AddDate(0, 0, -daysBack)
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
}
