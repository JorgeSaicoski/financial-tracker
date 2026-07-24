# Contributing to financial-tracker

Pick the walkthrough that matches what you're doing:

| You're doing | Read |
|---|---|
| Any of the below, first time — the layer map itself | [architecture.md](architecture.md) |
| A complete new feature (new storage, new everything) | [new-feature.md](new-feature.md) |
| A feature composing existing storage (no/little new infra) | [feature-without-infra.md](feature-without-infra.md) |
| Changing an existing endpoint | [update-route.md](update-route.md) |
| Fixing a bug | [bug-fix.md](bug-fix.md) |

Each uses real, current code from this repo, or — where marked — a
realistic worked example following the same real conventions. Follow
[new-feature.md](new-feature.md) top to bottom and you will have built
the same working `GET/POST /currencies` endpoints that exist in this
repo right now.

## How a request flows

The full layer map — including `internal/application/dto`, the layer that
sits between the application layer and everything above/below it — is in
[architecture.md](architecture.md). Short version:

```
browser (web/src/routes/+page.svelte)
  └─ API client        web/src/lib/api.js            fetch() wrapper, throws Error(body.error)
      └─ router        internal/interfaces/api/router.go       method+path → handler (Go 1.22 ServeMux patterns)
          └─ handler   internal/interfaces/api/handlers/       decode internal/interfaces/dto request, map errors → HTTP status
              └─ usecase  internal/application/usecases/       validation + business rules, no SQL/HTTP knowledge
                  └─ repository interface  internal/application/repositories/   the contract the usecase depends on,
                                                                        expressed in internal/application/dto types
                      └─ implementation    internal/infrastructure/sqlite/  the only place that knows SQL; converts
                                                                    rows to internal/application/dto before returning
                          └─ schema        migrations/*.sql        embedded, applied on boot
```

- **Handlers** never touch repositories directly — they call usecases.
- **Usecases** never import `database/sql` or `net/http` — and, per
  `AGENTS.md`, see `internal/application/dto` types (not `internal/domain/entities`
  directly) from repositories/services. financial-tracker's code already
  does this — see architecture.md's "Current compliance status" — keep
  new code typed against `application/dto`, not a domain entity, at any
  repository/service/usecase contract boundary.
- **Constructors return interface types** (`NewCreateMovement(...) CreateMovementUseCase`),
  so every layer depends on a contract, and tests swap in fakes.
- **`internal/cmd/api/main.go`** is the only place concrete implementations meet:
  it builds repositories → usecases → handlers → router.

## Where contracts live

Every contract sits in the **application layer** (see the architecture
rules in the workspace's `AGENTS.md` and the `CleanExampleGo` reference
repo):

| Contract kind | Lives in | Real examples |
|---|---|---|
| Application DTOs | `internal/application/dto/` | `MovementDTO`, `AccountDTO` |
| Use-case interfaces + their Input/Result types | `internal/application/usecases/interfaces.go` — every use-case contract consolidated in one file (this workspace's amended rule on top of CleanExampleGo's base "one file per use case"; see [architecture.md](architecture.md)) | `interfaces.go` holds `CreateMovementUseCase` and `CreateMovementInput`; `create_movement.go` holds only the concrete `createMovementUseCase` struct/constructor/logic |
| Repository interfaces | `internal/application/repositories/` (one file per aggregate) | `MovementRepository`, `AccountRepository` |
| Service ports / cross-service contracts | `internal/application/services/` | `LedgerGateway`, `SyncTrigger`, `SyncRunner` |

The domain layer (`internal/domain/entities`) holds pure entities only. A
type shared by two use cases (e.g. `AccountView`, returned by both
`ListAccountsUseCase` and `ReportAccountBalanceUseCase`) lives in the file
of whichever use case returns it first, with a comment noting the other
consumer — same package, no import needed. Never declare an interface in
the same file as an unrelated feature's implementation.

## Shared conventions

Money is `int64` in the smallest currency unit (cents/sats), sign carries
direction, never floats. Timestamps are UTC, formatted with the sqlite
package's `formatTime`/`parseTime` helpers so they sort correctly as
text. IDs are UUIDs from `internal/pkg/id.NewUUID()`, assigned by the repository
when empty.

## Definition of done

- [ ] `go build ./... && go vet ./... && go test ./...` all clean
- [ ] New/changed endpoints exercised with `curl` against a rebuilt
      container (`make rebuild`), including the error cases (bad input →
      400 with a helpful message, missing → 404)
- [ ] Frontend uses the endpoint and handles its errors (`err.message`)
- [ ] Migration applies cleanly on both a fresh DB (`make remove-db && make up`)
      and an existing one (`make rebuild` without wiping)
- [ ] README's API table and the `endpoints:` log line in `main.go` updated
- [ ] Contracts are where they belong (the use case's own file /
      `internal/application/repositories/` / `internal/application/services/`), not inline
      with an unrelated feature's implementation
