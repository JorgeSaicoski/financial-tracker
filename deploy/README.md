# Deploying financial-tracker (Podman + PostgreSQL)

This is the **production/deployable** stack — PostgreSQL instead of SQLite,
built images instead of dev servers, and rootless Podman throughout. For
day-to-day local development keep using the repo-root `docker-compose.yml`
(`make up`); it's unchanged.

Tool choice: **podman-compose**, not `podman kube play` — it reuses the same
compose file shape as the existing dev stack (one file to learn, not two
formats), and `podman generate systemd` (below) gets you unit-file
persistence without needing a Kubernetes YAML translation step.

## Prerequisites

- Rootless Podman + `podman-compose` on the host. Nothing else — unlike the
  root `docker-compose.yml`, this file does **not** need a local
  `../ledger-service` checkout; the ledger-service app image is built
  straight from its git repo. (Its Postgres init/migration SQL is a local
  copy under `deploy/ledger-postgres/` rather than fetched from git too —
  see the comment on `ledger-postgres` in `compose.yaml` for why.)

## ledger-service is optional

financial-tracker's local Postgres is always the source of truth; sync to
ledger-service tolerates it being unreachable and retries on a cooldown
(see root README's "Running locally" section) — nothing blocks on it.
Bring it up alongside the rest with `--profile ledger`:

```bash
podman-compose --profile ledger up -d --build
```

or omit `--profile ledger` to run financial-tracker + web without it (e.g.
pointing `LEDGER_SERVICE_URL` in `.env` at an externally-hosted instance
instead, or running fully standalone).

`LEDGER_SERVICE_GIT_URL` in `.env` controls what ledger-service build pulls
(defaults to `JorgeSaicoski/ledger-service`'s `main` branch) — pin it to a
tag or point it at a fork if needed, e.g.
`https://github.com/you/ledger-service.git#v1.2.0`.

## Bring the stack up

```bash
cd deploy
cp .env.example .env
# edit .env: set FT_POSTGRES_PASSWORD, LEDGER_POSTGRES_PASSWORD, and
# AUTHENTIK_POSTGRES_PASSWORD/AUTHENTIK_SECRET_KEY to real secrets (or
# point them at your secrets manager of choice — anything that lands in
# .env works, nothing is hardcoded in compose.yaml), and adjust
# DEFAULT_USER_ID/APP_HOSTNAME/PUBLIC_API_URL for your deployment.
podman-compose --profile ledger up -d --build   # or drop --profile ledger — see above
```

This starts, in dependency order: `ft-postgres` and `authentik-postgres`
(and `ledger-postgres` if `--profile ledger` is given, all healthchecked
before anything depending on them starts), `ledger-service` (same
profile), `authentik-server` + `authentik-worker` (identity provider —
see "Authentik" below), `financial-tracker` (API, `DB_DRIVER=postgres`),
`web` (production SvelteKit build via `@sveltejs/adapter-node`, not `npm
run dev`), and `caddy` (the reverse proxy — see "Networking" below for
the URL map).

```bash
podman-compose down          # stop everything; data volumes survive
podman-compose down --volumes  # wipe databases too — fresh start
podman-compose logs -f
podman-compose ps
```

## Networking: one entry point (caddy)

Postgres, ledger-service, Authentik, financial-tracker, and web only talk
to each other over the compose-internal network — **`caddy` is the only
service publishing host ports** (`8080:80`, `8443:443`; deliberately not
raw 80/443). It reverse-proxies everything else:

| URL | Routes to |
|---|---|
| `https://${APP_HOSTNAME}:8443/` | `web` (the SvelteKit app) |
| `https://${APP_HOSTNAME}:8443/api/*` | `financial-tracker` (prefix stripped) |
| `https://auth.${APP_HOSTNAME}:8443/` | `authentik-server` |

TLS is Caddy's automatic internal CA (self-signed) by default — see
`deploy/Caddyfile`'s comments for swapping in a real cert or ACME email.
Same-origin end state: `financial-tracker`'s `CORS_ALLOWED_ORIGIN` is
locked to `https://${APP_HOSTNAME}:8443` (see `compose.yaml`), not `*`.

**No real DNS?** Add both hostnames to `/etc/hosts` pointing at the
Podman host, e.g.:
```
127.0.0.1 financial-tracker.local auth.financial-tracker.local
```
and trust Caddy's internal CA locally, or click through the browser's
self-signed warning.

### Bypassing the proxy for local debugging

```bash
podman exec financial-tracker-api wget -qO- http://localhost:8081/movements
```

or temporarily uncomment the `ports:` block under `financial-tracker`
and/or `web` in `compose.yaml` — remove it again afterward (don't leave
both a direct port and the proxy open in a real deployment).

## Authentik (identity provider)

`authentik-server` + `authentik-worker` (no separate Redis — Authentik
dropped that dependency in release 2025.10, so Postgres + server + worker
is the current minimum stack) provide the OIDC login BACK-02/FRONT-04
(Phase 2) authenticate against. `deploy/authentik/blueprints/
financial-tracker.yaml` is bind-mounted into both containers and
auto-applied by the worker on startup, creating the OAuth2/OIDC Provider
and Application automatically — no manual "create a provider" clicking.

**Sub claim decision:** the provider's Subject mode is set to "Based on
the User's UUID" (`sub_mode: user_uuid` in the blueprint), so the OIDC
`sub` claim Authentik issues is already a lowercase UUID — exactly what
`DEFAULT_USER_ID`/ledger-service require today. BACK-02 can consume `sub`
directly with no transformation.

### One-time setup (still manual — Authentik requires it)

1. Bring the stack up (`podman-compose --profile ledger up -d --build`)
   and wait for `authentik-server`/`authentik-worker` to report healthy:
   `podman-compose logs -f authentik-server`.
2. Visit Authentik's initial-setup flow to create the admin account:
   `https://auth.${APP_HOSTNAME}:8443/if/flow/initial-setup/` (add an
   `/etc/hosts` entry first if you don't have real DNS — see
   "Networking" above).
3. Confirm the blueprint applied: Admin interface → **Customization →
   Blueprints** should show `financial-tracker OAuth2 provider +
   application` as applied; **Applications → Applications** should list
   `financial-tracker`. If it didn't apply (check
   `podman-compose logs authentik-worker` for blueprint errors — field
   names occasionally shift between Authentik releases), create the
   provider/application by hand instead: Admin interface → **Applications
   → Providers → Create → OAuth2/OpenID Provider** (client type
   **Public**, authorization flow `default-provider-authorization-
   explicit-consent`, Subject mode **Based on the User's UUID**, redirect
   URI matching `PUBLIC_OIDC_REDIRECT_URI` from `.env`), then
   **Applications → Applications → Create**, linking to that provider.
4. Values BACK-02/FRONT-04 need, once implemented:
   - `OIDC_ISSUER_URL` = `https://auth.${APP_HOSTNAME}:8443/application/o/financial-tracker/`
   - `PUBLIC_OIDC_CLIENT_ID` = the same `.env` value the blueprint used
     (default `financial-tracker`)

## Rootless-Podman / SELinux notes

- Two read-only bind mounts: `./Caddyfile` (into `caddy`) and
  `./authentik/blueprints` (into `authentik-server`/`authentik-worker`).
  Both are `:ro`, which SELinux's default container policy (`container_file_t`
  via `:z`) doesn't strictly require for read access on most rootless
  Podman setups — if you hit an SELinux denial reading either, add `:z`
  (shared label; fine here since nothing else needs these paths) to the
  mount in `compose.yaml`. Everything else (ledger-postgres's
  init/migration SQL) is baked into its image at build time instead — see
  above. The named volumes (`ft_postgres_data`, `ledger_postgres_data`,
  `authentik_postgres_data`, `authentik_media`, `caddy_data`,
  `caddy_config`) don't need relabeling either way — SELinux labeling only
  matters for host-path bind mounts.
