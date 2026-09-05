//go:build integration

package database

import (
	"context"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/migrations"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

func migrationDB(t *testing.T) *DB {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Fatal("TEST_DATABASE_URL is required; run make test-integration")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatal("TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	admin, err := NewDB(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	schema := "migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schema) + " CASCADE"); err != nil {
			t.Error(err)
		}
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := NewDB(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func migrate(t *testing.T, db *DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := db.RunMigrations(ctx); err != nil {
		t.Fatal(err)
	}
}

func execSQL(t *testing.T, db *DB, sql string) {
	t.Helper()
	if _, err := db.Exec(sql); err != nil {
		t.Fatal(err)
	}
}

func assertSchema(t *testing.T, db *DB) {
	t.Helper()
	var versions int
	if err := db.Get(&versions, "SELECT count(*) FROM schema_migrations"); err != nil || versions != 8 {
		t.Fatalf("migration count = %d, error = %v", versions, err)
	}
	for _, table := range []string{"users", "sites", "site_aliases", "versions", "password_reset_tokens", "build_submissions"} {
		var exists bool
		if err := db.Get(&exists, "SELECT to_regclass($1) IS NOT NULL", table); err != nil || !exists {
			t.Fatalf("missing table %s: %v", table, err)
		}
	}
	var wideColumns int
	err := db.Get(&wideColumns, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND character_maximum_length = 255
		AND ((table_name = 'sites' AND column_name IN ('live_version_id', 'preview_version_id'))
		OR (table_name = 'versions' AND column_name = 'build_id'))`)
	if err != nil || wideColumns != 3 {
		t.Fatalf("version column widths: %d, %v", wideColumns, err)
	}
	var indexes int
	err = db.Get(&indexes, `SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema()
		AND indexname IN ('idx_site_aliases_alias', 'idx_versions_build_id')`)
	if err != nil || indexes != 2 {
		t.Fatalf("compatibility indexes: %d, %v", indexes, err)
	}
	var validated bool
	err = db.Get(&validated, `SELECT convalidated FROM pg_constraint
		WHERE conrelid = 'users'::regclass AND conname = 'email_or_oauth'`)
	if err != nil || !validated {
		t.Fatalf("auth constraint is not validated: %v", err)
	}
}

func TestMigrationsEmptyAndRestart(t *testing.T) {
	db := migrationDB(t)
	migrate(t, db)
	assertSchema(t, db)
	execSQL(t, db, `INSERT INTO users (id,email,password_hash) VALUES
		('11111111-1111-1111-1111-111111111111','kept@example.test','hash');
		INSERT INTO sites (id,user_id,fqdn,template_id) VALUES
		('22222222-2222-2222-2222-222222222222','11111111-1111-1111-1111-111111111111','kept.test','starter');
		INSERT INTO versions (site_id,build_id) VALUES
		('22222222-2222-2222-2222-222222222222','kept-build')`)
	migrate(t, db)
	assertSchema(t, db)
	var status string
	if err := db.Get(&status, "SELECT status FROM versions WHERE build_id='kept-build'"); err != nil || status != "pending" {
		t.Fatalf("restart lost data/default: %q, %v", status, err)
	}
}

// Reproduce the original startup schema's differences from the file migrations.
func legacySchema(t *testing.T, db *DB, tracked bool) {
	t.Helper()
	names, err := fs.Glob(migrations.Files, "00[1-5]*.up.sql")
	if err != nil || len(names) != 5 {
		t.Fatalf("legacy fixture files: %v, %v", names, err)
	}
	for _, name := range names {
		sql, err := migrations.Files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		execSQL(t, db, string(sql))
	}
	execSQL(t, db, `ALTER TABLE users DROP CONSTRAINT email_or_oauth;
		ALTER TABLE sites ALTER COLUMN live_version_id TYPE varchar(100);
		ALTER TABLE sites ALTER COLUMN preview_version_id TYPE varchar(100);
		ALTER TABLE versions ALTER COLUMN build_id TYPE varchar(100);
		ALTER TABLE versions ALTER COLUMN status DROP DEFAULT;
		DROP INDEX idx_site_aliases_alias;
		DROP INDEX idx_versions_build_id;
		INSERT INTO users(email,password_hash) VALUES ('legacy@example.test','preserved')`)
	if tracked {
		execSQL(t, db, `CREATE TABLE schema_migrations(version integer PRIMARY KEY, applied_at timestamp NOT NULL DEFAULT now());
			INSERT INTO schema_migrations(version) SELECT generate_series(1,5)`)
	}
}

func TestMigrationsLegacyUpgrade(t *testing.T) {
	for _, tracked := range []bool{true, false} {
		name := "manual"
		if tracked {
			name = "tracked"
		}
		t.Run(name, func(t *testing.T) {
			db := migrationDB(t)
			legacySchema(t, db, tracked)
			migrate(t, db)
			assertSchema(t, db)
			var hash string
			if err := db.Get(&hash, "SELECT password_hash FROM users WHERE email='legacy@example.test'"); err != nil || hash != "preserved" {
				t.Fatalf("legacy user lost: %q, %v", hash, err)
			}
			migrate(t, db)
		})
	}
}

func TestMigrationsRollbackAndRetry(t *testing.T) {
	db := migrationDB(t)
	source := fstest.MapFS{
		"001_probe.up.sql":  {Data: []byte("CREATE TABLE probe (id integer)")},
		"002_broken.up.sql": {Data: []byte("THIS IS INVALID SQL")},
	}
	if err := db.runMigrations(context.Background(), source); err == nil {
		t.Fatal("invalid migration unexpectedly succeeded")
	}
	var remains bool
	if err := db.Get(&remains, "SELECT to_regclass('probe') IS NOT NULL OR to_regclass('schema_migrations') IS NOT NULL"); err != nil || remains {
		t.Fatalf("failed migration left schema/tracking: %v, %v", remains, err)
	}
	source["002_broken.up.sql"] = &fstest.MapFile{Data: []byte("ALTER TABLE probe ADD COLUMN name text")}
	if err := db.runMigrations(context.Background(), source); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
}

func TestMigrationsRejectInvalidLegacyData(t *testing.T) {
	db := migrationDB(t)
	legacySchema(t, db, true)
	execSQL(t, db, "INSERT INTO users(email) VALUES ('invalid@example.test')")
	if err := db.RunMigrations(context.Background()); err == nil {
		t.Fatal("invalid legacy identity should fail validation")
	}
	var count int
	if err := db.Get(&count, "SELECT count(*) FROM schema_migrations"); err != nil || count != 5 {
		t.Fatalf("failed upgrade recorded a version: %d, %v", count, err)
	}
	if err := db.Get(&count, "SELECT count(*) FROM users WHERE email='invalid@example.test'"); err != nil || count != 1 {
		t.Fatal("migration must preserve invalid data for operator repair")
	}
}

func TestMigrationsConcurrentStartup(t *testing.T) {
	db := migrationDB(t)
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			errors <- db.RunMigrations(ctx)
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	assertSchema(t, db)
}

func TestMigrationsRejectFutureVersion(t *testing.T) {
	db := migrationDB(t)
	migrate(t, db)
	execSQL(t, db, "INSERT INTO schema_migrations(version) VALUES(999)")
	if err := db.RunMigrations(context.Background()); err == nil {
		t.Fatal("older binary must reject a newer database")
	}
}
