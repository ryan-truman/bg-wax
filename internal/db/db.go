package db

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Open opens (or creates) the SQLite database at path and runs the schema
// to ensure all tables exist. Call Close on the returned *sql.DB when done.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Foreign key enforcement is off by default in SQLite; turn it on.
	// WAL mode gives better read/write concurrency for a local app.
	if _, err := db.Exec(`
		PRAGMA foreign_keys = ON;
		PRAGMA journal_mode = WAL;
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set pragmas: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("run schema: %w", err)
	}

	// Migrations for columns added after initial schema (errors ignored — column may already exist).
	db.Exec(`ALTER TABLE competitors ADD COLUMN removed INTEGER NOT NULL DEFAULT 0`)

	return db, nil
}