- No privileged ports (<1024) are opened by this file — `caddy`'s
  `8080`/`8443` are both unprivileged — so no `CAP_NET_BIND_SERVICE`
  concerns.
- The `postgres:*-alpine` images and `financial-tracker-web` (Node) run as
  their upstream non-root default user. `financial-tracker-api` and
  `ledger-service` are `alpine:latest`-based with no `USER` set, so they
  currently run as root **inside the container** — still fine under
  rootless Podman (root-in-container maps to your unprivileged host user,
  not host root), but worth tightening with an explicit non-root `USER` in
  both Dockerfiles at some point.

## Boot persistence (systemd / Quadlet)

`podman-compose up -d` doesn't survive a reboot on its own. Two options:

**Quick: `podman generate systemd`** (works today, no extra files):
```bash
podman-compose up -d
for name in financial-tracker-postgres authentik-postgres ledger-postgres \
  ledger-service authentik-server authentik-worker \
  financial-tracker-api financial-tracker-web financial-tracker-caddy \
  financial-tracker-backup; do
  podman generate systemd --new --files --name "$name"
done
mkdir -p ~/.config/systemd/user
mv container-*.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now \
  container-financial-tracker-postgres.service container-authentik-postgres.service \
  container-ledger-postgres.service container-ledger-service.service \
  container-authentik-server.service container-authentik-worker.service \
  container-financial-tracker-api.service container-financial-tracker-web.service \
  container-financial-tracker-caddy.service container-financial-tracker-backup.service
loginctl enable-linger "$USER"   # let user services start without a login session
```

