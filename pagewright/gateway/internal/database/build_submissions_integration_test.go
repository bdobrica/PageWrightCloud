//go:build integration

package database

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/migrations"
	"github.com/google/uuid"
)

func submissionFixture(t *testing.T) (*DB, *BuildSubmission) {
	t.Helper()
	db := migrationDB(t)
	migrate(t, db)
	p := &BuildSubmission{JobID: uuid.NewString(), SiteID: uuid.NewString(), OwnerID: uuid.NewString(), SourceVersion: "initial", TargetVersion: uuid.NewString(), Prompt: "Change title", RequestKey: uuid.NewString(), RequestHash: strings.Repeat("a", 64)}
	if _, err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES($1,$2,'hash')`, p.OwnerID, p.OwnerID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sites(id,user_id,fqdn,template_id) VALUES($1,$2,$3,'starter')`, p.SiteID, p.OwnerID, p.SiteID+".test"); err != nil {
		t.Fatal(err)
	}
	return db, p
}

func assertSubmissionCounts(t *testing.T, db *DB, count int) {
	t.Helper()
	for _, table := range []string{"versions", "build_submissions"} {
		var actual int
		if err := db.Get(&actual, "SELECT count(*) FROM "+table); err != nil || actual != count {
			t.Fatalf("%s count=%d want=%d error=%v", table, actual, count, err)
		}
	}
}

func TestBuildSubmissionReservationAndReplay(t *testing.T) {
	db, p := submissionFixture(t)
	ctx := context.Background()
	missing, err := db.FindBuildSubmission(ctx, p.OwnerID, p.SiteID, p.RequestKey)
	if err != nil || missing != nil {
		t.Fatalf("missing=%v error=%v", missing, err)
	}
	got, created, err := db.ReserveBuildSubmission(ctx, p)
	if err != nil || !created {
		t.Fatalf("reserve created=%v error=%v", created, err)
	}
	if got.JobID != p.JobID || got.TargetVersion != p.TargetVersion || got.JobID == got.TargetVersion || got.DispatchState != "ready" || got.Status != "pending" || got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("bad reservation %+v", got)
	}
	assertSubmissionCounts(t, db, 1)
	retry := *p
	retry.JobID = uuid.NewString()
	retry.TargetVersion = uuid.NewString()
	retry.Prompt = "Different nondeterministic provider instructions"
	replayed, created, err := db.ReserveBuildSubmission(ctx, &retry)
	if err != nil || created || replayed.JobID != got.JobID || replayed.Prompt != got.Prompt {
		t.Fatalf("replay=%+v created=%v error=%v", replayed, created, err)
	}
	retry.RequestHash = strings.Repeat("b", 64)
	if _, _, err := db.ReserveBuildSubmission(ctx, &retry); !errors.Is(err, ErrSubmissionConflict) {
		t.Fatalf("conflict error=%v", err)
	}
	assertSubmissionCounts(t, db, 1)
	retry.RequestKey = uuid.NewString()
	if _, created, err := db.ReserveBuildSubmission(ctx, &retry); err != nil || !created {
		t.Fatalf("new request: %v %v", created, err)
	}
	assertSubmissionCounts(t, db, 2)
}

func TestBuildSubmissionRollbackAndOwnership(t *testing.T) {
	for _, mode := range []string{"invalid_hash", "wrong_owner", "duplicate_target", "duplicate_job", "same_job_target", "nil_job"} {
		t.Run(mode, func(t *testing.T) {
			db, p := submissionFixture(t)
			count := 0
			if strings.HasPrefix(mode, "duplicate") {
				if _, _, err := db.ReserveBuildSubmission(context.Background(), p); err != nil {
					t.Fatal(err)
				}
				count = 1
			}
			p.RequestKey = uuid.NewString()
			switch mode {
			case "invalid_hash":
				p.RequestHash = "invalid"
			case "wrong_owner":
				p.OwnerID = uuid.NewString()
			case "duplicate_target":
				p.JobID = uuid.NewString()
			case "duplicate_job":
				p.TargetVersion = uuid.NewString()
			case "same_job_target":
				p.TargetVersion = p.JobID
			case "nil_job":
				p.JobID = uuid.Nil.String()
			}
			if _, _, err := db.ReserveBuildSubmission(context.Background(), p); err == nil {
				t.Fatal("invalid reservation accepted")
			}
			assertSubmissionCounts(t, db, count)
		})
	}
}

func TestBuildSubmissionSiteDeletionCascades(t *testing.T) {
	db, p := submissionFixture(t)
	if _, _, err := db.ReserveBuildSubmission(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteSite(p.SiteID + ".test"); err != nil {
		t.Fatalf("submission mapping blocks existing site deletion: %v", err)
	}
	assertSubmissionCounts(t, db, 0)
}

func TestBuildSubmissionConcurrentReservationAndClaim(t *testing.T) {
	db, p := submissionFixture(t)
	const contenders = 8
	var wg sync.WaitGroup
	results := make(chan *BuildSubmission, contenders)
	createdResults := make(chan bool, contenders)
	errorsCh := make(chan error, contenders)
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			proposal := *p
			proposal.JobID = uuid.NewString()
			proposal.TargetVersion = uuid.NewString()
			got, created, err := db.ReserveBuildSubmission(context.Background(), &proposal)
			results <- got
			createdResults <- created
			errorsCh <- err
		}()
	}
	wg.Wait()
	close(results)
	close(createdResults)
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	var jobID string
	for got := range results {
		if jobID == "" {
			jobID = got.JobID
		}
		if got.JobID != jobID {
			t.Fatal("retry allocated another job")
		}
	}
	createdCount := 0
	for created := range createdResults {
		if created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("created count %d", createdCount)
	}
	assertSubmissionCounts(t, db, 1)
	claims := make(chan bool, contenders)
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := db.ClaimBuildDispatch(context.Background(), jobID)
			if err != nil {
				t.Error(err)
			}
			claims <- claimed
		}()
	}
	wg.Wait()
	close(claims)
	claimCount := 0
	for claimed := range claims {
		if claimed {
			claimCount++
		}
	}
	if claimCount != 1 {
		t.Fatalf("claim count %d", claimCount)
	}
}

