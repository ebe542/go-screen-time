package policy

import "time"

// AppAccessPolicy combines the usage rules for one managed app.
type AppAccessPolicy struct {
	Schedule     UsageSchedule
	SharedBudget DailyBudget
	AppLimit     *AppDailyLimit
}

// Allows reports whether the app can be used at the given local time.
func (p AppAccessPolicy) Allows(at time.Time, sharedUsed, appUsed time.Duration) bool {
	return p.Schedule.Allows(at) &&
		p.SharedBudget.Allows(at, sharedUsed) &&
		p.AppLimit.Allows(appUsed)
}
