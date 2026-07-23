# Architecture — mirroring CleanExampleGo

This is the canonical layer breakdown for financial-tracker, mapped
directly onto `CleanExampleGo/` — the reference implementation for every
Go service in this workspace. It is not a variant, a simplified version,
or a "close enough": every layer CleanExampleGo defines exists here too,
with financial-tracker's actual entities (Movement, Account,
CreditCardPurchase, Currency, Transfer) standing in for CleanExampleGo's
example domain (Client, Book, Promotion).

If you're about to add or touch a contract and can't find the layer it
belongs in below, stop and re-read `CleanExampleGo/`'s READMEs before
guessing.

## Layer map

```
financial-tracker/
├── domain/
│   └── entities/                    Pure entities: Movement, Account,
│                                     CreditCardPurchase, Category,
│                                     PaymentMethod. Rich, not anemic:
│                                     single-entity business rules and
│                                     state transitions live here (e.g.
│                                     Movement.IsSynced(), the target
│                                     Account.Send()/Receive() below) —
│                                     zero knowledge of persistence or HTTP.
│
├── application/                     CORE — technology-agnostic
│   ├── dto/                         Application DTOs: what usecases,
│   │   ├── movement_dto.go          repositories and services actually
│   │   ├── account_dto.go           pass to each other. NOT domain
│   │   └── transfer_dto.go          entities — see "Why a separate DTO
│   │                                 layer" below.
│   │
│   ├── repositories/                Repository interfaces, expressed in
│   │   ├── movement_repository.go   terms of application/dto types.
│   │   └── account_repository.go
│   │
│   ├── services/                    External-system contracts (also in
│   │   └── ledger_gateway.go        terms of application/dto types):
│   │                                 LedgerGateway, SyncTrigger, SyncRunner.
│   │
│   └── usecases/
│       ├── interfaces.go            Every use-case interface + its
│       │                            Input/Result types, one file. Built
│       │                            from application/dto types and
│       │                            primitives, never domain entities.
│       ├── create_movement.go       Impl files: concrete struct,
│       ├── update_movement.go       constructor, orchestration logic
│       └── ...                      only. No SQL, no HTTP.
│
├── infrastructure/                  ADAPTERS — implements application contracts
│   └── sqlite/
│       ├── entities/                 (if/when needed) DB-row-shaped
│       │                             internal structs — NOT exported
│       │                             beyond this package.
│       ├── movement_repository.go    Implements application/repositories.
│       │                             Converts DB rows → application/dto
│       │                             via a ToDTO()-style method before
│       │                             returning.
│       └── account_repository.go
│
├── interfaces/                      API LAYER
│   ├── api/
│   │   ├── handlers/                Decodes interfaces/dto request →
│   │   │                            calls usecase → encodes
│   │   │                            interfaces/dto response. No business
│   │   │                            logic.
│   │   └── router.go
│   │
│   └── dto/                         HTTP request/response shapes
│       ├── movement_dto.go          (json tags, validation). What
│       └── transfer_dto.go          external clients see — distinct
│                                     from application/dto, which is
│                                     internal.
│
├── pkg/                             Shared utilities (errors, logger, id)
├── cmd/api/main.go                  Composition root: wires concrete
│                                     sqlite repos → usecases → handlers
│                                     → router. The only place concrete
│                                     types meet interfaces.
└── migrations/*.sql
```

## Why a separate DTO layer (`application/dto/` vs `interfaces/dto/`)

Two different concerns, two different DTO sets — collapsing them into one
(or into domain entities) is exactly the shortcut this doc exists to
prevent:

- **`interfaces/dto/`** — the API's contract with the outside world.
  JSON tags, `omitempty`, whatever shape is convenient for HTTP clients.
  Allowed to change when the API's public shape changes.
- **`application/dto/`** — the contract *between* usecases,
  repositories, and services. What the application layer needs, nothing
  more. Allowed to change when the application's internal needs change —
  independently of the API's shape and independently of how SQLite (or
  Postgres, later) happens to store a row.

Using `domain/entities` directly for repository/service/usecase
signatures — which is what financial-tracker's code does today — quietly
erases that boundary: a change to the DB schema's shape now free-rides
straight through the entity into every usecase and handler that imports
it, and vice versa. That's the coupling `application/dto` exists to cut.

