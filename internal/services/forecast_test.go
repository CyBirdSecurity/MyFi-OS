package services

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// newProfile returns a minimal UserProfile for forecast tests.
// Birthday is set so the user is currently 40 years old. monthlyExpenses
// drives retirement spending (see effectiveRetirementIncome) as well as the
// FI Number — MonthlyExpenses is the single source of truth for both.
func newProfile(retirementAge int, monthlyExpenses float64) *models.UserProfile {
	birthYear := time.Now().Year() - 40
	birthday := time.Date(birthYear, 1, 1, 0, 0, 0, 0, time.UTC)
	return &models.UserProfile{
		UserID:          uuid.New(),
		Birthday:        &birthday,
		RetirementAge:   retirementAge,
		MonthlyExpenses: monthlyExpenses,
		InflationRate:   0.03,
	}
}

// svc returns a ForecastService with nil stores (safe because simulate()
// receives all data as arguments and never touches the store fields).
func svc() *ForecastService { return &ForecastService{} }

// account creates a test account owned by the given user.
func account(userID uuid.UUID, accountType string) models.Account {
	return models.Account{
		ID:           uuid.New(),
		UserID:       userID,
		Name:         accountType,
		AccountType:  accountType,
		TaxTreatment: "taxable",
	}
}

func ptr[T any](v T) *T { return &v }

// approxEqual returns true if a and b are within 1 dollar (rounding noise).
func approxEqual(a, b float64) bool { return math.Abs(a-b) < 1.0 }

// currentYearFraction returns the remaining fraction of the current calendar year,
// mirroring the logic in simulateCore so tests can compute correct expected values.
func currentYearFraction() float64 {
	now := time.Now()
	yearEnd := time.Date(now.Year()+1, 1, 1, 0, 0, 0, 0, time.UTC)
	daysInYear := yearEnd.Sub(time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24
	daysRemaining := yearEnd.Sub(now).Hours() / 24
	f := daysRemaining / daysInYear
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// ---- Forecast simulation tests ----

// TestSimulate_CompoundGrowthNoContributions verifies basic compound interest.
// Running from currentYear to currentYear (inclusive) produces 1 data point:
// $100k at 10% for 1 year → $110k.
func TestSimulate_CompoundGrowthNoContributions(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(65, 0) // retire at 65, no withdrawals

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{acc.ID: 100_000}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.10}

	result := svc().simulate(
		[]models.Account{acc},
		startingBalances,
		nil, // no contributions
		effectiveRates,
		nil,
		profile, nil,
		currentYear-1, currentYear-1, // single year, past year → no proration
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	if len(result) != 1 {
		t.Fatalf("expected 1 year, got %d", len(result))
	}
	want := 100_000 * 1.10
	got := result[0].Total
	if !approxEqual(got, want) {
		t.Errorf("after 1 year at 10%%: want %.2f, got %.2f", want, got)
	}
}

// TestSimulate_MonthlyContributionAdded verifies that a monthly contribution
// is annualised and added to the balance each year.
func TestSimulate_MonthlyContributionAdded(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "traditional_401k")
	profile := newProfile(65, 0)

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{acc.ID: 0}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.0} // zero rate, isolate contribution

	contrib := models.Contribution{
		AccountID: acc.ID,
		UserID:    userID,
		Amount:    1_000,
		Frequency: "monthly",
		IsActive:  true,
	}

	result := svc().simulate(
		[]models.Account{acc},
		startingBalances,
		[]models.Contribution{contrib},
		effectiveRates,
		nil,
		profile, nil,
		currentYear-1, currentYear, // past startYear → no proration
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// Year 0: balance after contributions = $12k (user is not yet retired)
	// Year 1: balance after contributions = $24k
	if !approxEqual(result[1].Total, 24_000) {
		t.Errorf("expected 24000 after 2 years of $1k/month, got %.2f", result[1].Total)
	}
}

// TestSimulate_EmployerMatchMultipliesContribution verifies that employer match
// adds to the employee contribution (50% match → 1.5× total).
func TestSimulate_EmployerMatchMultipliesContribution(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "traditional_401k")
	profile := newProfile(65, 0)

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{acc.ID: 0}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.0}

	contrib := models.Contribution{
		AccountID:  acc.ID,
		UserID:     userID,
		Amount:     10_000, // annual
		Frequency:  "annually",
		MatchTiers: models.MatchTiers{{MatchPct: 0.50}}, // 50% employer match → $15k total
		IsActive:   true,
	}

	result := svc().simulate(
		[]models.Account{acc},
		startingBalances,
		[]models.Contribution{contrib},
		effectiveRates,
		nil,
		profile, nil,
		currentYear-1, currentYear-1, // past startYear → no proration
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// Year 0: $10k × (1 + 0.50) = $15k
	if !approxEqual(result[0].Total, 15_000) {
		t.Errorf("expected 15000 with 50%% employer match, got %.2f", result[0].Total)
	}
}

// TestSimulate_AnnualIncreaseCompounds verifies that the annual increase
// percentage scales contributions correctly in year 2.
func TestSimulate_AnnualIncreaseCompounds(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(65, 0)

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{acc.ID: 0}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.0}

	contrib := models.Contribution{
		AccountID:         acc.ID,
		UserID:            userID,
		Amount:            10_000,
		Frequency:         "annually",
		AnnualIncreasePct: 0.10, // 10% raise per year
		IsActive:          true,
	}

	result := svc().simulate(
		[]models.Account{acc},
		startingBalances,
		[]models.Contribution{contrib},
		effectiveRates,
		nil,
		profile, nil,
		currentYear-1, currentYear+1, // past startYear → no proration
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// yearIdx=0 → $10k × 1.10^0 = $10k
	// yearIdx=1 → $10k × 1.10^1 = $11k  cumulative: $21k
	// yearIdx=2 → $10k × 1.10^2 = $12.1k  cumulative: $33.1k
	wantYear2 := 10_000 + 11_000 + 12_100.0
	if !approxEqual(result[2].Total, wantYear2) {
		t.Errorf("year 2 total: want %.2f, got %.2f", wantYear2, result[2].Total)
	}
}

// TestSimulate_SocialSecurityReducesWithdrawal confirms that enabling Social
// Security reduces the amount drawn from the portfolio each year.
// User wants $60k/year total; SS provides $24k/year → portfolio covers $36k/year.
func TestSimulate_SocialSecurityReducesWithdrawal(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(65, 5_000) // $5k/month desired = $60k/year
	profile.UseSocialSecurity = true
	profile.SocialSecurityMonthly = 2_000 // $24k/year from SS

	// Put user right at retirement age so withdrawals start immediately.
	retirementBirthYear := time.Now().Year() - 65
	bd := time.Date(retirementBirthYear, 1, 1, 0, 0, 0, 0, time.UTC)
	profile.Birthday = &bd

	currentYear := time.Now().Year()
	startingBalance := 1_000_000.0
	startingBalances := map[uuid.UUID]float64{acc.ID: startingBalance}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.0} // zero rate, isolate withdrawal

	result := svc().simulate(
		[]models.Account{acc},
		startingBalances,
		nil,
		effectiveRates,
		nil,
		profile, nil,
		currentYear, currentYear,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// Without SS: withdrawal = $60k → balance = $940k
	// With SS:    withdrawal = $36k → balance = $964k
	wantBalance := startingBalance - 36_000.0
	got := result[0].Total
	if !approxEqual(got, wantBalance) {
		t.Errorf("SS reduced withdrawal: want balance %.2f, got %.2f", wantBalance, got)
	}
}

// TestSimulate_SocialSecurityExceedsIncome verifies that when SS income exceeds
// the desired retirement income, the portfolio withdrawal is clamped to zero
// (not negative).
func TestSimulate_SocialSecurityExceedsIncome(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(65, 1_000) // $1k/month desired
	profile.UseSocialSecurity = true
	profile.SocialSecurityMonthly = 2_000 // $2k/month SS > desired → zero portfolio draw

	retirementBirthYear := time.Now().Year() - 65
	bd := time.Date(retirementBirthYear, 1, 1, 0, 0, 0, 0, time.UTC)
	profile.Birthday = &bd

	currentYear := time.Now().Year()
	startingBalance := 500_000.0
	startingBalances := map[uuid.UUID]float64{acc.ID: startingBalance}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.0}

	result := svc().simulate(
		[]models.Account{acc},
		startingBalances,
		nil,
		effectiveRates,
		nil,
		profile, nil,
		currentYear, currentYear,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// No portfolio withdrawal should occur — balance unchanged.
	if !approxEqual(result[0].Total, startingBalance) {
		t.Errorf("SS > income: balance should be unchanged, got %.2f", result[0].Total)
	}
}

