package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrSubmissionConflict = errors.New("build submission conflict")

type BuildSubmission struct {
	JobID          string    `db:"job_id"`
	SiteID         string    `db:"site_id"`
	OwnerID        string    `db:"owner_id"`
	SourceVersion  string    `db:"source_version"`
	TargetVersion  string    `db:"target_version"`
	Prompt         string    `db:"prompt"`
	RequestKey     string    `db:"request_key"`
	RequestHash    string    `db:"request_hash"`
	DispatchState  string    `db:"dispatch_state"`
	Status         string    `db:"status"`
	ErrorMessage   string    `db:"error_message"`
	ErrorCode      string    `db:"error_code"`
	ResponseStatus int       `db:"response_status"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

func (db *DB) FindBuildSubmission(ctx context.Context, ownerID, siteID, key string) (*BuildSubmission, error) {
	var result BuildSubmission
	err := db.GetContext(ctx, &result, `SELECT * FROM build_submissions WHERE owner_id=$1 AND site_id=$2 AND request_key=$3`, ownerID, siteID, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ReserveBuildSubmission persists the execution-to-artifact mapping and version
// together before dispatch. The request lock serializes retries across processes.
func (db *DB) ReserveBuildSubmission(ctx context.Context, proposal *BuildSubmission) (*BuildSubmission, bool, error) {
	if proposal == nil {
		return nil, false, fmt.Errorf("submission is required")
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	lockKey := proposal.OwnerID + ":" + proposal.SiteID + ":" + proposal.RequestKey
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return nil, false, err
	}
	var siteID string
	if err := tx.GetContext(ctx, &siteID, `SELECT id FROM sites WHERE id=$1 AND user_id=$2 FOR SHARE`, proposal.SiteID, proposal.OwnerID); err != nil {
		return nil, false, fmt.Errorf("site ownership: %w", err)
	}
	var existing BuildSubmission
	err = tx.GetContext(ctx, &existing, `SELECT * FROM build_submissions WHERE owner_id=$1 AND site_id=$2 AND request_key=$3`, proposal.OwnerID, proposal.SiteID, proposal.RequestKey)
	if err == nil {
		if existing.RequestHash != proposal.RequestHash {
			return nil, false, ErrSubmissionConflict
		}
		return &existing, false, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO versions(site_id,build_id,status) VALUES($1,$2,'pending')`, proposal.SiteID, proposal.TargetVersion); err != nil {
		return nil, false, err
	}
	var result BuildSubmission
	err = tx.GetContext(ctx, &result, `INSERT INTO build_submissions
		(job_id,site_id,owner_id,source_version,target_version,prompt,request_key,request_hash)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *`, proposal.JobID, proposal.SiteID, proposal.OwnerID, proposal.SourceVersion, proposal.TargetVersion, proposal.Prompt, proposal.RequestKey, proposal.RequestHash)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return &result, true, nil
}

func (db *DB) ClaimBuildDispatch(ctx context.Context, jobID string) (bool, error) {
	result, err := db.ExecContext(ctx, `UPDATE build_submissions SET dispatch_state='dispatching', updated_at=now() WHERE job_id=$1 AND dispatch_state='ready'`, jobID)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

// RecordBuildOutcome commits the submission and version status atomically. A
// terminal dispatch decision cannot be replaced by a late competing response.
func (db *DB) RecordBuildOutcome(ctx context.Context, jobID, state, status, errorCode, errorMessage string, responseStatus int) error {
	if state != "accepted" && state != "rejected" {
		return fmt.Errorf("invalid dispatch outcome %q", state)
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current BuildSubmission
	if err := tx.GetContext(ctx, &current, `SELECT * FROM build_submissions WHERE job_id=$1 FOR UPDATE`, jobID); err != nil {
		return err
	}
	if current.DispatchState == "accepted" || current.DispatchState == "rejected" {
		if current.DispatchState == state && current.Status == status && current.ErrorCode == errorCode && current.ErrorMessage == errorMessage && current.ResponseStatus == responseStatus {
			return tx.Commit()
		}
		return ErrSubmissionConflict
	}
	if current.DispatchState != "dispatching" {
		return ErrSubmissionConflict
	}
	if _, err := tx.ExecContext(ctx, `UPDATE build_submissions SET dispatch_state=$2,status=$3,error_code=$4,error_message=$5,response_status=$6,updated_at=now() WHERE job_id=$1`, jobID, state, status, errorCode, errorMessage, responseStatus); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE versions SET status=$3 WHERE site_id=$1 AND build_id=$2`, current.SiteID, current.TargetVersion, status)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("submission version missing")
	}
	return tx.Commit()
}
