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

- Rootless Podman + `podman-compose` on the host.
- `../../ledger-service` checked out as a sibling of this repo (same
  assumption as the root `docker-compose.yml` — this stack builds it from
  source rather than duplicating its Dockerfile/migrations).

## Bring the stack up

```bash
cd deploy
cp .env.example .env
# edit .env: set FT_POSTGRES_PASSWORD and LEDGER_POSTGRES_PASSWORD to real
# secrets (or point them at your secrets manager of choice — anything that
# lands in .env works, nothing is hardcoded in compose.yaml), and adjust
# DEFAULT_USER_ID/PUBLIC_API_URL for your deployment.
podman-compose up -d --build
```

This starts, in dependency order: `ft-postgres` and `ledger-postgres`
(healthchecked before anything depending on them starts), `ledger-service`,
`financial-tracker` (API, `DB_DRIVER=postgres`), and `web` (production
SvelteKit build via `@sveltejs/adapter-node`, not `npm run dev`).

```bash
podman-compose down          # stop everything; data volumes survive
podman-compose down --volumes  # wipe databases too — fresh start
podman-compose logs -f
podman-compose ps
```

## Networking: no host ports by default

Only Postgres, ledger-service, financial-tracker, and web talk to each
other over the compose-internal network — **nothing publishes a host port**
in this file. That's deliberate: INFRA-03 adds the reverse proxy that will
be the one thing exposed to the outside world (mapped to 8080/8443, not
raw 80/443), fronting both `financial-tracker` and `web`. Until INFRA-03
lands, this stack is not reachable from a browser.

### Verifying without a proxy yet

```bash
podman exec financial-tracker-api wget -qO- http://localhost:8081/movements
```

or temporarily uncomment the `ports:` block under `financial-tracker`
and/or `web` in `compose.yaml` for local testing — remove it again once
INFRA-03's proxy is in place (don't leave both a direct port and the proxy
open in a real deployment).

## Rootless-Podman / SELinux notes

- Bind mounts (only `ledger-postgres`'s init-script/migration mounts here)
  use the `:z` relabel flag, same as the root `docker-compose.yml`. The
  named volumes (`ft_postgres_data`, `ledger_postgres_data`) don't need
  it — SELinux labeling only matters for host-path bind mounts.
- No privileged ports are opened by this file (see above), so no
  `CAP_NET_BIND_SERVICE` concerns.
- All images run as their upstream default (non-root) user; nothing here
  needs `--privileged` or extra capabilities.

## Boot persistence (systemd / Quadlet)

`podman-compose up -d` doesn't survive a reboot on its own. Two options:

**Quick: `podman generate systemd`** (works today, no extra files):
```bash
podman-compose up -d
podman generate systemd --new --files --name financial-tracker-api
podman generate systemd --new --files --name financial-tracker-web
podman generate systemd --new --files --name ledger-service
podman generate systemd --new --files --name financial-tracker-postgres
podman generate systemd --new --files --name ledger-postgres
mkdir -p ~/.config/systemd/user
mv container-*.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now container-financial-tracker-postgres.service \
  container-ledger-postgres.service container-ledger-service.service \
  container-financial-tracker-api.service container-financial-tracker-web.service
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
the supported path is `cmd/migrate-sqlite` (**not implemented yet — tracked
as BACK-06**; this stack works standalone with an empty Postgres database
in the meantime). Once it lands, the procedure is:

1. Stop whatever is writing to the SQLite file (dev stack, standalone
   binary, etc.) — no live writers during migration.
2. Bring up this stack's Postgres only: `podman-compose up -d ft-postgres`.
3. Run `cmd/migrate-sqlite` pointed at the SQLite file and this stack's
   `DATABASE_URL` (from `.env`).
4. Start the rest of the stack: `podman-compose up -d`.

## Backups

Not covered here — see INFRA-04 (Postgres backups + tested restore),
which builds on the `ft_postgres_data` volume this file creates.
