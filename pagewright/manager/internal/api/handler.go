package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/lock"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type Handler struct {
	queue      queue.Backend
	lockMgr    lock.Manager
	spawner    spawner.Spawner
	lockTTL    time.Duration
	managerURL string
}

func NewHandler(q queue.Backend, l lock.Manager, s spawner.Spawner, lockTTL time.Duration, managerURL string) *Handler {
	return &Handler{
		queue:      q,
		lockMgr:    l,
		spawner:    s,
		lockTTL:    lockTTL,
		managerURL: managerURL,
	}
}

func (h *Handler) SetupRoutes() *mux.Router {
	r := mux.NewRouter()

	// Health check
	r.HandleFunc("/health", h.HealthCheck).Methods("GET")

	// Job endpoints
	r.HandleFunc("/jobs", h.CreateJob).Methods("POST")
	r.HandleFunc("/jobs/{job_id}", h.GetJob).Methods("GET")
	r.HandleFunc("/jobs/{job_id}/status", h.UpdateJobStatus).Methods("POST")
	r.HandleFunc("/jobs/{job_id}/result", h.JobResult).Methods("POST")

	return r
}

func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req types.JobRequest
	if err := decodeRequest(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	// Validate required fields
	if blank(req.SiteID) || blank(req.OwnerID) || blank(req.Prompt) || blank(req.SourceVersion) || (req.TargetVersion != "" && blank(req.TargetVersion)) {
		writeError(w, http.StatusBadRequest, "invalid_request", "site_id, owner_id, prompt and source_version must be nonblank; target_version must be nonblank when supplied")
		return
	}

	// Generate job ID and target version if not provided
	jobID := uuid.New().String()
	if req.TargetVersion == "" {
		req.TargetVersion = uuid.New().String()
	}

	// Create job
	job := &types.Job{
		JobID:         jobID,
		SiteID:        req.SiteID,
		OwnerID:       req.OwnerID,
		Prompt:        req.Prompt,
		SourceVersion: req.SourceVersion,
		TargetVersion: req.TargetVersion,
		Status:        types.JobStatusPending,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	// Try to acquire lock
	ctx := r.Context()
	token, fencingToken, err := h.lockMgr.Acquire(ctx, req.SiteID, h.lockTTL)
	if err != nil {
		writeError(w, http.StatusConflict, "job_busy", fmt.Sprintf("Failed to acquire lock: %v", err))
		return
	}

	job.LockToken = token
	job.FencingToken = fencingToken

	// Push job to queue
	if err := h.queue.Push(ctx, job); err != nil {
		// Release lock if we can't queue the job
		h.lockMgr.Release(ctx, req.SiteID, token)
		writeError(w, http.StatusInternalServerError, "internal_error", fmt.Sprintf("Failed to queue job: %v", err))
		return
	}

	// Update job status to running and spawn worker
	job.Status = types.JobStatusRunning
	job.UpdatedAt = time.Now().UTC()
	if err := h.queue.UpdateJob(ctx, job); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", fmt.Sprintf("Failed to update job: %v", err))
		return
	}

	// Spawn worker
	workerID, err := h.spawner.Spawn(ctx, job, h.managerURL)
	if err != nil {
		// Update job as failed
		job.Status = types.JobStatusFailed
		job.ErrorMessage = fmt.Sprintf("Failed to spawn worker: %v", err)
		job.UpdatedAt = time.Now().UTC()
		h.queue.UpdateJob(ctx, job)

		// Release lock
		h.lockMgr.Release(ctx, req.SiteID, token)

		writeError(w, http.StatusInternalServerError, "internal_error", fmt.Sprintf("Failed to spawn worker: %v", err))
		return
	}

	job.WorkerID = workerID
	if err := h.queue.UpdateJob(ctx, job); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", fmt.Sprintf("Failed to update job: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(job)
}

func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["job_id"]

	if jobID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "job_id is required")
		return
	}

	job, err := h.queue.GetJob(r.Context(), jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "job_not_found", "Job not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func (h *Handler) UpdateJobStatus(w http.ResponseWriter, r *http.Request) {
	h.applyCallback(w, r, false)
}

func (h *Handler) JobResult(w http.ResponseWriter, r *http.Request) {
	h.applyCallback(w, r, true)
}

func (h *Handler) applyCallback(w http.ResponseWriter, r *http.Request, terminalOnly bool) {
	var update types.JobStatusUpdate
	if err := decodeRequest(r, &update); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid request body: %v", err))
		return
	}
	if blank(update.JobID) || blank(update.SiteID) || blank(update.OwnerID) || blank(update.SourceVersion) || blank(update.TargetVersion) {
		writeError(w, http.StatusBadRequest, "invalid_request", "job_id, site_id, owner_id, source_version and target_version must be nonblank")
		return
	}
	switch update.Status {
	case types.JobStatusRunning:
		if terminalOnly {
			writeError(w, http.StatusBadRequest, "invalid_request", "result status must be completed or failed")
			return
		}
	case types.JobStatusCompleted, types.JobStatusFailed:
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "status must be running, completed or failed")
		return
	}
	if (update.Status == types.JobStatusFailed && blank(update.ErrorMessage)) ||
		(update.Status != types.JobStatusFailed && update.ErrorMessage != "") ||
		(update.Status != types.JobStatusCompleted && update.ManifestPath != "") {
		writeError(w, http.StatusBadRequest, "invalid_request", "error_message is required only for failed callbacks; manifest_path is allowed only for completed callbacks")
		return
	}
	jobID := mux.Vars(r)["job_id"]
	if update.JobID != jobID {
		writeError(w, http.StatusConflict, "job_conflict", "Callback job_id does not match the route")
		return
	}
	ctx := r.Context()
	job, err := h.queue.GetJob(ctx, jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "job_not_found", "Job not found")
		return
	}
	// Identity validation is not callback authentication or fencing.
	if update.JobID != job.JobID || update.SiteID != job.SiteID || update.OwnerID != job.OwnerID ||
		update.SourceVersion != job.SourceVersion || update.TargetVersion != job.TargetVersion {
		writeError(w, http.StatusConflict, "job_conflict", "Callback identity or versions do not match the stored job")
		return
	}
	updated := *job
	updated.Status = update.Status
	updated.Result = update.Result
	updated.ErrorMessage = update.ErrorMessage
	updated.ManifestPath = update.ManifestPath
	updated.UpdatedAt = time.Now().UTC()
	if err := h.queue.UpdateJob(ctx, &updated); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", fmt.Sprintf("Failed to update job: %v", err))
		return
	}
	if update.Status == types.JobStatusCompleted || update.Status == types.JobStatusFailed {
		if updated.LockToken != "" {
			if err := h.lockMgr.Release(ctx, updated.SiteID, updated.LockToken); err != nil {
				fmt.Printf("Warning: Failed to release lock for site %s: %v\n", updated.SiteID, err)
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&updated)
}

func blank(value string) bool { return strings.TrimSpace(value) == "" }

// decodeRequest rejects unknown fields and multiple JSON values.
func decodeRequest(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("body must contain exactly one JSON object")
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(types.APIError{Error: code, Message: message})
}
