# TelegramAntiSpam

TelegramAntiSpam is a Telegram moderation bot for removing obvious spam from
groups. It is intentionally conservative: users with 10 or more observed posts
are never auto-banned or auto-deleted by the rule engine, even when a message
looks suspicious.

The intended moderation action mirrors the manual Telegram flow:

- Delete the spammer's recent tracked messages.
- Ban the user.
- Ask Telegram to revoke the banned user's messages where Bot API support is
  available.
- Send a group or channel notice: `[userID] / [Name] / [@telegramUsername] is banned because [rule reason], [N] messages deleted`.

Telegram's Bot API does not expose the client-side "Report Spam" checkbox, so
the bot cannot perform that part directly.

On startup the bot processes Telegram pending updates and checks messages dated
within the last 24 hours. The Bot API cannot fetch arbitrary pre-existing group
history, so this startup scan is limited to updates Telegram still has pending
for the bot.

## Status Checklist

- [x] Go bot module created and tested.
- [x] Hard guard tested: users with `>=10` observed posts are not moderated.
- [x] Low-history spam detection tested for `0`, Chinese/CJK-heavy text,
      Cyrillic/Russian-like text, and crypto spam.
- [x] Startup pending-update scan for recent 1-day messages tested.
- [x] Telegram transport builds.
- [x] Recent-message deletion plus ban/revoke moderation path implemented and
      tested with a fake Telegram client.
- [x] Administrator and creator safety guard tested.
- [x] Post-ban notification message implemented and tested.
- [x] Structured moderation action logs collect available account evidence and
      Telegram API limitation notes.
- [x] Non-root systemd service file tested on the remote host.
- [ ] Deployed to `the remote host`.
- [ ] Remote service verified healthy.
- [ ] Bot added as admin in `gophers_id`.
- [ ] Live `gophers_id` test completed.

## Configuration

The bot reads configuration from environment variables.

| Variable | Default | Description |
| --- | --- | --- |
| `TELEGRAM_BOT_TOKEN` | required | Bot token from BotFather. |
| `TELEANTISPAM_STATE_PATH` | `/var/lib/teleantispam/state.json` | Persistent per-chat user history. |
| `TELEANTISPAM_ALLOWED_CHAT_IDS` | empty | Optional comma-separated chat IDs. Empty means all chats. |
| `TELEANTISPAM_REPORT_CHAT_ID` | empty | Optional channel/chat ID for ban notices. Empty posts notices to the source group. |
| `TELEANTISPAM_MAX_SAFE_POSTS` | `10` | Users with this many observed posts are never auto-moderated. |
| `TELEANTISPAM_LOW_HISTORY_POSTS` | `2` | Suspicious users at or below this observed-post count are eligible. |
| `TELEANTISPAM_JOIN_WINDOW` | `48h` | Recent join window for spam eligibility. |
| `TELEANTISPAM_MAX_MESSAGE_AGE` | `24h` | Only messages newer than this are eligible for moderation on startup. |
| `TELEANTISPAM_DELETE_RECENT_LIMIT` | `10` | Maximum tracked recent messages to delete. |
| `TELEANTISPAM_ACTION_LOG_LIMIT` | `10000` | Maximum stored structured moderation action logs. |
| `TELEANTISPAM_POLL_TIMEOUT` | `60` | Telegram long-poll timeout in seconds. |
| `TELEANTISPAM_DRY_RUN` | `false` | Log actions without deleting or banning. |

## Local Commands

```sh
make test
make build
TELEGRAM_BOT_TOKEN=... TELEANTISPAM_DRY_RUN=true make run
```

## Deployment

The deployment target is a non-root service account named `teleantispam`.

```sh
TELEGRAM_BOT_TOKEN=... make deploy
```

To prepare the remote binary and systemd unit before the bot token is available:

```sh
TELEANTISPAM_INSTALL_ONLY=true make deploy
```

The deploy script installs:

- `/usr/local/bin/teleantispam`
- `/etc/systemd/system/teleantispam.service`
- `/etc/teleantispam/teleantispam.env`
- `/var/lib/teleantispam`

The service uses `User=teleantispam`, `Group=teleantispam`, and a hardened
systemd sandbox with write access only to `/var/lib/teleantispam`.
