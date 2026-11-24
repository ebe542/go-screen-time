package policy

import "time"

// DailyBudget defines the shared daily usage limits for a child.
type DailyBudget struct {
	WeekdayLimit time.Duration
	WeekendLimit time.Duration
}

// Allows reports whether used is below the limit for at's local day.
func (b DailyBudget) Allows(at time.Time, used time.Duration) bool {
	limit := b.WeekdayLimit
	if at.Weekday() == time.Saturday || at.Weekday() == time.Sunday {
		limit = b.WeekendLimit
	}

	return used < limit
}
