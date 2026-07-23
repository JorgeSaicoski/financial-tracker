# Walkthrough — a feature that needs application changes but little new infrastructure

Not every feature needs [new-feature.md](new-feature.md)'s full stack.
Some features are new **operations** over data that's already stored, not
new **state**. Before writing a migration or a repository method, ask:
*does this need somewhere new to put data, or does it just need a new
rule/composition over data that already has somewhere to live?* If it's
the latter, most of the full walkthrough's steps don't apply, and
skipping straight to them is how you end up with a duplicate table or a
pointless repository method that just calls another repository method.

**Worked example: transfer money between two accounts.** When this
walkthrough was written, transfer wasn't implemented yet, and the
prediction below was: no migration, no entity changes, no new repository
— just a usecase composing two existing `Create` calls. Transfer has
since been built for real (`POST /transfers`, `POST /transfers/{id}/cancel`),
and that prediction was **wrong in an instructive way**: it turned out to
need a migration after all (`005_add_movement_transfer_id.sql`, a
nullable `transfer_id` column) and an entity change (`Movement.TransferID`),
because linking the two legs by `transfer_id` — so cancelling one cancels
both, and a leg can't be cancelled alone — needs to survive a DB
round-trip, not just live in memory during one request. The two-`Create`
version below also has the atomicity gap called out further down, which
the real implementation closes with `MovementRepository.CreateBatch`
wrapping both inserts in one transaction (see
`infrastructure/sqlite/movement_repository.go`), exactly the "if this
were a real PR" option this walkthrough names as (b).

Read the real thing at `application/usecases/transfer_between_account.go`,
`application/usecases/cancel_transfer.go`, and
`interfaces/api/handlers/transfer_handler.go` for what it actually looks
like today. The walkthrough below is kept as-is, unedited from before the
real feature existed, because the mismatch is the lesson: **Step 0's
"what already exists" check is where you write down your best guess, not
a guarantee** — you can be wrong about needing a migration, and finding
out you were wrong two steps in is normal, not a process failure.

**Architecture note:** the code below (like the real implementation)
types the repository/usecase contract as `*entities.Movement` directly.
Per `contributing/architecture.md`, that's a known gap against
CleanExampleGo's `application/dto` layer, not the target shape — see that
doc before copying this pattern into new work. The same doc's "Rich
entities" section also shows what the two `Create` calls below would look
like as `Account.Send`/`Account.Receive` methods instead of inline
`Movement` construction in the usecase.

## Step 0 — work out what's actually new, before writing anything

Check off what already exists:

- **Storage** — `movements` table already has `account_id`, `amount`
  (signed), `currency`, `category`. A transfer is two rows in a table
  that already exists. **No migration.**
- **Repository methods** — `MovementRepository.Create` already inserts
  one movement; `AccountRepository.GetByID` already fetches an account.
  A transfer needs to call each of those twice. **No new repository
  interface, no new SQLite file.**
- **Domain entities** — `Movement` and `Account` already model everything
  a transfer touches. `entities.CategoryTransfer` already exists — reuse
  it instead of inventing a new category. **No entity changes.**

What's actually new:

