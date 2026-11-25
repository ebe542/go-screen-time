// Package usage tracks daily foreground usage for children and managed apps.
package usage

import (
	"sync"
	"time"
)

// Ledger stores daily usage totals in memory.
type Ledger struct {
	location    *time.Location
	days        map[dayKey]*dayTotals
	bonusGrants map[bonusGrantKey][]bonusGrant
	bonusMu     sync.RWMutex
	bonusCodes  map[[32]byte]issuedBonusCode
}

type dayKey struct {
	childID string
	year    int
	month   time.Month
	day     int
}

type dayTotals struct {
	shared        time.Duration
	byApp         map[string]time.Duration
	sharedRegular time.Duration
	budgetByApp   map[string]appBudgetUsage
}

type appBudgetUsage struct {
	regular time.Duration
	bonus   time.Duration
}

// BudgetUsage reports the shared and per-app charges for one policy day.
type BudgetUsage struct {
	SharedRegular time.Duration
	AppRegular    time.Duration
	AppBonus      time.Duration
}

// DailyLimits defines the regular limits for one policy day.
// A nil App limit means the app has no individual cap.
type DailyLimits struct {
	Shared time.Duration
	App    *time.Duration
}

// AppBonusAllowance is the total bonus granted to an app for the selected child.
type AppBonusAllowance struct {
	AppID     string
	Granted   time.Duration
	ExpiresAt time.Time
}

// NewLedger creates a ledger with a non-nil policy time zone.
func NewLedger(location *time.Location) *Ledger {
	return &Ledger{
		location: location,
		days:     make(map[dayKey]*dayTotals),
	}
}

func (l *Ledger) addUsage(childID, appID string, at time.Time, elapsed time.Duration) *dayTotals {
	key := l.keyFor(childID, at)
	totals := l.days[key]
	if totals == nil {
		totals = &dayTotals{byApp: make(map[string]time.Duration)}
		l.days[key] = totals
	}

	totals.shared += elapsed
	totals.byApp[appID] += elapsed
	return totals
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

// RecordAllocation records actual usage and charges within one local day.
// The timestamp at identifies the policy day of the interval.
func (l *Ledger) RecordAllocation(childID, appID string, at time.Time, allocation Allocation) {
	if allocation.Actual <= 0 {
		return
	}

	totals := l.addUsage(childID, appID, at, allocation.Actual)
	totals.sharedRegular += allocation.Regular

	if totals.budgetByApp == nil {
		totals.budgetByApp = make(map[string]appBudgetUsage)
	}

	app := totals.budgetByApp[appID]
	app.regular += allocation.Regular
	app.bonus += allocation.AppBonus
	totals.budgetByApp[appID] = app
}

// BudgetTotals returns the budget charges for the local day containing at.
func (l *Ledger) BudgetTotals(childID, appID string, at time.Time) BudgetUsage {
	totals := l.days[l.keyFor(childID, at)]
	if totals == nil {
		return BudgetUsage{}
	}

	app := totals.budgetByApp[appID]
	return BudgetUsage{
		SharedRegular: totals.sharedRegular,
		AppRegular:    app.regular,
		AppBonus:      app.bonus,
	}
}

// RecordWithBudget allocates and records an interval within one local policy day.
func (l *Ledger) RecordWithBudget(
	childID, appID string,
	start, end time.Time,
	limits DailyLimits,
	bonus AppBonusAllowance,
) Allocation {
	used := l.BudgetTotals(childID, appID, start)
	regular := remainingTime(limits.Shared, used.SharedRegular)
	if limits.App != nil {
		regular = min(regular, remainingTime(*limits.App, used.AppRegular))
	}

	allocation := AllocateInterval(appID, start, end, regular, AppBonusGrant{
		AppID:     bonus.AppID,
		Remaining: remainingTime(bonus.Granted, used.AppBonus),
		ExpiresAt: bonus.ExpiresAt,
	})
	l.RecordAllocation(childID, appID, start, allocation)
	return allocation
}

func remainingTime(limit, used time.Duration) time.Duration {
	if used >= limit {
		return 0
	}
	return limit - used
}

// Record adds a foreground interval and splits it across local calendar days.
func (l *Ledger) Record(childID, appID string, start, end time.Time) {
	l.ForEachDaySegment(start, end, func(segmentStart, segmentEnd time.Time) {
		l.addUsage(childID, appID, segmentStart, segmentEnd.Sub(segmentStart))
	})
}

// RecordAcrossDaysWithBudget records an interval using the limits and bonus
// for each local day. budgetAt receives the segment start in the ledger's zone.
func (l *Ledger) RecordAcrossDaysWithBudget(
	childID, appID string,
	start, end time.Time,
	budgetAt func(at time.Time) (DailyLimits, AppBonusAllowance),
) {
	l.ForEachDaySegment(start, end, func(segmentStart, segmentEnd time.Time) {
		limits, bonus := budgetAt(segmentStart.In(l.location))
		l.RecordWithBudget(childID, appID, segmentStart, segmentEnd, limits, bonus)
	})
}

// ForEachDaySegment visits each non-empty part of an interval within one local day.
func (l *Ledger) ForEachDaySegment(
	start, end time.Time,
	record func(start, end time.Time),
) {
	for start.Before(end) {
		midnight := l.NextMidnight(start)

		segmentEnd := end
		if midnight.After(start) && midnight.Before(end) {
			segmentEnd = midnight
		}

		record(start, segmentEnd)
		start = segmentEnd
	}
}

// NextMidnight returns the next midnight in the policy time zone.
func (l *Ledger) NextMidnight(at time.Time) time.Time {
	year, month, day := at.In(l.location).Date()
	return time.Date(year, month, day+1, 0, 0, 0, 0, l.location)
}
