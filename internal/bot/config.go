package bot

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultStatePath         = "/var/lib/teleantispam/state.json"
	defaultMaxSafePosts      = 10
	defaultLowHistoryPosts   = 2
	defaultJoinWindow        = 48 * time.Hour
	defaultMaxMessageAge     = 24 * time.Hour
	defaultDeleteRecentLimit = 10
	defaultActionLogLimit    = 10000
	defaultPollTimeout       = 60
)

type Config struct {
	Token                 string
	StatePath             string
	AllowedChatIDs        map[int64]bool
	ReportChatID          int64
	MaxSafePosts          int
	LowHistoryPosts       int
	JoinWindow            time.Duration
	MaxMessageAge         time.Duration
	DeleteRecentLimit     int
	ActionLogLimit        int
	PollTimeout           int
	DryRun                bool
	AllowUnknownNoHistory bool
}

func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		Token:                 strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		StatePath:             getEnvDefault("TELEANTISPAM_STATE_PATH", defaultStatePath),
		MaxSafePosts:          getEnvIntDefault("TELEANTISPAM_MAX_SAFE_POSTS", defaultMaxSafePosts),
		LowHistoryPosts:       getEnvIntDefault("TELEANTISPAM_LOW_HISTORY_POSTS", defaultLowHistoryPosts),
		JoinWindow:            getEnvDurationDefault("TELEANTISPAM_JOIN_WINDOW", defaultJoinWindow),
		MaxMessageAge:         getEnvDurationDefault("TELEANTISPAM_MAX_MESSAGE_AGE", defaultMaxMessageAge),
		DeleteRecentLimit:     getEnvIntDefault("TELEANTISPAM_DELETE_RECENT_LIMIT", defaultDeleteRecentLimit),
		ActionLogLimit:        getEnvIntDefault("TELEANTISPAM_ACTION_LOG_LIMIT", defaultActionLogLimit),
		PollTimeout:           getEnvIntDefault("TELEANTISPAM_POLL_TIMEOUT", defaultPollTimeout),
		DryRun:                getEnvBoolDefault("TELEANTISPAM_DRY_RUN", false),
		AllowUnknownNoHistory: getEnvBoolDefault("TELEANTISPAM_ALLOW_UNKNOWN_NO_HISTORY", false),
	}

	if cfg.Token == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.MaxSafePosts < 1 {
		return Config{}, fmt.Errorf("TELEANTISPAM_MAX_SAFE_POSTS must be positive")
	}
	if cfg.LowHistoryPosts < 0 {
		return Config{}, fmt.Errorf("TELEANTISPAM_LOW_HISTORY_POSTS cannot be negative")
	}
	if cfg.JoinWindow <= 0 {
		return Config{}, fmt.Errorf("TELEANTISPAM_JOIN_WINDOW must be positive")
	}
	if cfg.MaxMessageAge <= 0 {
		return Config{}, fmt.Errorf("TELEANTISPAM_MAX_MESSAGE_AGE must be positive")
	}
	if cfg.DeleteRecentLimit < 1 {
		return Config{}, fmt.Errorf("TELEANTISPAM_DELETE_RECENT_LIMIT must be positive")
	}
	if cfg.ActionLogLimit < 1 {
		return Config{}, fmt.Errorf("TELEANTISPAM_ACTION_LOG_LIMIT must be positive")
	}
	if cfg.PollTimeout < 1 {
		return Config{}, fmt.Errorf("TELEANTISPAM_POLL_TIMEOUT must be positive")
	}

	allowed, err := parseAllowedChatIDs(os.Getenv("TELEANTISPAM_ALLOWED_CHAT_IDS"))
	if err != nil {
		return Config{}, err
	}
	cfg.AllowedChatIDs = allowed

	reportChatID, err := parseOptionalInt64("TELEANTISPAM_REPORT_CHAT_ID", os.Getenv("TELEANTISPAM_REPORT_CHAT_ID"))
	if err != nil {
		return Config{}, err
	}
	cfg.ReportChatID = reportChatID
	return cfg, nil
}

func (cfg Config) AllowsChat(chatID int64) bool {
	return len(cfg.AllowedChatIDs) == 0 || cfg.AllowedChatIDs[chatID]
}

func (cfg Config) NoticeChatID(sourceChatID int64) int64 {
	if cfg.ReportChatID != 0 {
		return cfg.ReportChatID
	}
	return sourceChatID
}

func getEnvDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getEnvIntDefault(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBoolDefault(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvDurationDefault(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseAllowedChatIDs(raw string) (map[int64]bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	result := make(map[int64]bool)
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		chatID, err := strconv.ParseInt(item, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid TELEANTISPAM_ALLOWED_CHAT_IDS item %q: %w", item, err)
		}
		result[chatID] = true
	}
	return result, nil
}

func parseOptionalInt64(key string, raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return value, nil
}
