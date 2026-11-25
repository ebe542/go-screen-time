package usage

import (
	"slices"
	"time"

	"github.com/ebe542/go-screen-time/internal/policy"
)

// SharedBonusAllowance grants extra shared time until local policy midnight.
type SharedBonusAllowance struct {
	Granted   time.Duration
	ExpiresAt time.Time
}

// AppBonuses holds the shared and app-specific allowances for one child.
type AppBonuses struct {
	Shared SharedBonusAllowance
	App    AppBonusAllowance
}

func (b SharedBonusAllowance) availableAt(at time.Time) time.Duration {
	if at.Before(b.ExpiresAt) {
		return max(b.Granted, 0)
	}
	return 0
}

// AllowsApp evaluates access with an optional app-specific bonus.
func (l *Ledger) AllowsApp(
	childID, appID string,
	at time.Time,
	access policy.AppAccessPolicy,
	breakStartedAt time.Time,
	bonus AppBonusAllowance,
) bool {
	return l.AllowsAppWithBonuses(
		childID, appID, at, access, breakStartedAt, AppBonuses{App: bonus},
	)
}

// AllowsAppWithBonuses evaluates access using recorded charges and active bonuses.
func (l *Ledger) AllowsAppWithBonuses(
	childID, appID string,
	at time.Time,
	access policy.AppAccessPolicy,
	breakStartedAt time.Time,
	bonuses AppBonuses,
) bool {
	local := at.In(l.location)
	used := l.BudgetTotals(childID, appID, local)

	available := policy.DailyBonus{
		Shared:    bonuses.Shared.availableAt(local),
		ExpiresAt: bonuses.Shared.ExpiresAt,
	}
	if bonuses.App.AppID == appID && local.Before(bonuses.App.ExpiresAt) {
		available.AppRemaining = remainingTime(bonuses.App.Granted, used.AppBonus)
		available.ExpiresAt = bonuses.App.ExpiresAt
	}

	return access.AllowsWithBonus(
		local, used.SharedRegular, used.AppRegular, breakStartedAt, available,
	)
}

// RecordAppUsage records foreground time using the policy's daily budgets.
func (l *Ledger) RecordAppUsage(
	childID, appID string,
	start, end time.Time,
	access policy.AppAccessPolicy,
	bonuses AppBonuses,
) {
	l.RecordAcrossDaysWithBudget(childID, appID, start, end,
		func(at time.Time) (DailyLimits, AppBonusAllowance) {
			shared := access.SharedBudget.LimitFor(at)
			if shared >= 0 {
				shared += bonuses.Shared.availableAt(at)
			}

			limits := DailyLimits{Shared: shared}
			if access.AppLimit != nil {
				limits.App = &access.AppLimit.Limit
			}
			return limits, bonuses.App
		},
	)
}

// AllowsAppWithGrants evaluates access using the child's stored grants.
func (l *Ledger) AllowsAppWithGrants(
	childID, appID string,
	at time.Time,
	access policy.AppAccessPolicy,
	breakStartedAt time.Time,
) bool {
	return l.AllowsAppWithBonuses(
		childID, appID, at, access, breakStartedAt,
		l.BonusesFor(childID, appID, at),
	)
}

// RecordAppUsageWithGrants records usage using the grants effective in each segment.
func (l *Ledger) RecordAppUsageWithGrants(
	childID, appID string,
	start, end time.Time,
	access policy.AppAccessPolicy,
) {
	l.bonusMu.RLock()
	defer l.bonusMu.RUnlock()

	l.ForEachDaySegment(start, end, func(from, to time.Time) {
		day := l.keyFor(childID, from)
		var boundaries []time.Time
		for _, scope := range []string{"", appID} {
			key := bonusGrantKey{day: day, appID: scope}
			for _, grant := range l.bonusGrants[key] {
				if grant.grantedAt.After(from) && grant.grantedAt.Before(to) {
					boundaries = append(boundaries, grant.grantedAt)
				}
			}
		}
		slices.SortFunc(boundaries, time.Time.Compare)

		for _, boundary := range append(boundaries, to) {
			if from.Before(boundary) {
				bonuses := l.bonusesForLocked(childID, appID, from)
				l.recordAppSegment(childID, appID, from, boundary, access, bonuses)
				from = boundary
			}
		}
	})
}

func (l *Ledger) recordAppSegment(
	childID, appID string,
	start, end time.Time,
	access policy.AppAccessPolicy,
	bonuses AppBonuses,
) {
	local := start.In(l.location)
	shared := access.SharedBudget.LimitFor(local)
	if shared >= 0 {
		shared += bonuses.Shared.availableAt(local)
	}

	limits := DailyLimits{Shared: shared}
	if access.AppLimit != nil {
		limits.App = &access.AppLimit.Limit
	}
	l.RecordWithBudget(childID, appID, start, end, limits, bonuses.App)
}
