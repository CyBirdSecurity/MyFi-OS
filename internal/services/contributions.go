package services

import (
	"math"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// AccountOwner returns the household member whose salary an account's
// contributions are based on: the attributed member, or 'self' for an
// unattributed account. Joint accounts have no single owner, so nil.
func AccountOwner(acc models.Account, members []models.HouseholdMember) *models.HouseholdMember {
	if acc.IsJoint {
		return nil
	}
	for i := range members {
		m := &members[i]
		if acc.MemberID != nil && m.ID == *acc.MemberID {
			return m
		}
		if acc.MemberID == nil && m.Relationship == "self" {
			return m
		}
	}
	return nil
}

// OverlayOwnerIncome fills in each contribution's owner salary and raise.
func OverlayOwnerIncome(contribs []models.Contribution, accounts []models.Account, members []models.HouseholdMember) {
	accountsByID := make(map[uuid.UUID]models.Account, len(accounts))
	for _, acc := range accounts {
		accountsByID[acc.ID] = acc
	}
	for i := range contribs {
		if owner := AccountOwner(accountsByID[contribs[i].AccountID], members); owner != nil {
			contribs[i].OwnerAnnualIncome = AnnualSalary(owner)
			contribs[i].OwnerSalaryGrowth = owner.SalaryGrowthPct
		}
	}
}

// AnnualSalary is the member's yearly income, or 0 when they aren't employed.
func AnnualSalary(m *models.HouseholdMember) float64 {
	if m == nil || !m.IsEmployed {
		return 0
	}
	return m.MonthlyIncome * 12
}

// NeedsIncome reports whether a contribution can only be computed from the
// owner's salary: a percent contribution, or a match capped by salary.
func NeedsIncome(c models.Contribution) bool {
	return c.AmountType == "percent" || c.MatchTiers.HasCap()
}

// paymentsPerYear is how many times a fixed contribution is made each year.
func paymentsPerYear(frequency string) float64 {
	switch frequency {
	case "monthly":
		return 12
	case "biweekly":
		return 26
	}
	return 1 // "annually"
}

// annualContribution returns the employee's contribution and the employer
// match for one year, yearsOut years after today's salary and yearsSinceStart
// years after the contribution began. Amounts are nominal.
func annualContribution(c models.Contribution, yearsOut, yearsSinceStart int) (employee, match float64) {
	salary := c.OwnerAnnualIncome * math.Pow(1+c.OwnerSalaryGrowth, float64(yearsOut))
	if c.AmountType == "percent" {
		employee = c.IncomePct * salary
	} else {
		employee = c.Amount * paymentsPerYear(c.Frequency) * math.Pow(1+c.AnnualIncreasePct, float64(yearsSinceStart))
	}
	return employee, employerMatch(c.MatchTiers, employee, salary)
}

// employerMatch applies each tier in turn to the next slice of the employee
// contribution: a capped tier covers up to UpToPct of salary, an uncapped one
// whatever is left.
func employerMatch(tiers models.MatchTiers, employee, salary float64) float64 {
	remaining, match := employee, 0.0
	for _, t := range tiers {
		if remaining <= 0 {
			break
		}
		portion := remaining
		if t.UpToPct != nil {
			portion = math.Min(remaining, *t.UpToPct*salary)
		}
		match += portion * t.MatchPct
		remaining -= portion
	}
	return match
}

// monthlyEmployeeContribution is a contribution's employee amount today,
// as a monthly equivalent.
func monthlyEmployeeContribution(c models.Contribution) float64 {
	employee, _ := annualContribution(c, 0, 0)
	return employee / 12
}

// addMonthlyContribution changes a contribution by delta dollars a month,
// never below zero. A percent contribution changes its percent of salary.
func addMonthlyContribution(c *models.Contribution, delta float64) {
	if c.AmountType == "percent" {
		if c.OwnerAnnualIncome > 0 {
			c.IncomePct = math.Max(0, c.IncomePct+delta*12/c.OwnerAnnualIncome)
		}
		return
	}
	c.Amount = math.Max(0, c.Amount+monthlyDeltaForFrequency(delta, c.Frequency))
}
