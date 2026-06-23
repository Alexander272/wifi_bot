-- +goose Up
CREATE TABLE IF NOT EXISTS hotspot_sessions (
    id BIGSERIAL PRIMARY KEY,
    username TEXT NOT NULL,
    mac TEXT NOT NULL,
    address TEXT NOT NULL DEFAULT '',
    uptime TEXT NOT NULL DEFAULT '',
    bytes_in BIGINT NOT NULL DEFAULT 0,
    bytes_out BIGINT NOT NULL DEFAULT 0,
    packets_in BIGINT NOT NULL DEFAULT 0,
    packets_out BIGINT NOT NULL DEFAULT 0,
    idle_time TEXT NOT NULL DEFAULT '',
    server TEXT NOT NULL DEFAULT '',
    collected_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_hotspot_sessions_collected_at ON hotspot_sessions(collected_at);

-- +goose Down
DROP TABLE IF EXISTS hotspot_sessions;
