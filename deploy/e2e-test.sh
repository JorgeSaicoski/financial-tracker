#!/usr/bin/env bash
# End-to-end smoke test against the full deploy/ stack (see
# claude/checklist.md's "e2e testing" section — this is the target file,
# not unit tests; those already run via `make test`/CI's `go test ./...`).
#
# Exercises core CRUD (accounts, movements, categories) plus whatever of
# the auth flow is genuinely curl-reachable: with AUTH_DISABLED=true (see
# compose.yaml), every request is attributed to DEFAULT_USER_ID with no
# token needed, so this checks that identity resolves and GET /config
# correctly reports auth as off. The real browser-redirect PKCE login
# against Authentik is verified separately (Goal 2's own browser-driven
# pass) — a curl script can't complete that redirect.
#
# Assumes the stack is already up (`make deploy-up AUTH_DISABLED=true`,
# or CI's own docker compose invocation) and reachable at APP_HOSTNAME.
# Requires curl and jq.
set -euo pipefail

APP_HOSTNAME="${APP_HOSTNAME:-financial-tracker.local}"
BASE_URL="https://${APP_HOSTNAME}:8443/api"
# Caddy's TLS is its own internal CA (self-signed, see Caddyfile) and
# APP_HOSTNAME may not resolve via real DNS (homelab default) — trust
# neither, just point curl straight at localhost instead of requiring an
# /etc/hosts entry, so this also runs unmodified on a CI runner.
CURL=(curl -sk --resolve "${APP_HOSTNAME}:8443:127.0.0.1" --fail-with-body)

# Unique per run so a rerun against a stack whose Postgres data persists
# (a real deploy, or a local `make deploy-up` left running between runs —
# unlike a fresh-DB CI run) doesn't collide with account/category names
# from a previous run (accounts are unique per user+name).
run_id="e2e-$(date +%s)-$$"

pass=0
fail=0

check() {
	local desc="$1"
	shift
	if "$@"; then
		echo "ok   - $desc"
		pass=$((pass + 1))
	else
		echo "FAIL - $desc"
		fail=$((fail + 1))
	fi
}

echo "=== e2e-test: $BASE_URL ==="

# --- config / identity ---
config=$("${CURL[@]}" "$BASE_URL/config")
check "GET /config reports auth_enabled=false (AUTH_DISABLED)" \
	test "$(jq -r .auth_enabled <<<"$config")" = "false"

me=$("${CURL[@]}" "$BASE_URL/me")
check "GET /me resolves an identity with no token" \
	test -n "$(jq -r .id <<<"$me")"

# --- currencies ---
"${CURL[@]}" -X POST "$BASE_URL/currencies" \
	-H 'Content-Type: application/json' -d '{"code":"usd"}' >/dev/null
currencies=$("${CURL[@]}" "$BASE_URL/currencies")
check "usd registered and listed" \
	grep -q '"usd"' <<<"$currencies"

# --- accounts ---
account=$("${CURL[@]}" -X POST "$BASE_URL/accounts" \
	-H 'Content-Type: application/json' \
	-d "{\"name\":\"$run_id checking\",\"type\":\"bank\",\"currency\":\"usd\"}")
account_id=$(jq -r .id <<<"$account")
check "POST /accounts created an account" \
	test -n "$account_id" -a "$account_id" != "null"

accounts=$("${CURL[@]}" "$BASE_URL/accounts")
check "GET /accounts lists the new account" \
	grep -q "$account_id" <<<"$accounts"

# --- categories ---
categories=$("${CURL[@]}" "$BASE_URL/categories")
check "GET /categories includes the seeded 'other' system category" \
	grep -q '"other"' <<<"$categories"

category=$("${CURL[@]}" -X POST "$BASE_URL/categories" \
	-H 'Content-Type: application/json' \
	-d "{\"name\":\"$run_id-category\",\"avoidability_percent\":50}")
category_id=$(jq -r .id <<<"$category")
check "POST /categories created a category" \
	test -n "$category_id" -a "$category_id" != "null"

# --- movements ---
movement=$("${CURL[@]}" -X POST "$BASE_URL/movements" \
	-H 'Content-Type: application/json' \
	-d "{\"amount\":-1500,\"currency\":\"usd\",\"description\":\"e2e coffee\",\"category_id\":\"$category_id\",\"payment_method\":\"cash\",\"account_id\":\"$account_id\"}")
movement_id=$(jq -r .id <<<"$movement")
check "POST /movements created a movement" \
	test -n "$movement_id" -a "$movement_id" != "null"

movements=$("${CURL[@]}" "$BASE_URL/movements")
check "GET /movements lists the new movement" \
	grep -q "$movement_id" <<<"$movements"

updated=$("${CURL[@]}" -X PATCH "$BASE_URL/movements/$movement_id" \
	-H 'Content-Type: application/json' \
	-d '{"description":"e2e coffee (renamed)"}')
check "PATCH /movements/{id} renamed the movement" \
	test "$(jq -r .movement.description <<<"$updated")" = "e2e coffee (renamed)"

cancelled=$("${CURL[@]}" -X POST "$BASE_URL/movements/$movement_id/cancel")
check "POST /movements/{id}/cancel voided the movement" \
	test "$(jq -r .movement.status <<<"$cancelled")" = "voided"

# --- cleanup: remove the test category from this user's own list ---
# (curl --fail-with-body already makes a non-2xx here trip `set -e`.)
"${CURL[@]}" -X DELETE "$BASE_URL/categories/$category_id" >/dev/null
echo "ok   - DELETE /categories/{id} succeeded"
pass=$((pass + 1))

echo "=== e2e-test: $pass passed, $fail failed ==="
test "$fail" -eq 0
