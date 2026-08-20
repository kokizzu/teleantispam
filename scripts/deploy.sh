#!/usr/bin/env bash
set -euo pipefail

host="${TELEANTISPAM_DEPLOY_HOST:-DEPLOY_HOST}"
binary="${TELEANTISPAM_BINARY:-bin/teleantispam}"
remote_tmp="/tmp/teleantispam.$$"
install_only="${TELEANTISPAM_INSTALL_ONLY:-false}"
ssh_opts=(-x -o BatchMode=yes -o ConnectTimeout=15)
scp_opts=(-o BatchMode=yes -o ConnectTimeout=15)

source scripts/load-local-env.sh

if [[ ! -x "$binary" ]]; then
  echo "missing binary: $binary" >&2
  exit 1
fi

if [[ -z "${TELEGRAM_BOT_TOKEN:-}" && "$install_only" != "true" ]]; then
  if ! ssh "${ssh_opts[@]}" "$host" "test -s /etc/teleantispam/teleantispam.env && grep -q '^TELEGRAM_BOT_TOKEN=.' /etc/teleantispam/teleantispam.env"; then
    echo "missing TELEGRAM_BOT_TOKEN locally and no populated remote /etc/teleantispam/teleantispam.env exists" >&2
    exit 1
  fi
fi

ssh "${ssh_opts[@]}" "$host" "mkdir -p '$remote_tmp'"
scp "${scp_opts[@]}" "$binary" "$host:$remote_tmp/teleantispam"
scp "${scp_opts[@]}" deploy/teleantispam.service "$host:$remote_tmp/teleantispam.service"

if [[ -n "${TELEGRAM_BOT_TOKEN:-}" ]]; then
  tmp_env="$(mktemp)"
  trap 'rm -f "$tmp_env"' EXIT
  TELEANTISPAM_ENV_OUTPUT="$tmp_env" bash scripts/render-env.sh
  scp "${scp_opts[@]}" "$tmp_env" "$host:$remote_tmp/teleantispam.env"
fi

ssh "${ssh_opts[@]}" "$host" "set -euo pipefail
id -u teleantispam >/dev/null 2>&1 || useradd --system --home-dir /var/lib/teleantispam --shell /usr/sbin/nologin teleantispam
install -o root -g root -m 0755 '$remote_tmp/teleantispam' /usr/local/bin/teleantispam
install -o root -g root -m 0644 '$remote_tmp/teleantispam.service' /etc/systemd/system/teleantispam.service
install -o teleantispam -g teleantispam -m 0750 -d /var/lib/teleantispam
install -o root -g teleantispam -m 0750 -d /etc/teleantispam
if [ -f '$remote_tmp/teleantispam.env' ]; then
  install -o root -g teleantispam -m 0640 '$remote_tmp/teleantispam.env' /etc/teleantispam/teleantispam.env
elif [ ! -f /etc/teleantispam/teleantispam.env ] && [ '$install_only' != 'true' ]; then
  echo 'missing /etc/teleantispam/teleantispam.env and TELEGRAM_BOT_TOKEN was not provided' >&2
  exit 1
fi
systemctl daemon-reload
if command -v systemd-analyze >/dev/null 2>&1; then
  systemd-analyze verify /etc/systemd/system/teleantispam.service
fi
if [ '$install_only' = 'true' ]; then
  echo 'teleantispam installed in install-only mode; service not enabled or started'
  rm -rf '$remote_tmp'
  exit 0
fi
systemctl enable --now teleantispam.service
systemctl restart teleantispam.service
systemctl --no-pager --full status teleantispam.service
rm -rf '$remote_tmp'"
