// Package migrations embeds financial-tracker's own SQLite schema (separate
// from ledger-service's Postgres migrations) so the binary carries it —
// no runtime file-path dependency in the container.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
