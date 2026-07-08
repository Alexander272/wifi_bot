-- +goose Up
ALTER TABLE user_sessions ADD COLUMN ttl_duration BIGINT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE user_sessions DROP COLUMN ttl_duration;
