package types

import "time"

// Job represents a work unit passed from manager
type Job struct {
	JobID         string    `json:"job_id"`
	SiteID        string    `json:"site_id"`
	Prompt        string    `json:"prompt"`
	SourceVersion string    `json:"source_version"`
	TargetVersion string    `json:"target_version"`
	Status        string    `json:"status"`
	LockToken     string    `json:"lock_token,omitempty"`
	FencingToken  int64     `json:"fencing_token"`
	WorkerID      string    `json:"worker_id,omitempty"`
	Result        string    `json:"result,omitempty"`
	ErrorMessage  string    `json:"error_message,omitempty"`
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
	Status        string `json:"status"` // completed, failed
	TargetVersion string `json:"target_version"`
	Result        string `json:"result"`
	ErrorMessage  string `json:"error_message,omitempty"`
	ManifestPath  string `json:"manifest_path,omitempty"`
}
