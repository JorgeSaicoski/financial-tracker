# Walkthrough — updating an existing route

**What changed:** `GET /movements` gained optional `from`/`to` interval
filtering. Real diff, current code, in build order.

**Architecture note:** the signature below (`[]*entities.Movement`) is
the repo's current, real shape — and also the known `internal/application/dto` gap
described in [architecture.md](architecture.md): this contract should be
typed against an application DTO, not the domain entity, per
CleanExampleGo. Shown here as-is because it's real, current code, not
because it's the target to copy into a *new* contract.

## 1. Repository interface — `internal/application/repositories/movement_repository.go`

Before:
```go
ListByUser(ctx context.Context, userID string, currency *string, limit, offset int) ([]*entities.Movement, error)
```
After:
```go
// ListByUser filters by optional currency and optional [from, to)
// time interval on the movement's effective timestamp.
ListByUser(ctx context.Context, userID string, currency *string, from, to *time.Time, limit, offset int) ([]*entities.Movement, error)
```
Optional filters are pointers — `nil` means "no bound," matching how
`currency *string` already worked in this same signature.

## 2. SQLite implementation — `internal/infrastructure/sqlite/movement_repository.go`

Inside `ListByUser`, right after the existing currency filter:
```go
if from != nil {
	query += ` AND timestamp >= ?`
	args = append(args, formatTime(*from))
}
if to != nil {
	query += ` AND timestamp < ?`
	args = append(args, formatTime(*to))
}
```

## 3. Use-case contract and implementation

The `ListMovementsUseCase` interface lives in
`internal/application/usecases/list_movements.go` — same file as its
implementation, one file per use case (see [README.md](README.md)); its
`Execute` signature gains `from, to *time.Time` there:
```go
type ListMovementsUseCase interface {
	Execute(ctx context.Context, userID string, currency *string, from, to *time.Time, limit, offset int) (ListMovementsResult, error)
}
```
The implementation in `internal/application/usecases/list_movements.go` gains the
same parameters, passed straight through to the repository, plus one
validation line before the query runs:
```go
if from != nil && to != nil && !from.Before(*to) {
	return ListMovementsResult{}, apperrors.ErrInvalidInput
}
```

## 4. Handler — `internal/interfaces/api/handlers/movement_handler.go`

Inside `ListMovements`:
```go
from, err := parseTimeParam(r, "from", false)
if err != nil {
	h.writeError(w, http.StatusBadRequest, "invalid from (want YYYY-MM-DD or RFC 3339)")
	return
}
to, err := parseTimeParam(r, "to", true)
if err != nil {
	h.writeError(w, http.StatusBadRequest, "invalid to (want YYYY-MM-DD or RFC 3339)")
	return
}
```
`parseTimeParam` is shared (`internal/interfaces/api/handlers/http_helpers.go`),
so any handler needing a date-range query param reuses it instead of
reimplementing date parsing:
```go
// parseTimeParam accepts RFC 3339 or a bare date. A bare date means the
// whole day: as a lower bound it's that day's midnight UTC; as an upper
// bound (endOfDay) it's the next midnight, pairing with the repository's
// exclusive "timestamp < to".
func parseTimeParam(r *http.Request, name string, endOfDay bool) (*time.Time, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		t = t.UTC()
		return &t, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, errBadTimeParam
	}
	if endOfDay {
		t = t.Add(24 * time.Hour)
	}
	return &t, nil
}
```

## 5. Fix what the compiler breaks

Changing an interface signature breaks every implementer and every fake:
`fakeMovementRepo` and `fakeRepo` in the test files, and every existing
call site of `ListByUser`/`ListMovements.Execute` in
`internal/infrastructure/sqlite/repository_test.go` and
`internal/application/usecases/cancel_movement_test.go`. Give the fakes real
filtering behavior — a `_, _` no-op parameter defeats the point of the
test. `go vet ./...` finds every broken call site for you; fix them one
by one until it's clean.

## 6. Frontend, if the field is user-facing

`web/src/lib/api.js`'s `listMovements()` currently takes no arguments —
extending it to accept an optional interval is a normal
function-signature change, same as any other JS function; existing
callers with no arguments keep working unchanged.

## 7. New column vs new query param

This example added no column — `timestamp` already existed. If your
change needs new storage (like the `account_id` column added to
`movements` in `migrations/004_create_accounts_tables.sql` via
`ALTER TABLE`), you must also update the `movementColumns` constant,
`insertMovement`, and `scanMovement` in
`internal/infrastructure/sqlite/movement_repository.go` **together** — their
column order must match exactly, or you'll scan a `category` value into
the `account_id` field silently.

## 8. Rebuild and verify

```bash
go build ./... && go vet ./... && go test ./...
make rebuild
curl -s "localhost:8081/movements?from=2026-07-01&to=2026-07-22"
```
