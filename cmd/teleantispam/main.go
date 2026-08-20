package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kokizzu/teleantispam/internal/bot"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("TelegramAntiSpam version=%s commit=%s\n", version, commit)
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
