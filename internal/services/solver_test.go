package services

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// newSolverInputs builds a minimal forecastInputs for solver tests: one
// brokerage account with a starting balance, one monthly contribution, and
// a profile with a fixed MonthlyExpenses (so fiNumberForProfile is
// well-defined).
func newSolverInputs(retirementAge int, startingBalance, monthlyContribution, effectiveRate float64) *forecastInputs {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(retirementAge, 3_000) // $3k/mo MonthlyExpenses
	currentYear := time.Now().Year()

	contrib := models.Contribution{
		ID:        uuid.New(),
		AccountID: acc.ID,
		UserID:    userID,
		Amount:    monthlyContribution,
		Frequency: "monthly",
		IsActive:  true,
	}

	return &forecastInputs{
		profile:          profile,
		allAccounts:      []models.Account{acc},
		startingBalances: map[uuid.UUID]float64{acc.ID: startingBalance},
		effectiveRates:   map[uuid.UUID]float64{acc.ID: effectiveRate},
		allContribs:      []models.Contribution{contrib},
		homeDetails:      map[uuid.UUID]*models.HomeDetails{},
		primaryBirthYear: profile.Birthday.Year(),
		currentYear:      currentYear,
		endYear:          currentYear + (100 - 40),
	}
}

// newSolverInputsWithExpenseOnly is like newSolverInputs but sets
// MonthlyExpenses directly on the baseline profile — the normal state for a
// user who filled in "Monthly Expenses in Retirement" in Settings.
func newSolverInputsWithExpenseOnly(retirementAge int, startingBalance, effectiveRate, monthlyExpenses float64) *forecastInputs {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(retirementAge, 0)
	profile.MonthlyExpenses = monthlyExpenses
	currentYear := time.Now().Year()

	return &forecastInputs{
		profile:          profile,
		allAccounts:      []models.Account{acc},
		startingBalances: map[uuid.UUID]float64{acc.ID: startingBalance},
		effectiveRates:   map[uuid.UUID]float64{acc.ID: effectiveRate},
		homeDetails:      map[uuid.UUID]*models.HomeDetails{},
		primaryBirthYear: profile.Birthday.Year(),
		currentYear:      currentYear,
		endYear:          currentYear + (100 - 40),
	}
}

