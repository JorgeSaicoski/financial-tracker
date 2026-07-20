# financial-tracker

Personal finance tracker. Records every movement (income/expense) a user makes.

**Current architecture:** financial-tracker has no database of its own yet.
Every movement is persisted by delegating to [ledger-service](https://github.com/JorgeSaicoski/ledger-service)
over HTTP — it's the only source of truth for now. financial-tracker owns the
domain model (`Movement`), the API the Svelte frontend talks to, and the
balance calculation (ledger-service deliberately doesn't compute balances).

Backend layout follows Clean Architecture (see `CleanExampleGo` for the
reference pattern this was modeled on):

```
domain/entities          Movement entity (business rules)
application/dto          data contracts between layers
application/repositories MovementRepository interface — the swap point
application/usecases     CreateMovement, GetMovement, ListMovements
infrastructure/ledgerservice  implements MovementRepository via ledger-service HTTP API
interfaces/api            /movements HTTP handlers + router (what the Svelte app calls)
interfaces/dto             API request/response shapes
pkg/errors, pkg/logger    shared utilities
cmd/api/main.go           wiring/entrypoint
web/                      SvelteKit frontend
```

The only place that knows about ledger-service is `infrastructure/ledgerservice`.
When financial-tracker gets its own database, a new
`infrastructure/postgresql.MovementRepository` implementing the same interface
drops in with a two-line change in `cmd/api/main.go` — no usecase or handler
changes required.

## MVP scope / known limitations

- **No auth.** Every request without an explicit `user_id` is attributed to
  a fixed dev user (`DEFAULT_USER_ID`). Fine for single-user personal use;
  needs real user management before this is multi-user.
- **No categories/descriptions.** ledger-service's transaction model is just
  `user_id, amount, currency, timestamp` — financial-tracker's `Movement`
  can't carry more than that until it has its own database to store the
  extra metadata in.
- **Quirk worth knowing:** ledger-service's `POST /transactions` only
  returns the created id (a bare string), not the full record. financial-tracker's
  ledger-service client does a create-then-fetch to return a full `Movement`
  to its own callers.

## API

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/movements` | Create a movement. Body: `{amount, currency?, user_id?}` (currency/user_id default if omitted). |
| `GET` | `/movements?id={uuid}` | Fetch one movement. |
| `GET` | `/movements?user_id={uuid}&currency=&limit=&offset=` | List a user's movements + computed `balance`. |

`amount` is an integer in the smallest currency unit (cents), negative for
expenses, positive for income, and cannot be zero.

## Running locally

1. **ledger-service** (separate repo, has its own compose file):
   ```bash
   cd ../ledger-service
   podman-compose up -d --build   # or docker-compose
   ```
   This brings up Postgres + ledger-service on `:8080`.

2. **financial-tracker API**:
   ```bash
   cp .env.example .env   # adjust if needed
   go run ./cmd/api
   ```
   Listens on `:8081` by default, talks to ledger-service at
   `LEDGER_SERVICE_URL` (default `http://localhost:8080`).

3. **Svelte frontend** (`web/`) — requires Node.js/npm, not installed in
   this environment, so it has not been run or verified end-to-end here:
   ```bash
   cd web
   cp .env.example .env   # PUBLIC_API_URL, defaults to http://localhost:8081
   npm install
   npm run dev
   ```
   Opens on `:5173` and calls the financial-tracker API directly (CORS is
   wide open in `interfaces/api/router.go` for local dev — tighten before
   deploying anywhere real).

## Testing

```bash
go build ./...
go vet ./...
```

Backend has been manually smoke-tested end-to-end against a live
ledger-service instance (create, get, list-with-balance, 400/404 error
paths, CORS preflight). No automated Go tests yet — add
`*_test.go` alongside usecases/handlers as the next hardening step.

## Roadmap

- Own Postgres-backed `MovementRepository` implementation, with a backfill
  script pulling history from ledger-service via `GET /transactions?user_id=`.
- Categories/descriptions once there's a local DB to hold them.
- Real auth instead of `DEFAULT_USER_ID`.
