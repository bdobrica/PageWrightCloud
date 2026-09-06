package database

import (
	"context"
	"fmt"
	"net/http"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
)

// Claim a bounded, fair polling batch across replicas. This is NOT dispatch
// authority: ClaimBuildDispatch remains the only ready -> dispatching gate.
func (db *DB) RecoveryBatch(ctx context.Context) ([]BuildSubmission, error) {
	var rows []BuildSubmission
	err := db.SelectContext(ctx, &rows, `WITH due AS (
 SELECT job_id FROM build_submissions
 WHERE (dispatch_state IN ('ready','dispatching') OR (dispatch_state='accepted' AND status IN ('pending','running')))
 AND recovery_checked_at < now()-interval '30 seconds' AND updated_at < now()-interval '30 seconds'
 ORDER BY recovery_checked_at,job_id LIMIT 32 FOR UPDATE SKIP LOCKED
 ) UPDATE build_submissions b SET recovery_checked_at=now() FROM due WHERE b.job_id=due.job_id RETURNING b.*`)
	return rows, err
}

func (db *DB) RecoveryProblem(ctx context.Context, id, code string) error {
	_, err := db.ExecContext(ctx, `UPDATE build_submissions SET recovery_error=$2 WHERE job_id=$1 AND (dispatch_state IN ('ready','dispatching') OR status IN ('pending','running'))`, id, code)
	return err
}

// ObserveBuildJob persists verified manager evidence without reopening terminal
// history. Missing manager evidence never calls this method or triggers POST.
func (db *DB) ObserveBuildJob(ctx context.Context, job *types.Job) error {
	if job == nil {
		return fmt.Errorf("job required")
	}
	if err := job.Validate(); err != nil {
		return err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var s BuildSubmission
	if err := tx.GetContext(ctx, &s, `SELECT * FROM build_submissions WHERE job_id=$1 FOR UPDATE`, job.JobID); err != nil {
		return err
	}
	if job.SiteID != s.SiteID || job.OwnerID != s.OwnerID || job.SourceVersion != s.SourceVersion || job.TargetVersion != s.TargetVersion || job.Prompt != s.Prompt {
		return ErrSubmissionConflict
	}
	if s.DispatchState == "ready" || s.DispatchState == "rejected" {
		return ErrSubmissionConflict
	}
	status := string(job.Status)
	if s.Status == "completed" || s.Status == "failed" {
		if s.Status != status {
			return ErrSubmissionConflict
		}
		return tx.Commit() // Never revise a conservative terminal outcome.
	}
	if s.Status == "running" && status == "pending" {
		return ErrSubmissionConflict
	}
	state, response := "accepted", http.StatusOK
	if s.DispatchState == "dispatching" && (job.ErrorCode == "job_busy" || job.ErrorCode == "spawn_failed") {
		state = "rejected"
		response = http.StatusBadGateway
		if job.ErrorCode == "job_busy" {
			response = http.StatusConflict
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE build_submissions SET dispatch_state=$2,status=$3,error_code=$4,error_message=$5,response_status=$6,recovery_error='',updated_at=now() WHERE job_id=$1`, s.JobID, state, status, job.ErrorCode, job.ErrorMessage, response); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE versions SET status=$3 WHERE site_id=$1 AND build_id=$2`, s.SiteID, s.TargetVersion, status)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("submission version missing")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO job_history(job_id,status,error_code,error_message,result,manifest_path) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(job_id,status) DO NOTHING`, s.JobID, status, job.ErrorCode, job.ErrorMessage, job.Result, job.ManifestPath); err != nil {
		return err
	}
	return tx.Commit()
}
