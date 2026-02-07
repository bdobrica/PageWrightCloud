package database

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type DB struct {
	*sqlx.DB
}

func NewDB(connectionString string) (*DB, error) {
	db, err := sqlx.Connect("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{db}, nil
}

func (db *DB) Close() error {
	return db.DB.Close()
}

// RunMigrations executes SQL migration files
// In production, consider using a migration tool like golang-migrate
func (db *DB) RunMigrations(migrationsPath string) error {
	// This is a simplified version - in production use golang-migrate or similar
	// For now, migrations are applied manually or via docker-compose init scripts
	return nil
}
