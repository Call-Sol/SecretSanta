package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool opens a pgx pool and checks that the database answers.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, annotate("parse database url", err, databaseURL)
	}
	cfg.MaxConns = 10
	cfg.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, annotate("create database pool", err, databaseURL)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, annotate("ping database", err, databaseURL)
	}
	return pool, nil
}

// WithTx runs fn inside a transaction and rolls back unless fn returns nil.
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) (err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		rbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rbErr := tx.Rollback(rbCtx)
		if rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) && err == nil {
			err = fmt.Errorf("rollback transaction: %w", rbErr)
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func annotate(action string, err error, databaseURL string) error {
	return fmt.Errorf("%s: %s", action, redact(err.Error(), secrets(databaseURL)...))
}

func secrets(databaseURL string) []string {
	out := []string{}
	if databaseURL != "" {
		out = append(out, databaseURL)
	}
	u, err := url.Parse(databaseURL)
	if err != nil || u.User == nil {
		return out
	}
	if u.User.String() != "" {
		out = append(out, u.User.String())
	}
	if pw, ok := u.User.Password(); ok && pw != "" {
		out = append(out, pw)
	}
	return out
}

func redact(msg string, secrets ...string) string {
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, secret, "***")
	}
	return msg
}
