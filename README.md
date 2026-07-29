# financial-tracker

[![CI](https://github.com/JorgeSaicoski/financial-tracker/actions/workflows/ci.yml/badge.svg)](https://github.com/JorgeSaicoski/financial-tracker/actions/workflows/ci.yml)

Personal finance tracker. Records every movement (income/expense) a user makes.

> Contributing? The **[contributing/](contributing/)** folder walks through
> adding a feature end-to-end (migration → repository → usecase → handler →
> route → frontend), building a feature over existing storage, changing an
> existing route, and the bug-fix workflow — each with a real example from
> this codebase. Start at [contributing/README.md](contributing/README.md).

**Current architecture:** financial-tracker is local-first. Movements are
written to its own SQLite database (the source of truth), so creating,
listing, and cancelling movements works even when
[ledger-service](https://github.com/JorgeSaicoski/ledger-service) is down.
A background sync process pushes movements to ledger-service whenever it's
reachable (every `SYNC_INTERVAL`, or on demand via `POST /sync`); each
movement carries a `sync_status` so the UI can show what's still pending.

Movements carry a payment method (cash, debit/credit card, pix, bank
transfer, other), a free-text description, and a category from the user's
own extendable registry (`GET /categories`) — see "Categories &
avoidability" below. Credit-card purchases can be split into monthly
installments; installments only sync to ledger-service once their date
arrives. Movements can be cancelled: one that never reached ledger-service
is just voided locally, while one that already synced gets a compensating
reversal movement (ledger-service never deletes — corrections are new
transactions), which the sync then pushes.

Beyond movements, the tracker knows about:

- **Currencies** — a user-extendable registry (`usd`/`brl` seeded, add
  `btc` or anything else via `POST /currencies`) backing the frontend
  dropdown. Movements store the code as plain text.
- **Accounts** — the places money sits (bank, investment, crypto wallet,
  cash, other), each holding exactly one currency. Movements can be
  assigned to an account. The user periodically *reports* what an account
  really holds (`POST /accounts/{id}/balance`); the API then derives an
  `estimated_balance` (last report + movements since) and, once two
  reports exist, the account's **return**: the balance change the
  movements don't explain — interest/yield we couldn't know up front.
- **Cashflow** — `GET /cashflow?from&to`: money in vs money out over an
  interval, grouped per currency (usd and btc are never summed together)
  and per account. Transfers are excluded — they're neither income nor
  expense.
- **Transfers** — `POST /transfers` moves money between two of the user's
  own same-currency accounts as a linked debit/credit pair of movements
  (category `transfer`, shared `transfer_id`) that always nets to zero.
  Cancelling one (`POST /transfers/{id}/cancel`) cancels both legs, each
  per its own sync status; a single leg can't be cancelled directly via
  `POST /movements/{id}/cancel`.
- **Exchange rates** — user-managed historical rates against USD
  (`POST /exchange-rates`, `GET /exchange-rates`), a decimal string never
  a float. Backing the (not yet built) purchasing-power report; posting
  the same currency + effective date again backfills/corrects that row
  instead of duplicating it.

Backend layout follows Clean Architecture (see `CleanExampleGo` for the
reference pattern this was modeled on): the **domain** layer holds pure
entities only, and the **application** layer owns every contract —
repository interfaces, service ports, and use-case interfaces:

```
domain/entities              Movement, CreditCardPurchase, Account (+snapshots),
                             fixed Category/PaymentMethod/AccountType lists; single-entity
                             rules live here too (e.g. Account.Send()/Receive() for transfers)
application/dto              MovementDTO, AccountDTO, CreditCardPurchaseDTO, ExchangeRateDTO
                             — what repositories/services/usecases actually pass to each
                             other, converted from domain entities at the infrastructure
                             boundary
application/repositories     MovementRepository, CreditCardPurchaseRepository,
                             AccountRepository, CurrencyRepository, ExchangeRateRepository,
                             CategoryRepository
                             interfaces, expressed in application/dto types — the swap points
application/services         LedgerGateway, SyncTrigger, SyncRunner — service contracts the
                             application defines; sync/infrastructure implement them
application/usecases         every use-case interface + Input/Result/View type consolidated
                             in interfaces.go; each usecase's concrete struct/constructor/
                             logic in its own file: CreateMovement, UpdateMovement,
                             CancelMovement, CreateCreditCardPurchase,
                             CancelCreditCardPurchase, GetMovement, ListMovements (computes
                             balance), CreateAccount, ListAccounts (computes
                             balances/returns), ReportAccountBalance, GetCashflow,
                             ListCurrencies, AddCurrency, TransferBetweenAccounts,
                             CancelTransfer, SetExchangeRate, ListExchangeRates,
                             DeleteExchangeRate, ToUSD, CreateCategory, ListCategories,
                             UpdateCategory, DeleteCategory
application/sync             SyncService: pushes pending movements to ledger-service via the
                             LedgerGateway port (background ticker + manual trigger)
infrastructure/sqlite        implements the repositories on the local SQLite DB (source of truth,
                             the default)
infrastructure/postgresql    same repository contracts on Postgres instead, selected via
                             DB_DRIVER=postgres
infrastructure/ledgerservice HTTP client for ledger-service + LedgerGateway adapter
  /entities                  internal wire structs matching ledger-service's JSON
interfaces/api               HTTP handlers + router (what the Svelte app calls)
interfaces/dto               API request/response shapes
migrations/                  financial-tracker's own SQLite schema, embedded into the binary
migrations/postgres          the same schema ported to Postgres dialect, embedded separately
pkg/errors, pkg/logger, pkg/id  shared utilities
cmd/api/main.go              wiring/entrypoint
web/                         SvelteKit frontend
```

See `contributing/architecture.md` for the full layer-by-layer rationale
(why `internal/application/dto` is a separate contract from `internal/interfaces/dto`, why
single-entity logic like `Account.Send`/`Receive` belongs on the entity
rather than inlined in a usecase, and so on).

Every constructor returns its interface type, not the concrete struct —
each layer depends on a contract instead of an implementation. Usecases
know nothing about SQL or HTTP; `internal/application/sync` reaches ledger-service
only through its `LedgerGateway` port, which `internal/infrastructure/ledgerservice`
implements.

## Cancel semantics (worth understanding)

- A movement that **never synced** is set to `status=voided` — excluded
  from the balance, never pushed to ledger-service.
- A movement that **already synced** stays `active` forever (mirroring
  ledger-service's no-delete rule); cancelling it creates a reversal
  movement with the opposite amount, linked via
  `cancels_movement_id`/`reversed_by_movement_id`. Original + reversal net
  to zero in the balance, exactly as ledger-service's own records would.
- Reversals themselves can't be cancelled (no reversal-of-reversal chains).
- Cancelling a credit-card purchase applies the same rule per installment:
  due/synced installments get reversals, future ones just get voided.

## MVP scope / known limitations

- **Auth is opt-out, not opt-in.** By default the API verifies every
  request's Authentik-issued bearer token (`OIDC_ISSUER_URL` required) and
  the frontend's OIDC login flow (`web/src/lib/auth.svelte.js`) enforces
  its own guard, driven by `GET /config`'s `auth_enabled` — which is just
  the inverse of `AUTH_DISABLED`, so the two always agree. Set
  `AUTH_DISABLED=true` (see `.env.example`) for local dev / single-user
  self-hosting without a running Authentik instance: every request is then
  attributed to a fixed dev user (`DEFAULT_USER_ID`) and the frontend shows
  no login guard either.
- **No idempotency key on sync.** If a push to ledger-service succeeds but
  the response is lost, the retry duplicates the transaction there. The
  real fix is idempotency-key support in ledger-service's API (follow-up
  in that repo).
- **Installment dates are simplified**: one per month from the purchase
  date; no awareness of a card's real closing/due day.
- **Ledger-service only stores money facts** (`user_id, amount, currency`):
  description/category/payment method live only in financial-tracker's DB.
- **`PATCH /recurring-rules/{id}` can't clear `ends_at`.** Setting or
  changing it works normally; there's no way to send "remove the end
  date" through this endpoint (an omitted field means "leave unchanged",
  indistinguishable from "clear it" without a sentinel value this
  endpoint doesn't define yet).
- **`POST /import/archive` drops reversal links.** `cancels_movement_id`/
  `reversed_by_movement_id` are self-referencing foreign keys on
  `movements`, checked immediately (not deferred) by both SQLite and
  Postgres here; an original and its reversal reference each other in
  opposite directions, so no single insertion order satisfies both within
  one restore. Everything else about both rows (amount, status, currency,
  ...) restores exactly — only the explicit cross-link between them is
  lost.

## Per-user settings & entitlements (`user_settings` table)

Ledger sync is per-user, not global: each user has `ledger_sync_entitled`
(operator/billing-controlled — what they're *allowed* to use) and
`ledger_sync_enabled` (user preference — what they've *chosen*).
Effective sync is `entitled AND enabled`. A missing row means
all-`true` (today's default, unchanged behavior) — rows are only created
lazily by the first `PATCH /settings` write, so existing users need no
backfill. `cloud_storage_entitled` is stored and exposed the same way,
but nothing enforces it yet (that's BACK-19's job, once there's a paid
tier to sell).

- The sync loop (`application/sync/service.go`) excludes every
  sync-disabled user's movements from each pass — including movements
  that were already `pending` before the user turned sync off, not just
  new ones.
- A movement created while a user's effective sync is off gets
  `sync_status: "local"` instead of `"pending"`, so `GET /movements`
  never shows fake "queued" work that will never actually sync.
  Re-enabling (`PATCH /settings {"ledger_sync_enabled": true}`)
  reclassifies that user's `"local"` rows back to `"pending"` in one
  step, so the very next sync pass picks up exactly the backlog created
  while it was off — nothing already-synced is touched.
- **v1 has no admin API for entitlements** — they're operator-only, set
  directly against the database, e.g.:
  ```sql
  -- SQLite
  INSERT INTO user_settings (user_id, ledger_sync_entitled, ledger_sync_enabled, cloud_storage_entitled, created_at, updated_at)
  VALUES ('<user-id>', 0, 0, 1, datetime('now'), datetime('now'))
  ON CONFLICT(user_id) DO UPDATE SET ledger_sync_entitled = 0;
  ```
  A real admin surface is icebox (see `financial-tracker-plan.md`).

## Financial plans (`plans` table, BACK-10)

A plan is a monthly-figure goal with a pace checker computed on every read
— it never changes `status`/numbers as a side effect of a `GET`. Two
types, always in the plan's own `currency` (never summed across
currencies):

- **`stress_test`** — a hypothetical recurring cost (e.g. "what if I had a
  $500/mo car payment") that never posts a real movement. Its progress is
  the real month-to-date income minus expense (reusing `GET /cashflow`'s
  own exclusions: voided movements and the `transfer` category) minus
  `monthly_target_amount` — a real surplus/deficit against a hypothetical
  cost. v1's month-end projection linearly extrapolates month-to-date
  expenses; income is never extrapolated (it's lumpy — salary lands on
  one day), so an early-month check can flag "behind" before payday. That
  is accepted v1 behavior, not a bug.
- **`savings`** — a real target amount by a real deadline, funded by real
  movements tagged with the new nullable `movements.plan_id` column.
  Progress is the literal `SUM(amount)` of non-cancelled tagged
  movements. The recommended funding shape is a `POST /transfers` call
  with `plan_id` set — the destination leg gets tagged, category stays
  `transfer`, so the contribution never inflates `GET /cashflow`'s
  income/expense totals. A plain tagged movement (money arriving from
  outside, not a transfer) is also accepted, and — being real new money
  in a category other than `transfer` — does count in cashflow, same as
  any other income.

The pace checker (`GET /plans/{id}`'s `on_track` + `projected_shortfall`)
compares linear expected pace (day 20 of a 30-day month → 66% of the
month's figure) against actual progress. Moving a plan out of `active`
(`completed`/`abandoned`) is always an explicit `PATCH /plans/{id}`, never
implicit. Frontend is out of scope for this ticket — a plans screen is
icebox.

## Categories & avoidability (`categories` table)

Categories are a per-user, extendable registry (`GET/POST/PATCH/DELETE
/categories`), not a fixed enum — same shape as the `currencies` registry
but scoped per user. Each category carries an `avoidability_percent`
(0-100): how easy that kind of spend is to skip (a restaurant category
might be 80% avoidable; groceries might be 20%). Posting a movement with a
brand-new category name implicitly registers it at a neutral 50% default
(same idempotent-`Add` shape as `POST /currencies`) — no separate
"create category" round-trip required before first use.

`transfer` and `income` are **system categories**: not spend, so
`avoidability_percent` stays `NULL` and the API rejects any attempt to
create, rename, delete, or set an avoidability on either name. They're
lazily ensured per user on first `GET /categories`, same absence-safe
pattern as `user_settings` above — no migration backfill needed for new
users. Deleting a category still referenced by movements is allowed (it's
a label, not a foreign key) — those movements just become "unrecognized"
for avoidability purposes going forward.

For a genuine one-off spend that doesn't deserve its own category (went
karting once — 100% avoidable, never happening again), movements carry
their own optional `avoidability_percent` override
(`POST`/`PATCH /movements`), independent of `category`. A movement's
**effective avoidability** — used by BACK-12's purchasing-power report —
resolves in this order: the movement's own override if set; else its
category's stored `avoidability_percent`; else no value (excluded from
any avoidability-weighted aggregate, same exclusion `transfer` already
gets from cashflow totals).

## API

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/config` | Unauthenticated. `{standalone, auth_enabled}` — what the frontend reads before deciding whether to show the login guard. |
| `GET` | `/settings?user_id=` | The caller's own settings — entitlement (operator-controlled, read-only here) and preference. Defaults to all-`true` if the user has never touched them (no row needed). |
| `PATCH` | `/settings?user_id=` | Body: `{ledger_sync_enabled}` — the only field a user may change. Any attempt to set `ledger_sync_entitled`/`cloud_storage_entitled` (or any other key) is rejected with 400. Re-enabling reclassifies movements created while sync was off (`sync_status: "local"`) back to `"pending"`, so the next `/sync` pass pushes exactly that backlog. |
| `POST` | `/movements` | Create a movement. Body: `{amount, currency?, user_id?, description?, category?, payment_method?, installments?, account_id?, plan_id?, avoidability_percent?}`. With `payment_method="credit_card"` and `installments > 1`, splits into monthly installments and returns the purchase + its movements (no `account_id` allowed in that case). An `account_id`'s currency must match the movement's. `plan_id` (BACK-10) tags the movement as funding a savings plan — the plan must exist, belong to the caller, be `active`, be a savings (not stress-test) plan, and share the movement's currency, or the request is rejected (400). A new `category` name is implicitly registered at 50% avoidability; `avoidability_percent` (0-100) is this movement's own ad-hoc override, independent of `category` — see "Categories & avoidability" above. |
| `GET` | `/movements?id={uuid}` | Fetch one movement. |
| `GET` | `/movements?user_id={uuid}&currency=&from=&to=&limit=&offset=` | List movements + computed `balance` (voided rows excluded from the balance). `from`/`to` take `YYYY-MM-DD` or RFC 3339 (`to` is inclusive when date-only). Each row carries `status` and `sync_status`. |
| `PATCH` | `/movements/{id}` | Edit one movement. Body: any subset of `{description, category, payment_method, account_id, plan_id, amount, currency, timestamp, avoidability_percent}`. `description`/`category`/`payment_method`/`account_id`/`plan_id`/`avoidability_percent` are local-only metadata and always editable (`account_id: ""`/`plan_id: ""` clears it; `plan_id` set to a non-empty value is validated the same way as on create). `amount`/`currency`/`timestamp` edit in place if the movement hasn't synced yet; on an already-synced movement they instead produce a reversal + a replacement (both returned, original left untouched). Rejects voided/reversed movements, reversals themselves, and financial edits on a single credit-card installment or transfer leg (409 in all cases). |
| `POST` | `/movements/{id}/cancel` | Cancel one movement (void or reversal — see semantics above). Returns the movement and, if created, the reversal. |
| `POST` | `/credit-card-purchases/{id}/cancel` | Cancel a whole installment purchase. Returns which installments were voided vs reversed. |
| `POST` | `/sync` | Run one sync pass against ledger-service now. Returns `{synced, failed}`. |
| `GET` | `/categories` | The caller's own category registry (`{id, name, avoidability_percent}[]`) plus the fixed payment-method list; lazily seeds `transfer`/`income` first. |
| `POST` | `/categories` | Create a category. Body: `{name, avoidability_percent?}` (0-100; omitted defaults to 50). 400 on reserved names (`transfer`/`income`); 409 on a duplicate name (case-insensitive) for this user. |
| `PATCH` | `/categories/{id}` | Rename and/or change `avoidability_percent`. 400 outside 0-100 or on a system category. |
| `DELETE` | `/categories/{id}` | Remove a category the user owns. 400 on a system category. Movements still referencing the deleted name are untouched — they just resolve to no avoidability going forward. |
| `GET` | `/cashflow?from=&to=&user_id=` | Money in / out / net over the interval, per currency (`totals`) and per account (`by_account`, unassigned movements in their own bucket). `from`/`to` required. Transfers are excluded. |
| `GET` | `/accounts` | All accounts with `estimated_balance`, latest `reported_balance`/`reported_at`, `movements_since_report` and `last_return` (+ the valid `account_types`). |
| `POST` | `/accounts` | Create an account. Body: `{name, type?, currency?, user_id?}`. Currency must be registered; duplicate names (case-insensitive) are rejected. |
| `POST` | `/accounts/{id}/balance` | Report the account's real current balance: `{balance}` (smallest unit). Returns the updated account view, including the newly computed `last_return` when a previous report exists. |
| `GET` | `/currencies` | Registered currency codes. |
| `POST` | `/currencies` | Register a code: `{code}` (2–10 lowercase alphanumerics). Idempotent; returns the updated list. |
| `POST` | `/transfers` | Move money between two of the user's own accounts. Body: `{from_account_id, to_account_id, amount, description?, user_id?, timestamp?, plan_id?}` (`amount` positive). v1 requires both accounts to hold the same currency. Creates a linked debit (`-amount` on `from_account_id`) and credit (`+amount` on `to_account_id`) atomically, category `transfer`, sharing a `transfer_id`. `plan_id` (BACK-10), when set, tags only the credit (destination) leg — the recommended way to fund a savings plan without inflating income/expense cashflow. Returns `{transfer_id, debit, credit}`. |
| `POST` | `/transfers/{id}/cancel` | Cancel both legs of a transfer (`{id}` is the `transfer_id`). Each leg is voided or reversed independently based on its own `sync_status`, same as `/movements/{id}/cancel`. Returns `{debit, credit}`, each shaped like `POST /movements/{id}/cancel`'s response. |
| `GET` | `/exchange-rates?user_id=` | The user's exchange-rate history, grouped by currency (current rate + full history, newest `effective_from` first). |
| `POST` | `/exchange-rates` | Set/backfill a currency's rate against USD. Body: `{currency, units_per_usd, user_id?, effective_from?}` (`units_per_usd` a decimal string; `effective_from` defaults to today, normalized to midnight UTC). Posting the same `(currency, effective_from)` again replaces that row instead of duplicating it. |
| `DELETE` | `/exchange-rates/{id}` | Remove a rate row the user owns. |
| `GET` | `/recurring-rules?user_id=` | The user's recurring rules (rent, salary, subscriptions) that generate ordinary movements on schedule. |
| `POST` | `/recurring-rules` | Create a rule. Body: `{amount, currency?, user_id?, description?, category?, payment_method?, account_id?, day_of_month, starts_at?, ends_at?}`. `day_of_month` is `"1"`-`"28"` or `"last"` (never 29-31, so a fixed day never drifts across months of different lengths); `starts_at` defaults to now. |
| `PATCH` | `/recurring-rules/{id}` | Edit a rule / deactivate it (`{active: false}`) — any edit affects future generations only, movements already generated are never touched. Any subset of `{description, category, payment_method, account_id, amount, currency, day_of_month, ends_at, active}`; `account_id: ""` clears it. There's no way to clear an already-set `ends_at` back to "no end date" through this endpoint. |
| `GET` | `/settings/local-archive?user_id=` | The user's `local_archive_enabled` toggle (BACK-15's "no cloud" tier; defaults to `false`). |
| `PUT` | `/settings/local-archive` | Set the toggle: `{local_archive_enabled, user_id?}`. Independent of any cloud-storage setting — never deletes or stops writing anything server-side by itself. |
| `GET` | `/export/archive?user_id=` | The user's full restorable state — accounts, movements, credit-card purchases — as plaintext JSON. The frontend's "Local backup" panel encrypts this client-side (AES-256-GCM, PBKDF2-SHA256-derived key) before it's ever saved to a file; this endpoint itself has no encryption of its own. |
| `POST` | `/import/archive` | Restore a (frontend-decrypted) archive in the same shape `GET /export/archive` returns. Idempotent by row ID — a row that already exists is skipped, never overwritten; safe to import the same archive more than once. `cancels_movement_id`/`reversed_by_movement_id` are not restored (see Known limitations). Returns counts restored/skipped per collection. |
| `POST` | `/plans` | Create a plan (BACK-10). Body: `{name, plan_type, currency, monthly_target_amount, target_amount?, account_id?, start_date?, end_date?}`. `plan_type` is `stress_test` or `savings`. A savings plan requires `target_amount` and `account_id` (the account's currency must match `currency`); a stress-test plan rejects both. `start_date` defaults to now. |
| `GET` | `/plans` | List every plan the caller owns, each with lightweight progress: a savings plan's all-time contribution total, or a stress-test plan's current (not projected) month-to-date surplus/deficit. |
| `GET` | `/plans/{id}` | One plan's full progress plus the pace checker computed on read: `on_track` and, for a savings plan, `projected_shortfall` (linear month-end projection vs. `monthly_target_amount`). Never changes `status` as a side effect. |
| `PATCH` | `/plans/{id}` | Edit a plan. Body: any subset of `{name, target_amount, monthly_target_amount, end_date, status}`. `target_amount` is only editable on a savings plan. `status` (`active`/`completed`/`abandoned`) is the only way a plan leaves `active` — a `GET` never does this itself. |

`amount` is an integer in the smallest currency unit (cents), negative for
expenses, positive for income, and cannot be zero. Splitting an amount too
small for its installment count (would create zero-cent installments) is
rejected.

## Running locally

### Whole stack via podman/docker compose (recommended)

Requires `../ledger-service` to exist as a sibling checkout — `docker-compose.yml`
builds it straight from that source rather than duplicating its Dockerfile.

```bash
make up         # builds and starts postgres, ledger-service, financial-tracker api, web
make logs       # follow logs
make down       # stop and remove everything (data volumes survive)
make restart    # down + up
make rebuild    # down + build images + up — REQUIRED after changing Go code (see contributing/bug-fix.md)
make remove-db  # wipe ALL databases (tracker SQLite + ledger postgres) for a fresh start
make ps         # see what's running
```

A copy of these targets also lives in the parent directory's `Makefile`
(one level up), delegating here — so `make up` works from either place.

This brings up:
- `postgres` + `ledger-service` on `:8080` (ledger-service's own DB)
- `financial-tracker` API on `:8081`, its SQLite file on the
  `financial_tracker_data` volume
- `web` (SvelteKit dev server, hot-reloading against the bind-mounted `web/` dir) on `:5173`

financial-tracker no longer depends on ledger-service to start — stop the
`ledger-service` container and movements keep working; they catch up via
the background sync (or the UI's "Sync now" button) once it's back.

Run `make help` for the full target list. Note for Podman on SELinux
(Fedora/RHEL): the `web` bind mount needs the `:z` relabel flag, which is
already set in `docker-compose.yml` — without it `npm install` fails with
`EACCES` writing into the mounted directory.

### Running pieces individually (no containers)

1. **financial-tracker API** (works with or without ledger-service up):
   ```bash
   cp .env.example .env   # adjust if needed
   make run                # or: go run ./internal/cmd/api
   ```
   Listens on `:8081`, stores data at `DB_PATH` (default
   `./data/financial-tracker.db`), syncs to `LEDGER_SERVICE_URL`
   (default `http://localhost:8080`) every `SYNC_INTERVAL` (default 30s),
   and generates due recurring-rule movements every `RECURRING_INTERVAL`
   (default 1h).

   Set `DB_DRIVER=postgres` and `DATABASE_URL=postgres://...` to run against
   Postgres instead — `DB_PATH` is then ignored. Both drivers apply their
   own embedded migrations on startup and implement the same repository
   contracts, so usecases/handlers behave identically either way.
2. **ledger-service** (separate repo, has its own compose file) — optional
   at runtime:
   ```bash
   cd ../ledger-service
   podman-compose up -d --build   # or docker-compose
   ```
3. **Svelte frontend** (`web/`) — needs Node.js/npm on the host:
   ```bash
   cd web
   cp .env.example .env   # PUBLIC_API_URL, defaults to http://localhost:8081
   npm install
   npm run dev
   ```
   Opens on `:5173`. CORS is wide open in `internal/interfaces/api/router.go` for
   local dev — tighten before deploying anywhere real.

   `PUBLIC_OIDC_ISSUER`/`PUBLIC_OIDC_CLIENT_ID` (`web/.env.example`) are
   only needed when the API enforces auth (the default — see
   `AUTH_DISABLED` in `.env.example`); leave them blank when running with
   `AUTH_DISABLED=true`.

### Deploying (PostgreSQL, production images)

The stack above is dev-only (SQLite, Svelte dev server). For a deployable
Podman stack on PostgreSQL with production builds, see
[`deploy/README.md`](deploy/README.md).

## Testing

```bash
go build ./...
go vet ./...
go test ./...
```

Automated tests cover the trickiest correctness points: cancel semantics
(void vs reversal, double-cancel conflicts, reversal-of-reversal
rejection), installment split math (signed amounts, remainder cents,
too-small totals), balance calculation with cancelled movements, the sync
pass (success/failure recording, retry cooldown vs manual sync), per-user
sync toggles (two users, one disabled — the loop excludes only theirs,
at the repository-query level; the full disable → create → enable →
"local" rows reclassify to exactly the right backlog cycle), and the
SQLite repositories (including the atomic reversal link). The Postgres
repositories in `internal/infrastructure/postgresql` mirror the same test suite but
only run against a real database, guarded by `TEST_DATABASE_URL` — unset,
they're skipped so `go test ./...` still passes offline:

```bash
TEST_DATABASE_URL="postgres://user:password@localhost:5432/financial_tracker_test?sslmode=disable" go test ./internal/infrastructure/postgresql/...
```

Manually smoke-tested end-to-end: movements created/listed/cancelled with
ledger-service **down**, then a `POST /sync` after bringing it up pushed
everything (installments only once due), reversals included, with the
balance netting to zero after a full purchase cancel.

## Roadmap

- Idempotency keys for sync pushes (needs a ledger-service API change).
- Server-side JWT verification (the frontend's OIDC login flow already
  exists; the API doesn't check tokens yet — see the MVP limitations note
  above) instead of `DEFAULT_USER_ID`.
- Installment dates aligned to a card's real statement/closing day.
- Backfill script importing pre-SQLite history from ledger-service.
