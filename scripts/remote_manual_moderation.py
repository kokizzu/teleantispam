#!/usr/bin/env python3
"""Plan and apply an exact, reviewable remote Telegram moderation action."""

from __future__ import annotations

import base64
import hashlib
import json
import shutil
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path


PROJECT = Path(__file__).resolve().parent.parent
REQUEST = PROJECT / "tmp" / "manual-moderation-request.json"
PLAN = PROJECT / "tmp" / "manual-moderation-plan.json"
RESULT = PROJECT / "tmp" / "manual-moderation-result.json"
SSH_OPTIONS = ("-x", "-o", "BatchMode=yes", "-o", "ConnectTimeout=15")


REMOTE_LOOKUP = r'''
import json
import sys
from pathlib import Path

message_ids = [int(value) for value in sys.argv[1:]]
data = json.loads(Path("/var/lib/teleantispam/state.json").read_text(encoding="utf-8"))
matches = []
for chat_id, users in (data.get("chats") or {}).items():
    for user_id, history in (users or {}).items():
        recent = (history or {}).get("recent_message_ids") or []
        found = [message_id for message_id in message_ids if message_id in recent]
        if found:
            matches.append({
                "chat_id": int(chat_id),
                "user_id": int(user_id),
                "found_message_ids": found,
                "message_count": int((history or {}).get("message_count") or 0),
                "first_seen_at": (history or {}).get("first_seen_at"),
                "joined_at": (history or {}).get("joined_at"),
                "last_message_at": (history or {}).get("last_message_at"),
                "recent_message_ids": recent,
            })
print(json.dumps(matches))
'''


REMOTE_APPLY = r'''
set -euo pipefail
decode_arg() {
  printf '%s' "$1" | base64 -d
}
chat_id="$(decode_arg "$1")"
user_id="$(decode_arg "$2")"
message_ids="$(decode_arg "$3")"
reason="$(decode_arg "$4")"
sample="$(decode_arg "$5")"
systemctl stop teleantispam.service
trap "systemctl start teleantispam.service" EXIT
systemd-run --wait --collect --pipe \
  --property=User=teleantispam \
  --property=Group=teleantispam \
  --property=WorkingDirectory=/var/lib/teleantispam \
  --property=EnvironmentFile=/etc/teleantispam/teleantispam.env \
  /usr/local/bin/teleantispam \
    -manual-moderate \
    "-manual-chat-id=$chat_id" \
    "-manual-user-id=$user_id" \
    "-manual-message-ids=$message_ids" \
    "-manual-reason=$reason" \
    "-manual-message-sample=$sample"
systemctl start teleantispam.service
trap - EXIT
systemctl is-active teleantispam.service
'''


def now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def canonical_json(value: object) -> bytes:
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")) + "\n").encode()


def sha256(value: object) -> str:
    return hashlib.sha256(canonical_json(value)).hexdigest()


def write_json_atomic(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    staged = path.with_name(path.name + ".new")
    staged.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    staged.replace(path)


def load_request() -> dict[str, object]:
    request = json.loads(REQUEST.read_text(encoding="utf-8"))
    required = ("chat_id", "user_id", "message_ids", "reason", "message_text_sample")
    missing = [key for key in required if key not in request]
    if missing:
        raise ValueError(f"request is missing: {', '.join(missing)}")
    if not isinstance(request["chat_id"], int) or request["chat_id"] == 0:
        raise ValueError("chat_id must be a non-zero integer")
    if not isinstance(request["user_id"], int) or request["user_id"] <= 0:
        raise ValueError("user_id must be a positive integer")
    message_ids = request["message_ids"]
    if not isinstance(message_ids, list) or not message_ids or any(not isinstance(value, int) or value <= 0 for value in message_ids):
        raise ValueError("message_ids must contain positive integers")
    if len(set(message_ids)) != len(message_ids):
        raise ValueError("message_ids must not contain duplicates")
    if not str(request["reason"]).strip():
        raise ValueError("reason must not be empty")
    return request


def lookup(host: str, message_ids: list[int]) -> list[dict[str, object]]:
    result = subprocess.run(
        ["ssh", *SSH_OPTIONS, host, "python3", "-", *map(str, message_ids)],
        input=REMOTE_LOOKUP,
        text=True,
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if result.returncode != 0:
        raise RuntimeError(result.stderr.strip() or "remote state lookup failed")
    return json.loads(result.stdout)


def plan(host: str) -> None:
    request = load_request()
    message_ids = list(request["message_ids"])
    matches = lookup(host, message_ids)
    expected = [
        row for row in matches
        if row["chat_id"] == request["chat_id"] and row["user_id"] == request["user_id"]
    ]
    if len(expected) != 1:
        raise RuntimeError("remote state does not uniquely map the requested messages to the requested chat/user")
    if sorted(expected[0]["found_message_ids"]) != sorted(message_ids):
        raise RuntimeError("not every requested message belongs to the requested chat/user in remote state")
    if any(row not in expected for row in matches):
        raise RuntimeError("at least one requested message ID also matched another remote history entry")

    value = {
        "version": 1,
        "plan_id": "manual-moderation-" + sha256(request)[:16],
        "created_at": now(),
        "request_sha256": sha256(request),
        "target": request,
        "remote_evidence": expected[0],
        "effects": [
            "delete the exact listed Telegram messages",
            "ban the exact listed Telegram user from the exact listed chat",
            "append the standard moderation action and send its notice",
        ],
        "rollback": "Deleted Telegram messages cannot be restored; a ban can only be reversed by a separate unban action.",
    }
    write_json_atomic(PLAN, value)
    print(json.dumps(value, ensure_ascii=False, indent=2))
    print(f"PLAN={PLAN}")


def encode(value: object) -> str:
    return base64.b64encode(str(value).encode()).decode()


def apply(host: str) -> None:
    request = load_request()
    value = json.loads(PLAN.read_text(encoding="utf-8"))
    if value.get("version") != 1 or value.get("request_sha256") != sha256(request) or value.get("target") != request:
        raise RuntimeError("plan is missing or stale; run make plan-remote-manual-moderation and review it again")
    args = [
        encode(request["chat_id"]),
        encode(request["user_id"]),
        encode(",".join(map(str, request["message_ids"]))),
        encode(request["reason"]),
        encode(request["message_text_sample"]),
    ]
    result = subprocess.run(
        ["ssh", *SSH_OPTIONS, host, "bash", "-s", "--", *args],
        input=REMOTE_APPLY,
        text=True,
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )
    outcome = {
        "version": 1,
        "plan_id": value["plan_id"],
        "applied_at": now(),
        "exit_code": result.returncode,
        "output": result.stdout,
    }
    write_json_atomic(RESULT, outcome)
    print(result.stdout, end="")
    print(f"RESULT={RESULT}")
    if result.returncode != 0:
        raise RuntimeError(f"remote moderation failed with exit code {result.returncode}")


def notify(message: str) -> None:
    executable = shutil.which("notify-send")
    if executable:
        subprocess.run([executable, "TeleAntiSpam2Bot moderation", message], check=False)


def main() -> int:
    if len(sys.argv) != 3 or sys.argv[1] not in {"plan", "apply"}:
        raise ValueError("usage: remote_manual_moderation.py {plan|apply} DEPLOY_HOST")
    mode, host = sys.argv[1:]
    if mode == "plan":
        plan(host)
        notify("Manual moderation plan is ready for review")
    else:
        apply(host)
        notify("Manual moderation completed")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        notify(f"Manual moderation failed: {exc}")
        raise SystemExit(1)
