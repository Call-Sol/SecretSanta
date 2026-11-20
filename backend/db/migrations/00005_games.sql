-- +goose Up
-- One current game per group. Draws are added later; active_draw_id waits for that table.

CREATE TABLE games (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    group_id bigint NOT NULL REFERENCES groups (id),
    name text NOT NULL,
    status text NOT NULL,
    budget_rub integer NULL,
    registration_deadline timestamptz NULL,
    exchange_on date NULL,
    active_draw_id bigint NULL,
    created_by bigint NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT games_name_len CHECK (char_length(btrim(name)) BETWEEN 1 AND 80),
    CONSTRAINT games_status_check CHECK (status IN ('DRAFT', 'REGISTRATION', 'LOCKED', 'DRAWN', 'COMPLETED', 'CANCELLED')),
    CONSTRAINT games_budget_check CHECK (budget_rub IS NULL OR (budget_rub > 0 AND budget_rub <= 100000000))
);

CREATE UNIQUE INDEX games_one_current_per_group
    ON games (group_id)
    WHERE status IN ('DRAFT', 'REGISTRATION', 'LOCKED', 'DRAWN');

CREATE TABLE game_participants (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    game_id bigint NOT NULL REFERENCES games (id),
    user_id bigint NOT NULL REFERENCES users (id),
    status text NOT NULL,
    assignment_state text NOT NULL,
    registered_at timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz NULL,
    UNIQUE (game_id, user_id),
    CONSTRAINT game_participants_status_check CHECK (status IN ('PENDING', 'CONFIRMED', 'DECLINED')),
    CONSTRAINT game_participants_assignment_check CHECK (assignment_state IN ('NOT_APPLICABLE', 'AWAITING_ASSIGNMENT', 'ASSIGNED'))
);

-- +goose Down
DROP TABLE game_participants;
DROP TABLE games;
