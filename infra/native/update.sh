#!/usr/bin/env bash
# Rebuild and redeploy the native install after pulling new code:
#   sudo ./infra/native/update.sh [--prebuilt]   (--prebuilt: bundle from `make bundle`, no build on the server)
# Database migrations run automatically when the new binary starts.
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "Run with sudo: sudo $0"; exit 1; }
BUILD_USER=${SUDO_USER:?run via sudo from your normal user}
REPO=$(cd "$(dirname "$0")/../.." && pwd)

if [[ "${1:-}" == --prebuilt ]]; then
  [[ -x "$REPO/.local/homeops" && -f "$REPO/frontend/dist/index.html" ]] || { echo "--prebuilt: missing .local/homeops or frontend/dist"; exit 1; }
else
echo "▸ Building as $BUILD_USER"
sudo -u "$BUILD_USER" -H bash -lc "
  set -e
  export PATH=\$HOME/.local/go/bin:\$PATH
  cd '$REPO/frontend' && npm ci --no-audit --no-fund >/dev/null && npm run build >/dev/null
  cd '$REPO/backend' && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o '$REPO/.local/homeops' ./cmd/homeops
"
fi
echo "▸ Installing"
install -m 0755 "$REPO/.local/homeops" /opt/homeops/homeops.new && mv /opt/homeops/homeops.new /opt/homeops/homeops
rm -rf /opt/homeops/web.new && cp -r "$REPO/frontend/dist" /opt/homeops/web.new && rm -rf /opt/homeops/web && mv /opt/homeops/web.new /opt/homeops/web
install -m 0755 "$REPO/infra/native/backup.sh" /opt/homeops/backup.sh
for u in homeops.service homeops-backup.service homeops-backup.timer homeops-seaweedfs.service; do
  [[ -f /etc/systemd/system/$u ]] && install -m 0644 "$REPO/infra/native/$u" "/etc/systemd/system/$u"
done
systemctl daemon-reload
systemctl restart homeops
PORT=$(sed -n 's/^HOMEOPS_ADDR=.*:\([0-9]*\)$/\1/p' /etc/homeops/homeops.env)
for _ in $(seq 1 30); do curl -fs "http://127.0.0.1:${PORT:-8080}/api/health" >/dev/null && { echo "✔ Updated and running"; exit 0; }; sleep 1; done
echo "HomeOps didn't come back up. Check: journalctl -u homeops -n 50"; exit 1
