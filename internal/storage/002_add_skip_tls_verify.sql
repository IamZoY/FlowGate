-- Migration v2: add skip_tls_verify columns for self-signed certificate support.
ALTER TABLE apps ADD COLUMN src_skip_tls_verify INTEGER NOT NULL DEFAULT 0;
ALTER TABLE apps ADD COLUMN dst_skip_tls_verify INTEGER NOT NULL DEFAULT 0;

INSERT OR IGNORE INTO schema_version VALUES (2, unixepoch());
