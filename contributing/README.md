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

The full, corrected layer map — including the `application/dto` layer
this repo is currently missing — is in [architecture.md](architecture.md).
Short version:

```
browser (web/src/routes/+page.svelte)
  └─ API client        web/src/lib/api.js            fetch() wrapper, throws Error(body.error)
      └─ router        interfaces/api/router.go       method+path → handler (Go 1.22 ServeMux patterns)
          └─ handler   interfaces/api/handlers/       decode interfaces/dto request, map errors → HTTP status
              └─ usecase  application/usecases/       validation + business rules, no SQL/HTTP knowledge
                  └─ repository interface  application/repositories/   the contract the usecase depends on,
                                                                        expressed in application/dto types
                      └─ implementation    infrastructure/sqlite/  the only place that knows SQL; converts
                                                                    rows to application/dto before returning
                          └─ schema        migrations/*.sql        embedded, applied on boot
```

- **Handlers** never touch repositories directly — they call usecases.
- **Usecases** never import `database/sql` or `net/http` — and, per
  `AGENTS.md`, should see `application/dto` types (not `domain/entities`
  directly) from repositories/services. financial-tracker's code doesn't
  do this today — see architecture.md's "Current compliance status" —
  don't copy that shape into new code.
- **Constructors return interface types** (`NewCreateMovement(...) CreateMovementUseCase`),
  so every layer depends on a contract, and tests swap in fakes.
- **`cmd/api/main.go`** is the only place concrete implementations meet:
  it builds repositories → usecases → handlers → router.

## Where contracts live

Every contract sits in the **application layer**, consolidated so it's
visible in one place (see the architecture rules in the workspace's
`AGENTS.md` and the `CleanExampleGo` reference repo):

| Contract kind | Lives in | Real examples |
|---|---|---|
| Application DTOs | `application/dto/` (missing today — see [architecture.md](architecture.md)) | `MovementDTO`, `AccountDTO` |
| Use-case interfaces + their Input/Result types | `application/usecases/interfaces.go` (one file, all of them) | `CreateMovementUseCase`, `CreateMovementInput`, `AccountView` |
| Repository interfaces | `application/repositories/` (one file per aggregate) | `MovementRepository`, `AccountRepository` |
| Service ports / cross-service contracts | `application/services/` | `LedgerGateway`, `SyncTrigger`, `SyncRunner` |

The domain layer (`domain/entities`) holds pure entities only. Never
define an interface in the same file as its implementation or its
consumer — contract in the files above, implementation in its own file.

## Shared conventions

Money is `int64` in the smallest currency unit (cents/sats), sign carries
direction, never floats. Timestamps are UTC, formatted with the sqlite
package's `formatTime`/`parseTime` helpers so they sort correctly as
text. IDs are UUIDs from `pkg/id.NewUUID()`, assigned by the repository
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
- [ ] Contracts are where they belong (`interfaces.go` /
      `application/repositories/` / `application/services/`), not inline
      with an implementation