func TestBuildSubmissionOutcomeAtomicity(t *testing.T) {
	db, p := submissionFixture(t)
	ctx := context.Background()
	if _, _, err := db.ReserveBuildSubmission(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordBuildOutcome(ctx, p.JobID, "accepted", "running", "", "", 201); !errors.Is(err, ErrSubmissionConflict) {
		t.Fatalf("unclaimed outcome=%v", err)
	}
	if claimed, err := db.ClaimBuildDispatch(ctx, p.JobID); err != nil || !claimed {
		t.Fatalf("claim=%v error=%v", claimed, err)
	}
	// Force the second write to fail and verify the first update rolls back.
	execSQL(t, db, `ALTER TABLE versions ADD CONSTRAINT test_pending_only CHECK(status='pending')`)
	if err := db.RecordBuildOutcome(ctx, p.JobID, "accepted", "running", "", "", 201); err == nil {
		t.Fatal("version rejection did not fail outcome")
	}
	got, err := db.FindBuildSubmission(ctx, p.OwnerID, p.SiteID, p.RequestKey)
	if err != nil || got.DispatchState != "dispatching" || got.Status != "pending" {
		t.Fatalf("partial outcome=%+v %v", got, err)
	}
	execSQL(t, db, `ALTER TABLE versions DROP CONSTRAINT test_pending_only`)
	if err := db.RecordBuildOutcome(ctx, p.JobID, "rejected", "failed", "site_busy", "site has active job", 409); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordBuildOutcome(ctx, p.JobID, "rejected", "failed", "site_busy", "site has active job", 409); err != nil {
		t.Fatal("identical replay", err)
	}
	if err := db.RecordBuildOutcome(ctx, p.JobID, "accepted", "running", "", "", 201); !errors.Is(err, ErrSubmissionConflict) {
		t.Fatalf("late overwrite=%v", err)
	}
	got, err = db.FindBuildSubmission(ctx, p.OwnerID, p.SiteID, p.RequestKey)
	if err != nil || got.DispatchState != "rejected" || got.ErrorCode != "site_busy" || got.ResponseStatus != 409 || got.Prompt != p.Prompt || got.TargetVersion != p.TargetVersion {
		t.Fatalf("stored=%+v %v", got, err)
	}
	var status string
	if err := db.Get(&status, `SELECT status FROM versions WHERE site_id=$1 AND build_id=$2`, p.SiteID, p.TargetVersion); err != nil || status != "failed" {
		t.Fatalf("version=%s %v", status, err)
	}
}

func TestBuildSubmissionSurvivesReconnect(t *testing.T) {
	db, p := submissionFixture(t)
	ctx := context.Background()
	if _, _, err := db.ReserveBuildSubmission(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimBuildDispatch(ctx, p.JobID); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordBuildOutcome(ctx, p.JobID, "accepted", "running", "", "", 201); err != nil {
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
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewDB(u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	migrate(t, reopened)
	got, err := reopened.FindBuildSubmission(ctx, p.OwnerID, p.SiteID, p.RequestKey)
	if err != nil || got.JobID != p.JobID || got.DispatchState != "accepted" || got.Status != "running" {
		t.Fatalf("reopened=%+v %v", got, err)
	}
	assertSubmissionCounts(t, reopened, 1)
}

func TestMigrationsSixToSeven(t *testing.T) {
	db := migrationDB(t)
	old := fstest.MapFS{}
	names, err := fs.Glob(migrations.Files, "00[1-6]*.up.sql")
	if err != nil || len(names) != 6 {
		t.Fatalf("old migrations=%v %v", names, err)
	}
	for _, name := range names {
		data, err := migrations.Files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		old[name] = &fstest.MapFile{Data: data}
	}
	if err := db.runMigrations(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `INSERT INTO users(id,email,password_hash) VALUES('11111111-1111-1111-1111-111111111111','legacy-mapping@test','hash'); INSERT INTO sites(id,user_id,fqdn,template_id) VALUES('22222222-2222-2222-2222-222222222222','11111111-1111-1111-1111-111111111111','legacy-mapping.test','starter'); INSERT INTO versions(site_id,build_id) VALUES('22222222-2222-2222-2222-222222222222','legacy-job-id')`)
	migrate(t, db)
	assertSchema(t, db)
	migrate(t, db)
	var count int
	if err := db.Get(&count, `SELECT count(*) FROM versions WHERE build_id='legacy-job-id'`); err != nil || count != 1 {
		t.Fatalf("legacy versions=%d %v", count, err)
	}
	if err := db.Get(&count, `SELECT count(*) FROM build_submissions`); err != nil || count != 0 {
		t.Fatalf("guessed mappings=%d %v", count, err)
	}
}
