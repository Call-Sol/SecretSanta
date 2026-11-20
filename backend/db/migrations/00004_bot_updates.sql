-- +goose Up
-- First-launch bookkeeping: one row per Telegram update, the polling offset,
-- and a session row for later wizards.

CREATE TABLE processed_updates (
    update_id bigint PRIMARY KEY,
    processed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE bot_offsets (
    id integer PRIMARY KEY,
    update_offset bigint NOT NULL,
    CONSTRAINT bot_offsets_singleton CHECK (id = 1)
);

CREATE TABLE bot_sessions (
    user_id bigint PRIMARY KEY REFERENCES users (id),
    state text NOT NULL,
    context jsonb NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE bot_sessions;
DROP TABLE bot_offsets;
DROP TABLE processed_updates;
