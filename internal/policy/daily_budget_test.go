package policy

import (
	"testing"
	"time"
)

func TestDailyBudgetAllows(t *testing.T) {
	budget := DailyBudget{
		WeekdayLimit: 2 * time.Hour,
		WeekendLimit: 3 * time.Hour,
	}

	friday := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	saturday := friday.AddDate(0, 0, 1)

	tests := []struct {
		name string
		at   time.Time
		used time.Duration
		want bool
	}{
		{"weekday just below limit", friday, 2*time.Hour - time.Second, true},
		{"weekday at limit", friday, 2 * time.Hour, false},
		{"weekend at weekday limit", saturday, 2 * time.Hour, true},
		{"weekend at limit", saturday, 3 * time.Hour, false},
		{"weekend above limit", saturday, 3*time.Hour + time.Second, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := budget.Allows(tt.at, tt.used); got != tt.want {
				t.Errorf("Allows(%s, %s) = %t, want %t",
					tt.at.Weekday(), tt.used, got, tt.want)
			}
		})
	}
}
