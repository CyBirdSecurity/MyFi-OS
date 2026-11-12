package models

import (
	"testing"
	"time"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestRecurringOccurrence(t *testing.T) {
	cases := []struct {
		name  string
		freq  string
		start time.Time
		n     int
		want  time.Time
	}{
		{"monthly start", "monthly", date(2026, 1, 15), 0, date(2026, 1, 15)},
		{"monthly next", "monthly", date(2026, 1, 15), 1, date(2026, 2, 15)},
		{"monthly across year", "monthly", date(2026, 11, 1), 2, date(2027, 1, 1)},
		{"monthly 31st clamps in Feb", "monthly", date(2026, 1, 31), 1, date(2026, 2, 28)},
		{"monthly 31st returns after Feb", "monthly", date(2026, 1, 31), 2, date(2026, 3, 31)},
		{"monthly 31st in leap Feb", "monthly", date(2028, 1, 31), 1, date(2028, 2, 29)},
		{"biweekly start", "biweekly", date(2026, 1, 2), 0, date(2026, 1, 2)},
		{"biweekly", "biweekly", date(2026, 1, 2), 3, date(2026, 2, 13)},
	}
	for _, c := range cases {
		r := RecurringTransaction{Frequency: c.freq, StartDate: c.start}
		if got := r.Occurrence(c.n); !got.Equal(c.want) {
			t.Errorf("%s: Occurrence(%d) = %s, want %s", c.name, c.n, got.Format("2006-01-02"), c.want.Format("2006-01-02"))
		}
	}
}
