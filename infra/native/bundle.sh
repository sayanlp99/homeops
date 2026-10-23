#!/usr/bin/env bash
# Build a self-contained deploy bundle (no Go/Node needed on the server):
#   ./infra/native/bundle.sh [out.tar.gz]
# Then on the server:  tar xzf homeops-bundle.tar.gz && cd homeops && sudo ./infra/native/install.sh --prebuilt
# Put the server's .env inside the bundle directory (next to README.md) before installing.
set -euo pipefail
REPO=$(cd "$(dirname "$0")/../.." && pwd)
OUT=${1:-$REPO/.local/homeops-bundle.tar.gz}
export PATH="$HOME/.local/go/bin:$PATH"
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
echo "▸ Building web app"
(cd "$REPO/frontend" && { [[ -d node_modules ]] || npm ci --no-audit --no-fund; } && npm run build >/dev/null)
echo "▸ Building server (linux/amd64, static)"
mkdir -p "$STAGE/homeops/.local" "$STAGE/homeops/frontend" "$STAGE/homeops/infra"
(cd "$REPO/backend" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o "$STAGE/homeops/.local/homeops" ./cmd/homeops)
(cd "$REPO/backend" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o "$STAGE/homeops/.local/mailbridge" ./cmd/mailbridge)
# Release downloads already verified on this machine travel with the bundle (the
# install scripts re-check their SHA-256), so the server needn't reach GitHub.
for f in stalwart-x86_64-unknown-linux-gnu.tar.gz stalwart-cli-x86_64-unknown-linux-gnu.tar.xz roundcubemail-1.7.4-complete.tar.gz cloudflared.deb; do
  [[ -f "$REPO/.local/$f" ]] && cp "$REPO/.local/$f" "$STAGE/homeops/.local/"
done
cp -r "$REPO/frontend/dist" "$STAGE/homeops/frontend/dist"
cp -r "$REPO/infra/native" "$STAGE/homeops/infra/native"
cp "$REPO/README.md" "$STAGE/homeops/"
mkdir -p "$(dirname "$OUT")"
tar -czf "$OUT" -C "$STAGE" homeops
echo "✔ $OUT ($(du -h "$OUT" | cut -f1))"
