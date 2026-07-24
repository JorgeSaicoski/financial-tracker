# Walkthrough — a complete new feature

**What we're building:** a currency registry — `GET /currencies` (list
codes) and `POST /currencies` (register a new one) — so the frontend's
currency dropdown is real data instead of two hardcoded options. This is
the actual feature at `internal/interfaces/api/handlers/currency_handler.go`
et al. today; every file below is copy-pasted from the repo, not
paraphrased.

Before starting, read [architecture.md](architecture.md) and "Where
contracts live" in [README.md](README.md) — steps 2 and 4 below put
each piece in its required place. Currency has no domain entity, so this
particular walkthrough doesn't hit the `internal/application/dto` gap
architecture.md describes — `CurrencyRepository` already takes/returns
plain `string`s, not an entity. Features built around `Movement` or
`Account` (most of them) do hit it; see that doc before typing a domain
entity into a new contract.

## Step 1 — migration: `migrations/003_create_currencies_table.sql`

New features that need storage start with a numbered migration.
`go:embed *.sql` in `migrations/migrations.go` picks up any `.sql` file
in the directory automatically; `sqlite.Migrate` applies unapplied ones
in filename order on boot, tracked by name in `schema_migrations`. Number
it one past the highest existing file.

```sql
-- currencies is the user-extendable registry backing the frontend's
-- currency dropdown (GET /currencies). Movements store currency as plain
-- text, so the seed also backfills any code already present in the data.
CREATE TABLE IF NOT EXISTS currencies (
    code       TEXT PRIMARY KEY,
    created_at TEXT NOT NULL
);

INSERT OR IGNORE INTO currencies (code, created_at)
VALUES ('usd', strftime('%Y-%m-%dT%H:%M:%f000000Z', 'now')),
       ('brl', strftime('%Y-%m-%dT%H:%M:%f000000Z', 'now'));

INSERT OR IGNORE INTO currencies (code, created_at)
SELECT DISTINCT currency, strftime('%Y-%m-%dT%H:%M:%f000000Z', 'now')
FROM movements;
```

Never edit a migration that's already shipped — write a new file instead,
even to fix a typo in one that's out.

## Step 2 — repository interface: `internal/application/repositories/currency_repository.go`

The usecase layer will depend on this interface, never on SQLite
directly. Keep it to exactly what usecases need, and write down any
semantic promise (like "`Add` is idempotent") here, because both the fake
used in tests and the real SQLite implementation must honor it.

```go
package repositories

import "context"

// CurrencyRepository is the registry of currency codes the user tracks
// (usd, brl, btc, ...). It exists so the frontend dropdown is data, not
// hardcoded — movements themselves store the code as plain text.
type CurrencyRepository interface {
	List(ctx context.Context) ([]string, error)
	// Add registers a code; adding an existing code is a no-op.
	Add(ctx context.Context, code string) error
}
```

## Step 3 — SQLite implementation: `internal/infrastructure/sqlite/currency_repository.go`

```go
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
)

type currencyRepository struct {
	db *sql.DB
}

// NewCurrencyRepository returns the domain interface type, not the
// concrete struct, so callers depend only on the contract.
func NewCurrencyRepository(db *sql.DB) repositories.CurrencyRepository {
	return &currencyRepository{db: db}
}

func (r *currencyRepository) List(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT code FROM currencies ORDER BY code ASC`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query currencies: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		out = append(out, code)
	}
	return out, rows.Err()
}

