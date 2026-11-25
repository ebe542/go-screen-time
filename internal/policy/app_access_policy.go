package policy

import "time"

// AppAccessPolicy combines the usage rules for one managed app.
type AppAccessPolicy struct {
	Schedule     UsageSchedule
	SharedBudget DailyBudget
	AppLimit     *AppDailyLimit
	BreakRule    BreakRule
}

// DailyBonus holds extra time granted for the current policy day.
type DailyBonus struct {
	Shared       time.Duration
	AppRemaining time.Duration
	ExpiresAt    time.Time
}

// Allows reports whether the app can be used without bonus time.
func (p AppAccessPolicy) Allows(
	at time.Time,
	sharedUsed, appUsed time.Duration,
	breakStartedAt time.Time,
) bool {
	return p.AllowsWithBonus(at, sharedUsed, appUsed, breakStartedAt, DailyBonus{})
}

// AllowsWithBonus reports whether the app can be used with the given bonus.
func (p AppAccessPolicy) AllowsWithBonus(
	at time.Time,
	sharedUsed, appUsed time.Duration,
	breakStartedAt time.Time,
	bonus DailyBonus,
) bool {
	if !breakStartedAt.IsZero() && p.BreakRule.IsActive(at, breakStartedAt) {
		return false
	}
	if !p.Schedule.Allows(at) {
		return false
	}

	limit := p.SharedBudget.LimitFor(at)
	sharedAllows := sharedUsed < limit
	bonusActive := at.Before(bonus.ExpiresAt)
	if !sharedAllows && bonusActive && limit >= 0 {
		sharedAllows = sharedUsed-limit < bonus.Shared
	}

	if sharedAllows && p.AppLimit.Allows(appUsed) {
		return true
	}
	return bonusActive && bonus.AppRemaining > 0
}
