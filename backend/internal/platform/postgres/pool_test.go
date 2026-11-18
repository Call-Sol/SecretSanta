package postgres

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewPoolRejectsUnreachableDatabaseWithoutPassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	const dsn = "postgres://secretsanta:super-secret-pass@127.0.0.1:1/secretsanta?sslmode=disable"
	_, err := NewPool(ctx, dsn)
	if err == nil {
		t.Fatal("expected ping to fail")
	}
	if strings.Contains(err.Error(), "super-secret-pass") || strings.Contains(err.Error(), dsn) {
		t.Fatalf("database url leaked: %v", err)
	}
}

func TestRedactReplacesPassword(t *testing.T) {
	got := redact("dial postgres://secretsanta:super-secret-pass@127.0.0.1/db", "super-secret-pass")
	if strings.Contains(got, "super-secret-pass") {
		t.Fatalf("password leaked: %s", got)
	}
}
