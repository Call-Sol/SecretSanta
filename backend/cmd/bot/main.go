package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alekseymakarkin/secret-santa/internal/config"
	"github.com/alekseymakarkin/secret-santa/internal/groups"
	"github.com/alekseymakarkin/secret-santa/internal/platform/logger"
	"github.com/alekseymakarkin/secret-santa/internal/platform/postgres"
	"github.com/alekseymakarkin/secret-santa/internal/telegram"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log := logger.New(cfg.LogLevel)
	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	me, err := telegram.NewClient(cfg.TelegramBotToken).GetMe(ctx)
	if err != nil {
		return err
	}
	log.Info("telegram getMe", "bot_id", me.ID, "username", me.Username)
	if cfg.TelegramMode != config.ModePolling {
		return fmt.Errorf("telegram mode %s is not implemented", cfg.TelegramMode)
	}
	log.Info("telegram polling")
	return telegram.Poll(ctx, telegram.NewClient(cfg.TelegramBotToken), telegram.NewPostgres(pool), groups.NewPostgres(pool), log, telegram.Options{
		BotUsername: cfg.TelegramBotUsername,
		SessionTTL:  cfg.SessionTTL,
	})
}
