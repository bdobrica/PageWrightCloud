//go:build integration

package database

import (
	"bytes"
	"context"
	"io/fs"
	"net/url"
	"os"
	"testing"
	"testing/fstest"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/migrations"
)

func TestBootstrapMigrationReservationAndReconnect(t *testing.T) {
	db := migrationDB(t)
	old := fstest.MapFS{}
	names, err := fs.Glob(migrations.Files, "00[1-7]*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		body, err := migrations.Files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		old[name] = &fstest.MapFile{Data: body}
	}
	if err := db.runMigrations(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `INSERT INTO users(id,email,password_hash) VALUES('11111111-1111-1111-1111-111111111111','bootstrap-upgrade@test','hash'); INSERT INTO sites(id,user_id,fqdn,template_id) VALUES('22222222-2222-2222-2222-222222222222','11111111-1111-1111-1111-111111111111','legacy-bootstrap.test','starter')`)
	migrate(t, db)
	migrate(t, db)
	legacy, err := db.GetSiteByFQDN("legacy-bootstrap.test")
	if err != nil || legacy.InitializationStatus != "legacy" {
		t.Fatalf("legacy altered: %v %v", legacy, err)
	}
	owner := legacy.UserID
	execSQL(t, db, `CREATE FUNCTION reject_bootstrap_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test rollback'; END $$; CREATE TRIGGER reject_bootstrap_insert BEFORE INSERT ON site_bootstraps FOR EACH ROW EXECUTE FUNCTION reject_bootstrap_insert()`)
	if _, _, err := db.ReserveSiteBootstrap(context.Background(), owner, "rollback-bootstrap.test"); err == nil {
		t.Fatal("expected reservation failure")
	}
	site, err := db.GetSiteByFQDN("rollback-bootstrap.test")
	if err != nil || site != nil {
		t.Fatal("failed reservation left a site")
	}
	execSQL(t, db, `DROP TRIGGER reject_bootstrap_insert ON site_bootstraps; DROP FUNCTION reject_bootstrap_insert()`)
	site, record, err := db.ReserveSiteBootstrap(context.Background(), owner, "resume-bootstrap.test")
	if err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := db.Get(&schema, `SELECT current_schema()`); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	reconnected, err := NewDB(u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer reconnected.Close()
	again, saved, err := reconnected.ReserveSiteBootstrap(context.Background(), owner, site.FQDN)
	if err != nil || again.ID != site.ID || !bytes.Equal(saved.Archive, record.Archive) || !bytes.Equal(saved.Manifest, record.Manifest) || !bytes.Equal(saved.ExecutionLog, record.ExecutionLog) {
		t.Fatalf("reservation changed on reconnect: %v", err)
	}
}
