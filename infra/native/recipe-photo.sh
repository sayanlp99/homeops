#!/usr/bin/env bash
# Set a recipe's photo from an image file on the server, processed like an upload.
#   sudo ./infra/native/recipe-photo.sh <household> <recipe title> <image file>
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "Run with sudo"; exit 1; }
[[ $# -eq 3 ]] || { echo "usage: recipe-photo.sh <household> <recipe title> <image file>"; exit 1; }
set -a; . /etc/homeops/homeops.env; set +a
/opt/homeops/homeops recipe-photo "$@"
