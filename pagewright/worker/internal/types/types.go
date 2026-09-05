package types

import (
	"fmt"
	"strings"
	"time"
)

// Job represents a work unit passed from manager
type Job struct {
	JobID         string    `json:"job_id"`
	SiteID        string    `json:"site_id"`
	OwnerID       string    `json:"owner_id"`
	Prompt        string    `json:"prompt"`
	SourceVersion string    `json:"source_version"`
	TargetVersion string    `json:"target_version"`
	Status        string    `json:"status"`
	LockToken     string    `json:"lock_token,omitempty"`
	FencingToken  int64     `json:"fencing_token"`
	WorkerID      string    `json:"worker_id,omitempty"`
	Result        string    `json:"result,omitempty"`
	ErrorMessage  string    `json:"error_message,omitempty"`
	ManifestPath  string    `json:"manifest_path,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Manifest describes the output artifact
type Manifest struct {
	SiteID         string    `json:"site_id"`
	BuildID        string    `json:"build_id"`
	BaseBuildID    string    `json:"base_build_id"`
	FencingToken   int64     `json:"fencing_token"`
	Prompt         string    `json:"prompt"`
	CreatedAt      time.Time `json:"created_at"`
	FileCount      int       `json:"file_count"`
	TotalSize      int64     `json:"total_size"`
	Entrypoints    []string  `json:"entrypoints"`
	Screenshots    []string  `json:"screenshots"`
	ChecksPassed   bool      `json:"checks_passed"`
	ConsoleErrors  int       `json:"console_errors"`
	FilesChanged   []string  `json:"files_changed"`
	ChangesSummary string    `json:"changes_summary"`
}

// WorkerStatus represents current execution state
type WorkerStatus struct {
	State        string `json:"state"` // idle, fetching, unpacking, executing, packing, uploading, done, failed
	CurrentStep  string `json:"current_step"`
	Progress     int    `json:"progress"` // 0-100
	CodexRunning bool   `json:"codex_running"`
	Error        string `json:"error,omitempty"`
}

// JobResult is sent back to manager when work completes
type JobResult struct {
	JobID         string `json:"job_id"`
	SiteID        string `json:"site_id"`
	OwnerID       string `json:"owner_id"`
	SourceVersion string `json:"source_version"`
	Status        string `json:"status"` // completed, failed
	TargetVersion string `json:"target_version"`
	Result        string `json:"result,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	ManifestPath  string `json:"manifest_path,omitempty"`
}

// ValidateLaunch rejects incomplete or terminal snapshots before any work starts.
func (j Job) ValidateLaunch() error {
	for _, field := range []struct{ name, value string }{
		{"job_id", j.JobID}, {"site_id", j.SiteID}, {"owner_id", j.OwnerID},
		{"prompt", j.Prompt}, {"source_version", j.SourceVersion}, {"target_version", j.TargetVersion},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required", field.name)
		}
	}
	if j.Status != "pending" && j.Status != "running" {
		return fmt.Errorf("worker launch status must be pending or running")
	}
	if j.CreatedAt.IsZero() || j.UpdatedAt.IsZero() {
		return fmt.Errorf("created_at and updated_at are required")
	}
	if j.ErrorMessage != "" {
		return fmt.Errorf("worker launch cannot include error_message")
	}
	if j.ManifestPath != "" {
		return fmt.Errorf("worker launch cannot include manifest_path")
	}
	return nil
}

// Validate enforces the terminal callback contract independently of execution.
func (r JobResult) Validate() error {
	for _, field := range []struct{ name, value string }{
		{"job_id", r.JobID}, {"site_id", r.SiteID}, {"owner_id", r.OwnerID},
		{"source_version", r.SourceVersion}, {"target_version", r.TargetVersion},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required", field.name)
		}
	}
	switch r.Status {
	case "completed":
		if r.ErrorMessage != "" {
			return fmt.Errorf("completed result cannot include error_message")
		}
	case "failed":
		if strings.TrimSpace(r.ErrorMessage) == "" {
			return fmt.Errorf("failed result requires error_message")
		}
		if r.ManifestPath != "" {
			return fmt.Errorf("failed result cannot include manifest_path")
		}
	default:
		return fmt.Errorf("result status must be completed or failed")
	}
	return nil
}
