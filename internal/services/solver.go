package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// SolveFor names the single variable a reverse-solve request bisects over,
// holding the other levers fixed.
type SolveFor string

const (
	SolveContribution  SolveFor = "contribution"
	SolveReturnRate    SolveFor = "returnRate"
	SolveRetirementAge SolveFor = "retirementAge"
)

// SolveParams describes a "what would it take" request: pick one variable to
// solve for, holding the other levers at the given absolute value (a nil
// field keeps the current plan's value for that lever). Mirrors ScenarioParams'
// absolute-value convention; solveScenario converts these to internal deltas.
type SolveParams struct {
	SolveFor SolveFor `json:"solveFor"`
	// TargetRetirementAge is the age the solution must reach FI by. Required
	// when solving for contribution or return rate; ignored when solving for
	// retirement age itself, since that's the value being solved for.
	TargetRetirementAge *int `json:"targetRetirementAge"`
	// MonthlyContribution is the absolute total monthly contribution held
	// fixed unless SolveFor is "contribution" (the value being solved for).
	MonthlyContribution *float64 `json:"monthlyContribution"`
	// ReturnRate is the absolute blended return rate held fixed unless
	// SolveFor is "returnRate".
	ReturnRate *float64 `json:"returnRate"`
	// MonthlyExpense is the absolute monthly expense, always applied.
	MonthlyExpense *float64 `json:"monthlyExpense"`
}

// SolveResult is the outcome of a reverse-solve request.
type SolveResult struct {
	SolveFor SolveFor `json:"solveFor"`
	// SolvedValue is an absolute value: a dollar amount (contribution), a
	// rate (returnRate), or an age in years (retirementAge), depending on
	// SolveFor.
	SolvedValue float64        `json:"solvedValue"`
	Achievable  bool           `json:"achievable"`
	Scenario    ScenarioResult `json:"scenario"`
}

const (
	// maxContributionSearchDelta bounds how much additional monthly
	// contribution the solver will consider — generous relative to any
	// plausible household budget.
	maxContributionSearchDelta = 50_000.0
	minReturnRateSearchDelta   = -1.0
	maxReturnRateSearchDelta   = 0.30
	solverBisectionIterations  = 60
	solverMaxRetirementAge     = 100
)

