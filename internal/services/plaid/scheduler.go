package plaid

import (
	"context"
	"log"
	"sync"
	"time"
)

// SchedulerStatus is returned by Scheduler.Status().
type SchedulerStatus struct {
	LastRunAt  *time.Time `json:"lastRunAt"`
	LastRunErr string     `json:"lastRunErr"`
	RunCount   int        `json:"runCount"`
}

// unlinkedItemMaxAge is how long a connection may sit without a linked account
// before the daily run removes it. It leaves ample time to finish the mapping
// step after linking.
const unlinkedItemMaxAge = 2 * time.Hour

// Scheduler runs a daily sync of every active Plaid Item, mirroring
// AutoSnapshotService's structure.
type Scheduler struct {
	svc *Service

	// afterSync, when set, runs after every sync run so dependent jobs (the
	// weekly auto-snapshot) see freshly pulled holdings.
	afterSync func(ctx context.Context)

	mu         sync.Mutex
	lastRunAt  *time.Time
	lastRunErr string
	runCount   int
}

func NewScheduler(svc *Service) *Scheduler { return &Scheduler{svc: svc} }

// SetAfterSync registers a callback run after each sync run completes.
// Call it before Run.
func (s *Scheduler) SetAfterSync(fn func(ctx context.Context)) { s.afterSync = fn }

// Status returns the current state of the sync scheduler.
func (s *Scheduler) Status() SchedulerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SchedulerStatus{
		LastRunAt:  s.lastRunAt,
		LastRunErr: s.lastRunErr,
		RunCount:   s.runCount,
	}
}

// Run starts the daily sync loop and blocks until ctx is cancelled. The first
// run is delayed briefly so startup isn't slowed by network calls.
func (s *Scheduler) Run(ctx context.Context) {
	log.Println("plaid: starting daily sync scheduler")

	select {
	case <-time.After(2 * time.Minute):
		s.runAndRecord(ctx)
	case <-ctx.Done():
		return
	}

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.runAndRecord(ctx)
		case <-ctx.Done():
			log.Println("plaid: sync scheduler stopped")
			return
		}
	}
}

func (s *Scheduler) runAndRecord(ctx context.Context) {
	if n, err := s.svc.RemoveUnlinkedItems(ctx, unlinkedItemMaxAge); err != nil {
		log.Printf("plaid: unlinked item cleanup: %v", err)
	} else if n > 0 {
		log.Printf("plaid: removed %d unlinked item(s)", n)
	}
	err := s.syncAll(ctx)
	if s.afterSync != nil {
		s.afterSync(ctx)
	}
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

// syncAll syncs every active item across all users, continuing past
// individual failures (per-item errors are recorded on the item itself).
func (s *Scheduler) syncAll(ctx context.Context) error {
	items, err := s.svc.items.ListAllActive(ctx)
	if err != nil {
		return err
	}
	log.Printf("plaid: syncing %d item(s)", len(items))
	for _, item := range items {
		// Failures are logged by syncItem and recorded on the item itself.
		_, _ = s.svc.SyncItemCached(ctx, item)
	}
	return nil
}
