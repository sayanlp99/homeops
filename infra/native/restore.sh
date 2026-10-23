#!/usr/bin/env bash
# Restore a nightly backup on the native (systemd) install.
#
#   sudo ./infra/native/restore.sh /var/lib/homeops/backups/homeops-YYYY-MM-DD.sql.gz [files-YYYY-MM-DD.tar.gz]
#
# Replaces ALL current HomeOps data. The app is stopped during the restore so nothing
# writes mid-way. RESTORE_DB=<name> restores into another database instead (for a
# rehearsal); the app is then left running.
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "Run with sudo"; exit 1; }
sql=${1:?usage: restore.sh homeops-YYYY-MM-DD.sql.gz [files-YYYY-MM-DD.tar.gz]}
files=${2:-}
db=${RESTORE_DB:-homeops}
[[ -f $sql ]] || { echo "no such file: $sql"; exit 1; }
[[ -z $files || -f $files ]] || { echo "no such file: $files"; exit 1; }
gunzip -t "$sql"

if [[ $db == homeops ]]; then
  printf 'This replaces ALL current HomeOps data with %s. Type "restore" to continue: ' "$(basename "$sql")"
  read -r answer
  [[ $answer == restore ]] || { echo "aborted"; exit 1; }
  systemctl stop homeops
  trap 'systemctl start homeops' EXIT
fi

echo "▸ Database → $db"
# Backups are taken with --clean --if-exists, so replaying one overwrites what's there.
gunzip -c "$sql" | sudo -u postgres psql -q -v ON_ERROR_STOP=1 -d "$db" >/dev/null
sudo -u postgres psql -d "$db" -c "GRANT ALL ON SCHEMA public TO homeops" -q
# The dump has no owners; give the app's role its tables, views and sequences back.
sudo -u postgres psql -d "$db" -qtc "SELECT format('ALTER %s %I OWNER TO homeops;',
    CASE c.relkind WHEN 'v' THEN 'VIEW' WHEN 'S' THEN 'SEQUENCE' WHEN 'm' THEN 'MATERIALIZED VIEW' ELSE 'TABLE' END, c.relname)
  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'v', 'S', 'm')" \
  | sudo -u postgres psql -q -v ON_ERROR_STOP=1 -d "$db" >/dev/null

if [[ -n $files && $db == homeops ]]; then
  echo "▸ Uploaded files"
  dir=$(tar -tzf "$files" | head -1 | cut -d/ -f1)   # seaweed or files
  systemctl stop homeops-seaweedfs 2>/dev/null || true
  rm -rf "/var/lib/homeops/$dir.before-restore"
  [[ -d /var/lib/homeops/$dir ]] && mv "/var/lib/homeops/$dir" "/var/lib/homeops/$dir.before-restore"
  tar -xzf "$files" -C /var/lib/homeops
  chown -R homeops:homeops "/var/lib/homeops/$dir"
  systemctl start homeops-seaweedfs 2>/dev/null || true
  echo "  previous files kept in /var/lib/homeops/$dir.before-restore (delete once happy)"
fi
echo "✔ Restored $(basename "$sql") into $db"
