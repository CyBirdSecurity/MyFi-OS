package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// ForecastYear holds the projected portfolio state for a single calendar year.
type ForecastYear struct {
	Year      int                `json:"year"`
	Age       int                `json:"age"`
	SpouseAge *int               `json:"spouseAge,omitempty"`
	Accounts  map[string]float64 `json:"accounts"` // accountID (string) -> balance
	Total     float64            `json:"total"`
	// Shortfall is retirement spending the portfolio couldn't cover this
	// year; any shortfall means the money ran out.
	Shortfall float64 `json:"-"`
	// Monte Carlo percentile bands — only populated when bands are requested.
	P10 float64 `json:"p10,omitempty"`
	P25 float64 `json:"p25,omitempty"`
	P50 float64 `json:"p50,omitempty"`
	P75 float64 `json:"p75,omitempty"`
	P90 float64 `json:"p90,omitempty"`
}

// AccountMeta carries display info needed by the chart legend.
type AccountMeta struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	AccountType           string `json:"accountType"`
	OwnerUserID           string `json:"ownerUserId"`
	CountsTowardPortfolio bool   `json:"countsTowardPortfolio"`
}

// FIMetrics holds computed financial independence metrics.
type FIMetrics struct {
	FINumber             float64  `json:"fiNumber"`
	CurrentPortfolio     float64  `json:"currentPortfolio"`
	FIProgressPct        float64  `json:"fiProgressPct"`
	TimeToFIYear         *int     `json:"timeToFIYear"`
	TimeToFIAge          *int     `json:"timeToFIAge"`
	CoastFIAmount        float64  `json:"coastFIAmount"`
	IsCoastFI            bool     `json:"isCoastFI"`
	SavingsRate          *float64 `json:"savingsRate"`
	WithdrawalRate       *float64 `json:"withdrawalRate"`
	WithdrawalRateWarn   bool     `json:"withdrawalRateWarn"`
	MonteCarloSuccessPct float64  `json:"monteCarloSuccessPct"`
	// HasMonteCarlo is set once MonteCarloSuccessPct holds a real result,
	// which can be 0%.
	HasMonteCarlo bool `json:"-"`
	// FINumberGrowth is how fast the FI number, in today's dollars, grows in
	// the projection's dollars each year: inflation when the forecast is
	// nominal, 0 when it's inflation-adjusted.
	FINumberGrowth float64 `json:"fiNumberGrowth"`
	// ProjectedRetirementPortfolio is the base-case portfolio in the primary
	// member's retirement year (today's portfolio if that year has passed).
	ProjectedRetirementPortfolio float64 `json:"projectedRetirementPortfolio"`
}

// ForecastResult is the full response for the /forecast endpoint.
type ForecastResult struct {
	Base                 []ForecastYear  `json:"base"`
	Optimistic           []ForecastYear  `json:"optimistic"`
	Pessimistic          []ForecastYear  `json:"pessimistic"`
	AccountMeta          []AccountMeta   `json:"accountMeta"`
	RetirementYear       int             `json:"retirementYear"`
	SpouseRetirementYear *int            `json:"spouseRetirementYear,omitempty"`
	FIMetrics            *FIMetrics      `json:"fiMetrics,omitempty"`
	ReadinessScore       *ReadinessScore `json:"readinessScore,omitempty"`
	MortgagePayoffYears  map[string]int  `json:"mortgagePayoffYears,omitempty"`
}

// ─── Readiness Score ──────────────────────────────────────────────────────────

// ScoreComponent is one weighted sub-score that feeds into ReadinessScore.
type ScoreComponent struct {
	Name    string  `json:"name"`
	Score   int     `json:"score"`
	Weight  float64 `json:"weight"`
	Grade   string  `json:"grade"`
	Insight string  `json:"insight"`
}

// ReadinessScore is a synthesised 0-100 retirement readiness score.
type ReadinessScore struct {
	Overall    int              `json:"overall"`
	Grade      string           `json:"grade"`
	Components []ScoreComponent `json:"components"`
	Headline   string           `json:"headline"`
}

// ─── Retirement Plan ──────────────────────────────────────────────────────────

// DrawdownYear tracks the portfolio balance during the post-retirement
// decumulation phase for a single calendar year.
type DrawdownYear struct {
	Year    int     `json:"year"`
	Age     int     `json:"age"`
	Balance float64 `json:"balance"`
}

// RetirementScenario compares three standard withdrawal-rate strategies.
type RetirementScenario struct {
	Label         string  `json:"label"`
	Rate          float64 `json:"rate"`
	MonthlyIncome float64 `json:"monthlyIncome"`
	DepletionAge  *int    `json:"depletionAge"` // nil → survives to 100
}

// RetirementPlan is the full response for GET /retirement.
type RetirementPlan struct {
	RetirementAge        int                  `json:"retirementAge"`
	RetirementYear       int                  `json:"retirementYear"`
	RetirementPortfolio  float64              `json:"retirementPortfolio"`
	SustainableMonthly   float64              `json:"sustainableMonthly"`
	MonthlySpending      float64              `json:"monthlySpending"`
	AnnualSocialSecurity float64              `json:"annualSocialSecurity"`
	DepletionAge         *int                 `json:"depletionAge"`
	SurvivesToHundred    bool                 `json:"survivesToHundred"`
	DrawdownSchedule     []DrawdownYear       `json:"drawdownSchedule"`
	RateScenarios        []RetirementScenario `json:"rateScenarios"`
}

// ─── Scenario Builder ─────────────────────────────────────────────────────────

// ScenarioParams defines overrides applied on top of the user's actual data
// for a single what-if scenario.
// ScenarioParams describes one what-if scenario as absolute target values —
// e.g. "retire at 62" or "return rate of 6%" — rather than deltas from the
// current plan. RunScenarios computes the delta against CurrentPlanSummary
// internally before applying it to the simulation. A nil field means "keep
// the current plan's value for this lever."
type ScenarioParams struct {
	Name                string   `json:"name"`
	RetirementAge       *int     `json:"retirementAge"`
	MonthlyContribution *float64 `json:"monthlyContribution"`
	ReturnRate          *float64 `json:"returnRate"`
	MonthlyExpense      *float64 `json:"monthlyExpense"`
}

// ScenarioResult is the output for a single scenario run.
type ScenarioResult struct {
	Name                  string         `json:"name"`
	Years                 []ForecastYear `json:"years"`
	RetirementYear        int            `json:"retirementYear"`
	FIYear                *int           `json:"fiYear"`
	FIAge                 *int           `json:"fiAge"`
	PortfolioAtRetirement float64        `json:"portfolioAtRetirement"`
	SustainableMonthly    float64        `json:"sustainableMonthly"`
}

// ScenariosResult wraps the baseline and all user-defined scenarios.
type ScenariosResult struct {
	Baseline    ScenarioResult     `json:"baseline"`
	Scenarios   []ScenarioResult   `json:"scenarios"`
	AccountMeta []AccountMeta      `json:"accountMeta"`
	CurrentPlan CurrentPlanSummary `json:"currentPlan"`
}

// CurrentPlanSummary surfaces the inputs driving the "Current Plan" baseline
// so the Scenarios page can show them without sending the user to Settings.
type CurrentPlanSummary struct {
	RetirementAge       int     `json:"retirementAge"`
	ReturnRate          float64 `json:"returnRate"`          // portfolio-weighted blended rate across all accounts
	MonthlyContribution float64 `json:"monthlyContribution"` // sum of active contributions, normalized to a monthly amount
	MonthlyExpense      float64 `json:"monthlyExpense"`
}

// ─── Shared input bundle ──────────────────────────────────────────────────────

// forecastInputs holds all data loaded from the database that is shared
// across Calculate, ComputeRetirementPlan, and RunScenarios.
type forecastInputs struct {
	profile              *models.UserProfile
	spouseProfile        *models.UserProfile
	allAccounts          []models.Account
	startingBalances     map[uuid.UUID]float64
	effectiveRates       map[uuid.UUID]float64
	allContribs          []models.Contribution
	allHoldings          []models.Holding
	homeDetails          map[uuid.UUID]*models.HomeDetails
	accountMeta          []AccountMeta
	primaryBirthYear     int
	spouseBirthYear      *int
	currentYear          int
	endYear              int
	retirementYear       int
	spouseRetirementYear *int
	spendGoals           []models.Goal
	liabilities          map[uuid.UUID]liabilityTerms
}

// ForecastService runs portfolio projection simulations.
type ForecastService struct {
	accounts         *db.AccountStore
	snapshots        *db.SnapshotStore
	holdings         *db.HoldingStore
	profiles         *db.ProfileStore
	contributions    *db.ContributionStore
	returnRates      *db.ReturnRateStore
	householdMembers *db.HouseholdMembersStore
	home             *db.HomeStore
	transactions     *db.TransactionStore
	goals            *db.GoalStore
	creditCards      *db.CreditCardStore
	loans            *db.LoanStore
}

func NewForecastService(
	accounts *db.AccountStore,
	snapshots *db.SnapshotStore,
	holdings *db.HoldingStore,
	profiles *db.ProfileStore,
	contributions *db.ContributionStore,
	returnRates *db.ReturnRateStore,
	householdMembers *db.HouseholdMembersStore,
	home *db.HomeStore,
	goals *db.GoalStore,
) *ForecastService {
	return &ForecastService{
		accounts:         accounts,
		snapshots:        snapshots,
		holdings:         holdings,
		profiles:         profiles,
		contributions:    contributions,
		returnRates:      returnRates,
		householdMembers: householdMembers,
		home:             home,
		goals:            goals,
	}
}

// SetTransactionStore enables budget-derived expense calculation.
func (fs *ForecastService) SetTransactionStore(ts *db.TransactionStore) {
	fs.transactions = ts
}

