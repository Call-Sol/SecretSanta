//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestMigrateUpIsIdempotent(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := MigrateUp(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	if err := MigrateUp(ctx, dsn); err != nil {
		t.Fatal(err)
	}

	pool, err := NewPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	var applied int
	err = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM goose_db_version
		WHERE version_id = 1 AND is_applied = true
	`).Scan(&applied)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 1 {
		t.Fatalf("applied version 1 rows = %d", applied)
	}

	err = WithTx(ctx, pool, func(tx pgx.Tx) error {
		var one int
		return tx.QueryRow(ctx, "SELECT 1").Scan(&one)
	})
	if err != nil {
		t.Fatal(err)
	}
}
