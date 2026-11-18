package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/config"
	"github.com/alekseymakarkin/secret-santa/internal/platform/postgres"
)

func main() {
	cfg, err := config.LoadForMigrate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %s\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := postgres.MigrateUp(ctx, cfg.DatabaseURL); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %s\n", err)
		os.Exit(1)
	}
	fmt.Println("migrations applied")
}