// SetLiabilityStores enables projecting credit card and loan payoff from
// their stored interest rate and payment.
func (fs *ForecastService) SetLiabilityStores(cc *db.CreditCardStore, loans *db.LoanStore) {
	fs.creditCards = cc
	fs.loans = loans
}

// homeSimState tracks the mutable per-year state of a home account during simulation.
// Home equity is projected by growing the home value at the appreciation rate and
// reducing the loan balance via a standard fixed-rate amortization schedule.
type homeSimState struct {
	homeValue           float64 // current estimated property value (grows at appreciation rate)
	loanBalance         float64 // remaining mortgage principal
	monthlyPayment      float64 // fixed monthly P+I payment (constant for life of loan)
	monthlyRate         float64 // interest_rate / 12
	extraMonthlyPayment float64 // additional principal reduction per month
}

// buildHomeSimState initialises a homeSimState for the simulation.
// Returns nil when there is no active mortgage to amortise.
func buildHomeSimState(hd *models.HomeDetails, startingEquity float64) *homeSimState {
	if hd.InterestRate <= 0 || hd.LoanTermYears <= 0 || hd.CurrentLoanBalance <= 0 {
		return nil // no mortgage, or already paid off
	}

	remaining := remainingLoanMonths(hd)
	if remaining <= 0 {
		return nil // loan term already elapsed
	}

	monthlyRate := hd.InterestRate / 12

	// Monthly payment: P × r / (1 − (1+r)^−n)
	// Uses current balance and remaining term so it's correct regardless of whether
	// the user has made extra payments or provided an original loan amount.
	var monthlyPayment float64
	if monthlyRate > 0 {
		monthlyPayment = hd.CurrentLoanBalance * monthlyRate /
			(1 - math.Pow(1+monthlyRate, -float64(remaining)))
	} else {
		// 0% interest: divide principal evenly across remaining months
		monthlyPayment = hd.CurrentLoanBalance / float64(remaining)
	}

	// Home value: last_zestimate if available, otherwise back-calculate from equity + balance.
	homeValue := startingEquity + hd.CurrentLoanBalance
	if hd.LastZestimate != nil && *hd.LastZestimate > 0 {
		homeValue = *hd.LastZestimate
	}

	return &homeSimState{
		homeValue:           homeValue,
		loanBalance:         hd.CurrentLoanBalance,
		monthlyPayment:      monthlyPayment,
		monthlyRate:         monthlyRate,
		extraMonthlyPayment: hd.ExtraMonthlyPayment,
	}
}

// remainingLoanMonths computes how many monthly payments are left on the mortgage.
//
// Priority:
//  1. MortgageStartDate: elapsed months subtracted from total term.
//  2. OriginalLoanValue: algebraically derive remaining months from the original
//     monthly payment and the current balance.
//  3. Fallback: treat the full term as remaining (pessimistic, slightly overstates
//     paydown speed when used in forecast).
func remainingLoanMonths(hd *models.HomeDetails) int {
	return remainingLoanMonthsAt(hd, time.Now())
}

// remainingLoanMonthsAt is remainingLoanMonths as of a given date.
func remainingLoanMonthsAt(hd *models.HomeDetails, now time.Time) int {
	total := hd.LoanTermYears * 12

	if hd.MortgageStartDate != nil {
		start := *hd.MortgageStartDate
		elapsed := (now.Year()-start.Year())*12 + int(now.Month()) - int(start.Month())
		if elapsed < 0 {
			elapsed = 0
		}
		remaining := total - elapsed
		if remaining <= 0 {
			return 0
		}
		return remaining
	}

	if hd.OriginalLoanValue > 0 && hd.InterestRate > 0 {
		r := hd.InterestRate / 12
		n := float64(total)
		// Original monthly payment
		M := hd.OriginalLoanValue * r * math.Pow(1+r, n) / (math.Pow(1+r, n) - 1)
		if M > hd.CurrentLoanBalance*r {
			// n_remaining = −ln(1 − P·r / M) / ln(1+r)
			remaining := -math.Log(1-hd.CurrentLoanBalance*r/M) / math.Log(1+r)
			if remaining > 0 {
				return int(math.Ceil(remaining))
			}
		}
	}

	return total // fallback: assume full term remaining
}

// countsTowardPortfolio reports whether an account's balance should be
// included in FI Number / Forecast / Retirement portfolio totals: the money
// that can actually fund retirement. Illiquid assets (cars, other property)
// and debts never count, and a home only counts when its HomeDetails opt in
// via IncludeInRetirementCalcs. All of them still appear in Net Worth and the
// Accounts page, which read balances independently of this function.
func countsTowardPortfolio(acc models.Account, homeDetails map[uuid.UUID]*models.HomeDetails) bool {
	switch acc.AccountType {
	case "home":
		hd, ok := homeDetails[acc.ID]
		return ok && hd.IncludeInRetirementCalcs
	case "automobile", "other_asset":
		return false
	}
	return !isLiabilityAccount(acc)
}

// isLiabilityAccount reports whether an account's balance is money owed,
// stored as a negative balance.
func isLiabilityAccount(acc models.Account) bool {
	switch acc.AccountType {
	case "credit_card", "loan", "other_liability":
		return true
	}
	return false
}

// liabilityTerms is how the forecast pays down a debt account each month.
type liabilityTerms struct {
	monthlyRate    float64 // APR / 12
	monthlyPayment float64
}

// loadLiabilityTerms reads each debt's interest rate and monthly payment: a
// credit card's from its card details (the last payment made, else the
// minimum due), a loan's from its loan details. A debt with no payment on
// record is left out and held at its current balance.
func (s *ForecastService) loadLiabilityTerms(ctx context.Context, accounts []models.Account) map[uuid.UUID]liabilityTerms {
	terms := make(map[uuid.UUID]liabilityTerms)
	for _, acc := range accounts {
		var apr, payment *float64
		switch acc.AccountType {
		case "credit_card":
			if s.creditCards == nil {
				continue
			}
			d, err := s.creditCards.Get(ctx, acc.ID, acc.UserID)
			if err != nil {
				continue
			}
			apr, payment = d.APRPercentage, d.MinimumPaymentAmount
			if d.LastPaymentAmount != nil && *d.LastPaymentAmount > 0 {
				payment = d.LastPaymentAmount
			}
		case "loan", "other_liability":
			if s.loans == nil {
				continue
			}
			d, err := s.loans.Get(ctx, acc.ID, acc.UserID)
			if err != nil {
				continue
			}
			apr, payment = d.APRPercentage, d.MonthlyPayment
		default:
			continue
		}
		if payment == nil || *payment <= 0 {
			continue
		}
		t := liabilityTerms{monthlyPayment: *payment}
		if apr != nil && *apr > 0 {
			t.monthlyRate = *apr / 100 / 12
		}
		terms[acc.ID] = t
	}
	return terms
}

// isDrainableAccount reports whether an account can serve as a source of
// funds for retirement withdrawals and spend-goal deductions. Physical
// assets (homes, vehicles, other assets) and liabilities can't be sold off
// incrementally to cover annual spending, so simulated withdrawals must
// never touch them even when their tax_treatment matches a drain tier.
func isDrainableAccount(acc models.Account) bool {
	switch acc.AccountType {
	case "home", "automobile", "other_asset", "credit_card", "loan", "other_liability":
		return false
	}
	return true
}

// ErrBirthdayRequired means the user hasn't entered their date of birth yet,
// which every projection needs. It's an incomplete profile, not a failure.
var ErrBirthdayRequired = errors.New("birthday not set — please complete your household profile before viewing projections")

