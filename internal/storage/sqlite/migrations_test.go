package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/usage"
)

func TestOpenAppliesInitialMigration(t *testing.T) {
	ledger, err := Open(filepath.Join(t.TempDir(), "usage.db"), time.UTC)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer ledger.Close()

	var _ usage.Store = ledger

	tables := []string{
		"bonus_grants",
		"bonus_codes",
		"bonus_redemption_attempts",
		"daily_usage",
	}
	for _, name := range tables {
		var found string
		err := ledger.db.QueryRow(
			"SELECT name FROM sqlite_schema WHERE type = 'table' AND name = ?",
			name,
		).Scan(&found)
		if err != nil {
			t.Errorf("find table %q: %v", name, err)
		}
	}

	assertInitialMigrationRecordedOnce(t, ledger)
}

func TestOpenPreservesDataAndDoesNotRepeatMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	at := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)

	ledger, err := Open(path, time.UTC)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := ledger.GrantBonus("child-a", "", at, 5*time.Minute); err != nil {
		_ = ledger.Close()
		t.Fatalf("grant bonus: %v", err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	ledger, err = Open(path, time.UTC)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer ledger.Close()

	bonuses, err := ledger.BonusesFor("child-a", "app-a", at)
	if err != nil {
		t.Fatalf("read stored bonus: %v", err)
	}
	if bonuses.Shared.Granted != 5*time.Minute {
		t.Errorf("stored shared bonus = %s, want 5m", bonuses.Shared.Granted)
	}

	assertInitialMigrationRecordedOnce(t, ledger)
}

func assertInitialMigrationRecordedOnce(t *testing.T, ledger *Ledger) {
	t.Helper()

	var count int
	err := ledger.db.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations WHERE version = 1",
	).Scan(&count)
	if err != nil {
		t.Fatalf("read migration history: %v", err)
	}
	if count != 1 {
		t.Errorf("initial migration recorded %d times, want 1", count)
	}
}