## Worked example: `MovementRepository`

**What CleanExampleGo's pattern requires** (target — see "Current
compliance status" below for where financial-tracker's actual code
stands today):

```go
// application/dto/movement_dto.go
package dto

import "time"

// MovementDTO is what the application layer works with — usecases,
// MovementRepository, and anything that calls them. Infrastructure
// converts its own row/record shape into this before returning it.
type MovementDTO struct {
	ID            string
	UserID        string
	Amount        int64
	Currency      string
	Description   string
	Category      string
	PaymentMethod string
	AccountID     *string
	TransferID    *string
	Status        string
	SyncStatus    string
	Timestamp     time.Time
	// ...remaining fields mirroring domain/entities.Movement's shape
}
```

```go
// application/repositories/movement_repository.go
package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
)

type MovementRepository interface {
	Create(ctx context.Context, movement *dto.MovementDTO) (*dto.MovementDTO, error)
	GetByID(ctx context.Context, id string) (*dto.MovementDTO, error)
	// ...
}
```

```go
// infrastructure/sqlite/movement_repository.go
package sqlite

import (
	"github.com/JorgeSaicoski/financial-tracker/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
)

// scanMovement reads a DB row into the domain entity (business-rule
// methods like IsSynced() are still useful inside this package), then
// converts to the application DTO at the boundary.
func toMovementDTO(m *entities.Movement) *dto.MovementDTO {
	return &dto.MovementDTO{
		ID:       m.ID,
		UserID:   m.UserID,
		Amount:   m.Amount,
		Currency: m.Currency,
		// ...
	}
}
```

A usecase's `Execute` takes/returns `dto.MovementDTO` (or its own
Input/Result type built from one), never `*entities.Movement` — same
rule CleanExampleGo's `application/repositories/README.md` states for
`ClientRepository`.

## Current compliance status

financial-tracker's application layer does **not** do this today:
`application/repositories/movement_repository.go`,
`application/services/sync.go`'s `LedgerGateway`,
`application/usecases/interfaces.go`, and every usecase/adapter impl
(including `infrastructure/ledgerservice`'s `gateway.Publish`/
`Transaction.ToEntity()`) take/return `*domain/entities.Movement` (and
`*entities.Account`, etc.) directly, and there is no `application/dto`
package. This is a **known, critical architecture violation**, not an
accepted variant — see `AGENTS.md`'s "Architecture" section at the
workspace root.

Bringing the existing code into compliance means introducing
`application/dto` and updating every repository interface, every
usecase's Input/Result types, and every infrastructure implementation's
return path (plus their tests) — a large, cross-cutting change. That
migration hasn't been scoped or started; it needs an explicit decision
to take on, not a silent refactor bundled into an unrelated change. Until
it happens: **new code must not add to the violation** (no new
repository/service/usecase contract typed against a domain entity), and
anyone touching this area should flag the gap rather than treat the
current state as the standard to copy.

## Worked example: infra adapting an *external* system (`LedgerGateway`)

The `MovementRepository`/SQLite example above shows infra adapting our
own database's row shape. `infrastructure/ledgerservice` is the sharper
version of the same rule, because it crosses a real external-system
boundary, not just a DB row: **infrastructure's job is to adapt whatever
comes in — a DB row, another service's JSON, anything — to the contract
the application layer defined, not the other way around.** This is real,
current code, and it already gets the *shape* of the pattern right —
adapting at the boundary with explicit conversion functions, not letting
ledger-service's format leak past `infrastructure/ledgerservice`. It's
only converting to the wrong target type (a domain entity, per the gap
above), not doing the wrong thing.

What's there today, `infrastructure/ledgerservice/entities/transaction.go`
— ledger-service's own wire format, private to this package:

