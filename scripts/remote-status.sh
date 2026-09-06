#!/usr/bin/env bash
set -euo pipefail

source scripts/load-local-env.sh

host="${TELEANTISPAM_DEPLOY_HOST:-}"
if [[ -z "$host" ]]; then
  echo "missing TELEANTISPAM_DEPLOY_HOST" >&2
  exit 1
fi

ssh_opts=(-x -o BatchMode=yes -o ConnectTimeout=15)

ssh "${ssh_opts[@]}" "$host" 'bash -s' <<'REMOTE'
set -e
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

echo "== moderation stats =="
if [ -s /var/lib/teleantispam/state.json ]; then
  python3 - <<'PY'
import json
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path

path = Path("/var/lib/teleantispam/state.json")
data = json.loads(path.read_text())

def parse_time(value):
    if not value:
        return None
    if isinstance(value, str):
        try:
            return datetime.fromisoformat(value.replace("Z", "+00:00"))
        except ValueError:
            return None
    return None

def format_time(value):
    if not value:
        return "-"
    if isinstance(value, str):
        if value.startswith("0001-01-01"):
            return "state-start"
        return value
    if value.year == 1:
        return "state-start"
    return value.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")

def snapshot(actions, from_time=None, to_time=None):
    stats = {
        "moderation_actions": 0,
        "bans": 0,
        "deleted_messages": 0,
        "failed_actions": 0,
        "dry_run_actions": 0,
        "retry_actions": 0,
    }
    reasons = defaultdict(lambda: {"moderation_actions": 0, "bans": 0, "deleted_messages": 0})
    for action in actions:
        at = parse_time(action.get("at"))
        if from_time and at and at < from_time:
            continue
        if to_time and at and at >= to_time:
            continue
        stats["moderation_actions"] += 1
        if action.get("banned"):
            stats["bans"] += 1
        stats["deleted_messages"] += int(action.get("deleted_count") or 0)
        if action.get("errors"):
            stats["failed_actions"] += 1
        if action.get("dry_run"):
            stats["dry_run_actions"] += 1
        if action.get("retry"):
            stats["retry_actions"] += 1
        reason = action.get("reason") or "unknown"
        reasons[reason]["moderation_actions"] += 1
        if action.get("banned"):
            reasons[reason]["bans"] += 1
        reasons[reason]["deleted_messages"] += int(action.get("deleted_count") or 0)
    stats["reasons"] = [
        {"reason": reason, **values}
        for reason, values in sorted(
            reasons.items(),
            key=lambda item: (-item[1]["moderation_actions"], item[0]),
        )
    ]
    return stats

def format_reasons(reasons):
    if not reasons:
        return "-"
    return ",".join(
        f"{item.get('reason') or 'unknown'}:{int(item.get('moderation_actions') or 0)}/{int(item.get('bans') or 0)}/{int(item.get('deleted_messages') or 0)}"
        for item in reasons
    )

def print_snapshot(prefix, stats):
    print(
        f"{prefix}_actions={int(stats.get('moderation_actions') or 0)} "
        f"{prefix}_bans={int(stats.get('bans') or 0)} "
        f"{prefix}_deleted={int(stats.get('deleted_messages') or 0)} "
        f"{prefix}_failed={int(stats.get('failed_actions') or 0)} "
        f"{prefix}_dry_run={int(stats.get('dry_run_actions') or 0)} "
        f"{prefix}_retry={int(stats.get('retry_actions') or 0)} "
        f"{prefix}_reasons={format_reasons(stats.get('reasons') or [])}"
    )

chats = data.get("chats") or {}
tracked_chats = sum(1 for users in chats.values() if users)
tracked_users = sum(len(users or {}) for users in chats.values())
observed_messages = sum(
    int((history or {}).get("message_count") or 0)
    for users in chats.values()
    for history in (users or {}).values()
)
actions = data.get("moderation_actions") or []
stats_state = data.get("stats") or {}
last_startup = stats_state.get("last_startup_at") or ""
all_time = snapshot(actions)
previous_run = stats_state.get("previous_run") or {}
if not previous_run:
    previous_run = all_time

print(f"last_startup_at={last_startup or '-'}")
print(f"tracked_chats={tracked_chats} tracked_users={tracked_users} observed_messages={observed_messages}")
print_snapshot("all_time", all_time)
print(
    f"previous_run_from={format_time(previous_run.get('from'))} "
    f"previous_run_to={format_time(previous_run.get('to'))}"
)
print_snapshot("previous_run", previous_run)
PY
else
  echo "missing /var/lib/teleantispam/state.json"
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
journalctl -u teleantispam.service -n 30 -o cat --no-pager \
  | sed -E 's#(https://api\.telegram\.org/bot)[^/[:space:]]+/#\1<redacted>/#g' || true

echo "== recent admin check logs =="
journalctl -u teleantispam-admin-check.service -n 20 -o cat --no-pager \
  | sed -E 's#(https://api\.telegram\.org/bot)[^/[:space:]]+/#\1<redacted>/#g' || true
REMOTE
