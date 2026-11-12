package services

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// ── Public result types ─────────────────────────────────────────────────────

type TaxAnalysis struct {
	BracketAnalysis BracketAnalysis `json:"bracketAnalysis"`
	RothLadder      RothLadder      `json:"rothLadder"`
	WithdrawalOrder WithdrawalOrder `json:"withdrawalOrder"`
	RMDProjection   []RMDYear       `json:"rmdProjection"`
	ACAAnalysis     ACAAnalysis     `json:"acaAnalysis"`
}

type BracketAnalysis struct {
	FilingStatus      string    `json:"filingStatus"`
	GrossIncome       float64   `json:"grossIncome"`
	StandardDeduction float64   `json:"standardDeduction"`
	EstimatedAGI      float64   `json:"estimatedAgi"`
	FederalTaxOwed    float64   `json:"federalTaxOwed"`
	StateTaxOwed      float64   `json:"stateTaxOwed"`
	TotalTaxOwed      float64   `json:"totalTaxOwed"`
	EffectiveRate     float64   `json:"effectiveRate"`
	MarginalRate      float64   `json:"marginalRate"`
	BracketHeadroom   float64   `json:"bracketHeadroom"` // $ until next bracket
	AllBrackets       []Bracket `json:"allBrackets"`
}

type Bracket struct {
	Rate float64 `json:"rate"`
	Min  float64 `json:"min"`
	Max  float64 `json:"max"` // 0 = top bracket (no ceiling)
}

type RothLadder struct {
	TaxDeferredBalance float64      `json:"taxDeferredBalance"`
	RothBalance        float64      `json:"rothBalance"`
	RecommendedAnnual  float64      `json:"recommendedAnnual"` // 0 when the current bracket is above the projected retirement bracket — converting now isn't recommended
	ConversionAmount   float64      `json:"conversionAmount"`  // effective (recommended or override)
	AnnualTaxCost      float64      `json:"annualTaxCost"`
	ReturnRateUsed     float64      `json:"returnRateUsed"`
	Rungs              []LadderRung `json:"rungs"`
	// RetirementRate is the projected marginal bracket during retirement,
	// nil when there's no retirement income assumption to project from
	// (effectiveRetirementIncome is 0). Single source of truth for the
	// "converting now vs. waiting" comparison — the client should not
	// recompute this independently.
	RetirementRate        *float64 `json:"retirementRate,omitempty"`
	RetirementGrossIncome float64  `json:"retirementGrossIncome"`
}

type LadderRung struct {
	Year          int     `json:"year"`
	Age           int     `json:"age"`
	ConversionAmt float64 `json:"conversionAmount"`
	TaxCost       float64 `json:"taxCost"`
	AccessYear    int     `json:"accessYear"`
	AccessAge     int     `json:"accessAge"`
	TradBalance   float64 `json:"tradBalance"`
	RothBalance   float64 `json:"rothBalance"`
}

type WithdrawalOrder struct {
	Buckets         []WithdrawalBucket `json:"buckets"`
	TotalPortfolio  float64            `json:"totalPortfolio"`
	MonthlySpending float64            `json:"monthlySpending"`
}

type WithdrawalBucket struct {
	TaxTreatment  string       `json:"taxTreatment"`
	Label         string       `json:"label"`
	Balance       float64      `json:"balance"`
	PortfolioPct  float64      `json:"portfolioPct"`
	Priority      int          `json:"priority"`
	YearsOfIncome float64      `json:"yearsOfIncome"` // at monthly retirement spending rate
	TaxNote       string       `json:"taxNote"`
	Accounts      []TaxAccount `json:"accounts"`
}

type TaxAccount struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	AccountType string  `json:"accountType"`
	Balance     float64 `json:"balance"`
}

type RMDYear struct {
	Age          int     `json:"age"`
	Year         int     `json:"year"`
	StartBalance float64 `json:"startBalance"`
	RMDAmount    float64 `json:"rmdAmount"`
	TaxEstimate  float64 `json:"taxEstimate"`
	EndBalance   float64 `json:"endBalance"`
}

