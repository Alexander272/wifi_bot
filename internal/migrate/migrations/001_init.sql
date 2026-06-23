-- +goose Up
CREATE TABLE IF NOT EXISTS auth_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     TEXT NOT NULL,
    action      TEXT NOT NULL,
    code        TEXT,
    mac         TEXT,
    ip          TEXT,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_auth_logs_user_id ON auth_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_auth_logs_created_at ON auth_logs(created_at);

-- +goose Down
DROP TABLE IF EXISTS auth_logs;
