package telegram

import (
	"context"
	"log/slog"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/games"
	"github.com/alekseymakarkin/secret-santa/internal/groups"
)

// Poll reads private updates and answers /start with the main menu.
func Poll(ctx context.Context, client *Client, store Store, directory groups.Directory, catalog games.Catalog, log *slog.Logger, opts Options) error {
	if log == nil {
		log = slog.Default()
	}
	handler := NewHandler(store, directory, catalog, client, log, opts)
	offset, err := store.Offset(ctx)
	if err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		batch, err := client.GetUpdates(ctx, offset, 30)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Error("telegram getUpdates", "err", err.Error())
			sleep(ctx, 2*time.Second)
			continue
		}
		for _, upd := range batch {
			if err := handler.Handle(ctx, upd); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			next := upd.UpdateID + 1
			if next <= offset {
				continue
			}
			if err := store.SaveOffset(ctx, next); err != nil {
				return err
			}
			offset = next
		}
	}
}

func sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
