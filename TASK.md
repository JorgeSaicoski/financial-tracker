# Task: implement payment methods, categories, cancellable movements, and local-first sync

## What we want

financial-tracker should stop being fully dependent on ledger-service being
up. It needs its own local database, and on top of that:

- Movements can carry a **payment method** (cash, debit card, credit card,
  pix, bank transfer, other).
- **Credit card purchases can be split into installments** — one purchase,
  N scheduled movements over time.
- Movements get a free-text **description** and a **category** from a
  fixed predefined list.
- Movements can be **cancelled**. ledger-service never deletes anything
  (corrections are compensating transactions), so cancelling a movement
  that already reached ledger-service must create a reversal there;
  cancelling one that never reached ledger-service just voids it locally
  — no reversal needed.
- Writing a movement must **never block** on ledger-service being
  reachable. financial-tracker's local database is the source of truth;
  ledger-service is synced to in the background, with a manual sync
  trigger available too.

## Instructions

1. Read `PLAN.md` in this directory — it's the detailed design for this
   work (data model, architecture, cancel/balance semantics, sync
   mechanism, file-by-file changes, rollout order).
2. Check whether the plan is actually good enough to implement as-is:
   - Does the data model support everything above without gaps?
   - Does the cancel/balance-calculation logic hold up (re-derive it,
     don't just trust it)?
   - Are the three "Open questions" in the plan (installment-purchase
     route shape, category names, sync tuning) resolved or reasonably
     defaulted so implementation isn't blocked on them?
   - Any inconsistency with the existing codebase (`domain/`,
     `application/`, `infrastructure/`, `interfaces/`, `cmd/api/main.go`)
     that the plan missed?
3. If the plan holds up: implement it, following the layer-by-layer
   changes and rollout order it lays out. Keep the existing Clean
   Architecture boundaries (domain/entities and domain/repositories don't
   know about SQLite or ledger-service specifics; usecases don't know
   about HTTP or SQL).
4. If something in the plan doesn't hold up: fix the plan first (update
   `PLAN.md`), explain what changed and why, then implement the corrected
   version. Don't silently implement something different from what's
   written.
5. As each rollout step lands, it should build (`go build ./...`) and
   pass `go vet ./...`. Add tests alongside the new usecases/repository
   code (there are none yet for the existing code — at minimum, the new
   cancel and sync logic needs coverage since they're the trickiest
   correctness points).
6. Update `README.md`'s architecture section, roadmap, and API table to
   match what actually got built (not what was planned, if they diverge).

## Definition of done

- `POST /movements` accepts payment method, description, category, and
  (for credit card) an installment count, and behaves correctly for all
  of them.
- `POST /movements/{id}/cancel` and the credit-card-purchase equivalent
  work per the plan's semantics, verified against both a synced and an
  unsynced movement.
- Stopping ledger-service does not break movement creation, listing, or
  cancellation; a `POST /sync` (or the background ticker) catches
  everything up once it's back.
- `GET /movements` and `GET /categories` reflect the new fields.
- `go build ./...` and `go vet ./...` pass.
