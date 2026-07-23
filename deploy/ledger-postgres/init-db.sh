#!/bin/bash
set -e

# Mirrors ledger-service's own init-db.sh (../../ledger-service/init-db.sh
# in a sibling checkout) — kept here, not fetched or bind-mounted, so
# deploy/compose.yaml's ledger-postgres build needs no local ledger-service
# checkout and no extra network fetch beyond the git clone the ledger-service
# app build already requires. Update both together if ledger-service's
# schema changes.

# Create test database
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE DATABASE ledger_db_test;
EOSQL

# Run migrations on main database
echo "Running migrations on ledger_db..."
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" < /docker-entrypoint-initdb.d/001_create_transactions_table.sql

# Run migrations on test database
echo "Running migrations on ledger_db_test..."
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "ledger_db_test" < /docker-entrypoint-initdb.d/001_create_transactions_table.sql
