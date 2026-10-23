#!/bin/sh
# Nightly HomeOps backup for the native (systemd) install. Writes to
# /var/lib/homeops/backups and keeps BACKUP_KEEP_DAYS (default 30) days.
set -eu
dir=/var/lib/homeops/backups
day=$(date +%F)
umask 077
pg_dump --no-owner --clean --if-exists "$DATABASE_URL" | gzip -9 > "$dir/homeops-$day.sql.gz.partial"
mv "$dir/homeops-$day.sql.gz.partial" "$dir/homeops-$day.sql.gz"
if [ -d /var/lib/homeops/seaweed ]; then
  tar -czf "$dir/files-$day.tar.gz.partial" -C /var/lib/homeops seaweed && mv "$dir/files-$day.tar.gz.partial" "$dir/files-$day.tar.gz"
elif [ -d /var/lib/homeops/files ]; then
  tar -czf "$dir/files-$day.tar.gz.partial" -C /var/lib/homeops files && mv "$dir/files-$day.tar.gz.partial" "$dir/files-$day.tar.gz"
fi
find "$dir" \( -name 'homeops-*.sql.gz' -o -name 'files-*.tar.gz' \) -mtime +"${BACKUP_KEEP_DAYS:-30}" -delete
echo "backup done: $(ls -1 "$dir" | grep "$day" | tr '\n' ' ')"
