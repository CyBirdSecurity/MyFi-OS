package plaid

import (
	"fmt"
	"strings"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// SubtypeToAccountType maps a Plaid account type/subtype to a MyFi
// account_type and its default tax_treatment.
func SubtypeToAccountType(plaidType, subtype string) (accountType, taxTreatment string) {
	switch strings.ToLower(subtype) {
	case "401k":
		accountType = "traditional_401k"
	case "roth 401k":
		accountType = "roth_401k"
	case "403b", "403(b)":
		accountType = "traditional_403b"
	case "457b":
		accountType = "457b"
	case "ira", "rollover ira", "sep ira", "simple ira", "keogh":
		accountType = "traditional_ira"
	case "roth", "roth ira":
		accountType = "roth_ira"
	case "hsa":
		accountType = "hsa"
	case "529":
		accountType = "529"
	case "brokerage", "non-taxable brokerage account", "mutual fund", "stock plan", "pension", "retirement", "profit sharing plan", "trust", "ugma", "utma":
		accountType = "brokerage"
	case "crypto exchange", "non-custodial wallet":
		accountType = "crypto"
	case "checking", "cash management", "ebt", "paypal", "prepaid":
		accountType = "checking"
	case "savings":
		accountType = "savings"
	case "money market":
		accountType = "money_market"
	case "cd":
		accountType = "savings"
	case "credit card":
		accountType = "credit_card"
	case "mortgage", "home equity":
		// A Plaid mortgage account must attach to an existing MyFi home
		// account — Plaid has no address, so it can never create one.
		// ApplyMappings enforces that at the create-new boundary.
		accountType = "home"
	case "auto", "student", "personal", "commercial", "construction", "consumer", "line of credit", "loan", "overdraft":
		accountType = "loan"
	default:
		switch strings.ToLower(plaidType) {
		case "investment", "brokerage":
			accountType = "brokerage"
		case "depository":
			accountType = "checking"
		case "credit":
			accountType = "credit_card"
		case "loan":
			accountType = "loan"
		default:
			accountType = "other_asset"
		}
	}
	taxTreatment = models.AccountTypeTaxDefaults[accountType]
	if taxTreatment == "" {
		taxTreatment = "taxable"
	}
	return accountType, taxTreatment
}

// SecurityTypeToAssetClass maps a Plaid security type to a MyFi holdings
// asset_class. The holdings.asset_class check constraint only allows
// equity/bond/crypto/cash_equivalent/other (migration 000034 consolidated
// etf and mutual_fund into equity) — this must stay in sync with that set,
// matching the convention already used by the OFX importer
// (internal/services/ofx/institutions/vanguard.go).
func SecurityTypeToAssetClass(plaidSecType string) string {
	switch strings.ToLower(plaidSecType) {
	case "equity", "etf", "mutual fund":
		return "equity"
	case "fixed income":
		return "bond"
	case "cash":
		return "cash_equivalent"
	case "cryptocurrency":
		return "crypto"
	default:
		return "other"
	}
}

// IsLiability reports whether a Plaid account type represents money owed.
// MyFi stores liability balances as negative numbers, while Plaid reports
// a positive "amount owed" — callers must negate.
func IsLiability(plaidType string) bool {
	switch strings.ToLower(plaidType) {
	case "credit", "loan":
		return true
	default:
		return false
	}
}

// IsInvestmentType reports whether a Plaid account type carries holdings.
func IsInvestmentType(plaidType string) bool {
	switch strings.ToLower(plaidType) {
	case "investment", "brokerage":
		return true
	default:
		return false
	}
}

// ResolveTicker picks the ticker to store for a Plaid security and whether the
// holding must be manually priced. Securities without a real trading symbol
// (manualPrice=true) are valued at the institution-reported price and never
// looked up against the live price provider.
func ResolveTicker(sec Security) (ticker string, manualPrice bool) {
	if t := strings.TrimSpace(sec.Ticker); t != "" {
		// Some institutions report symbols with currency suffixes or spaces
		// (e.g. "BTC-USD", "VMFXX**"); keep alphanumerics, dots and dashes.
		return strings.ToUpper(strings.TrimRight(t, "*")), false
	}
	if c := strings.TrimSpace(sec.CUSIP); c != "" {
		return strings.ToUpper(c), true
	}
	id := sec.SecurityID
	if len(id) > 8 {
		id = id[:8]
	}
	return fmt.Sprintf("PLAID-%s", strings.ToUpper(id)), true
}
