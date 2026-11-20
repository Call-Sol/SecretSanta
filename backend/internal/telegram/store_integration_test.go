//go:build integration

package telegram

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/platform/postgres"
	"github.com/alekseymakarkin/secret-santa/internal/users"
)

func TestClaimUpsertsUserOnce(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := postgres.MigrateUp(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	pool, err := postgres.NewPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	store := NewPostgres(pool)
	telegramID := time.Now().UnixNano()
	updateID := telegramID
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM processed_updates WHERE update_id = $1`, updateID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE telegram_id = $1`, telegramID)
	})

	id := users.Profile(telegramID, "santa_user", "Анна", "")
	claimed, userID, err := store.Claim(ctx, updateID, id, true, true)
	if err != nil || !claimed || userID == 0 {
		t.Fatalf("claim claimed=%v user=%d err=%v", claimed, userID, err)
	}
	again, _, err := store.Claim(ctx, updateID, users.Profile(telegramID, "santa_user", "Анна Петрова", ""), true, true)
	if err != nil || again {
		t.Fatalf("second claim claimed=%v err=%v", again, err)
	}

	var name string
	var started bool
	err = pool.QueryRow(ctx, `
		SELECT display_name, dm_started_at IS NOT NULL
		FROM users WHERE telegram_id = $1
	`, telegramID).Scan(&name, &started)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Анна" || !started {
		t.Fatalf("stored name %q started %v", name, started)
	}

	names, err := store.ActiveGroupNames(ctx, userID)
	if err != nil || len(names) != 0 {
		t.Fatalf("groups %v err %v", names, err)
	}
}