// TestSimulate_WithdrawalNeverGoesNegative verifies that even with large
// withdrawals, account balances are clamped to zero in the output.
func TestSimulate_WithdrawalNeverGoesNegative(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(65, 100_000) // $100k/month — far more than the balance

	retirementBirthYear := time.Now().Year() - 65
	bd := time.Date(retirementBirthYear, 1, 1, 0, 0, 0, 0, time.UTC)
	profile.Birthday = &bd

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{acc.ID: 10_000}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.0}

	result := svc().simulate(
		[]models.Account{acc},
		startingBalances,
		nil,
		effectiveRates,
		nil,
		profile, nil,
		currentYear, currentYear+5,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	for _, fy := range result {
		if fy.Total < 0 {
			t.Errorf("year %d: total is negative (%.2f)", fy.Year, fy.Total)
		}
	}
}

// TestSimulate_TaxAwareWithdrawal verifies that taxable accounts are drained before
// tax-free accounts, and that proportional ordering is applied within a tier.
func TestSimulate_TaxAwareWithdrawal(t *testing.T) {
	userID := uuid.New()
	acc1 := account(userID, "brokerage") // taxable — $200k
	acc2 := account(userID, "roth_401k")
	acc2.TaxTreatment = "tax_free"   // override helper default
	profile := newProfile(65, 3_000) // $36k/year withdrawal

	retirementBirthYear := time.Now().Year() - 65
	bd := time.Date(retirementBirthYear, 1, 1, 0, 0, 0, 0, time.UTC)
	profile.Birthday = &bd

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{
		acc1.ID: 200_000,
		acc2.ID: 100_000,
	}
	effectiveRates := map[uuid.UUID]float64{
		acc1.ID: 0.0,
		acc2.ID: 0.0,
	}

	result := svc().simulate(
		[]models.Account{acc1, acc2},
		startingBalances,
		nil,
		effectiveRates,
		nil,
		profile, nil,
		currentYear, currentYear,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// All $36k should come from the taxable account; Roth untouched.
	got1 := result[0].Accounts[acc1.ID.String()]
	got2 := result[0].Accounts[acc2.ID.String()]
	if !approxEqual(got1, 200_000-36_000) {
		t.Errorf("taxable acc: want 164000, got %.2f", got1)
	}
	if !approxEqual(got2, 100_000) {
		t.Errorf("tax_free acc: want 100000 (untouched), got %.2f", got2)
	}
}

// TestSimulate_TaxAwareWithdrawal_ProportionalWithinTier verifies proportional
// distribution when multiple accounts share the same tax tier.
func TestSimulate_TaxAwareWithdrawal_ProportionalWithinTier(t *testing.T) {
	userID := uuid.New()
	acc1 := account(userID, "brokerage") // taxable, $200k
	acc2 := account(userID, "savings")   // taxable, $100k
	profile := newProfile(65, 3_000)     // $36k/year withdrawal

	retirementBirthYear := time.Now().Year() - 65
	bd := time.Date(retirementBirthYear, 1, 1, 0, 0, 0, 0, time.UTC)
	profile.Birthday = &bd

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{
		acc1.ID: 200_000,
		acc2.ID: 100_000,
	}
	effectiveRates := map[uuid.UUID]float64{
		acc1.ID: 0.0,
		acc2.ID: 0.0,
	}

	result := svc().simulate(
		[]models.Account{acc1, acc2},
		startingBalances,
		nil,
		effectiveRates,
		nil,
		profile, nil,
		currentYear, currentYear,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// Both taxable: proportional — acc1 -$24k (2/3), acc2 -$12k (1/3).
	got1 := result[0].Accounts[acc1.ID.String()]
	got2 := result[0].Accounts[acc2.ID.String()]
	if !approxEqual(got1, 176_000) {
		t.Errorf("acc1 (2/3 share): want 176000, got %.2f", got1)
	}
	if !approxEqual(got2, 88_000) {
		t.Errorf("acc2 (1/3 share): want 88000, got %.2f", got2)
	}
}

// TestSimulate_WithdrawalExcludesHomeEquity verifies that retirement-income
// withdrawals never draw down a home account, even though it defaults to the
// "taxable" tier. Home balances are recomputed every year from their own
// amortization/appreciation state (see the AccountType == "home" branch in
// simulateCore), completely independent of any withdrawal taken from
// balances[acc.ID] the prior year — so pooling home into the taxable
// withdrawal tier would let withdrawn dollars silently regenerate the next
// year while diluting how much the actually-liquid taxable accounts absorb.
func TestSimulate_WithdrawalExcludesHomeEquity(t *testing.T) {
	userID := uuid.New()
	homeAcc := account(userID, "home") // taxable by default, $100k starting equity
	brokerage := account(userID, "brokerage")
	profile := newProfile(40, 10_000) // $120k/year withdrawal

	mortgageStart := time.Now()
	homeDetails := map[uuid.UUID]*models.HomeDetails{
		homeAcc.ID: {
			AccountID:          homeAcc.ID,
			MortgageStartDate:  &mortgageStart,
			OriginalLoanValue:  200_000,
			CurrentLoanBalance: 200_000,
			InterestRate:       0.06,
			LoanTermYears:      30,
		},
	}

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{
		homeAcc.ID:   100_000,
		brokerage.ID: 50_000,
	}
	effectiveRates := map[uuid.UUID]float64{
		homeAcc.ID:   0.0, // no appreciation, so any equity increase is from paydown alone
		brokerage.ID: 0.0,
	}

	result := svc().simulate(
		[]models.Account{homeAcc, brokerage},
		startingBalances,
		nil,
		effectiveRates,
		homeDetails,
		profile, nil,
		currentYear, currentYear,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	gotHome := result[0].Accounts[homeAcc.ID.String()]
	gotBrokerage := result[0].Accounts[brokerage.ID.String()]

	// The $120k need exceeds brokerage's $50k balance, but home must never be
	// tapped to cover the shortfall — brokerage alone absorbs up to its full
	// balance, and the unmet remainder is simply left unfunded (no other
	// tiers exist in this fixture).
	if !approxEqual(gotBrokerage, 0) {
		t.Errorf("brokerage: want fully drained (0), got %.2f", gotBrokerage)
	}
	// With 0% appreciation, home equity only grows via mortgage principal
	// paydown — it can never go below its starting balance if withdrawals
	// are correctly excluded from it.
	if gotHome < 100_000 {
		t.Errorf("home equity: want >= 100000 (untouched by withdrawal), got %.2f", gotHome)
	}
}

// TestSimulate_WithdrawalExcludesNonDrainableAssets verifies that vehicles
// and other physical-asset accounts are never tapped to fund retirement
// withdrawals, even though they default to "taxable" like brokerage accounts.
func TestSimulate_WithdrawalExcludesNonDrainableAssets(t *testing.T) {
	userID := uuid.New()
	vehicle := account(userID, "automobile")
	otherAsset := account(userID, "other_asset")
	brokerage := account(userID, "brokerage")
	profile := newProfile(40, 10_000) // already retired, $120k/year withdrawal

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{
		vehicle.ID:    30_000,
		otherAsset.ID: 20_000,
		brokerage.ID:  50_000,
	}
	effectiveRates := map[uuid.UUID]float64{
		vehicle.ID:    0.0,
		otherAsset.ID: 0.0,
		brokerage.ID:  0.0,
	}

	result := svc().simulate(
		[]models.Account{vehicle, otherAsset, brokerage},
		startingBalances,
		nil,
		effectiveRates,
		nil,
		profile, nil,
		currentYear, currentYear,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// The $120k need exceeds brokerage's $50k, but the shortfall must be left
	// unfunded rather than covered by selling the vehicle or other asset.
	if got := result[0].Accounts[brokerage.ID.String()]; !approxEqual(got, 0) {
		t.Errorf("brokerage: want fully drained (0), got %.2f", got)
	}
	if got := result[0].Accounts[vehicle.ID.String()]; !approxEqual(got, 30_000) {
		t.Errorf("vehicle: want untouched (30000), got %.2f", got)
	}
	if got := result[0].Accounts[otherAsset.ID.String()]; !approxEqual(got, 20_000) {
		t.Errorf("other_asset: want untouched (20000), got %.2f", got)
	}
}

// TestSimulate_SpendGoalExcludesNonDrainableAssets verifies the same
// exclusion for one-time spending goal deductions.
func TestSimulate_SpendGoalExcludesNonDrainableAssets(t *testing.T) {
	userID := uuid.New()
	vehicle := account(userID, "automobile")
	brokerage := account(userID, "brokerage")
	profile := newProfile(65, 0) // not retired, no income-need withdrawals

	currentYear := time.Now().Year()
	targetDate := time.Date(currentYear, 6, 1, 0, 0, 0, 0, time.UTC)
	goal := models.Goal{
		GoalType:     "spend",
		TargetAmount: 80_000,
		TargetDate:   &targetDate,
	}

	startingBalances := map[uuid.UUID]float64{
		vehicle.ID:   30_000,
		brokerage.ID: 50_000,
	}
	effectiveRates := map[uuid.UUID]float64{
		vehicle.ID:   0.0,
		brokerage.ID: 0.0,
	}

	result := svc().simulate(
		[]models.Account{vehicle, brokerage},
		startingBalances,
		nil,
		effectiveRates,
		nil,
		profile, nil,
		currentYear, currentYear,
		profile.Birthday.Year(), nil, 0, []models.Goal{goal}, nil,
	)

	// The $80k goal exceeds brokerage's $50k — the vehicle must not absorb
	// the remainder.
	if got := result[0].Accounts[brokerage.ID.String()]; !approxEqual(got, 0) {
		t.Errorf("brokerage: want fully drained (0), got %.2f", got)
	}
	if got := result[0].Accounts[vehicle.ID.String()]; !approxEqual(got, 30_000) {
		t.Errorf("vehicle: want untouched (30000), got %.2f", got)
	}
}

// TestSimulate_ExcludesHomeFromTotalWhenOptedOut verifies that a home
// account with IncludeInRetirementCalcs=false is dropped from the year's
// Total (FI Number / Forecast / Retirement portfolio) while still appearing
// in the per-account Accounts breakdown (Net Worth / Accounts page still
// shows it — only the retirement-planning total excludes it).
func TestSimulate_ExcludesHomeFromTotalWhenOptedOut(t *testing.T) {
	userID := uuid.New()
	homeAcc := account(userID, "home")
	brokerage := account(userID, "brokerage")
	profile := newProfile(65, 0) // not retired yet, no withdrawals to worry about

	homeDetails := map[uuid.UUID]*models.HomeDetails{
		homeAcc.ID: {AccountID: homeAcc.ID, IncludeInRetirementCalcs: false},
	}

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{
		homeAcc.ID:   500_000,
		brokerage.ID: 100_000,
	}
	effectiveRates := map[uuid.UUID]float64{
		homeAcc.ID:   0.0,
		brokerage.ID: 0.0,
	}

	result := svc().simulate(
		[]models.Account{homeAcc, brokerage},
		startingBalances,
		nil,
		effectiveRates,
		homeDetails,
		profile, nil,
		currentYear, currentYear,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	if !approxEqual(result[0].Total, 100_000) {
		t.Errorf("Total: want 100000 (home excluded), got %.2f", result[0].Total)
	}
	if !approxEqual(result[0].Accounts[homeAcc.ID.String()], 500_000) {
		t.Errorf("home account balance should still be present in the breakdown, got %.2f", result[0].Accounts[homeAcc.ID.String()])
	}
}

// TestCountsTowardPortfolio_OptedOutHome verifies the predicate that
// AccountMeta.CountsTowardPortfolio is derived from — the chart uses this
// field to decide which accounts to render, so it must match simulateCore's
// own Total exclusion exactly.
func TestCountsTowardPortfolio_OptedOutHome(t *testing.T) {
	userID := uuid.New()
	homeAcc := account(userID, "home")
	brokerage := account(userID, "brokerage")

	homeDetails := map[uuid.UUID]*models.HomeDetails{
		homeAcc.ID: {AccountID: homeAcc.ID, IncludeInRetirementCalcs: false},
	}

	if countsTowardPortfolio(homeAcc, homeDetails) {
		t.Error("opted-out home should not count toward portfolio")
	}
	if !countsTowardPortfolio(brokerage, homeDetails) {
		t.Error("non-home account should always count toward portfolio")
	}

	homeDetails[homeAcc.ID].IncludeInRetirementCalcs = true
	if !countsTowardPortfolio(homeAcc, homeDetails) {
		t.Error("opted-in home should count toward portfolio")
	}

	delete(homeDetails, homeAcc.ID)
	if countsTowardPortfolio(homeAcc, homeDetails) {
		t.Error("home with no saved details should not count toward portfolio")
	}

	for _, typ := range []string{"automobile", "other_asset", "credit_card", "loan", "other_liability"} {
		if countsTowardPortfolio(account(userID, typ), homeDetails) {
			t.Errorf("%s should not count toward portfolio", typ)
		}
	}
}

// TestSimulate_LiabilityPaidDown verifies a debt with a payment on record is
// amortized month by month, stays negative in the breakdown, and never
// counts toward the portfolio Total.
func TestSimulate_LiabilityPaidDown(t *testing.T) {
	userID := uuid.New()
	loan := account(userID, "loan")
	brokerage := account(userID, "brokerage")
	profile := newProfile(65, 0)

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{loan.ID: -18_000, brokerage.ID: 50_000}
	effectiveRates := map[uuid.UUID]float64{loan.ID: 0.5, brokerage.ID: 0}
	liabilities := map[uuid.UUID]liabilityTerms{loan.ID: {monthlyRate: 0, monthlyPayment: 1_000}}

	result := svc().simulate(
		[]models.Account{loan, brokerage},
		startingBalances,
		nil,
		effectiveRates,
		nil,
		profile, nil,
		currentYear-1, currentYear, // past startYear → full 12 months per year
		profile.Birthday.Year(), nil, 0, nil, liabilities,
	)

	// $1k/month at 0%: $6k left after year 0, paid off in year 1. The loan's
	// return rate is ignored.
	if got := result[0].Accounts[loan.ID.String()]; !approxEqual(got, -6_000) {
		t.Errorf("year 0 loan balance: want -6000, got %.2f", got)
	}
	if got := result[1].Accounts[loan.ID.String()]; !approxEqual(got, 0) {
		t.Errorf("year 1 loan balance: want 0 (paid off), got %.2f", got)
	}
	if !approxEqual(result[0].Total, 50_000) {
		t.Errorf("Total should exclude the debt: want 50000, got %.2f", result[0].Total)
	}
}

// TestSimulate_LiabilityInterestAndNoTerms verifies interest accrues monthly
// on a debt, and a debt with no payment on record is held at its balance.
func TestSimulate_LiabilityInterestAndNoTerms(t *testing.T) {
	userID := uuid.New()
	card := account(userID, "credit_card")
	loan := account(userID, "loan")
	profile := newProfile(65, 0)

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{card.ID: -10_000, loan.ID: -5_000}
	liabilities := map[uuid.UUID]liabilityTerms{card.ID: {monthlyRate: 0.24 / 12, monthlyPayment: 300}}

	result := svc().simulate(
		[]models.Account{card, loan},
		startingBalances,
		nil,
		map[uuid.UUID]float64{},
		nil,
		profile, nil,
		currentYear-1, currentYear-1,
		profile.Birthday.Year(), nil, 0, nil, liabilities,
	)

	want := 10_000.0
	for m := 0; m < 12; m++ {
		want += want*0.02 - 300
	}
	if got := result[0].Accounts[card.ID.String()]; !approxEqual(got, -want) {
		t.Errorf("card balance: want %.2f, got %.2f", -want, got)
	}
	if got := result[0].Accounts[loan.ID.String()]; !approxEqual(got, -5_000) {
		t.Errorf("loan with no payment on record should stay at -5000, got %.2f", got)
	}
}

// TestSimulate_PartnerAccountFollowsPartnerRetirement verifies a partner's
// account (same login user, attributed to the partner member) keeps receiving
// contributions until the partner retires, not the primary.
func TestSimulate_PartnerAccountFollowsPartnerRetirement(t *testing.T) {
	profile := newProfile(35, 0) // primary (age 40) is already retired
	// Partner accounts belong to the same login user; only the flag differs.
	userID := profile.UserID
	acc := account(userID, "brokerage")
	acc.IsPartnerAccount = true

	spouseBirthYear := time.Now().Year() - 35
	spouse := &models.UserProfile{RetirementAge: 65}

	currentYear := time.Now().Year()
	contrib := models.Contribution{AccountID: acc.ID, UserID: userID, Amount: 10_000, Frequency: "annually", IsActive: true}

	result := svc().simulate(
		[]models.Account{acc},
		map[uuid.UUID]float64{acc.ID: 0},
		[]models.Contribution{contrib},
		map[uuid.UUID]float64{acc.ID: 0},
		nil,
		profile, spouse,
		currentYear-1, currentYear-1,
		profile.Birthday.Year(), &spouseBirthYear, 0, nil, nil,
	)

	if !approxEqual(result[0].Total, 10_000) {
		t.Errorf("partner (still working) should keep contributing: want 10000, got %.2f", result[0].Total)
	}
}

// TestSimulate_PartnerSocialSecurityReducesWithdrawal verifies the partner's
// Social Security offsets retirement spending alongside the primary's.
func TestSimulate_PartnerSocialSecurityReducesWithdrawal(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(65, 5_000) // $60k/year
	profile.UseSocialSecurity = true
	profile.SocialSecurityMonthly = 2_000 // $24k/year
	bd := time.Date(time.Now().Year()-70, 1, 1, 0, 0, 0, 0, time.UTC)
	profile.Birthday = &bd
	profile.SocialSecurityStartAge = 67

	spouseBirthYear := time.Now().Year() - 68
	spouse := &models.UserProfile{RetirementAge: 65, UseSocialSecurity: true, SocialSecurityMonthly: 1_000, SocialSecurityStartAge: 67}

	currentYear := time.Now().Year()
	result := svc().simulate(
		[]models.Account{acc},
		map[uuid.UUID]float64{acc.ID: 1_000_000},
		nil,
		map[uuid.UUID]float64{acc.ID: 0},
		nil,
		profile, spouse,
		currentYear-1, currentYear-1,
		profile.Birthday.Year(), &spouseBirthYear, 0, nil, nil,
	)

	// $60k − $24k − $12k = $24k drawn. The projection starts this year, so
	// today's dollars need no inflating.
	want := 1_000_000.0 - 24_000
	if !approxEqual(result[0].Total, want) {
		t.Errorf("want %.2f after both benefits, got %.2f", want, result[0].Total)
	}
}

// TestSimulate_RMDWithdrawal verifies that Required Minimum Distributions are
// taken from tax_deferred accounts once the account owner reaches age 73.
func TestSimulate_RMDWithdrawal(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "traditional_401k")
	acc.TaxTreatment = "tax_deferred"

	// User is 73 and retired (retirementAge=65).
	birthYear := time.Now().Year() - 73
	bd := time.Date(birthYear, 1, 1, 0, 0, 0, 0, time.UTC)
	profile := newProfile(65, 0) // no income need beyond RMD
	profile.Birthday = &bd

	currentYear := time.Now().Year()
	startingBalance := 100_000.0
	startingBalances := map[uuid.UUID]float64{acc.ID: startingBalance}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.0}

	result := svc().simulate(
		[]models.Account{acc},
		startingBalances,
		nil,
		effectiveRates,
		nil,
		profile, nil,
		currentYear, currentYear,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// RMD at 73: period = 26.5 → rmd = 100,000 / 26.5 ≈ 3,773.58. Nothing is
	// spent and there's no taxable account to move it to, so it stays put.
	if got := result[0].Total; !approxEqual(got, startingBalance) {
		t.Errorf("RMD at 73 with nowhere to go: want balance %.2f, got %.2f", startingBalance, got)
	}
}

// TestSimulate_RMDExcessMovesToTaxable verifies an RMD beyond spending is
// reinvested in a taxable account rather than vanishing.
func TestSimulate_RMDExcessMovesToTaxable(t *testing.T) {
	userID := uuid.New()
	ira := account(userID, "traditional_ira")
	ira.TaxTreatment = "tax_deferred"
	brokerage := account(userID, "brokerage")

	birthYear := time.Now().Year() - 74 // 73 in the simulated year; born before 1960: RMDs at 73
	bd := time.Date(birthYear, 1, 1, 0, 0, 0, 0, time.UTC)
	profile := newProfile(65, 100) // $1,200/yr spending, far below the RMD
	profile.Birthday = &bd

	currentYear := time.Now().Year()
	result := svc().simulate(
		[]models.Account{ira, brokerage},
		map[uuid.UUID]float64{ira.ID: 100_000, brokerage.ID: 10_000},
		nil,
		map[uuid.UUID]float64{ira.ID: 0, brokerage.ID: 0},
		nil,
		profile, nil,
		currentYear-1, currentYear-1, // past startYear → no proration
		birthYear, nil, 0, nil, nil,
	)

	rmd := 100_000.0 / 26.5
	if got := result[0].Accounts[ira.ID.String()]; !approxEqual(got, 100_000-rmd) {
		t.Errorf("IRA: want %.2f, got %.2f", 100_000-rmd, got)
	}
	// The brokerage gets the RMD minus the $1,200 spent.
	if got := result[0].Accounts[brokerage.ID.String()]; !approxEqual(got, 10_000+rmd-1_200) {
		t.Errorf("brokerage: want %.2f, got %.2f", 10_000+rmd-1_200, got)
	}
}

// TestRMDStartAge verifies SECURE 2.0's start ages.
func TestRMDStartAge(t *testing.T) {
	if rmdStartAge(1959) != 73 || rmdStartAge(1960) != 75 {
		t.Errorf("got %d for 1959 and %d for 1960, want 73 and 75", rmdStartAge(1959), rmdStartAge(1960))
	}
}

// TestSimulate_NominalSpendingInflatesFromToday verifies that in nominal
// mode, retirement spending (entered in today's dollars) is inflated from
// today, not from the retirement date.
func TestSimulate_NominalSpendingInflatesFromToday(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(65, 5_000) // 40 today, $60k/yr in today's dollars
	currentYear := time.Now().Year()

	result := svc().simulate(
		[]models.Account{acc},
		map[uuid.UUID]float64{acc.ID: 5_000_000},
		nil,
		map[uuid.UUID]float64{acc.ID: 0},
		nil,
		profile, nil,
		currentYear, currentYear+26,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// Age 65 is 25 years out: $60k × 1.03^25.
	for i := 1; i < len(result); i++ {
		if result[i].Age == 65 {
			want := 60_000 * math.Pow(1.03, 25)
			if got := result[i-1].Total - result[i].Total; !approxEqual(got, want) {
				t.Errorf("first retirement year: want %.2f withdrawn, got %.2f", want, got)
			}
			return
		}
	}
	t.Fatal("age 65 not in projection")
}

// TestSimulate_ShortfallWhenMoneyRunsOut verifies unmet spending is
// recorded, which is what Monte Carlo success and the Retirement page's
// depletion age are judged by.
func TestSimulate_ShortfallWhenMoneyRunsOut(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(65, 5_000) // $60k/yr
	profile.UseInflationAdjustment = true
	profile.InflationRate = 0 // keep balances flat so the shortfall is exact
	bd := time.Date(time.Now().Year()-70, 1, 1, 0, 0, 0, 0, time.UTC)
	profile.Birthday = &bd
	currentYear := time.Now().Year()

	result := svc().simulate(
		[]models.Account{acc},
		map[uuid.UUID]float64{acc.ID: 100_000},
		nil,
		map[uuid.UUID]float64{acc.ID: 0},
		nil,
		profile, nil,
		currentYear-1, currentYear,
		bd.Year(), nil, 0, nil, nil,
	)

	if result[0].Shortfall != 0 {
		t.Errorf("year 1 is covered: want no shortfall, got %.2f", result[0].Shortfall)
	}
	// $40k left against $60k of spending.
	if !approxEqual(result[1].Shortfall, 20_000) {
		t.Errorf("year 2: want a 20000 shortfall, got %.2f", result[1].Shortfall)
	}
	if spendingMet(result) {
		t.Error("spendingMet should be false once a year falls short")
	}
}

// TestComputeEffectiveRates_CountsManualCash verifies cash entered by hand
// (stored with no price) still weighs into the account's return rate.
func TestComputeEffectiveRates_CountsManualCash(t *testing.T) {
	acc := account(uuid.New(), "brokerage")
	price := 100.0
	holdings := []models.Holding{
		{AccountID: acc.ID, AssetClass: "equity", Shares: 500, PricePerShare: &price},
		{AccountID: acc.ID, AssetClass: "cash_equivalent", Shares: 50_000},
	}
	rate := svc().computeEffectiveRates([]models.Account{acc}, holdings, nil)[acc.ID]
	if want := (0.07 + 0.005) / 2; math.Abs(rate-want) > 1e-9 {
		t.Errorf("rate: want %.4f, got %.4f", want, rate)
	}
	if w := computeAssetClassWeights([]models.Account{acc}, holdings)[acc.ID]; math.Abs(w["cash_equivalent"]-0.5) > 1e-9 {
		t.Errorf("Monte Carlo weights: want half cash, got %v", w)
	}
}

// TestSimulate_WithdrawsFromMonthlyExpenses verifies that retirement
// withdrawals are driven directly by profile.MonthlyExpenses — the single
// source of truth for both current and assumed-in-retirement spending —
// regardless of UseBudgetForExpenses (which only controls where
// MonthlyExpenses' value itself comes from, not whether it applies here).
func TestSimulate_WithdrawsFromMonthlyExpenses(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(65, 4_000) // $48k/year

	retirementBirthYear := time.Now().Year() - 65
	bd := time.Date(retirementBirthYear, 1, 1, 0, 0, 0, 0, time.UTC)
	profile.Birthday = &bd

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{acc.ID: 500_000}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.0}

	result := svc().simulate(
		[]models.Account{acc},
		startingBalances,
		nil,
		effectiveRates,
		nil,
		profile, nil,
		currentYear, currentYear,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	want := 500_000.0 - 48_000.0
	got := result[0].Total
	if !approxEqual(got, want) {
		t.Errorf("expected retirement withdrawal driven by MonthlyExpenses ($48k/yr): want %.2f, got %.2f (no withdrawal would mean %.2f)",
			want, got, 500_000.0)
	}
}

// TestSimulate_NoWithdrawalWhenMonthlyExpensesUnset asserts that a zero
// MonthlyExpenses (the user hasn't set one) means no retirement withdrawal
// is assumed, rather than silently defaulting to some other figure.
func TestSimulate_NoWithdrawalWhenMonthlyExpensesUnset(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(65, 0) // MonthlyExpenses left at 0

	retirementBirthYear := time.Now().Year() - 65
	bd := time.Date(retirementBirthYear, 1, 1, 0, 0, 0, 0, time.UTC)
	profile.Birthday = &bd

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{acc.ID: 500_000}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.0}

	result := svc().simulate(
		[]models.Account{acc},
		startingBalances,
		nil,
		effectiveRates,
		nil,
		profile, nil,
		currentYear, currentYear,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	if !approxEqual(result[0].Total, 500_000) {
		t.Errorf("expected no withdrawal when MonthlyExpenses is unset, got %.2f", result[0].Total)
	}
}

// TestSimulate_ContributionsStopAtRetirement verifies that contributions stop
// once the account owner reaches retirement age.
func TestSimulate_ContributionsStopAtRetirement(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "traditional_401k")
	profile := newProfile(65, 0)

	// User is exactly 64 — retires next year.
	birthYear := time.Now().Year() - 64
	bd := time.Date(birthYear, 1, 1, 0, 0, 0, 0, time.UTC)
	profile.Birthday = &bd

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{acc.ID: 0}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.0}

	contrib := models.Contribution{
		AccountID: acc.ID,
		UserID:    userID,
		Amount:    12_000,
		Frequency: "annually",
		IsActive:  true,
	}

	result := svc().simulate(
		[]models.Account{acc},
		startingBalances,
		[]models.Contribution{contrib},
		effectiveRates,
		nil,
		profile, nil,
		currentYear, currentYear+2,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// Year 0 (age 64, pre-retirement): +$12k prorated by the remaining year fraction.
	// Year 1 (age 65, retirement):     no contribution — balance stays at year-0 level.
	// Year 2 (age 66, retirement):     no contribution — balance unchanged.
	frac := currentYearFraction()
	wantYear0 := 12_000 * frac
	if !approxEqual(result[0].Total, wantYear0) {
		t.Errorf("pre-retirement: expected %.2f, got %.2f", wantYear0, result[0].Total)
	}
	if !approxEqual(result[1].Total, wantYear0) {
		t.Errorf("retirement year: expected %.2f (no new contribution), got %.2f", wantYear0, result[1].Total)
	}
	if !approxEqual(result[2].Total, wantYear0) {
		t.Errorf("post-retirement: expected %.2f (no new contribution), got %.2f", wantYear0, result[2].Total)
	}
}

// TestSimulate_OptimisticHigherThanBase verifies that the optimistic
// scenario (rateAdj = +0.02) always produces a higher total than the base.
func TestSimulate_OptimisticHigherThanBase(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(65, 0)

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{acc.ID: 100_000}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.07}

	base := svc().simulate(
		[]models.Account{acc}, startingBalances, nil, effectiveRates,
		nil,
		profile, nil, currentYear, currentYear+10,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)
	optimistic := svc().simulate(
		[]models.Account{acc}, startingBalances, nil, effectiveRates,
		nil,
		profile, nil, currentYear, currentYear+10,
		profile.Birthday.Year(), nil, 0.02, nil, nil,
	)
	pessimistic := svc().simulate(
		[]models.Account{acc}, startingBalances, nil, effectiveRates,
		nil,
		profile, nil, currentYear, currentYear+10,
		profile.Birthday.Year(), nil, -0.02, nil, nil,
	)

	lastBase := base[len(base)-1].Total
	lastOpt := optimistic[len(optimistic)-1].Total
	lastPess := pessimistic[len(pessimistic)-1].Total

	if lastOpt <= lastBase {
		t.Errorf("optimistic (%.2f) should exceed base (%.2f)", lastOpt, lastBase)
	}
	if lastPess >= lastBase {
		t.Errorf("pessimistic (%.2f) should be below base (%.2f)", lastPess, lastBase)
	}
}

// TestSimulate_InflationAdjustmentLowersEffectiveRate verifies that enabling
// inflation adjustment produces a lower final balance than nominal mode
// (because the real rate is lower than the nominal rate).
func TestSimulate_InflationAdjustmentLowersEffectiveRate(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")

	profileNominal := newProfile(65, 0)
	profileReal := newProfile(65, 0)
	profileReal.UseInflationAdjustment = true
	profileReal.InflationRate = 0.03

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{acc.ID: 100_000}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.07}

	nominal := svc().simulate(
		[]models.Account{acc}, startingBalances, nil, effectiveRates,
		nil,
		profileNominal, nil, currentYear, currentYear+20,
		profileNominal.Birthday.Year(), nil, 0, nil, nil,
	)
	real := svc().simulate(
		[]models.Account{acc}, startingBalances, nil, effectiveRates,
		nil,
		profileReal, nil, currentYear, currentYear+20,
		profileReal.Birthday.Year(), nil, 0, nil, nil,
	)

	lastNominal := nominal[len(nominal)-1].Total
	lastReal := real[len(real)-1].Total

	if lastReal >= lastNominal {
		t.Errorf("real-rate total (%.2f) should be less than nominal (%.2f)", lastReal, lastNominal)
	}
}

// TestSimulate_RealModeDeflatesContributions verifies that in inflation-adjusted
// (real-dollar) mode a fixed nominal contribution loses purchasing power each
// year instead of being silently treated as inflation-indexed. With a 0% nominal
// return the entire projection is just deflated contributions: two years of
// $1,000/mo should land at 24000/1.03 — the year-0 contribution deflated one
// year by the negative real rate, plus the year-1 contribution deflated once.
func TestSimulate_RealModeDeflatesContributions(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")

	profile := newProfile(65, 0)
	profile.UseInflationAdjustment = true
	profile.InflationRate = 0.03

	contrib := models.Contribution{
		AccountID: acc.ID,
		UserID:    userID,
		Amount:    1_000,
		Frequency: "monthly",
		IsActive:  true,
	}

	currentYear := time.Now().Year()
	// Two full past years → no first-year proration.
	result := svc().simulate(
		[]models.Account{acc},
		map[uuid.UUID]float64{acc.ID: 0},
		[]models.Contribution{contrib},
		map[uuid.UUID]float64{acc.ID: 0.0}, // 0% nominal, isolate contribution handling
		nil,
		profile, nil,
		currentYear-2, currentYear-1,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// Year 0: $12k contributed at full value. Year 1: the existing $12k is
	// deflated by the negative real rate (÷1.03) and the new $12k contribution
	// is deflated one year (÷1.03) → total 24000/1.03.
	want := 24_000 / 1.03
	got := result[len(result)-1].Total
	if !approxEqual(got, want) {
		t.Errorf("real-mode contributions: want %.2f, got %.2f", want, got)
	}
}

// ---- computeEffectiveRates tests ----

func TestComputeEffectiveRates_AccountOverrideWins(t *testing.T) {
	userID := uuid.New()
	override := 0.04
	acc := models.Account{
		ID:                 uuid.New(),
		UserID:             userID,
		AccountType:        "brokerage",
		TaxTreatment:       "taxable",
		ReturnRateOverride: &override,
	}

	rates := svc().computeEffectiveRates(
		[]models.Account{acc},
		nil, // no holdings
		map[string]float64{"equity": 0.07},
	)

	if rates[acc.ID] != override {
		t.Errorf("override: want %.4f, got %.4f", override, rates[acc.ID])
	}
}

func TestComputeEffectiveRates_WeightedHoldingsAverage(t *testing.T) {
	userID := uuid.New()
	acc := models.Account{
		ID:          uuid.New(),
		UserID:      userID,
		AccountType: "brokerage",
	}
	priceA := 100.0
	priceB := 100.0
	// 50 shares × $100 = $5k equity; 50 shares × $100 = $5k bond → 50/50 mix
	holdings := []models.Holding{
		{AccountID: acc.ID, UserID: userID, Ticker: "A", AssetClass: "equity", Shares: 50, PricePerShare: &priceA},
		{AccountID: acc.ID, UserID: userID, Ticker: "B", AssetClass: "bond", Shares: 50, PricePerShare: &priceB},
	}

	// equity=10%, bond=4% → weighted avg = 7%
	rateMap := map[string]float64{"equity": 0.10, "bond": 0.04}
	rates := svc().computeEffectiveRates([]models.Account{acc}, holdings, rateMap)

	want := (0.10 + 0.04) / 2
	if math.Abs(rates[acc.ID]-want) > 0.0001 {
		t.Errorf("weighted avg: want %.4f, got %.4f", want, rates[acc.ID])
	}
}

func TestComputeEffectiveRates_FallsBackToAccountTypeDefault(t *testing.T) {
	userID := uuid.New()
	acc := models.Account{
		ID:          uuid.New(),
		UserID:      userID,
		AccountType: "savings", // maps to cash_equivalent (0.5%)
	}

	rates := svc().computeEffectiveRates(
		[]models.Account{acc},
		nil,
		map[string]float64{},
	)

	want := models.DefaultAssetClassReturnRates["cash_equivalent"]
	if math.Abs(rates[acc.ID]-want) > 0.0001 {
		t.Errorf("default fallback: want %.4f, got %.4f", want, rates[acc.ID])
	}
}

// ---- resolveAssetRate tests ----

func TestResolveAssetRate_UserCustomRate(t *testing.T) {
	userRates := map[string]float64{"equity": 0.09}
	got := resolveAssetRate("equity", userRates)
	if got != 0.09 {
		t.Errorf("want 0.09, got %.4f", got)
	}
}

func TestResolveAssetRate_SystemDefaultFallback(t *testing.T) {
	got := resolveAssetRate("equity", map[string]float64{})
	want := models.DefaultAssetClassReturnRates["equity"]
	if got != want {
		t.Errorf("system default: want %.4f, got %.4f", want, got)
	}
}

func TestResolveAssetRate_UnknownClassFallsToOther(t *testing.T) {
	got := resolveAssetRate("nonexistent_asset_class", map[string]float64{})
	want := models.DefaultAssetClassReturnRates["other"]
	if got != want {
		t.Errorf("unknown class fallback: want %.4f, got %.4f", want, got)
	}
}

// ---- buildRateMap tests ----

func TestBuildRateMap(t *testing.T) {
	rates := []models.UserReturnRate{
		{AssetClass: "equity", ReturnRate: 0.08},
		{AssetClass: "bond", ReturnRate: 0.03},
	}
	m := buildRateMap(rates)
	if m["equity"] != 0.08 || m["bond"] != 0.03 {
		t.Errorf("unexpected rate map: %v", m)
	}
}

// ---- Contribution date-range tests ----

// TestSimulate_ContributionWithStartDateDelayed verifies that a contribution
// with a future StartDate does not contribute before that year.
func TestSimulate_ContributionWithStartDateDelayed(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "traditional_401k")
	profile := newProfile(65, 0)

	currentYear := time.Now().Year()
	startDate := time.Date(currentYear+2, 1, 1, 0, 0, 0, 0, time.UTC)

	contrib := models.Contribution{
		AccountID: acc.ID,
		UserID:    userID,
		Amount:    1_000,
		Frequency: "monthly",
		IsActive:  true,
		StartDate: &startDate,
	}

	// Simulate 4 years (index 0–3). Contribution only applies in years 2 and 3.
	result := svc().simulate(
		[]models.Account{acc},
		map[uuid.UUID]float64{acc.ID: 0},
		[]models.Contribution{contrib},
		map[uuid.UUID]float64{acc.ID: 0.0},
		nil,
		profile, nil,
		currentYear, currentYear+3,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// Years 0 and 1: no contribution; years 2 and 3: $12k each → total $24k.
	if !approxEqual(result[1].Total, 0) {
		t.Errorf("year 1 should have no contribution yet, got %.2f", result[1].Total)
	}
	if !approxEqual(result[3].Total, 24_000) {
		t.Errorf("expected $24k after 4 years (contributing only years 2–3), got %.2f", result[3].Total)
	}
}

// TestSimulate_ContributionWithEndDateStops verifies that a contribution
// does not apply after its EndDate year.
func TestSimulate_ContributionWithEndDateStops(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "traditional_401k")
	profile := newProfile(65, 0)

	currentYear := time.Now().Year()
	endDate := time.Date(currentYear+1, 12, 31, 0, 0, 0, 0, time.UTC)

	contrib := models.Contribution{
		AccountID: acc.ID,
		UserID:    userID,
		Amount:    1_000,
		Frequency: "monthly",
		IsActive:  true,
		EndDate:   &endDate,
	}

	// Simulate 4 years (index 0–3). Contribution stops after year 1.
	result := svc().simulate(
		[]models.Account{acc},
		map[uuid.UUID]float64{acc.ID: 0},
		[]models.Contribution{contrib},
		map[uuid.UUID]float64{acc.ID: 0.0},
		nil,
		profile, nil,
		currentYear, currentYear+3,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// Year 0: $12k * fraction (prorated); Year 1: $12k * fraction + $12k = $12k*(1+fraction).
	// Years 2 and 3: no contributions or growth — balance freezes.
	wantFrozen := 12_000 * (1 + currentYearFraction())
	if !approxEqual(result[2].Total, wantFrozen) {
		t.Errorf("year 2 balance should be %.2f (contribution already stopped), got %.2f", wantFrozen, result[2].Total)
	}
	if !approxEqual(result[3].Total, wantFrozen) {
		t.Errorf("year 3 balance should be %.2f (contribution already stopped), got %.2f", wantFrozen, result[3].Total)
	}
}

// TestSimulate_ContributionDateRangeBounded verifies that a contribution with
// both StartDate and EndDate only applies within that window.
func TestSimulate_ContributionDateRangeBounded(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "traditional_401k")
	profile := newProfile(65, 0)

	currentYear := time.Now().Year()
	startDate := time.Date(currentYear+2, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(currentYear+4, 12, 31, 0, 0, 0, 0, time.UTC)

	contrib := models.Contribution{
		AccountID: acc.ID,
		UserID:    userID,
		Amount:    1_000,
		Frequency: "monthly",
		IsActive:  true,
		StartDate: &startDate,
		EndDate:   &endDate,
	}

	// Simulate 6 years (index 0–5). Active window: years 2, 3, 4 → 3 × $12k = $36k.
	result := svc().simulate(
		[]models.Account{acc},
		map[uuid.UUID]float64{acc.ID: 0},
		[]models.Contribution{contrib},
		map[uuid.UUID]float64{acc.ID: 0.0},
		nil,
		profile, nil,
		currentYear, currentYear+5,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	if !approxEqual(result[1].Total, 0) {
		t.Errorf("year 1 should have no contribution yet, got %.2f", result[1].Total)
	}
	if !approxEqual(result[4].Total, 36_000) {
		t.Errorf("expected $36k at end of active window (years 2–4), got %.2f", result[4].Total)
	}
	if !approxEqual(result[5].Total, 36_000) {
		t.Errorf("year 5 balance should be unchanged after end date, got %.2f", result[5].Total)
	}
}

// TestSimulate_AnnualIncreaseRelativeToStartDate verifies that the annual
// increase percentage compounds from the contribution's own start year,
// not from the projection start year.
func TestSimulate_AnnualIncreaseRelativeToStartDate(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "traditional_401k")
	profile := newProfile(65, 0)

	currentYear := time.Now().Year()
	startDate := time.Date(currentYear+2, 1, 1, 0, 0, 0, 0, time.UTC)

	contrib := models.Contribution{
		AccountID:         acc.ID,
		UserID:            userID,
		Amount:            10_000,
		Frequency:         "annually",
		AnnualIncreasePct: 0.10, // 10% annual increase
		IsActive:          true,
		StartDate:         &startDate,
	}

	// Simulate years 0–4. Contribution starts at year 2.
	// year 2 (yearsSinceStart=0): $10,000 × 1.1^0 = $10,000
	// year 3 (yearsSinceStart=1): $10,000 × 1.1^1 = $11,000
	// year 4 (yearsSinceStart=2): $10,000 × 1.1^2 = $12,100
	// Total with zero rate: $33,100
	result := svc().simulate(
		[]models.Account{acc},
		map[uuid.UUID]float64{acc.ID: 0},
		[]models.Contribution{contrib},
		map[uuid.UUID]float64{acc.ID: 0.0},
		nil,
		profile, nil,
		currentYear, currentYear+4,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	wantYear2 := 10_000.0
	wantYear4 := 33_100.0
	if !approxEqual(result[2].Total, wantYear2) {
		t.Errorf("year 2 (first year of contribution): want %.2f, got %.2f", wantYear2, result[2].Total)
	}
	if !approxEqual(result[4].Total, wantYear4) {
		t.Errorf("year 4 (3 years of compounding from start): want %.2f, got %.2f", wantYear4, result[4].Total)
	}
}

// TestSimulate_TwoContributionPeriodsSequential verifies that two sequential
// contribution periods on the same account correctly gate on their date ranges.
func TestSimulate_TwoContributionPeriodsSequential(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "traditional_401k")
	profile := newProfile(65, 0)

	currentYear := time.Now().Year()
	periodAEnd := time.Date(currentYear+2, 12, 31, 0, 0, 0, 0, time.UTC)
	periodBStart := time.Date(currentYear+3, 1, 1, 0, 0, 0, 0, time.UTC)

	// Period A: $1,000/month, applies years 0–2.
	periodA := models.Contribution{
		AccountID: acc.ID,
		UserID:    userID,
		Amount:    1_000,
		Frequency: "monthly",
		IsActive:  true,
		EndDate:   &periodAEnd,
	}
	// Period B: $500/month, applies years 3–4.
	periodB := models.Contribution{
		AccountID: acc.ID,
		UserID:    userID,
		Amount:    500,
		Frequency: "monthly",
		IsActive:  true,
		StartDate: &periodBStart,
	}

	// Simulate 5 years (index 0–4).
	// With zero rate and zero increase:
	// year 0: +$12k → $12k
	// year 1: +$12k → $24k
	// year 2: +$12k → $36k  (last year of A)
	// year 3: +$6k  → $42k  (first year of B)
	// year 4: +$6k  → $48k
	result := svc().simulate(
		[]models.Account{acc},
		map[uuid.UUID]float64{acc.ID: 0},
		[]models.Contribution{periodA, periodB},
		map[uuid.UUID]float64{acc.ID: 0.0},
		nil,
		profile, nil,
		currentYear, currentYear+4,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// Year 0 is prorated: period A contributes $12k * fraction.
	// Subsequent years are full-year, so cumulative amounts shift by (fraction - 1) × $12k.
	frac := currentYearFraction()
	wantYear2 := 12_000 * (frac + 2)
	wantYear3 := 12_000*(frac+2) + 6_000
	wantYear4 := 12_000*(frac+2) + 12_000
	if !approxEqual(result[2].Total, wantYear2) {
		t.Errorf("year 2 (end of period A): want %.2f, got %.2f", wantYear2, result[2].Total)
	}
	if !approxEqual(result[3].Total, wantYear3) {
		t.Errorf("year 3 (first year of period B): want %.2f, got %.2f", wantYear3, result[3].Total)
	}
	if !approxEqual(result[4].Total, wantYear4) {
		t.Errorf("year 4 (second year of period B): want %.2f, got %.2f", wantYear4, result[4].Total)
	}
}

// ---- computeFIMetrics tests ----

// TestComputeFIMetrics_FINumber verifies the FI Number is 25× annual retirement income need.
func TestComputeFIMetrics_FINumber(t *testing.T) {
	profile := newProfile(65, 4_000) // $48k/year expenses → FI Number = $1.2M

	currentPortfolio := 500_000.0
	accounts := []models.Account{}
	rates := map[uuid.UUID]float64{}
	balances := map[uuid.UUID]float64{}

	m := svc().computeFIMetrics(profile, rates, accounts, balances, currentPortfolio, nil, profile.Birthday.Year(), nil)

	wantFI := float64(4_000 * 12 * 25)
	if !approxEqual(m.FINumber, wantFI) {
		t.Errorf("FI Number: want %.0f, got %.2f", wantFI, m.FINumber)
	}
	wantPct := (currentPortfolio / wantFI) * 100
	if math.Abs(m.FIProgressPct-wantPct) > 0.01 {
		t.Errorf("FI Progress: want %.2f%%, got %.2f%%", wantPct, m.FIProgressPct)
	}
}

// TestComputeFIMetrics_FINumberFromMonthlyExpenses verifies that FI Number
// is driven directly by MonthlyExpenses.
func TestComputeFIMetrics_FINumberFromMonthlyExpenses(t *testing.T) {
	profile := newProfile(65, 5_000) // $5k/month → $1.5M FI Number

	m := svc().computeFIMetrics(profile, nil, nil, nil, 0, nil, profile.Birthday.Year(), nil)

	want := float64(5_000 * 12 * 25)
	if !approxEqual(m.FINumber, want) {
		t.Errorf("FI Number: want %.0f, got %.2f", want, m.FINumber)
	}
}

// TestComputeFIMetrics_ZeroFINumberWhenMonthlyExpensesUnset verifies that FI
// Number is 0 when no MonthlyExpenses figure has been set.
func TestComputeFIMetrics_ZeroFINumberWhenMonthlyExpensesUnset(t *testing.T) {
	profile := newProfile(65, 0) // MonthlyExpenses left at 0

	m := svc().computeFIMetrics(profile, nil, nil, nil, 0, nil, profile.Birthday.Year(), nil)

	if m.FINumber != 0 {
		t.Errorf("expected FI Number 0 when MonthlyExpenses is unset, got %.2f", m.FINumber)
	}
}

// TestComputeFIMetrics_SavingsRate verifies savings rate calculation, which
// is driven by CurrentMonthlyExpenses specifically — not MonthlyExpenses
// (assumed retirement spending) — since the two can differ.
func TestComputeFIMetrics_SavingsRate(t *testing.T) {
	profile := newProfile(65, 3_000)
	profile.MonthlyIncome = 10_000
	profile.CurrentMonthlyExpenses = 6_000 // save $4k → 40% savings rate

	m := svc().computeFIMetrics(profile, nil, nil, nil, 0, nil, profile.Birthday.Year(), nil)

	if m.SavingsRate == nil {
		t.Fatal("expected SavingsRate to be set")
	}
	if math.Abs(*m.SavingsRate-0.40) > 0.001 {
		t.Errorf("savings rate: want 0.40, got %.4f", *m.SavingsRate)
	}
}

// TestComputeFIMetrics_WithdrawalRateWarn verifies the 4.5% warning threshold.
func TestComputeFIMetrics_WithdrawalRateWarn(t *testing.T) {
	profile := newProfile(65, 5_000)      // $60k/year withdrawal
	profile.UseInflationAdjustment = true // spending and portfolio both in today's dollars

	// $60k on $1.5M = 4.0% — well below threshold.
	safePortfolio := 60_000.0 / 0.04
	m := svc().computeFIMetrics(profile, nil, nil, nil, safePortfolio, nil, profile.Birthday.Year(), nil)
	if m.WithdrawalRateWarn {
		t.Errorf("expected no warning at 4.0%% withdrawal rate")
	}

	// $60k on $1.2M = 5.0% — above the 4.5% threshold.
	riskyPortfolio := 60_000.0 / 0.05
	m2 := svc().computeFIMetrics(profile, nil, nil, nil, riskyPortfolio, nil, profile.Birthday.Year(), nil)
	if !m2.WithdrawalRateWarn {
		t.Errorf("expected withdrawal rate warning above 4.5%%")
	}
}

// TestComputeFIMetrics_TimeToFI verifies TimeToFIYear is set from the base simulation.
func TestComputeFIMetrics_TimeToFI(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(65, 2_000) // $24k/yr retirement income → FI Number = $600k
	profile.MonthlyExpenses = 2_000
	profile.MonthlyIncome = 10_000

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{acc.ID: 400_000}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.07}

	base := svc().simulate(
		[]models.Account{acc}, startingBalances, nil, effectiveRates,
		nil,
		profile, nil, currentYear, currentYear+30,
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	m := svc().computeFIMetrics(profile, effectiveRates, []models.Account{acc},
		startingBalances, 400_000, base, profile.Birthday.Year(), nil)

	if m.FINumber != 600_000 {
		t.Errorf("FI Number: want 600000, got %.2f", m.FINumber)
	}
	if m.TimeToFIYear == nil {
		t.Error("expected TimeToFIYear to be set — portfolio should reach $600k within 30 years at 7%")
	}
}

// TestEffectiveSWRForProfile verifies the configured withdrawal rate is used
// when set (and positive), falling back to 4% otherwise. RunScenarios'
// SustainableMonthly, ComputeRetirementPlan, and the FI Number all derive
// from this helper.
func TestEffectiveSWRForProfile(t *testing.T) {
	profile := newProfile(65, 4_000)

	if got := effectiveSWRForProfile(profile); got != 0.04 {
		t.Errorf("unset withdrawal rate: want 0.04 default, got %.4f", got)
	}

	profile.WithdrawalRate = ptr(0.035)
	if got := effectiveSWRForProfile(profile); got != 0.035 {
		t.Errorf("configured withdrawal rate: want 0.035, got %.4f", got)
	}

	profile.WithdrawalRate = ptr(0.0)
	if got := effectiveSWRForProfile(profile); got != 0.04 {
		t.Errorf("zero withdrawal rate: want 0.04 default, got %.4f", got)
	}
}

// ---- shockForAccount tests ----
// (computeAssetClassWeights itself is covered in backtest_test.go)

// TestShockForAccount_SameClassSharesShock verifies that two accounts fully
// exposed to the same asset class receive an identical return deviation for
// a given year's draws — the whole point of per-class (rather than
// per-account) randomness.
func TestShockForAccount_SameClassSharesShock(t *testing.T) {
	shocks := map[string]float64{"equity": -1.5, "bond": 0.5}

	a := shockForAccount(map[string]float64{"equity": 1}, shocks)
	b := shockForAccount(map[string]float64{"equity": 1}, shocks)
	if a != b {
		t.Errorf("same-class accounts should share the shock: %.4f vs %.4f", a, b)
	}

	want := models.DefaultAssetClassStdDevs["equity"] * -1.5
	if math.Abs(a-want) > 0.0001 {
		t.Errorf("equity shock: want %.4f, got %.4f", want, a)
	}

	// Mixed account blends the class shocks by weight.
	mixed := shockForAccount(map[string]float64{"equity": 0.5, "bond": 0.5}, shocks)
	wantMixed := 0.5*models.DefaultAssetClassStdDevs["equity"]*-1.5 +
		0.5*models.DefaultAssetClassStdDevs["bond"]*0.5
	if math.Abs(mixed-wantMixed) > 0.0001 {
		t.Errorf("mixed shock: want %.4f, got %.4f", wantMixed, mixed)
	}
}

// ---- runMonteCarlo tests ----

// TestRunMonteCarlo_ReturnsSensiblePct verifies Monte Carlo returns a value in [0,100].
func TestRunMonteCarlo_ReturnsSensiblePct(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(65, 3_000)

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{acc.ID: 500_000}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.07}
	classWeights := map[uuid.UUID]map[string]float64{acc.ID: {"equity": 1}}
	endYear := currentYear + 60

	pct, _ := svc().runMonteCarlo(
		[]models.Account{acc}, startingBalances, nil,
		effectiveRates, classWeights, nil,
		profile, nil,
		currentYear, endYear,
		profile.Birthday.Year(), nil, nil,
		0, nil,
	)

	if pct < 0 || pct > 100 {
		t.Errorf("Monte Carlo result out of range: %.2f", pct)
	}
}

// TestRunMonteCarlo_HighBalanceMostlySucceeds verifies that a large portfolio
// with modest spending has a high success rate.
func TestRunMonteCarlo_HighBalanceMostlySucceeds(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(65, 1_000) // $12k/year on $2M → very low withdrawal rate

	currentYear := time.Now().Year()
	startingBalances := map[uuid.UUID]float64{acc.ID: 2_000_000}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.07}
	classWeights := map[uuid.UUID]map[string]float64{acc.ID: {"equity": 1}}
	endYear := currentYear + 60

	pct, _ := svc().runMonteCarlo(
		[]models.Account{acc}, startingBalances, nil,
		effectiveRates, classWeights, nil,
		profile, nil,
		currentYear, endYear,
		profile.Birthday.Year(), nil, nil,
		0, nil,
	)

	if pct < 80 {
		t.Errorf("expected >80%% success for well-funded portfolio, got %.1f%%", pct)
	}
}

// TestVolatilityDrag_CentersCompoundGrowth checks the correction's math: with
// it, the expected log growth of a random return equals ln(1+rate), so the
// median path compounds at the forecast's rate.
func TestVolatilityDrag_CentersCompoundGrowth(t *testing.T) {
	for _, tc := range []struct{ rate, sd float64 }{{0.07, 0.15}, {0.035, 0.07}, {0.07, 0.40}} {
		mean := tc.rate + volatilityDrag(tc.rate, tc.sd*tc.sd)
		// Second-order expansion of E[ln(1+mean+σZ)].
		logGrowth := math.Log(1+mean) - tc.sd*tc.sd/(2*(1+mean)*(1+mean))
		if diff := math.Abs(logGrowth - math.Log(1+tc.rate)); diff > 0.002 {
			t.Errorf("rate %.3f sd %.2f: log growth %.4f, want %.4f", tc.rate, tc.sd, logGrowth, math.Log(1+tc.rate))
		}
	}
}

// TestRunMonteCarlo_MedianTracksForecast verifies the simulation is centered on
// the deterministic forecast: with no contributions or withdrawals, the median
// (P50) path should grow at the account's expected rate. Without the
// volatility-drag correction it fell about 12% short after 20 years (the
// theoretical drag is ~18%; one 500-run seed puts the median ±~9% off). The
// tolerance covers that sampling noise but not a systematic shortfall.
func TestRunMonteCarlo_MedianTracksForecast(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(90, 0) // no spending, retirement well past the horizon

	currentYear := time.Now().Year()
	const start = 500_000.0
	startingBalances := map[uuid.UUID]float64{acc.ID: start}
	effectiveRates := map[uuid.UUID]float64{acc.ID: 0.07}
	classWeights := map[uuid.UUID]map[string]float64{acc.ID: {"equity": 1}}

	_, bands := svc().runMonteCarlo(
		[]models.Account{acc}, startingBalances, nil,
		effectiveRates, classWeights, nil,
		profile, nil,
		currentYear, currentYear+25,
		profile.Birthday.Year(), nil, nil,
		0, nil,
	)

	const years = 20
	forecast := start * math.Pow(1.07, years)
	median := bands[years].P50
	if diff := (median - forecast) / forecast; math.Abs(diff) > 0.10 {
		t.Errorf("median after %d years = %.0f, forecast = %.0f (%.0f%% off); want within 10%%",
			years, median, forecast, diff*100)
	}
}

// TestPortfolioAtRetirement verifies the retirement balance is read from the
// end of the last working year, not the retirement year (which already has a
// year of withdrawals taken out).
func TestPortfolioAtRetirement(t *testing.T) {
	years := []ForecastYear{
		{Year: 2054, Total: 9_800},
		{Year: 2055, Total: 0}, // first retirement year, after $60k of spending
		{Year: 2056, Total: 0},
	}
	if got := portfolioAtRetirement(years, 2055, 0); got != 9_800 {
		t.Errorf("want end of last working year (9800), got %.0f", got)
	}
	// Retiring in the first projected year: no working year to read.
	if got := portfolioAtRetirement(years, 2054, 0); got != 9_800 {
		t.Errorf("retiring in first projected year: want 9800, got %.0f", got)
	}
	// Already retired: nothing projected, use the fallback.
	if got := portfolioAtRetirement(years, 2040, 123); got != 123 {
		t.Errorf("already retired: want fallback 123, got %.0f", got)
	}
}

// TestComputeFIMetrics_CoastFIUsesRealReturn verifies Coast FI discounts the
// FI number (today's dollars) at the return after inflation.
func TestComputeFIMetrics_CoastFIUsesRealReturn(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(65, 5_000) // 25 years out, FI number $1.5M
	balances := map[uuid.UUID]float64{acc.ID: 100_000}
	rates := map[uuid.UUID]float64{acc.ID: 0.07}

	m := svc().computeFIMetrics(profile, rates, []models.Account{acc}, balances, 100_000, nil, profile.Birthday.Year(), nil)
	want := 1_500_000 / math.Pow(1.07/1.03, 25)
	if !approxEqual(m.CoastFIAmount, want) {
		t.Errorf("coast FI: want %.2f, got %.2f", want, m.CoastFIAmount)
	}
}

// TestFindFICrossing_InflatesFINumber verifies the FI number, in today's
// dollars, is grown to each year's dollars in a nominal projection.
func TestFindFICrossing_InflatesFINumber(t *testing.T) {
	profile := newProfile(65, 0)
	years := []ForecastYear{{Year: 2026, Total: 900_000}, {Year: 2027, Total: 1_020_000}, {Year: 2028, Total: 1_070_000}}
	// $1M today is $1.03M next year and $1.0609M the year after.
	if yr, _ := findFICrossing(years, 1_000_000, profile); yr == nil || *yr != 2028 {
		t.Errorf("nominal: want 2028, got %v", yr)
	}
	profile.UseInflationAdjustment = true
	if yr, _ := findFICrossing(years, 1_000_000, profile); yr == nil || *yr != 2027 {
		t.Errorf("inflation-adjusted: want 2027, got %v", yr)
	}
}

// TestDrawdown verifies the Retirement page's schedule and depletion age come
// from the simulation: balances at the start of each retirement year, and
// the first year spending went unmet.
func TestDrawdown(t *testing.T) {
	years := []ForecastYear{
		{Year: 2040, Age: 64, Total: 500_000},
		{Year: 2041, Age: 65, Total: 450_000},
		{Year: 2042, Age: 66, Total: 0, Shortfall: 10_000},
		{Year: 2043, Age: 67, Total: 0, Shortfall: 60_000},
	}
	schedule, depletion := drawdown(years, 2041, 65, 500_000)
	if len(schedule) != 4 || schedule[0].Balance != 500_000 || schedule[1].Age != 66 || schedule[1].Balance != 450_000 {
		t.Errorf("schedule = %+v", schedule)
	}
	if depletion == nil || *depletion != 66 {
		t.Errorf("depletion age: want 66, got %v", depletion)
	}
}
