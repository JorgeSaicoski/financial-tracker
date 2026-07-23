// Package postgres embeds financial-tracker's Postgres schema — the
// dialect-ported counterpart of the SQLite migrations one directory up —
// so the binary carries it with no runtime file-path dependency in the
// container.
package postgres

import "embed"

//go:embed *.sql
var FS embed.FS
