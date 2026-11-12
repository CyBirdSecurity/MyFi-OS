package services

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// paychecks returns one entry every two weeks from first through last.
func paychecks(first, last time.Time, amount float64, source string) []models.ContributionEntry {
	var out []models.ContributionEntry
	for d := first; !d.After(last); d = d.AddDate(0, 0, 14) {
		out = append(out, models.ContributionEntry{EntryDate: d, Amount: amount, Source: source})
	}
	return out
}

var plaidProvider = "plaid"

func syncedAccount() models.Account {
	return models.Account{ID: uuid.New(), AccountType: "traditional_401k", ExternalProvider: &plaidProvider}
}

func biweeklyPlan(amount float64) models.Contribution {
	return models.Contribution{ID: uuid.New(), AmountType: "fixed", Amount: amount, Frequency: "biweekly", IsActive: true}
}

func TestProgress_OnPlanBetweenPaychecks(t *testing.T) {
	now := date(2026, 7, 1)
	// 13 paychecks Jan 2 – Jun 19; the plan is measured through Jun 19, not
	// today, so the paycheck due Jul 3 doesn't count against it.
	entries := paychecks(date(2026, 1, 2), date(2026, 6, 19), 500, "employee")
	p := ProgressForAccount(syncedAccount(), []models.Contribution{biweeklyPlan(500)}, entries, now)
	if p == nil {
		t.Fatal("expected progress")
	}
	if p.EmployeeActual != 6_500 {
		t.Errorf("actual = %.2f, want 6500", p.EmployeeActual)
	}
	if !p.AsOf.Equal(date(2026, 6, 20)) {
		t.Errorf("asOf = %v, want Jun 20", p.AsOf)
	}
	if p.EmployeeOffPlan || p.Suggestion != nil {
		t.Errorf("expected on plan, got %+v", p)
	}
}

func TestProgress_StoppedContributingIsOffPlan(t *testing.T) {
	now := date(2026, 7, 1)
	plan := biweeklyPlan(500)
	entries := paychecks(date(2026, 1, 2), date(2026, 3, 27), 500, "employee") // 7 paychecks
	p := ProgressForAccount(syncedAccount(), []models.Contribution{plan}, entries, now)
	if !p.AsOf.Equal(now) {
		t.Errorf("asOf = %v, want today once a paycheck is overdue", p.AsOf)
	}
	if !p.EmployeeOffPlan {
		t.Fatalf("expected off plan: %+v", p)
	}
	s := p.Suggestion
	if s == nil || s.ContributionID == nil || *s.ContributionID != plan.ID {
		t.Fatalf("expected a suggestion for the plan, got %+v", s)
	}
	elapsed := float64(now.Sub(date(2026, 1, 1)).Hours()/24) / 365
	wantPace := 3_500 / elapsed
	if math.Abs(s.AnnualPace-wantPace) > 0.01 || math.Abs(s.Amount-wantPace/26) > 0.01 || s.Frequency != "biweekly" {
		t.Errorf("suggestion = %+v, want pace %.2f", s, wantPace)
	}
}

func TestProgress_PercentPlanSuggestsPercent(t *testing.T) {
	now := date(2026, 7, 1)
	plan := models.Contribution{ID: uuid.New(), AmountType: "percent", IncomePct: 0.06, IsActive: true, OwnerAnnualIncome: 100_000}
	// $10,000 by Jul 1 is well ahead of 6% of $100k.
	entries := []models.ContributionEntry{{EntryDate: date(2026, 6, 30), Amount: 10_000, Source: "employee"}}
	p := ProgressForAccount(syncedAccount(), []models.Contribution{plan}, entries, now)
	if !p.EmployeeOffPlan || p.Suggestion == nil {
		t.Fatalf("expected an off-plan suggestion: %+v", p)
	}
	if p.Suggestion.AmountType != "percent" || math.Abs(p.Suggestion.IncomePct-p.Suggestion.AnnualPace/100_000) > 1e-9 {
		t.Errorf("suggestion = %+v", p.Suggestion)
	}
}

func TestProgress_EmployerBehindOnly(t *testing.T) {
	now := date(2026, 7, 1)
	plan := biweeklyPlan(500)
	plan.MatchTiers = models.MatchTiers{{MatchPct: 0.5}}
	entries := paychecks(date(2026, 1, 2), date(2026, 6, 19), 500, "employee") // no match deposited yet
	p := ProgressForAccount(syncedAccount(), []models.Contribution{plan}, entries, now)
	if p.EmployeeOffPlan || !p.EmployerOffPlan || p.Suggestion != nil {
		t.Errorf("expected only the employer side off plan, got %+v", p)
	}
}

func TestProgress_NoPlanSuggestsOne(t *testing.T) {
	now := date(2026, 7, 1)
	entries := paychecks(date(2026, 1, 2), date(2026, 6, 19), 500, "employee")
	p := ProgressForAccount(syncedAccount(), nil, entries, now)
	if !p.EmployeeOffPlan || p.Suggestion == nil || p.Suggestion.ContributionID != nil || p.Suggestion.Frequency != "monthly" {
		t.Errorf("expected a new monthly plan suggestion, got %+v", p)
	}
}

func TestProgress_NothingToCompare(t *testing.T) {
	now := date(2026, 7, 1)
	plans := []models.Contribution{biweeklyPlan(500)}
	if p := ProgressForAccount(syncedAccount(), plans, nil, now); p != nil {
		t.Errorf("a linked account with no reported contributions should be skipped, got %+v", p)
	}
	manual := models.Account{ID: uuid.New(), AccountType: "roth_ira"}
	lastYear := []models.ContributionEntry{{EntryDate: date(2025, 12, 1), Amount: 500, Source: "employee"}}
	if p := ProgressForAccount(manual, plans, lastYear, now); p != nil {
		t.Errorf("a manual account with nothing logged this year should be skipped, got %+v", p)
	}
	// A linked account that reported contributions last year but none this
	// year has stopped, which is worth flagging.
	p := ProgressForAccount(syncedAccount(), plans, lastYear, now)
	if p == nil || !p.EmployeeOffPlan {
		t.Errorf("expected off plan, got %+v", p)
	}
}
