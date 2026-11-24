package policy

import "time"

// AppAccessPolicy combines the usage rules for one managed app.
type AppAccessPolicy struct {
	Schedule     UsageSchedule
	SharedBudget DailyBudget
	AppLimit     *AppDailyLimit
	BreakRule    BreakRule
}

// Allows reports whether the app can be used at the given local time.
// A zero breakStartedAt means no break has started.
func (p AppAccessPolicy) Allows(
	at time.Time,
	sharedUsed, appUsed time.Duration,
	breakStartedAt time.Time,
) bool {
	if !breakStartedAt.IsZero() && p.BreakRule.IsActive(at, breakStartedAt) {
		return false
	}

	return p.Schedule.Allows(at) &&
		p.SharedBudget.Allows(at, sharedUsed) &&
		p.AppLimit.Allows(appUsed)
}
