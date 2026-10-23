#!/usr/bin/env bash
# Deploys the Email Worker and copies the bridge token from the server into it, so the
# token is never printed. Needs `npx wrangler@4.148.0 login` once.
#   ./infra/native/mail/deploy-worker.sh [ssh-host]
set -euo pipefail
cd "$(dirname "$0")"
HOST=${1:-sayan@10.167.183.231}
W="npx --yes wrangler@4.148.0"
$W deploy
read -rsp "sudo password on $HOST (to read the bridge token): " PW; echo
printf '%s\n' "$PW" | ssh "$HOST" "sudo -S -p '' sed -n 's/^MAILBRIDGE_TOKEN=//p' /etc/homeops/mail.env" | $W secret put BRIDGE_TOKEN
echo "✔ Worker deployed. Now: Email Routing → Routing rules → Catch-all → Send to Worker → homeops-mail"