```go
// wire.Transaction / wire.TransactionRequest are ledger-service's JSON
// shape. domain/entities and application code never see these types.
type Transaction struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Amount    int64     `json:"amount"`
	Currency  string    `json:"currency"`
	Timestamp time.Time `json:"timestamp"`
}

func (t Transaction) ToEntity() *domain.Movement {
	return &domain.Movement{
		ID:       t.ID,
		UserID:   t.UserID,
		Amount:   t.Amount,
		Currency: t.Currency,
		Timestamp: t.Timestamp,
	}
}

type TransactionRequest struct {
	UserID   string `json:"user_id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}
```

And the adapter itself, `infrastructure/ledgerservice/gateway.go` —
notice it already narrows the full `Movement` down to just the three
fields ledger-service's contract accepts, by hand, field by field (no
reflection, no generic mapper — an explicit map is the point, so a field
ledger-service *shouldn't* see can't leak through by accident):

```go
// gateway adapts Client to the application layer's LedgerGateway port.
// Only the money facts cross the wire — ledger-service's transaction
// model doesn't know about descriptions, categories, or payment methods.
func (g *gateway) Publish(ctx context.Context, movement *entities.Movement) (string, error) {
	tx, err := g.client.CreateTransaction(ctx, wire.TransactionRequest{
		UserID:   movement.UserID,
		Amount:   movement.Amount,
		Currency: movement.Currency,
	})
	if err != nil {
		return "", err
	}
	return tx.ID, nil
}
```

**The only delta once `application/dto` exists**: `LedgerGateway`'s port
(`application/services/sync.go`) takes/returns `dto.MovementDTO` instead
of `*entities.Movement`, `ToEntity()` becomes `ToDTO()` returning
`*dto.MovementDTO`, and `Publish`'s body — the actual narrowing logic —
doesn't change at all:

```go
// application/services/sync.go (target)
type LedgerGateway interface {
	Publish(ctx context.Context, movement *dto.MovementDTO) (ledgerTransactionID string, err error)
}
```

```go
// infrastructure/ledgerservice/entities/transaction.go (target)
func (t Transaction) ToDTO() *dto.MovementDTO {
	return &dto.MovementDTO{
		ID:       t.ID,
		UserID:   t.UserID,
		Amount:   t.Amount,
		Currency: t.Currency,
		Timestamp: t.Timestamp,
	}
}
```

That's the whole point of putting the DTO at the application boundary:
the adapter's actual logic (which fields cross the wire, which don't)
is already correct and doesn't need to change — only the type it's
adapting *to* does.

## Rich entities: single-account logic belongs on `Account`, not the usecase

CleanExampleGo's domain layer isn't just data — its `README.md` calls
this out explicitly ("Rich Domain Model: Entities should have behavior,
not just data") and `domain/entities/README.md` gives `Book.Borrow()` /
`Book.Return()` as the pattern: a self-contained state transition that
validates and acts on *one* entity, with no repository access, called
*by* a usecase that still owns loading/persisting/orchestrating across
entities.

A transfer needs two accounts and two repository round-trips (load both,
persist both movements atomically) — that orchestration stays in
`TransferBetweenAccountsUseCase`, matching how `RentBookUseCase` still
owns loading the client and the book. But the decision "here is the
movement this account produces by sending/receiving money" is
single-entity logic, and belongs on `Account`, not inlined in the
usecase:

```go
// domain/entities/account.go

// Send validates the transfer from this account's side and returns the
// debit leg (a negative Movement) to persist. It does not touch
// repositories or persist anything itself — that's the usecase's job.
//
// Deliberately thin today: just the contract (same currency, positive
// amount, not sending to itself). Room to grow without changing the
// call site — e.g. a sufficient-balance check once Account tracks a
// balance, or an observability/monitoring hook here once we care about
// per-account transfer volume.
func (a *Account) Send(to *Account, amount int64, description string, timestamp time.Time) (*Movement, error) {
	if a.ID == to.ID {
		return nil, errors.New("cannot transfer to the same account")
	}
	if a.Currency != to.Currency {
		return nil, fmt.Errorf("currency mismatch: %s vs %s", a.Currency, to.Currency)
	}
	if amount <= 0 {
		return nil, errors.New("amount must be positive")
	}
	return &Movement{
		Amount:        -amount,
		Currency:      a.Currency,
		Description:   description,
		Category:      CategoryTransfer,
		PaymentMethod: PaymentMethodBankTransfer,
		AccountID:     &a.ID,
		Status:        MovementStatusActive,
		SyncStatus:    SyncStatusPending,
		Timestamp:     timestamp,
	}, nil
}

