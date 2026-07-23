// Package postgresql implements financial-tracker's repository contracts
// against PostgreSQL, as an alternative to infrastructure/sqlite selected
// via DB_DRIVER. It is the only package (besides migrations/postgres) that
// knows Postgres-specific SQL.
package postgresql

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver

	pgmigrations "github.com/JorgeSaicoski/financial-tracker/migrations/postgres"
)

// Open opens a connection pool to the given Postgres DATABASE_URL. Unlike
// SQLite, Postgres handles concurrent writers itself, so no connection
// limit is imposed here.
func Open(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgresql: open: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgresql: ping: %w", err)
	}
	return db, nil
}

// Migrate applies the embedded Postgres migrations that haven't run yet,
// tracked in a schema_migrations table by filename — the same mechanism
// infrastructure/sqlite uses for its own migrations.
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL)`); err != nil {
		return fmt.Errorf("postgresql: create schema_migrations: %w", err)
	}

	entries, err := pgmigrations.FS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("postgresql: read migrations: %w", err)
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
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = $1`, name).Scan(&applied); err != nil {
			return fmt.Errorf("postgresql: check migration %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}

		script, err := pgmigrations.FS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("postgresql: read migration %s: %w", name, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("postgresql: begin migration %s: %w", name, err)
		}
		// pgx's default extended query protocol only accepts one statement
		// per Exec call, unlike SQLite's driver — so each migration file is
		// split into its individual statements here.
		for _, stmt := range splitStatements(string(script)) {
			if _, err := tx.Exec(stmt); err != nil {
				tx.Rollback()
				return fmt.Errorf("postgresql: apply migration %s: %w", name, err)
			}
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (name, applied_at) VALUES ($1, $2)`, name, time.Now().UTC()); err != nil {
			tx.Rollback()
			return fmt.Errorf("postgresql: record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("postgresql: commit migration %s: %w", name, err)
		}
	}
	return nil
}

// splitStatements breaks a migration script into individual statements on
// ';' boundaries, respecting single-quoted string literals so a semicolon
// inside a string (e.g. a default value) doesn't split mid-statement.
func splitStatements(script string) []string {
	var stmts []string
	var buf strings.Builder
	inQuote := false

	for i := 0; i < len(script); i++ {
		c := script[i]
		buf.WriteByte(c)
		if c == '\'' {
			inQuote = !inQuote
		}
		if c == ';' && !inQuote {
			stmts = append(stmts, buf.String())
			buf.Reset()
		}
	}
	if strings.TrimSpace(buf.String()) != "" {
		stmts = append(stmts, buf.String())
	}
	return stmts
}
