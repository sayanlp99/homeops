#!/usr/bin/env bash
# Add a built-in recipe pack to someone's household from the server, the same as
# Recipes → Recipe packs → Add. Recipes whose title the household already has are skipped.
#   sudo ./infra/native/import-pack.sh <username or email> <pack> [household]
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "Run with sudo"; exit 1; }
[[ $# -ge 2 ]] || { echo "usage: import-pack.sh <username or email> <pack> [household]   (pack: bengali-home)"; exit 1; }
set -a; . /etc/homeops/homeops.env; set +a
/opt/homeops/homeops pack "$@"
