package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alekseymakarkin/secret-santa/internal/config"
	"github.com/alekseymakarkin/secret-santa/internal/platform/logger"
	"github.com/alekseymakarkin/secret-santa/internal/platform/postgres"
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

	log.Info("database ready")
	log.Info("outbox delivery is not started in this slice")
	<-ctx.Done()
	return nil
}
