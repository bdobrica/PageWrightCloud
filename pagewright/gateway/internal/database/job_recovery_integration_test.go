//go:build integration

package database

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
)

func recoveryJob(s *BuildSubmission) *types.Job {
	return &types.Job{JobAccepted: types.JobAccepted{JobID: s.JobID, SiteID: s.SiteID, OwnerID: s.OwnerID, SourceVersion: s.SourceVersion, TargetVersion: s.TargetVersion, Status: types.JobStatusRunning}, Prompt: s.Prompt, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
}

func TestRecoveryHistoryAtomicAndMonotonic(t *testing.T) {
	db, p := submissionFixture(t)
	ctx := context.Background()
	s, _, err := db.ReserveBuildSubmission(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	job := recoveryJob(s)
	if err := db.ObserveBuildJob(ctx, job); !errors.Is(err, ErrSubmissionConflict) {
		t.Fatal("unclaimed job accepted")
	}
	if _, err := db.ClaimBuildDispatch(ctx, s.JobID); err != nil {
		t.Fatal(err)
	}
	wrong := *job
	wrong.TargetVersion = "other"
	if err := db.ObserveBuildJob(ctx, &wrong); !errors.Is(err, ErrSubmissionConflict) {
		t.Fatal("wrong identity accepted")
	}
	if err := db.ObserveBuildJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	pending := *job
	pending.Status = types.JobStatusPending
	if err := db.ObserveBuildJob(ctx, &pending); !errors.Is(err, ErrSubmissionConflict) {
		t.Fatal("running regressed")
	}
	job.Status = types.JobStatusFailed
	job.ErrorMessage = "reserved output incomplete"
	job.ErrorCode = "artifact_incomplete"
	// A failed journal insert must roll back both submission and version changes.
	execSQL(t, db, `CREATE FUNCTION deny_history() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced history failure'; END $$; CREATE TRIGGER deny_history BEFORE INSERT ON job_history FOR EACH ROW EXECUTE FUNCTION deny_history()`)
	if err := db.ObserveBuildJob(ctx, job); err == nil {
		t.Fatal("journal failure ignored")
	}
	got, err := db.FindBuildSubmission(ctx, s.OwnerID, s.SiteID, s.RequestKey)
	if err != nil || got.Status != "running" {
		t.Fatal("partial outcome persisted")
	}
	execSQL(t, db, `DROP TRIGGER deny_history ON job_history; DROP FUNCTION deny_history()`)
	if err := db.ObserveBuildJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := db.ObserveBuildJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	job.Status = types.JobStatusCompleted
	job.ErrorMessage = ""
	job.ErrorCode = ""
	job.ManifestPath = "/sites/" + job.SiteID + "/artifacts/" + job.TargetVersion + "/manifest"
	if err := db.ObserveBuildJob(ctx, job); !errors.Is(err, ErrSubmissionConflict) {
		t.Fatal("late materialization changed terminal")
	}
	var count int
	if err := db.Get(&count, `SELECT count(*) FROM job_history WHERE job_id=$1`, s.JobID); err != nil || count != 3 {
		t.Fatalf("history %d %v", count, err)
	}
	var status string
	if err := db.Get(&status, `SELECT status FROM versions WHERE site_id=$1 AND build_id=$2`, s.SiteID, s.TargetVersion); err != nil || status != "failed" {
		t.Fatalf("version status %s %v", status, err)
	}
}

func TestRecoveryBatchClaimsAndMissingEvidence(t *testing.T) {
	db, p := submissionFixture(t)
	ctx := context.Background()
	s, _, err := db.ReserveBuildSubmission(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimBuildDispatch(ctx, s.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE build_submissions SET updated_at=now()-interval '1 minute' WHERE job_id=$1`, s.JobID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan []BuildSubmission, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := db.RecoveryBatch(ctx)
			if err != nil {
				t.Error(err)
			}
			results <- rows
		}()
	}
	wg.Wait()
	close(results)
	count := 0
	for rows := range results {
		count += len(rows)
	}
	if count != 1 {
		t.Fatalf("duplicate batch claims: %d", count)
	}
	if err := db.RecoveryProblem(ctx, s.JobID, "manager_evidence_missing_operator_required"); err != nil {
		t.Fatal(err)
	}
	got, err := db.FindBuildSubmission(ctx, s.OwnerID, s.SiteID, s.RequestKey)
	if err != nil || got.DispatchState != "dispatching" || got.Status != "pending" || got.RecoveryError != "manager_evidence_missing_operator_required" {
		t.Fatalf("missing evidence lost: %+v %v", got, err)
	}
	if claimed, err := db.ClaimBuildDispatch(ctx, s.JobID); err != nil || claimed {
		t.Fatal("uncertain job became dispatchable")
	}
}