**Better long-term: Quadlet** (`.container`/`.volume` unit files under
`~/.config/containers/systemd/`) — one unit per service instead of
generated wrapper scripts, and it re-derives from source on `systemctl
daemon-reload` instead of going stale like a `generate systemd` snapshot.
Not written yet; worth doing if/when this stack needs to survive image
rebuilds without manual regeneration.

## Migrating existing SQLite data

If you have existing data in the SQLite-backed dev/standalone deployment,
the supported path is `cmd/migrate-sqlite` — it copies every table
(currencies, accounts, account snapshots, credit-card purchases,
movements) in one Postgres transaction, preserving ids, timestamps, sync
state, and every link (reversals, installments, transfers). Re-importing
via CSV instead would silently drop all of that.

1. Stop whatever is writing to the SQLite file (dev stack, standalone
   binary, etc.) — no live writers during migration.
2. Bring up this stack's Postgres only: `podman-compose up -d ft-postgres`.
3. From the repo root, run:
   ```bash
   go run ./cmd/migrate-sqlite \
     --db-path /path/to/financial-tracker.db \
     --database-url "$DATABASE_URL"   # same value as ft-postgres's DSN in .env
   ```
   It refuses to run if the target already has movements/accounts/
   snapshots/purchases (a prior migration, most likely) — pass `--force`
   only if you're deliberately re-running into a target you know is safe
   to write into; it does **not** wipe the target first, so a `--force`
   run into data that doesn't already match will fail on the first id
   collision rather than silently merging.
4. Check the printed per-table source/target counts match (the command
   also exits non-zero on any mismatch) and spot-check a balance or two
   against the old deployment before relying on the new one.
5. Start the rest of the stack: `podman-compose up -d`.

## Backups

The `backup` service (`deploy/backup/`) dumps all three databases daily at
03:00 UTC and prunes old dumps, writing to the `backup_data` volume as
`gzip`-compressed, timestamped files: `financial_tracker_<UTC
timestamp>.sql.gz`, `authentik_<...>.sql.gz`, and — only when
`--profile ledger` is running — `ledger_<...>.sql.gz` (unreachable is
logged and skipped, not a failure, the rest of the time). Retention is
independent for two tiers: the newest `BACKUP_DAILY_RETENTION` (default 7)
plain dumps, and the newest `BACKUP_WEEKLY_RETENTION` (default 4) Sunday
dumps kept separately under `backup_data/weekly/`.

**Mechanism: sidecar container with cron, not a host systemd timer.**
Same reasoning INFRA-01 gave for podman-compose over `podman kube play` —
this whole stack is defined in `compose.yaml` alone; a host-level systemd
timer would be a second mechanism to keep in sync with it. `deploy/backup/
Dockerfile`'s `crond` runs in the foreground as the container's PID 1, so
`podman logs financial-tracker-backup` shows every run (including
failures — the job exits non-zero if a required target's dump fails).

**Off-host copying of `backup_data` is the operator's job** — this only
protects against database corruption/accidental deletion on the same
host, not host loss. Mount it, `rsync` it, whatever fits your setup; not
automated here.

### Running a backup on demand

```bash
podman exec financial-tracker-backup /usr/local/bin/backup.sh
```

### Restore procedure

Restoring into a **fresh** stack (the scenario this is actually for — a
lost/corrupted host):

1. Bring up just the Postgres containers, empty:
   ```bash
   podman-compose up -d ft-postgres authentik-postgres   # add ledger-postgres too if you run --profile ledger
   ```
2. For each database, in any order (they're independent), pick the dump
   to restore from `backup_data` (or `backup_data/weekly` for an
   older one) and:
   ```bash
   # financial-tracker
   gunzip -c /path/to/backup_data/financial_tracker_<timestamp>.sql.gz | \
     podman exec -i financial-tracker-postgres psql -U "$FT_POSTGRES_USER" -d "$FT_POSTGRES_DB"

   # Authentik
   gunzip -c /path/to/backup_data/authentik_<timestamp>.sql.gz | \
     podman exec -i authentik-postgres psql -U "$AUTHENTIK_POSTGRES_USER" -d "$AUTHENTIK_POSTGRES_DB"

   # ledger-service (only if you run --profile ledger)
   gunzip -c /path/to/backup_data/ledger_<timestamp>.sql.gz | \
     podman exec -i ledger-postgres psql -U "$LEDGER_POSTGRES_USER" -d "$LEDGER_POSTGRES_DB"
   ```
3. Start the rest of the stack: `podman-compose up -d`.
4. Verify: movement count and a balance spot-check in financial-tracker
   (`GET /movements`/`GET /cashflow` against a known-good number from
   before the loss) and that logging into Authentik with an existing user
   still works.