// Receive is Send's mirror for the destination side — same validation,
// the credit leg. Kept as its own method (not derived from Send) so each
// side can grow independently: e.g. a "did the target actually receive
// it" confirmation/monitoring hook later belongs here, not on the source
// account's method.
func (a *Account) Receive(from *Account, amount int64, description string, timestamp time.Time) (*Movement, error) {
	if a.ID == from.ID {
		return nil, errors.New("cannot transfer to the same account")
	}
	if a.Currency != from.Currency {
		return nil, fmt.Errorf("currency mismatch: %s vs %s", from.Currency, a.Currency)
	}
	if amount <= 0 {
		return nil, errors.New("amount must be positive")
	}
	return &Movement{
		Amount:        amount,
		Currency:      a.Currency,
		Description:   description,
		Category:      CategoryTransfer,
		PaymentMethod: PaymentMethodBankTransfer,
		AccountID:     &a.ID,
		Status:        MovementStatusActive,
		SyncStatus:    SyncStatusPending,
		Timestamp:     timestamp,
	}, nil
}
```

The usecase calls both, then owns the parts entities must never do —
loading via repositories and persisting atomically:

```go
// application/usecases/transfer_between_account.go (shape, not the
// current real implementation — see "Current compliance status")

func (uc *transferBetweenAccountsUseCase) Execute(ctx context.Context, input TransferBetweenAccountsInput) (TransferResult, error) {
	from, err := uc.ownedAccount(ctx, input.FromAccountID, input.UserID)
	if err != nil {
		return TransferResult{}, err
	}
	to, err := uc.ownedAccount(ctx, input.ToAccountID, input.UserID)
	if err != nil {
		return TransferResult{}, err
	}

	timestamp := input.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	debit, err := from.Send(to, input.Amount, input.Description, timestamp)
	if err != nil {
		return TransferResult{}, fmt.Errorf("%w: %v", apperrors.ErrInvalidInput, err)
	}
	credit, err := to.Receive(from, input.Amount, input.Description, timestamp)
	if err != nil {
		return TransferResult{}, fmt.Errorf("%w: %v", apperrors.ErrInvalidInput, err)
	}

	transferID := id.NewUUID()
	debit.TransferID, credit.TransferID = &transferID, &transferID

	// Still the usecase's job: atomic persistence across both legs.
	created, err := uc.movements.CreateBatch(ctx, []*entities.Movement{debit, credit})
	if err != nil {
		return TransferResult{}, err
	}
	return TransferResult{TransferID: transferID, Debit: created[0], Credit: created[1]}, nil
}
```

Same rule as the DTO one above: don't reinvent this per-usecase.
Whenever a usecase is about to inline a single-entity validate-and-build
step (as today's `transfer_between_account.go` does — it builds both
`Movement` structs directly inline instead of via `Account.Send`/
`Account.Receive`), that's the signal it belongs on the entity instead.

## Current compliance status (entity methods)

Documented here as the target shape and worked example only — this
principle is **not yet applied** anywhere in financial-tracker's actual
code. `transfer_between_account.go` currently builds both `Movement`
structs inline in the usecase; there is no `Account.Send`/
`Account.Receive` today. Unlike the `application/dto` gap above, this one
is scoped to transfer only for now — the rest of the usecases
(`update_movement`, `cancel_movement`, `cancel_transfer`,
`create_credit_card_purchase`, ...) haven't been individually reviewed
against this principle yet; do that case-by-case when touching each,
rather than assuming they all need the same treatment.

## Everything else (unaffected by the DTO gap above)

The rest of the request-flow and conventions already match CleanExampleGo
and are documented in this folder's `README.md`:

- Handlers never touch repositories directly.
- Usecases never import `database/sql` or `net/http`.
- Constructors return interface types.
- Contracts are consolidated (`application/usecases/interfaces.go`,
  one file per aggregate under `application/repositories/`), never
  declared inline next to an implementation or consumer.
- `cmd/api/main.go` is the sole composition root.
