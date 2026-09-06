#!/usr/bin/env bash
set -euo pipefail

binary="${BINARY:-bin/teleantispam}"
chat_id="${TELEANTISPAM_MANUAL_CHAT_ID:-}"
user_id="${TELEANTISPAM_MANUAL_USER_ID:-}"
message_ids="${TELEANTISPAM_MANUAL_MESSAGE_IDS:-}"
reason="${TELEANTISPAM_MANUAL_REASON:-manual}"
sample="${TELEANTISPAM_MANUAL_MESSAGE_SAMPLE:-}"

if [[ -z "$chat_id" || -z "$user_id" || -z "$message_ids" ]]; then
  echo "TELEANTISPAM_MANUAL_CHAT_ID, TELEANTISPAM_MANUAL_USER_ID, and TELEANTISPAM_MANUAL_MESSAGE_IDS are required" >&2
  exit 1
fi

exec "$binary" \
  -manual-moderate \
  -manual-chat-id="$chat_id" \
  -manual-user-id="$user_id" \
  -manual-message-ids="$message_ids" \
  -manual-reason="$reason" \
  -manual-message-sample="$sample"
