package handlers

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
)

type recoveryStore interface {
	RecoveryBatch(context.Context) ([]database.BuildSubmission, error)
	RecoveryProblem(context.Context, string, string) error
	ObserveBuildJob(context.Context, *types.Job) error
}

func (h *BuildHandler) RunRecovery(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := h.RecoverJobs(ctx); err != nil && ctx.Err() == nil {
			log.Print("Build history reconciliation pending")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (h *BuildHandler) RecoverJobs(ctx context.Context) error {
	store, ok := h.db.(recoveryStore)
	if !ok {
		return errors.New("recovery store unavailable")
	}
	listing, cancel := context.WithTimeout(ctx, 5*time.Second)
	rows, err := store.RecoveryBatch(listing)
	cancel()
	if err != nil {
		return err
	}
	var first error
	for _, s := range rows {
		operation, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := h.recoverJob(operation, store, s)
		cancel()
		if err != nil {
			if first == nil {
				first = err
			}
			code := "manager_evidence_unavailable"
			var managerErr *clients.ManagerError
			if errors.As(err, &managerErr) && managerErr.StatusCode == 404 {
				code = "manager_evidence_missing_operator_required"
			}
			if errors.Is(err, database.ErrSubmissionConflict) {
				code = "manager_evidence_conflict_operator_required"
			}
			save, cancel := context.WithTimeout(ctx, 2*time.Second)
			_ = store.RecoveryProblem(save, s.JobID, code)
			cancel()
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return first
}

func (h *BuildHandler) recoverJob(ctx context.Context, store recoveryStore, s database.BuildSubmission) error {
	if s.DispatchState == "ready" {
		claimed, err := h.db.ClaimBuildDispatch(ctx, s.JobID)
		if err != nil {
			return err
		}
		if claimed {
			job, err := h.managerClient.EnqueueJobContext(ctx, clients.ManagerJobRequest{JobID: s.JobID, SiteID: s.SiteID, OwnerID: s.OwnerID, SourceVersion: s.SourceVersion, TargetVersion: s.TargetVersion, Prompt: s.Prompt})
			if err == nil {
				return store.ObserveBuildJob(ctx, job)
			}
			// Even a rejected POST is reconciled from the remembered exact manager job.
			// No new identity, no reset to ready, and no second POST after a claim.
		}
	}
	job, err := h.managerClient.GetJobStatusContext(ctx, s.JobID)
	if err != nil {
		return err
	}
	return store.ObserveBuildJob(ctx, job)
}
