#!/usr/bin/env bash
set -euo pipefail

source scripts/load-local-env.sh

"${GO:-go}" run ./cmd/teleantispam "$@"