- **A business rule that doesn't exist yet**: two accounts must share a
  currency to transfer between them directly (cross-currency transfer
  would need a conversion rate, which is a separate feature — reject it
  for now, don't half-build it). This rule belongs in the usecase, same
  as `CreateMovement`'s existing "movement currency must match account
  currency" check in `application/usecases/create_movement.go`.
- **One new use-case contract + implementation** that composes the two
  existing repository calls.
- **One new handler + route + DTOs + wiring** — same shape as any other
  endpoint, [new-feature.md](new-feature.md) steps 6–9 apply unchanged.

That's the whole point of this walkthrough: the migration, entity,
repository-interface, and SQLite-implementation steps are **skipped
entirely**, and nobody should feel obligated to fill them in just
because the full walkthrough had them.

## Step 1 — the contract: `application/usecases/interfaces.go`

Contracts never live in the implementation file — the interface and its
Input/Result structs go into the consolidated `interfaces.go`, in their
own `// ---- Transfers ----` section:

```go
// ---- Transfers ----

// TransferBetweenAccountsInput describes moving money from one of the
// user's accounts to another. Amount is positive — the amount that
// leaves FromAccountID and arrives in ToAccountID.
type TransferBetweenAccountsInput struct {
	UserID        string
	FromAccountID string
	ToAccountID   string
	Amount        int64
	Description   string
}

// TransferBetweenAccountsResult is the two linked movements the transfer
// created: a debit on the source account, a credit on the destination.
type TransferBetweenAccountsResult struct {
	Debit  *entities.Movement
	Credit *entities.Movement
}

type TransferBetweenAccountsUseCase interface {
	Execute(ctx context.Context, input TransferBetweenAccountsInput) (TransferBetweenAccountsResult, error)
}
```

## Step 2 — the implementation: `application/usecases/transfer_between_accounts.go`

```go
package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

type transferBetweenAccountsUseCase struct {
	accounts  repositories.AccountRepository
	movements repositories.MovementRepository
}

// NewTransferBetweenAccounts returns interface type for dependency
// injection. Note it takes the two repositories that already exist —
// no new one.
func NewTransferBetweenAccounts(
	accounts repositories.AccountRepository,
	movements repositories.MovementRepository,
) TransferBetweenAccountsUseCase {
	return &transferBetweenAccountsUseCase{accounts: accounts, movements: movements}
}

func (uc *transferBetweenAccountsUseCase) Execute(
	ctx context.Context, input TransferBetweenAccountsInput,
) (TransferBetweenAccountsResult, error) {
	if input.UserID == "" || input.FromAccountID == "" || input.ToAccountID == "" || input.Amount <= 0 {
		return TransferBetweenAccountsResult{}, apperrors.ErrInvalidInput
	}
	if input.FromAccountID == input.ToAccountID {
		return TransferBetweenAccountsResult{}, fmt.Errorf("%w: can't transfer an account to itself", apperrors.ErrInvalidInput)
	}

	from, err := uc.accounts.GetByID(ctx, input.FromAccountID)
	if err != nil {
		return TransferBetweenAccountsResult{}, err
	}
	to, err := uc.accounts.GetByID(ctx, input.ToAccountID)
	if err != nil {
		return TransferBetweenAccountsResult{}, err
	}
	// Cross-currency transfer needs a conversion rate — a separate
	// feature. Reject rather than silently move the wrong amount.
	if from.Currency != to.Currency {
		return TransferBetweenAccountsResult{}, fmt.Errorf(
			"%w: accounts have different currencies (%s vs %s); cross-currency transfer isn't supported yet",
			apperrors.ErrInvalidInput, from.Currency, to.Currency)
	}

	now := time.Now().UTC()
	base := entities.Movement{
		UserID:        input.UserID,
		Currency:      from.Currency,
		Description:   input.Description,
		Category:      entities.CategoryTransfer, // already exists — no new enum value
		PaymentMethod: entities.PaymentMethodOther,
		Status:        entities.MovementStatusActive,
		SyncStatus:    entities.SyncStatusPending,
		Timestamp:     now,
		CreatedAt:     now,
	}

	debit := base
	debit.Amount = -input.Amount
	debit.AccountID = &input.FromAccountID
	debitCreated, err := uc.movements.Create(ctx, &debit)
	if err != nil {
		return TransferBetweenAccountsResult{}, err
	}

	credit := base
	credit.Amount = input.Amount
	credit.AccountID = &input.ToAccountID
	creditCreated, err := uc.movements.Create(ctx, &credit)
	if err != nil {
		// The debit above already committed — see the atomicity note
		// below before shipping this in a real PR.
		return TransferBetweenAccountsResult{}, err
	}

	return TransferBetweenAccountsResult{Debit: debitCreated, Credit: creditCreated}, nil
}
```

**A gap to be honest about, not hide**: this calls `Create` twice against
two independent inserts. If the second insert fails after the first
succeeds, the transfer is half-done — money left one account and never
arrived in the other. `application/usecases/cancel_movement.go` already
solved exactly this shape of problem for reversals, via
`MovementRepository.CreateReversal`, whose SQLite implementation
(`infrastructure/sqlite/movement_repository.go`) wraps both the insert
and the linked update in one `db.BeginTx`/`Commit` transaction. If this
were a real PR, the honest options are: (a) ship the two-`Create`
version and write down the gap in README's "MVP scope / known
limitations" the same way the sync idempotency gap is documented there,
or (b) add one new repository method,
`CreateTransferPair(ctx, debit, credit *entities.Movement) (*entities.Movement, *entities.Movement, error)`,
whose SQLite implementation wraps both inserts in one transaction — this
is the one place where this feature *would* earn a small, deliberate
piece of new infra, added because a real correctness gap demands it, not
by default.

## Step 3 — DTOs: `interfaces/dto/movement_dto.go` (or a new file)

```go
type TransferRequest struct {
	FromAccountID string `json:"from_account_id"`
	ToAccountID   string `json:"to_account_id"`
	Amount        int64  `json:"amount"`
	Description   string `json:"description,omitempty"`
}

type TransferResponse struct {
	Debit  MovementResponse `json:"debit"`
	Credit MovementResponse `json:"credit"`
}
```

Reuses `MovementResponse`, which already exists — no new response shape
for a single movement, just a wrapper holding two of them.

## Step 4 — handler + route

A handler method on the existing `AccountHandler` (it already owns
account-shaped operations) decodes `TransferRequest`, calls
`TransferBetweenAccountsUseCase.Execute`, maps `ErrInvalidInput`/
`ErrNotFound` the same way every other handler in this codebase does via
`writeUsecaseError`, and responds with `TransferResponse` built from
`toMovementResponse` (already exists in `movement_handler.go` — either
reuse it by making it a package-level function, or move it next to the
other DTO-mapping helpers in `http_helpers.go` if two handlers need it).
Route addition to `interfaces/api/router.go`:

```go
mux.HandleFunc("POST /accounts/transfer", accountHandler.Transfer)
```

## Step 5 — wiring in `cmd/api/main.go`

```go
transfer := usecases.NewTransferBetweenAccounts(accountRepo, movementRepo)
accountHandler := handlers.NewAccountHandler(createAccount, listAccounts, reportBalance, transfer, defaultUserID, log)
```

No new repository is constructed here — `accountRepo` and `movementRepo`
already exist from wiring earlier features.

## Step 6 — tests

`application/usecases/transfer_between_accounts_test.go` uses
`fakeAccountRepo` and `fakeMovementRepo` — both already exist in
`fakes_test.go` from earlier features. **No new fake needed.** That's
the same "little new infrastructure" story showing up again at the test
layer: a feature built entirely from existing repository interfaces
needs no new fakes to test it, either.

## What this walkthrough is really teaching

The skill isn't "how to build a transfer feature" — it's **recognizing
which walkthrough a feature needs before you start**. Default to
assuming a new feature needs the full [new-feature.md](new-feature.md)
treatment, and you'll build tables and repository methods nobody needed.
Default to assuming every feature is "just a usecase," and you'll
eventually bolt a feature onto a table it doesn't belong in. Step 0
above — listing what already exists before writing anything — is the
actual step to copy, more than any of the code that follows it.
