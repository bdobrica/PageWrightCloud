package types

import (
	"fmt"
	"strings"
	"time"
)

// Job wire types implement docs/JOB_CONTRACT.md. Job IDs identify executions,
// never artifact versions. Internal manager lease fields are not exposed here.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

type ManagerJobRequest struct {
	SiteID        string `json:"site_id"`
	OwnerID       string `json:"owner_id"`
	Prompt        string `json:"prompt"`
	SourceVersion string `json:"source_version"`
	TargetVersion string `json:"target_version,omitempty"`
}

type JobAccepted struct {
	JobID         string    `json:"job_id"`
	SiteID        string    `json:"site_id"`
	OwnerID       string    `json:"owner_id"`
	SourceVersion string    `json:"source_version"`
	TargetVersion string    `json:"target_version"`
	Status        JobStatus `json:"status"`
}

type Job struct {
	JobAccepted
	Prompt       string    `json:"prompt"`
	Result       string    `json:"result,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	ManifestPath string    `json:"manifest_path,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ManagerJobResponse = Job

func (j Job) Validate() error {
	for name, value := range map[string]string{
		"job_id": j.JobID, "site_id": j.SiteID, "owner_id": j.OwnerID,
		"source_version": j.SourceVersion, "target_version": j.TargetVersion, "prompt": j.Prompt,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("missing %s", name)
		}
	}
	switch j.Status {
	case JobStatusPending, JobStatusRunning, JobStatusCompleted, JobStatusFailed:
	default:
		return fmt.Errorf("invalid job status %q", j.Status)
	}
	if j.CreatedAt.IsZero() || j.UpdatedAt.IsZero() {
		return fmt.Errorf("missing job timestamps")
	}
	if j.Status == JobStatusFailed {
		if strings.TrimSpace(j.ErrorMessage) == "" {
			return fmt.Errorf("failed job requires error_message")
		}
	} else if j.ErrorMessage != "" {
		return fmt.Errorf("error_message requires failed status")
	}
	if j.ManifestPath != "" && j.Status != JobStatusCompleted {
		return fmt.Errorf("manifest_path requires completed status")
	}
	return nil
}
