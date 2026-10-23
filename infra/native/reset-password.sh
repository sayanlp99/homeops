#!/usr/bin/env bash
# Set a new password for a HomeOps user from the server, e.g. when they're locked out
# and password-reset email isn't set up. Signs them out everywhere.
#   sudo ./infra/native/reset-password.sh <username or email>
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "Run with sudo"; exit 1; }
user=${1:?usage: reset-password.sh <username or email>}
read -rsp "New password for $user (8+ characters): " pw; echo
read -rsp "Again: " pw2; echo
[[ $pw == "$pw2" ]] || { echo "Passwords don't match"; exit 1; }
# Root can read the app's settings (database URL) directly.
set -a; . /etc/homeops/homeops.env; set +a
printf '%s\n' "$pw" | /opt/homeops/homeops passwd "$user"