type ACAAnalysis struct {
	HouseholdSize    int            `json:"householdSize"`
	FPL              float64        `json:"fpl"`             // 100% FPL for this household
	ProjectedMAGI    float64        `json:"projectedMagi"`   // retirement-era income + conversion
	CliffAmount      float64        `json:"cliffAmount"`     // 400% FPL
	HeadroomToCliff  float64        `json:"headroomToCliff"` // negative = over cliff
	FPLPercent       float64        `json:"fplPercent"`      // MAGI as % of FPL
	SubsidyStatus    string         `json:"subsidyStatus"`   // full_subsidy|partial_subsidy|at_risk|over_cliff
	ConversionImpact float64        `json:"conversionImpact"`
	Thresholds       []ACAThreshold `json:"thresholds"`
}

type ACAThreshold struct {
	Pct    int     `json:"pct"`
	Amount float64 `json:"amount"`
	Label  string  `json:"label"`
}

// ── Service ─────────────────────────────────────────────────────────────────

type TaxService struct {
	accounts         *db.AccountStore
	snapshots        *db.SnapshotStore
	profiles         *db.ProfileStore
	householdMembers *db.HouseholdMembersStore
}

func NewTaxService(
	accounts *db.AccountStore,
	snapshots *db.SnapshotStore,
	profiles *db.ProfileStore,
	householdMembers *db.HouseholdMembersStore,
) *TaxService {
	return &TaxService{accounts: accounts, snapshots: snapshots, profiles: profiles, householdMembers: householdMembers}
}

