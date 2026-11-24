package usage

import (
	"time"

	"github.com/ebe542/go-screen-time/internal/policy"
)

// AllowsApp evaluates access using the recorded charges for the local policy day.
func (l *Ledger) AllowsApp(
	childID, appID string,
	at time.Time,
	access policy.AppAccessPolicy,
	breakStartedAt time.Time,
	bonus AppBonusAllowance,
) bool {
	local := at.In(l.location)
	used := l.BudgetTotals(childID, appID, local)

	var availableBonus policy.DailyBonus
	if bonus.AppID == appID {
		availableBonus.AppRemaining = remainingTime(bonus.Granted, used.AppBonus)
		availableBonus.ExpiresAt = bonus.ExpiresAt
	}

	return access.AllowsWithBonus(
		local, used.SharedRegular, used.AppRegular, breakStartedAt, availableBonus,
	)
}
