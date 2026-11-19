package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/admin"
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
	cfg, err := config.LoadForAdmin()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logger.New(cfg.LogLevel)
	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	panel, err := admin.New(admin.NewPostgres(pool), admin.Options{
		Username:     cfg.AdminUsername,
		Password:     cfg.AdminPassword,
		BotUsername:  cfg.TelegramBotUsername,
		SessionTTL:   cfg.SessionTTL,
		SecureCookie: cfg.AdminCookieSecure,
		Log:          log,
	})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.AdminHTTPAddr,
		Handler:           panel.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()
	log.Info("admin panel listening", "addr", cfg.AdminHTTPAddr)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
