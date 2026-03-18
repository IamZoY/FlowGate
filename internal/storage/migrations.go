package storage

import (
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed 001_initial_schema.sql
var schema001 string

// Migrate applies all schema migrations to db in order.
// It is idempotent: safe to call on every startup.
func Migrate(db *sql.DB) error {
	// Enable WAL mode and foreign keys for every connection.
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("pragma %q: %w", p, err)
		}
	}

	// Apply schema v1 if not already present.
	var version int
	row := db.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_version`)
	_ = row.Scan(&version) // table may not exist yet — ignore error

	if version < 1 {
		if _, err := db.Exec(schema001); err != nil {
			return fmt.Errorf("apply schema v1: %w", err)
		}
	}

	return nil
}
