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
	// Pragmas live in the DSN so they apply to every connection the driver
	// opens, not just the first one (PRAGMAs are per-connection in SQLite).
	//   foreign_keys  – enforce FK constraints (off by default in SQLite)
	//   journal_mode  – WAL for better read/write concurrency
	//   busy_timeout  – wait up to 5s for a lock instead of failing immediately
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// This is a single-user local app: capping the pool at one connection
	// eliminates lock contention entirely and guarantees that pragmas and
	// transactions all share the same underlying SQLite session.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("run schema: %w", err)
	}

	// Migrations for columns added after initial schema (errors ignored — column may already exist).
	db.Exec(`ALTER TABLE competitors ADD COLUMN removed INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE competitors ADD COLUMN name_edited INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE competitors ADD COLUMN order_id TEXT`)
	db.Exec(`ALTER TABLE matches ADD COLUMN bracket INTEGER`)

	return db, nil
}
