// Package sqlite implements the domain repository contracts against
// financial-tracker's own local SQLite database — the source of truth.
// It is the only package (besides migrations) that knows SQL.
package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver, registers as "sqlite"

	"github.com/JorgeSaicoski/financial-tracker/migrations"
)

// Open opens (creating if needed) the database file and applies pragmas
// suited to a single-process app: WAL for concurrent reads during the
// background sync, busy_timeout as a safety net, foreign keys on.
func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("sqlite: create data dir: %w", err)
		}
	}

	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", path, err)
	}
	// One connection serializes all writes at the pool level; SQLite has a
	// single writer anyway and this write volume is one user's movements.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: ping %s: %w", path, err)
	}
	return db, nil
}

// Migrate applies the embedded migrations that haven't run yet, tracked in
// a schema_migrations table by filename.
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("sqlite: create schema_migrations: %w", err)
	}

	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("sqlite: read migrations: %w", err)
	}

	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var applied int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&applied); err != nil {
			return fmt.Errorf("sqlite: check migration %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}

		script, err := migrations.FS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("sqlite: read migration %s: %w", name, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("sqlite: begin migration %s: %w", name, err)
		}
		if _, err := tx.Exec(string(script)); err != nil {
			tx.Rollback()
			return fmt.Errorf("sqlite: apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, name, formatTime(time.Now())); err != nil {
			tx.Rollback()
			return fmt.Errorf("sqlite: record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("sqlite: commit migration %s: %w", name, err)
		}
	}
	return nil
}

// timeLayout is fixed-width RFC 3339 UTC so stored timestamps sort
// correctly as text (variable-width fractions would not).
const timeLayout = "2006-01-02T15:04:05.000000000Z"

func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}
