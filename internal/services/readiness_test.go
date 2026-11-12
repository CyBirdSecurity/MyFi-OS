package services

import (
	"math"
	"testing"
)

func intPtr(i int) *int { return &i }

// fiMetrics builds metrics for a user who is fiProgress% of the way to a
// $1M FI number today and projected to have projectedPct% at retirement, with
// 85% Monte Carlo success and a 15% savings rate.
func fiMetrics(fiProgress, projectedPct float64, fiYear, fiAge *int) *FIMetrics {
	return &FIMetrics{
		FINumber:                     1_000_000,
		FIProgressPct:                fiProgress,
		ProjectedRetirementPortfolio: projectedPct / 100 * 1_000_000,
		TimeToFIYear:                 fiYear,
		TimeToFIAge:                  fiAge,
		MonteCarloSuccessPct:         85,
		HasMonteCarlo:                true,
		SavingsRate:                  floatPtr(0.15),
	}
}

func TestReadiness_TodaysBalanceDoesNotAffectScore(t *testing.T) {
	// Same projection at retirement; one is 50% there today, one only 5%.
	// A long runway shouldn't be penalized.
	far := computeReadinessScore(fiMetrics(5, 130, intPtr(2052), intPtr(57)), 2055, 2026)
	near := computeReadinessScore(fiMetrics(50, 130, intPtr(2052), intPtr(57)), 2055, 2026)
	if far.Overall != near.Overall {
		t.Errorf("score changed with today's progress: %d vs %d", far.Overall, near.Overall)
	}
}

func TestReadiness_SuccessfulProjectionScoresWell(t *testing.T) {
	s := computeReadinessScore(fiMetrics(50, 130, intPtr(2052), intPtr(57)), 2055, 2026)
	if s.Overall < 75 {
		t.Errorf("projected to reach FI before retirement with typical safety/savings, got %d (%s)", s.Overall, s.Grade)
	}
	if s.Components[0].Name != "On Track" || s.Components[0].Score != 100 {
		t.Errorf("On Track = %+v, want full marks once the projection reaches FI", s.Components[0])
	}
}

func TestReadiness_ScoreRisesWithProjectedCoverage(t *testing.T) {
	prev := -1
	for _, pct := range []float64{20, 40, 60, 80, 100} {
		s := computeReadinessScore(fiMetrics(10, pct, nil, nil), 2055, 2026)
		if s.Overall <= prev {
			t.Errorf("projected %.0f%% scored %d, not above the previous %d", pct, s.Overall, prev)
		}
		prev = s.Overall
	}
}

func TestReadiness_ReachingFIOutscoresFallingShort(t *testing.T) {
	onTrack := computeReadinessScore(fiMetrics(30, 110, intPtr(2053), intPtr(58)), 2055, 2026)
	behind := computeReadinessScore(fiMetrics(30, 70, intPtr(2062), intPtr(67)), 2055, 2026)
	if onTrack.Overall <= behind.Overall {
		t.Errorf("on track %d should beat behind %d", onTrack.Overall, behind.Overall)
	}
}

func TestReadiness_SafetyAndSavingsStillCount(t *testing.T) {
	strong := fiMetrics(50, 130, intPtr(2050), intPtr(55))
	strong.MonteCarloSuccessPct, strong.SavingsRate = 100, floatPtr(0.30)
	weak := fiMetrics(50, 130, intPtr(2050), intPtr(55))
	weak.MonteCarloSuccessPct, weak.SavingsRate = 40, floatPtr(0.02)
	if hi, lo := computeReadinessScore(strong, 2055, 2026), computeReadinessScore(weak, 2055, 2026); hi.Overall <= lo.Overall {
		t.Errorf("strong safety/savings %d should beat weak %d", hi.Overall, lo.Overall)
	}
}

func TestReadiness_NoFINumber(t *testing.T) {
	if s := computeReadinessScore(&FIMetrics{}, 2055, 2026); s != nil {
		t.Errorf("expected no score without an FI number, got %+v", s)
	}
}

func TestReadiness_ZeroMonteCarloSuccessCounts(t *testing.T) {
	m := fiMetrics(50, 130, intPtr(2052), intPtr(57))
	m.MonteCarloSuccessPct = 0
	low := 0.02
	m.WithdrawalRate = &low // would score well if 0% were mistaken for "no data"
	s := computeReadinessScore(m, 2055, 2026)
	if c := s.Components[1]; c.Score != 0 {
		t.Errorf("withdrawal safety with 0%% success: want 0, got %d (%s)", c.Score, c.Insight)
	}
}

func TestReadiness_FINumberGrowsToRetirementDollars(t *testing.T) {
	// A nominal projection of $1M at retirement against a $1M FI number in
	// today's dollars is only partway there once inflation is counted.
	m := fiMetrics(50, 100, nil, nil)
	m.FINumberGrowth = 0.03
	s := computeReadinessScore(m, 2036, 2026)
	want := int(100 / math.Pow(1.03, 10))
	if got := s.Components[0].Score; got != want {
		t.Errorf("on track: want %d, got %d", want, got)
	}
}
