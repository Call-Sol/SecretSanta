-- +goose Up
-- One reusable link per game. Starting the bot with it confirms participation.

ALTER TABLE games ADD COLUMN invite_token text NULL;
ALTER TABLE games ADD COLUMN invite_token_hash bytea NULL;

ALTER TABLE games ADD CONSTRAINT games_invite_token_len CHECK (
    invite_token IS NULL OR char_length(invite_token) BETWEEN 16 AND 64
);

CREATE UNIQUE INDEX games_invite_token_hash_unique
    ON games (invite_token_hash)
    WHERE invite_token_hash IS NOT NULL;

-- +goose Down
DROP INDEX games_invite_token_hash_unique;
ALTER TABLE games DROP CONSTRAINT games_invite_token_len;
ALTER TABLE games DROP COLUMN invite_token_hash;
ALTER TABLE games DROP COLUMN invite_token;