// Analyze computes the full tax picture for a user.
// conversionOverride, if non-nil, overrides the recommended Roth conversion amount.
func (s *TaxService) Analyze(ctx context.Context, userID uuid.UUID, conversionOverride *float64) (*TaxAnalysis, error) {
	profile, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Derive profile values with safe defaults.
	filingStatus := "single"
	stateRate := 0.0
	householdSize := 1
	annualGross := 0.0
	retirementAge := 65
	monthlySpending := 0.0
	currentAge := 0
	birthYear := 0
	useSS := false
	ssMonthly := 0.0

	if profile != nil {
		filingStatus = profile.FilingStatus
		if filingStatus == "" {
			filingStatus = "single"
		}
		if profile.StateTaxRate != nil {
			stateRate = *profile.StateTaxRate
		}
		monthlySpending = effectiveRetirementIncome(profile)
	}

	// Overlay per-person data from household members.
	members, _ := s.householdMembers.ListByUser(ctx, userID)
	dependentsCount := 0
	if profile != nil {
		dependentsCount = profile.DependentsCount
	}
	householdSize = dependentsCount + len(members)
	if householdSize < 1 {
		householdSize = 1
	}
	for _, m := range members {
		if m.IsEmployed {
			annualGross += m.MonthlyIncome * 12
		}
		if m.Relationship == "self" {
			retirementAge = m.RetirementAge
			useSS = m.UseSocialSecurity
			ssMonthly = m.SocialSecurityMonthly
			if m.Birthday != nil {
				birthYear = m.Birthday.Year()
				currentAge = time.Now().Year() - birthYear
			}
		}
	}

	// Load accounts and latest balances.
	accounts, err := s.accounts.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	latestSnaps, err := s.snapshots.LatestPerAccount(ctx, userID)
	if err != nil {
		return nil, err
	}
	balanceMap := make(map[uuid.UUID]float64, len(latestSnaps))
	for _, sn := range latestSnaps {
		balanceMap[sn.AccountID] = sn.Balance
	}

	// Categorize accounts into tax buckets.
	type bucket struct {
		balance  float64
		accounts []TaxAccount
	}
	buckets := map[string]*bucket{
		"taxable":      {accounts: []TaxAccount{}},
		"tax_deferred": {accounts: []TaxAccount{}},
		"tax_free":     {accounts: []TaxAccount{}},
	}
	for _, acc := range accounts {
		bal := balanceMap[acc.ID]
		b := buckets[acc.TaxTreatment]
		if b == nil {
			continue
		}
		b.balance += bal
		b.accounts = append(b.accounts, TaxAccount{
			ID:          acc.ID.String(),
			Name:        acc.Name,
			AccountType: acc.AccountType,
			Balance:     bal,
		})
	}

	tradBalance := buckets["tax_deferred"].balance
	rothBalance := buckets["tax_free"].balance

	// ── Bracket analysis ──────────────────────────────────────────────────
	brackets := bracketsForStatus(filingStatus)
	stdDed := standardDeduction(filingStatus)
	agi := math.Max(0, annualGross-stdDed)
	bracketAnalysis := computeBracketAnalysis(annualGross, agi, stateRate, brackets, stdDed, filingStatus)

	// ── Retirement-era income projection ────────────────────────────────
	// Working income (annualGross) stops at retirement, so it's the wrong
	// base for anything modeling life after work: deciding whether
	// converting now beats waiting for a lower bracket, and ACA MAGI (which
	// exists specifically for the pre-Medicare retirement gap).
	retirementGrossIncome := monthlySpending * 12
	if useSS {
		retirementGrossIncome += ssMonthly * 12 * 0.85 // IRS up-to-85%-taxable convention
	}
	var retirementRate *float64
	if retirementGrossIncome > 0 {
		retirementAGI := math.Max(0, retirementGrossIncome-stdDed)
		r := marginalRate(retirementAGI, brackets)
		retirementRate = &r
	}

	// ── Roth conversion ladder ────────────────────────────────────────────
	const defaultReturnRate = 0.06
	headroom := bracketAnalysis.BracketHeadroom
	recommendedConversion := math.Max(0, headroom)
	if retirementRate != nil && bracketAnalysis.MarginalRate > *retirementRate {
		// Retirement bracket is lower — converting now costs more tax than
		// waiting, so there's nothing to recommend converting yet.
		recommendedConversion = 0
	}

	conversionAmt := recommendedConversion
	if conversionOverride != nil && *conversionOverride >= 0 {
		conversionAmt = *conversionOverride
	}
	annualTaxCost := federalTaxOnAmount(agi, conversionAmt, brackets)

	years := 10
	if tradBalance <= 0 {
		years = 0
	}
	rungs := buildLadderRungs(tradBalance, rothBalance, conversionAmt, agi, currentAge, defaultReturnRate, brackets, years)

	rothLadder := RothLadder{
		TaxDeferredBalance:    tradBalance,
		RothBalance:           rothBalance,
		RecommendedAnnual:     recommendedConversion,
		ConversionAmount:      conversionAmt,
		AnnualTaxCost:         annualTaxCost,
		ReturnRateUsed:        defaultReturnRate,
		Rungs:                 rungs,
		RetirementRate:        retirementRate,
		RetirementGrossIncome: retirementGrossIncome,
	}

	// ── Withdrawal order ─────────────────────────────────────────────────
	totalPortfolio := buckets["taxable"].balance + buckets["tax_deferred"].balance + buckets["tax_free"].balance
	annualSpending := monthlySpending * 12

	woBuckets := []WithdrawalBucket{
		{
			TaxTreatment: "taxable",
			Label:        "Taxable",
			Balance:      buckets["taxable"].balance,
			Priority:     1,
			TaxNote:      "Withdraw first. Long-term gains taxed at 0–20%; ordinary income rates on dividends and short-term gains. Depleting this preserves tax-advantaged growth.",
			Accounts:     buckets["taxable"].accounts,
		},
		{
			TaxTreatment: "tax_deferred",
			Label:        "Tax-Deferred",
			Balance:      buckets["tax_deferred"].balance,
			Priority:     2,
			TaxNote:      "Withdraw second. Every dollar is ordinary income — coordinate with Social Security and Roth conversions to stay in a low bracket. RMDs begin at 73.",
			Accounts:     buckets["tax_deferred"].accounts,
		},
		{
			TaxTreatment: "tax_free",
			Label:        "Tax-Free (Roth / HSA)",
			Balance:      buckets["tax_free"].balance,
			Priority:     3,
			TaxNote:      "Withdraw last. Qualified distributions are tax-free with no RMDs. Let it compound as long as possible — it's your most valuable dollar in retirement.",
			Accounts:     buckets["tax_free"].accounts,
		},
	}
	for i := range woBuckets {
		b := &woBuckets[i]
		b.PortfolioPct = 0
		if totalPortfolio > 0 {
			b.PortfolioPct = b.Balance / totalPortfolio * 100
		}
		b.YearsOfIncome = 0
		if annualSpending > 0 && b.Balance > 0 {
			b.YearsOfIncome = b.Balance / annualSpending
		}
	}

	withdrawalOrder := WithdrawalOrder{
		Buckets:         woBuckets,
		TotalPortfolio:  totalPortfolio,
		MonthlySpending: monthlySpending,
	}

	// ── RMD projection ────────────────────────────────────────────────────
	rmdProjection := computeRMDProjection(tradBalance, currentAge, birthYear, agi, brackets, defaultReturnRate, retirementAge)

	// ── ACA analysis ──────────────────────────────────────────────────────
	// ACA subsidies matter for the retirement gap before Medicare (65), so
	// MAGI is retirement-era income (withdrawals/spending + taxable SS) plus
	// the conversion — not current working income, which stops at retirement.
	fpl := acaFPL(householdSize)
	projectedMAGI := retirementGrossIncome + conversionAmt
	cliffAmount := fpl * 4.0
	headroomToCliff := cliffAmount - projectedMAGI
	fplPct := 0.0
	if fpl > 0 {
		fplPct = projectedMAGI / fpl * 100
	}

	subsidyStatus := "full_subsidy"
	switch {
	case projectedMAGI > cliffAmount:
		subsidyStatus = "over_cliff"
	case projectedMAGI > cliffAmount*0.9:
		subsidyStatus = "at_risk" // within 10% of cliff
	case projectedMAGI > fpl*1.5:
		subsidyStatus = "partial_subsidy"
	}

	acaThresholds := []ACAThreshold{
		{100, fpl * 1.0, "Medicaid floor"},
		{150, fpl * 1.5, "Maximum savings"},
		{200, fpl * 2.0, "Enhanced savings"},
		{250, fpl * 2.5, "Cost-sharing reductions"},
		{400, fpl * 4.0, "Subsidy cliff"},
	}

	acaAnalysis := ACAAnalysis{
		HouseholdSize:    householdSize,
		FPL:              fpl,
		ProjectedMAGI:    projectedMAGI,
		CliffAmount:      cliffAmount,
		HeadroomToCliff:  headroomToCliff,
		FPLPercent:       fplPct,
		SubsidyStatus:    subsidyStatus,
		ConversionImpact: conversionAmt,
		Thresholds:       acaThresholds,
	}

	return &TaxAnalysis{
		BracketAnalysis: bracketAnalysis,
		RothLadder:      rothLadder,
		WithdrawalOrder: withdrawalOrder,
		RMDProjection:   rmdProjection,
		ACAAnalysis:     acaAnalysis,
	}, nil
}

