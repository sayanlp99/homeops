#!/usr/bin/env bash
# Bring photos saved before the current sizes (2048px, 640px thumbnail) up to date:
# recipes imported from a link get a bigger copy from the source when it's the same
# picture; every other photo gets a sharper thumbnail. Safe to re-run.
#   sudo ./infra/native/refresh-photos.sh [--dry-run]
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "Run with sudo"; exit 1; }
set -a; . /etc/homeops/homeops.env; set +a
/opt/homeops/homeops photos "$@"
