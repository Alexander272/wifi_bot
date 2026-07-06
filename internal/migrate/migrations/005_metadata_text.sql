-- +goose Up
ALTER TABLE auth_logs ALTER COLUMN metadata TYPE TEXT USING metadata::text;

-- +goose Down
ALTER TABLE auth_logs ALTER COLUMN metadata TYPE JSONB USING metadata::jsonb;
