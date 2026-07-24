.PHONY: help up down restart build rebuild logs test clean update ps run remove-db web web-install web-build deploy-up deploy-down deploy-restart deploy-logs deploy-ps deploy-remove-db

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
