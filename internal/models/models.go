package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type User struct {
	ID                  uuid.UUID      `db:"id"                     json:"id"`
	Email               string         `db:"email"                  json:"email"`
	PasswordHash        string         `db:"password_hash"          json:"-"`
	DisplayName         string         `db:"display_name"           json:"displayName"`
	FailedLoginAttempts int            `db:"failed_login_attempts"  json:"-"`
	LockedUntil         *time.Time     `db:"locked_until"           json:"-"`
	OnboardedAt         *time.Time     `db:"onboarded_at"           json:"onboardedAt"`
	DismissedGuides     pq.StringArray `db:"dismissed_guides"       json:"dismissedGuides"`
	SessionsValidAfter  *time.Time     `db:"sessions_valid_after"   json:"-"`
	CreatedAt           time.Time      `db:"created_at"             json:"createdAt"`
	UpdatedAt           time.Time      `db:"updated_at"             json:"updatedAt"`
}

type ImportLog struct {
	ID         uuid.UUID `db:"id"          json:"id"`
	UserID     uuid.UUID `db:"user_id"     json:"userId"`
	ImportType string    `db:"import_type" json:"importType"`
	Filename   string    `db:"filename"    json:"filename"`
	Status     string    `db:"status"      json:"status"`
	RowCount   int       `db:"row_count"   json:"rowCount"`
	Skipped    int       `db:"skipped"     json:"skipped"`
	ErrorCount int       `db:"error_count" json:"errorCount"`
	ImportedAt time.Time `db:"imported_at" json:"importedAt"`
}

type Account struct {
	ID                 uuid.UUID  `db:"id"                    json:"id"`
	UserID             uuid.UUID  `db:"user_id"               json:"userId"`
	Name               string     `db:"name"                  json:"name"`
	AccountType        string     `db:"account_type"          json:"accountType"`
	TaxTreatment       string     `db:"tax_treatment"         json:"taxTreatment"`
	Institution        *string    `db:"institution"           json:"institution"`
	Currency           string     `db:"currency"              json:"currency"`
	IsManual           bool       `db:"is_manual"             json:"isManual"`
	MemberID           *uuid.UUID `db:"member_id"             json:"memberId,omitempty"`
	IsJoint            bool       `db:"is_joint"              json:"isJoint"`
	ExternalProvider   *string    `db:"external_provider"     json:"externalProvider,omitempty"`
	ExternalAccountID  *string    `db:"external_account_id"   json:"externalAccountId,omitempty"`
	ExternalItemID     *string    `db:"external_item_id"      json:"externalItemId,omitempty"`
	LastSyncedAt       *time.Time `db:"last_synced_at"        json:"lastSyncedAt,omitempty"`
	ReturnRateOverride *float64   `db:"return_rate_override"  json:"returnRateOverride,omitempty"`
	DeletedAt          *time.Time `db:"deleted_at"            json:"-"`
	CreatedAt          time.Time  `db:"created_at"            json:"createdAt"`
	UpdatedAt          time.Time  `db:"updated_at"            json:"updatedAt"`
	// Computed fields — not stored in accounts table; populated by the store layer.
	IsPartnerAccount bool   `db:"-" json:"isPartnerAccount"`
	OwnerName        string `db:"-" json:"ownerName"`
}

