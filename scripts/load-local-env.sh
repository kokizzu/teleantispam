#!/usr/bin/env bash
set -euo pipefail

if [[ -f .env.override ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env.override
  set +a
fi
