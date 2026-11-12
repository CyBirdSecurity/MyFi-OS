package api

import (
	"math"
	"testing"
)

func TestValidBudgetLimit(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
		ok   bool
	}{
		{"whole dollars", 450, 450, true},
		{"rounds to cents", 19.999, 20, true},
		{"keeps cents", 12.34, 12.34, true},
		{"smallest allowed", 0.01, 0.01, true},
		{"largest allowed", maxBudgetLimit, maxBudgetLimit, true},
		{"zero", 0, 0, false},
		{"negative", -5, 0, false},
		{"rounds down to zero", 0.004, 0, false},
		{"too large", maxBudgetLimit + 1, 0, false},
		{"not a number", math.NaN(), 0, false},
		{"infinite", math.Inf(1), 0, false},
	}
	for _, c := range cases {
		got, ok := validBudgetLimit(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("%s: validBudgetLimit(%v) = %v, %v; want %v, %v", c.name, c.in, got, ok, c.want, c.ok)
		}
	}
}
