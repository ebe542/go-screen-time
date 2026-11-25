package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ebe542/go-screen-time/internal/policy"
	"github.com/ebe542/go-screen-time/internal/usage"
)

// Ledger persists grants, code state, and daily usage charges.
type Ledger struct {
	db       *sql.DB
	location *time.Location
}

var _ usage.Store = (*Ledger)(nil)

// Open opens a file-backed ledger in the policy time zone.
func Open(path string, location *time.Location) (*Ledger, error) {
	if path == "" || location == nil {
		return nil, errors.New("SQLite ledger requires a path and a policy time zone")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve SQLite ledger path: %w", err)
	}
	slashPath := filepath.ToSlash(absolute)
	if !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	options := url.Values{
		"_pragma": {"busy_timeout(5000)"},
		"_txlock": {"immediate"},
	}
	dsn := url.URL{Scheme: "file", Path: slashPath, RawQuery: options.Encode()}

	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, fmt.Errorf("open SQLite ledger: %w", err)
	}
	db.SetMaxOpenConns(1)

	ledger := &Ledger{db: db, location: location}
	if err := ledger.withTx(applyMigrations); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize SQLite ledger: %w", err)
	}
	return ledger, nil
}

// Close closes the database.
func (l *Ledger) Close() error {
	return l.db.Close()
}

func (l *Ledger) withTx(work func(*sql.Tx) error) error {
	tx, err := l.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := work(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// BonusesFor returns the stored grants effective at the given time.
func (l *Ledger) BonusesFor(childID, appID string, at time.Time) (usage.AppBonuses, error) {
	var result usage.AppBonuses
	err := l.withTx(func(tx *sql.Tx) error {
		day, err := l.loadDay(tx, childID, at)
		if err != nil {
			return err
		}
		result = day.BonusesFor(childID, appID, at)
		return nil
	})
	return result, err
}

// BudgetTotals returns the stored charges for the local policy day.
func (l *Ledger) BudgetTotals(childID, appID string, at time.Time) (usage.BudgetUsage, error) {
	var result usage.BudgetUsage
	err := l.withTx(func(tx *sql.Tx) error {
		day, err := l.loadDay(tx, childID, at)
		if err != nil {
			return err
		}
		result = day.BudgetTotals(childID, appID, at)
		return nil
	})
	return result, err
}

// AllowsAppWithGrants evaluates access using stored charges and grants.
func (l *Ledger) AllowsAppWithGrants(
	childID, appID string,
	at time.Time,
	access policy.AppAccessPolicy,
	breakStartedAt time.Time,
) (bool, error) {
	var allowed bool
	err := l.withTx(func(tx *sql.Tx) error {
		day, err := l.loadDay(tx, childID, at)
		if err != nil {
			return err
		}
		allowed = day.AllowsAppWithGrants(childID, appID, at, access, breakStartedAt)
		return nil
	})
	return allowed && err == nil, err
}

// RecordAppUsageWithGrants records all affected local days in one transaction.
func (l *Ledger) RecordAppUsageWithGrants(
	childID, appID string,
	start, end time.Time,
	access policy.AppAccessPolicy,
) error {
	return l.withTx(func(tx *sql.Tx) error {
		calendar := usage.NewLedger(l.location)
		var recordErr error

		calendar.ForEachDaySegment(start, end, func(from, to time.Time) {
			if recordErr != nil {
				return
			}
			day, err := l.loadDay(tx, childID, from)
			if err != nil {
				recordErr = err
				return
			}

			day.RecordAppUsageWithGrants(childID, appID, from, to, access)
			_, actual := day.Totals(childID, appID, from)
			charges := day.BudgetTotals(childID, appID, from)

			_, recordErr = tx.Exec(`
				INSERT INTO daily_usage (child_id, local_day, app_id, actual, regular, bonus)
				VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT (child_id, local_day, app_id) DO UPDATE SET
					actual = excluded.actual,
					regular = excluded.regular,
					bonus = excluded.bonus
			`, childID, from.In(l.location).Format(time.DateOnly), appID,
				int64(actual), int64(charges.AppRegular), int64(charges.AppBonus))
		})
		return recordErr
	})
}

func (l *Ledger) loadDay(tx *sql.Tx, childID string, at time.Time) (*usage.Ledger, error) {
	day := usage.NewLedger(l.location)
	year, month, date := at.In(l.location).Date()
	start := time.Date(year, month, date, 0, 0, 0, 0, l.location)

	if err := loadSQLiteGrants(tx, day, childID, start, day.NextMidnight(at)); err != nil {
		return nil, err
	}
	rows, err := tx.Query(`
		SELECT app_id, actual, regular, bonus
		FROM daily_usage WHERE child_id = ? AND local_day = ?
	`, childID, start.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var appID string
		var allocation usage.Allocation
		if err := rows.Scan(
			&appID, &allocation.Actual, &allocation.Regular, &allocation.AppBonus,
		); err != nil {
			return nil, err
		}
		day.RecordAllocation(childID, appID, start, allocation)
	}
	return day, rows.Err()
}

func loadSQLiteGrants(tx *sql.Tx, day *usage.Ledger, childID string, start, end time.Time) error {
	rows, err := tx.Query(`
		SELECT app_id, granted_at, duration FROM bonus_grants
		WHERE child_id = ? AND granted_at >= ? AND granted_at < ?
	`, childID, start.UnixNano(), end.UnixNano())
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var appID string
		var grantedAt int64
		var duration time.Duration
		if err := rows.Scan(&appID, &grantedAt, &duration); err != nil {
			return err
		}
		if err := day.GrantBonus(childID, appID, time.Unix(0, grantedAt), duration); err != nil {
			return err
		}
	}
	return rows.Err()
}
