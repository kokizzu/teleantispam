#!/usr/bin/env bash
set -euo pipefail

host="${TELEANTISPAM_DEPLOY_HOST:-DEPLOY_HOST}"
binary="${TELEANTISPAM_BINARY:-bin/teleantispam}"
remote_tmp="/tmp/teleantispam.$$"

if [[ ! -x "$binary" ]]; then
  echo "missing binary: $binary" >&2
  exit 1
fi

ssh "$host" "mkdir -p '$remote_tmp'"
scp "$binary" "$host:$remote_tmp/teleantispam"
scp deploy/teleantispam.service "$host:$remote_tmp/teleantispam.service"

if [[ -n "${TELEGRAM_BOT_TOKEN:-}" ]]; then
  tmp_env="$(mktemp)"
  trap 'rm -f "$tmp_env"' EXIT
  sed "s|^TELEGRAM_BOT_TOKEN=.*|TELEGRAM_BOT_TOKEN=${TELEGRAM_BOT_TOKEN}|" deploy/teleantispam.env.example > "$tmp_env"
  scp "$tmp_env" "$host:$remote_tmp/teleantispam.env"
fi

ssh "$host" "set -euo pipefail
id -u teleantispam >/dev/null 2>&1 || useradd --system --home-dir /var/lib/teleantispam --shell /usr/sbin/nologin teleantispam
install -o root -g root -m 0755 '$remote_tmp/teleantispam' /usr/local/bin/teleantispam
install -o root -g root -m 0644 '$remote_tmp/teleantispam.service' /etc/systemd/system/teleantispam.service
install -o teleantispam -g teleantispam -m 0750 -d /var/lib/teleantispam
install -o root -g teleantispam -m 0750 -d /etc/teleantispam
if [ -f '$remote_tmp/teleantispam.env' ]; then
  install -o root -g teleantispam -m 0640 '$remote_tmp/teleantispam.env' /etc/teleantispam/teleantispam.env
elif [ ! -f /etc/teleantispam/teleantispam.env ]; then
  echo 'missing /etc/teleantispam/teleantispam.env and TELEGRAM_BOT_TOKEN was not provided' >&2
  exit 1
fi
systemctl daemon-reload
systemctl enable --now teleantispam.service
systemctl restart teleantispam.service
systemctl --no-pager --full status teleantispam.service
rm -rf '$remote_tmp'"
