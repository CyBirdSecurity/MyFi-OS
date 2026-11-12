package services

import (
	"math"
	"testing"
	"time"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

func testHomeDetails(startDate time.Time) *models.HomeDetails {
	return &models.HomeDetails{
		MortgageStartDate:  &startDate,
		OriginalLoanValue:  300000,
		CurrentLoanBalance: 250000,
		InterestRate:       0.06,
		LoanTermYears:      30,
	}
}

// TestLoanBalanceAsOf_ZeroBeforeStart verifies that no liability is reported
// for dates before the mortgage existed, rather than the full loan amount.
func TestLoanBalanceAsOf_ZeroBeforeStart(t *testing.T) {
	start := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	hd := testHomeDetails(start)

	before := start.AddDate(-1, 0, 0)
	got := LoanBalanceAsOf(hd, before)
	if got != 0 {
		t.Errorf("balance before mortgage start: want 0, got %.2f", got)
	}
}

// TestLoanBalanceAsOf_OriginalValueAtStart verifies the balance at the start
// month equals the full original loan amount.
func TestLoanBalanceAsOf_OriginalValueAtStart(t *testing.T) {
	start := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	hd := testHomeDetails(start)

	got := LoanBalanceAsOf(hd, start)
	if got != hd.OriginalLoanValue {
		t.Errorf("balance at start: want %.2f, got %.2f", hd.OriginalLoanValue, got)
	}
}

// TestLoanBalanceAsOf_IncreasesGoingBackToStart verifies that the reconstructed
// balance is monotonically non-increasing as asOf moves forward in time — i.e.
// it increases the further back toward the mortgage start date you look,
// matching how a loan is actually paid down over time.
func TestLoanBalanceAsOf_IncreasesGoingBackToStart(t *testing.T) {
	start := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
	hd := testHomeDetails(start)

	prev := LoanBalanceAsOf(hd, start)
	for i := 1; i <= 24; i++ {
		asOf := start.AddDate(0, i, 0)
		got := LoanBalanceAsOf(hd, asOf)
		if got > prev {
			t.Fatalf("month %d: balance increased going forward in time (%.2f -> %.2f)", i, prev, got)
		}
		prev = got
	}
	if prev >= hd.OriginalLoanValue {
		t.Errorf("balance should have decreased below original loan value after 24 months, got %.2f", prev)
	}
}

// TestLoanBalanceAsOf_NoStartDateFallsBackToCurrentBalance verifies the
// pre-existing fallback behavior when no mortgage start date is recorded.
func TestLoanBalanceAsOf_NoStartDateFallsBackToCurrentBalance(t *testing.T) {
	hd := &models.HomeDetails{
		CurrentLoanBalance: 250000,
		OriginalLoanValue:  300000,
		LoanTermYears:      30,
	}
	got := LoanBalanceAsOf(hd, time.Now().AddDate(-5, 0, 0))
	if got != hd.CurrentLoanBalance {
		t.Errorf("fallback balance: want %.2f, got %.2f", hd.CurrentLoanBalance, got)
	}
}

// TestRollForwardLoanBalance_AppliesPaymentsSinceAsOf verifies a balance
// entered a year ago is advanced by twelve scheduled payments.
func TestRollForwardLoanBalance_AppliesPaymentsSinceAsOf(t *testing.T) {
	asOf := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
	hd := &models.HomeDetails{CurrentLoanBalance: 200_000, InterestRate: 0.06, LoanTermYears: 30, LoanBalanceAsOf: &asOf}

	RollForwardLoanBalance(hd, time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC))

	// No start date or original value: the full 360 months remain at asOf.
	r := 0.06 / 12
	payment := 200_000 * r / (1 - math.Pow(1+r, -360))
	want := 200_000.0
	for m := 0; m < 12; m++ {
		want -= payment - want*r
	}
	if math.Abs(hd.CurrentLoanBalance-want) > 0.01 {
		t.Errorf("balance: want %.2f, got %.2f", want, hd.CurrentLoanBalance)
	}
	if !hd.LoanBalanceEstimated {
		t.Error("rolled-forward balance should be marked estimated")
	}
}

// TestRollForwardLoanBalance_NoFullMonthYet verifies nothing changes until a
// full payment period has passed since the balance was entered.
func TestRollForwardLoanBalance_NoFullMonthYet(t *testing.T) {
	asOf := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	hd := &models.HomeDetails{CurrentLoanBalance: 200_000, InterestRate: 0.06, LoanTermYears: 30, LoanBalanceAsOf: &asOf}

	RollForwardLoanBalance(hd, time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC))

	if hd.CurrentLoanBalance != 200_000 || hd.LoanBalanceEstimated {
		t.Errorf("want unchanged 200000, got %.2f (estimated=%v)", hd.CurrentLoanBalance, hd.LoanBalanceEstimated)
	}
}

// TestRollForwardLoanBalance_StopsAtZero verifies a loan whose remaining
// payments have all come due rolls forward to fully paid off.
func TestRollForwardLoanBalance_StopsAtZero(t *testing.T) {
	asOf := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	start := time.Date(1994, 6, 1, 0, 0, 0, 0, time.UTC) // 6 months left at asOf
	hd := &models.HomeDetails{CurrentLoanBalance: 6_000, InterestRate: 0.05, LoanTermYears: 30, MortgageStartDate: &start, LoanBalanceAsOf: &asOf}

	RollForwardLoanBalance(hd, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	if hd.CurrentLoanBalance != 0 {
		t.Errorf("want 0 after the last payment, got %.2f", hd.CurrentLoanBalance)
	}
}

// TestBuildAmortizationSchedule_HousingCostsAndPMI verifies escrow costs are
// added to the monthly payment and PMI ends when the balance reaches 78% of
// the purchase price.
func TestBuildAmortizationSchedule_HousingCostsAndPMI(t *testing.T) {
	hd := &models.HomeDetails{
		CurrentLoanBalance:  380_000,
		InterestRate:        0.065,
		LoanTermYears:       30,
		PurchasePrice:       400_000,
		PropertyTaxAnnual:   6_000,
		HomeInsuranceAnnual: 1_800,
		HOAMonthly:          50,
		PMIMonthly:          150,
	}

	res := BuildAmortizationSchedule(hd, 0)
	s := res.Summary

	wantTotal := s.MonthlyPayment + 500 + 150 + 50 + 150
	if math.Abs(s.TotalMonthlyCost-wantTotal) > 0.01 {
		t.Errorf("total monthly cost: want %.2f, got %.2f", wantTotal, s.TotalMonthlyCost)
	}
	if s.PMIEndDate == nil {
		t.Fatal("expected a PMI end date")
	}
	for _, m := range res.Schedule {
		if m.Balance <= 0.78*400_000 {
			if m.Date != *s.PMIEndDate {
				t.Errorf("PMI end: want %s (first month at 78%%), got %s", m.Date, *s.PMIEndDate)
			}
			break
		}
	}

	// Already below 78%: no PMI charged.
	hd.CurrentLoanBalance = 300_000
	if got := BuildAmortizationSchedule(hd, 0).Summary; got.PMIMonthly != 0 || got.PMIEndDate != nil {
		t.Errorf("PMI should have dropped off, got %.2f ending %v", got.PMIMonthly, got.PMIEndDate)
	}

	// Paid off: escrow costs remain.
	hd.CurrentLoanBalance = 0
	if got := BuildAmortizationSchedule(hd, 0).Summary; math.Abs(got.TotalMonthlyCost-700) > 0.01 {
		t.Errorf("paid-off home should still cost tax+insurance+HOA (700), got %.2f", got.TotalMonthlyCost)
	}
}