// UserProfile holds household-level settings. Per-person data (birthday, income,
// retirement settings) is stored in HouseholdMember and overlaid at query time.
type UserProfile struct {
	ID                     uuid.UUID `db:"id"                      json:"id"`
	UserID                 uuid.UUID `db:"user_id"                 json:"userId"`
	UseInflationAdjustment bool      `db:"use_inflation_adjustment" json:"useInflationAdjustment"`
	InflationRate          float64   `db:"inflation_rate"           json:"inflationRate"`
	MonthlyExpenses        float64   `db:"monthly_expenses"         json:"monthlyExpenses"`
	CurrentMonthlyExpenses float64   `db:"current_monthly_expenses" json:"currentMonthlyExpenses"`
	UseBudgetForExpenses   bool      `db:"use_budget_for_expenses"  json:"useBudgetForExpenses"`
	WithdrawalRate         *float64  `db:"withdrawal_rate"          json:"withdrawalRate,omitempty"`
	FilingStatus           string    `db:"filing_status"            json:"filingStatus"`
	StateTaxRate           *float64  `db:"state_tax_rate"           json:"stateTaxRate,omitempty"`
	StateTaxCode           *string   `db:"state_tax_code"           json:"stateTaxCode,omitempty"`
	DependentsCount        int       `db:"dependents_count"         json:"dependentsCount"`
	CreatedAt              time.Time `db:"created_at"               json:"createdAt"`
	UpdatedAt              time.Time `db:"updated_at"               json:"updatedAt"`

	// Per-person fields — not in DB, overlaid from HouseholdMember by the service layer.
	Birthday               *time.Time `db:"-" json:"birthday,omitempty"`
	RetirementAge          int        `db:"-" json:"retirementAge"`
	UseSocialSecurity      bool       `db:"-" json:"useSocialSecurity"`
	SocialSecurityMonthly  float64    `db:"-" json:"socialSecurityMonthly"`
	SocialSecurityStartAge int        `db:"-" json:"socialSecurityStartAge"`
	MonthlyIncome          float64    `db:"-" json:"monthlyIncome"`
	// HouseholdSize = DependentsCount + member count; populated by service layer.
	HouseholdSize int `db:"-" json:"householdSize"`
}

// HouseholdMember represents one person in the household (self or partner).
type HouseholdMember struct {
	ID                     uuid.UUID  `db:"id"                         json:"id"`
	UserID                 uuid.UUID  `db:"user_id"                    json:"userId"`
	Relationship           string     `db:"relationship"               json:"relationship"`
	Name                   string     `db:"name"                       json:"name"`
	Birthday               *time.Time `db:"birthday"                   json:"birthday,omitempty"`
	IsEmployed             bool       `db:"is_employed"                json:"isEmployed"`
	MonthlyIncome          float64    `db:"monthly_income"             json:"monthlyIncome"`
	SalaryGrowthPct        float64    `db:"salary_growth_pct"          json:"salaryGrowthPct"`
	RetirementAge          int        `db:"retirement_age"             json:"retirementAge"`
	UseSocialSecurity      bool       `db:"use_social_security"        json:"useSocialSecurity"`
	SocialSecurityMonthly  float64    `db:"social_security_monthly"    json:"socialSecurityMonthly"`
	SocialSecurityStartAge int        `db:"social_security_start_age"  json:"socialSecurityStartAge"`
	CreatedAt              time.Time  `db:"created_at"                 json:"createdAt"`
	UpdatedAt              time.Time  `db:"updated_at"                 json:"updatedAt"`
}

