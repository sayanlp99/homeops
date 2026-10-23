#!/usr/bin/env bash
# Run HomeOps on this machine without Docker or Cloudflare, for testing.
#
#   ./run-local.sh            build if needed, start Postgres + server on :8080
#   ./run-local.sh --reset    same, but wipe local data first
#
# Data lives in backend/.devdata (database + uploaded photos). Open
# http://localhost:8080 on this PC for every feature. Other devices on your Wi-Fi can
# use http://<this-pc-ip>:8080, but browsers only allow passkeys, push notifications
# and offline mode on https or localhost, so those are unavailable there.
set -euo pipefail
cd "$(dirname "$0")"
ROOT=$PWD
BIN=$ROOT/.local
DATA=$ROOT/backend/.devdata
PORT=${HOMEOPS_PORT:-8080}
export PATH="$HOME/.local/go/bin:$PATH"

command -v go >/dev/null || { echo "Go 1.27+ is required (https://go.dev/dl)"; exit 1; }
command -v npm >/dev/null || { echo "Node.js 24+ is required"; exit 1; }

if [[ "${1:-}" == "--reset" ]]; then
  echo "Wiping local data in $DATA/pg and $DATA/files"
  rm -rf "$DATA/pg" "$DATA/runtime" "$DATA/files"
fi

mkdir -p "$BIN"
echo "▸ Building web app"
(cd frontend && { [[ -d node_modules ]] || npm ci --no-audit --no-fund; } && npm run build >/dev/null)
echo "▸ Building server"
(cd backend && go build -o "$BIN/homeops" ./cmd/homeops && go build -o "$BIN/devdb" ./cmd/devdb)

pids=()
cleanup() {
  echo
  echo "▸ Stopping"
  for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

for port in 5433 "$PORT"; do
  if (echo >/dev/tcp/127.0.0.1/$port) 2>/dev/null; then
    echo "Port $port is already in use (another HomeOps or Postgres still running?). Stop it and retry."
    exit 1
  fi
done

echo "▸ Starting database (embedded Postgres on :5433)"
(cd backend && exec "$BIN/devdb") >"$BIN/devdb.log" 2>&1 &
pids+=($!)
for _ in $(seq 1 120); do
  (echo >/dev/tcp/127.0.0.1/5433) 2>/dev/null && break
  sleep 0.5
done
(echo >/dev/tcp/127.0.0.1/5433) 2>/dev/null || { echo "Database didn't start; see $BIN/devdb.log"; exit 1; }

LAN_IP=$(hostname -I 2>/dev/null | awk '{print $1}')
echo "▸ Starting HomeOps"
echo
echo "   This PC:        http://localhost:$PORT"
[[ -n "$LAN_IP" ]] && echo "   Other devices:  http://$LAN_IP:$PORT   (no passkeys/push/offline over plain http)"
echo "   First visit creates the admin account. Ctrl+C to stop."
echo
DATABASE_URL="postgres://homeops:homeops@localhost:5433/homeops?sslmode=disable" \
HOMEOPS_ADDR=":$PORT" \
HOMEOPS_PUBLIC_URL="http://localhost:$PORT" \
HOMEOPS_STATIC_DIR="$ROOT/frontend/dist" \
HOMEOPS_DATA_DIR="$DATA/files" \
HOMEOPS_VAPID_EMAIL="admin@localhost" \
HOMEOPS_DEV=1 \
"$BIN/homeops" &
pids+=($!)
wait "${pids[-1]}"
