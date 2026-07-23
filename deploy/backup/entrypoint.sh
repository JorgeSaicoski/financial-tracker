#!/bin/sh
# busybox crond does not inherit the container's environment into jobs it
# spawns, so the DB credentials compose.yaml passes in as `environment:`
# would otherwise be invisible to backup.sh when cron runs it. Snapshot
# them to a file once at startup; backup.sh sources it every run (also
# covers on-demand `podman exec ... backup.sh` runs, same env either way).
set -eu

# `export -p` (not plain `env`) so values are safely shell-quoted —
# passwords may contain characters ($, spaces, quotes) that plain env's
# unquoted KEY=value output would mangle or, sourced literally, execute.
export -p | grep -E "(FT_POSTGRES_|AUTHENTIK_POSTGRES_|LEDGER_POSTGRES_|BACKUP_)" > /etc/backup.env

exec crond -f -l 2