// ── Bracket helpers ──────────────────────────────────────────────────────────

type fedBracket struct{ rate, min, max float64 }

// 2026 brackets — TCJA extended with inflation adjustment (IRS Rev. Proc. 2025-xx).
var single2026 = []fedBracket{
	{0.10, 0, 12150}, {0.12, 12150, 49450}, {0.22, 49450, 105450},
	{0.24, 105450, 201050}, {0.32, 201050, 255350}, {0.35, 255350, 639300},
	{0.37, 639300, 0},
}

var mfj2026 = []fedBracket{
	{0.10, 0, 24300}, {0.12, 24300, 98900}, {0.22, 98900, 210900},
	{0.24, 210900, 402100}, {0.32, 402100, 510700}, {0.35, 510700, 770150},
	{0.37, 770150, 0},
}

func bracketsForStatus(status string) []fedBracket {
	if status == "married_filing_jointly" {
		return mfj2026
	}
	return single2026
}

func standardDeduction(status string) float64 {
	if status == "married_filing_jointly" {
		return 30000
	}
	return 15000
}

func computeFederalTax(agi float64, brackets []fedBracket) float64 {
	tax := 0.0
	for _, b := range brackets {
		if agi <= b.min {
			break
		}
		top := b.max
		if top == 0 || agi < top {
			top = agi
		}
		tax += (top - b.min) * b.rate
	}
	return tax
}

// federalTaxOnAmount returns the marginal federal tax on adding `amount` to an existing AGI.
func federalTaxOnAmount(baseAGI, amount float64, brackets []fedBracket) float64 {
	if amount <= 0 {
		return 0
	}
	return computeFederalTax(baseAGI+amount, brackets) - computeFederalTax(baseAGI, brackets)
}

func marginalRate(agi float64, brackets []fedBracket) float64 {
	rate := brackets[0].rate
	for _, b := range brackets {
		if agi >= b.min {
			rate = b.rate
		}
	}
	return rate
}

