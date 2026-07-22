.PHONY: help up down restart build rebuild logs test clean update ps run remove-db

# Detect container runtime with full paths
CONTAINER_CMD := $(shell command -v /usr/bin/podman 2> /dev/null || command -v /usr/local/bin/podman 2> /dev/null || command -v podman 2> /dev/null || command -v /usr/bin/docker 2> /dev/null || command -v docker 2> /dev/null)
COMPOSE_CMD := $(shell command -v /usr/bin/podman-compose 2> /dev/null || command -v /usr/local/bin/podman-compose 2> /dev/null || command -v podman-compose 2> /dev/null || command -v /usr/bin/docker-compose 2> /dev/null || command -v docker-compose 2> /dev/null)

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
	@echo "  make test        - Run Go tests"
	@echo "  make remove-db   - Delete all databases (financial-tracker + ledger-service) for a fresh start"
	@echo "  make clean       - Clean up containers, volumes, and build artifacts"
	@echo "  make update      - Update Go dependencies"
	@echo "  make ps          - List running containers"

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
	go run ./cmd/api

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
