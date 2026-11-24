package usage

import "time"

// Allocation records actual usage and the time charged to each budget.
type Allocation struct {
	Actual   time.Duration
	Regular  time.Duration
	AppBonus time.Duration
}

// AppBonusGrant holds the remaining bonus time for one app.
type AppBonusGrant struct {
	AppID     string
	Remaining time.Duration
	ExpiresAt time.Time
}

// AllocateInterval charges regular time first and applies an app bonus
// only to usage before the grant expires.
func AllocateInterval(
	appID string,
	start, end time.Time,
	regularRemaining time.Duration,
	bonus AppBonusGrant,
) Allocation {
	allocation := Allocate(end.Sub(start), regularRemaining, 0)
	if bonus.AppID != appID || !start.Before(bonus.ExpiresAt) {
		return allocation
	}

	beforeExpiry := min(allocation.Actual, bonus.ExpiresAt.Sub(start))
	eligible := max(beforeExpiry-allocation.Regular, 0)
	allocation.AppBonus = min(eligible, max(bonus.Remaining, 0))

	return allocation
}

// Allocate charges regular time first, then uses the app's remaining bonus.
// Actual includes any elapsed time beyond both available balances.
func Allocate(elapsed, regularRemaining, appBonusRemaining time.Duration) Allocation {
	actual := max(elapsed, 0)
	regular := min(actual, max(regularRemaining, 0))
	appBonus := min(actual-regular, max(appBonusRemaining, 0))

	return Allocation{
		Actual:   actual,
		Regular:  regular,
		AppBonus: appBonus,
	}
}
