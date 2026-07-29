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

Movements carry a payment method — a user-extendable, per-user registry
(BACK-17; see below), not a fixed list — a free-text description, and a
category from the shared, globally-visible registry (`GET /categories`)
— see "Categories & avoidability" below. Credit-card purchases can be
split into monthly installments; installments only sync to
ledger-service once their date arrives. Movements can be cancelled: one
that never reached ledger-service
is just voided locally, while one that already synced gets a compensating
reversal movement (ledger-service never deletes — corrections are new
transactions), which the sync then pushes.

Beyond movements, the tracker knows about:

- **Currencies** — a user-extendable registry (`usd`/`brl` seeded, add
  `btc` or anything else via `POST /currencies`) backing the frontend
  dropdown. Movements store the code as plain text.
- **Payment methods** (BACK-17) — a user-extendable registry, same shape
  as currencies, replacing the old hardcoded enum (which baked in `pix`,
  Brazil's instant-payment rail, as if it applied everywhere). A fresh
  user gets `cash`, `debit_card`, `credit_card`, `bank_transfer`, `other`
  seeded automatically — never `pix`; a user who wants it (or any other
  country's rail) adds it themselves via `POST /payment-methods`, exactly
  like adding a currency code. `credit_card` and `bank_transfer` are
  system entries (installment validation and transfer legs both branch on
  them by name) and can't be renamed or deleted. A brand-new name used on
  a movement is implicitly registered at no extra cost — no separate
  create call required. Returned as part of `GET /categories`'s
  `payment_methods` field, now `{id, name}` rows instead of plain
  strings.
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
  a float. Backing the purchasing-power report below; posting the same
  currency + effective date again backfills/corrects that row instead of
  duplicating it.
- **Purchasing power** (`GET /reports/purchasing-power?months=`) — per
  calendar month, per native currency (never summed together): spending
  by category (each tagged with that category's `avoidability_percent`,
  BACK-14), income, total expenses, `potential_savings` (the
  avoidability-weighted counterfactual — Σ `expense × avoidability% /
  100`), and profit. Every one of those figures also gets a USD view,
  each movement converted at the BACK-11 rate effective *at its own
  timestamp* — this is what makes a currency's devaluation visible: the
  native profit stays identical across months, but the USD profit drops.
  `profit_usd_at_current_rate` additionally converts the whole month at
  *today's* rate, so the response shows both "what it was worth then"
  and "what it's worth now." Transfers are excluded (so are contributions
  to investment accounts, since those are transfers too) and cancelled
  movements never count; a currency missing a rate for some date marks
  that month's USD figures `usd_incomplete: true` rather than guessing —
  native figures are always complete regardless.
- **CSV history import** (BACK-03) — users export/scan their bank
  statements, hand them to any AI along with `GET /import/movements/spec`'s
  published model, and upload the result via `POST /import/movements` to
  backfill history. Every row is validated before anything writes
  (strict by default: any bad row imports nothing), and rows matching an
  existing movement or another row in the same file are flagged as
  duplicates rather than silently re-imported.

Backend layout follows Clean Architecture (see `CleanExampleGo` for the
reference pattern this was modeled on): the **domain** layer holds pure
entities only, and the **application** layer owns every contract —
repository interfaces, service ports, and use-case interfaces:

```
domain/entities              Movement, CreditCardPurchase, Account (+snapshots),
                             fixed Category/AccountType lists (payment methods are a
                             per-user registry instead, see application/repositories);
                             single-entity rules live here too (e.g. Account.Send()/
                             Receive() for transfers)
application/dto              MovementDTO, AccountDTO, CreditCardPurchaseDTO, ExchangeRateDTO
                             — what repositories/services/usecases actually pass to each
                             other, converted from domain entities at the infrastructure
                             boundary
application/repositories     MovementRepository, CreditCardPurchaseRepository,
                             AccountRepository, CardRepository, CurrencyRepository,
                             ExchangeRateRepository, CategoryRepository
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
                             DeleteExchangeRate, ToUSD, CreateCard, ListCards, GetCard,
                             UpdateCard, DeleteCard, CreateCategory, ListCategories,
                             UpdateCategory, DeleteCategory, GetPurchasingPower
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
- **Identity provider is pluggable (BACK-20)**: `cmd/api` selects which
  `services.IdentityVerifier` to construct via `AUTH_PROVIDER`
  (`authentik`, the default, or `simple` —
  `internal/infrastructure/simpleauth`, any other issuer speaking the same
  OIDC-like `iss`/`sub`/`exp`/`aud` + JWKS contract, config'd via
  `SIMPLE_AUTH_ISSUER_URL`/`SIMPLE_AUTH_JWKS_URL`/`SIMPLE_AUTH_AUDIENCE` —
  see `.env.example`). Only `cmd/api/main.go` knows which provider is
  active; `interfaces/api` and every usecase depend solely on the
  `services.IdentityVerifier` interface. **Not included**: an actual
  standalone username/password auth service (BACK-20's other deliverable,
  a new sibling repo) — this only ships the financial-tracker-side adapter
  ready to point at one once it exists.
- **No idempotency key on sync.** If a push to ledger-service succeeds but
  the response is lost, the retry duplicates the transaction there. The
  real fix is idempotency-key support in ledger-service's API (follow-up
  in that repo).
- **Installment dates are simplified when no card is linked**: one per
  month from the purchase date. Linking a `card_id` (`POST /movements`,
  `POST /cards`) dates each installment on that card's real closing/due
  day instead — see "Cards" below.
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
backfill. `cloud_storage_entitled` is stored and exposed the same way;
BACK-19 (below) is what actually drives it now that there's a paid tier
to sell.

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

## Cards (`cards` table)

A card profile (`GET/POST/PATCH/DELETE /cards`) records a credit card's
**closing day** (statement cut) and **due day** (when an installment
actually hits your money) — `"1"`-`"28"` or `"last"`, same convention as
recurring rules. Linking a purchase to a card (`card_id` on `POST
/movements` or a credit-card `POST /movements` with `installments`)
dates each installment on the card's real due days instead of a flat
monthly offset from the purchase date.

Each card's `GET`/`GET /cards/{id}` response carries a computed picture:

- **`next_due_total` / `next_due_date`** — the statement that has
  already closed and is waiting to be paid, net of payments already
  recorded against the card (see below). This is "how much to pay right
  now."
- **`open_cycle_total`** — purchases made after the last closing day,
  still accumulating toward the *next* statement — not due yet.
- **`available_credit`** (only when `credit_limit` is set) —
  `credit_limit` minus everything outstanding (`next_due_total +
  open_cycle_total`).
- **`budget_remaining` / `over_budget`** (only when `monthly_budget` is
  set) — `monthly_budget` minus `open_cycle_total`, the user's own
  spending goal for the cycle, independent of `credit_limit`.

**Paying the card bill is not a second expense.** Recording a payment is
an ordinary `POST /movements` on the paying account (`category:
"transfer"`, a negative `amount`) with `card_payment_for_card_id` set —
it reduces `next_due_total` and affects the paying account's balance
like any movement, but (like every `transfer`) never shows up as a
second expense in `GET /cashflow`'s category totals; the original
purchases already counted once, at purchase time.

A known v1 simplification: payments aren't tied to a specific statement
(there's no separate invoice/statement entity) — every payment ever
recorded against a card nets against whichever `next_due_total` is
current, rather than being scoped to the exact statement it was
intended for. Revisit only if it turns out to matter in practice.

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

## At-rest encryption & pseudonymous ledger sync (BACK-16)

The `cloud_storage_enabled`/`ledger_sync_enabled` toggles above are backed
by real cryptography, not just booleans:

- **Field-level encryption** (Postgres backend only — `DB_DRIVER=postgres`):
  `movements.description` and `accounts.name` are encrypted at rest with
  AES-256-GCM under a per-user data key. Each user's data key is generated
  once, then wrapped (also AES-256-GCM) under a single server-held master
  key (`ENCRYPTION_MASTER_KEY`, env/secrets-manager only, never exposed
  over the API). Amounts, currency, category, dates, and account/user ids
  stay plaintext — every existing balance/cashflow/purchasing-power
  calculation needs them server-side, and encrypting them away would
  break those, not make them "more private." SQLite (standalone/dev)
  never encrypts these fields; there's no stolen-disk threat model
  distinct from the user's own machine to protect against there.
- **Pseudonymous ledger sync** (both backends): when `ledger_sync_enabled`
  is on, `infrastructure/ledgerservice.gateway.Publish` sends
  ledger-service a random, non-reversible per-user pseudonym UUID instead
  of the real user id, and a deterministic HMAC-SHA256 token
  (`LEDGER_HMAC_KEY`) instead of the plain currency code — e.g.
  "pseudonym `f47ac10b-...` received 10 of token `c_9b2f...`" instead of
  "user `alice` received 10 USD." The same (user, currency) pair always
  tokenizes identically, so ledger-service's own consistency checks still
  work. Amounts are never hidden or tokenized — the ledger's validator
  and auditability require real numeric values; only *who* and *what
  currency* are pseudonymized. Movements already synced under the real
  user id before this landed stay as-is in ledger-service's append-only
  log — pseudonymization applies to sync going forward only.

**What this tier does and doesn't protect against:**
- **Protects against:** a stolen disk/backup, a leaked DB dump, a
  compromised read-replica — anyone with raw bytes but not the running,
  authenticated application.
- **Does not protect against:** the operator of financial-tracker itself.
  The server must decrypt free-text fields to serve them back to their
  owner, and must have plaintext amounts/currency/category/dates to
  compute anything server-side. True zero-knowledge cloud storage is a
  contradiction — a server that computes your balance can read your
  amounts. A user who wants that guarantee wants BACK-09 (standalone) or
  BACK-15 (local archive), not this tier.

Key rotation is out of scope for v1 — `ENCRYPTION_MASTER_KEY`/
`LEDGER_HMAC_KEY` loss makes every already-encrypted field/pseudonym
permanently unrecoverable, so back them up the same way you'd back up a
database password (see `deploy/.env.example`).

## Paid cloud-storage subscription (BACK-19)

The app is fully usable for free, forever, with your data kept only in a
local, password-encrypted archive you manage yourself (BACK-15) — the
server never holds a durable copy of a free-tier user's data beyond what
the local database already has. Paying (~10 USD/year, an anchor price,
not final) doesn't unlock features; it unlocks *the operator taking on
the responsibility of not losing your data*: a durable, at-rest-encrypted
(BACK-16) copy in Postgres, usable from more than one device, that you
don't have to remember to back up yourself. This is strictly less
private than the free tier, not more — see BACK-16's "what this tier
does and doesn't protect against" above.

- **`subscriptions` table**: one row per user —
  `{user_id, provider, provider_subscription_id, status, current_period_end,
  created_at, updated_at}`. `status` is `active` | `past_due` | `canceled`.
  This is the payment provider's view of billing state; the entitlement
  the rest of the app actually reads is still `user_settings.cloud_storage_entitled`.
- **`POST /billing/webhook`** — provider-signed, unauthenticated by user
  token (the payment provider has no financial-tracker session). Body is
  a provider-agnostic shape (`{user_id, provider, provider_subscription_id,
  status, current_period_end}`); a real Stripe (or other) integration
  translates its own event payload into this shape before calling in.
  Authenticity comes from an `X-Billing-Signature` header — HMAC-SHA256
  over the raw request body, keyed by `BILLING_WEBHOOK_SECRET` — checked
  before the body is even JSON-decoded.
- **Entitlement only flips *true* immediately** (on `active`): gaining
  access should never wait. Losing it — from either `past_due` or an
  explicit `canceled` — never flips immediately, even from the webhook
  that reports it. **A grace period (`BILLING_GRACE_PERIOD_DAYS`,
  default 7) applies to both**: "a late card shouldn't cut off access
  instantly," and cancelling gets the same leniency rather than an
  asymmetric instant cutoff. A background sweep
  (`internal/application/billing`, interval `BILLING_SWEEP_INTERVAL`,
  default 1h) flips `cloud_storage_entitled` to `false` once
  `current_period_end + grace period` has passed for a still-`past_due`
  or still-`canceled` subscription.
- **Grandfathering**: a user who already existed before this shipped
  keeps `cloud_storage_entitled = true` regardless of subscription
  status — the existing "absence of a row means true" default (above)
  already covers them; nobody already using the product loses access
  silently because billing shipped. Only a genuinely new signup (first
  time `EnsureUser` ever sees that user id) gets an explicit
  `cloud_storage_entitled = false` row, overriding that default.
- **Non-destructive lapse**: losing entitlement never deletes hosted data
  on the spot — nothing in this codebase purges data on an entitlement
  change. Hosted data stays exportable (`GET /export/archive`) through a
  documented 30-day retention window after entitlement lapses; an actual
  automated purge past that window is not implemented (would be its own
  future ticket) — the window today is a policy statement, not enforced
  by a deletion job.
- **`GET /billing/plan?currency=`** — the annual price
  (`BILLING_REFERENCE_PRICE_USD_CENTS`, default 1000 = $10.00/year)
  converted to the requested currency using the caller's own BACK-11
  exchange rate, e.g. `{"currency": "brl", "amount": 5000}`. Falls back
  to `{"currency": "usd", "amount": <reference>}` when no rate is known
  for the requested currency — the response's own `currency` field always
  says what currency `amount` is actually in; never assume it echoes the
  request.
- **`GET /settings`** gains `subscription_status`/
  `subscription_current_period_end` (omitted entirely for a user who has
  never had a subscription row) alongside the existing entitlement
  booleans.
- **Ledger-audit-tier monetization is undecided**: whether
  `ledger_sync_enabled` (BACK-16's pseudonymous audit sync) is bundled
  into this same subscription, sold separately, or stays permanently free
  is not decided by this ticket — `ProcessBillingWebhookUseCase` only
  ever touches `cloud_storage_entitled` today.
## Categories & avoidability (`categories` table)

Categories are a **shared, globally-visible registry**
(`GET/POST/PATCH/DELETE /categories`), referenced everywhere by
`category_id` — not a fixed enum, and not per-user (BACK-14's original
per-user registry became shared in its own follow-up: "I will create
restaurant category with 80% and offer it for whoever wants to get it —
if someone doesn't agree they can just create a new one"). Two different
users independently creating "restaurant" each get their own row and
their own id; there's no name uniqueness or per-user ownership. Each
category carries an `avoidability_percent` (0-100): how easy that kind of
spend is to skip (a restaurant category might be 80% avoidable; groceries
might be 20%).

`category_maintainers` is the only place edit rights live: a category's
creator is its sole contributor at creation time (`is_contributor` on the
API response tells the frontend whether to show edit controls).
`PATCH /categories/{id}` (rename, change `avoidability_percent`) requires
the caller be a contributor; anyone may still use any category on their
own movements regardless of who created it. `transfer`, `income`, and
`other` are the three **system categories**, seeded once with fixed ids
(`entities.CategoryTransferID`/`CategoryIncomeID`/`CategoryOtherID`) so
every environment agrees on them — not spend, so `avoidability_percent`
stays `NULL`, and no one can create, rename, or remove them through the
API regardless of contributor status. A per-user cap
(`max_categories_per_user`, operator-configurable via the `limits` table,
default 10) limits how many categories one user may create/contribute to.

`DELETE /categories/{id}` doesn't delete the row (others may still be
using it) — it removes the category from the caller's own list
(`user_hidden_categories`); `?reassign_existing=true` additionally moves
the caller's own movements/purchases off it onto their resolved default
category first (`user_settings.default_category_id`, falling back to the
global `other` category), scoped strictly to the caller's own rows even
though the category itself is shared.

For a genuine one-off spend that doesn't deserve its own category (went
karting once — 100% avoidable, never happening again), movements carry
their own optional `avoidability_percent` override
(`POST`/`PATCH /movements`), independent of `category_id`. A movement's
**effective avoidability** — used by BACK-12's purchasing-power report —
resolves in this order: the movement's own override if set; else its
category's stored `avoidability_percent`; else no value (excluded from
any avoidability-weighted aggregate, same exclusion `transfer` already
gets from cashflow totals).

## API

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/import/movements/spec` | BACK-03: the CSV history-import model — columns (with real allowed values: registered currencies, categories, payment methods, the user's own account names) plus a ready-to-copy template. The frontend renders this instead of hardcoding the spec. |
| `POST` | `/import/movements?dry_run=&allow_partial=&skip_duplicates=` | Import a CSV backfill (multipart `file` field or a raw `text/csv` body; header `date,amount,currency,description,category,payment_method,account`, max 1 MiB / 10k rows). Validates every row first; `dry_run=true` reports without writing. Default **strict**: any row error imports nothing (`allow_partial=true` imports the valid rows and skips the rest). Rows matching an existing movement or an earlier row in the same file on `(date, amount, currency, normalized description)` are flagged in `duplicates[]` but still import by default — `skip_duplicates=true` excludes them. Response: `{imported, skipped, errors[], duplicates[]}`. |
| `GET` | `/export/movements?include_cancelled=` | BACK-09: the revert direction of `POST /import/movements` — streams the caller's history as CSV in exactly the import model above (see `internal/infrastructure/csv/README.md`), so a file round-tripped through import then export reproduces the same rows. Excludes voided movements and reversal pairs by default; `include_cancelled=true` includes everything with extra `status`/`cancels_movement_id`/`reversed_by_movement_id` columns. Available in every mode, not just standalone. |
| `GET` | `/config` | Unauthenticated. `{standalone, auth_enabled}` — what the frontend reads before deciding whether to show the login guard. |
| `GET` | `/settings?user_id=` | The caller's own settings — entitlement (operator-controlled, read-only here) and preference. Defaults to all-`true` if the user has never touched them (no row needed). |
| `PATCH` | `/settings?user_id=` | Body: `{ledger_sync_enabled}` — the only field a user may change. Any attempt to set `ledger_sync_entitled`/`cloud_storage_entitled` (or any other key) is rejected with 400. Re-enabling reclassifies movements created while sync was off (`sync_status: "local"`) back to `"pending"`, so the next `/sync` pass pushes exactly that backlog. |
| `POST` | `/movements` | Create a movement. Body: `{amount, currency?, user_id?, description?, category_id?, payment_method?, installments?, account_id?, card_id?, card_payment_for_card_id?, plan_id?, avoidability_percent?}`. With `payment_method="credit_card"` and `installments > 1`, splits into monthly installments and returns the purchase + its movements (no `account_id` allowed in that case); pass `card_id` there too to date them on the card's real due days. `card_id` (single-movement charges) requires `payment_method="credit_card"` and dates the movement on the card's due day instead of "now". `card_payment_for_card_id` marks a payment settling that card's statement — see "Cards" above. An `account_id`'s currency (and a `card_id`/`card_payment_for_card_id`'s) must match the movement's. `category_id` (BACK-14 follow-up) must reference an existing category — any category, since they're globally shared — or the request is rejected (400); omitted leaves the movement uncategorized. `avoidability_percent` (0-100) is this movement's own ad-hoc override, independent of `category_id` — see "Categories & avoidability" above. `plan_id` (BACK-10) tags the movement as funding a savings plan — the plan must exist, belong to the caller, be `active`, be a savings (not stress-test) plan, and share the movement's currency, or the request is rejected (400). |
| `GET` | `/movements?id={uuid}` | Fetch one movement. |
| `GET` | `/movements?user_id={uuid}&currency=&from=&to=&limit=&offset=` | List movements + computed `balance` (voided rows excluded from the balance). `from`/`to` take `YYYY-MM-DD` or RFC 3339 (`to` is inclusive when date-only). Each row carries `status` and `sync_status`. |
| `PATCH` | `/movements/{id}` | Edit one movement. Body: any subset of `{description, category_id, payment_method, account_id, plan_id, amount, currency, timestamp, avoidability_percent}`. `description`/`category_id`/`payment_method`/`account_id`/`plan_id`/`avoidability_percent` are local-only metadata and always editable (`account_id: ""`/`plan_id: ""`/`category_id: ""` clears it; a non-empty `category_id`/`plan_id` is validated the same way as on create). `amount`/`currency`/`timestamp` edit in place if the movement hasn't synced yet; on an already-synced movement they instead produce a reversal + a replacement (both returned, original left untouched). Rejects voided/reversed movements, reversals themselves, and financial edits on a single credit-card installment or transfer leg (409 in all cases). |
| `POST` | `/movements/{id}/cancel` | Cancel one movement (void or reversal — see semantics above). Returns the movement and, if created, the reversal. |
| `POST` | `/credit-card-purchases/{id}/cancel` | Cancel a whole installment purchase. Returns which installments were voided vs reversed. |
| `POST` | `/sync` | Run one sync pass against ledger-service now. Returns `{synced, failed}`. |
| `GET` | `/categories` | The shared category registry (`{id, name, avoidability_percent, is_contributor}[]`, `is_contributor` relative to the caller) plus the caller's own payment-method registry (BACK-17 — `payment_methods` is `[{id, name}]`, system/default entries lazily ensured first). |
| `POST` | `/categories` | Create a category. Body: `{name, avoidability_percent?}` (0-100; omitted defaults to 50). Caller becomes its sole contributor. 400 on reserved names (`transfer`/`income`/`other`) or once the caller has reached `max_categories_per_user`. |
| `PATCH` | `/categories/{id}` | Rename and/or change `avoidability_percent`. 400 outside 0-100, on a system category, or if the caller isn't a contributor. |
| `DELETE` | `/categories/{id}?reassign_existing=` | Remove the category from the caller's own list (the row itself isn't deleted — others may still use it). 400 on a system category. `reassign_existing=true` additionally moves the caller's own movements/purchases off it onto their resolved default category first. |
| `POST` | `/payment-methods` | Register a payment method: `{name}`. Reserved system names (`credit_card`, `bank_transfer`) and case-insensitive duplicates are rejected (409). |
| `PATCH` | `/payment-methods/{id}` | Rename a payment method: `{name}`. Rejected on system entries (400). |
| `DELETE` | `/payment-methods/{id}` | Remove a payment method. Rejected on system entries (400); one still referenced by movements is fine — it's a label, not an FK. |
| `GET` | `/cashflow?from=&to=&user_id=` | Money in / out / net over the interval, per currency (`totals`) and per account (`by_account`, unassigned movements in their own bucket). `from`/`to` required. Transfers are excluded. |
| `GET` | `/reports/purchasing-power?months=` | Per-month, per-currency spending/income/profit/`potential_savings`, each with a USD view — see "Purchasing power" above. `months` defaults to 6, clamped to 24. |
| `GET` | `/accounts` | All accounts with `estimated_balance`, latest `reported_balance`/`reported_at`, `movements_since_report` and `last_return` (+ the valid `account_types`). |
| `POST` | `/accounts` | Create an account. Body: `{name, type?, currency?, user_id?}`. Currency must be registered; duplicate names (case-insensitive) are rejected. |
| `POST` | `/accounts/{id}/balance` | Report the account's real balance: `{balance, timestamp?}` (smallest unit; `timestamp` omitted means now, an explicit one backfills a past report). Returns the updated account view, including the newly computed `last_return` when a previous report exists. |
| `GET` | `/accounts/{id}/balance` | The account's full reported-balance history, newest first, each entry paired with its own return (the balance change since the previous report, or since the account's implicit zero starting balance for the earliest one). |
| `GET` | `/cards` | All cards with their computed `next_due_total`/`next_due_date`/`open_cycle_total`/`available_credit`/`budget_remaining`/`over_budget` — see "Cards" above. |
| `POST` | `/cards` | Create a card. Body: `{name, last_four?, closing_day, due_day, credit_limit?, monthly_budget?, currency}`. `closing_day`/`due_day` are `"1"`-`"28"` or `"last"`. |
| `GET` | `/cards/{id}` | Fetch one card, same computed shape as `GET /cards`. |
| `PATCH` | `/cards/{id}` | Edit a card. Any subset of `{name, last_four, closing_day, due_day, credit_limit, monthly_budget}`. No way to clear an already-set `credit_limit`/`monthly_budget` back to "not tracked" through this endpoint. |
| `DELETE` | `/cards/{id}` | Remove a card. Rejected (409) if any movement or credit-card purchase still references it. |
| `GET` | `/currencies` | Registered currency codes. |
| `POST` | `/currencies` | Register a code: `{code}` (2–10 lowercase alphanumerics). Idempotent; returns the updated list. |
| `POST` | `/transfers` | Move money between two of the user's own accounts. Body: `{from_account_id, to_account_id, amount, description?, user_id?, timestamp?, plan_id?}` (`amount` positive). v1 requires both accounts to hold the same currency. Creates a linked debit (`-amount` on `from_account_id`) and credit (`+amount` on `to_account_id`) atomically, category `transfer`, sharing a `transfer_id`. `plan_id` (BACK-10), when set, tags only the credit (destination) leg — the recommended way to fund a savings plan without inflating income/expense cashflow. Returns `{transfer_id, debit, credit}`. |
| `POST` | `/transfers/{id}/cancel` | Cancel both legs of a transfer (`{id}` is the `transfer_id`). Each leg is voided or reversed independently based on its own `sync_status`, same as `/movements/{id}/cancel`. Returns `{debit, credit}`, each shaped like `POST /movements/{id}/cancel`'s response. |
| `GET` | `/exchange-rates?user_id=` | The user's exchange-rate history, grouped by currency (current rate + full history, newest `effective_from` first). |
| `POST` | `/exchange-rates` | Set/backfill a currency's rate against USD. Body: `{currency, units_per_usd, user_id?, effective_from?}` (`units_per_usd` a decimal string; `effective_from` defaults to today, normalized to midnight UTC). Posting the same `(currency, effective_from)` again replaces that row instead of duplicating it. |
| `DELETE` | `/exchange-rates/{id}` | Remove a rate row the user owns. |
| `GET` | `/recurring-rules?user_id=` | The user's recurring rules (rent, salary, subscriptions) that generate ordinary movements on schedule. |
| `POST` | `/recurring-rules` | Create a rule. Body: `{amount, currency?, user_id?, description?, category_id?, payment_method?, account_id?, day_of_month, starts_at?, ends_at?}`. `category_id` (BACK-14 follow-up) must reference an existing category, omitted leaves it uncategorized. `day_of_month` is `"1"`-`"28"` or `"last"` (never 29-31, so a fixed day never drifts across months of different lengths); `starts_at` defaults to now. |
| `PATCH` | `/recurring-rules/{id}` | Edit a rule / deactivate it (`{active: false}`) — any edit affects future generations only, movements already generated are never touched. Any subset of `{description, category_id, payment_method, account_id, amount, currency, day_of_month, ends_at, active}`; `account_id: ""`/`category_id: ""` clears it. There's no way to clear an already-set `ends_at` back to "no end date" through this endpoint. |
| `GET` | `/settings/local-archive?user_id=` | The user's `local_archive_enabled` toggle (BACK-15's "no cloud" tier; defaults to `false`). |
| `PUT` | `/settings/local-archive` | Set the toggle: `{local_archive_enabled, user_id?}`. Independent of any cloud-storage setting — never deletes or stops writing anything server-side by itself. |
| `GET` | `/export/archive?user_id=` | The user's full restorable state — accounts, movements, credit-card purchases — as plaintext JSON. The frontend's "Local backup" panel encrypts this client-side (AES-256-GCM, PBKDF2-SHA256-derived key) before it's ever saved to a file; this endpoint itself has no encryption of its own. |
| `POST` | `/import/archive` | Restore a (frontend-decrypted) archive in the same shape `GET /export/archive` returns. Idempotent by row ID — a row that already exists is skipped, never overwritten; safe to import the same archive more than once. `cancels_movement_id`/`reversed_by_movement_id` are not restored (see Known limitations). Returns counts restored/skipped per collection. |
| `POST` | `/plans` | Create a plan (BACK-10). Body: `{name, plan_type, currency, monthly_target_amount, target_amount?, account_id?, start_date?, end_date?}`. `plan_type` is `stress_test` or `savings`. A savings plan requires `target_amount` and `account_id` (the account's currency must match `currency`); a stress-test plan rejects both. `start_date` defaults to now. |
| `GET` | `/plans` | List every plan the caller owns, each with lightweight progress: a savings plan's all-time contribution total, or a stress-test plan's current (not projected) month-to-date surplus/deficit. |
| `GET` | `/plans/{id}` | One plan's full progress plus the pace checker computed on read: `on_track` and, for a savings plan, `projected_shortfall` (linear month-end projection vs. `monthly_target_amount`). Never changes `status` as a side effect. |
| `PATCH` | `/plans/{id}` | Edit a plan. Body: any subset of `{name, target_amount, monthly_target_amount, end_date, status}`. `target_amount` is only editable on a savings plan. `status` (`active`/`completed`/`abandoned`) is the only way a plan leaves `active` — a `GET` never does this itself. |
| `POST` | `/billing/webhook` | Unauthenticated by user token — provider-signed instead (`X-Billing-Signature`, HMAC-SHA256 over the raw body, `BILLING_WEBHOOK_SECRET`). Body: `{user_id, provider, provider_subscription_id, status, current_period_end}`. Upserts the subscription row; `active` flips `cloud_storage_entitled` to `true` immediately, `past_due`/`canceled` don't flip it until the grace-period sweep. |
| `GET` | `/billing/plan?currency=` | The annual price converted to `currency` using the caller's own exchange rate, falling back to the USD reference price if none is known. Response's own `currency` field says what `amount` is actually expressed in. |

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

### Download

Prebuilt binaries for every tagged release
(`v*`) are published as [GitHub Releases](../../releases) — no Go
toolchain, no Node, nothing to build. Pick the archive for your OS/arch,
extract it, and run the binary (see "Running locally (standalone)"
below for what it does). Each release also ships a `.sha256` file per
archive to verify the download.

- **Linux** (`amd64`/`arm64`): `tar -xzf financial-tracker-standalone-linux-<arch>.tar.gz`,
  then `STANDALONE=true ./financial-tracker-standalone`.
- **macOS** (`amd64`/`arm64`): same as Linux. Gatekeeper will block an
  unsigned binary the first time — either `xattr -d com.apple.quarantine
  financial-tracker-standalone` after extracting, or right-click → Open
  once in Finder and confirm. (Code signing/notarization is icebox —
  see `claude/ideas/`.)
- **Windows** (`amd64`): unzip
  `financial-tracker-standalone-windows-amd64.zip`, then run
  `financial-tracker-standalone.exe` (double-click, or from a terminal
  with `STANDALONE=true` set first).

### Running locally (standalone)

BACK-09's "fully local" distribution: one binary, no server, no account,
no external services — your data lives in a single SQLite file on your
own machine, exportable to CSV anytime.

```bash
make web-build-standalone   # real frontend embedded — see "Download" above for prebuilt releases
STANDALONE=true ./financial-tracker-standalone   # or: ./financial-tracker-standalone --standalone
```

(`make build-standalone` also exists — same binary, but skips the
frontend build/embed step entirely, so `/` serves a plain "no frontend
embedded" page instead of the real UI. The API itself
(`/movements`, `/accounts`, `/import`, `/export`, etc.) works
identically either way; useful if you only care about the API and don't
have Node available.)

What this mode changes, all automatic (no other env vars needed):
- **Storage**: forces `DB_DRIVER=sqlite`. `DB_PATH` defaults to an
  OS-appropriate per-user data directory (`os.UserConfigDir()/financial-tracker/financial-tracker.db`
  — e.g. `~/.config/financial-tracker/` on Linux) instead of `./data`, so
  the binary can be run from anywhere, not just its own working
  directory. Override with `DB_PATH=/wherever/you/want.db` if you'd
  rather pick the location yourself. **Back up your data by copying that
  one file.**
- **No account, no login**: every request is attributed to a single
  fixed local user — same mechanism as `AUTH_DISABLED=true`, forced on
  automatically.
- **No sync**: there is no ledger-service in this mode. Movements are
  created with `sync_status: "local"` (never "pending forever"), `POST
  /sync` is explicitly rejected (404 with a clear message), and
  cancelling a movement always voids it locally — nothing has ever
  synced, so there's never a reversal to create.
- **CSV export, anytime**: `GET /export/movements` streams your history
  in exactly the CSV import model (`date,amount,currency,description,category,payment_method,account`),
  excluding voided/reversed movements by default
  (`include_cancelled=true` for the full picture with extra status
  columns). This endpoint exists in every mode, not just standalone —
  your data is always portable. Export → wipe the data file → re-import
  via `POST /import/movements` round-trips to the same movement list and
  balance.

**Embedded frontend:** `go:embed` bakes in whatever static files exist
under `internal/webui/dist/` at compile time — empty (just a
placeholder) unless something copied a real build there first.
`make web-build-standalone` (INFRA-06) does exactly that: it runs
`web/`'s static-adapter build (`npm run build:static` — `svelte.config.js`'s
`BUILD_TARGET=static` switches from the deployed stack's `adapter-node`
to `adapter-static` with an SPA fallback, since there's no Node runtime
inside the Go binary to run `adapter-node`'s output) and copies the
result into `internal/webui/dist/` before compiling. Every downloaded
release binary (see "Download" above) already has this baked in via
INFRA-06's release workflow; only a from-source `make build-standalone`
(skipping the frontend step) gets the placeholder page instead. See
`internal/webui/webui.go` and `internal/interfaces/api/spa.go` for the
embedding/fallback-routing mechanism itself.

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
- Backfill script importing pre-SQLite history from ledger-service.
