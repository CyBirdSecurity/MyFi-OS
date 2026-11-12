package services

import (
	"context"
	"log"
	"time"

	"github.com/cybirdsecurity/myfi-os/internal/db"
)

// RecurringTransactionService adds due occurrences of recurring transactions
// (e.g. a monthly mortgage payment) once a day.
type RecurringTransactionService struct {
	transactions *db.TransactionStore
}

func NewRecurringTransactionService(transactions *db.TransactionStore) *RecurringTransactionService {
	return &RecurringTransactionService{transactions: transactions}
}

// Run generates due occurrences at startup and then shortly after each UTC
// midnight, until ctx is cancelled.
func (s *RecurringTransactionService) Run(ctx context.Context) {
	log.Println("recurring: starting recurring transaction scheduler")
	s.generate(ctx)
	for {
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 5, 0, 0, time.UTC)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-timer.C:
			s.generate(ctx)
		case <-ctx.Done():
			timer.Stop()
			log.Println("recurring: scheduler stopped")
			return
		}
	}
}

func (s *RecurringTransactionService) generate(ctx context.Context) {
	n, err := s.transactions.GenerateDueRecurring(ctx, nil, Today())
	if err != nil {
		log.Printf("recurring: generate failed: %v", err)
		return
	}
	if n > 0 {
		log.Printf("recurring: added %d transaction(s)", n)
	}
}

// Today is the current UTC date at midnight, the cutoff for due occurrences.
func Today() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}
