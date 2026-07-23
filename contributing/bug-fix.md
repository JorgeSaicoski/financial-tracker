# Walkthrough — fixing a bug

Real case, from this repo's history: the UI's category/payment-method
dropdowns were rendering empty, so nothing in the form was selectable.

## 1. Reproduce against the API directly, not the UI

The UI hides status codes and swallows some errors on purpose (see
`loadCategories()`'s empty `catch` in `+page.svelte`, which exists so a
categories-endpoint hiccup doesn't break the rest of the page):
```bash
curl -sv localhost:8081/categories
```
This returned a plain `404 page not found` — but `GET /categories` is a
real route in `router.go`. That's the whole bug, right there, from one
command.

## 2. Check you're running the code you're reading

Go's API container does **not** hot-reload — the binary is baked in at
image build time, so a container started before your latest code change
is still serving the old binary regardless of what's on disk. (The `web`
container is the opposite: it bind-mounts `web/` and Vite hot-reloads,
no rebuild needed.) That was the actual root cause here — the container
predated the `/categories` route entirely. The fix wasn't a code change:
```bash
make rebuild
curl -s localhost:8081/categories   # now returns the real category list
```

## 3. When the bug *is* in the code, write a failing test first

In the layer where the bug lives:
- Usecase/validation bug → a test in `application/usecases/*_test.go`
  using the fakes in `fakes_test.go`, styled like
  `TestAddCurrencyRejectsBadCodes` in
  [new-feature.md](new-feature.md) step 10.
- SQL bug → `infrastructure/sqlite/repository_test.go`, which runs
  against a real temporary SQLite database, not a fake.
- Wrong HTTP status/shape → exercise the handler directly, or just
  `curl` against a rebuilt container.

`application/usecases/cancel_movement_test.go` is a good model for a
bug-shaped test: it locks down exact edge cases (double-cancel,
reversal-of-reversal) that were easy to reintroduce by accident.

## 4. Fix at the right layer

Validation belongs in usecases, SQL correctness in
`infrastructure/sqlite`, status-code mapping in handlers. Don't patch a
backend bug by adding a workaround in the frontend. And if the fix
involves a new interface, it goes where contracts live
(`application/usecases/interfaces.go`, `application/repositories/`,
`application/services/` — see [README.md](README.md) and
[architecture.md](architecture.md)), never inline next to an
implementation, and typed against an `application/dto` type rather than
a domain entity if you're touching a repository/service contract (the
existing code doesn't do this yet — see architecture.md — but a bug fix
is not the place to silently perpetuate that gap into new signatures).

## 5. Verify twice

`go test ./...` green, then re-run the exact reproduction command from
step 1 against a rebuilt container, then confirm in the browser at
`localhost:5173`.
