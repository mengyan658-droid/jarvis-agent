#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

"$ROOT_DIR/scripts/stop.sh"
"$ROOT_DIR/scripts/start.sh"
