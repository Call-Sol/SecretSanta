-- +goose Up
-- A draw, its assignments, and the private messages that follow it commit together.

ALTER TABLE game_participants
    ADD CONSTRAINT game_participants_id_game_unique UNIQUE (id, game_id);

CREATE TABLE draws (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    game_id bigint NOT NULL REFERENCES games (id),
    version integer NOT NULL,
    status text NOT NULL,
    created_by bigint NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (game_id, version),
    UNIQUE (id, game_id),
    CONSTRAINT draws_status_check CHECK (status IN ('ACTIVE', 'INVALIDATED')),
    CONSTRAINT draws_version_check CHECK (version > 0)
);

CREATE UNIQUE INDEX draws_one_active_per_game
    ON draws (game_id)
    WHERE status = 'ACTIVE';

ALTER TABLE games
    ADD CONSTRAINT games_active_draw_fk
    FOREIGN KEY (active_draw_id) REFERENCES draws (id);

CREATE TABLE assignments (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    draw_id bigint NOT NULL,
    game_id bigint NOT NULL,
    giver_participant_id bigint NOT NULL,
    receiver_participant_id bigint NOT NULL,
    UNIQUE (draw_id, giver_participant_id),
    UNIQUE (draw_id, receiver_participant_id),
    CONSTRAINT assignments_not_self CHECK (giver_participant_id <> receiver_participant_id),
    CONSTRAINT assignments_draw_fk FOREIGN KEY (draw_id, game_id) REFERENCES draws (id, game_id),
    CONSTRAINT assignments_giver_fk FOREIGN KEY (giver_participant_id, game_id) REFERENCES game_participants (id, game_id),
    CONSTRAINT assignments_receiver_fk FOREIGN KEY (receiver_participant_id, game_id) REFERENCES game_participants (id, game_id)
);

CREATE TABLE notification_outbox (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users (id),
    game_id bigint NULL REFERENCES games (id),
    draw_id bigint NULL REFERENCES draws (id),
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    status text NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    claimed_at timestamptz NULL,
    last_error text NULL,
    sent_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT notification_outbox_status_check CHECK (status IN ('PENDING', 'SENDING', 'SENT', 'FAILED', 'CANCELLED')),
    CONSTRAINT notification_outbox_attempts_check CHECK (attempts >= 0)
);

CREATE INDEX notification_outbox_due
    ON notification_outbox (next_attempt_at)
    WHERE status IN ('PENDING', 'SENDING');

-- +goose Down
DROP TABLE notification_outbox;
DROP TABLE assignments;
ALTER TABLE games DROP CONSTRAINT games_active_draw_fk;
DROP TABLE draws;
ALTER TABLE game_participants DROP CONSTRAINT game_participants_id_game_unique;
