#!/bin/sh
# Nightly Postgres backup loop, run by the `backup` compose service.
# Writes /backups/homeops-YYYY-MM-DD.sql.gz at $BACKUP_AT local time and keeps
# $BACKUP_KEEP_DAYS days. `docker compose exec backup /bin/sh /backup.sh now`
# takes a backup immediately.
set -eu

backup() {
  file="/backups/homeops-$(date +%F).sql.gz"
  tmp="$file.partial"
  pg_dump --no-owner --clean --if-exists | gzip -9 > "$tmp"
  mv "$tmp" "$file"
  find /backups -name 'homeops-*.sql.gz' -mtime +"${BACKUP_KEEP_DAYS:-30}" -delete
  echo "$(date '+%F %T') backup written: $file ($(du -h "$file" | cut -f1))"
}

if [ "${1:-}" = "now" ]; then
  backup
  exit 0
fi

echo "backup service: daily at ${BACKUP_AT:-02:30}, keeping ${BACKUP_KEEP_DAYS:-30} days"
at=$(echo "${BACKUP_AT:-02:30}" | tr -d :)
while true; do
  # HHMM compared as integers; the file check makes this safe across restarts.
  if [ "$(date +%H%M)" -ge "$at" ] && [ ! -f "/backups/homeops-$(date +%F).sql.gz" ]; then
    backup || echo "$(date '+%F %T') backup FAILED"
  fi
  sleep 60
done
