package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ebe542/go-screen-time/internal/usage"
)

// GrantBonus stores a direct grant for the child and optional app.
func (l *Ledger) GrantBonus(childID, appID string, at time.Time, duration time.Duration) error {
	return insertSQLiteGrant(l.db, childID, appID, at, duration)
}

var errBonusGrantAlreadyRedeemed = errors.New("bonus grant was already redeemed")

// RedeemVerifiedBonus credits an authenticated grant once, without prior issuance.
func (l *Ledger) RedeemVerifiedBonus(grant usage.VerifiedBonusGrant, at time.Time) error {
	return l.withTx(func(tx *sql.Tx) error {
		return redeemVerifiedBonus(tx, grant, at)
	})
}

func redeemVerifiedBonus(tx *sql.Tx, grant usage.VerifiedBonusGrant, at time.Time) error {
	if grant.ID == "" {
		return errors.New("bonus grant ID is required")
	}

	result, err := tx.Exec(`
		INSERT INTO redeemed_bonus_grants (grant_id, redeemed_at)
		VALUES (?, ?)
		ON CONFLICT (grant_id) DO NOTHING
	`, grant.ID, at.UnixNano())
	if err != nil {
		return fmt.Errorf("record bonus redemption: %w", err)
	}

	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check bonus redemption: %w", err)
	}
	if inserted == 0 {
		return errBonusGrantAlreadyRedeemed
	}

	return insertSQLiteGrant(tx, grant.ChildID, grant.AppID, at, grant.Duration)
}

type sqliteExecer interface {
	Exec(string, ...any) (sql.Result, error)
}

func insertSQLiteGrant(
	target sqliteExecer,
	childID, appID string,
	at time.Time,
	duration time.Duration,
) error {
	if childID == "" || duration <= 0 {
		return errors.New("a bonus grant requires a child ID and a positive duration")
	}
	_, err := target.Exec(`
		INSERT INTO bonus_grants (child_id, app_id, granted_at, duration)
		VALUES (?, ?, ?, ?)
	`, childID, appID, at.UnixNano(), int64(duration))
	return err
}
