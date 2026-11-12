package plaid

import "strings"

// System budget category names (seeded is_system in migrations 000036/000047).
// Income and Transfer are excluded from spend totals by the budget queries, so
// money movement and paychecks must land here rather than in a spend category.
const (
	CategoryIncome   = "Income"
	CategoryTransfer = "Transfer"
)

// TxnClass is how a Plaid bank transaction should be recorded in the budget.
type TxnClass int

const (
	TxnSpend    TxnClass = iota // an expense, categorised by PFC/rules/history
	TxnIncome                   // a paycheck/deposit → Income system category
	TxnTransfer                 // money movement / card payment → Transfer category
)

// pfcDetailedToCategory maps specific Plaid PFC "detailed" values to MyFi
// categories where the "primary" value is too coarse (e.g. splitting groceries
// out of food & drink). Checked before pfcPrimaryToCategory.
var pfcDetailedToCategory = map[string]string{
	"FOOD_AND_DRINK_GROCERIES":               "Groceries",
	"PERSONAL_CARE_GYMS_AND_FITNESS_CENTERS": "Fitness & Wellness",
	"RENT_AND_UTILITIES_RENT":                "Housing",
	"GENERAL_SERVICES_EDUCATION":             "Education",
	"GENERAL_SERVICES_AUTOMOTIVE":            "Auto",
	"GENERAL_MERCHANDISE_PET_SUPPLIES":       "Pets",
	"LOAN_PAYMENTS_CAR_PAYMENT":              "Auto",
	"LOAN_PAYMENTS_MORTGAGE_PAYMENT":         "Housing",
	"GOVERNMENT_AND_NON_PROFIT_DONATIONS":    "Gifts & Donations",
}

// pfcPrimaryToCategory maps a Plaid PFC "primary" value to a MyFi category name.
// Names must match the system categories seeded in migration 000036.
// Deliberately partial: ambiguous primaries (GENERAL_SERVICES, LOAN_PAYMENTS)
// are left out so they fall through to vendor rules / history / needs_review
// rather than being force-fit into a wrong bucket.
var pfcPrimaryToCategory = map[string]string{
	"FOOD_AND_DRINK":            "Dining & Restaurants",
	"GENERAL_MERCHANDISE":       "Shopping",
	"HOME_IMPROVEMENT":          "Shopping",
	"MEDICAL":                   "Health & Medical",
	"PERSONAL_CARE":             "Personal Care",
	"TRANSPORTATION":            "Transportation",
	"TRAVEL":                    "Travel",
	"ENTERTAINMENT":             "Entertainment",
	"RENT_AND_UTILITIES":        "Utilities",
	"BANK_FEES":                 "Fees & Banking",
	"GOVERNMENT_AND_NON_PROFIT": "Fees & Banking",
}

// PFCCategoryName returns the MyFi spend category name a Plaid PFC maps to, or
// "" when there's no confident mapping (the classifier then falls back to
// vendor rules, merchant history, and finally needs_review).
func PFCCategoryName(primary, detailed string) string {
	if c, ok := pfcDetailedToCategory[strings.ToUpper(detailed)]; ok {
		return c
	}
	if c, ok := pfcPrimaryToCategory[strings.ToUpper(primary)]; ok {
		return c
	}
	return ""
}

// ClassifyPFC decides how a Plaid bank transaction is recorded in the budget,
// from its Personal Finance Category and amount sign. Plaid's amount is
// positive when money leaves the account (spending) and negative when money
// enters (deposits, refunds).
//
// Transfers and credit-card payments are money movement, not spending, so they
// route to Transfer regardless of sign. Any remaining inflow (negative amount)
// or explicit INCOME category is recorded as Income. Everything else is spend,
// with a best-effort category hint. Treating a merchant refund as Income is a
// deliberate simplification: it keeps the refund out of spend totals (which the
// budget only sums over positive amounts) without needing negative-spend math.
func ClassifyPFC(primary, detailed string, amount float64) (class TxnClass, spendHint string) {
	p := strings.ToUpper(primary)
	d := strings.ToUpper(detailed)
	if p == "TRANSFER_IN" || p == "TRANSFER_OUT" || d == "LOAN_PAYMENTS_CREDIT_CARD_PAYMENT" {
		return TxnTransfer, ""
	}
	if p == "INCOME" || amount < 0 {
		return TxnIncome, ""
	}
	return TxnSpend, PFCCategoryName(primary, detailed)
}
