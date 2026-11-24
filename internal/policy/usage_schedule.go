package policy

import "time"

// UsageSchedule defines the usage windows for weekdays and weekends.
type UsageSchedule struct {
	WeekdayWindows []UsageWindow
	WeekendWindows []UsageWindow
}

// Allows reports whether at falls within a window for its local day.
func (s UsageSchedule) Allows(at time.Time) bool {
	windows := s.WeekdayWindows
	if at.Weekday() == time.Saturday || at.Weekday() == time.Sunday {
		windows = s.WeekendWindows
	}

	for _, window := range windows {
		if window.Allows(at) {
			return true
		}
	}
	return false
}