func (r *currencyRepository) Add(ctx context.Context, code string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO currencies (code, created_at) VALUES (?, ?)`,
		code, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("sqlite: insert currency: %w", err)
	}
	return nil
}
```

`formatTime` is a shared helper already defined in this package
(`internal/infrastructure/sqlite/db.go`) — reuse it instead of formatting time
yourself, so every table sorts timestamps identically.

## Step 4 — use-case contract + implementation: `internal/application/usecases/currencies.go`

Every use-case interface and its Input/Result types go in the **same file**
as the concrete implementation that satisfies it — one file per use case
(CleanExampleGo's own rule: "one file per use case!"), never a separate
consolidated file. The interface comes first, then the concrete struct,
constructor, and `Execute` body. This file has two usecases because
listing and adding are independent operations with independent contracts
— that's the pattern used throughout `internal/application/usecases`:

```go
package usecases

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

// currencyCodePattern keeps codes short lowercase identifiers (usd, brl,
// btc, usdt, ...) so they behave everywhere a currency string is used.
var currencyCodePattern = regexp.MustCompile(`^[a-z0-9]{2,10}$`)

type ListCurrenciesUseCase interface {
	Execute(ctx context.Context) ([]string, error)
}

type listCurrenciesUseCase struct {
	repo repositories.CurrencyRepository
}

// NewListCurrencies returns interface type for dependency injection.
func NewListCurrencies(repo repositories.CurrencyRepository) ListCurrenciesUseCase {
	return &listCurrenciesUseCase{repo: repo}
}

func (uc *listCurrenciesUseCase) Execute(ctx context.Context) ([]string, error) {
	return uc.repo.List(ctx)
}

// AddCurrencyUseCase registers a new currency code; adding an existing
// code is a no-op. Returns the normalized (lowercased) code.
type AddCurrencyUseCase interface {
	Execute(ctx context.Context, code string) (string, error)
}

type addCurrencyUseCase struct {
	repo repositories.CurrencyRepository
}

// NewAddCurrency returns interface type for dependency injection.
func NewAddCurrency(repo repositories.CurrencyRepository) AddCurrencyUseCase {
	return &addCurrencyUseCase{repo: repo}
}

func (uc *addCurrencyUseCase) Execute(ctx context.Context, code string) (string, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if !currencyCodePattern.MatchString(code) {
		return "", fmt.Errorf("%w: currency code must be 2-10 lowercase letters or digits", apperrors.ErrInvalidInput)
	}
	if err := uc.repo.Add(ctx, code); err != nil {
		return "", err
	}
	return code, nil
}
```

If your feature has request-shaped data crossing the boundary, its struct
goes in the same file too — see `CreateMovementInput` in
`create_movement.go` or `CreateAccountInput` in `create_account.go` for
the pattern. A type shared by two use cases (e.g. `AccountView`, returned
by both `ListAccountsUseCase` and `ReportAccountBalanceUseCase`) lives in
whichever file returns it first, with a one-line comment pointing at the
other consumer — same package, no import needed. All validation and
normalization lives in the impl, not in the handler and not in SQL.

`apperrors.ErrInvalidInput` is one of four sentinels in `internal/pkg/errors`
(`ErrInvalidInput`, `ErrNotFound`, `ErrConflict`, `ErrUpstream`). Wrap it
with `fmt.Errorf("%w: <human message>", ...)` — the message is what a 400
response shows the caller, so write it for a person, not a log.

## Step 5 — request/response DTOs: `internal/interfaces/dto/account_dto.go`

DTOs are plain structs with JSON tags, snake_case on the wire. These two
live in `account_dto.go` alongside the account DTOs added in the same
change — a new file per feature is also fine; there's no hard rule, just
don't scatter one feature's DTOs across unrelated files.

```go
type CurrenciesResponse struct {
	Currencies []string `json:"currencies"`
}

type AddCurrencyRequest struct {
	Code string `json:"code"`
}
```

## Step 6 — handler: `internal/interfaces/api/handlers/currency_handler.go`

```go
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

// CurrencyHandler exposes the user-extendable currency registry backing
// the frontend's currency dropdown.
type CurrencyHandler interface {
	ListCurrencies(w http.ResponseWriter, r *http.Request)
	AddCurrency(w http.ResponseWriter, r *http.Request)
}

type currencyHandler struct {
	listCurrencies usecases.ListCurrenciesUseCase
	addCurrency    usecases.AddCurrencyUseCase
	log            logger.Logger
}

// NewCurrencyHandler returns interface type for dependency injection.
func NewCurrencyHandler(
	listCurrencies usecases.ListCurrenciesUseCase,
	addCurrency usecases.AddCurrencyUseCase,
	log logger.Logger,
) CurrencyHandler {
	return &currencyHandler{listCurrencies: listCurrencies, addCurrency: addCurrency, log: log}
}

// ListCurrencies handles GET /currencies.
func (h *currencyHandler) ListCurrencies(w http.ResponseWriter, r *http.Request) {
	currencies, err := h.listCurrencies.Execute(r.Context())
	if err != nil {
		h.log.Error("list currencies failed: %v", err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(h.log, w, http.StatusOK, interfacedto.CurrenciesResponse{Currencies: currencies})
}

// AddCurrency handles POST /currencies. Adding an existing code is a
// no-op success, so the frontend can add without checking first.
func (h *currencyHandler) AddCurrency(w http.ResponseWriter, r *http.Request) {
	var req interfacedto.AddCurrencyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "invalid request body")
		return
	}

	if _, err := h.addCurrency.Execute(r.Context(), req.Code); err != nil {
		if apperrors.Is(err, apperrors.ErrInvalidInput) {
			writeError(h.log, w, http.StatusBadRequest, err.Error())
			return
		}
		h.log.Error("add currency failed: %v", err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
		return
	}

	currencies, err := h.listCurrencies.Execute(r.Context())
	if err != nil {
		h.log.Error("list currencies after add failed: %v", err)
		writeError(h.log, w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(h.log, w, http.StatusCreated, interfacedto.CurrenciesResponse{Currencies: currencies})
}
```

(Handler interfaces are the one contract kind that stays next to its
implementation — they're the interfaces *layer's* own surface, consumed
only by the router one file away.)

`writeJSON` and `writeError` are shared across every handler, defined
once in `internal/interfaces/api/handlers/http_helpers.go`:

```go
func writeJSON(log logger.Logger, w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Error("failed to encode JSON response: %v", err)
	}
}

func writeError(log logger.Logger, w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(interfacedto.ErrorResponse{Error: message}); err != nil {
		log.Error("failed to encode error response: %v", err)
	}
}
```

Always use these instead of writing headers/encoding JSON by hand in a
new handler — every response gets the same `Content-Type` and the same
error shape (`{"error": "..."}`) this way.

## Step 7 — route: `internal/interfaces/api/router.go`

Add two lines to `NewRouter`. If your feature needs a brand-new handler
*type* (as currencies did), add it as a parameter too — the compiler will
point you at every call site that needs updating (`main.go`, tests).

```go
func NewRouter(
	movementHandler handlers.MovementHandler,
	accountHandler handlers.AccountHandler,
	currencyHandler handlers.CurrencyHandler, // added
) http.Handler {
	mux := http.NewServeMux()

	// ...existing movement/account routes...

	mux.HandleFunc("GET /currencies", currencyHandler.ListCurrencies)
	mux.HandleFunc("POST /currencies", currencyHandler.AddCurrency)

	return withCORS(mux)
}
```

Path parameters use Go 1.22's `ServeMux` patterns directly, e.g.
`"POST /accounts/{id}/balance"`, read in the handler with
`r.PathValue("id")` — see `internal/interfaces/api/handlers/account_handler.go`
for a real example if your route needs one.

## Step 8 — wire it up: `internal/cmd/api/main.go`

This is the only file where concrete SQLite repos, usecases, and
handlers are actually constructed and connected. Three additions:

```go
// with the other repositories:
currencyRepo := sqlite.NewCurrencyRepository(db)

// with the other usecases:
listCurrencies := usecases.NewListCurrencies(currencyRepo)
addCurrency := usecases.NewAddCurrency(currencyRepo)

// with the other handlers:
currencyHandler := handlers.NewCurrencyHandler(listCurrencies, addCurrency, log)

// pass it into the router:
router := api.NewRouter(movementHandler, accountHandler, currencyHandler)
```

Also extend the startup log line that lists every endpoint, so
`make logs` always shows the truth:

```go
log.Info("endpoints: POST /movements | GET /movements | POST /movements/{id}/cancel | " +
	"POST /credit-card-purchases/{id}/cancel | POST /sync | GET /categories | GET /cashflow | " +
	"GET|POST /accounts | POST /accounts/{id}/balance | GET|POST /currencies")
```

## Step 9 — tests

Usecase tests live in `internal/application/usecases/*_test.go` and run against
in-memory fakes, not a real database. Add your fake to
`internal/application/usecases/fakes_test.go`:

```go
// fakeCurrencyRepo is an in-memory CurrencyRepository. Add is idempotent,
// matching the real SQLite implementation's INSERT OR IGNORE.
type fakeCurrencyRepo struct {
	codes map[string]bool
}

func newFakeCurrencyRepo(seed ...string) *fakeCurrencyRepo {
	f := &fakeCurrencyRepo{codes: map[string]bool{}}
	for _, c := range seed {
		f.codes[c] = true
	}
	return f
}

func (f *fakeCurrencyRepo) List(_ context.Context) ([]string, error) {
	out := make([]string, 0, len(f.codes))
	for c := range f.codes {
		out = append(out, c)
	}
	sort.Strings(out)
	return out, nil
}

func (f *fakeCurrencyRepo) Add(_ context.Context, code string) error {
	f.codes[code] = true
	return nil
}
```

Then the actual test file, `internal/application/usecases/currencies_test.go`:

```go
package usecases

import (
	"context"
	"errors"
	"testing"

	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func TestAddCurrencyNormalizesAndValidates(t *testing.T) {
	uc := NewAddCurrency(newFakeCurrencyRepo())

	code, err := uc.Execute(context.Background(), "  BTC  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "btc" {
		t.Errorf("code = %q, want lowercased/trimmed %q", code, "btc")
	}
}

func TestAddCurrencyRejectsBadCodes(t *testing.T) {
	uc := NewAddCurrency(newFakeCurrencyRepo())

	for _, bad := range []string{"", "x", "this-is-way-too-long", "USD!"} {
		if _, err := uc.Execute(context.Background(), bad); !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Errorf("code %q: want ErrInvalidInput, got %v", bad, err)
		}
	}
}

func TestAddCurrencyIsIdempotent(t *testing.T) {
	repo := newFakeCurrencyRepo("usd")
	uc := NewAddCurrency(repo)

	if _, err := uc.Execute(context.Background(), "usd"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list := NewListCurrencies(repo)
	codes, err := list.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(codes) != 1 || codes[0] != "usd" {
		t.Errorf("codes = %v, want exactly [usd]", codes)
	}
}

func TestListCurrencies(t *testing.T) {
	repo := newFakeCurrencyRepo("usd", "brl")
	uc := NewListCurrencies(repo)

	codes, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(codes) != 2 {
		t.Errorf("codes = %v, want 2 entries", codes)
	}
}
```

Run everything:

```bash
go build ./... && go vet ./... && go test ./...
```

If you also touched a repository interface, `go vet` will fail on every
fake and SQLite test that implements it until you update them all — that
failure is the compiler doing your review for you, not a problem to work
around.

## Step 10 — frontend API client: `web/src/lib/api.js`

One exported function per endpoint, built on the shared `request()`
helper already in this file (it sets the base URL and JSON headers, and
throws `Error(body.error)` on any non-2xx response — so every page-level
caller just needs a `try { } catch (err) { error = err.message }`):

```js
export function getCurrencies() {
	return request('/currencies');
}

export function addCurrency(code) {
	return request('/currencies', {
		method: 'POST',
		body: JSON.stringify({ code })
	});
}
```

## Step 11 — frontend page: `web/src/routes/+page.svelte`

State, with a safe default so the UI works even before the API call
resolves:

```js
let currencies = $state(['usd', 'brl']);
```

A load function, called from `onMount`:

```js
async function loadCurrencies() {
	try {
		const data = await getCurrencies();
		if (data.currencies?.length) currencies = data.currencies;
	} catch {
		// Keep the usd/brl defaults.
	}
}
```

```js
onMount(() => {
	load();
	loadCategories();
	loadCurrencies();   // added
});
```

An action handler that calls the mutating endpoint and updates local
state from the response instead of re-fetching:

```js
async function handleAddCurrency() {
	const code = prompt('New currency code (e.g. btc, eur):');
	if (!code) return;
	error = '';
	try {
		const data = await addCurrency(code.trim().toLowerCase());
		currencies = data.currencies ?? currencies;
		currencyInput = code.trim().toLowerCase();
	} catch (err) {
		error = err.message;
	}
}
```

And markup binding the state:

```svelte
<select bind:value={currencyInput} aria-label="Currency">
	{#each currencies as currency (currency)}
		<option value={currency}>{currency.toUpperCase()}</option>
	{/each}
</select>
<button type="button" class="ghost" title="Add a currency" onclick={handleAddCurrency}>+</button>
```

## Step 12 — see it run

Go code only exists inside the API container after an image rebuild —
the container does not hot-reload:

```bash
make rebuild
curl -s localhost:8081/currencies                      # {"currencies":["brl","usd"]}
curl -s -X POST localhost:8081/currencies -d '{"code":"btc"}'
curl -s localhost:8081/currencies                      # {"currencies":["brl","btc","usd"]}
curl -s -X POST localhost:8081/currencies -d '{"code":"x"}'   # 400, bad code rejected
```

Then open `localhost:5173`, click the `+` next to the currency dropdown,
and confirm the new code shows up in the list.
