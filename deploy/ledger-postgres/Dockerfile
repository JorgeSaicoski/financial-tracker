FROM docker.io/library/postgres:15-alpine
COPY init-db.sh /docker-entrypoint-initdb.d/init-db.sh
COPY 001_create_transactions_table.sql /docker-entrypoint-initdb.d/001_create_transactions_table.sql
