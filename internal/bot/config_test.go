package bot

import (
	"testing"
	"time"
)

func TestLoadConfigFromEnvRequiresToken(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	if _, err := LoadConfigFromEnv(); err == nil {
		t.Fatal("expected missing token error")
	}
}

func TestLoadConfigFromEnvParsesModerationSettings(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "123456:token")
	t.Setenv("TELEANTISPAM_STATE_PATH", "/tmp/teleantispam-state.json")
	t.Setenv("TELEANTISPAM_ALLOWED_CHAT_IDS", "-1001,-1002")
	t.Setenv("TELEANTISPAM_REPORT_CHAT_ID", "-2001")
	t.Setenv("TELEANTISPAM_MAX_SAFE_POSTS", "11")
	t.Setenv("TELEANTISPAM_LOW_HISTORY_POSTS", "3")
	t.Setenv("TELEANTISPAM_JOIN_WINDOW", "6h")
	t.Setenv("TELEANTISPAM_MAX_MESSAGE_AGE", "12h")
	t.Setenv("TELEANTISPAM_DELETE_RECENT_LIMIT", "7")
	t.Setenv("TELEANTISPAM_ACTION_LOG_LIMIT", "99")
	t.Setenv("TELEANTISPAM_POLL_TIMEOUT", "33")
	t.Setenv("TELEANTISPAM_DRY_RUN", "true")
	t.Setenv("TELEANTISPAM_ALLOW_UNKNOWN_NO_HISTORY", "true")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Token != "123456:token" {
		t.Fatalf("unexpected token: %q", cfg.Token)
	}
	if cfg.StatePath != "/tmp/teleantispam-state.json" {
		t.Fatalf("unexpected state path: %q", cfg.StatePath)
	}
	if !cfg.AllowsChat(-1001) || !cfg.AllowsChat(-1002) || cfg.AllowsChat(-1003) {
		t.Fatalf("unexpected allowed chats: %#v", cfg.AllowedChatIDs)
	}
	if cfg.NoticeChatID(-1001) != -2001 {
		t.Fatalf("unexpected notice chat: %d", cfg.NoticeChatID(-1001))
	}
	if cfg.MaxSafePosts != 11 || cfg.LowHistoryPosts != 3 || cfg.DeleteRecentLimit != 7 || cfg.ActionLogLimit != 99 || cfg.PollTimeout != 33 {
		t.Fatalf("unexpected integer settings: %#v", cfg)
	}
	if cfg.JoinWindow != 6*time.Hour || cfg.MaxMessageAge != 12*time.Hour {
		t.Fatalf("unexpected duration settings: %#v", cfg)
	}
	if !cfg.DryRun {
		t.Fatalf("expected dry run true")
	}
	if !cfg.AllowUnknownNoHistory {
		t.Fatalf("expected unknown no-history moderation opt-in true")
	}
}

func TestLoadConfigFromEnvRejectsInvalidChatIDs(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "123456:token")
	t.Setenv("TELEANTISPAM_ALLOWED_CHAT_IDS", "-1001,nope")
	if _, err := LoadConfigFromEnv(); err == nil {
		t.Fatal("expected invalid allowed chat id error")
	}
}
