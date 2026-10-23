#!/usr/bin/env bash
# Native install of HomeOps on Ubuntu/Debian with systemd, no Docker.
#
#   sudo ./infra/native/install.sh [--storage seaweedfs|disk] [--port 8090] [--no-tunnel] [--prebuilt]
#
# --prebuilt skips the build (Go/Node not needed on the server): it expects
# .local/homeops and frontend/dist next to infra/, as made by `make bundle`.
#
# Reads settings from the repo's .env (HOMEOPS_PUBLIC_URL, HOMEOPS_VAPID_EMAIL,
# POSTGRES_PASSWORD, S3_ACCESS_KEY, S3_SECRET_KEY, TUNNEL_TOKEN). Safe to re-run:
# it updates the binaries and config and restarts services; data is kept.
#
# Layout:  /opt/homeops         app binary, web app, backup script, weed
#          /etc/homeops         homeops.env, s3.json (root:homeops, 0640)
#          /var/lib/homeops     seaweed/ (or files/), backups/
set -euo pipefail

WEED_VERSION=4.48
WEED_SHA256=4a7d108384d044d95212d1342cdda9533fa55842c1c9b41f606ca3c8a9561124
CLOUDFLARED_VERSION=2026.9.3
CLOUDFLARED_SHA256=bc073ef293d504cf5ac533bd0aa1c824ef6b4f358765ccaa6628a8a95cacb4b7

STORAGE=seaweedfs
TUNNEL=1
PORT=8080
PREBUILT=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --storage) STORAGE=$2; shift 2 ;;
    --no-tunnel) TUNNEL=0; shift ;;
    --port) PORT=$2; shift 2 ;;
    --prebuilt) PREBUILT=1; shift ;;
    *) echo "unknown option: $1"; exit 2 ;;
  esac
done
[[ "$STORAGE" == seaweedfs || "$STORAGE" == disk ]] || { echo "--storage must be seaweedfs or disk"; exit 2; }
[[ $EUID -eq 0 ]] || { echo "Run with sudo: sudo $0 $*"; exit 1; }
BUILD_USER=${SUDO_USER:-}
[[ -n "$BUILD_USER" && "$BUILD_USER" != root ]] || { echo "Run via sudo from your normal user account (the build runs as you)."; exit 1; }

REPO=$(cd "$(dirname "$0")/../.." && pwd)
step() { echo; echo "▸ $*"; }

