#!/usr/bin/env bash
set -euo pipefail

source scripts/load-local-env.sh

host="${TELEANTISPAM_DEPLOY_HOST:-}"
mode="${1:-}"

if [[ -z "$host" ]]; then
  echo "missing TELEANTISPAM_DEPLOY_HOST" >&2
  exit 1
fi
if [[ "$mode" != "plan" && "$mode" != "apply" ]]; then
  echo "usage: remote-manual-moderate.sh {plan|apply}" >&2
  exit 1
fi

python3 scripts/remote_manual_moderation.py "$mode" "$host"
