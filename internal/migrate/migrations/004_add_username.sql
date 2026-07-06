-- +goose Up
ALTER TABLE auth_logs ADD COLUMN IF NOT EXISTS username TEXT NOT NULL DEFAULT '';
ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS username TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE auth_logs DROP COLUMN IF EXISTS username;
ALTER TABLE user_sessions DROP COLUMN IF EXISTS username;
