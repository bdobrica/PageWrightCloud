//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"
)

// Transport fixtures still acquire real manager authority; no storage bypass.
func transportAttempt(t *testing.T, site, version string) string {
	t.Helper()
	manager := os.Getenv("TEST_MANAGER_URL")
	if manager == "" {
		t.Fatal("TEST_MANAGER_URL required")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	data, _ := json.Marshal(map[string]string{"site_id": site, "owner_id": "fixture-owner", "prompt": "transport fixture", "source_version": "initial", "target_version": version})
	response, err := client.Post(manager+"/jobs", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var job map[string]any
	err = json.NewDecoder(response.Body).Decode(&job)
	response.Body.Close()
	if err != nil || response.StatusCode != 201 {
		t.Fatalf("fixture admission: %d %v", response.StatusCode, err)
	}
	id, _ := job["job_id"].(string)
	deadline := time.Now().Add(10 * time.Second)
	for job["status"] == "pending" && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		response, err = client.Get(manager + "/jobs/" + id)
		if err != nil {
			t.Fatal(err)
		}
		err = json.NewDecoder(response.Body).Decode(&job)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if job["status"] != "running" || job["lock_token"] == nil || job["fencing_token"] == nil {
		t.Fatalf("fixture not fenced: %+v", job)
	}
	attempt := map[string]any{}
	for _, key := range []string{"job_id", "site_id", "owner_id", "source_version", "target_version", "lock_token", "fencing_token"} {
		attempt[key] = job[key]
	}
	header, _ := json.Marshal(attempt)
	t.Cleanup(func() {
		attempt["status"] = "failed"
		attempt["error_message"] = "transport-only fixture ended"
		data, _ := json.Marshal(attempt)
		response, err := client.Post(manager+"/jobs/"+id+"/result", "application/json", bytes.NewReader(data))
		if err != nil {
			t.Error(err)
			return
		}
		response.Body.Close()
		if response.StatusCode != 200 && response.StatusCode != 409 {
			t.Errorf("fixture cleanup: %d", response.StatusCode)
		}
	})
	return string(header)
}
