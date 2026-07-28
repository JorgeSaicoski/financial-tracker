.PHONY: help up down restart build rebuild logs test clean update ps run remove-db web web-install web-build build-standalone web-build-standalone deploy-up deploy-down deploy-restart deploy-logs deploy-ps deploy-remove-db e2e-test

# Version stamp for `go build -ldflags "-X main.version=..."` — INFRA-06's
# release workflow overrides this with the pushed tag; a local build just
# gets "dev".
VERSION ?= dev

# Detect container runtime with full paths
CONTAINER_CMD := $(shell command -v /usr/bin/podman 2> /dev/null || command -v /usr/local/bin/podman 2> /dev/null || command -v podman 2> /dev/null || command -v /usr/bin/docker 2> /dev/null || command -v docker 2> /dev/null)
COMPOSE_CMD := $(shell command -v /usr/bin/podman-compose 2> /dev/null || command -v /usr/local/bin/podman-compose 2> /dev/null || command -v podman-compose 2> /dev/null || command -v /usr/bin/docker-compose 2> /dev/null || command -v docker-compose 2> /dev/null)

# deploy/compose.yaml's ledger-service + ledger-postgres sit behind the
# "ledger" profile (it's optional - sync tolerates it being unreachable).
# Pass LEDGER=1 to include them, e.g. `make deploy-up LEDGER=1`.
DEPLOY_PROFILE := $(if $(LEDGER),--profile ledger,)

# Default target
help:
	@echo "Available targets:"
	@echo "  make up          - Start the full stack (postgres, ledger-service, financial-tracker api, web)"
	@echo "  make down        - Stop and remove all services"
	@echo "  make restart     - Restart all services"
	@echo "  make build       - Build the container images"
	@echo "  make rebuild     - Rebuild and start services"
	@echo "  make logs        - View service logs"
	@echo "  make run         - Run the API locally (go run, no containers - needs ledger-service running separately)"
	@echo "  make web         - Run the web frontend locally (npm run dev, no containers - needs the API running separately)"
	@echo "  make web-install - Install/update web frontend dependencies (npm install)"
	@echo "  make web-build   - Build the web frontend for production (npm run build)"
	@echo "  make build-standalone - Build the standalone binary (BACK-09: one file, no"
	@echo "                   server, no account — no real frontend embedded, see below)"
	@echo "  make web-build-standalone - Build the standalone binary WITH the real frontend"
	@echo "                   embedded (static-adapter build copied into internal/webui/dist/"
	@echo "                   first). This is what 'Running locally (standalone)' in the"
	@echo "                   README actually means to run."
	@echo "  make test        - Run Go tests"
	@echo "  make remove-db   - Delete all databases (financial-tracker + ledger-service) for a fresh start"
	@echo "  make clean       - Clean up containers, volumes, and build artifacts"
	@echo "  make update      - Update Go dependencies"
	@echo "  make ps          - List running containers"
	@echo ""
	@echo "Deployable stack (deploy/compose.yaml - Postgres, built images, no host"
	@echo "ports until INFRA-03's proxy lands; see deploy/README.md). Requires"
	@echo "deploy/.env (copy from deploy/.env.example first). Add LEDGER=1 to any"
	@echo "target to include ledger-service + its own Postgres (--profile ledger):"
	@echo "  make deploy-up        - Build and start the deploy stack"
	@echo "  make deploy-down      - Stop the deploy stack (volumes survive)"
	@echo "  make deploy-restart   - Restart the deploy stack"
	@echo "  make deploy-remove-db - Stop the deploy stack and wipe its volumes"
	@echo "  make deploy-logs      - View deploy stack logs"
	@echo "  make deploy-ps        - List deploy stack containers"
	@echo "  make e2e-test         - curl-driven CRUD smoke test against an already-running deploy stack"

# Start services (assumes ../ledger-service exists as a sibling checkout)
up:
	$(COMPOSE_CMD) up -d
	@echo "Services started."
	@echo "  financial-tracker API: http://localhost:8081"
	@echo "  web:                   http://localhost:5173"
	@echo "  ledger-service:        http://localhost:8080"

# Stop services
down:
	$(COMPOSE_CMD) down

# Restart services
restart: down up

# Build the container images
build:
	$(COMPOSE_CMD) build

