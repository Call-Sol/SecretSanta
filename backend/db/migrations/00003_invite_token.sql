-- +goose Up
-- Keep the raw invite token so the operator panel can show the deeplink again.
ALTER TABLE group_invites ADD COLUMN token text;

-- +goose Down
ALTER TABLE group_invites DROP COLUMN token;
