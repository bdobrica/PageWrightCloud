package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type DB struct {
	*sqlx.DB
	ctx context.Context
}

func NewDB(connectionString string) (*DB, error) {
	db, err := sqlx.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{DB: db}, nil
}

// WithContext returns a request-local view of the shared pool. Never mutate the
// context on the application-wide DB; explicit Context methods remain unchanged.
func (db *DB) WithContext(ctx context.Context) *DB { return &DB{DB: db.DB, ctx: ctx} }
func (db *DB) operationContext() context.Context {
	if db.ctx != nil {
		return db.ctx
	}
	return context.Background()
}
func (db *DB) Get(dest interface{}, query string, args ...interface{}) error {
	return db.DB.GetContext(db.operationContext(), dest, query, args...)
}
func (db *DB) Select(dest interface{}, query string, args ...interface{}) error {
	return db.DB.SelectContext(db.operationContext(), dest, query, args...)
}
func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return db.DB.ExecContext(db.operationContext(), query, args...)
}
func (db *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return db.DB.QueryRowContext(db.operationContext(), query, args...)
}
func (db *DB) Beginx() (*sqlx.Tx, error) { return db.DB.BeginTxx(db.operationContext(), nil) }

func (db *DB) Close() error {
	return db.DB.Close()
}
