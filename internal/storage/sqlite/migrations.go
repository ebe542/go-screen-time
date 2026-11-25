package sqlite

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func applyMigrations(tx *sql.Tx) error {
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY
		)
	`); err != nil {
		return fmt.Errorf("create migration history: %w", err)
	}

	files, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("list embedded migrations: %w", err)
	}

	previousVersion := 0
	for _, filename := range files {
		prefix, _, ok := strings.Cut(path.Base(filename), "_")
		version, err := strconv.Atoi(prefix)
		if !ok || err != nil || version <= previousVersion {
			return fmt.Errorf("invalid migration order or filename %q", filename)
		}
		previousVersion = version

		var applied int
		err = tx.QueryRow(
			"SELECT version FROM schema_migrations WHERE version = ?", version,
		).Scan(&applied)
		switch {
		case err == nil:
			continue
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("read migration %d status: %w", version, err)
		}

		script, err := migrationFiles.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("read migration %q: %w", filename, err)
		}
		if _, err := tx.Exec(string(script)); err != nil {
			return fmt.Errorf("apply migration %q: %w", filename, err)
		}
		if _, err := tx.Exec(
			"INSERT INTO schema_migrations (version) VALUES (?)", version,
		); err != nil {
			return fmt.Errorf("record migration %d: %w", version, err)
		}
	}

	return nil
}
