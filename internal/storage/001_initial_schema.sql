-- FlowGate initial schema
-- Applied once by internal/storage/migrations.go on startup.

CREATE TABLE IF NOT EXISTS groups (
    id          TEXT    PRIMARY KEY,
    name        TEXT    NOT NULL UNIQUE,
    description TEXT    NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS apps (
    id               TEXT    PRIMARY KEY,
    group_id       TEXT    NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    name             TEXT    NOT NULL,
    description      TEXT    NOT NULL DEFAULT '',

    -- source MinIO (GetObject)
    src_endpoint     TEXT    NOT NULL,
    src_access_key   TEXT    NOT NULL,
    src_secret_key   TEXT    NOT NULL, -- AES-GCM encrypted
    src_bucket       TEXT    NOT NULL,
    src_region       TEXT    NOT NULL DEFAULT 'us-east-1',
    src_use_ssl      INTEGER NOT NULL DEFAULT 0,

    -- destination MinIO (PutObject)
    dst_endpoint     TEXT    NOT NULL,
    dst_access_key   TEXT    NOT NULL,
    dst_secret_key   TEXT    NOT NULL, -- AES-GCM encrypted
    dst_bucket       TEXT    NOT NULL,
    dst_region       TEXT    NOT NULL DEFAULT 'us-east-1',
    dst_use_ssl      INTEGER NOT NULL DEFAULT 0,

    webhook_secret   TEXT    NOT NULL, -- per-app HMAC Authorization token
    enabled          INTEGER NOT NULL DEFAULT 1,
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,

    UNIQUE(group_id, name)
);

-- Used by GetAppByRoute: resolves (group_slug, app_slug) → App in one query.
CREATE INDEX IF NOT EXISTS idx_apps_route ON apps(name, group_id);

CREATE TABLE IF NOT EXISTS transfers (
    id                TEXT    PRIMARY KEY,
    app_id            TEXT    NOT NULL REFERENCES apps(id),
    object_key        TEXT    NOT NULL,
    src_bucket        TEXT    NOT NULL,
    dst_bucket        TEXT    NOT NULL,
    object_size       INTEGER NOT NULL DEFAULT 0,
    etag              TEXT    NOT NULL DEFAULT '',
    status            TEXT    NOT NULL DEFAULT 'pending', -- pending|in_progress|success|failed
    error_message     TEXT    NOT NULL DEFAULT '',
    bytes_transferred INTEGER NOT NULL DEFAULT 0,
    started_at        INTEGER,          -- nullable Unix seconds
    completed_at      INTEGER,          -- nullable Unix seconds
    duration_ms       REAL    NOT NULL DEFAULT 0,
    created_at        INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_transfers_app_id ON transfers(app_id);
CREATE INDEX IF NOT EXISTS idx_transfers_status  ON transfers(status);
CREATE INDEX IF NOT EXISTS idx_transfers_created ON transfers(created_at DESC);

-- Schema version tracking.
CREATE TABLE IF NOT EXISTS schema_version (
    version    INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL
);
INSERT OR IGNORE INTO schema_version VALUES (1, unixepoch());
