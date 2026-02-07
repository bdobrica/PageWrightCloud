package api

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.SiteID == "" || req.Prompt == "" {
		http.Error(w, "site_id and prompt are required", http.StatusBadRequest)
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
		http.Error(w, fmt.Sprintf("Failed to acquire lock: %v", err), http.StatusConflict)
		return
	}

	job.LockToken = token
	job.FencingToken = fencingToken

	// Push job to queue
	if err := h.queue.Push(ctx, job); err != nil {
		// Release lock if we can't queue the job
		h.lockMgr.Release(ctx, req.SiteID, token)
		http.Error(w, fmt.Sprintf("Failed to queue job: %v", err), http.StatusInternalServerError)
		return
	}

	// Update job status to running and spawn worker
	job.Status = types.JobStatusRunning
	job.UpdatedAt = time.Now().UTC()
	if err := h.queue.UpdateJob(ctx, job); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update job: %v", err), http.StatusInternalServerError)
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

		http.Error(w, fmt.Sprintf("Failed to spawn worker: %v", err), http.StatusInternalServerError)
		return
	}

	job.WorkerID = workerID
	h.queue.UpdateJob(ctx, job)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(job)
}

func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["job_id"]

	if jobID == "" {
		http.Error(w, "job_id is required", http.StatusBadRequest)
		return
	}

	job, err := h.queue.GetJob(r.Context(), jobID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get job: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func (h *Handler) UpdateJobStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["job_id"]

	if jobID == "" {
		http.Error(w, "job_id is required", http.StatusBadRequest)
		return
	}

	var update types.JobStatusUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Get current job
	job, err := h.queue.GetJob(ctx, jobID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Job not found: %v", err), http.StatusNotFound)
		return
	}

	// Update job
	job.Status = update.Status
	job.Result = update.Result
	job.ErrorMessage = update.ErrorMessage
	job.UpdatedAt = time.Now().UTC()

	if err := h.queue.UpdateJob(ctx, job); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update job: %v", err), http.StatusInternalServerError)
		return
	}

	// Release lock if job is completed or failed
	if update.Status == types.JobStatusCompleted || update.Status == types.JobStatusFailed {
		if job.LockToken != "" {
			if err := h.lockMgr.Release(ctx, job.SiteID, job.LockToken); err != nil {
				fmt.Printf("Warning: Failed to release lock for site %s: %v\n", job.SiteID, err)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func (h *Handler) JobResult(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["job_id"]

	if jobID == "" {
		http.Error(w, "job_id is required", http.StatusBadRequest)
		return
	}

	var result struct {
		JobID         string `json:"job_id"`
		Status        string `json:"status"`
		TargetVersion string `json:"target_version"`
		Result        string `json:"result"`
		ErrorMessage  string `json:"error_message,omitempty"`
		ManifestPath  string `json:"manifest_path,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Get current job
	job, err := h.queue.GetJob(ctx, jobID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Job not found: %v", err), http.StatusNotFound)
		return
	}

	// Update job with worker result
	if result.Status == "completed" {
		job.Status = types.JobStatusCompleted
		job.Result = result.Result
	} else {
		job.Status = types.JobStatusFailed
		job.ErrorMessage = result.ErrorMessage
	}
	job.UpdatedAt = time.Now().UTC()

	if err := h.queue.UpdateJob(ctx, job); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update job: %v", err), http.StatusInternalServerError)
		return
	}

	// Release lock
	if job.LockToken != "" {
		if err := h.lockMgr.Release(ctx, job.SiteID, job.LockToken); err != nil {
			fmt.Printf("Warning: Failed to release lock for site %s: %v\n", job.SiteID, err)
		}
	}

	fmt.Printf("Job %s completed by worker: status=%s, version=%s\n", jobID, result.Status, result.TargetVersion)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Job result received",
		"status":  result.Status,
	})
}
