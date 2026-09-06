package types

import (
	"testing"
	"time"
)

func TestValidateLaunch(t *testing.T) {
	valid := Job{LockToken: "attempt", FencingToken: 1, JobID: "job", SiteID: "site", OwnerID: "owner", Prompt: "prompt", SourceVersion: "source", TargetVersion: "target", Status: "running", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	for _, status := range []string{"running"} {
		job := valid
		job.Status = status
		if err := job.ValidateLaunch(); err != nil {
			t.Fatal(err)
		}
	}
	for _, field := range []string{"job_id", "site_id", "owner_id", "prompt", "source_version", "target_version", "status", "created_at", "updated_at"} {
		job := valid
		switch field {
		case "job_id":
			job.JobID = " "
		case "site_id":
			job.SiteID = " "
		case "owner_id":
			job.OwnerID = " "
		case "prompt":
			job.Prompt = " "
		case "source_version":
			job.SourceVersion = " "
		case "target_version":
			job.TargetVersion = " "
		case "status":
			job.Status = ""
		case "created_at":
			job.CreatedAt = time.Time{}
		case "updated_at":
			job.UpdatedAt = time.Time{}
		}
		if err := job.ValidateLaunch(); err == nil {
			t.Errorf("missing %s accepted", field)
		}
	}
	for _, status := range []string{"pending", "completed", "failed", "queued", "succeeded"} {
		job := valid
		job.Status = status
		if err := job.ValidateLaunch(); err == nil {
			t.Errorf("launch status %s accepted", status)
		}
	}
	for _, status := range []string{"pending", "running"} {
		for _, field := range []string{"error_message", "manifest_path"} {
			job := valid
			job.Status = status
			if field == "error_message" {
				job.ErrorMessage = "failure"
			} else {
				job.ManifestPath = "manifest"
			}
			if err := job.ValidateLaunch(); err == nil {
				t.Errorf("%s launch accepted %s", status, field)
			}
		}
	}
	for _, mutate := range []func(*Job){func(j *Job) { j.LockToken = "" }, func(j *Job) { j.FencingToken = 0 }} {
		job := valid
		mutate(&job)
		if err := job.ValidateLaunch(); err == nil {
			t.Fatal("unfenced launch accepted")
		}
	}
}
