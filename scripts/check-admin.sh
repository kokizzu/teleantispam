#!/usr/bin/env bash
set -euo pipefail

source scripts/load-local-env.sh

binary="${BINARY:-bin/teleantispam}"
exec "$binary" -check-admin "$@"
