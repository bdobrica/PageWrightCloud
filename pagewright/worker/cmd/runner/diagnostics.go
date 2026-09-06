package main

import (
	"encoding/json"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/types"
)

// Allowlisting is intentional redaction: arbitrary model/tool text can contain
// secrets in unknown encodings, so regex replacement cannot make it safe to keep.
// The original output remains bounded in executor memory, never in this log.
func privateDiagnostic(job *types.Job, output string) string {
	data, _ := json.Marshal(struct {
		JobID       string `json:"job_id"`
		SiteID      string `json:"site_id"`
		Version     string `json:"target_version"`
		OutputBytes int    `json:"executor_output_bytes"`
		Redaction   string `json:"redaction"`
	}{job.JobID, job.SiteID, job.TargetVersion, len(output), "executor output withheld"})
	if len(data) > 4096 {
		return `{"redaction":"oversized diagnostic identity withheld"}`
	}
	return string(data)
}
