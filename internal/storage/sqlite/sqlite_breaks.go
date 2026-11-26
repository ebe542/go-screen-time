package sqlite

import (
	"database/sql"
	"errors"
	"time"

	"github.com/ebe542/go-screen-time/internal/policy"
)

// LoadBreakState reads the saved state for one child.
func (l *Ledger) LoadBreakState(childID string) (policy.BreakState, error) {
	var state policy.BreakState
	var startedAt sql.NullInt64
	err := l.db.QueryRow(`
		SELECT used_nanoseconds, started_at
		FROM break_states
		WHERE child_id = ?
	`, childID).Scan(&state.Used, &startedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return policy.BreakState{}, nil
	}
	if err != nil {
		return policy.BreakState{}, err
	}
	if startedAt.Valid {
		state.StartedAt = time.Unix(0, startedAt.Int64).UTC()
	}
	return state, nil
}

// SaveBreakState stores a state change that does not book app usage.
func (l *Ledger) SaveBreakState(childID string, state policy.BreakState) error {
	return saveBreakState(l.db, childID, state)
}

func saveBreakState(db sqliteExecer, childID string, state policy.BreakState) error {
	var startedAt any
	if !state.StartedAt.IsZero() {
		startedAt = state.StartedAt.UnixNano()
	}
	_, err := db.Exec(`
		INSERT INTO break_states (child_id, used_nanoseconds, started_at)
		VALUES (?, ?, ?)
		ON CONFLICT (child_id) DO UPDATE SET
			used_nanoseconds = excluded.used_nanoseconds,
			started_at = excluded.started_at
	`, childID, int64(state.Used), startedAt)
	return err
}

// RecordAppUsageAndBreakState saves usage and break state in one transaction.
func (l *Ledger) RecordAppUsageAndBreakState(
	childID, appID string,
	start, end time.Time,
	access policy.AppAccessPolicy,
	state policy.BreakState,
) error {
	return l.withTx(func(tx *sql.Tx) error {
		if err := l.recordAppUsage(tx, childID, appID, start, end, access); err != nil {
			return err
		}
		return saveBreakState(tx, childID, state)
	})
}
