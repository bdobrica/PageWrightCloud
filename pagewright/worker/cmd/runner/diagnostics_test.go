package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPrivateDiagnosticWithholdsRawSecrets(t *testing.T) {
	job := contractJob()
	raw := "Bearer provider-secret\npassword=hunter2\n" + strings.Repeat("base64-secret-", 100000)
	got := privateDiagnostic(&job, raw)
	if len(got) > 4096 || strings.Contains(got, "provider-secret") || strings.Contains(got, "hunter2") || strings.Contains(got, job.LockToken) || strings.Contains(got, "base64-secret") {
		t.Fatal("unbounded or sensitive diagnostic")
	}
	var data map[string]any
	if json.Unmarshal([]byte(got), &data) != nil || data["job_id"] != job.JobID || data["site_id"] != job.SiteID || data["target_version"] != job.TargetVersion || data["executor_output_bytes"] != float64(len(raw)) {
		t.Fatalf("correlation lost: %s", got)
	}
	job.SiteID = strings.Repeat("x", 10000)
	if len(privateDiagnostic(&job, raw)) > 4096 {
		t.Fatal("oversized identity not bounded")
	}
}
