#!/bin/sh
# Dumps every target Postgres database in the stack, prunes old dumps per
# retention policy. Run daily by crond (see entrypoint.sh/Dockerfile) or
# on demand: `podman exec financial-tracker-backup /usr/local/bin/backup.sh`.
set -eu

. /etc/backup.env

BACKUP_DIR=/backups
DAILY_RETENTION="${BACKUP_DAILY_RETENTION:-7}"
WEEKLY_RETENTION="${BACKUP_WEEKLY_RETENTION:-4}"
TIMESTAMP="$(date -u +%Y-%m-%dT%H-%M)"
DOW="$(date -u +%u)" # 1=Monday .. 7=Sunday

mkdir -p "$BACKUP_DIR/weekly"

status=0

# dump_target NAME HOST USER PASSWORD DB REQUIRED
# REQUIRED=1: failure to reach the target fails the whole run (financial-
# tracker's and Authentik's data are this stack's actual source of truth).
# REQUIRED=0: ledger-postgres only exists with --profile ledger, so an
# unreachable target there is expected, not a failure — log and skip.
dump_target() {
	name="$1" host="$2" user="$3" pass="$4" db="$5" required="$6"

	if ! PGPASSWORD="$pass" pg_isready -h "$host" -U "$user" -d "$db" >/dev/null 2>&1; then
		if [ "$required" = "1" ]; then
			echo "backup: FAILED — required target $name ($host) unreachable" >&2
			status=1
		else
			echo "backup: optional target $name ($host) unreachable, skipping (expected when --profile ledger isn't running)"
		fi
		return
	fi

	file="$BACKUP_DIR/${name}_${TIMESTAMP}.sql.gz"
	if ! PGPASSWORD="$pass" pg_dump -h "$host" -U "$user" "$db" | gzip > "$file"; then
		echo "backup: FAILED — pg_dump/gzip for $name ($host) exited non-zero" >&2
		rm -f "$file"
		status=1
		return
	fi
	echo "backup: wrote $file ($(du -h "$file" | cut -f1))"

	if [ "$DOW" = "7" ]; then
		cp "$file" "$BACKUP_DIR/weekly/${name}_${TIMESTAMP}.sql.gz"
		echo "backup: also kept as weekly snapshot"
	fi
}

# prune DIR PREFIX KEEP — deletes everything but the newest KEEP dumps for
# one target in one directory (plain daily dumps and the weekly/ set are
# pruned independently, on their own retention counts).
prune() {
	dir="$1" prefix="$2" keep="$3"
	ls -1t "$dir"/"${prefix}"_*.sql.gz 2>/dev/null | tail -n "+$((keep + 1))" | while IFS= read -r old; do
		rm -f "$old"
		echo "backup: pruned $old"
	done
}

dump_target financial_tracker ft-postgres "$FT_POSTGRES_USER" "$FT_POSTGRES_PASSWORD" "$FT_POSTGRES_DB" 1
dump_target authentik authentik-postgres "$AUTHENTIK_POSTGRES_USER" "$AUTHENTIK_POSTGRES_PASSWORD" "$AUTHENTIK_POSTGRES_DB" 1
dump_target ledger ledger-postgres "${LEDGER_POSTGRES_USER:-}" "${LEDGER_POSTGRES_PASSWORD:-}" "${LEDGER_POSTGRES_DB:-}" 0

for prefix in financial_tracker authentik ledger; do
	prune "$BACKUP_DIR" "$prefix" "$DAILY_RETENTION"
	prune "$BACKUP_DIR/weekly" "$prefix" "$WEEKLY_RETENTION"
done

echo "backup: run complete at $(date -u +%Y-%m-%dT%H:%M:%SZ), status=$status"
exit "$status"