# --- settings from .env (simple KEY=value lines; comments ignored) ---
declare -A CFG
if [[ -f "$REPO/.env" ]]; then
  while IFS= read -r line; do
    line=${line%%#*}
    [[ "$line" =~ ^[[:space:]]*([A-Z0-9_]+)=(.*)$ ]] || continue
    v=${BASH_REMATCH[2]}
    v=${v%"${v##*[![:space:]]}"}
    CFG[${BASH_REMATCH[1]}]=$v
  done < "$REPO/.env"
fi
rand() { head -c "$1" /dev/urandom | od -An -tx1 | tr -d ' \n'; }
PUBLIC_URL=${CFG[HOMEOPS_PUBLIC_URL]:-}
if [[ -z "$PUBLIC_URL" || "$PUBLIC_URL" == *example.com* ]]; then
  PUBLIC_URL=http://localhost:$PORT
  echo "Note: HOMEOPS_PUBLIC_URL not set in .env; using $PUBLIC_URL (local only)."
fi
VAPID_EMAIL=${CFG[HOMEOPS_VAPID_EMAIL]:-admin@localhost}
DB_PASSWORD=${CFG[POSTGRES_PASSWORD]:-$(rand 24)}
S3_ACCESS=${CFG[S3_ACCESS_KEY]:-$(rand 16)}
S3_SECRET=${CFG[S3_SECRET_KEY]:-$(rand 32)}
TUNNEL_TOKEN=${CFG[TUNNEL_TOKEN]:-}
[[ "$DB_PASSWORD" =~ ^[A-Za-z0-9]+$ ]] || { echo "POSTGRES_PASSWORD must be letters and digits only"; exit 1; }

# --- build as the normal user ---
if [[ $PREBUILT -eq 1 ]]; then
  [[ -x "$REPO/.local/homeops" && -f "$REPO/frontend/dist/index.html" ]] || { echo "--prebuilt: missing .local/homeops or frontend/dist"; exit 1; }
else
step "Building as $BUILD_USER"
sudo -u "$BUILD_USER" -H bash -lc "
  set -e
  export PATH=\$HOME/.local/go/bin:\$PATH
  command -v go >/dev/null || { echo 'Go 1.27+ not found (install to ~/.local/go or system-wide)'; exit 1; }
  command -v npm >/dev/null || { echo 'Node.js 24+ not found'; exit 1; }
  cd '$REPO/frontend' && { [ -d node_modules ] || npm ci --no-audit --no-fund; } && npm run build >/dev/null
  cd '$REPO/backend' && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o '$REPO/.local/homeops' ./cmd/homeops
"
fi

# --- packages, user, directories ---
step "Installing PostgreSQL (if needed)"
if ! command -v pg_ctlcluster >/dev/null; then
  apt-get update -q
  DEBIAN_FRONTEND=noninteractive apt-get install -y -q postgresql postgresql-client curl ca-certificates
fi
systemctl enable --now postgresql >/dev/null

step "Creating service user and directories"
id homeops >/dev/null 2>&1 || useradd --system --home-dir /var/lib/homeops --shell /usr/sbin/nologin homeops
install -d -m 0755 /opt/homeops /opt/homeops/bin
install -d -m 0750 -o homeops -g homeops /var/lib/homeops /var/lib/homeops/backups
install -d -m 0750 -o root -g homeops /etc/homeops

step "Database"
sudo -u postgres psql -q -v ON_ERROR_STOP=1 -v pw="$DB_PASSWORD" <<'SQL'
SELECT 'CREATE ROLE homeops LOGIN' WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'homeops') \gexec
ALTER ROLE homeops WITH LOGIN PASSWORD :'pw';
SELECT 'CREATE DATABASE homeops OWNER homeops' WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'homeops') \gexec
SQL

step "Installing app to /opt/homeops"
install -m 0755 "$REPO/.local/homeops" /opt/homeops/homeops.new && mv /opt/homeops/homeops.new /opt/homeops/homeops
rm -rf /opt/homeops/web.new && cp -r "$REPO/frontend/dist" /opt/homeops/web.new && rm -rf /opt/homeops/web && mv /opt/homeops/web.new /opt/homeops/web
install -m 0755 "$REPO/infra/native/backup.sh" /opt/homeops/backup.sh
install -m 0644 "$REPO/README.md" /opt/homeops/README.md

if [[ "$STORAGE" == seaweedfs ]]; then
  step "SeaweedFS $WEED_VERSION"
  if ! /opt/homeops/bin/weed version 2>/dev/null | grep -q " $WEED_VERSION "; then
    tmp=$(mktemp -d)
    curl -fsSL --speed-limit 20000 --speed-time 60 -o "$tmp/weed.tgz" "https://github.com/seaweedfs/seaweedfs/releases/download/$WEED_VERSION/linux_amd64.tar.gz"
    echo "$WEED_SHA256  $tmp/weed.tgz" | sha256sum -c --quiet
    tar -xzf "$tmp/weed.tgz" -C "$tmp" weed
    install -m 0755 "$tmp/weed" /opt/homeops/bin/weed
    rm -rf "$tmp"
  fi
  install -d -m 0750 -o homeops -g homeops /var/lib/homeops/seaweed
  umask 027
  cat > /etc/homeops/s3.json <<JSON
{"identities":[{"name":"homeops","credentials":[{"accessKey":"$S3_ACCESS","secretKey":"$S3_SECRET"}],"actions":["Admin","Read","Write","List","Tagging"]}]}
JSON
  chown root:homeops /etc/homeops/s3.json && chmod 0640 /etc/homeops/s3.json
  STORAGE_ENV="HOMEOPS_S3_ENDPOINT=127.0.0.1:8333
HOMEOPS_S3_BUCKET=homeops
HOMEOPS_S3_ACCESS_KEY=$S3_ACCESS
HOMEOPS_S3_SECRET_KEY=$S3_SECRET"
else
  install -d -m 0750 -o homeops -g homeops /var/lib/homeops/files
  STORAGE_ENV="HOMEOPS_DATA_DIR=/var/lib/homeops/files"
