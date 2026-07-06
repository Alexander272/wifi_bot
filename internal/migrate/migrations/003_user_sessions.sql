-- +goose Up
CREATE TABLE IF NOT EXISTS user_sessions (
    id         BIGSERIAL PRIMARY KEY,
    user_id    TEXT NOT NULL,
    code       TEXT NOT NULL,
    mac        TEXT NOT NULL,
    ip         TEXT NOT NULL DEFAULT '',
    login_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    logout_at  TIMESTAMPTZ,
    is_active  BOOLEAN NOT NULL DEFAULT true
);

CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_mac ON user_sessions(mac);
CREATE INDEX IF NOT EXISTS idx_user_sessions_active ON user_sessions(is_active);

-- +goose Down
DROP TABLE IF EXISTS user_sessions;
