package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kokizzu/teleantispam/internal/bot"
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
	flag.Parse()
	if *showVersion {
		fmt.Printf("TelegramAntiSpam version=%s commit=%s\n", version, commit)
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("starting TelegramAntiSpam version=%s commit=%s dry_run=%t", version, commit, cfg.DryRun)
	if err := bot.Run(ctx, cfg, store); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("run bot: %v", err)
	}
}

func envDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
