// Package errors defines the small set of domain-level error kinds used
// across financial-tracker so handlers can map them to HTTP status codes
// without knowing which infrastructure (ledger-service today, Postgres
// tomorrow) produced them.
package errors

import "errors"

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
	ErrUpstream     = errors.New("upstream service error")
)

// Is re-exports the standard errors.Is so callers only need to import this package.
func Is(err, target error) bool {
	return errors.Is(err, target)
}
