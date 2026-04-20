package db

import "database/sql"

// Store wraps a *sql.DB connection pool and provides methods for common
// database operations. Use this instead of the package-level DB global
// for new code. Existing code can migrate incrementally.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store from an existing *sql.DB connection.
func NewStore(database *sql.DB) *Store {
	return &Store{db: database}
}

// DefaultStore returns a Store wrapping the package-level DB global.
// Use this as a bridge during migration from the global to DI.
func DefaultStore() *Store {
	return &Store{db: DB}
}

// Conn returns the underlying *sql.DB for raw queries.
func (s *Store) Conn() *sql.DB {
	return s.db
}

// Ping verifies the database connection is alive.
func (s *Store) Ping() error {
	return s.db.Ping()
}

// QueryRow is a convenience wrapper.
func (s *Store) QueryRow(query string, args ...interface{}) *sql.Row {
	return s.db.QueryRow(query, args...)
}

// Query is a convenience wrapper.
func (s *Store) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return s.db.Query(query, args...)
}

// Exec is a convenience wrapper.
func (s *Store) Exec(query string, args ...interface{}) (sql.Result, error) {
	return s.db.Exec(query, args...)
}

// BeginTx starts a transaction.
func (s *Store) BeginTx() (*sql.Tx, error) {
	return s.db.Begin()
}