func headroomInBracket(agi float64, brackets []fedBracket) float64 {
	for _, b := range brackets {
		if b.max > 0 && agi < b.max {
			return b.max - agi
		}
	}
	return 0 // in top bracket
}

func computeBracketAnalysis(gross, agi, stateRate float64, brackets []fedBracket, stdDed float64, filingStatus string) BracketAnalysis {
	fedTax := computeFederalTax(agi, brackets)
	stateTax := gross * stateRate
	total := fedTax + stateTax
	effectiveRate := 0.0
	if gross > 0 {
		effectiveRate = total / gross
	}

	allBrackets := make([]Bracket, len(brackets))
	for i, b := range brackets {
		allBrackets[i] = Bracket{Rate: b.rate, Min: b.min, Max: b.max}
	}

	return BracketAnalysis{
		FilingStatus:      filingStatus,
		GrossIncome:       gross,
		StandardDeduction: stdDed,
		EstimatedAGI:      agi,
		FederalTaxOwed:    fedTax,
		StateTaxOwed:      stateTax,
		TotalTaxOwed:      total,
		EffectiveRate:     effectiveRate,
		MarginalRate:      marginalRate(agi, brackets),
		BracketHeadroom:   headroomInBracket(agi, brackets),
		AllBrackets:       allBrackets,
	}
}

// ── Roth ladder ──────────────────────────────────────────────────────────────

func buildLadderRungs(tradBalance, rothBalance, annualConversion, baseAGI float64, currentAge int, returnRate float64, brackets []fedBracket, years int) []LadderRung {
	if years == 0 {
		return []LadderRung{}
	}
	rungs := make([]LadderRung, 0, years)
	trad := tradBalance
	roth := rothBalance
	nowYear := time.Now().Year()

	for i := 0; i < years; i++ {
		trad *= 1 + returnRate
		roth *= 1 + returnRate

		conv := math.Min(annualConversion, math.Max(0, trad))
		taxCost := federalTaxOnAmount(baseAGI, conv, brackets)

		trad -= conv
		roth += conv

		age := currentAge + i
		year := nowYear + i
		rungs = append(rungs, LadderRung{
			Year:          year,
			Age:           age,
			ConversionAmt: conv,
			TaxCost:       taxCost,
			AccessYear:    year + 5,
			AccessAge:     age + 5,
			TradBalance:   trad,
			RothBalance:   roth,
		})
	}
	return rungs
}

// ── RMD projection ───────────────────────────────────────────────────────────

func computeRMDProjection(tradBalance float64, currentAge, birthYear int, baseAGI float64, brackets []fedBracket, returnRate float64, retirementAge int) []RMDYear {
	if tradBalance <= 0 || currentAge <= 0 {
		return []RMDYear{}
	}
	nowYear := time.Now().Year()

	// Project balance forward to age 73 (or use current if already past 73).
	startAge := 73
	if currentAge > 73 {
		startAge = currentAge
	}
	yearsToProject := startAge - currentAge
	balance := tradBalance * math.Pow(1+returnRate, float64(yearsToProject))

	var projection []RMDYear
	for age := startAge; age <= 90 && balance > 0; age++ {
		factor := rmdDistributionPeriod(age)
		rmd := balance / factor
		if rmd > balance {
			rmd = balance
		}
		taxEst := federalTaxOnAmount(baseAGI, rmd, brackets)
		startBal := balance
		balance = (balance - rmd) * (1 + returnRate)

		year := birthYear + age
		if birthYear == 0 {
			year = nowYear + (age - currentAge)
		}
		projection = append(projection, RMDYear{
			Age:          age,
			Year:         year,
			StartBalance: startBal,
			RMDAmount:    rmd,
			TaxEstimate:  taxEst,
			EndBalance:   balance,
		})
	}
	return projection
}

// ── ACA helpers ──────────────────────────────────────────────────────────────

// acaFPL returns the 2026 Federal Poverty Level for the given household size (continental US).
func acaFPL(householdSize int) float64 {
	if householdSize <= 0 {
		householdSize = 1
	}
	return 16100 + float64(householdSize-1)*5670
}

// ── Unused helper alias so models import stays clean ─────────────────────────

var _ = models.DefaultAssetClassReturnRates // ensure import used
