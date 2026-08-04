package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store wraps the underlying SQLite connection for the annalist database.
type Store struct {
	db *sql.DB
}

// New opens (creating if needed) the SQLite database at {dataDir}/app.db and
// applies migrations. The driver "sqlite" is registered by modernc.org/sqlite.
func New(dataDir string) (*Store, error) {
	// 0700: the data dir holds the settings DB and (as clone parent) full git
	// checkouts of the operator's repositories — other local users must not
	// be able to list or read them.
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating data dir %q: %w", dataDir, err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)",
		filepath.Join(dataDir, "app.db"))
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	s := &Store{db: conn}
	if err := s.migrate(); err != nil {
		conn.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