// SolveScenario finds the value of one variable (contribution, return rate,
// or retirement age) needed to reach financial independence by a target age,
// holding the other variables fixed. It reuses the same pure simulate()
// function RunScenarios uses, evaluated repeatedly inside a bisection (for
// the continuous contribution/return-rate cases) or a linear age scan.
func (s *ForecastService) SolveScenario(ctx context.Context, userID uuid.UUID, p SolveParams) (*SolveResult, error) {
	inp, err := s.loadForecastInputs(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.solveScenario(inp, p)
}

// solveScenario holds the actual search logic, separated from DB loading so
// it can be unit-tested against a directly-constructed forecastInputs.
func (s *ForecastService) solveScenario(inp *forecastInputs, p SolveParams) (*SolveResult, error) {
	currentReturnRate := portfolioWeightedRate(inp.allAccounts, inp.startingBalances, inp.effectiveRates, inp.homeDetails)
	currentMonthlyContribution := totalMonthlyContributions(inp.allContribs)

	fixedContributionDelta := 0.0
	if p.MonthlyContribution != nil {
		fixedContributionDelta = *p.MonthlyContribution - currentMonthlyContribution
	}
	fixedReturnRateDelta := 0.0
	if p.ReturnRate != nil {
		fixedReturnRateDelta = *p.ReturnRate - currentReturnRate
	}
	// The FI number depends only on expenses/withdrawal rate, not on
	// contribution, return rate, or retirement age — compute it once.
	expenseAdjustedProfile := *inp.profile
	if p.MonthlyExpense != nil {
		target := *p.MonthlyExpense
		if target < 0 {
			target = 0
		}
		expenseAdjustedProfile.MonthlyExpenses = target
	}
	targetFINumber := fiNumberForProfile(&expenseAdjustedProfile)

	evaluate := func(contributionDelta, returnRateDelta float64, retirementAge int) ScenarioResult {
		profileCopy := *inp.profile
		profileCopy.RetirementAge = retirementAge
		if p.MonthlyExpense != nil {
			target := *p.MonthlyExpense
			if target < 0 {
				target = 0
			}
			profileCopy.MonthlyExpenses = target
		}
		retirementYear := inp.primaryBirthYear + retirementAge

		contribs := inp.allContribs
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

		years := s.simulate(
			inp.allAccounts, inp.startingBalances, contribs, inp.effectiveRates, inp.homeDetails,
			&profileCopy, inp.spouseProfile, inp.currentYear, inp.endYear,
			inp.primaryBirthYear, inp.spouseBirthYear, returnRateDelta, inp.spendGoals, inp.liabilities,
		)

		fiYear, fiAge := findFICrossing(years, fiNumberForProfile(&profileCopy), &profileCopy)

		atRetirement := portfolioAtRetirement(years, retirementYear, 0)

		return ScenarioResult{
			Name:                  "Solved",
			Years:                 years,
			RetirementYear:        retirementYear,
			FIYear:                fiYear,
			FIAge:                 fiAge,
			PortfolioAtRetirement: atRetirement,
			SustainableMonthly:    sustainableMonthly(&profileCopy, atRetirement, retirementYear-inp.currentYear),
		}
	}

	// achievesTarget reports whether the portfolio at targetAge's year meets
	// or exceeds the FI number — i.e. "could retire at this age."
	achievesTarget := func(scenario ScenarioResult, targetAge int) bool {
		targetYear := inp.primaryBirthYear + targetAge
		for _, y := range scenario.Years {
			if y.Year == targetYear {
				return y.Total >= targetFINumber*todayDollarFactor(inp.profile, targetYear-inp.currentYear)
			}
		}
		return false
	}

	switch p.SolveFor {
	case SolveContribution:
		if p.TargetRetirementAge == nil {
			return nil, fmt.Errorf("targetRetirementAge is required when solving for contribution")
		}
		target := *p.TargetRetirementAge
		lo, hi := -currentMonthlyContribution, maxContributionSearchDelta
		reaches := func(delta float64) bool { return achievesTarget(evaluate(delta, fixedReturnRateDelta, target), target) }
		result := bisectSolve(p.SolveFor, lo, hi, reaches, func(v float64) ScenarioResult {
			return evaluate(v, fixedReturnRateDelta, target)
		})
		result.SolvedValue += currentMonthlyContribution // convert the solved delta back to an absolute contribution
		return result, nil

	case SolveReturnRate:
		if p.TargetRetirementAge == nil {
			return nil, fmt.Errorf("targetRetirementAge is required when solving for returnRate")
		}
		target := *p.TargetRetirementAge
		lo, hi := minReturnRateSearchDelta, maxReturnRateSearchDelta
		reaches := func(delta float64) bool {
			return achievesTarget(evaluate(fixedContributionDelta, delta, target), target)
		}
		result := bisectSolve(p.SolveFor, lo, hi, reaches, func(v float64) ScenarioResult {
			return evaluate(fixedContributionDelta, v, target)
		})
		result.SolvedValue += currentReturnRate // convert the solved delta back to an absolute rate
		return result, nil

	case SolveRetirementAge:
		currentAge := inp.currentYear - inp.primaryBirthYear
		for age := currentAge; age <= solverMaxRetirementAge; age++ {
			scenario := evaluate(fixedContributionDelta, fixedReturnRateDelta, age)
			if achievesTarget(scenario, age) {
				return &SolveResult{SolveFor: p.SolveFor, SolvedValue: float64(age), Achievable: true, Scenario: scenario}, nil
			}
		}
		scenario := evaluate(fixedContributionDelta, fixedReturnRateDelta, solverMaxRetirementAge)
		return &SolveResult{SolveFor: p.SolveFor, SolvedValue: float64(solverMaxRetirementAge), Achievable: false, Scenario: scenario}, nil

	default:
		return nil, fmt.Errorf("invalid solveFor: %q", p.SolveFor)
	}
}

// totalMonthlyContributions sums currently-active contributions, normalized
// to a monthly-equivalent amount regardless of their actual frequency, used
// as the lower bound for the contribution-delta search (can't reduce
// contributions by more than eliminating them entirely).
func totalMonthlyContributions(contribs []models.Contribution) float64 {
	total := 0.0
	for _, c := range contribs {
		if c.IsActive {
			total += monthlyEmployeeContribution(c)
		}
	}
	return total
}

// monthlyDeltaForFrequency converts a "$X/mo" scenario delta into the
// equivalent amount for a contribution scheduled at the given frequency, so
// a monthly-contribution-change scenario applies correctly regardless of
// whether the underlying contribution is actually monthly, biweekly (a
// common payroll cadence), or annual — previously the delta was only ever
// applied to contributions with Frequency == "monthly", so it silently had
// no effect on biweekly or annual contributions.
func monthlyDeltaForFrequency(monthlyDelta float64, frequency string) float64 {
	switch frequency {
	case "biweekly":
		return monthlyDelta * 12 / 26
	case "annually":
		return monthlyDelta * 12
	default: // "monthly"
		return monthlyDelta
	}
}

// bisectSolve finds the smallest value in [lo, hi] for which reaches(v) is
// true, assuming reaches is monotonically non-decreasing over the range
// (increasing the solved-for variable never hurts the outcome). If reaches(hi)
// is false, the target isn't achievable within bounds.
func bisectSolve(solveFor SolveFor, lo, hi float64, reaches func(float64) bool, scenarioAt func(float64) ScenarioResult) *SolveResult {
	if !reaches(hi) {
		return &SolveResult{SolveFor: solveFor, SolvedValue: hi, Achievable: false, Scenario: scenarioAt(hi)}
	}
	for i := 0; i < solverBisectionIterations; i++ {
		mid := lo + (hi-lo)/2
		if reaches(mid) {
			hi = mid
		} else {
			lo = mid
		}
	}
	return &SolveResult{SolveFor: solveFor, SolvedValue: hi, Achievable: true, Scenario: scenarioAt(hi)}
}
