package handlers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
)

func (h *BuildHandler) reservationError(w http.ResponseWriter, err error) {
	if errors.Is(err, database.ErrSubmissionConflict) {
		respondError(w, http.StatusConflict, "Idempotency-Key was already used with different input")
		return
	}
	respondError(w, http.StatusInternalServerError, "failed to persist build submission; no job was dispatched")
}

func (h *BuildHandler) dispatchBuild(w http.ResponseWriter, r *http.Request, s *database.BuildSubmission) {
	if s.DispatchState == "accepted" || s.DispatchState == "rejected" {
		respondSubmission(w, s)
		return
	}
	if s.DispatchState == "ready" {
		claimed, err := h.db.ClaimBuildDispatch(r.Context(), s.JobID)
		if err != nil {
			uncertainSubmission(w, s, "failed to claim submission; retry with the same Idempotency-Key")
			return
		}
		if claimed {
			s.DispatchState = "dispatching"
			job, err := h.managerClient.EnqueueJobContext(r.Context(), clients.ManagerJobRequest{
				JobID: s.JobID, SiteID: s.SiteID, OwnerID: s.OwnerID,
				Prompt: s.Prompt, SourceVersion: s.SourceVersion, TargetVersion: s.TargetVersion,
			})
			if err == nil {
				h.recordAccepted(w, r, s, job)
				return
			}
			var managerErr *clients.ManagerError
			if errors.As(err, &managerErr) {
				switch managerErr.Code {
				case "job_busy", "spawn_failed", "invalid_request", "job_conflict":
					h.recordRejected(w, r, s, managerErr.Code)
					return
				}
			}
			// A failed dial cannot have submitted bytes. Other transport/HTTP
			// failures are ambiguous and must not create a second execution.
			var networkErr *net.OpError
			if errors.As(err, &networkErr) && networkErr.Op == "dial" {
				h.recordRejected(w, r, s, "manager_unavailable")
				return
			}
		}
	}
	// A prior attempt may have reached the manager even if the response or local
	// outcome write was lost. Read back the exact ID; never blindly re-dispatch.
	job, err := h.managerClient.GetJobStatusContext(r.Context(), s.JobID)
	if err != nil || job.SiteID != s.SiteID || job.OwnerID != s.OwnerID || job.SourceVersion != s.SourceVersion || job.TargetVersion != s.TargetVersion || job.Prompt != s.Prompt {
		uncertainSubmission(w, s, "submission outcome is uncertain; retry with the same Idempotency-Key to reconcile (no automatic redispatch)")
		return
	}
	if job.ErrorCode == "job_busy" || job.ErrorCode == "spawn_failed" {
		h.recordRejected(w, r, s, job.ErrorCode)
		return
	}
	h.recordAccepted(w, r, s, job)
}

func outcomeContext(r *http.Request) (context.Context, context.CancelFunc) {
	// Persist the manager result even if the browser disconnected meanwhile.
	return context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
}

func (h *BuildHandler) recordAccepted(w http.ResponseWriter, r *http.Request, s *database.BuildSubmission, job *types.Job) {
	ctx, cancel := outcomeContext(r)
	defer cancel()
	if err := h.db.RecordBuildOutcome(ctx, s.JobID, "accepted", string(job.Status), "", job.ErrorMessage, http.StatusOK); err != nil {
		uncertainSubmission(w, s, "manager may have accepted the job but its local outcome could not be saved; retry with the same Idempotency-Key")
		return
	}
	s.DispatchState, s.Status, s.ErrorMessage = "accepted", string(job.Status), job.ErrorMessage
	respondSubmission(w, s)
}

func (h *BuildHandler) recordRejected(w http.ResponseWriter, r *http.Request, s *database.BuildSubmission, code string) {
	status, message := http.StatusBadGateway, "manager rejected the build submission"
	switch code {
	case "job_busy":
		status, message = http.StatusConflict, "site already has an active job"
	case "job_conflict":
		status, message = http.StatusConflict, "manager job identity conflicts with the persisted submission"
	case "spawn_failed":
		message = "worker could not be started"
	case "manager_unavailable":
		status, message = http.StatusServiceUnavailable, "manager could not be reached; this submission was not dispatched"
	}
	ctx, cancel := outcomeContext(r)
	defer cancel()
	if err := h.db.RecordBuildOutcome(ctx, s.JobID, "rejected", "failed", code, message, status); err != nil {
		uncertainSubmission(w, s, "submission rejection could not be saved; retry with the same Idempotency-Key")
		return
	}
	s.DispatchState, s.Status, s.ErrorCode, s.ErrorMessage, s.ResponseStatus = "rejected", "failed", code, message, status
	respondSubmission(w, s)
}

func respondSubmission(w http.ResponseWriter, s *database.BuildSubmission) {
	if s.DispatchState == "rejected" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.ResponseStatus)
		respondJSON(w, map[string]string{"error": s.ErrorCode, "message": s.ErrorMessage, "submission_state": "rejected", "job_id": s.JobID, "target_version": s.TargetVersion})
		return
	}
	respondJSON(w, types.BuildResponse{JobAccepted: &types.JobAccepted{
		JobID: s.JobID, SiteID: s.SiteID, OwnerID: s.OwnerID,
		SourceVersion: s.SourceVersion, TargetVersion: s.TargetVersion,
		Status: types.JobStatus(s.Status), ErrorMessage: s.ErrorMessage,
	}})
}

func uncertainSubmission(w http.ResponseWriter, s *database.BuildSubmission, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	respondJSON(w, map[string]string{"error": "submission_uncertain", "message": message, "submission_state": "dispatching", "job_id": s.JobID, "target_version": s.TargetVersion})
}
