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
├── internal/                        All Go code lives here — CleanExampleGo's
│   │                                 layers nest under Go's own `internal/`
│   │                                 convention (compiler-enforced: nothing
│   │                                 outside this module can import it),
│   │                                 which is what actually separates the
│   │                                 architecture layers from the non-Go
│   │                                 siblings at the true root (migrations/,
│   │                                 deploy/, web/, contributing/).
│   │
│   ├── domain/
│   │   └── entities/                    Pure entities: Movement, Account,
│   │                                     CreditCardPurchase, Category,
│   │                                     PaymentMethod. Rich, not anemic:
│   │                                     single-entity business rules and
│   │                                     state transitions live here (e.g.
│   │                                     Movement.IsSynced(),
│   │                                     Account.Send()/Receive() below) —
│   │                                     zero knowledge of persistence or HTTP.
│   │
│   ├── application/                     CORE — technology-agnostic
│   │   ├── dto/                         Application DTOs: what usecases,
│   │   │   ├── movement_dto.go          repositories and services actually
│   │   │   ├── account_dto.go           pass to each other. NOT domain
│   │   │   ├── credit_card_purchase_dto.go  entities — see "Why a separate
│   │   │   └── exchange_rate_dto.go     DTO layer" below.
│   │   │
│   │   ├── repositories/                Repository interfaces, expressed in
│   │   │   ├── movement_repository.go   terms of application/dto types.
│   │   │   └── account_repository.go
│   │   │
│   │   ├── services/                    External-system contracts (also in
│   │   │   └── sync.go                  terms of application/dto types):
│   │   │                                 LedgerGateway, SyncTrigger, SyncRunner.
│   │   │
│   │   └── usecases/                    Every use-case interface plus its
│   │       ├── interfaces.go            Input/Result/View types live
│   │       ├── create_movement.go       together in one consolidated
│   │       ├── update_movement.go       interfaces.go, so every contract
│   │       └── ...                      is visible in one place — this
│   │                                     workspace's amended rule on top
│   │                                     of CleanExampleGo's base "one
│   │                                     file per use case" (see
│   │                                     AGENTS.md's "Architecture"
│   │                                     section). Each usecase's concrete
│   │                                     struct/constructor/orchestration
│   │                                     logic still gets its own file
│   │                                     (create_movement.go, ...) — only
│   │                                     the contract itself moved. A type
│   │                                     shared by two usecases (e.g.
│   │                                     AccountView) also lives in
│   │                                     interfaces.go.
│   │
│   ├── infrastructure/                  ADAPTERS — implements application contracts
│   │   └── sqlite/
│   │       ├── entities/                 (if/when needed) DB-row-shaped
│   │       │                             internal structs — NOT exported
│   │       │                             beyond this package.
│   │       ├── movement_repository.go    Implements application/repositories.
│   │       │                             Converts DB rows → application/dto
│   │       │                             via a ToDTO()-style method before
│   │       │                             returning.
│   │       └── account_repository.go
│   │
│   ├── interfaces/                      API LAYER
│   │   ├── api/
│   │   │   ├── handlers/                Decodes interfaces/dto request →
│   │   │   │                            calls usecase → encodes
│   │   │   │                            interfaces/dto response. No business
│   │   │   │                            logic.
│   │   │   └── router.go
│   │   │
│   │   └── dto/                         HTTP request/response shapes
│   │       ├── movement_dto.go          (json tags, validation). What
│   │       └── transfer_dto.go          external clients see — distinct
│   │                                     from application/dto, which is
│   │                                     internal.
│   │
│   ├── pkg/                             Shared utilities (errors, logger, id)
│   └── cmd/api/main.go                  Composition root: wires concrete
│                                         sqlite repos → usecases → handlers
│                                         → router. The only place concrete
│                                         types meet interfaces.
│
└── migrations/*.sql                 Plain SQL (plus a thin go:embed shim) —
                                      not a Go-layer concern, so it stays at
                                      the true root; internal/ packages
                                      still import it like any other module
                                      path (internal/ only restricts who can
                                      import internal/, not what internal/
                                      can import).
```

## Why a separate DTO layer (`internal/application/dto/` vs `internal/interfaces/dto/`)

Two different concerns, two different DTO sets — collapsing them into one
(or into domain entities) is exactly the shortcut this doc exists to
prevent:

- **`internal/interfaces/dto/`** — the API's contract with the outside world.
  JSON tags, `omitempty`, whatever shape is convenient for HTTP clients.
  Allowed to change when the API's public shape changes.
- **`internal/application/dto/`** — the contract *between* usecases,
  repositories, and services. What the application layer needs, nothing
  more. Allowed to change when the application's internal needs change —
  independently of the API's shape and independently of how SQLite (or
  Postgres, later) happens to store a row.

Using `internal/domain/entities` directly for repository/service/usecase
signatures would quietly erase that boundary: a change to the DB schema's
shape would free-ride straight through the entity into every usecase and
handler that imports it, and vice versa. That's the coupling
`internal/application/dto` exists to cut — and, per the "Current
compliance status" section below, financial-tracker's repository/service/
usecase contracts already use `application/dto` types, not entities,
today.

## Worked example: `MovementRepository`

**What CleanExampleGo's pattern requires** — and, per "Current compliance
status" below, already matches financial-tracker's actual
`MovementRepository` today:

```go
// internal/application/dto/movement_dto.go
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
	// ...remaining fields mirroring internal/domain/entities.Movement's shape
}
```

```go
// internal/application/repositories/movement_repository.go
package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

type MovementRepository interface {
	Create(ctx context.Context, movement *dto.MovementDTO) (*dto.MovementDTO, error)
	GetByID(ctx context.Context, id string) (*dto.MovementDTO, error)
	// ...
}
```

```go
// internal/infrastructure/sqlite/movement_repository.go
package sqlite

import (
	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
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
rule CleanExampleGo's `internal/application/repositories/README.md` states for
`ClientRepository`.

## Current compliance status

financial-tracker's application layer does this today: `internal/application/dto`
exists (`movement_dto.go`, `account_dto.go`, `credit_card_purchase_dto.go`,
`exchange_rate_dto.go`), and every repository interface in
`internal/application/repositories/` (`MovementRepository`,
`AccountRepository`, `CreditCardPurchaseRepository`,
`ExchangeRateRepository`), `internal/application/services/sync.go`'s
`LedgerGateway`, and every usecase interface in
`internal/application/usecases/interfaces.go` take/return
`application/dto` types (`*dto.MovementDTO`, `*dto.AccountDTO`, etc.), not
`*internal/domain/entities.Movement` directly. Infrastructure adapts at
the boundary as the pattern requires: `internal/infrastructure/sqlite`
and `internal/infrastructure/postgresql` scan DB rows straight into
`dto.MovementDTO`/`dto.AccountDTO`/..., and
`internal/infrastructure/ledgerservice`'s wire `Transaction.ToDTO()`
converts ledger-service's JSON shape to `*dto.MovementDTO` before
`gateway.Publish` (which itself takes `*dto.MovementDTO`) hands it up.
`CurrencyRepository` never needed a DTO — it always dealt in plain
`string` codes.

This is not a case of every domain entity having vanished from the
application layer, and that's fine: several usecases (e.g.
`create_movement.go`, `cancel_movement.go`, `update_movement.go`,
`create_credit_card_purchase.go`) still construct an
`entities.Movement` internally — to run domain validation/enum checks and
build the row's shape — before converting it to `dto.MovementDTO` via
`dto.MovementFromEntity(...)` at the point they hand it to a repository.
That's fully compliant: the entity never crosses a contract boundary: the
repository interface itself, the thing another package actually depends
on, is typed against `dto.MovementDTO` from end to end.

The historical gap this section used to describe (contracts typed
directly against `domain/entities`, no `application/dto` package at all)
is fixed. There is no known outstanding violation of this rule in the
current codebase; if you find one while working here, treat it as a bug,
not as the documented standard.

## Worked example: infra adapting an *external* system (`LedgerGateway`)

The `MovementRepository`/SQLite example above shows infra adapting our
own database's row shape. `internal/infrastructure/ledgerservice` is the sharper
version of the same rule, because it crosses a real external-system
boundary, not just a DB row: **infrastructure's job is to adapt whatever
comes in — a DB row, another service's JSON, anything — to the contract
the application layer defined, not the other way around.** This is real,
current code, and it already gets both the *shape* of the pattern right
— adapting at the boundary with explicit conversion functions, not
letting ledger-service's format leak past
`internal/infrastructure/ledgerservice` — and the *target type*: it
converts to `dto.MovementDTO`, not a domain entity.

What's there today, `internal/infrastructure/ledgerservice/entities/transaction.go`
— ledger-service's own wire format, private to this package:

```go
// wire.Transaction / wire.TransactionRequest are ledger-service's JSON
// shape. internal/domain/entities and application code never see these types.
type Transaction struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Amount    int64     `json:"amount"`
	Currency  string    `json:"currency"`
	Timestamp time.Time `json:"timestamp"`
}

func (t Transaction) ToDTO() *dto.MovementDTO {
	return &dto.MovementDTO{
		ID:        t.ID,
		UserID:    t.UserID,
		Amount:    t.Amount,
		Currency:  t.Currency,
		Timestamp: t.Timestamp,
	}
}

type TransactionRequest struct {
	UserID   string `json:"user_id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}
```

And the adapter itself, `internal/infrastructure/ledgerservice/gateway.go` —
notice it already narrows the full `MovementDTO` down to just the three
fields ledger-service's contract accepts, by hand, field by field (no
reflection, no generic mapper — an explicit map is the point, so a field
ledger-service *shouldn't* see can't leak through by accident):

```go
// gateway adapts Client to the application layer's LedgerGateway port
// (application/services). Only the money facts cross the wire —
// ledger-service's transaction model doesn't know about descriptions,
// categories, or payment methods.
func (g *gateway) Publish(ctx context.Context, movement *dto.MovementDTO) (string, error) {
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

`LedgerGateway`'s port (`internal/application/services/sync.go`) matches
that same shape — `dto.MovementDTO` in, `ledgerTransactionID` out:

```go
// internal/application/services/sync.go
type LedgerGateway interface {
	Publish(ctx context.Context, movement *dto.MovementDTO) (ledgerTransactionID string, err error)
}
```

That's the whole point of putting the DTO at the application boundary:
the adapter's actual logic (which fields cross the wire, which don't) is
the part that matters, and it was already correct — the type it adapts
*to* is what had to line up with the rest of the application layer, and
now does.

## Rich entities: single-account logic belongs on `Account`, not the usecase

CleanExampleGo's domain layer isn't just data — its `README.md` calls
this out explicitly ("Rich Domain Model: Entities should have behavior,
not just data") and `internal/domain/entities/README.md` gives `Book.Borrow()` /
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
// internal/domain/entities/account.go

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
	if err := a.validateTransfer(to, amount); err != nil {
		return nil, err
	}
	return a.transferLeg(-amount, description, timestamp), nil
}

// Receive is Send's mirror for the destination side — same validation,
// the credit leg. Kept as its own method (not derived from Send) so each
// side can grow independently: e.g. a "did the target actually receive
// it" confirmation/monitoring hook later belongs here, not on the source
// account's method.
func (a *Account) Receive(from *Account, amount int64, description string, timestamp time.Time) (*Movement, error) {
	if err := a.validateTransfer(from, amount); err != nil {
		return nil, err
	}
	return a.transferLeg(amount, description, timestamp), nil
}

// validateTransfer and transferLeg are the shared same-currency/
// positive-amount/not-self checks and Movement-building logic behind
// both Send and Receive.
func (a *Account) validateTransfer(other *Account, amount int64) error {
	if other == nil {
		return errors.New("other account is required")
	}
	if a.ID != "" && other.ID != "" && a.ID == other.ID {
		return errors.New("cannot transfer to the same account")
	}
	if a.Currency != other.Currency {
		return fmt.Errorf("cross-currency transfers aren't supported yet (%q vs %q)", a.Currency, other.Currency)
	}
	if amount <= 0 {
		return errors.New("amount must be positive")
	}
	return nil
}

func (a *Account) transferLeg(amount int64, description string, timestamp time.Time) *Movement {
	return &Movement{
		UserID:        a.UserID,
		Amount:        amount,
		Currency:      a.Currency,
		Description:   description,
		Category:      CategoryTransfer,
		PaymentMethod: PaymentMethodBankTransfer,
		AccountID:     &a.ID,
		Status:        MovementStatusActive,
		SyncStatus:    SyncStatusPending,
		Timestamp:     timestamp,
		CreatedAt:     time.Now().UTC(),
	}
}
```

The usecase calls both, then owns the parts entities must never do —
loading via repositories and persisting atomically:

```go
// internal/application/usecases/transfer_between_account.go — this is
// the actual current implementation, dto conversions included (accounts
// come back from the repository as dto.AccountDTO; .ToEntity() runs the
// entity's Send/Receive, then dto.MovementFromEntity converts each leg
// back before it's persisted).

func (uc *transferBetweenAccountsUseCase) Execute(ctx context.Context, input TransferBetweenAccountsInput) (TransferResult, error) {
	fromDTO, err := uc.ownedAccount(ctx, input.FromAccountID, input.UserID)
	if err != nil {
		return TransferResult{}, err
	}
	toDTO, err := uc.ownedAccount(ctx, input.ToAccountID, input.UserID)
	if err != nil {
		return TransferResult{}, err
	}

	timestamp := input.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	from, to := fromDTO.ToEntity(), toDTO.ToEntity()
	debit, err := from.Send(to, input.Amount, input.Description, timestamp)
	if err != nil {
		return TransferResult{}, fmt.Errorf("%w: %v", apperrors.ErrInvalidInput, err)
	}
	credit, err := to.Receive(from, input.Amount, input.Description, timestamp)
	if err != nil {
		return TransferResult{}, fmt.Errorf("%w: %v", apperrors.ErrInvalidInput, err)
	}

	// Linking the pair is cross-entity orchestration — the usecase's job.
	transferID := id.NewUUID()
	debit.TransferID, credit.TransferID = &transferID, &transferID

	// Still the usecase's job: atomic persistence across both legs.
	created, err := uc.movements.CreateBatch(ctx, []*dto.MovementDTO{
		dto.MovementFromEntity(debit),
		dto.MovementFromEntity(credit),
	})
	if err != nil {
		return TransferResult{}, err
	}
	return TransferResult{TransferID: transferID, Debit: created[0], Credit: created[1]}, nil
}
```

Same rule as the DTO one above: don't reinvent this per-usecase. If a
usecase is about to inline a single-entity validate-and-build step
directly instead of calling an entity method like `Account.Send`/
`Account.Receive`, that's the signal it belongs on the entity instead.

## Current compliance status (entity methods)

`Account.Send`/`Account.Receive` exist in
`internal/domain/entities/account.go` and
`transfer_between_account.go` calls them — this principle **is** applied
for transfers today, matching the worked example above. It's still
scoped to transfers only: the rest of the usecases that build a
`Movement` (`update_movement.go`, `cancel_movement.go`,
`cancel_transfer.go`, `create_credit_card_purchase.go`) construct
`entities.Movement` literals directly inline rather than through an
entity method, and haven't been individually reviewed against this
principle yet — do that case-by-case when touching each, rather than
assuming they all need the same treatment (most of them are cancel/
reversal logic, which doesn't obviously map onto a `Send`/`Receive`-style
method the way a transfer's two symmetric legs did).

## Everything else

The rest of the request-flow and conventions already match CleanExampleGo
and are documented in this folder's `README.md`:

- Handlers never touch repositories directly.
- Usecases never import `database/sql` or `net/http`.
- Constructors return interface types.
- Contracts sit next to what implements them: every use-case contract
  (interface + Input/Result/View types) consolidated in
  `internal/application/usecases/interfaces.go` (this workspace's amended
  rule, see AGENTS.md), one file per aggregate under
  `internal/application/repositories/` — never declared inline next to an
  unrelated feature's implementation or consumer.
- `internal/cmd/api/main.go` is the sole composition root.