fi

step "Writing /etc/homeops/homeops.env"
umask 027
cat > /etc/homeops/homeops.env <<ENV
# Managed by infra/native/install.sh — re-run it to change settings.
DATABASE_URL=postgres://homeops:$DB_PASSWORD@127.0.0.1:5432/homeops?sslmode=disable
HOMEOPS_ADDR=127.0.0.1:$PORT
HOMEOPS_PUBLIC_URL=$PUBLIC_URL
HOMEOPS_STATIC_DIR=/opt/homeops/web
HOMEOPS_VAPID_EMAIL=$VAPID_EMAIL
$STORAGE_ENV
BACKUP_KEEP_DAYS=${CFG[BACKUP_KEEP_DAYS]:-30}
ENV
chown root:homeops /etc/homeops/homeops.env && chmod 0640 /etc/homeops/homeops.env

step "systemd units"
install -m 0644 "$REPO/infra/native/homeops.service" /etc/systemd/system/homeops.service
install -m 0644 "$REPO/infra/native/homeops-backup.service" /etc/systemd/system/homeops-backup.service
install -m 0644 "$REPO/infra/native/homeops-backup.timer" /etc/systemd/system/homeops-backup.timer
if [[ "$STORAGE" == seaweedfs ]]; then
  install -m 0644 "$REPO/infra/native/homeops-seaweedfs.service" /etc/systemd/system/homeops-seaweedfs.service
fi
systemctl daemon-reload
if [[ "$STORAGE" == seaweedfs ]]; then
  systemctl enable homeops-seaweedfs >/dev/null
  systemctl restart homeops-seaweedfs
fi
systemctl enable homeops homeops-backup.timer >/dev/null
systemctl restart homeops
systemctl start homeops-backup.timer

if [[ $TUNNEL -eq 1 && -n "$TUNNEL_TOKEN" ]]; then
  step "Cloudflare Tunnel (cloudflared $CLOUDFLARED_VERSION)"
  if ! dpkg-query -W -f='${Version}' cloudflared 2>/dev/null | grep -q "^$CLOUDFLARED_VERSION"; then
    tmp=$(mktemp -d)
    # A copy shipped in the bundle avoids slow GitHub downloads on some home connections.
    if [[ -f "$REPO/.local/cloudflared.deb" ]]; then
      cp "$REPO/.local/cloudflared.deb" "$tmp/cloudflared.deb"
    else
      curl -fsSL --speed-limit 20000 --speed-time 60 -o "$tmp/cloudflared.deb" "https://github.com/cloudflare/cloudflared/releases/download/$CLOUDFLARED_VERSION/cloudflared-linux-amd64.deb"
    fi
    echo "$CLOUDFLARED_SHA256  $tmp/cloudflared.deb" | sha256sum -c --quiet
    apt-get install -y -q "$tmp/cloudflared.deb"
    rm -rf "$tmp"
  fi
  if ! systemctl list-unit-files cloudflared.service >/dev/null 2>&1 || ! systemctl is-enabled cloudflared >/dev/null 2>&1; then
    cloudflared service install "$TUNNEL_TOKEN"
  fi
  systemctl restart cloudflared
elif [[ $TUNNEL -eq 1 ]]; then
  echo "No TUNNEL_TOKEN in .env — skipping Cloudflare Tunnel (re-run after adding it)."
fi

step "Waiting for HomeOps"
for _ in $(seq 1 60); do
  curl -fs http://127.0.0.1:$PORT/api/health >/dev/null && break
  sleep 1
done
if curl -fs http://127.0.0.1:$PORT/api/health >/dev/null; then
  echo
  echo "✔ HomeOps is running."
  echo "   Local:   http://localhost:$PORT  (on the server; listens on localhost only)"
  [[ "$PUBLIC_URL" == https://* ]] && echo "   Public:  $PUBLIC_URL  (via Cloudflare Tunnel)"
  echo "   Status:  systemctl status homeops      Logs: journalctl -u homeops -f"
  echo "   Backups: /var/lib/homeops/backups (nightly 02:30; run now: sudo systemctl start homeops-backup)"
else
  echo "HomeOps didn't come up. Check: journalctl -u homeops -n 50"
  exit 1
fi