# Rebuild and start
rebuild: down build up

# View logs
logs:
	$(COMPOSE_CMD) logs -f

# Run the API locally without containers (requires ledger-service already running)
run:
	go run ./internal/cmd/api

# Install web frontend dependencies. Re-runs npm install whenever
# package.json/package-lock.json change; a no-op otherwise.
web-install: web/node_modules/.installed

web/node_modules/.installed: web/package.json web/package-lock.json
	cd web && npm install
	@touch web/node_modules/.installed

# Run the web frontend locally without containers (requires the API
# already running - see web/.env.example for PUBLIC_API_URL).
web: web-install
	cd web && npm run dev -- --host 0.0.0.0

# Build the web frontend for production
web-build: web-install
	cd web && npm run build

# Build the standalone binary (BACK-09): one file, no server, no
# account. go:embed bakes in whatever's under internal/webui/dist/ at
# compile time — empty by default (only a .gitkeep is version-controlled,
# see internal/webui/webui.go's doc comment), so this always produces a
# working API-only binary; it only serves the real UI once a static
# frontend build has been copied into that directory first (see
# README's "Running locally (standalone)" section for the current status
# of that step).
build-standalone:
	go build -ldflags "-X main.version=$(VERSION)" -o financial-tracker-standalone ./internal/cmd/api
	@echo "Built ./financial-tracker-standalone — run with STANDALONE=true or --standalone"

# Build the standalone binary with the real frontend embedded (INFRA-06):
# the static-adapter build (web/svelte.config.js's BUILD_TARGET=static
# path) copied into internal/webui/dist/ right before go:embed needs it
# to exist, i.e. right before `go build`. See internal/webui/webui.go's
# doc comment for why this can't just happen at go:embed time itself.
web-build-standalone: web-install
	cd web && npm run build:static
	find internal/webui/dist -mindepth 1 ! -name '.gitkeep' -delete
	cp -r web/build/. internal/webui/dist/
	go build -ldflags "-X main.version=$(VERSION)" -o financial-tracker-standalone ./internal/cmd/api
	@echo "Built ./financial-tracker-standalone (version $(VERSION), with embedded frontend) — run with STANDALONE=true or --standalone"

# Run Go tests
test:
	go test -v ./...

# Delete all databases: financial-tracker's local SQLite volume AND
# ledger-service's postgres volume. Everything is recreated empty (and
# migrations re-run) on the next make up.
remove-db:
	$(COMPOSE_CMD) down --volumes
	@echo "Databases removed (financial-tracker + ledger-service). Run 'make up' to start fresh."

# Clean up
clean:
	$(COMPOSE_CMD) down -v
	rm -f financial-tracker
	go clean

# Update Go dependencies
update:
	go get -u ./...
	go mod tidy

# List running containers
ps:
	$(COMPOSE_CMD) ps

# --- Deployable stack (deploy/compose.yaml) ---
# Postgres-backed, built images, rootless Podman - see deploy/README.md.
# Unlike `up` above, this needs deploy/.env (cp deploy/.env.example first).

deploy-up:
	cd deploy && $(COMPOSE_CMD) $(DEPLOY_PROFILE) up -d --build

deploy-down:
	cd deploy && $(COMPOSE_CMD) down

deploy-restart: deploy-down deploy-up

# Wipe the deploy stack's Postgres volume(s) for a fresh start.
deploy-remove-db:
	cd deploy && $(COMPOSE_CMD) down --volumes
	@echo "Deploy stack databases removed. Run 'make deploy-up' to start fresh."

deploy-logs:
	cd deploy && $(COMPOSE_CMD) $(DEPLOY_PROFILE) logs -f

deploy-ps:
	cd deploy && $(COMPOSE_CMD) ps

# curl-driven smoke test (accounts/movements/categories CRUD + whatever of
# auth is curl-reachable) against an already-running deploy/ stack — see
# deploy/e2e-test.sh's own header comment and claude/checklist.md's
# "e2e testing" section. Not unit tests (those are `make test`). APP_HOSTNAME
# must match whatever deploy/.env set (defaults to financial-tracker.local).
e2e-test:
	APP_HOSTNAME=$${APP_HOSTNAME:-financial-tracker.local} bash deploy/e2e-test.sh
