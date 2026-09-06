package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kokizzu/TeleAntiSpam2Bot/internal/bot"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	checkAdmin := flag.Bool("check-admin", false, "check whether the bot has required admin rights and exit")
	checkAdminChat := flag.String("check-admin-chat", envDefault("TELEANTISPAM_ADMIN_CHECK_CHAT", bot.DefaultAdminCheckChat), "chat username or ID for admin-right checks")
	checkAdminStatusPath := flag.String("check-admin-status-path", envDefault("TELEANTISPAM_ADMIN_CHECK_STATUS_PATH", bot.DefaultAdminCheckStatusPath), "path to write the admin-right check status JSON")
	checkAdminNotifyChat := flag.String("check-admin-notify-chat", envDefault("TELEANTISPAM_ADMIN_CHECK_NOTIFY_CHAT", ""), "chat username or ID to notify once when admin rights are ready")
	retryFailedModeration := flag.Bool("retry-failed-moderation", false, "retry recent failed moderation actions and exit")
	retryFailedMaxAge := flag.Duration("retry-failed-max-age", envDurationDefault("TELEANTISPAM_RETRY_FAILED_ACTION_MAX_AGE", bot.DefaultRetryFailedActionMaxAge), "maximum age of failed moderation actions to retry")
	manualModerate := flag.Bool("manual-moderate", false, "delete specific message IDs, ban the user, send the standard notice, and log the action")
	manualChatID := flag.Int64("manual-chat-id", 0, "chat ID for manual moderation")
	manualUserID := flag.Int64("manual-user-id", 0, "user ID for manual moderation")
	manualMessageIDs := flag.String("manual-message-ids", "", "comma-separated message IDs for manual moderation")
	manualReason := flag.String("manual-reason", "manual", "reason for manual moderation")
	manualMessageSample := flag.String("manual-message-sample", "", "optional sample text for the manual action log")
	flag.Parse()
	if *showVersion {
		fmt.Printf("TeleAntiSpam2Bot version=%s commit=%s\n", version, commit)
		return
	}
	if *checkAdmin {
		token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
		if token == "" {
			log.Fatal("TELEGRAM_BOT_TOKEN is required")
		}
		result, err := bot.RunAdminCheck(token, *checkAdminChat, *checkAdminStatusPath, *checkAdminNotifyChat, time.Now())
		log.Print(result.Summary())
		if err != nil {
			log.Fatalf("check admin: %v", err)
		}
		return
	}

	cfg, err := bot.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	store, err := bot.OpenFileStore(cfg.StatePath)
	if err != nil {
		log.Fatalf("open state store: %v", err)
	}
	if *manualModerate {
		api, err := bot.NewTelegramClient(cfg.Token)
		if err != nil {
			log.Fatalf("new Telegram client: %v", err)
		}
		messageIDs, err := parseManualMessageIDs(*manualMessageIDs)
		if err != nil {
			log.Fatalf("parse manual message IDs: %v", err)
		}
		summary, err := bot.ManualModerate(cfg, store, api, bot.ManualModerationRequest{
			ChatID:            *manualChatID,
			UserID:            *manualUserID,
			MessageIDs:        messageIDs,
			Reason:            *manualReason,
			MessageTextSample: *manualMessageSample,
			Now:               time.Now(),
		})
		log.Print(summary.Summary())
		if err != nil {
			log.Fatalf("manual moderation: %v", err)
		}
		return
	}
	if *retryFailedModeration {
		api, err := bot.NewTelegramClient(cfg.Token)
		if err != nil {
			log.Fatalf("new Telegram client: %v", err)
		}
		summary, err := bot.RetryFailedModerationActions(cfg, store, api, time.Now(), *retryFailedMaxAge)
		log.Print(summary.Summary())
		if err != nil {
			log.Fatalf("retry failed moderation: %v", err)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("starting TeleAntiSpam2Bot version=%s commit=%s dry_run=%t", version, commit, cfg.DryRun)
	startupStats, err := store.RecordStartupStats(time.Now())
	if err != nil {
		log.Fatalf("record startup stats: %v", err)
	}
	logStartupStats(startupStats)
	if err := bot.Run(ctx, cfg, store); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("run bot: %v", err)
	}
}

func logStartupStats(stats bot.StartupStats) {
	previousFrom := stats.PreviousStartupAt.Format(time.RFC3339)
	if stats.PreviousStartupAt.IsZero() {
		previousFrom = "state-start"
	}
	seeded := ""
	if stats.PreviousRunSeededFromAllActionLog {
		seeded = " seeded_from_action_log=true"
	}
	log.Printf(
		"startup_stats tracked_chats=%d tracked_users=%d observed_messages=%d all_time_actions=%d all_time_bans=%d all_time_deleted=%d all_time_failed=%d all_time_dry_run=%d all_time_retry=%d all_time_reasons=%s previous_from=%s previous_actions=%d previous_bans=%d previous_deleted=%d previous_failed=%d previous_dry_run=%d previous_retry=%d previous_reasons=%s%s",
		stats.TrackedChats,
		stats.TrackedUsers,
		stats.ObservedMessages,
		stats.AllTime.ModerationActions,
		stats.AllTime.Bans,
		stats.AllTime.DeletedMessages,
		stats.AllTime.FailedActions,
		stats.AllTime.DryRunActions,
		stats.AllTime.RetryActions,
		formatReasonStats(stats.AllTime.Reasons),
		previousFrom,
		stats.PreviousRun.ModerationActions,
		stats.PreviousRun.Bans,
		stats.PreviousRun.DeletedMessages,
		stats.PreviousRun.FailedActions,
		stats.PreviousRun.DryRunActions,
		stats.PreviousRun.RetryActions,
		formatReasonStats(stats.PreviousRun.Reasons),
		seeded,
	)
}

func formatReasonStats(reasons []bot.ReasonStats) string {
	if len(reasons) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		parts = append(parts, fmt.Sprintf("%s:%d/%d/%d", reason.Reason, reason.ModerationActions, reason.Bans, reason.DeletedMessages))
	}
	return strings.Join(parts, ",")
}

func envDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envDurationDefault(key string, fallback time.Duration) time.Duration {
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

func parseManualMessageIDs(raw string) ([]int, error) {
	var ids []int
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		id, err := strconv.Atoi(item)
		if err != nil {
			return nil, fmt.Errorf("invalid message ID %q: %w", item, err)
		}
		if id <= 0 {
			return nil, fmt.Errorf("invalid message ID %q: must be positive", item)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("at least one message ID is required")
	}
	return ids, nil
}
