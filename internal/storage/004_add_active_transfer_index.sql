-- Migration v4: composite index for dedup lookups on active transfers.

CREATE INDEX IF NOT EXISTS idx_transfers_active_lookup
ON transfers(app_id, object_key, status);

INSERT OR IGNORE INTO schema_version VALUES (4, unixepoch());
