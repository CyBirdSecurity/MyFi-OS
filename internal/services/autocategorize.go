package services

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
)

// autoCategorizeInterval is how often the job checks for uncategorized
// transactions without being nudged (e.g. ones a Plaid sync added).
const autoCategorizeInterval = 15 * time.Minute

// autoCategorizePageSize caps merchants handled per pass so one large import
// can't monopolize the CPU; the next pass picks up the rest.
const autoCategorizePageSize = 100

// AutoCategorizeService suggests categories for review-flagged, uncategorized
// transactions in the background, so imports never wait on a slow local LLM.
// Suggestions keep the review flag, so the user still confirms them.
type AutoCategorizeService struct {
	transactions *db.TransactionStore
	classifier   *ClassifierService
	wake         chan struct{}
}

func NewAutoCategorizeService(transactions *db.TransactionStore, classifier *ClassifierService) *AutoCategorizeService {
	return &AutoCategorizeService{transactions: transactions, classifier: classifier, wake: make(chan struct{}, 1)}
}

// Notify asks the job to run soon (e.g. right after an import). Never blocks.
func (s *AutoCategorizeService) Notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run processes pending transactions at startup, whenever notified, and on a
// fixed interval, until ctx is cancelled.
func (s *AutoCategorizeService) Run(ctx context.Context) {
	if !s.classifier.HasLLM() {
		log.Println("auto-categorize: no LLM configured; disabled")
		return
	}
	log.Println("auto-categorize: starting background categorizer")
	ticker := time.NewTicker(autoCategorizeInterval)
	defer ticker.Stop()
	for {
		s.process(ctx)
		select {
		case <-ctx.Done():
			log.Println("auto-categorize: stopped")
			return
		case <-s.wake:
		case <-ticker.C:
		}
	}
}

// process works through pending merchants page by page until none are left.
func (s *AutoCategorizeService) process(ctx context.Context) {
	for ctx.Err() == nil {
		pending, err := s.transactions.ListPendingAutoCategorize(ctx, autoCategorizePageSize)
		if err != nil {
			log.Printf("auto-categorize: %v", err)
			return
		}
		if len(pending) == 0 {
			return
		}

		byUser := make(map[uuid.UUID][]string)
		for _, p := range pending {
			byUser[p.UserID] = append(byUser[p.UserID], p.Merchant)
		}
		suggested := 0
		for userID, merchants := range byUser {
			cats, err := s.classifier.SuggestCategories(ctx, merchants)
			if err != nil || ctx.Err() != nil {
				// LLM unreachable or shutting down: leave these pending for the
				// next pass instead of marking them attempted.
				if err != nil {
					log.Printf("auto-categorize: LLM unavailable, will retry: %v", err)
				}
				return
			}
			for _, m := range merchants {
				var catID *uuid.UUID
				if id, ok := cats[m]; ok {
					catID = &id
					suggested++
				}
				// Marked attempted even without a suggestion so a merchant the
				// model can't place isn't retried on every pass.
				if err := s.transactions.ApplyAutoCategory(ctx, userID, m, catID); err != nil {
					log.Printf("auto-categorize: %v", err)
				}
			}
		}
		log.Printf("auto-categorize: suggested categories for %d of %d merchant(s)", suggested, len(pending))
	}
}