// TestSolveScenario_MonthlyExpenseOverrideWins verifies that a scenario's
// MonthlyExpense target overrides the baseline profile's MonthlyExpenses
// when computing the FI number.
func TestSolveScenario_MonthlyExpenseOverrideWins(t *testing.T) {
	// $900k, flat (no growth, no contributions) — can never reach the true
	// $1.2M FI number for a $4,000/mo scenario target (4% SWR). A regression
	// that failed to override the baseline's $3,000/mo MonthlyExpenses would
	// instead compute the FI number from the smaller baseline figure, which
	// $900k would wrongly clear.
	inp := newSolverInputsWithExpenseOnly(65, 900_000, 0.0, 3_000)

	desiredExpense := 4_000.0
	result, err := svc().solveScenario(inp, SolveParams{
		SolveFor:       SolveRetirementAge,
		MonthlyExpense: &desiredExpense,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Achievable {
		t.Errorf("expected NOT achievable: a flat $900k portfolio can never reach the true $1.2M FI number for a $4,000/mo target; got achievable at age %.0f (suggests a bogus, understated FI number)",
			result.SolvedValue)
	}
}

func TestSolveScenario_Contribution_Achievable(t *testing.T) {
	inp := newSolverInputs(65, 100_000, 500, 0.07)
	target := 65

	result, err := svc().solveScenario(inp, SolveParams{
		SolveFor:            SolveContribution,
		TargetRetirementAge: &target,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Achievable {
		t.Fatalf("expected achievable at $50k/mo search ceiling, got: %+v", result)
	}

	// A higher required contribution should let the portfolio actually reach
	// the FI number by the target age — verify the solved scenario clears it.
	fiNumber := fiNumberForProfile(inp.profile)
	if result.Scenario.PortfolioAtRetirement < fiNumber-1 {
		t.Errorf("solved scenario's portfolio at retirement (%.2f) should meet FI number (%.2f)",
			result.Scenario.PortfolioAtRetirement, fiNumber)
	}
}

func TestSolveScenario_Contribution_NotAchievable(t *testing.T) {
	// Retiring next year with almost nothing saved and a tiny return rate —
	// even $50k/mo extra for one year can't reach a real FI number.
	inp := newSolverInputs(41, 1_000, 0, 0.01)
	target := 41

	result, err := svc().solveScenario(inp, SolveParams{
		SolveFor:            SolveContribution,
		TargetRetirementAge: &target,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Achievable {
		t.Fatalf("expected not achievable, got: %+v", result)
	}
	if result.SolvedValue != maxContributionSearchDelta {
		t.Errorf("expected solved value to be the search ceiling %.0f, got %.2f", maxContributionSearchDelta, result.SolvedValue)
	}
}

func TestSolveScenario_ReturnRate_Achievable(t *testing.T) {
	inp := newSolverInputs(65, 200_000, 1_000, 0.0)
	target := 65

	result, err := svc().solveScenario(inp, SolveParams{
		SolveFor:            SolveReturnRate,
		TargetRetirementAge: &target,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Achievable {
		t.Fatalf("expected achievable, got: %+v", result)
	}
	// SolvedValue is an absolute rate (current blended rate + the solved
	// delta), so it must fall within the search bounds *offset* by the
	// current blended rate.
	currentReturnRate := portfolioWeightedRate(inp.allAccounts, inp.startingBalances, inp.effectiveRates, inp.homeDetails)
	lo, hi := currentReturnRate+minReturnRateSearchDelta, currentReturnRate+maxReturnRateSearchDelta
	if result.SolvedValue < lo || result.SolvedValue > hi {
		t.Errorf("solved absolute return rate %.4f out of search bounds [%.4f, %.4f]", result.SolvedValue, lo, hi)
	}
}

// TestSolveScenario_ReturnRate_SolvedValueIsAbsolute proves SolvedValue is a
// true absolute rate, not a raw delta: for a single-account portfolio, the
// applied rate is effectiveRate + delta, so solving for the same target
// should converge to the same absolute rate regardless of the account's
// starting effectiveRate — the delta conversion exactly cancels the baseline.
// A regression back to returning a raw delta would make these diverge by the
// difference between the two baselines (0.06).
func TestSolveScenario_ReturnRate_SolvedValueIsAbsolute(t *testing.T) {
	target := 65
	lowBase := newSolverInputs(65, 200_000, 1_000, 0.02)
	highBase := newSolverInputs(65, 200_000, 1_000, 0.08)

	lowResult, err := svc().solveScenario(lowBase, SolveParams{SolveFor: SolveReturnRate, TargetRetirementAge: &target})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	highResult, err := svc().solveScenario(highBase, SolveParams{SolveFor: SolveReturnRate, TargetRetirementAge: &target})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diff := lowResult.SolvedValue - highResult.SolvedValue; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("expected the solved absolute return rate to be independent of the account's baseline rate: low-baseline=%.6f, high-baseline=%.6f",
			lowResult.SolvedValue, highResult.SolvedValue)
	}
}

func TestSolveScenario_RetirementAge_Achievable(t *testing.T) {
	// Already-large portfolio with healthy contributions should let the
	// solver find a retirement age at or before the profile's original 65.
	inp := newSolverInputs(65, 2_000_000, 2_000, 0.07)

	result, err := svc().solveScenario(inp, SolveParams{SolveFor: SolveRetirementAge})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Achievable {
		t.Fatalf("expected achievable, got: %+v", result)
	}
	currentAge := inp.currentYear - inp.primaryBirthYear
	if result.SolvedValue < float64(currentAge) || result.SolvedValue > 65 {
		t.Errorf("expected solved retirement age between %d and 65, got %.0f", currentAge, result.SolvedValue)
	}
}

// TestSolveScenario_MonthlyExpenseAffectsDrawdown verifies that a
// MonthlyExpense scenario override affects both the FI number and the actual
// per-year withdrawal simulateCore takes in retirement — so lowering planned
// retirement spending changes the solved contribution and the projected
// drawdown.
func TestSolveScenario_MonthlyExpenseAffectsDrawdown(t *testing.T) {
	inp := newSolverInputs(65, 100_000, 500, 0.07)
	target := 65

	baseline, err := svc().solveScenario(inp, SolveParams{
		SolveFor: SolveContribution, TargetRetirementAge: &target,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// newSolverInputs sets a $3,000/mo baseline MonthlyExpenses; a
	// MonthlyExpense scenario override replaces it, so $1,000/mo here means
	// "$2,000/mo less."
	reducedExpense := 1_000.0
	reduced, err := svc().solveScenario(inp, SolveParams{
		SolveFor: SolveContribution, TargetRetirementAge: &target,
		MonthlyExpense: &reducedExpense, // plan to spend $2,000/mo less in retirement
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reduced.SolvedValue >= baseline.SolvedValue {
		t.Errorf("expected lowering retirement spending to reduce the required monthly contribution: baseline=%.2f, reduced=%.2f",
			baseline.SolvedValue, reduced.SolvedValue)
	}

	// The reduced-spending scenario withdraws less in its first full year of
	// retirement — proof the delta reached the actual drawdown simulation,
	// not just the FI number. (The withdrawal is what the 7% growth would
	// have added beyond the year's closing balance.)
	firstWithdrawal := func(years []ForecastYear) float64 {
		retirementYear := inp.primaryBirthYear + target
		for i := 1; i < len(years); i++ {
			if years[i].Year == retirementYear+1 {
				return years[i-1].Total*1.07 - years[i].Total
			}
		}
		t.Fatal("retirement year not in projection")
		return 0
	}
	baselineDraw, reducedDraw := firstWithdrawal(baseline.Scenario.Years), firstWithdrawal(reduced.Scenario.Years)
	if reducedDraw >= baselineDraw {
		t.Errorf("expected the reduced-spending scenario to withdraw less (%.2f) than the baseline (%.2f)", reducedDraw, baselineDraw)
	}
}

// newSolverInputsWithFrequency is like newSolverInputs but lets the test pick
// the seed contribution's frequency, to exercise the non-"monthly" cases.
func newSolverInputsWithFrequency(retirementAge int, startingBalance, contributionAmount, effectiveRate float64, frequency string) *forecastInputs {
	userID := uuid.New()
	acc := account(userID, "brokerage")
	profile := newProfile(retirementAge, 3_000)
	currentYear := time.Now().Year()

	contrib := models.Contribution{
		ID:        uuid.New(),
		AccountID: acc.ID,
		UserID:    userID,
		Amount:    contributionAmount,
		Frequency: frequency,
		IsActive:  true,
	}

	return &forecastInputs{
		profile:          profile,
		allAccounts:      []models.Account{acc},
		startingBalances: map[uuid.UUID]float64{acc.ID: startingBalance},
		effectiveRates:   map[uuid.UUID]float64{acc.ID: effectiveRate},
		allContribs:      []models.Contribution{contrib},
		homeDetails:      map[uuid.UUID]*models.HomeDetails{},
		primaryBirthYear: profile.Birthday.Year(),
		currentYear:      currentYear,
		endYear:          currentYear + (100 - 40),
	}
}

// TestSolveScenario_ContributionAppliesRegardlessOfFrequency guards against a
// bug where the contribution adjustment was only ever applied to
// contributions with Frequency == "monthly", so fixing a higher target
// contribution silently had no effect on biweekly (a common payroll cadence)
// or annual contributions.
func TestSolveScenario_ContributionAppliesRegardlessOfFrequency(t *testing.T) {
	inp := newSolverInputsWithFrequency(65, 100_000, 300, 0.07, "biweekly")
	target := 65

	noExtra, err := svc().solveScenario(inp, SolveParams{
		SolveFor: SolveReturnRate, TargetRetirementAge: &target,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// $300 biweekly = $650/mo — fix an absolute target $2,000/mo above that.
	withExtraTarget := totalMonthlyContributions(inp.allContribs) + 2_000
	withExtra, err := svc().solveScenario(inp, SolveParams{
		SolveFor: SolveReturnRate, TargetRetirementAge: &target,
		MonthlyContribution: &withExtraTarget,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if withExtra.SolvedValue >= noExtra.SolvedValue {
		t.Errorf("expected fixing an extra $2,000/mo contribution to lower the required return rate even though the seed contribution is biweekly: no-extra=%.4f, with-extra=%.4f",
			noExtra.SolvedValue, withExtra.SolvedValue)
	}
}

func TestMonthlyDeltaForFrequency(t *testing.T) {
	cases := []struct {
		frequency string
		delta     float64
		want      float64
	}{
		{"monthly", 120, 120},
		{"biweekly", 120, 120 * 12 / 26},
		{"annually", 120, 120 * 12},
	}
	for _, c := range cases {
		got := monthlyDeltaForFrequency(c.delta, c.frequency)
		if !approxEqual(got, c.want) {
			t.Errorf("monthlyDeltaForFrequency(%.2f, %q) = %.4f, want %.4f", c.delta, c.frequency, got, c.want)
		}
	}
}

func TestSolveScenario_MissingTargetAge_Errors(t *testing.T) {
	inp := newSolverInputs(65, 100_000, 500, 0.07)

	if _, err := svc().solveScenario(inp, SolveParams{SolveFor: SolveContribution}); err == nil {
		t.Error("expected error when targetRetirementAge is missing for SolveContribution")
	}
	if _, err := svc().solveScenario(inp, SolveParams{SolveFor: SolveReturnRate}); err == nil {
		t.Error("expected error when targetRetirementAge is missing for SolveReturnRate")
	}
}

func TestSolveScenario_MonotonicityWithSpendGoal(t *testing.T) {
	// A one-time spend goal midway through accumulation shouldn't break the
	// bisection's monotonicity assumption: a higher contribution should never
	// make the target harder to reach.
	inp := newSolverInputs(65, 150_000, 500, 0.06)
	goalDate := time.Date(inp.currentYear+10, 6, 1, 0, 0, 0, 0, time.UTC)
	inp.spendGoals = []models.Goal{{
		ID:           uuid.New(),
		GoalType:     "spend",
		TargetDate:   &goalDate,
		TargetAmount: 40_000,
	}}
	target := 65

	result, err := svc().solveScenario(inp, SolveParams{
		SolveFor:            SolveContribution,
		TargetRetirementAge: &target,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Achievable {
		t.Fatalf("expected achievable even with a one-time spend goal, got: %+v", result)
	}
}
