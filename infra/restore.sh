#!/bin/sh
# Restore a backup into the running database:  ./infra/restore.sh backups/homeops-2026-10-22.sql.gz
# The app is stopped during the restore so nothing writes mid-way.
set -eu
file=${1:?usage: infra/restore.sh backups/homeops-YYYY-MM-DD.sql.gz}
[ -f "$file" ] || { echo "no such file: $file"; exit 1; }
printf 'This replaces ALL current HomeOps data with %s. Type "restore" to continue: ' "$file"
read -r answer
[ "$answer" = "restore" ] || { echo "aborted"; exit 1; }
docker compose stop app
gunzip -c "$file" | docker compose exec -T db psql -q -v ON_ERROR_STOP=1 -U homeops -d homeops
docker compose start app
echo "restored from $file"
