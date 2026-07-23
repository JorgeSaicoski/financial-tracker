package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/JorgeSaicoski/financial-tracker/infrastructure/postgresql"
	"github.com/JorgeSaicoski/financial-tracker/infrastructure/sqlite"
)

func main() {
	dbPath := flag.String("db-path", envOr("DB_PATH", "./data/financial-tracker.db"), "path to the source SQLite database (env DB_PATH)")
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "target Postgres DATABASE_URL (env DATABASE_URL)")
	force := flag.Bool("force", false, "migrate into a target that already has data (does not wipe it first — inserts will fail on any id collision)")
	flag.Parse()

	if *databaseURL == "" {
		fmt.Fprintln(os.Stderr, "migrate-sqlite: --database-url (or DATABASE_URL) is required")
		os.Exit(1)
	}

	ctx := context.Background()

	src, err := sqlite.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate-sqlite: open source SQLite %s: %v\n", *dbPath, err)
		os.Exit(1)
	}
	defer src.Close()
	if err := sqlite.Migrate(src); err != nil {
		fmt.Fprintf(os.Stderr, "migrate-sqlite: migrate source schema: %v\n", err)
		os.Exit(1)
	}

	dst, err := postgresql.Open(*databaseURL, postgresql.PoolConfig{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate-sqlite: open target Postgres: %v\n", err)
		os.Exit(1)
	}
	defer dst.Close()
	if err := postgresql.Migrate(dst); err != nil {
		fmt.Fprintf(os.Stderr, "migrate-sqlite: migrate target schema: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("migrate-sqlite: copying %s -> postgres (force=%v)\n", *dbPath, *force)

	counts, err := run(ctx, src, dst, *force)
	for _, c := range counts {
		fmt.Printf("  %-24s source=%d target=%d\n", c.table, c.source, c.target)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate-sqlite: FAILED: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("migrate-sqlite: done — verify counts/balance in the target before pointing the API at it.")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
