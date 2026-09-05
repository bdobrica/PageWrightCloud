package database

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/migrations"
)

// RunMigrations atomically applies pending embedded migrations and their records.
func (db *DB) RunMigrations(ctx context.Context) error {
	return db.runMigrations(ctx, migrations.Files)
}

func (db *DB) runMigrations(ctx context.Context, source fs.FS) error {
	names, err := fs.Glob(source, "*.up.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	if len(names) == 0 {
		return fmt.Errorf("no up migrations found")
	}
	type migration struct {
		version int
		name    string
		sql     string
	}
	pending := make([]migration, 0, len(names))
	for _, name := range names {
		prefix, _, ok := strings.Cut(name, "_")
		version, err := strconv.Atoi(prefix)
		if !ok || err != nil || version < 1 {
			return fmt.Errorf("invalid migration filename %q", name)
		}
		body, err := fs.ReadFile(source, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if strings.TrimSpace(string(body)) == "" {
			return fmt.Errorf("empty migration %s", name)
		}
		pending = append(pending, migration{version: version, name: name, sql: string(body)})
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].version < pending[j].version })
	for i, migration := range pending {
		if migration.version != i+1 {
			return fmt.Errorf("migration versions must be consecutive from 1: expected %d, found %d (%s)", i+1, migration.version, migration.name)
		}
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migrations: %w", err)
	}
	defer tx.Rollback()
	// A transaction-scoped lock serializes startup before even the tracking-table
	// DDL. PostgreSQL releases it on commit, rollback, or connection loss.
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", int64(0x506167655772)); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL DEFAULT NOW()
	)`); err != nil {
		return fmt.Errorf("create migration tracking table: %w", err)
	}
	var applied []int
	if err := tx.SelectContext(ctx, &applied, "SELECT version FROM schema_migrations ORDER BY version"); err != nil {
		return fmt.Errorf("read migration history: %w", err)
	}
	for i, version := range applied {
		if version > len(pending) {
			return fmt.Errorf("database migration version %d is newer than supported version %d; use a compatible gateway binary", version, len(pending))
		}
		if version != i+1 {
			return fmt.Errorf("migration history is not consecutive: expected version %d, found %d", i+1, version)
		}
	}
	for _, migration := range pending[len(applied):] {
		if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.name, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", migration.version); err != nil {
			return fmt.Errorf("record migration %s: %w", migration.name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}
