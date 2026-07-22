# Plan: local database, payment methods (with credit-card installments), categories, and cancellable movements

## Goal

financial-tracker currently has no database of its own — every read and
write is a synchronous, blocking HTTP call to ledger-service
(`infrastructure/ledgerservice` implements `domain/repositories.MovementRepository`
directly against ledger-service's API). This plan:

1. Gives financial-tracker its own SQLite database and makes it the
   **source of truth**, with ledger-service becoming a secondary system
   that gets synced to (instead of being called synchronously on every
   request).
2. Adds **payment method** to movements, including **credit-card purchases
   split into installments**.
3. Adds **description** (free text) and **category** (fixed predefined
   list) to movements.
4. Adds **cancel** for movements. Since ledger-service is immutable
   (`requirements.md`: "No deletion — corrections via new compensating
   transactions"), cancelling a movement that already reached
   ledger-service creates a reversal transaction there; cancelling a
   movement that never reached ledger-service just voids it locally.
5. Makes ledger-service **optional at request time**: writes never block
   on it. A background sync process (plus a manual trigger) pushes
   pending movements to ledger-service whenever it's reachable.

This also happens to complete two items already on the README roadmap
("Own Postgres-backed MovementRepository" and "Categories/descriptions
once there's a local DB") — just with SQLite instead of Postgres, since
financial-tracker's write volume is a single user's personal movements,
not a shared ledger.

## Non-goals (for this pass)

- Real auth / multi-user (`DEFAULT_USER_ID` stays as-is).
- Aligning installment due dates to a real card's statement/closing day —
  installments are spaced by a fixed interval (see below); real
  billing-cycle alignment is future work.
- Idempotency keys on ledger-service's API (see "Known limitation" below).
- Multi-currency credit card purchases (a purchase has one currency).

## Architecture change

**Before:** `MovementHandler → usecases → MovementRepository (ledgerservice HTTP client)`.
Every request is only as available as ledger-service.

**After:** `MovementHandler → usecases → MovementRepository (SQLite)`, plus
a separate `SyncService` that talks to ledger-service in the background.

```
                    ┌─────────────────────┐
POST/GET /movements │  usecases            │
   ───────────────► │  (unchanged shape)   │
                    └──────────┬───────────┘
                               │ domain/repositories.MovementRepository
                               ▼
                    ┌─────────────────────┐
                    │ infrastructure/sqlite│  <- NEW, source of truth
                    │  (financial-tracker  │
                    │   .db)               │
                    └──────────┬───────────┘
                               │ read pending/failed rows
                               ▼
                    ┌─────────────────────┐
                    │ SyncService          │  <- NEW
                    │ (background ticker + │
                    │  POST /sync trigger) │
                    └──────────┬───────────┘
                               │ infrastructure/ledgerservice.Client (unchanged)
                               ▼
                        ledger-service (may be down — that's fine)
```

`infrastructure/ledgerservice` (the HTTP client and wire structs) stays
exactly as it is today — it's just called by the new `SyncService` instead
of being the `MovementRepository` itself.

## Data model (SQLite)

### `movements` table

| Column | Type | Notes |
|---|---|---|
| `id` | TEXT PK | UUID, generated locally on insert |
| `user_id` | TEXT | unchanged |
| `amount` | INTEGER | unchanged (smallest unit, signed) |
| `currency` | TEXT | unchanged |
| `description` | TEXT, nullable | free text |
| `category` | TEXT | one of the fixed categories below |
| `payment_method` | TEXT | `cash`, `debit_card`, `credit_card`, `pix`, `bank_transfer`, `other` |
| `credit_card_purchase_id` | TEXT, nullable, FK | set only when `payment_method='credit_card'` and the purchase was split into installments |
| `installment_number` | INTEGER, nullable | 1-based; set alongside `credit_card_purchase_id` |
| `status` | TEXT | `active` \| `voided` (see cancel semantics below) |
| `cancels_movement_id` | TEXT, nullable, FK → movements.id | set on a reversal row, points at the original |
| `reversed_by_movement_id` | TEXT, nullable, FK → movements.id | set on the original once a reversal exists |
| `timestamp` | TIMESTAMP | movement's effective date (installments are future-dated) |
| `sync_status` | TEXT | `pending` \| `synced` \| `failed` |
| `ledger_transaction_id` | TEXT, nullable | the id ledger-service assigned once synced |
| `sync_attempts` | INTEGER, default 0 | |
| `last_sync_error` | TEXT, nullable | |
| `last_sync_attempt_at` | TIMESTAMP, nullable | drives the retry cooldown (skip rows attempted too recently) |
| `synced_at` | TIMESTAMP, nullable | set when `sync_status` becomes `synced` |
| `created_at` | TIMESTAMP | row creation time (may differ from `timestamp` for future installments) |

### `credit_card_purchases` table

| Column | Type | Notes |
|---|---|---|
| `id` | TEXT PK | UUID |
| `user_id` | TEXT | |
| `description` | TEXT | |
| `category` | TEXT | |
| `total_amount` | INTEGER | signed, smallest unit |
| `currency` | TEXT | |
| `installment_count` | INTEGER | |
| `purchase_date` | TIMESTAMP | |
| `status` | TEXT | `active` \| `cancelled` (all-installments-cancelled marker) |
| `created_at` | TIMESTAMP | |

A purchase is a grouping record only; the actual money movements are the
rows in `movements` with `credit_card_purchase_id` set.

### Fixed category list

Predefined, enforced at the application layer (and via a `CHECK` constraint
in SQLite):

`food`, `transport`, `housing`, `utilities`, `health`, `entertainment`,
`shopping`, `education`, `income`, `transfer`, `other`

(Exact names are easy to bikeshed later — flag any you want changed and
I'll adjust before implementing. A `GET /categories` endpoint exposes this
list so the frontend never hardcodes it.)

## Credit card installments

`POST /movements` gains optional fields: `payment_method`,
`description`, `category`, and — only relevant when
`payment_method="credit_card"` — `installments` (int, default 1).

- **`installments <= 1`:** behaves exactly like today — one `movements`
  row, `payment_method="credit_card"`, no purchase record.
- **`installments > 1`:** creates one `credit_card_purchases` row plus
  `installments` rows in `movements`:
  - `amount` = `total_amount / installments`, with the rounding remainder
    (cents) added to the **last** installment so they always sum exactly
    to `total_amount`. If the per-installment base amount rounds to zero
    (e.g. 5 cents split 12 ways), reject with 400 — ledger-service
    rejects zero-amount transactions, so such rows could never sync.
  - `timestamp` for installment *i* = `purchase_date + i months` (i = 0..n-1),
    i.e. one per month starting the purchase month. This is a
    simplification — it doesn't know the card's real closing/due day.
  - All installments start `status='active'`, `sync_status='pending'`.

**Installments don't sync early.** The sync worker only picks up rows
where `timestamp <= now()` (see Sync mechanism below), so a purchase made
today with 12 installments only pushes installment #1 to ledger-service
now; installments #2–12 wait until their own due date arrives, then get
picked up automatically on the next sync pass. No separate "scheduled"
state is needed — the existing `pending` status plus a date filter handles
it.

**Cancelling a whole purchase** (`POST /credit-card-purchases/{id}/cancel`):
loops over its installments and cancels each one individually with the
same rule as a single movement (see below) — due/synced installments get
a reversal, not-yet-due ones just get voided. The purchase row is marked
`cancelled` once all its installments are non-active.

## Cancel semantics

`POST /movements/{id}/cancel`:

1. Not found → 404.
2. The movement is itself a reversal (`cancels_movement_id` set) → 400.
   Cancelling a reversal would spawn reversal-of-reversal chains; if the
   cancel was a mistake, re-create the movement instead.
3. Already `voided`, or already has `reversed_by_movement_id` set →
   409 "already cancelled".
4. **Never synced** (`sync_status` is `pending` or `failed`) → set
   `status='voided'` locally. Nothing to undo in ledger-service since it
   never got there; the sync worker's query excludes non-`active` rows so
   it will never try to push a voided movement.
5. **Already synced** (`sync_status='synced'`) → create a new reversal
   row: `amount = -original.amount`, same `user_id`/`currency`, category
   and payment method copied from the original (but **not**
   `credit_card_purchase_id`/`installment_number` — reversals are never
   part of the purchase's installment set, so purchase-level cancel
   doesn't try to cancel them), `description` = `"Reversal of
   {original.id}"`, `cancels_movement_id =
   original.id`, `status='active'`, `sync_status='pending'`. Save it
   locally (always succeeds), set `original.reversed_by_movement_id =
   reversal.id`, and attempt a best-effort immediate push of the reversal
   to ledger-service with a short timeout. If that push fails or times
   out, the reversal just sits as `pending` and the background sync
   worker retries it later — cancelling never blocks on ledger-service
   being up.

Note the **original synced movement's `status` stays `active`** even
after cancellation — it's immutable once it reached ledger-service,
matching ledger-service's own no-delete philosophy. "Is this cancelled?"
is answered by `reversed_by_movement_id IS NOT NULL`, not by `status`.

### Why balance calculation doesn't need to change

`ListMovementsUseCase` today sums `amount` over the returned movements.
The only change needed: **exclude `status='voided'` rows** from that sum
(and, if it's cleaner, exclude them from the list response too, or return
them with a distinct badge — your call). Everything else nets out for
free:

- A synced-then-cancelled original + its reversal are both `active` rows
  with opposite amounts → they already sum to zero, exactly like
  ledger-service's own balance would compute it. No special-casing.
- A never-synced-then-cancelled movement is `voided` → excluded, as if it
  never happened, on both sides (it never reached ledger-service either).

## Sync mechanism

- `SyncService` (new, `application/sync/service.go`): given a
  `MovementRepository` and a `LedgerGateway` **port** (an interface
  defined in `application/sync`, implemented by
  `infrastructure/ledgerservice` wrapping its existing `Client` —
  application code must not import infrastructure, per the dependency
  rule), runs one "sync pass":
  1. Query local movements where `status='active' AND sync_status IN
     ('pending','failed') AND timestamp <= now() ORDER BY timestamp ASC`.
  2. For each, call `client.CreateTransaction` (existing method, unchanged).
  3. On success: `sync_status='synced'`, store `ledger_transaction_id`,
     `synced_at`.
  4. On failure: `sync_status='failed'`, increment `sync_attempts`, store
     `last_sync_error`. (Simple cap + cooldown between attempts, e.g. skip
     rows synced-attempted in the last N seconds, to avoid hammering a
     down ledger-service every tick.)
- **Background trigger:** a ticker in `cmd/api/main.go` (e.g. every 30s)
  runs a sync pass in a goroutine.
- **Manual trigger:** `POST /sync` runs one pass synchronously and
  returns a summary (`{"synced": N, "failed": N}`) — useful for a "sync
  now" button in the UI or for tests. The manual pass ignores the retry
  cooldown (the user explicitly asked for a sync now); only the
  background ticker skips recently-attempted rows.
- `GET /movements` response includes `sync_status` (and
  `ledger_transaction_id` when present) so the frontend can show a
  "pending sync" indicator per movement.

### Known limitation: no idempotency key

If a sync attempt to ledger-service succeeds but the response is lost
(network blip after the write, before financial-tracker records
`ledger_transaction_id`), a retry would create a **duplicate** transaction
in ledger-service. ledger-service's API has no idempotency-key support
today. Given the low request volume this is an acceptable initial risk,
but flagging it — the real fix is adding a client-generated idempotency
key to ledger-service's `POST /transactions`, which is a change to that
service, not this one. Worth a follow-up ticket there.

## Changes by layer

- **`domain/entities`**: extend `Movement` with `Description`,
  `Category`, `PaymentMethod`, `Status`, `SyncStatus`,
  `LedgerTransactionID`, `CreditCardPurchaseID`, `InstallmentNumber`,
  `CancelsMovementID`, `ReversedByMovementID`. New `CreditCardPurchase`
  entity.
- **`domain/repositories`**: `MovementRepository` gains
  `ListPendingSync(ctx) ([]*Movement, error)`, `UpdateSyncResult(ctx, id,
  result) error`, `Void(ctx, id) error`, `CreateReversal(ctx, original,
  reversal) error` (atomic: insert reversal + link original in one
  transaction). New `CreditCardPurchaseRepository`.
- **`application/usecases`**: extend `CreateMovementUseCase` (validate
  category/payment_method enums); new `CreateCreditCardPurchaseUseCase`,
  `CancelMovementUseCase`, `CancelCreditCardPurchaseUseCase`,
  `SyncMovementsUseCase`. `ListMovementsUseCase` balance calc excludes
  `voided` rows.
- **`application/sync`** (new package): `SyncService` described above.
- **`infrastructure/sqlite`** (new package): implements
  `MovementRepository` and `CreditCardPurchaseRepository` against a local
  `database/sql` connection. Recommend `modernc.org/sqlite` (pure Go, no
  cgo) over `mattn/go-sqlite3` — keeps the existing multi-stage Dockerfile
  simple with no C toolchain in the build image.
- **`infrastructure/ledgerservice`**: `Client` unchanged; new small
  `gateway.go` adapts it to `application/sync`'s `LedgerGateway` port.
  The current `movement_repository.go` in this package goes away
  (superseded by `infrastructure/sqlite`).
- **`migrations/`** (new, financial-tracker's own, separate from
  ledger-service's): `001_create_movements_table.sql`,
  `002_create_credit_card_purchases_table.sql`, exposed as an embedded
  `embed.FS` (tiny `migrations.go`) so the binary carries its own schema
  — no runtime file-path dependency in the container.
- **`interfaces/dto`**: extend `CreateMovementRequest`/`MovementResponse`
  with the new fields; new `CreateCreditCardPurchaseRequest`,
  `CancelMovementResponse`, `SyncSummaryResponse`, `CategoryListResponse`.
- **`interfaces/api`**: new routes:
  - `POST /movements/{id}/cancel`
  - `POST /credit-card-purchases` (only reached when `installments > 1`;
    or fold into `POST /movements` — pick one, see open question below)
  - `POST /credit-card-purchases/{id}/cancel`
  - `POST /sync`
  - `GET /categories`
- **`cmd/api/main.go`**: wire `infrastructure/sqlite` in place of
  `infrastructure/ledgerservice.NewMovementRepository`; start the
  `ledgerservice.NewClient` + `SyncService` background ticker; add
  `DB_PATH` env var (default e.g. `./data/financial-tracker.db`).
- **`docker-compose.yml`**: add a volume for financial-tracker's SQLite
  file (parallel to the existing `postgres_data` volume for
  ledger-service); `ledger-service` becomes non-blocking for
  financial-tracker to start (drop or soften the `depends_on`, since
  financial-tracker must work even if ledger-service is down).
- **`Dockerfile`**: `COPY go.mod go.sum ./` — the project now has real
  dependencies (the SQLite driver), so `go.sum` exists and the build
  needs it. Handler error mapping also gains a distinction: `ErrUpstream`
  still maps to 502, but unknown errors now map to 500 (they come from
  the local DB, not an upstream).
- **`web/`**: frontend changes (payment method / installments / category
  picker, cancel button, sync status badge) are out of scope for this
  plan doc — follow-up once the API is in place.

## Resolved decisions (were open questions)

1. **Route shape for installment purchases** — single endpoint:
   `POST /movements` with an optional `installments` field; the server
   creates the purchase + N installment rows when
   `payment_method="credit_card"` and `installments > 1` (and rejects
   `installments > 1` with any other payment method). No separate
   `POST /credit-card-purchases` create route; only the purchase-level
   cancel route exists.
2. **Category names** — the eleven listed above, as-is.
3. **Sync tuning** — background tick 30s, retry cooldown 60s, both
   overridable via env (`SYNC_INTERVAL`, `SYNC_RETRY_COOLDOWN`, Go
   duration strings).

## Rollout order

1. SQLite migrations + `infrastructure/sqlite` `MovementRepository`
   (feature-flagged in behind the existing interface — no API change
   yet, just proves the new repo works).
2. Swap `cmd/api/main.go` to use it as the primary repository; drop
   ledger-service from the request path.
3. Add `application/sync` + `POST /sync` + background ticker; verify
   movements created while ledger-service is stopped still work, and
   backfill once it's back up.
4. Add description/category/payment_method fields end-to-end (DTO →
   usecase → repo).
5. Add credit-card installments (purchase table + split logic).
6. Add cancel (single movement, then purchase-level cancel).
7. Update README (architecture section, roadmap, API table) to match.
