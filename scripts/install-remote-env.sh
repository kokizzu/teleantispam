#!/usr/bin/env bash
set -euo pipefail

source scripts/load-local-env.sh

host="${TELEANTISPAM_DEPLOY_HOST:-}"
if [[ -z "$host" ]]; then
  echo "missing TELEANTISPAM_DEPLOY_HOST" >&2
  exit 1
fi

ssh_opts=(-x -o BatchMode=yes -o ConnectTimeout=15)
scp_opts=(-o BatchMode=yes -o ConnectTimeout=15)

if [[ -z "${TELEGRAM_BOT_TOKEN:-}" ]]; then
  echo "missing TELEGRAM_BOT_TOKEN" >&2
  exit 1
fi

tmp_env="$(mktemp)"
trap 'rm -f "$tmp_env"' EXIT
TELEANTISPAM_ENV_OUTPUT="$tmp_env" bash scripts/render-env.sh

remote_tmp="/tmp/teleantispam-env.$$"
ssh "${ssh_opts[@]}" "$host" "mkdir -p '$remote_tmp'"
scp "${scp_opts[@]}" "$tmp_env" "$host:$remote_tmp/teleantispam.env"
ssh "${ssh_opts[@]}" "$host" "set -euo pipefail
id -u teleantispam >/dev/null 2>&1 || useradd --system --home-dir /var/lib/teleantispam --shell /usr/sbin/nologin teleantispam
install -o root -g teleantispam -m 0750 -d /etc/teleantispam
install -o root -g teleantispam -m 0640 '$remote_tmp/teleantispam.env' /etc/teleantispam/teleantispam.env
rm -rf '$remote_tmp'
echo 'installed /etc/teleantispam/teleantispam.env'"
