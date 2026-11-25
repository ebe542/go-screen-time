package usage

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"time"
)

type issuedBonusCode struct {
	childID   string
	appID     string
	issuedAt  time.Time
	expiresAt time.Time
	duration  time.Duration
	redeemed  bool
}

// IssueBonusCode creates a one-time code scoped to a child and optional app.
func (l *Ledger) IssueBonusCode(
	childID, appID string,
	issuedAt, expiresAt time.Time,
	duration time.Duration,
) (string, error) {
	if childID == "" || duration <= 0 {
		return "", errors.New("a bonus code requires a child ID and a positive duration")
	}
	if !expiresAt.After(issuedAt) || expiresAt.After(l.NextMidnight(issuedAt)) {
		return "", errors.New("bonus code must expire after issuance and by local midnight")
	}

	l.bonusMu.Lock()
	defer l.bonusMu.Unlock()

	if l.bonusCodes == nil {
		l.bonusCodes = make(map[[32]byte]issuedBonusCode)
	}

	for {
		code := rand.Text()
		digest := sha256.Sum256([]byte(code))
		if _, exists := l.bonusCodes[digest]; exists {
			continue
		}

		l.bonusCodes[digest] = issuedBonusCode{
			childID:   childID,
			appID:     appID,
			issuedAt:  issuedAt,
			expiresAt: expiresAt,
			duration:  duration,
		}
		return code, nil
	}
}

// RedeemBonusCode converts a valid unused code into a bonus grant.
func (l *Ledger) RedeemBonusCode(childID, code string, at time.Time) error {
	digest := sha256.Sum256([]byte(code))

	l.bonusMu.Lock()
	defer l.bonusMu.Unlock()

	entry, exists := l.bonusCodes[digest]
	if !exists || entry.redeemed || entry.childID != childID ||
		at.Before(entry.issuedAt) || !at.Before(entry.expiresAt) {
		return errors.New("bonus code cannot be redeemed")
	}

	if err := l.grantBonusLocked(childID, entry.appID, at, entry.duration); err != nil {
		return err
	}
	entry.redeemed = true
	l.bonusCodes[digest] = entry
	return nil
}