// loadForecastInputs loads all database data needed to run any forecast
// computation. It is shared by Calculate, ComputeRetirementPlan, and RunScenarios.
func (s *ForecastService) loadForecastInputs(ctx context.Context, userID uuid.UUID) (*forecastInputs, error) {
	profile, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	if profile == nil {
		profile = &models.UserProfile{UserID: userID, InflationRate: 0.03, FilingStatus: "single"}
	}

	// Load household members and overlay per-person data onto profile.
	members, _ := s.householdMembers.ListByUser(ctx, userID)
	var selfMember, partnerMember *models.HouseholdMember
	for i := range members {
		switch members[i].Relationship {
		case "self":
			selfMember = &members[i]
		case "partner":
			partnerMember = &members[i]
		}
	}

	if selfMember == nil || selfMember.Birthday == nil {
		return nil, ErrBirthdayRequired
	}

	// Populate self-member per-person fields into the shared profile object.
	profile.Birthday = selfMember.Birthday
	profile.RetirementAge = selfMember.RetirementAge
	profile.UseSocialSecurity = selfMember.UseSocialSecurity
	profile.SocialSecurityMonthly = selfMember.SocialSecurityMonthly
	profile.SocialSecurityStartAge = selfMember.SocialSecurityStartAge
	for _, m := range members {
		if m.IsEmployed {
			profile.MonthlyIncome += m.MonthlyIncome
		}
	}
	if profile.RetirementAge == 0 {
		profile.RetirementAge = 65
	}
	if profile.SocialSecurityStartAge == 0 {
		profile.SocialSecurityStartAge = 67
	}

	// Replace manual monthly expenses with 3-month budget average when opted in.
	if profile.UseBudgetForExpenses && s.transactions != nil {
		if avg, err := s.transactions.MonthlyAvgExpenses(ctx, userID); err == nil && avg > 0 {
			profile.MonthlyExpenses = avg
		}
	}

	primaryBirthYear := profile.Birthday.Year()
	currentYear := time.Now().Year()
	currentAge := currentYear - primaryBirthYear
	endYear := currentYear + (100 - currentAge)
	// Guard against bad birthday data (e.g. currentAge > 100), which would
	// otherwise make endYear < currentYear and panic downstream when sizing
	// per-year result slices.
	if endYear < currentYear {
		endYear = currentYear
	}

	// Build spouse profile from partner member if present.
	var spouseProfile *models.UserProfile
	var spouseBirthYear *int
	if partnerMember != nil && partnerMember.Birthday != nil {
		y := partnerMember.Birthday.Year()
		spouseBirthYear = &y
		spouseProfile = &models.UserProfile{
			Birthday:               partnerMember.Birthday,
			RetirementAge:          partnerMember.RetirementAge,
			UseSocialSecurity:      partnerMember.UseSocialSecurity,
			SocialSecurityMonthly:  partnerMember.SocialSecurityMonthly,
			SocialSecurityStartAge: partnerMember.SocialSecurityStartAge,
		}
		if spouseProfile.RetirementAge == 0 {
			spouseProfile.RetirementAge = 65
		}
		if spouseProfile.SocialSecurityStartAge == 0 {
			spouseProfile.SocialSecurityStartAge = 67
		}
	}

	allAccounts, err := s.accounts.List(ctx, userID)
	if err != nil {
		return nil, err
	}

	startingBalances, err := s.computeStartingBalances(ctx, userID, allAccounts)
	if err != nil {
		return nil, err
	}

	userRates, _ := s.returnRates.ListByUser(ctx, userID)
	rateMap := buildRateMap(userRates)

	allHoldings, err := s.holdings.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	effectiveRates := s.computeEffectiveRates(allAccounts, allHoldings, rateMap)

	allContribs, _ := s.contributions.ListByUser(ctx, userID)
	OverlayOwnerIncome(allContribs, allAccounts, members)

	homeDetails := make(map[uuid.UUID]*models.HomeDetails)
	for _, acc := range allAccounts {
		if acc.AccountType == "home" {
			if hd, hdErr := s.home.Get(ctx, acc.ID, acc.UserID); hdErr == nil {
				RollForwardLoanBalance(hd, time.Now())
				homeDetails[acc.ID] = hd
				// Start from live equity, as net worth does.
				if hd.LastZestimate != nil && *hd.LastZestimate > 0 {
					startingBalances[acc.ID] = *hd.LastZestimate - hd.CurrentLoanBalance
				}
			}
		}
	}

	accountMeta := make([]AccountMeta, 0, len(allAccounts))
	for _, acc := range allAccounts {
		accountMeta = append(accountMeta, AccountMeta{
			ID:                    acc.ID.String(),
			Name:                  acc.Name,
			AccountType:           acc.AccountType,
			OwnerUserID:           acc.UserID.String(),
			CountsTowardPortfolio: countsTowardPortfolio(acc, homeDetails),
		})
	}

	retirementYear := primaryBirthYear + profile.RetirementAge
	var spouseRetirementYear *int
	if spouseProfile != nil && spouseBirthYear != nil {
		ry := *spouseBirthYear + spouseProfile.RetirementAge
		spouseRetirementYear = &ry
	}

	var spendGoals []models.Goal
	if s.goals != nil {
		allGoals, _ := s.goals.List(ctx, userID)
		for _, g := range allGoals {
			if g.GoalType == "spend" && !g.IsAchieved && g.TargetDate != nil {
				spendGoals = append(spendGoals, g)
			}
		}
	}

	return &forecastInputs{
		liabilities:          s.loadLiabilityTerms(ctx, allAccounts),
		profile:              profile,
		spouseProfile:        spouseProfile,
		allAccounts:          allAccounts,
		startingBalances:     startingBalances,
		effectiveRates:       effectiveRates,
		allContribs:          allContribs,
		allHoldings:          allHoldings,
		homeDetails:          homeDetails,
		accountMeta:          accountMeta,
		primaryBirthYear:     primaryBirthYear,
		spouseBirthYear:      spouseBirthYear,
		currentYear:          currentYear,
		endYear:              endYear,
		retirementYear:       retirementYear,
		spouseRetirementYear: spouseRetirementYear,
		spendGoals:           spendGoals,
	}, nil
}

// Calculate produces a full forecast for userID through age 100.
func (s *ForecastService) Calculate(ctx context.Context, userID uuid.UUID) (*ForecastResult, error) {
	inp, err := s.loadForecastInputs(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := &ForecastResult{
		AccountMeta:          inp.accountMeta,
		RetirementYear:       inp.retirementYear,
		SpouseRetirementYear: inp.spouseRetirementYear,
	}

	result.Base = s.simulate(
		inp.allAccounts, inp.startingBalances, inp.allContribs, inp.effectiveRates, inp.homeDetails,
		inp.profile, inp.spouseProfile, inp.currentYear, inp.endYear,
		inp.primaryBirthYear, inp.spouseBirthYear, 0, inp.spendGoals, inp.liabilities,
	)

	result.Optimistic = s.simulate(
		inp.allAccounts, inp.startingBalances, inp.allContribs, inp.effectiveRates, inp.homeDetails,
		inp.profile, inp.spouseProfile, inp.currentYear, inp.endYear,
		inp.primaryBirthYear, inp.spouseBirthYear, 0.02, inp.spendGoals, inp.liabilities,
	)
	result.Pessimistic = s.simulate(
		inp.allAccounts, inp.startingBalances, inp.allContribs, inp.effectiveRates, inp.homeDetails,
		inp.profile, inp.spouseProfile, inp.currentYear, inp.endYear,
		inp.primaryBirthYear, inp.spouseBirthYear, -0.02, inp.spendGoals, inp.liabilities,
	)

	totalBalance := 0.0
	for _, acc := range inp.allAccounts {
		if countsTowardPortfolio(acc, inp.homeDetails) {
			totalBalance += inp.startingBalances[acc.ID]
		}
	}
	result.FIMetrics = s.computeFIMetrics(
		inp.profile, inp.effectiveRates, inp.allAccounts, inp.startingBalances,
		totalBalance, result.Base, inp.primaryBirthYear, inp.homeDetails,
	)

	classWeights := computeAssetClassWeights(inp.allAccounts, inp.allHoldings)

	mcArgs := [3]struct {
		rateAdj float64
		years   []ForecastYear
	}{
		{0, result.Base},
		{0.02, result.Optimistic},
		{-0.02, result.Pessimistic},
	}

	// The three scenarios share only read-only inputs and each seeds its own
	// rng, so they run in parallel with identical results.
	var mcSuccess [3]float64
	var mcBands [3][]mcBand
	var wg sync.WaitGroup
	for si, arg := range mcArgs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mcSuccess[si], mcBands[si] = s.runMonteCarlo(
				inp.allAccounts, inp.startingBalances, inp.allContribs, inp.effectiveRates, classWeights, inp.homeDetails,
				inp.profile, inp.spouseProfile, inp.currentYear, inp.endYear,
				inp.primaryBirthYear, inp.spouseBirthYear, inp.spendGoals,
				arg.rateAdj, inp.liabilities,
			)
		}()
	}
	wg.Wait()

	for si, arg := range mcArgs {
		bands := mcBands[si]
		if si == 0 {
			result.FIMetrics.MonteCarloSuccessPct = mcSuccess[si]
			result.FIMetrics.HasMonteCarlo = true
		}
		for i := range arg.years {
			if i < len(bands) {
				arg.years[i].P10 = bands[i].P10
				arg.years[i].P25 = bands[i].P25
				arg.years[i].P50 = bands[i].P50
				arg.years[i].P75 = bands[i].P75
				arg.years[i].P90 = bands[i].P90
			}
		}
	}

	result.ReadinessScore = computeReadinessScore(result.FIMetrics, inp.retirementYear, inp.currentYear)

	for accID, hd := range inp.homeDetails {
		sched := BuildAmortizationSchedule(hd, hd.ExtraMonthlyPayment)
		if len(sched.Schedule) == 0 {
			continue
		}
		lastRow := sched.Schedule[len(sched.Schedule)-1]
		t, err := time.Parse("2006-01-02", lastRow.Date)
		if err != nil {
			continue
		}
		if result.MortgagePayoffYears == nil {
			result.MortgagePayoffYears = make(map[string]int)
		}
		result.MortgagePayoffYears[accID.String()] = t.Year()
	}

	return result, nil
}

// ComputeRetirementPlan shows how the portfolio holds up in retirement at a
// given monthly spending (today's dollars; 0 uses the profile's). It runs the
// same simulation as the forecast, so both pages always agree.
func (s *ForecastService) ComputeRetirementPlan(ctx context.Context, userID uuid.UUID, monthlySpending float64) (*RetirementPlan, error) {
	inp, err := s.loadForecastInputs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if monthlySpending <= 0 {
		monthlySpending = effectiveRetirementIncome(inp.profile)
	}
	run := func(monthly float64) []ForecastYear {
		profileCopy := *inp.profile
		profileCopy.MonthlyExpenses = monthly
		return s.simulate(
			inp.allAccounts, inp.startingBalances, inp.allContribs, inp.effectiveRates, inp.homeDetails,
			&profileCopy, inp.spouseProfile, inp.currentYear, inp.endYear,
			inp.primaryBirthYear, inp.spouseBirthYear, 0, inp.spendGoals, inp.liabilities,
		)
	}

	years := run(monthlySpending)
	retirementPortfolio := portfolioAtRetirement(years, inp.retirementYear, 0)
	schedule, depletionAge := drawdown(years, inp.retirementYear, inp.profile.RetirementAge, retirementPortfolio)
	yearsToRetirement := inp.retirementYear - inp.currentYear

	// The benchmark rates are a share of the portfolio at retirement, turned
	// into today's dollars to spend.
	rateScenarios := []RetirementScenario{
		{Label: "Conservative (4%)", Rate: 0.04},
		{Label: "Moderate (5%)", Rate: 0.05},
		{Label: "Aggressive (6%)", Rate: 0.06},
	}
	for i, rs := range rateScenarios {
		monthly := retirementPortfolio * rs.Rate / 12 / todayDollarFactor(inp.profile, yearsToRetirement)
		rateScenarios[i].MonthlyIncome = monthly
		_, rateScenarios[i].DepletionAge = drawdown(run(monthly), inp.retirementYear, inp.profile.RetirementAge, retirementPortfolio)
	}

	// Social Security the plan counts, in today's dollars, from each
	// person's claiming age.
	annualSS := 0.0
	for _, p := range []*models.UserProfile{inp.profile, inp.spouseProfile} {
		if p != nil && p.UseSocialSecurity {
			annualSS += p.SocialSecurityMonthly * 12
		}
	}

	return &RetirementPlan{
		RetirementAge:        inp.profile.RetirementAge,
		RetirementYear:       inp.retirementYear,
		RetirementPortfolio:  retirementPortfolio,
		SustainableMonthly:   sustainableMonthly(inp.profile, retirementPortfolio, yearsToRetirement),
		MonthlySpending:      monthlySpending,
		AnnualSocialSecurity: annualSS,
		DepletionAge:         depletionAge,
		SurvivesToHundred:    depletionAge == nil,
		DrawdownSchedule:     schedule,
		RateScenarios:        rateScenarios,
	}, nil
}

