#!/usr/bin/env bash
set -euo pipefail

source scripts/load-local-env.sh

host="${TELEANTISPAM_DEPLOY_HOST:-}"
request_file="tmp/telegram-message-id.txt"

if [[ -z "$host" ]]; then
  echo "missing TELEANTISPAM_DEPLOY_HOST" >&2
  exit 1
fi
if [[ ! -s "$request_file" ]]; then
  echo "missing message-ID request: $request_file" >&2
  exit 1
fi

message_id="$(tr -d '[:space:]' < "$request_file")"
if [[ ! "$message_id" =~ ^[1-9][0-9]*$ ]]; then
  echo "message ID must be a positive integer: $message_id" >&2
  exit 1
fi

ssh_opts=(-x -o BatchMode=yes -o ConnectTimeout=15)
ssh "${ssh_opts[@]}" "$host" python3 - "$message_id" <<'PY'
import json
import sys
from pathlib import Path

message_id = int(sys.argv[1])
path = Path("/var/lib/teleantispam/state.json")
data = json.loads(path.read_text(encoding="utf-8"))
matches = []
for chat_id, users in (data.get("chats") or {}).items():
    for user_id, history in (users or {}).items():
        recent = (history or {}).get("recent_message_ids") or []
        if message_id in recent:
            matches.append({
                "chat_id": int(chat_id),
                "user_id": int(user_id),
                "message_count": int((history or {}).get("message_count") or 0),
                "first_seen_at": (history or {}).get("first_seen_at"),
                "joined_at": (history or {}).get("joined_at"),
                "last_message_at": (history or {}).get("last_message_at"),
                "recent_message_ids": recent,
            })
print(json.dumps({"message_id": message_id, "matches": matches}, indent=2))
if len(matches) != 1:
    raise SystemExit(2)
PY

if command -v notify-send >/dev/null 2>&1; then
  notify-send "TeleAntiSpam2Bot lookup" "Finished resolving Telegram message $message_id"
fi
