// Package policy evaluates access rules for managed apps.
// It determines whether a given local time falls within a usage window.
package policy

import (
	"fmt"
	"time"
)

type UsageWindow struct {
	startMinute int
	endMinute   int
}

// NewUsageWindow creates a window with an inclusive start and exclusive end.
func NewUsageWindow(start, end TimeOfDay) (UsageWindow, error) {
	if start.minutes >= end.minutes {
		return UsageWindow{}, fmt.Errorf("usage window start must precede its end")
	}

	return UsageWindow{
		startMinute: start.minutes,
		endMinute:   end.minutes,
	}, nil
}

func (w UsageWindow) Allows(at time.Time) bool {
	minute := at.Hour()*60 + at.Minute()
	return w.startMinute <= minute && minute < w.endMinute
}
