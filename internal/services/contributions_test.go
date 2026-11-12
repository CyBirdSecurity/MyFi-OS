package services

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

func pct(v float64) *float64 { return &v }

// 100% of the first 3% of salary, then 50% of the next 2%.
var safeHarbor = models.MatchTiers{{MatchPct: 1, UpToPct: pct(0.03)}, {MatchPct: 0.5, UpToPct: pct(0.02)}}

func TestEmployerMatch_Tiers(t *testing.T) {
	cases := []struct {
		name     string
		employee float64
		want     float64
	}{
		{"inside the first tier", 2_000, 2_000},    // 2% of 100k
		{"across both tiers", 4_000, 3_000 + 500},  // 4%: 3k at 100%, 1k at 50%
		{"above every cap", 10_000, 3_000 + 1_000}, // capped at 5% of salary
		{"no contribution", 0, 0},
	}
	for _, tc := range cases {
		if got := employerMatch(safeHarbor, tc.employee, 100_000); math.Abs(got-tc.want) > 0.01 {
			t.Errorf("%s: got %.2f, want %.2f", tc.name, got, tc.want)
		}
	}
}

func TestEmployerMatch_UncappedTierNeedsNoSalary(t *testing.T) {
	if got := employerMatch(models.MatchTiers{{MatchPct: 0.5}}, 10_000, 0); got != 5_000 {
		t.Errorf("got %.2f, want 5000", got)
	}
}

func TestAnnualContribution_PercentGrowsWithSalary(t *testing.T) {
	c := models.Contribution{
		AmountType:        "percent",
		IncomePct:         0.06,
		MatchTiers:        safeHarbor,
		OwnerAnnualIncome: 100_000,
		OwnerSalaryGrowth: 0.03,
	}
	employee, match := annualContribution(c, 0, 0)
	if employee != 6_000 || match != 4_000 {
		t.Errorf("year 0: got %.2f + %.2f, want 6000 + 4000", employee, match)
	}
	employee, match = annualContribution(c, 2, 2)
	salary := 100_000 * 1.03 * 1.03
	if math.Abs(employee-0.06*salary) > 0.01 || math.Abs(match-0.04*salary) > 0.01 {
		t.Errorf("year 2: got %.2f + %.2f, want %.2f + %.2f", employee, match, 0.06*salary, 0.04*salary)
	}
}

func TestAnnualContribution_FixedAmountCappedBySalary(t *testing.T) {
	c := models.Contribution{
		AmountType:        "fixed",
		Amount:            1_000,
		Frequency:         "monthly",
		MatchTiers:        safeHarbor,
		OwnerAnnualIncome: 100_000,
	}
	employee, match := annualContribution(c, 0, 0)
	if employee != 12_000 || match != 4_000 {
		t.Errorf("got %.2f + %.2f, want 12000 + 4000", employee, match)
	}
}

func TestAddMonthlyContribution(t *testing.T) {
	percent := models.Contribution{AmountType: "percent", IncomePct: 0.05, OwnerAnnualIncome: 120_000}
	addMonthlyContribution(&percent, 100) // +$1,200/yr = +1% of salary
	if math.Abs(percent.IncomePct-0.06) > 1e-9 {
		t.Errorf("percent: got %.4f, want 0.06", percent.IncomePct)
	}
	addMonthlyContribution(&percent, -10_000)
	if percent.IncomePct != 0 {
		t.Errorf("percent: got %.4f, want clamped to 0", percent.IncomePct)
	}

	fixed := models.Contribution{AmountType: "fixed", Amount: 500, Frequency: "monthly"}
	addMonthlyContribution(&fixed, 100)
	if fixed.Amount != 600 {
		t.Errorf("fixed: got %.2f, want 600", fixed.Amount)
	}
}

func TestAccountOwner(t *testing.T) {
	self := models.HouseholdMember{ID: uuid.New(), Relationship: "self"}
	partner := models.HouseholdMember{ID: uuid.New(), Relationship: "partner"}
	members := []models.HouseholdMember{partner, self}

	if got := AccountOwner(models.Account{}, members); got == nil || got.ID != self.ID {
		t.Error("an unattributed account should belong to self")
	}
	if got := AccountOwner(models.Account{MemberID: &partner.ID}, members); got == nil || got.ID != partner.ID {
		t.Error("an attributed account should belong to that member")
	}
	if got := AccountOwner(models.Account{IsJoint: true}, members); got != nil {
		t.Error("a joint account has no single owner")
	}
}

func TestSimulate_PercentContributionUsesSalary(t *testing.T) {
	userID := uuid.New()
	acc := account(userID, "traditional_401k")
	profile := newProfile(65, 0)
	currentYear := time.Now().Year()

	contrib := models.Contribution{
		AccountID:         acc.ID,
		UserID:            userID,
		AmountType:        "percent",
		IncomePct:         0.10,
		MatchTiers:        safeHarbor,
		IsActive:          true,
		OwnerAnnualIncome: 100_000,
	}

	result := svc().simulate(
		[]models.Account{acc},
		map[uuid.UUID]float64{acc.ID: 0},
		[]models.Contribution{contrib},
		map[uuid.UUID]float64{acc.ID: 0},
		nil,
		profile, nil,
		currentYear-1, currentYear-1, // past startYear → no proration
		profile.Birthday.Year(), nil, 0, nil, nil,
	)

	// $10k from the employee plus a $4k match (capped at 5% of salary).
	if !approxEqual(result[0].Total, 14_000) {
		t.Errorf("expected 14000, got %.2f", result[0].Total)
	}
}
