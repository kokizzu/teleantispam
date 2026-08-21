# TeleAntiSpam2Bot

<p align="center">
  <img src="assets/telegram-antispam-icon.png" alt="TeleAntiSpam2Bot icon" width="180">
</p>

TeleAntiSpam2Bot is a Telegram moderation bot for removing obvious spam from
groups. It is intentionally conservative: users with 10 or more observed posts
are never auto-banned or auto-deleted by the rule engine, even when a message
looks suspicious.

By default, auto-ban requires that the bot observed the user joining recently,
except for high-confidence one-message spam that combines an external contact
handle with a money/work pitch. This avoids treating an old group member as
"new" only because the bot was just started with an empty state file. The looser
no-history path can be enabled with `TELEANTISPAM_ALLOW_UNKNOWN_NO_HISTORY=true`
after accepting that tradeoff.

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
- [x] Failed Telegram member lookup is fail-closed and tested.
- [x] Plain unknown no-history users are not moderated by default; broad opt-in
      path tested.
- [x] High-confidence unknown no-history crypto and Cyrillic recruitment spam
      moderation tested.
- [x] Post-ban notification message implemented and tested.
- [x] Structured moderation action logs collect available account evidence and
      Telegram API limitation notes.
- [x] Local `.env.override` is git-ignored and loaded by run/deploy scripts.
- [x] Bot profile icon generated, saved in `assets/`, uploaded, and verified.
- [x] Non-root systemd service file tested on the remote host.
- [x] Deployed to the remote host.
- [x] Remote service verified healthy.
- [x] Hourly admin-promotion check deployed to the remote host.
- [x] Hourly admin check notifies if admin/delete/ban permissions are revoked.
- [x] Bot added as admin in `gophers_id`.
- [x] One-shot retry command tested and run for pre-admin failed moderation
      actions; bans were retried, while stale messages can remain non-deletable.
- [x] Live `gophers_id` test completed.

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
| `TELEANTISPAM_ALLOW_UNKNOWN_NO_HISTORY` | `false` | Allow auto-moderation for suspicious users without an observed recent join and without observed post history. |
| `TELEANTISPAM_ADMIN_CHECK_CHAT` | `@gophers_id` | Chat checked by the hourly admin-promotion timer. |
| `TELEANTISPAM_ADMIN_CHECK_STATUS_PATH` | `/var/lib/teleantispam/gophers-admin-status.json` | Status JSON written by the hourly admin-promotion timer. |
| `TELEANTISPAM_ADMIN_CHECK_NOTIFY_CHAT` | empty | Optional chat username or ID notified once when the bot first has the required admin rights, and once if those rights are later revoked. Empty means the checked chat. |
| `TELEANTISPAM_RETRY_FAILED_ACTION_MAX_AGE` | `48h` | Maximum age of failed moderation actions eligible for one-shot retry. |

## Local Commands

```sh
make test
make build
make remote-status
make check-admin
make retry-failed-moderation
make remote-retry-failed-moderation
TELEANTISPAM_DRY_RUN=true make run
```

For local secrets, create `.env.override`. It is ignored by git and loaded by
`make run`, `make deploy`, and `make install-remote-env`.

## Deployment

The deployment target is a non-root service account named `teleantispam`.
Set `TELEANTISPAM_DEPLOY_HOST` in `.env.override` or in the shell before
running deployment commands.

```sh
make deploy
```

To prepare the remote binary and systemd unit before the bot token is available:

```sh
TELEANTISPAM_INSTALL_ONLY=true make deploy
```

To install only `/etc/teleantispam/teleantispam.env` later:

```sh
make install-remote-env
make deploy
```

The deploy script installs:

- `/usr/local/bin/teleantispam`
- `/etc/systemd/system/teleantispam.service`
- `/etc/teleantispam/teleantispam.env`
- `/var/lib/teleantispam`

The service uses `User=teleantispam`, `Group=teleantispam`, and a hardened
systemd sandbox with write access only to `/var/lib/teleantispam`.
