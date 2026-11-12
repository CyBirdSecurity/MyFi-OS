package plaid

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// contributionLookback is how far back contributions are synced. A year
// covers January 1st, and also shows whether an account's institution
// reports contributions at all.
const contributionLookback = 365 * 24 * time.Hour

// iraAccountTypes take contributions as plain cash deposits, so a deposit
// counts as one there.
var iraAccountTypes = map[string]bool{
	"traditional_ira": true, "roth_ira": true, "sep_ira": true, "simple_ira": true,
}

// Descriptions marking money that moved between accounts rather than new
// money put in.
var rolloverWords = []string{"rollover", "roll over", "roll-over", "transfer", "conversion", "acat"}

// Descriptions marking the employer's money. Plaid has no field for this.
var employerWords = []string{"employer", "match", "safe harbor", "profit sharing", "profit-sharing", "nonelective", "non-elective", "company contribution"}

// classifyContribution reports whether an investment transaction is a
// contribution to an account of the given type, and whose: "employee" or
// "employer".
func classifyContribution(t InvestmentTransaction, accountType string) (source string, ok bool) {
	name := strings.ToLower(t.Name)
	for _, w := range rolloverWords {
		if strings.Contains(name, w) {
			return "", false
		}
	}
	if t.Subtype != "contribution" && !(t.Subtype == "deposit" && iraAccountTypes[accountType]) {
		return "", false
	}
	for _, w := range employerWords {
		if strings.Contains(name, w) {
			return "employer", true
		}
	}
	return "employee", true
}

// syncContributions records the last year of contributions to linked
// retirement accounts. Failures are added to result rather than aborting the
// sync that already succeeded.
func (s *Service) syncContributions(
	ctx context.Context,
	api API,
	item models.PlaidItem,
	token string,
	linkedByPlaidID map[string]models.Account,
	plaidAccounts []Account,
	result *SyncResult,
) {
	if s.contribEntries == nil {
		return
	}
	var plaidIDs []string
	for _, pa := range plaidAccounts {
		if acc, ok := linkedByPlaidID[pa.AccountID]; ok && IsInvestmentType(pa.Type) && models.RetirementAccountTypes[acc.AccountType] {
			plaidIDs = append(plaidIDs, pa.AccountID)
		}
	}
	if len(plaidIDs) == 0 {
		return
	}

	now := time.Now().UTC()
	txns, _, err := api.GetInvestmentTransactions(ctx, token, now.Add(-contributionLookback), now, plaidIDs)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("contributions: %v", err))
		return
	}

	var entries []db.ContributionEntryParams
	for _, t := range txns {
		acc, linked := linkedByPlaidID[t.AccountID]
		if !linked || !models.RetirementAccountTypes[acc.AccountType] {
			continue
		}
		source, ok := classifyContribution(t, acc.AccountType)
		if !ok || t.ID == "" || t.Amount == 0 {
			continue
		}
		id, name := t.ID, t.Name
		entries = append(entries, db.ContributionEntryParams{
			AccountID:   acc.ID,
			UserID:      item.UserID,
			EntryDate:   t.Date,
			Amount:      math.Abs(t.Amount),
			Source:      source,
			Description: &name,
			ExternalID:  &id,
		})
	}
	if err := s.contribEntries.UpsertSynced(ctx, entries); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("contributions: %v", err))
	}
}
