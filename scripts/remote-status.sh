#!/usr/bin/env bash
set -euo pipefail

source scripts/load-local-env.sh

host="${TELEANTISPAM_DEPLOY_HOST:-}"
if [[ -z "$host" ]]; then
  echo "missing TELEANTISPAM_DEPLOY_HOST" >&2
  exit 1
fi

ssh_opts=(-x -o BatchMode=yes -o ConnectTimeout=15)

ssh "${ssh_opts[@]}" "$host" 'set -e
echo "== binary =="
if [ -x /usr/local/bin/teleantispam ]; then
  /usr/local/bin/teleantispam -version || true
  ls -l /usr/local/bin/teleantispam
else
  echo "missing /usr/local/bin/teleantispam"
fi

echo "== service =="
systemctl show teleantispam.service \
  -p User -p Group -p LoadState -p ActiveState -p SubState -p UnitFileState -p FragmentPath \
  --no-pager || true

echo "== admin check timer =="
systemctl show teleantispam-admin-check.timer \
  -p LoadState -p ActiveState -p SubState -p UnitFileState -p NextElapseUSecRealtime -p LastTriggerUSecRealtime \
  --no-pager || true

echo "== admin check status =="
if [ -s /var/lib/teleantispam/gophers-admin-status.json ]; then
  cat /var/lib/teleantispam/gophers-admin-status.json
else
  echo "missing /var/lib/teleantispam/gophers-admin-status.json"
fi

echo "== env =="
if test -s /etc/teleantispam/teleantispam.env && grep -q "^TELEGRAM_BOT_TOKEN=." /etc/teleantispam/teleantispam.env; then
  echo "TELEGRAM_BOT_TOKEN=present"
else
  echo "TELEGRAM_BOT_TOKEN=missing"
fi

echo "== hardening =="
if systemctl cat teleantispam.service >/dev/null 2>&1; then
  systemctl cat teleantispam.service --no-pager \
    | sed -n "/^\[Service\]/,/^\[Install\]/p" \
    | grep -E "^(User|Group|NoNewPrivileges|ProtectSystem|ProtectHome|ReadWritePaths|CapabilityBoundingSet|PrivateTmp)=" || true
else
  echo "service not installed"
fi

echo "== recent logs =="
journalctl -u teleantispam.service -n 30 --no-pager || true

echo "== recent admin check logs =="
journalctl -u teleantispam-admin-check.service -n 20 --no-pager || true
'
