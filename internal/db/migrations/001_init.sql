CREATE TABLE IF NOT EXISTS app_config (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS models (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    provider        TEXT NOT NULL,
    base_url        TEXT NOT NULL,
    api_key         TEXT NOT NULL,
    model_id        TEXT NOT NULL,
    reasoning       INTEGER DEFAULT 5,
    coding          INTEGER DEFAULT 5,
    creativity      INTEGER DEFAULT 5,
    speed           INTEGER DEFAULT 5,
    cost_efficiency INTEGER DEFAULT 5,
    max_rpm         INTEGER DEFAULT 60,
    max_tpm         INTEGER DEFAULT 100000,
    is_active       INTEGER DEFAULT 1,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS conversations (
    id          TEXT PRIMARY KEY,
    title       TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT,
    role            TEXT NOT NULL,
    content         TEXT NOT NULL,
    model_id        TEXT,
    complexity      TEXT,
    tokens_in       INTEGER DEFAULT 0,
    tokens_out      INTEGER DEFAULT 0,
    latency_ms      INTEGER DEFAULT 0,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS request_logs (
    id          TEXT PRIMARY KEY,
    model_id    TEXT NOT NULL,
    source      TEXT NOT NULL,
    complexity  TEXT,
    route_mode  TEXT NOT NULL,
    status      TEXT NOT NULL,
    tokens_in   INTEGER DEFAULT 0,
    tokens_out  INTEGER DEFAULT 0,
    latency_ms  INTEGER DEFAULT 0,
    error_msg   TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO app_config (key, value) VALUES ('proxy_port', '9680');
INSERT OR IGNORE INTO app_config (key, value) VALUES ('proxy_enabled', 'false');
