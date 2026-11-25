package usage

import (
	"errors"
	"time"
)

type bonusGrantKey struct {
	day   dayKey
	appID string
}

type bonusGrant struct {
	grantedAt time.Time
	duration  time.Duration
}

// GrantBonus grants time to one child for the local day containing at.
// An empty appID grants time to the shared budget.
func (l *Ledger) GrantBonus(childID, appID string, at time.Time, duration time.Duration) error {
	l.bonusMu.Lock()
	defer l.bonusMu.Unlock()

	return l.grantBonusLocked(childID, appID, at, duration)
}

func (l *Ledger) grantBonusLocked(childID, appID string, at time.Time, duration time.Duration) error {
	if childID == "" || duration <= 0 {
		return errors.New("a bonus grant requires a child ID and a positive duration")
	}

	if l.bonusGrants == nil {
		l.bonusGrants = make(map[bonusGrantKey][]bonusGrant)
	}

	key := bonusGrantKey{day: l.keyFor(childID, at), appID: appID}
	l.bonusGrants[key] = append(l.bonusGrants[key], bonusGrant{
		grantedAt: at,
		duration:  duration,
	})
	return nil
}

// BonusesFor returns the allowances effective at the given time.
func (l *Ledger) BonusesFor(childID, appID string, at time.Time) AppBonuses {
	l.bonusMu.RLock()
	defer l.bonusMu.RUnlock()

	return l.bonusesForLocked(childID, appID, at)
}

func (l *Ledger) bonusesForLocked(childID, appID string, at time.Time) AppBonuses {
	day := l.keyFor(childID, at)
	expiresAt := l.NextMidnight(at)

	var bonuses AppBonuses
	if shared := l.bonusGrantedBy(bonusGrantKey{day: day}, at); shared > 0 {
		bonuses.Shared = SharedBonusAllowance{Granted: shared, ExpiresAt: expiresAt}
	}
	if appID != "" {
		key := bonusGrantKey{day: day, appID: appID}
		if app := l.bonusGrantedBy(key, at); app > 0 {
			bonuses.App = AppBonusAllowance{
				AppID:     appID,
				Granted:   app,
				ExpiresAt: expiresAt,
			}
		}
	}
	return bonuses
}

// bonusGrantedBy sums grants up to at while the caller holds bonusMu.
func (l *Ledger) bonusGrantedBy(key bonusGrantKey, at time.Time) time.Duration {
	var total time.Duration
	for _, grant := range l.bonusGrants[key] {
		if !at.Before(grant.grantedAt) {
			total += grant.duration
		}
	}
	return total
}
