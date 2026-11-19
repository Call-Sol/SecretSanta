-- +goose Up
-- Groups and join invites for the operator panel.
-- owner_id and created_by stay nullable until a Telegram user exists.

CREATE TABLE users (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    telegram_id bigint NOT NULL UNIQUE,
    username text NULL,
    display_name text NOT NULL,
    dm_started_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE groups (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL,
    owner_id bigint NULL REFERENCES users (id),
    status text NOT NULL DEFAULT 'ACTIVE',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz NULL,
    CONSTRAINT groups_name_len CHECK (char_length(btrim(name)) BETWEEN 1 AND 80),
    CONSTRAINT groups_status_check CHECK (status IN ('ACTIVE', 'ARCHIVED'))
);

CREATE TABLE group_members (
    group_id bigint NOT NULL REFERENCES groups (id),
    user_id bigint NOT NULL REFERENCES users (id),
    role text NOT NULL,
    status text NOT NULL,
    joined_at timestamptz NOT NULL DEFAULT now(),
    left_at timestamptz NULL,
    PRIMARY KEY (group_id, user_id),
    CONSTRAINT group_members_role_check CHECK (role IN ('OWNER', 'ADMIN', 'MEMBER')),
    CONSTRAINT group_members_status_check CHECK (status IN ('ACTIVE', 'LEFT'))
);

CREATE UNIQUE INDEX group_members_one_active_owner
    ON group_members (group_id)
    WHERE role = 'OWNER' AND status = 'ACTIVE';

CREATE TABLE group_invites (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    group_id bigint NOT NULL REFERENCES groups (id),
    token_hash bytea NOT NULL UNIQUE,
    created_by bigint NULL REFERENCES users (id),
    expires_at timestamptz NULL,
    max_uses integer NULL,
    used_count integer NOT NULL DEFAULT 0,
    revoked_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT group_invites_max_uses_check CHECK (max_uses IS NULL OR max_uses > 0),
    CONSTRAINT group_invites_used_count_check CHECK (used_count >= 0)
);

CREATE INDEX group_invites_group_id_idx ON group_invites (group_id);

CREATE TABLE admin_sessions (
    token_hash bytea PRIMARY KEY,
    csrf_token text NOT NULL,
    flash text NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE admin_sessions;
DROP TABLE group_invites;
DROP TABLE group_members;
DROP TABLE groups;
DROP TABLE users;
