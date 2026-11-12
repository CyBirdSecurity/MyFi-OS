package plaid

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// syncLiabilities fetches Plaid's liabilities data for the Item and writes it
// into the linked home account's home_details (mortgage terms) or credit_card
// account's credit_card_details (APR/statement/limit). Kept non-fatal — an
// Item linked without liabilities consent (e.g. via "investments" mode) must
// not fail the rest of the sync.
func (s *Service) syncLiabilities(
	ctx context.Context,
	api API,
	item models.PlaidItem,
	token string,
	linkedByPlaidID map[string]models.Account,
	plaidAccounts []Account,
	result *SyncResult,
) {
	// Skip the call entirely when nothing linked could use it — avoids a
	// spurious error on an Item with no consenting mortgage/credit-card account.
	hasTarget := false
	for _, pa := range plaidAccounts {
		if acc, ok := linkedByPlaidID[pa.AccountID]; ok && (acc.AccountType == "home" || acc.AccountType == "credit_card") {
			hasTarget = true
			break
		}
	}
	if !hasTarget {
		return
	}

	mortgages, creditCardLiabilities, err := api.GetLiabilities(ctx, token)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("%s: liabilities: %v", displayName(item), err))
		return
	}

	balanceByPlaidID := make(map[string]Account, len(plaidAccounts))
	for _, pa := range plaidAccounts {
		balanceByPlaidID[pa.AccountID] = pa
	}

	for _, m := range mortgages {
		acc, ok := linkedByPlaidID[m.AccountID]
		if !ok || acc.AccountType != "home" {
			continue
		}
		s.syncMortgageLiability(ctx, item.UserID, acc, m, balanceByPlaidID[m.AccountID], result)
	}

	for _, cc := range creditCardLiabilities {
		acc, ok := linkedByPlaidID[cc.AccountID]
		if !ok || acc.AccountType != "credit_card" {
			continue
		}
		s.syncCreditCardLiability(ctx, item.UserID, acc, cc, balanceByPlaidID[cc.AccountID], result)
	}
}

// syncMortgageLiability upserts Plaid's mortgage terms into the linked home
// account's home_details via get-then-merge, so fields Plaid can't know
// (address, extra_monthly_payment, track_mortgage_as_liability,
// include_in_retirement_calcs) are preserved. loan_term_years is left as-is —
// Plaid's loan_term is free text (e.g. "30-year"), not reliably parseable.
// It then recomputes equity the same way home_handler.go's computeEquity does
// and writes it as the account's balance snapshot: a home account's balance
// must be equity (zestimate - loan balance), never the raw liability amount.
func (s *Service) syncMortgageLiability(
	ctx context.Context,
	userID uuid.UUID,
	acc models.Account,
	m MortgageLiability,
	pa Account,
	result *SyncResult,
) {
	params := db.UpsertHomeParams{
		AccountID:     acc.ID,
		UserID:        userID,
		LoanTermYears: 30,
	}
	existing, err := s.home.Get(ctx, acc.ID, userID)
	if err == nil && existing != nil {
		params.Address = existing.Address
		params.LoanTermYears = existing.LoanTermYears
		params.OriginalLoanValue = existing.OriginalLoanValue
		params.CurrentLoanBalance = existing.CurrentLoanBalance
		params.InterestRate = existing.InterestRate
		params.MortgageStartDate = existing.MortgageStartDate
		params.ExtraMonthlyPayment = existing.ExtraMonthlyPayment
		params.TrackMortgageAsLiability = existing.TrackMortgageAsLiability
		params.IncludeInRetirementCalcs = existing.IncludeInRetirementCalcs
		params.PropertyTaxAnnual = existing.PropertyTaxAnnual
		params.HomeInsuranceAnnual = existing.HomeInsuranceAnnual
		params.HOAMonthly = existing.HOAMonthly
		params.PMIMonthly = existing.PMIMonthly
		params.PurchasePrice = existing.PurchasePrice
	}

	if m.InterestRatePercentage != nil {
		params.InterestRate = *m.InterestRatePercentage / 100
	}
	if m.OriginationPrincipalAmount != nil {
		params.OriginalLoanValue = *m.OriginationPrincipalAmount
	}
	if m.OriginationDate != nil {
		params.MortgageStartDate = m.OriginationDate
	}
	if pa.CurrentBalance != nil {
		params.CurrentLoanBalance = *pa.CurrentBalance
		params.BalanceIsCurrent = true
	}

	updated, err := s.home.Upsert(ctx, params)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("%s: liabilities sync: %v", acc.Name, err))
		return
	}

	if updated.LastZestimate != nil && *updated.LastZestimate > 0 {
		now := time.Now().UTC()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		equity := *updated.LastZestimate - updated.CurrentLoanBalance
		if _, err := s.snapshots.Upsert(ctx, db.UpsertSnapshotParams{
			AccountID:    acc.ID,
			UserID:       userID,
			SnapshotDate: today,
			Balance:      equity,
			Source:       "computed",
		}); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: equity snapshot: %v", acc.Name, err))
			return
		}
	}
	if err := s.accounts.TouchSynced(ctx, acc.ID, userID); err == nil {
		result.AccountsSynced++
	}
}

// syncCreditCardLiability upserts Plaid's credit-card liability data. Unlike
// home_details, every field here is Plaid-sourced, so no get-then-merge is
// needed — a fresh upsert always reflects Plaid's latest values. The
// account's balance snapshot and TouchSynced/AccountsSynced bookkeeping are
// handled by the generic per-account loop in SyncItem (credit_card accounts
// aren't skipped there, unlike home) — this only writes the extra liability
// detail fields alongside it.
func (s *Service) syncCreditCardLiability(
	ctx context.Context,
	userID uuid.UUID,
	acc models.Account,
	cc CreditCardLiability,
	pa Account,
	result *SyncResult,
) {
	params := db.UpsertCreditCardParams{
		AccountID:              acc.ID,
		UserID:                 userID,
		APRPercentage:          cc.APRPercentage,
		CreditLimit:            pa.Limit,
		LastStatementBalance:   cc.LastStatementBalance,
		LastStatementIssueDate: cc.LastStatementIssueDate,
		MinimumPaymentAmount:   cc.MinimumPaymentAmount,
		NextPaymentDueDate:     cc.NextPaymentDueDate,
		LastPaymentAmount:      cc.LastPaymentAmount,
		LastPaymentDate:        cc.LastPaymentDate,
		IsOverdue:              cc.IsOverdue,
	}
	if _, err := s.creditCards.Upsert(ctx, params); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("%s: liabilities sync: %v", acc.Name, err))
	}
}
