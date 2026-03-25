-- Migration v3: add per-app retry config and per-transfer retry tracking.

-- Per-app retry configuration (0 = use global default).
ALTER TABLE apps ADD COLUMN retry_max_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE apps ADD COLUMN retry_backoff_seconds INTEGER NOT NULL DEFAULT 0;

-- Per-transfer retry state.
ALTER TABLE transfers ADD COLUMN retry_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE transfers ADD COLUMN max_retries INTEGER NOT NULL DEFAULT 0;
ALTER TABLE transfers ADD COLUMN next_retry_at INTEGER;

INSERT OR IGNORE INTO schema_version VALUES (3, unixepoch());
