package models

import (
	"time"

	"github.com/google/uuid"
)

type BudgetCategory struct {
	ID        uuid.UUID  `db:"id"         json:"id"`
	UserID    *uuid.UUID `db:"user_id"    json:"userId,omitempty"`
	Name      string     `db:"name"       json:"name"`
	IsSystem  bool       `db:"is_system"  json:"isSystem"`
	SortOrder int        `db:"sort_order" json:"sortOrder"`
	CreatedAt time.Time  `db:"created_at" json:"createdAt"`
}

// BudgetLimit is a user's monthly spending limit for one category.
type BudgetLimit struct {
	CategoryID   uuid.UUID `db:"category_id"   json:"categoryId"`
	MonthlyLimit float64   `db:"monthly_limit" json:"monthlyLimit"`
}

type Transaction struct {
	ID             uuid.UUID  `db:"id"              json:"id"`
	UserID         uuid.UUID  `db:"user_id"         json:"userId"`
	TransDate      time.Time  `db:"trans_date"      json:"transDate"`
	Merchant       string     `db:"merchant"        json:"merchant"`
	Amount         float64    `db:"amount"          json:"amount"`
	CategoryID     *uuid.UUID `db:"category_id"     json:"categoryId"`
	CategoryName   string     `db:"-"               json:"categoryName,omitempty"`
	AccountID      *uuid.UUID `db:"account_id"      json:"accountId,omitempty"`
	AccountLabel   *string    `db:"account_label"   json:"accountLabel,omitempty"`
	SourceFile     *string    `db:"source_file"     json:"sourceFile,omitempty"`
	RawDescription string     `db:"raw_description" json:"rawDescription"`
	NeedsReview    bool       `db:"needs_review"    json:"needsReview"`
	Notes          *string    `db:"notes"           json:"notes"`
	// External provider identity for synced (e.g. Plaid) transactions; null for
	// manual/CSV rows. Present so SELECT t.* / RETURNING transactions.* scans
	// don't fail on the columns migration 000054 added.
	ExternalTransactionID *string `db:"external_transaction_id" json:"-"`
	ExternalProvider      *string `db:"external_provider"       json:"-"`
	// Set once the background job has tried to suggest a category.
	AutoCategorizedAt *time.Time `db:"auto_categorized_at" json:"-"`
	CreatedAt         time.Time  `db:"created_at"      json:"createdAt"`
	UpdatedAt         time.Time  `db:"updated_at"      json:"updatedAt"`
}

type BudgetSummaryRow struct {
	CategoryID   uuid.UUID `json:"categoryId"`
	CategoryName string    `json:"categoryName"`
	Total        float64   `json:"total"`
	MonthAvg     float64   `json:"monthAvg"`
	Count        int       `json:"count"`
}

type MonthlyBreakdownRow struct {
	Month        int     `db:"month"         json:"month"`
	CategoryName string  `db:"category_name" json:"categoryName"`
	Total        float64 `db:"total"         json:"total"`
}

type VendorRule struct {
	ID         uuid.UUID `db:"id"`
	Keyword    string    `db:"keyword"`
	CategoryID uuid.UUID `db:"category_id"`
}

// RecurringTransaction is a template that adds a transaction every period
// (monthly or every two weeks) starting on StartDate.
type RecurringTransaction struct {
	ID             uuid.UUID  `db:"id"              json:"id"`
	UserID         uuid.UUID  `db:"user_id"         json:"userId"`
	Merchant       string     `db:"merchant"        json:"merchant"`
	Amount         float64    `db:"amount"          json:"amount"`
	CategoryID     *uuid.UUID `db:"category_id"     json:"categoryId"`
	CategoryName   string     `db:"-"               json:"categoryName,omitempty"`
	AccountID      *uuid.UUID `db:"account_id"      json:"accountId,omitempty"`
	AccountLabel   *string    `db:"account_label"   json:"accountLabel,omitempty"`
	Notes          *string    `db:"notes"           json:"notes"`
	Frequency      string     `db:"frequency"       json:"frequency"`
	StartDate      time.Time  `db:"start_date"      json:"startDate"`
	GeneratedCount int        `db:"generated_count" json:"-"`
	NextDate       time.Time  `db:"-"               json:"nextDate"`
	CreatedAt      time.Time  `db:"created_at"      json:"createdAt"`
}

// Occurrence returns the date of the nth occurrence (0 = StartDate). Monthly
// occurrences keep StartDate's day of month, clamped to the month's last day
// (a payment on the 31st falls on Feb 28), without drifting afterwards.
func (r *RecurringTransaction) Occurrence(n int) time.Time {
	s := r.StartDate
	if r.Frequency == "biweekly" {
		return s.AddDate(0, 0, 14*n)
	}
	firstOfMonth := time.Date(s.Year(), s.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	lastDay := firstOfMonth.AddDate(0, 1, -1).Day()
	day := s.Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(firstOfMonth.Year(), firstOfMonth.Month(), day, 0, 0, 0, 0, time.UTC)
}
