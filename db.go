package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// openDB opens (and creates if necessary) the SQLite database used to store
// notes. The database lives in the user's configuration directory so that it
// survives application updates.
func openDB() (*sql.DB, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	dbDir := filepath.Join(configDir, "PinNote")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return openDBAt(filepath.Join(dbDir, "pinnote.db"))
}

// openDBAt opens the SQLite database at an explicit path (used by tests to
// run against an in-memory database).
func openDBAt(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// SQLite allows exactly one writer at a time; serialise access, keep
	// foreign keys enforced, and bound the pool so writers never queue behind
	// many idle read connections.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply pragma %q: %w", p, err)
		}
	}
	db.SetMaxOpenConns(1)

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS notes (
	id          TEXT PRIMARY KEY,
	title       TEXT NOT NULL DEFAULT '',
	content     TEXT NOT NULL DEFAULT '',
	pinned      INTEGER NOT NULL DEFAULT 0,
	deleted_at  INTEGER,
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_notes_deleted_at ON notes(deleted_at);
CREATE INDEX IF NOT EXISTS idx_notes_updated_at ON notes(updated_at);
CREATE INDEX IF NOT EXISTS idx_notes_pinned_updated ON notes(pinned DESC, updated_at DESC);
`
	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	return nil
}
