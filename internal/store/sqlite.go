package store

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGO)
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps the sql.DB connection with application-specific helpers.
type DB struct {
	Conn *sql.DB
}

// New opens a SQLite database at the given path, applies PRAGMAs,
// and runs all embedded migrations.
func New(dbPath string) (*DB, error) {
	// Ensure the directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("store: create data dir: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite: %w", err)
	}

	// Security & performance PRAGMAs
	pragmas := []string{
		"PRAGMA journal_mode=WAL",          // write-ahead log for concurrency
		"PRAGMA foreign_keys=ON",           // enforce FK constraints
		"PRAGMA busy_timeout=5000",         // wait up to 5s on lock
		"PRAGMA synchronous=NORMAL",        // balance durability/speed
		"PRAGMA cache_size=-64000",         // 64MB cache
		"PRAGMA temp_store=MEMORY",         // temp tables in memory
	}
	for _, p := range pragmas {
		if _, err := conn.Exec(p); err != nil {
			conn.Close()
			return nil, fmt.Errorf("store: pragma %q: %w", p, err)
		}
	}

	db := &DB{Conn: conn}

	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("store: migrate: %w", err)
	}

	return db, nil
}

// migrate reads embedded SQL files and executes them in order.
func (db *DB) migrate() error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if _, err := db.Conn.Exec(string(content)); err != nil {
			return fmt.Errorf("exec migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// Ping verifies the database is reachable.
func (db *DB) Ping() error {
	return db.Conn.Ping()
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.Conn.Close()
}
