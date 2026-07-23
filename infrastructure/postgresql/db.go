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

// PoolConfig bounds the connection pool Open creates. A zero-valued field
// (including a zero PoolConfig{} entirely) falls back to the matching
// DefaultPoolConfig value, so callers that don't care can pass PoolConfig{}.
type PoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// DefaultPoolConfig is sized for one financial-tracker API instance talking
// to one Postgres database — not a high-throughput multi-tenant service.
var DefaultPoolConfig = PoolConfig{
	MaxOpenConns:    10,
	MaxIdleConns:    5,
	ConnMaxLifetime: 30 * time.Minute,
	ConnMaxIdleTime: 5 * time.Minute,
}

// Open opens a connection pool to the given Postgres DATABASE_URL, bounded
// by cfg. Unlike SQLite (which this package's sibling caps at a single
// connection because SQLite itself only has one writer), Postgres handles
// concurrent writers fine — but an unbounded database/sql pool in an HTTP
// API can still grow without limit under load and exhaust Postgres's own
// max_connections, so the pool is capped here instead of left at Go's
// default of "unlimited".
func Open(databaseURL string, cfg PoolConfig) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgresql: open: %w", err)
	}

	maxOpenConns := cfg.MaxOpenConns
	if maxOpenConns <= 0 {
		maxOpenConns = DefaultPoolConfig.MaxOpenConns
	}
	maxIdleConns := cfg.MaxIdleConns
	if maxIdleConns <= 0 {
		maxIdleConns = DefaultPoolConfig.MaxIdleConns
	}
	connMaxLifetime := cfg.ConnMaxLifetime
	if connMaxLifetime <= 0 {
		connMaxLifetime = DefaultPoolConfig.ConnMaxLifetime
	}
	connMaxIdleTime := cfg.ConnMaxIdleTime
	if connMaxIdleTime <= 0 {
		connMaxIdleTime = DefaultPoolConfig.ConnMaxIdleTime
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)
	db.SetConnMaxIdleTime(connMaxIdleTime)

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
// ';' boundaries, respecting single-quoted string literals (including the
// standard SQL escaped-quote convention of doubling the quote character,
// e.g. O + quote + quote + Reilly for a literal O'Reilly) and '--' line
// comments, so a semicolon or apostrophe inside either doesn't split
// mid-statement or desync the parser's quote-tracking for the rest of the
// file.
func splitStatements(script string) []string {
	var stmts []string
	var buf strings.Builder
	inQuote := false
	inLineComment := false

	for i := 0; i < len(script); i++ {
		c := script[i]

		if inLineComment {
			buf.WriteByte(c)
			if c == '\n' {
				inLineComment = false
			}
			continue
		}

		if !inQuote && c == '-' && i+1 < len(script) && script[i+1] == '-' {
			inLineComment = true
			buf.WriteByte(c)
			continue
		}

		if c == '\'' {
			if inQuote && i+1 < len(script) && script[i+1] == '\'' {
				// '' inside a string is an escaped literal quote, not the
				// closing quote — consume both bytes and stay in-string.
				buf.WriteByte(c)
				buf.WriteByte(script[i+1])
				i++
				continue
			}
			inQuote = !inQuote
			buf.WriteByte(c)
			continue
		}

		buf.WriteByte(c)
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
