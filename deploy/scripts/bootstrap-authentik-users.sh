#!/usr/bin/env bash
# Non-interactive Authentik test-user bootstrap (claude/checklist.md's
# "Authentik" section) — creates a handful of real login-capable users via
# the Admin API, no browser click-through. Needs AUTHENTIK_BOOTSTRAP_TOKEN
# (deploy/.env, wired into compose.yaml's authentik-server/-worker
# environment) — Authentik mints that token for `akadmin` on first boot
# from AUTHENTIK_BOOTSTRAP_PASSWORD/AUTHENTIK_BOOTSTRAP_TOKEN, see
# deploy/README.md's Authentik section.
#
# Reaches the Admin API the same way deploy/e2e-test.sh reaches the app:
# through Caddy on the already-published 8443, --resolve'd straight to
# 127.0.0.1 so this needs no /etc/hosts entry and works unmodified on a CI
# runner. authentik-server itself publishes no host port (compose.yaml) —
# Caddy's auth.${APP_HOSTNAME} vhost is the only way in from outside the
# compose network.
#
# Usage: ./bootstrap-authentik-users.sh [username...]
# Defaults to three users (ft-test-1, ft-test-2, ft-test-3) if none given.
# Idempotent: an existing username is left alone (its password is not
# reset) rather than erroring, so reruns against a stack whose Postgres
# data persists are safe.
set -euo pipefail

APP_HOSTNAME="${APP_HOSTNAME:-financial-tracker.local}"
AUTH_HOST="auth.${APP_HOSTNAME}"
BASE_URL="https://${AUTH_HOST}:8443/api/v3"

: "${AUTHENTIK_BOOTSTRAP_TOKEN:?AUTHENTIK_BOOTSTRAP_TOKEN must be set (see deploy/.env)}"

CURL=(curl -sk --resolve "${AUTH_HOST}:8443:127.0.0.1" --fail-with-body
	-H "Authorization: Bearer ${AUTHENTIK_BOOTSTRAP_TOKEN}")

usernames=("$@")
if [ "${#usernames[@]}" -eq 0 ]; then
	usernames=(ft-test-1 ft-test-2 ft-test-3)
fi

random_password() {
	openssl rand -base64 18
}

for username in "${usernames[@]}"; do
	existing=$("${CURL[@]}" "${BASE_URL}/core/users/?username=${username}")
	pk=$(jq -r '.results[0].pk // empty' <<<"$existing")

	if [ -n "$pk" ]; then
		echo "skip  - $username already exists (pk=$pk), password left as-is"
		continue
	fi

	created=$("${CURL[@]}" -X POST "${BASE_URL}/core/users/" \
		-H 'Content-Type: application/json' \
		-d "{\"username\":\"${username}\",\"name\":\"${username}\",\"email\":\"${username}@${APP_HOSTNAME}\",\"is_active\":true,\"path\":\"users\"}")
	pk=$(jq -r '.pk' <<<"$created")
	if [ -z "$pk" ] || [ "$pk" = "null" ]; then
		echo "FAIL  - could not create $username: $created" >&2
		exit 1
	fi

	password=$(random_password)
	"${CURL[@]}" -X POST "${BASE_URL}/core/users/${pk}/set_password/" \
		-H 'Content-Type: application/json' \
		-d "{\"password\":\"${password}\"}" >/dev/null

	echo "ok    - created $username (pk=$pk) password=${password}"
done
