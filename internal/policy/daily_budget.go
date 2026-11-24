package policy

import "time"

// DailyBudget defines the shared daily usage limits for a child.
type DailyBudget struct {
	WeekdayLimit time.Duration
	WeekendLimit time.Duration
}

// Allows reports whether used is below the limit for at's local day.
func (b DailyBudget) Allows(at time.Time, used time.Duration) bool {
	return used < b.limitFor(at)
}

func (b DailyBudget) limitFor(at time.Time) time.Duration {
	if at.Weekday() == time.Saturday || at.Weekday() == time.Sunday {
		return b.WeekendLimit
	}
	return b.WeekdayLimit
}
