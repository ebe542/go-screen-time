// Package usage tracks daily foreground usage for children and managed apps.
package usage

import "time"

// Ledger stores daily usage totals in memory.
type Ledger struct {
	location *time.Location
	days     map[dayKey]*dayTotals
}

type dayKey struct {
	childID string
	year    int
	month   time.Month
	day     int
}

type dayTotals struct {
	shared time.Duration
	byApp  map[string]time.Duration
}

// NewLedger creates a ledger with a non-nil policy time zone.
func NewLedger(location *time.Location) *Ledger {
	return &Ledger{
		location: location,
		days:     make(map[dayKey]*dayTotals),
	}
}

// Record adds a foreground interval and splits it across local calendar days.
func (l *Ledger) Record(childID, appID string, start, end time.Time) {
	for start.Before(end) {
		year, month, day := start.In(l.location).Date()
		midnight := time.Date(year, month, day+1, 0, 0, 0, 0, l.location)

		segmentEnd := end
		if midnight.After(start) && midnight.Before(end) {
			segmentEnd = midnight
		}

		l.addUsage(childID, appID, start, segmentEnd.Sub(start))
		start = segmentEnd
	}
}

func (l *Ledger) addUsage(childID, appID string, at time.Time, elapsed time.Duration) {
	key := l.keyFor(childID, at)
	totals := l.days[key]
	if totals == nil {
		totals = &dayTotals{byApp: make(map[string]time.Duration)}
		l.days[key] = totals
	}

	totals.shared += elapsed
	totals.byApp[appID] += elapsed
}

// Totals returns the child's shared usage and the selected app's usage for
// the local calendar day containing at.
func (l *Ledger) Totals(childID, appID string, at time.Time) (time.Duration, time.Duration) {
	totals := l.days[l.keyFor(childID, at)]
	if totals == nil {
		return 0, 0
	}

	return totals.shared, totals.byApp[appID]
}

func (l *Ledger) keyFor(childID string, at time.Time) dayKey {
	year, month, day := at.In(l.location).Date()
	return dayKey{childID: childID, year: year, month: month, day: day}
}
