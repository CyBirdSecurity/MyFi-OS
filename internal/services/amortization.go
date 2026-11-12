package services

import (
	"math"
	"time"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// AmortizationMonth holds the computed values for one month in a payment schedule.
type AmortizationMonth struct {
	Month              int     `json:"month"`
	Date               string  `json:"date"` // YYYY-MM-DD, first of the payment month
	Payment            float64 `json:"payment"`
	Principal          float64 `json:"principal"`
	Interest           float64 `json:"interest"`
	ExtraPrincipal     float64 `json:"extraPrincipal"`
	Balance            float64 `json:"balance"`
	CumulativeInterest float64 `json:"cumulativeInterest"`
}

// AmortizationSummary holds aggregate stats for one payment scenario.
type AmortizationSummary struct {
	MonthlyPayment float64 `json:"monthlyPayment"`
	TotalInterest  float64 `json:"totalInterest"`
	TotalMonths    int     `json:"totalMonths"`
	PayoffDate     string  `json:"payoffDate"`
	// Housing costs on top of principal and interest. PMIMonthly is 0 once
	// PMI has dropped off; PMIEndDate is when it's scheduled to.
	PropertyTaxMonthly float64 `json:"propertyTaxMonthly"`
	InsuranceMonthly   float64 `json:"insuranceMonthly"`
	HOAMonthly         float64 `json:"hoaMonthly"`
	PMIMonthly         float64 `json:"pmiMonthly"`
	PMIEndDate         *string `json:"pmiEndDate,omitempty"`
	TotalMonthlyCost   float64 `json:"totalMonthlyCost"`
}

// AmortizationWithExtra holds the delta between the base schedule and an
// accelerated schedule where an extra principal payment is made each month.
type AmortizationWithExtra struct {
	TotalInterest float64 `json:"totalInterest"`
	TotalMonths   int     `json:"totalMonths"`
	PayoffDate    string  `json:"payoffDate"`
	InterestSaved float64 `json:"interestSaved"`
	MonthsSaved   int     `json:"monthsSaved"`
}

// AmortizationResult is the full response returned by BuildAmortizationSchedule.
type AmortizationResult struct {
	Schedule  []AmortizationMonth    `json:"schedule"`
	Summary   AmortizationSummary    `json:"summary"`
	WithExtra *AmortizationWithExtra `json:"withExtra,omitempty"`
}

// BuildAmortizationSchedule generates a month-by-month amortization schedule for
// the given home details. extraPayment is an additional principal reduction applied
// each month on top of the regular payment. When extraPayment > 0 the result also
// includes a WithExtra summary showing how much interest and time is saved.
func BuildAmortizationSchedule(hd *models.HomeDetails, extraPayment float64) *AmortizationResult {
	remaining := remainingLoanMonths(hd)
	if remaining <= 0 || hd.CurrentLoanBalance <= 0 {
		summary := AmortizationSummary{PayoffDate: time.Now().Format("2006-01-02")}
		addHousingCosts(&summary, hd, 0, 0)
		return &AmortizationResult{Schedule: []AmortizationMonth{}, Summary: summary}
	}

	monthlyRate := hd.InterestRate / 12
	var monthlyPayment float64
	if monthlyRate > 0 {
		monthlyPayment = hd.CurrentLoanBalance * monthlyRate /
			(1 - math.Pow(1+monthlyRate, -float64(remaining)))
	} else {
		monthlyPayment = hd.CurrentLoanBalance / float64(remaining)
	}

	// Determine the date of the first payment month.
	startDate := time.Now()
	if hd.MortgageStartDate != nil {
		// Advance from mortgage start by the number of elapsed months.
		start := *hd.MortgageStartDate
		elapsed := (startDate.Year()-start.Year())*12 + int(startDate.Month()) - int(start.Month())
		if elapsed < 0 {
			elapsed = 0
		}
		startDate = start.AddDate(0, elapsed, 0)
	}
	// Normalise to the first of the month.
	startDate = time.Date(startDate.Year(), startDate.Month(), 1, 0, 0, 0, 0, time.UTC)

	schedule := buildSchedule(hd.CurrentLoanBalance, monthlyRate, monthlyPayment, 0, startDate)
	summary := AmortizationSummary{
		MonthlyPayment: monthlyPayment,
		TotalMonths:    len(schedule),
		PayoffDate:     payoffDate(schedule),
	}
	for _, m := range schedule {
		summary.TotalInterest += m.Interest
	}
	addHousingCosts(&summary, hd, monthlyRate, monthlyPayment)

	result := &AmortizationResult{Schedule: schedule, Summary: summary}

	if extraPayment > 0 {
		extra := buildSchedule(hd.CurrentLoanBalance, monthlyRate, monthlyPayment, extraPayment, startDate)
		var extraInterest float64
		for _, m := range extra {
			extraInterest += m.Interest
		}
		result.WithExtra = &AmortizationWithExtra{
			TotalInterest: extraInterest,
			TotalMonths:   len(extra),
			PayoffDate:    payoffDate(extra),
			InterestSaved: summary.TotalInterest - extraInterest,
			MonthsSaved:   len(schedule) - len(extra),
		}
	}

	return result
}

// addHousingCosts fills in the costs of owning the home on top of principal
// and interest. PMI is charged until the balance reaches 78% of the purchase
// price (the automatic cutoff), judged against the current home value when no
// purchase price was entered, on the schedule the user actually pays
// (including their saved extra payment).
func addHousingCosts(sum *AmortizationSummary, hd *models.HomeDetails, monthlyRate, monthlyPayment float64) {
	sum.PropertyTaxMonthly = hd.PropertyTaxAnnual / 12
	sum.InsuranceMonthly = hd.HomeInsuranceAnnual / 12
	sum.HOAMonthly = hd.HOAMonthly

	if hd.PMIMonthly > 0 && hd.CurrentLoanBalance > 0 {
		basis := hd.PurchasePrice
		if basis <= 0 && hd.LastZestimate != nil {
			basis = *hd.LastZestimate
		}
		cutoff := 0.78 * basis
		switch {
		case basis <= 0:
			sum.PMIMonthly = hd.PMIMonthly // nothing to judge the cutoff by
		case hd.CurrentLoanBalance > cutoff:
			sum.PMIMonthly = hd.PMIMonthly
			start := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
			for _, m := range buildSchedule(hd.CurrentLoanBalance, monthlyRate, monthlyPayment, hd.ExtraMonthlyPayment, start) {
				if m.Balance <= cutoff {
					d := m.Date
					sum.PMIEndDate = &d
					break
				}
			}
		}
	}

	sum.TotalMonthlyCost = sum.MonthlyPayment + sum.PropertyTaxMonthly + sum.InsuranceMonthly + sum.HOAMonthly + sum.PMIMonthly
}

// RollForwardLoanBalance advances hd.CurrentLoanBalance from the date it was
// entered or synced (LoanBalanceAsOf) to now by applying each monthly payment
// due since, including the saved extra payment, so a manually entered
// mortgage doesn't go stale. It sets LoanBalanceEstimated when it applied any.
func RollForwardLoanBalance(hd *models.HomeDetails, now time.Time) {
	if hd.LoanBalanceAsOf == nil || hd.CurrentLoanBalance <= 0 || hd.LoanTermYears <= 0 {
		return
	}
	asOf := *hd.LoanBalanceAsOf
	months := (now.Year()-asOf.Year())*12 + int(now.Month()) - int(asOf.Month())
	if now.Day() < asOf.Day() {
		months--
	}
	if months <= 0 {
		return
	}
	remaining := remainingLoanMonthsAt(hd, asOf)
	if remaining <= 0 {
		return
	}

	monthlyRate := hd.InterestRate / 12
	payment := hd.CurrentLoanBalance / float64(remaining)
	if monthlyRate > 0 {
		payment = hd.CurrentLoanBalance * monthlyRate / (1 - math.Pow(1+monthlyRate, -float64(remaining)))
	}
	balance := hd.CurrentLoanBalance
	for m := 0; m < months && balance > 0; m++ {
		principal := payment - balance*monthlyRate + hd.ExtraMonthlyPayment
		if principal <= 0 {
			break
		}
		balance -= principal
	}
	if balance < 0 {
		balance = 0
	}
	hd.CurrentLoanBalance = math.Round(balance*100) / 100
	hd.LoanBalanceEstimated = true
}

// LoanBalanceAsOf reconstructs the outstanding loan balance at a past date by
// running the amortization forward from the original loan terms at
// MortgageStartDate. This is an approximation for historical net-worth
// reporting: it assumes ExtraMonthlyPayment (if any) applied from the start
// and doesn't account for unrecorded extra/skipped payments. Returns
// CurrentLoanBalance if the loan has no recorded start date or original value.
func LoanBalanceAsOf(hd *models.HomeDetails, asOf time.Time) float64 {
	if hd.MortgageStartDate == nil || hd.OriginalLoanValue <= 0 || hd.LoanTermYears <= 0 {
		return hd.CurrentLoanBalance
	}
	start := *hd.MortgageStartDate
	if asOf.Before(start) {
		return 0
	}

	monthlyRate := hd.InterestRate / 12
	totalMonths := hd.LoanTermYears * 12
	var monthlyPayment float64
	if monthlyRate > 0 {
		monthlyPayment = hd.OriginalLoanValue * monthlyRate /
			(1 - math.Pow(1+monthlyRate, -float64(totalMonths)))
	} else {
		monthlyPayment = hd.OriginalLoanValue / float64(totalMonths)
	}

	schedule := buildSchedule(hd.OriginalLoanValue, monthlyRate, monthlyPayment, hd.ExtraMonthlyPayment, start)
	elapsed := (asOf.Year()-start.Year())*12 + int(asOf.Month()) - int(start.Month())
	if elapsed <= 0 {
		return hd.OriginalLoanValue
	}
	if elapsed > len(schedule) {
		return 0
	}
	return schedule[elapsed-1].Balance
}

// buildSchedule runs the amortization loop and returns the monthly rows.
func buildSchedule(balance, monthlyRate, monthlyPayment, extraPrincipal float64, start time.Time) []AmortizationMonth {
	var rows []AmortizationMonth
	cumInterest := 0.0
	month := 1
	current := balance

	for current > 0 {
		interest := current * monthlyRate
		principal := monthlyPayment - interest
		if principal <= 0 {
			// Negative amortisation guard — data error, stop to avoid infinite loop.
			break
		}

		extra := extraPrincipal
		if principal+extra > current {
			extra = math.Max(0, current-principal)
		}
		current -= principal + extra
		if current < 0 {
			current = 0
		}

		cumInterest += interest
		date := start.AddDate(0, month-1, 0)
		rows = append(rows, AmortizationMonth{
			Month:              month,
			Date:               date.Format("2006-01-02"),
			Payment:            principal + interest,
			Principal:          principal,
			Interest:           interest,
			ExtraPrincipal:     extra,
			Balance:            current,
			CumulativeInterest: cumInterest,
		})
		month++

		// Safety cap: no real mortgage exceeds 50 years.
		if month > 600 {
			break
		}
	}

	return rows
}

// payoffDate returns the date string of the last row in the schedule, or today if empty.
func payoffDate(schedule []AmortizationMonth) string {
	if len(schedule) == 0 {
		return time.Now().Format("2006-01-02")
	}
	return schedule[len(schedule)-1].Date
}