// drawdown picks the retirement years out of a simulation: the balance at
// the start of each year of retirement through age 100, and the age spending
// first goes unmet (nil when it never does).
func drawdown(years []ForecastYear, retirementYear, retirementAge int, atRetirement float64) ([]DrawdownYear, *int) {
	schedule := []DrawdownYear{{Year: retirementYear, Age: retirementAge, Balance: atRetirement}}
	var depletionAge *int
	for _, y := range years {
		if y.Year < retirementYear {
			continue
		}
		// A year's closing balance is what the next year starts with.
		if y.Age+1 <= 100 {
			schedule = append(schedule, DrawdownYear{Year: y.Year + 1, Age: y.Age + 1, Balance: y.Total})
		}
		if depletionAge == nil && y.Shortfall > 0.5 {
			a := y.Age
			depletionAge = &a
		}
	}
	return schedule, depletionAge
}

// portfolioAtRetirement is the projected portfolio on the day the primary
// retires: the balance at the end of their last working year. The simulated
// retirement year itself already has a year of withdrawals taken out, so
// reading it would count that spending twice in anything that then draws the
// portfolio down. When there's no working year left to project (retiring this
// year), the retirement year is used, and fallback when neither is projected.
func portfolioAtRetirement(years []ForecastYear, retirementYear int, fallback float64) float64 {
	result := fallback
	for _, y := range years {
		if y.Year == retirementYear-1 {
			return y.Total
		}
		if y.Year == retirementYear {
			result = y.Total
		}
	}
	return result
}

// RunScenarios evaluates up to 3 what-if scenarios against the user's actual data.
func (s *ForecastService) RunScenarios(ctx context.Context, userID uuid.UUID, params []ScenarioParams) (*ScenariosResult, error) {
	if len(params) > 3 {
		return nil, fmt.Errorf("maximum 3 scenarios allowed")
	}

	inp, err := s.loadForecastInputs(ctx, userID)
	if err != nil {
		return nil, err
	}

	currentPlan := CurrentPlanSummary{
		RetirementAge:       inp.profile.RetirementAge,
		ReturnRate:          portfolioWeightedRate(inp.allAccounts, inp.startingBalances, inp.effectiveRates, inp.homeDetails),
		MonthlyContribution: totalMonthlyContributions(inp.allContribs),
		MonthlyExpense:      effectiveRetirementIncome(inp.profile),
	}

	runOne := func(p *ScenarioParams, name string) ScenarioResult {
		profileCopy := *inp.profile
		if p != nil && p.RetirementAge != nil {
			profileCopy.RetirementAge = *p.RetirementAge
		}
		if p != nil && p.MonthlyExpense != nil {
			target := *p.MonthlyExpense
			if target < 0 {
				target = 0
			}
			profileCopy.MonthlyExpenses = target
		}
		retirementYear := inp.primaryBirthYear + profileCopy.RetirementAge

		contribs := inp.allContribs
		if p != nil && p.MonthlyContribution != nil {
			contributionDelta := *p.MonthlyContribution - currentPlan.MonthlyContribution
			if contributionDelta != 0 {
				cloned := make([]models.Contribution, len(inp.allContribs))
				copy(cloned, inp.allContribs)
				for i := range cloned {
					if cloned[i].IsActive {
						addMonthlyContribution(&cloned[i], contributionDelta)
					}
				}
				contribs = cloned
			}
		}

		rateAdj := 0.0
		if p != nil && p.ReturnRate != nil {
			rateAdj = *p.ReturnRate - currentPlan.ReturnRate
		}

		years := s.simulate(
			inp.allAccounts, inp.startingBalances, contribs, inp.effectiveRates, inp.homeDetails,
			&profileCopy, inp.spouseProfile, inp.currentYear, inp.endYear,
			inp.primaryBirthYear, inp.spouseBirthYear, rateAdj, inp.spendGoals, inp.liabilities,
		)

		fiYear, fiAge := findFICrossing(years, fiNumberForProfile(&profileCopy), &profileCopy)

		atRetirement := portfolioAtRetirement(years, retirementYear, 0)

		return ScenarioResult{
			Name:                  name,
			Years:                 years,
			RetirementYear:        retirementYear,
			FIYear:                fiYear,
			FIAge:                 fiAge,
			PortfolioAtRetirement: atRetirement,
			SustainableMonthly:    sustainableMonthly(&profileCopy, atRetirement, retirementYear-inp.currentYear),
		}
	}

	baseline := runOne(nil, "Current Plan")

	results := make([]ScenarioResult, 0, len(params))
	for _, p := range params {
		pp := p
		results = append(results, runOne(&pp, pp.Name))
	}

	return &ScenariosResult{
		Baseline:    baseline,
		Scenarios:   results,
		AccountMeta: inp.accountMeta,
		CurrentPlan: currentPlan,
	}, nil
}

// ─── Readiness Score computation ──────────────────────────────────────────────

