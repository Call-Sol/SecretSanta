package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/alekseymakarkin/secret-santa/db"
	"github.com/pressly/goose/v3"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// MigrateUp applies embedded goose migrations.
func MigrateUp(ctx context.Context, databaseURL string) error {
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return annotate("open database", err, databaseURL)
	}
	defer sqlDB.Close()

	if err := sqlDB.PingContext(ctx); err != nil {
		return annotate("ping database", err, databaseURL)
	}

	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, sqlDB, "migrations"); err != nil {
		return annotate("apply migrations", err, databaseURL)
	}
	return nil
}

func migrationFiles() ([]string, error) {
	var names []string
	err := fs.WalkDir(db.Migrations, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			names = append(names, path)
		}
		return nil
	})
	return names, err
}
