package telegram

import (
	"context"
	"errors"
	"fmt"

	"github.com/alekseymakarkin/secret-santa/internal/platform/postgres"
	"github.com/alekseymakarkin/secret-santa/internal/users"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres stores update deduplication, the polling offset, and users.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres returns a store backed by pool.
func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

// Claim records the update once and upserts the person in the same transaction.
// claimed is false when this update_id was already handled.
func (s *Postgres) Claim(ctx context.Context, updateID int64, id users.Identity, present, markDM bool) (bool, int64, error) {
	var claimed bool
	var userID int64
	err := postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO processed_updates (update_id) VALUES ($1)
			ON CONFLICT DO NOTHING
		`, updateID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		claimed = true
		if !present {
			return nil
		}
		var username *string
		if id.Username != "" {
			username = &id.Username
		}
		return tx.QueryRow(ctx, `
			INSERT INTO users (telegram_id, username, display_name, dm_started_at)
			VALUES ($1, $2, $3, CASE WHEN $4 THEN now() ELSE NULL END)
			ON CONFLICT (telegram_id) DO UPDATE
			SET username = EXCLUDED.username,
			    display_name = EXCLUDED.display_name,
			    dm_started_at = COALESCE(users.dm_started_at, EXCLUDED.dm_started_at),
			    updated_at = now()
			RETURNING id
		`, id.TelegramID, username, id.DisplayName, markDM).Scan(&userID)
	})
	if err != nil {
		return false, 0, fmt.Errorf("claim update: %w", err)
	}
	return claimed, userID, nil
}

// Offset returns the next update id to fetch. Zero means start from the earliest pending update.
func (s *Postgres) Offset(ctx context.Context) (int64, error) {
	var next int64
	err := s.pool.QueryRow(ctx, `SELECT update_offset FROM bot_offsets WHERE id = 1`).Scan(&next)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read offset: %w", err)
	}
	return next, nil
}

// SaveOffset moves the polling cursor forward.
func (s *Postgres) SaveOffset(ctx context.Context, next int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO bot_offsets (id, update_offset) VALUES (1, $1)
		ON CONFLICT (id) DO UPDATE
		SET update_offset = EXCLUDED.update_offset
		WHERE EXCLUDED.update_offset > bot_offsets.update_offset
	`, next)
	if err != nil {
		return fmt.Errorf("save offset: %w", err)
	}
	return nil
}

// ActiveGroupNames lists groups the person still belongs to.
func (s *Postgres) ActiveGroupNames(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT g.name
		FROM group_members m
		JOIN groups g ON g.id = m.group_id
		WHERE m.user_id = $1 AND m.status = 'ACTIVE' AND g.status = 'ACTIVE'
		ORDER BY g.name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("list groups: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	return names, nil
}