// computeReadinessScore grades how on track the user is to retire on their
// own timeline. It's driven by the projection, not today's balance: the main
// component is how much of the FI number the base forecast has in place by
// the retirement year, so a long runway is never penalized and a plan that's
// projected to reach FI in time scores well on its own. Withdrawal safety and
// savings rate refine it.
func computeReadinessScore(fi *FIMetrics, retirementYear, currentYear int) *ReadinessScore {
	if fi == nil || fi.FINumber <= 0 {
		return nil
	}

	grade := func(s int) string {
		switch {
		case s >= 90:
			return "A"
		case s >= 75:
			return "B"
		case s >= 60:
			return "C"
		case s >= 45:
			return "D"
		default:
			return "F"
		}
	}

	// Component 1: On Track — share of the FI number projected to be in
	// place at retirement (full marks once the projection reaches it).
	// The FI number is in today's dollars; grow it to the retirement year's.
	fiAtRetirement := fi.FINumber * math.Pow(1+fi.FINumberGrowth, float64(max(retirementYear-currentYear, 0)))
	projectedPct := fi.ProjectedRetirementPortfolio / fiAtRetirement * 100
	if retirementYear <= currentYear {
		projectedPct = fi.FIProgressPct
	}
	trackScore := int(math.Min(math.Max(projectedPct, 0), 100))
	var trackInsight string
	switch {
	case fi.TimeToFIYear != nil && *fi.TimeToFIYear <= retirementYear:
		yearsEarly := retirementYear - *fi.TimeToFIYear
		when := "right at your retirement goal"
		if yearsEarly > 0 {
			when = fmt.Sprintf("%d year(s) before your retirement goal", yearsEarly)
		}
		trackInsight = fmt.Sprintf("Projected to reach your FI number at age %d, %s (%.0f%% saved today).",
			*fi.TimeToFIAge, when, fi.FIProgressPct)
	case fi.TimeToFIYear != nil:
		trackInsight = fmt.Sprintf("Projected to have %.0f%% of your FI number at retirement; FI reached %d year(s) later.",
			projectedPct, *fi.TimeToFIYear-retirementYear)
	default:
		trackInsight = fmt.Sprintf("Projected to have %.0f%% of your FI number at retirement — consider increasing contributions or adjusting your plan.",
			projectedPct)
	}

	// Component 2: Withdrawal Safety.
	safetyScore := 50 // default when no data
	var safetyInsight string
	if fi.HasMonteCarlo {
		safetyScore = int(fi.MonteCarloSuccessPct)
		safetyInsight = fmt.Sprintf("%.0f%% Monte Carlo success rate across 500 simulations.", fi.MonteCarloSuccessPct)
	} else if fi.WithdrawalRate != nil {
		wr := *fi.WithdrawalRate * 100
		switch {
		case wr <= 3.5:
			safetyScore = 95
			safetyInsight = fmt.Sprintf("%.1f%% withdrawal rate — very sustainable.", wr)
		case wr <= 4.0:
			safetyScore = 80
			safetyInsight = fmt.Sprintf("%.1f%% withdrawal rate — within the safe 4%% guideline.", wr)
		case wr <= 4.5:
			safetyScore = 65
			safetyInsight = fmt.Sprintf("%.1f%% withdrawal rate — slightly above the 4%% guideline.", wr)
		case wr <= 5.0:
			safetyScore = 45
			safetyInsight = fmt.Sprintf("%.1f%% withdrawal rate — consider reducing spending or retiring later.", wr)
		default:
			safetyScore = 25
			safetyInsight = fmt.Sprintf("%.1f%% withdrawal rate — high risk of portfolio depletion.", wr)
		}
	} else {
		safetyInsight = "Set a retirement income target in Settings to calculate withdrawal safety."
	}

	// Component 3: Savings Rate. Lightly weighted: once the projection
	// reaches FI, saving more matters less.
	savingsScore := 0
	var savingsInsight string
	if fi.SavingsRate != nil {
		pct := *fi.SavingsRate * 100
		savingsScore = int(math.Min((*fi.SavingsRate/0.20)*100, 100))
		switch {
		case pct >= 20:
			savingsInsight = fmt.Sprintf("%.1f%% savings rate — excellent, at or above the 20%% target.", pct)
		case pct >= 15:
			savingsInsight = fmt.Sprintf("%.1f%% savings rate — good, close to the 20%% target.", pct)
		case pct >= 10:
			savingsInsight = fmt.Sprintf("%.1f%% savings rate — consider increasing toward 20%%.", pct)
		default:
			savingsInsight = fmt.Sprintf("%.1f%% savings rate — boosting this will significantly improve your outlook.", pct)
		}
	} else {
		savingsInsight = "Add income and expenses to your profile to calculate your savings rate."
	}

	components := []ScoreComponent{
		{Name: "On Track", Score: trackScore, Weight: 0.65, Grade: grade(trackScore), Insight: trackInsight},
		{Name: "Withdrawal Safety", Score: safetyScore, Weight: 0.25, Grade: grade(safetyScore), Insight: safetyInsight},
		{Name: "Savings Rate", Score: savingsScore, Weight: 0.10, Grade: grade(savingsScore), Insight: savingsInsight},
	}

	overall := 0.0
	for _, c := range components {
		overall += float64(c.Score) * c.Weight
	}
	overallInt := int(math.Round(overall))

	var headline string
	switch {
	case overallInt >= 90:
		headline = "You're on track — your projection reaches your FI number by your retirement goal."
	case overallInt >= 75:
		headline = "Good shape — close to fully on track for your retirement goal."
	case overallInt >= 60:
		headline = "You're on your way — meaningful improvements would get you on track."
	case overallInt >= 45:
		headline = "Some gaps to address — increasing savings or adjusting your timeline will help."
	default:
		headline = "Behind your goal for now — every step compounds significantly over time."
	}

	return &ReadinessScore{
		Overall:    overallInt,
		Grade:      grade(overallInt),
		Components: components,
		Headline:   headline,
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// simulate is a convenience wrapper around simulateCore for deterministic scenarios.
func (s *ForecastService) simulate(
	accounts []models.Account,
	startingBalances map[uuid.UUID]float64,
	contributions []models.Contribution,
	effectiveRates map[uuid.UUID]float64,
	homeDetails map[uuid.UUID]*models.HomeDetails,
	profile *models.UserProfile,
	spouseProfile *models.UserProfile,
	startYear, endYear int,
	primaryBirthYear int,
	spouseBirthYear *int,
	rateAdj float64,
	spendGoals []models.Goal,
	liabilities map[uuid.UUID]liabilityTerms,
) []ForecastYear {
	contribsByAccount := make(map[uuid.UUID][]models.Contribution)
	for _, c := range contributions {
		if c.IsActive {
			contribsByAccount[c.AccountID] = append(contribsByAccount[c.AccountID], c)
		}
	}
	return s.simulateCore(
		accounts, startingBalances, contribsByAccount,
		func(id uuid.UUID, _ int) float64 {
			r := effectiveRates[id] + rateAdj
			// Match the floor used by the Monte Carlo path (a single year can't
			// lose more than 100% of value) instead of flooring at 0%, which
			// would silently hide real downside for low-yield assets in the
			// Pessimistic scenario.
			if r < -1.0 {
				r = -1.0
			}
			return r
		},
		homeDetails,
		profile, spouseProfile, startYear, endYear, primaryBirthYear, spouseBirthYear,
		spendGoals, liabilities,
	)
}

// simulateCore runs the year-by-year projection. getRate is called once per account
// per year to get its annual return rate before inflation adjustment; yearIndex is
// the zero-based offset from startYear, letting callers (e.g. a historical
// backtest) look up a rate from a specific point in a return sequence rather
// than a stateless per-account constant.
// For home accounts with an active mortgage, amortization is applied instead of a
// simple return rate: the home value appreciates at the account's return rate while
// the loan principal is paid down via a fixed monthly payment schedule.
func (s *ForecastService) simulateCore(
	accounts []models.Account,
	startingBalances map[uuid.UUID]float64,
	contribsByAccount map[uuid.UUID][]models.Contribution,
	getRate func(id uuid.UUID, yearIndex int) float64,
	homeDetails map[uuid.UUID]*models.HomeDetails,
	profile *models.UserProfile,
	spouseProfile *models.UserProfile,
	startYear, endYear int,
	primaryBirthYear int,
	spouseBirthYear *int,
	spendGoals []models.Goal,
	liabilities map[uuid.UUID]liabilityTerms,
) []ForecastYear {
	balances := make(map[uuid.UUID]float64, len(startingBalances))
	for k, v := range startingBalances {
		balances[k] = v
	}

	// Initialise amortization state for home accounts.
	homeStates := make(map[uuid.UUID]*homeSimState)
	for _, acc := range accounts {
		if acc.AccountType == "home" {
			if hd, ok := homeDetails[acc.ID]; ok {
				if state := buildHomeSimState(hd, startingBalances[acc.ID]); state != nil {
					homeStates[acc.ID] = state
				}
			}
		}
	}

	// Debts are paid down month by month in nominal dollars (a loan payment
	// doesn't rise with inflation); owed holds what's left on each. The
	// payments come out of income, like the mortgage's, not the portfolio.
	owed := make(map[uuid.UUID]float64)
	for _, acc := range accounts {
		if isLiabilityAccount(acc) && startingBalances[acc.ID] < 0 {
			owed[acc.ID] = -startingBalances[acc.ID]
		}
	}
	// deflator is cumulative inflation since today, used to express the
	// nominal debts in today's dollars when inflation adjustment is on.
	deflator := 1.0

	// The starting balances reflect the portfolio value as of today, which already
	// includes year-to-date growth and contributions for the current year. To avoid
	// double-counting, only apply the remaining fraction of year 1 when the
	// simulation starts in the current calendar year.
	now := time.Now()
	firstYearFraction := 1.0
	if startYear == now.Year() {
		yearEnd := time.Date(startYear+1, 1, 1, 0, 0, 0, 0, time.UTC)
		daysInYear := yearEnd.Sub(time.Date(startYear, 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24
		daysRemaining := yearEnd.Sub(now).Hours() / 24
		firstYearFraction = daysRemaining / daysInYear
		if firstYearFraction < 0 {
			firstYearFraction = 0
		} else if firstYearFraction > 1 {
			firstYearFraction = 1
		}
	}

	var result []ForecastYear

	for year := startYear; year <= endYear; year++ {
		primaryAge := year - primaryBirthYear
		yearIndex := year - startYear

		yearFraction := 1.0
		if year == startYear {
			yearFraction = firstYearFraction
		}
		if profile.UseInflationAdjustment {
			deflator *= math.Pow(1+profile.InflationRate, yearFraction)
		}

		// The window of `year` still being simulated — from `now` through Dec 31
		// for the first (partial) year, or the full calendar year otherwise.
		// Contributions are prorated against this window by actual start/end
		// date rather than the coarser per-year yearFraction, so a contribution
		// that starts or ends mid-year isn't counted for months it wasn't active.
		windowStart := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		if year == startYear && startYear == now.Year() {
			windowStart = now
		}
		windowEnd := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)

		var spouseAge *int
		if spouseBirthYear != nil {
			a := year - *spouseBirthYear
			spouseAge = &a
		}

		primaryRetired := primaryAge >= profile.RetirementAge

		spouseRetired := false
		if spouseProfile != nil && spouseAge != nil {
			spouseRetired = *spouseAge >= spouseProfile.RetirementAge
		}

		// Apply return and contributions to each account.
		for _, acc := range accounts {
			if isLiabilityAccount(acc) {
				debt, ok := owed[acc.ID]
				if !ok {
					continue // nothing owed: leave the balance as it is
				}
				if t, ok := liabilities[acc.ID]; ok {
					months := int(math.Round(yearFraction * 12))
					for m := 0; m < months && debt > 0; m++ {
						debt += debt*t.monthlyRate - t.monthlyPayment
					}
					if debt < 0 {
						debt = 0
					}
					owed[acc.ID] = debt
				}
				balances[acc.ID] = -debt / deflator
				continue
			}

			// Home accounts with an active mortgage use amortization logic.
			if acc.AccountType == "home" {
				if state, ok := homeStates[acc.ID]; ok {
					// Grow home value at the property appreciation rate.
					appreciationRate := getRate(acc.ID, yearIndex)
					if profile.UseInflationAdjustment {
						appreciationRate = (1+appreciationRate)/(1+profile.InflationRate) - 1
					}
					state.homeValue *= math.Pow(1+appreciationRate, yearFraction)

					// Pay down mortgage principal for the remaining months of the year.
					paydownMonths := int(math.Round(yearFraction * 12))
					for m := 0; m < paydownMonths && state.loanBalance > 0; m++ {
						interest := state.loanBalance * state.monthlyRate
						principal := state.monthlyPayment - interest
						if principal <= 0 {
							// Negative amortisation guard — shouldn't happen with valid data.
							break
						}
						extra := state.extraMonthlyPayment
						if principal+extra > state.loanBalance {
							extra = math.Max(0, state.loanBalance-principal)
						}
						state.loanBalance -= principal + extra
						if state.loanBalance < 0 {
							state.loanBalance = 0
						}
					}

					equity := state.homeValue - state.loanBalance
					if equity < 0 {
						equity = 0
					}
					balances[acc.ID] = equity
					continue
				}
			}

			balance := balances[acc.ID]

			rate := getRate(acc.ID, yearIndex)
			if profile.UseInflationAdjustment {
				rate = (1+rate)/(1+profile.InflationRate) - 1
			}
			balance *= math.Pow(1+rate, yearFraction)

			ownerRetired := primaryRetired
			if spouseProfile != nil && acc.IsPartnerAccount {
				ownerRetired = spouseRetired
			}

			if !ownerRetired {
				for _, c := range contribsByAccount[acc.ID] {
					contribStartYear := startYear
					if c.StartDate != nil {
						contribStartYear = c.StartDate.Year()
					}

					contribFraction := activeFraction(windowStart, windowEnd, c.StartDate, c.EndDate)
					if contribFraction <= 0 {
						continue
					}

					employee, match := annualContribution(c, yearIndex, year-contribStartYear)
					annualBase := employee + match
					// In real-dollar mode the projection is expressed in today's
					// dollars, so a fixed nominal contribution buys less each
					// year — deflate it by cumulative inflation rather than
					// silently treating it as inflation-indexed. Users who do
					// plan to raise contributions with inflation can model that
					// via AnnualIncreasePct.
					if profile.UseInflationAdjustment {
						annualBase /= math.Pow(1+profile.InflationRate, float64(yearIndex))
					}
					balance += annualBase * contribFraction
				}
			}

			balances[acc.ID] = balance
		}

		// Spending, Social Security and planned purchases are entered in
		// today's dollars; inflate them to this year's dollars when the
		// projection is in nominal dollars.
		toYearDollars := todayDollarFactor(profile, yearIndex)

		// This year's retirement spending need, net of Social Security.
		need := 0.0
		if primaryRetired {
			need = effectiveRetirementIncome(profile)*12 - socialSecurityForYear(profile, primaryBirthYear, year)
			if spouseBirthYear != nil {
				need -= socialSecurityForYear(spouseProfile, *spouseBirthYear, year)
			}
			need = math.Max(0, need) * toYearDollars
		}

		// Step 1: Required Minimum Distributions from tax_deferred accounts.
		// RMDs are a legal, per-owner age-based requirement, so they apply to a
		// spouse-owned account once the spouse reaches RMD age even if the
		// primary user hasn't retired yet — unlike the income-need withdrawals
		// below, which are tied to the primary's own retirement timeline.
		totalRMD := 0.0
		rmdByAccount := make(map[uuid.UUID]float64)
		for _, acc := range accounts {
			if acc.TaxTreatment != "tax_deferred" {
				continue
			}
			ownerAge, ownerBirthYear := primaryAge, primaryBirthYear
			if spouseProfile != nil && spouseAge != nil && acc.IsPartnerAccount {
				ownerAge, ownerBirthYear = *spouseAge, *spouseBirthYear
			}
			if ownerAge < rmdStartAge(ownerBirthYear) {
				continue
			}
			bal := balances[acc.ID]
			if bal <= 0 {
				continue
			}
			rmd := math.Min(bal/rmdDistributionPeriod(ownerAge), bal)
			balances[acc.ID] -= rmd
			rmdByAccount[acc.ID] = rmd
			totalRMD += rmd
		}
		// An RMD beyond what's spent is still the household's money: it's
		// reinvested in a taxable account, or left where it was when there's
		// none to move it to.
		if excess := totalRMD - need; excess > 0 {
			reinvestRMDExcess(accounts, balances, rmdByAccount, totalRMD, excess)
		}

		// Step 2: Withdrawals for the need RMDs didn't cover, in tax-aware
		// order: taxable → tax_deferred → tax_free. Non-drainable accounts
		// (see isDrainableAccount) are excluded even though they default to
		// "taxable" — vehicles and other physical assets can't fund annual
		// spending. Homes additionally have their balance recomputed every
		// year from homeStates (appreciation minus amortization) above,
		// unconditionally overwriting balances[acc.ID] — so any withdrawal
		// taken from home equity here would silently reappear next year
		// instead of actually reducing the portfolio.
		shortfall := drainByTier(accounts, balances, math.Max(0, need-totalRMD))

		// Apply one-time spending goal deductions for this year.
		for _, sg := range spendGoals {
			if sg.TargetDate.Year() != year {
				continue
			}
			if sg.TargetAmount > 0 {
				drainByTier(accounts, balances, sg.TargetAmount*toYearDollars)
			}
		}

		yearBalances := make(map[string]float64, len(accounts))
		total := 0.0
		for _, acc := range accounts {
			b := math.Round(balances[acc.ID]*100) / 100
			if b < 0 && !isLiabilityAccount(acc) {
				b = 0
			}
			yearBalances[acc.ID.String()] = b
			if countsTowardPortfolio(acc, homeDetails) {
				total += b
			}
		}

		result = append(result, ForecastYear{
			Year:      year,
			Age:       primaryAge,
			SpouseAge: spouseAge,
			Accounts:  yearBalances,
			Total:     math.Round(total*100) / 100,
			Shortfall: shortfall,
		})
	}

	return result
}

// todayDollarFactor converts today's dollars into the dollars of a year
// yearsOut from now: 1 when the projection is inflation-adjusted (already in
// today's dollars), otherwise the cumulative inflation over those years.
func todayDollarFactor(profile *models.UserProfile, yearsOut int) float64 {
	if profile.UseInflationAdjustment || yearsOut <= 0 {
		return 1
	}
	return math.Pow(1+profile.InflationRate, float64(yearsOut))
}

// drainByTier withdraws amount from drainable accounts, taxable first, then
// tax_deferred, then tax_free, pro rata within each tier. Returns what it
// couldn't cover.
func drainByTier(accounts []models.Account, balances map[uuid.UUID]float64, amount float64) float64 {
	for _, tier := range []string{"taxable", "tax_deferred", "tax_free"} {
		if amount <= 0 {
			break
		}
		tierTotal := 0.0
		for _, acc := range accounts {
			if acc.TaxTreatment == tier && isDrainableAccount(acc) {
				if b := balances[acc.ID]; b > 0 {
					tierTotal += b
				}
			}
		}
		if tierTotal <= 0 {
			continue
		}
		tierWithdrawal := math.Min(amount, tierTotal)
		for _, acc := range accounts {
			if acc.TaxTreatment != tier || !isDrainableAccount(acc) {
				continue
			}
			if b := balances[acc.ID]; b > 0 {
				balances[acc.ID] = b - math.Min(tierWithdrawal*(b/tierTotal), b)
			}
		}
		amount -= tierWithdrawal
	}
	return math.Max(0, amount)
}

// reinvestRMDExcess puts the part of this year's RMDs that wasn't spent into
// the drainable taxable accounts, pro rata by balance (all into the first one
// when none has a balance yet). With no taxable account to receive it, the
// excess goes back to the accounts it came from.
func reinvestRMDExcess(accounts []models.Account, balances map[uuid.UUID]float64, rmdByAccount map[uuid.UUID]float64, totalRMD, excess float64) {
	var taxable []models.Account
	taxableTotal := 0.0
	for _, acc := range accounts {
		if acc.TaxTreatment == "taxable" && isDrainableAccount(acc) {
			taxable = append(taxable, acc)
			taxableTotal += math.Max(0, balances[acc.ID])
		}
	}
	switch {
	case len(taxable) == 0:
		for id, rmd := range rmdByAccount {
			balances[id] += excess * rmd / totalRMD
		}
	case taxableTotal <= 0:
		balances[taxable[0].ID] += excess
	default:
		for _, acc := range taxable {
			balances[acc.ID] += excess * math.Max(0, balances[acc.ID]) / taxableTotal
		}
	}
}

// rmdStartAge is the age RMDs begin under SECURE 2.0: 75 for anyone born in
// 1960 or later, 73 otherwise.
func rmdStartAge(birthYear int) int {
	if birthYear >= 1960 {
		return 75
	}
	return 73
}

// socialSecurityForYear returns p's annual Social Security benefit in year,
// or 0 before they start claiming it or if they don't use it.
func socialSecurityForYear(p *models.UserProfile, birthYear, year int) float64 {
	if p == nil || !p.UseSocialSecurity {
		return 0
	}
	startAge := p.SocialSecurityStartAge
	if startAge <= 0 {
		startAge = p.RetirementAge
	}
	if year < birthYear+startAge {
		return 0
	}
	return p.SocialSecurityMonthly * 12
}

// rmdUniformLifetimeTable is the IRS Uniform Lifetime Table distribution periods by age.
var rmdUniformLifetimeTable = map[int]float64{
	73: 26.5, 74: 25.5, 75: 24.6, 76: 23.7, 77: 22.9,
	78: 22.0, 79: 21.1, 80: 20.2, 81: 19.4, 82: 18.5,
	83: 17.7, 84: 16.8, 85: 16.0, 86: 15.2, 87: 14.4,
	88: 13.7, 89: 12.9, 90: 12.2, 91: 11.5, 92: 10.8,
	93: 10.1, 94: 9.5, 95: 8.9, 96: 8.4, 97: 7.8,
	98: 7.3, 99: 6.8, 100: 6.4, 101: 6.0, 102: 5.6,
	103: 5.2, 104: 4.9, 105: 4.6, 106: 4.3, 107: 4.1,
	108: 3.9, 109: 3.7, 110: 3.5, 111: 3.4, 112: 3.3,
	113: 3.1, 114: 3.0, 115: 2.9, 116: 2.8, 117: 2.7,
	118: 2.5, 119: 2.3, 120: 2.0,
}

// rmdDistributionPeriod returns the IRS Uniform Lifetime distribution period for an age.
func rmdDistributionPeriod(age int) float64 {
	if p, ok := rmdUniformLifetimeTable[age]; ok {
		return p
	}
	if age > 120 {
		return 2.0
	}
	return 26.5
}

// shockForAccount converts one year's per-class shocks (standard-normal draws
// shared by every account) into an account-level return deviation by scaling
// each class's draw by its std dev and the account's weight in that class.
// varianceForAccount is the variance of an account's yearly return shock
// (shockForAccount draws are independent per asset class).
func varianceForAccount(weights map[string]float64) float64 {
	var variance float64
	for class, w := range weights {
		sd := models.DefaultAssetClassStdDevs[class]
		if sd == 0 {
			sd = models.DefaultAssetClassStdDevs["other"]
		}
		variance += (w * sd) * (w * sd)
	}
	return variance
}

// volatilityDrag is the amount to add to an expected return `rate` so random
// yearly returns with the given variance compound, at the median, at `rate`
// itself: E[ln(1+r)] ≈ ln(1+mean) − σ²/(2(1+mean)²). Without it the Monte
// Carlo median falls well below the deterministic forecast, so Withdrawal
// Safety would judge a gloomier plan than the one the user sees.
func volatilityDrag(rate, variance float64) float64 {
	// Solve ln(x) − σ²/(2x²) = ln(1+rate) for x = 1+mean by fixed-point
	// iteration; it converges in a few steps for any realistic σ.
	target := 1 + rate
	x := target
	for range 20 {
		x = target * math.Exp(variance/(2*x*x))
	}
	return x - target
}

func shockForAccount(weights, shocks map[string]float64) float64 {
	var shock float64
	for class, w := range weights {
		sd := models.DefaultAssetClassStdDevs[class]
		if sd == 0 {
			sd = models.DefaultAssetClassStdDevs["other"]
		}
		shock += w * sd * shocks[class]
	}
	return shock
}

// mcBand holds the portfolio total percentiles for a single calendar year.
type mcBand struct {
	P10, P25, P50, P75, P90 float64
}

// runMonteCarlo runs 500 simulations with normally distributed returns
// (seed 42) and returns:
//   - the percentage of runs where the portfolio survives to endYear
//   - per-year P10/P25/P50/P75/P90 portfolio totals
//
// Randomness is drawn per asset class per year rather than per account:
// every account exposed to the same class shares that class's shock (scaled
// by its weight, see shockForAccount), so two accounts holding the same asset
// class move together instead of artificially diversifying each other.
func (s *ForecastService) runMonteCarlo(
	accounts []models.Account,
	startingBalances map[uuid.UUID]float64,
	contributions []models.Contribution,
	effectiveRates map[uuid.UUID]float64,
	classWeights map[uuid.UUID]map[string]float64,
	homeDetails map[uuid.UUID]*models.HomeDetails,
	profile *models.UserProfile,
	spouseProfile *models.UserProfile,
	startYear, endYear int,
	primaryBirthYear int,
	spouseBirthYear *int,
	spendGoals []models.Goal,
	rateAdj float64,
	liabilities map[uuid.UUID]liabilityTerms,
) (float64, []mcBand) {
	const numSims = 500
	rng := rand.New(rand.NewSource(42))

	contribsByAccount := make(map[uuid.UUID][]models.Contribution)
	for _, c := range contributions {
		if c.IsActive {
			contribsByAccount[c.AccountID] = append(contribsByAccount[c.AccountID], c)
		}
	}

	numYears := endYear - startYear + 1
	// allTotals[yearIdx][simIdx] = portfolio total
	allTotals := make([][]float64, numYears)
	for i := range allTotals {
		allTotals[i] = make([]float64, 0, numSims)
	}

	// Sorted union of asset classes across all accounts so the rng draw order
	// is deterministic for the fixed seed.
	classSet := make(map[string]struct{})
	for _, w := range classWeights {
		for class := range w {
			classSet[class] = struct{}{}
		}
	}
	classes := make([]string, 0, len(classSet))
	for c := range classSet {
		classes = append(classes, c)
	}
	sort.Strings(classes)

	variances := make(map[uuid.UUID]float64, len(classWeights))
	for id, w := range classWeights {
		variances[id] = varianceForAccount(w)
	}

	successes := 0
	for i := 0; i < numSims; i++ {
		// One shared standard-normal draw per asset class per year.
		yearShocks := make([]map[string]float64, numYears)
		for y := range yearShocks {
			shocks := make(map[string]float64, len(classes))
			for _, class := range classes {
				shocks[class] = rng.NormFloat64()
			}
			yearShocks[y] = shocks
		}

		years := s.simulateCore(
			accounts, startingBalances, contribsByAccount,
			func(id uuid.UUID, yearIndex int) float64 {
				mean := effectiveRates[id] + rateAdj
				r := mean + volatilityDrag(mean, variances[id]) + shockForAccount(classWeights[id], yearShocks[yearIndex])
				if r < -1.0 {
					r = -1.0
				}
				return r
			},
			homeDetails,
			profile, spouseProfile, startYear, endYear, primaryBirthYear, spouseBirthYear,
			spendGoals, liabilities,
		)
		for yi, y := range years {
			if yi < numYears {
				allTotals[yi] = append(allTotals[yi], y.Total)
			}
		}
		if spendingMet(years) {
			successes++
		}
	}

	bands := make([]mcBand, numYears)
	for yi, totals := range allTotals {
		if len(totals) == 0 {
			continue
		}
		sort.Float64s(totals)
		bands[yi] = mcBand{
			P10: percentile(totals, 10),
			P25: percentile(totals, 25),
			P50: percentile(totals, 50),
			P75: percentile(totals, 75),
			P90: percentile(totals, 90),
		}
	}

	return float64(successes) / float64(numSims) * 100, bands
}

// spendingMet reports whether a simulated run covered retirement spending in
// every year. Checking the final balance instead would count a run as a
// success whenever something that's never sold, like a home included in
// retirement calcs, is still there, even after the investments ran out.
func spendingMet(years []ForecastYear) bool {
	for _, y := range years {
		if y.Shortfall > 0.5 { // ignore rounding
			return false
		}
	}
	return len(years) > 0
}

// percentile returns the p-th percentile (0–100) of a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p / 100 * float64(len(sorted)-1)
	lo := int(idx)
	hi := lo + 1
	if hi >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// effectiveRetirementIncome returns the monthly amount a plan should assume
// is spent in retirement — profile.MonthlyExpenses, the single source of
// truth for household spending (current and assumed-in-retirement alike).
// UseBudgetForExpenses controls where that figure itself comes from (a
// manually-typed value vs. a 3-month transaction average — see
// loadForecastInputs), not whether it applies here.
//
// This is the single source of truth for retirement spending — both the FI
// Number (fiNumberForProfile) and the actual withdrawal simulation
// (simulateCore, ComputeRetirementPlan) must use it, or the two can silently
// disagree.
func effectiveRetirementIncome(profile *models.UserProfile) float64 {
	return profile.MonthlyExpenses
}

// effectiveSWRForProfile returns the profile's configured safe withdrawal
// rate, defaulting to 4% when unset. Every SWR-derived figure (FI Number,
// sustainable income) must use this so Settings changes flow everywhere.
func effectiveSWRForProfile(profile *models.UserProfile) float64 {
	if profile.WithdrawalRate != nil && *profile.WithdrawalRate > 0 {
		return *profile.WithdrawalRate
	}
	return 0.04
}

// sustainableMonthly is the monthly income a portfolio supports at the safe
// withdrawal rate, in today's dollars so it reads alongside spending.
func sustainableMonthly(profile *models.UserProfile, portfolio float64, yearsOut int) float64 {
	return portfolio * effectiveSWRForProfile(profile) / 12 / todayDollarFactor(profile, yearsOut)
}

// computeFIMetrics calculates FIRE-related metrics from the base simulation.
// fiNumberForProfile computes the FI number (annual retirement expenses
// divided by the effective safe withdrawal rate) for a given profile.
func fiNumberForProfile(profile *models.UserProfile) float64 {
	// FI Number = annual retirement expenses / SWR (the 25× rule assumes 4%).
	annualRetirementExpenses := effectiveRetirementIncome(profile) * 12
	return annualRetirementExpenses / effectiveSWRForProfile(profile)
}

// findFICrossing scans years in chronological order and returns the year/age
// at which the portfolio total first reaches fiNumber (in today's dollars,
// inflated to each year's dollars), or nil if it never does.
func findFICrossing(years []ForecastYear, fiNumber float64, profile *models.UserProfile) (*int, *int) {
	if fiNumber <= 0 {
		return nil, nil
	}
	for i, y := range years {
		if y.Total >= fiNumber*todayDollarFactor(profile, i) {
			yr := y.Year
			age := y.Age
			return &yr, &age
		}
	}
	return nil, nil
}

func (s *ForecastService) computeFIMetrics(
	profile *models.UserProfile,
	effectiveRates map[uuid.UUID]float64,
	accounts []models.Account,
	startingBalances map[uuid.UUID]float64,
	currentPortfolio float64,
	baseYears []ForecastYear,
	primaryBirthYear int,
	homeDetails map[uuid.UUID]*models.HomeDetails,
) *FIMetrics {
	m := &FIMetrics{CurrentPortfolio: currentPortfolio}

	m.FINumber = fiNumberForProfile(profile)

	// FI Progress.
	if m.FINumber > 0 {
		m.FIProgressPct = math.Min((currentPortfolio/m.FINumber)*100, 100)
	}

	// Time to FI: first year the base simulation crosses the FI Number.
	m.TimeToFIYear, m.TimeToFIAge = findFICrossing(baseYears, m.FINumber, profile)
	if !profile.UseInflationAdjustment {
		m.FINumberGrowth = profile.InflationRate
	}

	// Coast FI: the amount needed today to reach FI Number at retirement via compounding alone.
	currentYear := time.Now().Year()
	retirementYear := primaryBirthYear + profile.RetirementAge
	yearsToRetirement := retirementYear - currentYear
	if yearsToRetirement > 0 && m.FINumber > 0 {
		weightedRate := portfolioWeightedRate(accounts, startingBalances, effectiveRates, homeDetails)
		if weightedRate <= 0 {
			weightedRate = models.DefaultAssetClassReturnRates["equity"]
		}
		// The FI number is in today's dollars, so it compounds at the real
		// return (after inflation), whichever way the forecast is shown.
		realRate := (1+weightedRate)/(1+profile.InflationRate) - 1
		m.CoastFIAmount = m.FINumber / math.Pow(1+realRate, float64(yearsToRetirement))
		m.IsCoastFI = currentPortfolio >= m.CoastFIAmount
	}

	// Projected portfolio at retirement, used by the readiness score.
	m.ProjectedRetirementPortfolio = portfolioAtRetirement(baseYears, retirementYear, currentPortfolio)

	// Savings rate = (income - expenses) / income.
	if profile.MonthlyIncome > 0 {
		monthlySavings := profile.MonthlyIncome - profile.CurrentMonthlyExpenses
		if monthlySavings < 0 {
			monthlySavings = 0
		}
		sr := monthlySavings / profile.MonthlyIncome
		m.SavingsRate = &sr
	}

	// Withdrawal rate = annual income need / projected portfolio at
	// retirement, both in the retirement year's dollars.
	annualWithdrawal := effectiveRetirementIncome(profile) * 12 * todayDollarFactor(profile, yearsToRetirement)
	if annualWithdrawal > 0 {
		retirementPortfolio := m.ProjectedRetirementPortfolio
		if retirementPortfolio > 0 {
			wr := annualWithdrawal / retirementPortfolio
			m.WithdrawalRate = &wr
			m.WithdrawalRateWarn = wr > 0.045
		}
	}

	return m
}

// activeFraction returns the fraction of the calendar year containing
// windowStart during which an item bounded by the optional [start, end]
// interval overlaps with [windowStart, windowEnd). Used to prorate a
// contribution's annual amount by its actual start/end date rather than the
// coarser fraction applied to the simulation year as a whole.
func activeFraction(windowStart, windowEnd time.Time, start, end *time.Time) float64 {
	activeStart := windowStart
	if start != nil && start.After(activeStart) {
		activeStart = *start
	}
	activeEnd := windowEnd
	if end != nil {
		// EndDate is stored as midnight at the start of that calendar day but
		// means "active through the end of this day" — push the boundary to
		// the start of the next day so the end date itself is fully included.
		inclusiveEnd := end.AddDate(0, 0, 1)
		if inclusiveEnd.Before(activeEnd) {
			activeEnd = inclusiveEnd
		}
	}
	if !activeEnd.After(activeStart) {
		return 0
	}
	yearStart := time.Date(windowStart.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	yearEnd := time.Date(windowStart.Year()+1, 1, 1, 0, 0, 0, 0, time.UTC)
	daysInYear := yearEnd.Sub(yearStart).Hours() / 24
	overlapDays := activeEnd.Sub(activeStart).Hours() / 24
	return overlapDays / daysInYear
}

// portfolioWeightedRate returns a balance-weighted average effective return
// rate across accounts that count toward the portfolio (see
// countsTowardPortfolio) — an opted-out home shouldn't skew the blended rate
// used for Coast FI and the Scenarios page's "Return Rate" stat.
func portfolioWeightedRate(
	accounts []models.Account,
	balances map[uuid.UUID]float64,
	rates map[uuid.UUID]float64,
	homeDetails map[uuid.UUID]*models.HomeDetails,
) float64 {
	total := 0.0
	for _, acc := range accounts {
		if !countsTowardPortfolio(acc, homeDetails) {
			continue
		}
		if b := balances[acc.ID]; b > 0 {
			total += b
		}
	}
	if total <= 0 {
		return 0
	}
	var weighted float64
	for _, acc := range accounts {
		if !countsTowardPortfolio(acc, homeDetails) {
			continue
		}
		if b := balances[acc.ID]; b > 0 {
			weighted += (b / total) * rates[acc.ID]
		}
	}
	return weighted
}

// holdingValue is a holding's market value. Cash holdings store their dollar
// amount in Shares and usually have no price; anything else needs one.
func holdingValue(h models.Holding) (float64, bool) {
	if h.AssetClass == "cash_equivalent" {
		return h.Shares, true
	}
	if h.PricePerShare != nil {
		return h.Shares * *h.PricePerShare, true
	}
	return 0, false
}

// computeStartingBalances builds the initial balance map using live holding values where available,
// falling back to the latest snapshot balance.
func (s *ForecastService) computeStartingBalances(
	ctx context.Context,
	userID uuid.UUID,
	accounts []models.Account,
) (map[uuid.UUID]float64, error) {
	balances := make(map[uuid.UUID]float64, len(accounts))

	allHoldings, _ := s.holdings.ListByUser(ctx, userID)
	holdingValueByAccount := make(map[uuid.UUID]float64)
	for _, h := range allHoldings {
		if v, ok := holdingValue(h); ok {
			holdingValueByAccount[h.AccountID] += v
		}
	}

	latestSnaps, _ := s.snapshots.LatestPerAccount(ctx, userID)
	snapByAccount := make(map[uuid.UUID]float64, len(latestSnaps))
	for _, snap := range latestSnaps {
		snapByAccount[snap.AccountID] = snap.Balance
	}

	for _, acc := range accounts {
		if hv, ok := holdingValueByAccount[acc.ID]; ok && hv != 0 {
			balances[acc.ID] = hv
		} else if b, ok := snapByAccount[acc.ID]; ok {
			balances[acc.ID] = b
		}
	}

	return balances, nil
}

// computeEffectiveRates determines the annual return rate for each account.
// Priority: account-level override > weighted holdings average > account-type default.
func (s *ForecastService) computeEffectiveRates(
	accounts []models.Account,
	holdings []models.Holding,
	rateMap map[string]float64,
) map[uuid.UUID]float64 {
	type acClassValue struct {
		assetClass string
		value      float64
	}
	holdingsByAccount := make(map[uuid.UUID][]acClassValue)
	for _, h := range holdings {
		if v, ok := holdingValue(h); ok && v > 0 {
			holdingsByAccount[h.AccountID] = append(holdingsByAccount[h.AccountID], acClassValue{h.AssetClass, v})
		}
	}

	rates := make(map[uuid.UUID]float64, len(accounts))
	for _, acc := range accounts {
		if acc.ReturnRateOverride != nil {
			rates[acc.ID] = *acc.ReturnRateOverride
			continue
		}

		if hvs := holdingsByAccount[acc.ID]; len(hvs) > 0 {
			total := 0.0
			for _, hv := range hvs {
				total += hv.value
			}
			var weighted float64
			for _, hv := range hvs {
				r := resolveAssetRate(hv.assetClass, rateMap)
				weighted += (hv.value / total) * r
			}
			rates[acc.ID] = weighted
			continue
		}

		assetClass := models.AccountTypeDefaultAssetClass[acc.AccountType]
		rates[acc.ID] = resolveAssetRate(assetClass, rateMap)
	}

	return rates
}

// resolveAssetRate looks up a rate in the user's custom map, falling back to system defaults.
func resolveAssetRate(assetClass string, userRates map[string]float64) float64 {
	if r, ok := userRates[assetClass]; ok {
		return r
	}
	if r, ok := models.DefaultAssetClassReturnRates[assetClass]; ok {
		return r
	}
	return models.DefaultAssetClassReturnRates["other"]
}

// buildRateMap converts a slice of UserReturnRate records to a map.
func buildRateMap(rates []models.UserReturnRate) map[string]float64 {
	m := make(map[string]float64, len(rates))
	for _, r := range rates {
		m[r.AssetClass] = r.ReturnRate
	}
	return m
}

// EffectiveRateResult is the return value of EffectiveRateForAccount.
type EffectiveRateResult struct {
	Rate       float64 `json:"rate"`
	Source     string  `json:"source"` // "account_override" | "holdings_weighted" | "user_custom" | "asset_class_default"
	AssetClass string  `json:"assetClass,omitempty"`
}

// EffectiveRateForAccount computes the effective return rate for a single account
// using the same priority logic as the full forecast simulation.
func (s *ForecastService) EffectiveRateForAccount(ctx context.Context, accountID, userID uuid.UUID) (*EffectiveRateResult, error) {
	acc, err := s.accounts.Get(ctx, accountID, userID)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}

	if acc.ReturnRateOverride != nil {
		return &EffectiveRateResult{
			Rate:   *acc.ReturnRateOverride,
			Source: "account_override",
		}, nil
	}

	userRates, err := s.returnRates.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get return rates: %w", err)
	}
	rm := buildRateMap(userRates)

	holdings, err := s.holdings.ListByAccount(ctx, accountID, userID)
	if err != nil {
		return nil, fmt.Errorf("get holdings: %w", err)
	}

	type acv struct {
		assetClass string
		value      float64
	}
	var hvs []acv
	for _, h := range holdings {
		if h.PricePerShare != nil && *h.PricePerShare > 0 {
			hvs = append(hvs, acv{h.AssetClass, h.Shares * *h.PricePerShare})
		}
	}
	if len(hvs) > 0 {
		total := 0.0
		for _, hv := range hvs {
			total += hv.value
		}
		var weighted float64
		for _, hv := range hvs {
			weighted += (hv.value / total) * resolveAssetRate(hv.assetClass, rm)
		}
		return &EffectiveRateResult{Rate: weighted, Source: "holdings_weighted"}, nil
	}

	assetClass := models.AccountTypeDefaultAssetClass[acc.AccountType]
	source := "asset_class_default"
	if _, ok := rm[assetClass]; ok {
		source = "user_custom"
	}
	return &EffectiveRateResult{
		Rate:       resolveAssetRate(assetClass, rm),
		Source:     source,
		AssetClass: assetClass,
	}, nil
}
