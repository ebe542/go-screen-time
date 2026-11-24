package policy

import "fmt"

// TimeOfDay represents a local clock time at minute precision.
// Its zero value is midnight. The value 24:00 marks the end of the day.
type TimeOfDay struct {
	minutes int
}

// NewTimeOfDay creates a clock time from 00:00 through 24:00.
func NewTimeOfDay(hour, minute int) (TimeOfDay, error) {
	if hour < 0 || hour > 24 || minute < 0 || minute > 59 ||
		(hour == 24 && minute != 0) {
		return TimeOfDay{}, fmt.Errorf("invalid time of day: hour=%d, minute=%d", hour, minute)
	}

	return TimeOfDay{minutes: hour*60 + minute}, nil
}