type AllocationTarget struct {
	ID        uuid.UUID `db:"id"         json:"id"`
	UserID    uuid.UUID `db:"user_id"    json:"userId"`
	EquityPct float64   `db:"equity_pct" json:"equityPct"`
	BondPct   float64   `db:"bond_pct"   json:"bondPct"`
	CryptoPct float64   `db:"crypto_pct" json:"cryptoPct"`
	CashPct   float64   `db:"cash_pct"   json:"cashPct"`
	OtherPct  float64   `db:"other_pct"  json:"otherPct"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt time.Time `db:"updated_at" json:"updatedAt"`
}

type Contribution struct {
	ID                uuid.UUID  `db:"id"                  json:"id"`
	AccountID         uuid.UUID  `db:"account_id"          json:"accountId"`
	UserID            uuid.UUID  `db:"user_id"             json:"userId"`
	Label             *string    `db:"label"               json:"label"`
	Amount            float64    `db:"amount"              json:"amount"`
	Frequency         string     `db:"frequency"           json:"frequency"`
	AnnualIncreasePct float64    `db:"annual_increase_pct" json:"annualIncreasePct"`
	AmountType        string     `db:"amount_type"         json:"amountType"` // "fixed" (Amount per Frequency) or "percent" (IncomePct of salary)
	IncomePct         float64    `db:"income_pct"          json:"incomePct"`
	MatchTiers        MatchTiers `db:"match_tiers"         json:"matchTiers"`
	IsActive          bool       `db:"is_active"           json:"isActive"`
	NextPayDate       *time.Time `db:"next_pay_date"       json:"nextPayDate"`
	StartDate         *time.Time `db:"start_date"          json:"startDate"`
	EndDate           *time.Time `db:"end_date"            json:"endDate"`
	CreatedAt         time.Time  `db:"created_at"          json:"createdAt"`
	UpdatedAt         time.Time  `db:"updated_at"          json:"updatedAt"`
	// The account owner's salary and yearly raise — not stored; overlaid by
	// the forecast service for percent contributions and match caps.
	OwnerAnnualIncome float64 `db:"-" json:"-"`
	OwnerSalaryGrowth float64 `db:"-" json:"-"`
}

// RetirementAccountTypes are the 401(k)/403(b)/457(b) and IRA account types
// whose actual contributions are tracked against the plan.
var RetirementAccountTypes = map[string]bool{
	"traditional_401k": true, "roth_401k": true,
	"traditional_403b": true, "roth_403b": true, "457b": true,
	"traditional_ira": true, "roth_ira": true, "sep_ira": true, "simple_ira": true,
}

// ContributionEntry is money actually put into an account: synced from Plaid
// (ExternalID set) or logged by hand. Source is "employee" or "employer".
type ContributionEntry struct {
	ID           uuid.UUID `db:"id"            json:"id"`
	AccountID    uuid.UUID `db:"account_id"    json:"accountId"`
	UserID       uuid.UUID `db:"user_id"       json:"userId"`
	EntryDate    time.Time `db:"entry_date"    json:"entryDate"`
	Amount       float64   `db:"amount"        json:"amount"`
	Source       string    `db:"source"        json:"source"`
	Description  *string   `db:"description"   json:"description"`
	ExternalID   *string   `db:"external_id"   json:"-"`
	SourceEdited bool      `db:"source_edited" json:"sourceEdited"`
	CreatedAt    time.Time `db:"created_at"    json:"createdAt"`
	UpdatedAt    time.Time `db:"updated_at"    json:"updatedAt"`
	// IsSynced is true for entries from Plaid, which can't be edited or
	// deleted — only their Source corrected.
	IsSynced bool `db:"-" json:"isSynced"`
}

// MatchTier matches MatchPct of the employee contribution on the next UpToPct
// of salary. A nil UpToPct matches everything that's left; only the last tier
// may have one.
type MatchTier struct {
	MatchPct float64  `json:"matchPct"`
	UpToPct  *float64 `json:"upToPct"`
}

// MatchTiers is stored as a JSONB array.
type MatchTiers []MatchTier

func (t MatchTiers) Value() (driver.Value, error) {
	if t == nil {
		t = MatchTiers{}
	}
	return json.Marshal(t)
}

func (t *MatchTiers) Scan(src any) error {
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, t)
	case string:
		return json.Unmarshal([]byte(v), t)
	case nil:
		*t = nil
		return nil
	}
	return fmt.Errorf("match tiers: unsupported type %T", src)
}

// HasCap reports whether any tier is capped by salary.
func (t MatchTiers) HasCap() bool {
	for _, tier := range t {
		if tier.UpToPct != nil {
			return true
		}
	}
	return false
}

type UserReturnRate struct {
	ID         uuid.UUID `db:"id"          json:"id"`
	UserID     uuid.UUID `db:"user_id"     json:"userId"`
	AssetClass string    `db:"asset_class" json:"assetClass"`
	ReturnRate float64   `db:"return_rate" json:"returnRate"`
	CreatedAt  time.Time `db:"created_at"  json:"createdAt"`
	UpdatedAt  time.Time `db:"updated_at"  json:"updatedAt"`
}

type BalanceSnapshot struct {
	ID           uuid.UUID `db:"id"            json:"id"`
	AccountID    uuid.UUID `db:"account_id"    json:"accountId"`
	UserID       uuid.UUID `db:"user_id"       json:"userId"`
	SnapshotDate time.Time `db:"snapshot_date" json:"snapshotDate"`
	Balance      float64   `db:"balance"       json:"balance"`
	Miles        *int      `db:"miles"         json:"miles,omitempty"`
	Notes        *string   `db:"notes"         json:"notes"`
	Source       string    `db:"source"        json:"source"`
	CreatedAt    time.Time `db:"created_at"    json:"createdAt"`
	UpdatedAt    time.Time `db:"updated_at"    json:"updatedAt"`
}

type Holding struct {
	ID             uuid.UUID  `db:"id"               json:"id"`
	AccountID      uuid.UUID  `db:"account_id"       json:"accountId"`
	UserID         uuid.UUID  `db:"user_id"          json:"userId"`
	Ticker         string     `db:"ticker"           json:"ticker"`
	Name           *string    `db:"name"             json:"name"`
	AssetClass     string     `db:"asset_class"      json:"assetClass"`
	Shares         float64    `db:"shares"           json:"shares"`
	CostBasis      *float64   `db:"cost_basis"       json:"costBasis"`
	ManualPrice    bool       `db:"manual_price"     json:"manualPrice"`
	PricePerShare  *float64   `db:"price_per_share"  json:"pricePerShare"`
	PriceFetchedAt *time.Time `db:"price_fetched_at" json:"priceFetchedAt"`
	DripEnabled    bool       `db:"drip_enabled"     json:"dripEnabled"`
	// The holding's account type, joined in by HoldingStore.ListByUser only.
	AccountType string  `db:"account_type"     json:"-"`
	TotalValue  float64 `db:"-"                json:"totalValue"`
	// From the live quote, not stored: what the ticker is (EQUITY, ETF,
	// MUTUALFUND...) and its full name, for the fund type and size view.
	InstrumentType string    `db:"-" json:"instrumentType,omitempty"`
	QuoteName      string    `db:"-" json:"quoteName,omitempty"`
	CreatedAt      time.Time `db:"created_at"       json:"createdAt"`
	UpdatedAt      time.Time `db:"updated_at"       json:"updatedAt"`
}

// PriceTicker is the symbol to look up this holding's market price under.
// Yahoo lists coins against a currency ("BTC-USD"), and a bare coin symbol
// there can be an unrelated security ("BTC" is a Grayscale ETF). A crypto
// account only holds coins, so its bare symbols get "-USD" added; elsewhere
// the ticker is used as entered, since a crypto ETF like "FBTC" would
// otherwise turn into an unrelated coin.
func (h Holding) PriceTicker() string {
	if h.AccountType == "crypto" && !strings.Contains(h.Ticker, "-") {
		return h.Ticker + "-USD"
	}
	return h.Ticker
}

type HoldingDividend struct {
	ID               uuid.UUID `db:"id"                 json:"id"`
	HoldingID        uuid.UUID `db:"holding_id"         json:"holdingId"`
	UserID           uuid.UUID `db:"user_id"            json:"userId"`
	ExDate           time.Time `db:"ex_date"            json:"exDate"`
	DividendPerShare float64   `db:"dividend_per_share" json:"dividendPerShare"`
	SharesBefore     float64   `db:"shares_before"      json:"sharesBefore"`
	DividendAmount   float64   `db:"dividend_amount"    json:"dividendAmount"`
	DripApplied      bool      `db:"drip_applied"       json:"dripApplied"`
	SharesAdded      *float64  `db:"shares_added"       json:"sharesAdded"`
	PriceUsed        *float64  `db:"price_used"         json:"priceUsed"`
	CreatedAt        time.Time `db:"created_at"         json:"createdAt"`
}

// PlaidItem is one linked Plaid Item (an institution login). The access token
// is stored AES-256-GCM encrypted and is never serialized to JSON.
type PlaidItem struct {
	ID                   uuid.UUID  `db:"id"                     json:"id"`
	UserID               uuid.UUID  `db:"user_id"                json:"userId"`
	ItemID               string     `db:"item_id"                json:"itemId"`
	AccessTokenEncrypted string     `db:"access_token_encrypted" json:"-"`
	InstitutionID        *string    `db:"institution_id"         json:"institutionId,omitempty"`
	InstitutionName      *string    `db:"institution_name"       json:"institutionName,omitempty"`
	Status               string     `db:"status"                 json:"status"`
	ErrorCode            *string    `db:"error_code"             json:"errorCode,omitempty"`
	LastSyncedAt         *time.Time `db:"last_synced_at"         json:"lastSyncedAt,omitempty"`
	LastSyncError        *string    `db:"last_sync_error"        json:"lastSyncError,omitempty"`
	BackfillRequested    bool       `db:"backfill_requested"     json:"backfillRequested"`
	BackfilledAt         *time.Time `db:"backfilled_at"          json:"backfilledAt,omitempty"`
	TransactionsCursor   *string    `db:"transactions_cursor"    json:"-"`
	InvestmentsRefreshAt *time.Time `db:"investments_refresh_at" json:"-"`
	CreatedAt            time.Time  `db:"created_at"             json:"createdAt"`
	UpdatedAt            time.Time  `db:"updated_at"             json:"updatedAt"`
}

type Goal struct {
	ID                uuid.UUID  `db:"id"                  json:"id"`
	UserID            uuid.UUID  `db:"user_id"             json:"userId"`
	Name              string     `db:"name"                json:"name"`
	GoalType          string     `db:"goal_type"           json:"goalType"`
	AccountID         *uuid.UUID `db:"account_id"          json:"accountId,omitempty"`
	AccountTypeFilter *string    `db:"account_type_filter" json:"accountTypeFilter,omitempty"`
	AssetClassFilter  *string    `db:"asset_class_filter"  json:"assetClassFilter,omitempty"`
	TargetAmount      float64    `db:"target_amount"       json:"targetAmount"`
	TargetDate        *time.Time `db:"target_date"         json:"targetDate"`
	Notes             *string    `db:"notes"               json:"notes"`
	IsAchieved        bool       `db:"is_achieved"         json:"isAchieved"`
	CreatedAt         time.Time  `db:"created_at"          json:"createdAt"`
	UpdatedAt         time.Time  `db:"updated_at"          json:"updatedAt"`
}

// DefaultAssetClassStdDevs are baseline annual return standard deviations by asset class
// used for Monte Carlo simulation.
var DefaultAssetClassStdDevs = map[string]float64{
	"equity":          0.15,
	"bond":            0.07,
	"crypto":          0.40,
	"cash_equivalent": 0.01,
	"other":           0.10,
}

// DefaultAssetClassReturnRates are baseline annual return rates by asset class.
var DefaultAssetClassReturnRates = map[string]float64{
	"equity":          0.07,
	"bond":            0.035,
	"crypto":          0.07,
	"cash_equivalent": 0.005,
	"other":           0.03,
}

// AccountTypeDefaultAssetClass maps account types to their representative asset class.
var AccountTypeDefaultAssetClass = map[string]string{
	"checking":         "cash_equivalent",
	"savings":          "cash_equivalent",
	"money_market":     "cash_equivalent",
	"brokerage":        "equity",
	"crypto":           "crypto",
	"home":             "other",
	"other_asset":      "other",
	"automobile":       "other",
	"traditional_ira":  "equity",
	"sep_ira":          "equity",
	"simple_ira":       "equity",
	"traditional_401k": "equity",
	"traditional_403b": "equity",
	"457b":             "equity",
	"roth_ira":         "equity",
	"roth_401k":        "equity",
	"roth_403b":        "equity",
	"hsa":              "equity",
	"529":              "equity",
	"credit_card":      "cash_equivalent",
	"loan":             "cash_equivalent",
	"other_liability":  "cash_equivalent",
}

// AccountTypeTaxDefaults maps each account_type to its default tax_treatment.
var AccountTypeTaxDefaults = map[string]string{
	"checking":         "taxable",
	"savings":          "taxable",
	"money_market":     "taxable",
	"brokerage":        "taxable",
	"crypto":           "taxable",
	"home":             "taxable",
	"other_asset":      "taxable",
	"automobile":       "taxable",
	"credit_card":      "taxable",
	"loan":             "taxable",
	"other_liability":  "taxable",
	"traditional_ira":  "tax_deferred",
	"sep_ira":          "tax_deferred",
	"simple_ira":       "tax_deferred",
	"traditional_401k": "tax_deferred",
	"traditional_403b": "tax_deferred",
	"457b":             "tax_deferred",
	"roth_ira":         "tax_free",
	"roth_401k":        "tax_free",
	"roth_403b":        "tax_free",
	"hsa":              "tax_free",
	"529":              "tax_free",
}

// ValidAccountTypes lists all accepted account_type values.
var ValidAccountTypes = func() map[string]bool {
	m := make(map[string]bool, len(AccountTypeTaxDefaults))
	for k := range AccountTypeTaxDefaults {
		m[k] = true
	}
	return m
}()

// HomeDetails stores all property and loan metadata for home accounts.
type HomeDetails struct {
	ID                       uuid.UUID  `db:"id"                   json:"id"`
	AccountID                uuid.UUID  `db:"account_id"           json:"accountId"`
	UserID                   uuid.UUID  `db:"user_id"              json:"userId"`
	Address                  string     `db:"address"              json:"address"`
	LastZestimate            *float64   `db:"last_zestimate"       json:"lastZestimate,omitempty"`
	LastZestimateAt          *time.Time `db:"last_zestimate_at"    json:"lastZestimateAt,omitempty"`
	InterestRate             float64    `db:"interest_rate"         json:"interestRate"`
	LoanTermYears            int        `db:"loan_term_years"       json:"loanTermYears"`
	OriginalLoanValue        float64    `db:"original_loan_value"   json:"originalLoanValue"`
	CurrentLoanBalance       float64    `db:"current_loan_balance"  json:"currentLoanBalance"`
	MortgageStartDate        *time.Time `db:"mortgage_start_date"            json:"mortgageStartDate,omitempty"`
	ExtraMonthlyPayment      float64    `db:"extra_monthly_payment"          json:"extraMonthlyPayment"`
	TrackMortgageAsLiability bool       `db:"track_mortgage_as_liability"    json:"trackMortgageAsLiability"`
	IncludeInRetirementCalcs bool       `db:"include_in_retirement_calcs"    json:"includeInRetirementCalcs"`
	LoanBalanceAsOf          *time.Time `db:"loan_balance_as_of"             json:"loanBalanceAsOf,omitempty"`
	PropertyTaxAnnual        float64    `db:"property_tax_annual"            json:"propertyTaxAnnual"`
	HomeInsuranceAnnual      float64    `db:"home_insurance_annual"          json:"homeInsuranceAnnual"`
	HOAMonthly               float64    `db:"hoa_monthly"                    json:"hoaMonthly"`
	PMIMonthly               float64    `db:"pmi_monthly"                    json:"pmiMonthly"`
	PurchasePrice            float64    `db:"purchase_price"                 json:"purchasePrice"`
	Equity                   *float64   `db:"-"                              json:"equity,omitempty"`
	// LoanBalanceEstimated is set when CurrentLoanBalance was rolled forward
	// from LoanBalanceAsOf by the payments due since (see RollForwardLoanBalance).
	LoanBalanceEstimated bool      `db:"-" json:"loanBalanceEstimated"`
	CreatedAt            time.Time `db:"created_at"            json:"createdAt"`
	UpdatedAt            time.Time `db:"updated_at"            json:"updatedAt"`
}

// CreditCardDetails stores Plaid liabilities data (or manual entry) for
// credit_card accounts: APR, statement balance, minimum payment, due date,
// and credit limit.
type CreditCardDetails struct {
	ID                     uuid.UUID  `db:"id"                        json:"id"`
	AccountID              uuid.UUID  `db:"account_id"                json:"accountId"`
	UserID                 uuid.UUID  `db:"user_id"                   json:"userId"`
	APRPercentage          *float64   `db:"apr_percentage"            json:"aprPercentage,omitempty"`
	CreditLimit            *float64   `db:"credit_limit"              json:"creditLimit,omitempty"`
	LastStatementBalance   *float64   `db:"last_statement_balance"    json:"lastStatementBalance,omitempty"`
	LastStatementIssueDate *time.Time `db:"last_statement_issue_date" json:"lastStatementIssueDate,omitempty"`
	MinimumPaymentAmount   *float64   `db:"minimum_payment_amount"    json:"minimumPaymentAmount,omitempty"`
	NextPaymentDueDate     *time.Time `db:"next_payment_due_date"     json:"nextPaymentDueDate,omitempty"`
	LastPaymentAmount      *float64   `db:"last_payment_amount"       json:"lastPaymentAmount,omitempty"`
	LastPaymentDate        *time.Time `db:"last_payment_date"         json:"lastPaymentDate,omitempty"`
	IsOverdue              bool       `db:"is_overdue"                json:"isOverdue"`
	CreatedAt              time.Time  `db:"created_at"                json:"createdAt"`
	UpdatedAt              time.Time  `db:"updated_at"                json:"updatedAt"`
}

// LoanDetails stores the interest rate and monthly payment for loan and
// other_liability accounts, used to project their payoff in the forecast.
type LoanDetails struct {
	ID             uuid.UUID `db:"id"              json:"id"`
	AccountID      uuid.UUID `db:"account_id"      json:"accountId"`
	UserID         uuid.UUID `db:"user_id"         json:"userId"`
	APRPercentage  *float64  `db:"apr_percentage"  json:"aprPercentage,omitempty"`
	MonthlyPayment *float64  `db:"monthly_payment" json:"monthlyPayment,omitempty"`
	CreatedAt      time.Time `db:"created_at"      json:"createdAt"`
	UpdatedAt      time.Time `db:"updated_at"      json:"updatedAt"`
}

// VehicleDetails stores vehicle-specific metadata for automobile accounts.
type VehicleDetails struct {
	ID           uuid.UUID  `db:"id"            json:"id"`
	AccountID    uuid.UUID  `db:"account_id"    json:"accountId"`
	UserID       uuid.UUID  `db:"user_id"       json:"userId"`
	VehicleYear  int        `db:"vehicle_year"  json:"vehicleYear"`
	VehicleMake  string     `db:"vehicle_make"  json:"vehicleMake"`
	VehicleModel string     `db:"vehicle_model" json:"vehicleModel"`
	VehicleTrim  *string    `db:"vehicle_trim"  json:"vehicleTrim,omitempty"`
	LastValue    *float64   `db:"last_value"    json:"lastValue,omitempty"`
	LastValueAt  *time.Time `db:"last_value_at" json:"lastValueAt,omitempty"`
	CreatedAt    time.Time  `db:"created_at"    json:"createdAt"`
	UpdatedAt    time.Time  `db:"updated_at"    json:"updatedAt"`
}
