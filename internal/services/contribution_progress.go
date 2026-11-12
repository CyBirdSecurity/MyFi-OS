package services

import (
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// offPlanThreshold is how far actual contributions can drift from the plan,
// as a share of the planned amount, before the user is told.
const offPlanThreshold = 0.10

// paycheckGrace is how long after the last recorded contribution the plan is
// still measured only through that contribution, so being between paychecks
// doesn't read as falling behind.
const paycheckGrace = 31 * 24 * time.Hour

// ContributionProgress compares this year's actual contributions to an
// account with what its plan expected by AsOf.
type ContributionProgress struct {
	AccountID        uuid.UUID               `json:"accountId"`
	Year             int                     `json:"year"`
	AsOf             time.Time               `json:"asOf"`
	EmployeeActual   float64                 `json:"employeeActual"`
	EmployerActual   float64                 `json:"employerActual"`
	EmployeeExpected float64                 `json:"employeeExpected"`
	EmployerExpected float64                 `json:"employerExpected"`
	EmployeeOffPlan  bool                    `json:"employeeOffPlan"`
	EmployerOffPlan  bool                    `json:"employerOffPlan"`
	Suggestion       *ContributionSuggestion `json:"suggestion,omitempty"`
}

// ContributionSuggestion is a plan change matching this year's pace of
// employee contributions. ContributionID is the plan to update; nil means add
// a new one.
type ContributionSuggestion struct {
	ContributionID *uuid.UUID `json:"contributionId,omitempty"`
	AnnualPace     float64    `json:"annualPace"`
	AmountType     string     `json:"amountType"`
	Amount         float64    `json:"amount"`
	Frequency      string     `json:"frequency"`
	IncomePct      float64    `json:"incomePct"`
}

// ProgressForAccount compares an account's contributions this year with its
// active plans (owner income already overlaid). entries are the account's
// recent entries. Returns nil when there's nothing to compare: a linked
// account whose institution hasn't reported any contributions, or a manual
// account where none were logged this year.
func ProgressForAccount(acc models.Account, plans []models.Contribution, entries []models.ContributionEntry, now time.Time) *ContributionProgress {
	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	p := &ContributionProgress{AccountID: acc.ID, Year: now.Year()}
	var lastEntry *time.Time
	for _, e := range entries {
		if e.EntryDate.Before(yearStart) || e.EntryDate.After(now) {
			continue
		}
		if e.Source == "employer" {
			p.EmployerActual += e.Amount
		} else {
			p.EmployeeActual += e.Amount
		}
		if lastEntry == nil || e.EntryDate.After(*lastEntry) {
			d := e.EntryDate
			lastEntry = &d
		}
	}
	synced := acc.ExternalProvider != nil
	if (synced && len(entries) == 0) || (!synced && lastEntry == nil) {
		return nil
	}

	// Measure through the end of the last contribution's day while the next
	// paycheck could still be on its way, otherwise through today.
	p.AsOf = now
	if lastEntry != nil && now.Sub(*lastEntry) <= paycheckGrace {
		p.AsOf = lastEntry.AddDate(0, 0, 1)
	}

	var active []models.Contribution
	for _, c := range plans {
		if !c.IsActive {
			continue
		}
		active = append(active, c)
		startYear := now.Year()
		if c.StartDate != nil {
			startYear = c.StartDate.Year()
		}
		employee, match := annualContribution(c, 0, now.Year()-startYear)
		f := activeFraction(yearStart, p.AsOf, c.StartDate, c.EndDate)
		p.EmployeeExpected += employee * f
		p.EmployerExpected += match * f
	}
	p.EmployeeOffPlan = offPlan(p.EmployeeActual, p.EmployeeExpected)
	p.EmployerOffPlan = offPlan(p.EmployerActual, p.EmployerExpected)

	if p.EmployeeOffPlan {
		p.Suggestion = suggestPlan(active, p.EmployeeActual, yearStart, p.AsOf, now)
	}
	return p
}

func offPlan(actual, expected float64) bool {
	if expected <= 0 {
		return actual > 0
	}
	return math.Abs(actual-expected)/expected >= offPlanThreshold
}

// suggestPlan turns this year's employee contributions into a yearly pace and
// the plan that would match it. With several active plans there's no single
// one to change, so no suggestion.
func suggestPlan(active []models.Contribution, actual float64, yearStart, asOf, now time.Time) *ContributionSuggestion {
	elapsed := activeFraction(yearStart, asOf, nil, nil)
	if elapsed <= 0 || len(active) > 1 {
		return nil
	}
	pace := actual / elapsed
	if len(active) == 0 {
		if pace <= 0 {
			return nil
		}
		return &ContributionSuggestion{AnnualPace: pace, AmountType: "fixed", Amount: pace / 12, Frequency: "monthly"}
	}

	c := active[0]
	s := &ContributionSuggestion{ContributionID: &c.ID, AnnualPace: pace, AmountType: c.AmountType, Frequency: c.Frequency}
	if c.AmountType == "percent" {
		if c.OwnerAnnualIncome <= 0 {
			return nil
		}
		s.IncomePct = pace / c.OwnerAnnualIncome
		return s
	}
	// Undo this year's scheduled increase so the plan's base amount grows
	// back to the current pace.
	startYear := now.Year()
	if c.StartDate != nil {
		startYear = c.StartDate.Year()
	}
	growth := math.Pow(1+c.AnnualIncreasePct, float64(now.Year()-startYear))
	s.Amount = pace / paymentsPerYear(c.Frequency) / growth
	return s
}
