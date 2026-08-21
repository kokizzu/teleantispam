#!/usr/bin/env bash
set -euo pipefail

source scripts/load-local-env.sh

host="${TELEANTISPAM_DEPLOY_HOST:-}"
if [[ -z "$host" ]]; then
  echo "missing TELEANTISPAM_DEPLOY_HOST" >&2
  exit 1
fi

ssh_opts=(-x -o BatchMode=yes -o ConnectTimeout=15)

ssh "${ssh_opts[@]}" "$host" 'set -euo pipefail
systemctl stop teleantispam.service
trap "systemctl start teleantispam.service" EXIT
systemd-run --wait --collect --pipe \
  --property=User=teleantispam \
  --property=Group=teleantispam \
  --property=WorkingDirectory=/var/lib/teleantispam \
  --property=EnvironmentFile=/etc/teleantispam/teleantispam.env \
  /usr/local/bin/teleantispam -retry-failed-moderation
systemctl start teleantispam.service
trap - EXIT
systemctl --no-pager --full status teleantispam.service
'
