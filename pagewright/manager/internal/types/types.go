package types

import "time"

// JobStatus represents the state of a job
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

// Job represents a work request
type Job struct {
	JobID         string    `json:"job_id"`
	SiteID        string    `json:"site_id"`
	Prompt        string    `json:"prompt"`
	SourceVersion string    `json:"source_version,omitempty"`
	TargetVersion string    `json:"target_version,omitempty"`
	Status        JobStatus `json:"status"`
	LockToken     string    `json:"lock_token,omitempty"`
	FencingToken  int64     `json:"fencing_token,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	WorkerID      string    `json:"worker_id,omitempty"`
	Result        string    `json:"result,omitempty"`
	ErrorMessage  string    `json:"error_message,omitempty"`
}

// JobRequest represents an incoming job request
type JobRequest struct {
	SiteID        string `json:"site_id"`
	Prompt        string `json:"prompt"`
	SourceVersion string `json:"source_version,omitempty"`
	TargetVersion string `json:"target_version,omitempty"`
}

// JobStatusUpdate represents a status update from a worker
type JobStatusUpdate struct {
	Status       JobStatus `json:"status"`
	Result       string    `json:"result,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
}
