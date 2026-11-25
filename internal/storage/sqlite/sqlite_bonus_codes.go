package sqlite

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ebe542/go-screen-time/internal/usage"
)

const (
	sqliteRedemptionWindow = time.Minute
	sqliteRedemptionLimit  = 5
)

var errSQLiteRedemptionDenied = errors.New("bonus code cannot be redeemed")

// IssueBonusCode stores the digest and returns the code to the caller.
func (l *Ledger) IssueBonusCode(
	childID, appID string,
	issuedAt, expiresAt time.Time,
	duration time.Duration,
) (string, error) {
	if childID == "" || duration <= 0 {
		return "", errors.New("a bonus code requires a child ID and a positive duration")
	}
	midnight := usage.NewLedger(l.location).NextMidnight(issuedAt)
	if !expiresAt.After(issuedAt) || expiresAt.After(midnight) {
		return "", errors.New("bonus code must expire after issuance and by local midnight")
	}

	code := rand.Text()
	digest := sha256.Sum256([]byte(code))
	_, err := l.db.Exec(`
		INSERT INTO bonus_codes (digest, child_id, app_id, issued_at, expires_at, duration)
		VALUES (?, ?, ?, ?, ?, ?)
	`, digest[:], childID, appID, issuedAt.UnixNano(), expiresAt.UnixNano(), int64(duration))
	if err != nil {
		return "", err
	}
	return code, nil
}

// RedeemBonusCode consumes a code and records its grant in one transaction.
func (l *Ledger) RedeemBonusCode(childID, code string, at time.Time) error {
	if childID == "" {
		return errSQLiteRedemptionDenied
	}
	digest := sha256.Sum256([]byte(code))
	rejected := false

	err := l.withTx(func(tx *sql.Tx) error {
		windowStart, failures, err := bonusFailuresAt(tx, childID, at)
		if err != nil {
			return err
		}
		if failures >= sqliteRedemptionLimit {
			rejected = true
			return nil
		}

		now := at.UnixNano()
		var appID string
		var duration int64
		err = tx.QueryRow(`
			UPDATE bonus_codes SET redeemed = 1
			WHERE digest = ? AND child_id = ? AND redeemed = 0
				AND issued_at <= ? AND expires_at > ?
			RETURNING app_id, duration
		`, digest[:], childID, now, now).Scan(&appID, &duration)
		if errors.Is(err, sql.ErrNoRows) {
			rejected = true
			return recordBonusFailure(tx, childID, windowStart, failures+1)
		}
		if err != nil {
			return fmt.Errorf("consume bonus code: %w", err)
		}

		return insertSQLiteGrant(tx, childID, appID, at, time.Duration(duration))
	})
	if err != nil {
		return err
	}
	if rejected {
		return errSQLiteRedemptionDenied
	}
	return nil
}

func bonusFailuresAt(tx *sql.Tx, childID string, at time.Time) (int64, int, error) {
	var start int64
	var failures int
	err := tx.QueryRow(`
		SELECT window_start, failures FROM bonus_redemption_attempts WHERE child_id = ?
	`, childID).Scan(&start, &failures)
	if errors.Is(err, sql.ErrNoRows) {
		return at.UnixNano(), 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("load redemption attempts: %w", err)
	}
	if !at.Before(time.Unix(0, start).Add(sqliteRedemptionWindow)) {
		return at.UnixNano(), 0, nil
	}
	return start, failures, nil
}

// RedeemBonusMessageAttempt checks the child's attempt limit before
// redeeming a verified grant. A nil grant counts as a failed attempt.
func (l *Ledger) RedeemBonusMessageAttempt(
	childID string,
	at time.Time,
	grant *usage.VerifiedBonusGrant,
) error {
	if childID == "" {
		return errSQLiteRedemptionDenied
	}

	rejected := false
	err := l.withTx(func(tx *sql.Tx) error {
		windowStart, failures, err := bonusFailuresAt(tx, childID, at)
		if err != nil {
			return err
		}
		if failures >= sqliteRedemptionLimit {
			rejected = true
			return nil
		}

		if grant != nil && grant.ChildID == childID {
			err := redeemVerifiedBonus(tx, *grant, at)
			if !errors.Is(err, errBonusGrantAlreadyRedeemed) {
				return err
			}
		}

		rejected = true
		return recordBonusFailure(tx, childID, windowStart, failures+1)
	})
	if err != nil {
		return err
	}
	if rejected {
		return errSQLiteRedemptionDenied
	}
	return nil
}

func recordBonusFailure(tx *sql.Tx, childID string, windowStart int64, failures int) error {
	_, err := tx.Exec(`
		INSERT INTO bonus_redemption_attempts (child_id, window_start, failures)
		VALUES (?, ?, ?)
		ON CONFLICT (child_id) DO UPDATE SET
			window_start = excluded.window_start,
			failures = excluded.failures
	`, childID, windowStart, failures)
	return err
}
