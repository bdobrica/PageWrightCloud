//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/storage"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/types"
)

// This exercises the worker's real callback sender against the isolated manager,
// without starting an executor or claiming the AI build pipeline works.
func TestManagerWorkerContractRoundTrip(t *testing.T) {
	managerURL := strings.TrimRight(os.Getenv("TEST_MANAGER_URL"), "/")
	if managerURL == "" {
		t.Fatal("TEST_MANAGER_URL is required; run make test-integration from the repository root")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	for _, status := range []string{"completed", "failed"} {
		t.Run(status, func(t *testing.T) {
			siteID := fmt.Sprintf("worker-contract-%s-%d", status, time.Now().UnixNano())
			request := map[string]string{"site_id": siteID, "owner_id": "contract-owner", "prompt": "Update heading", "source_version": "source-version", "target_version": "target-version"}
			payload, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := client.Post(managerURL+"/jobs", "application/json", bytes.NewReader(payload))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("create status = %d", resp.StatusCode)
			}
			var job types.Job
			if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(10 * time.Second)
			for job.Status == "pending" && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
				poll, err := client.Get(managerURL + "/jobs/" + job.JobID)
				if err != nil {
					t.Fatal(err)
				}
				err = json.NewDecoder(poll.Body).Decode(&job)
				poll.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			if job.Status != "running" {
				t.Fatalf("job did not dispatch: %+v", job)
			}
			if err := job.ValidateLaunch(); err != nil {
				t.Fatalf("manager snapshot is not launchable: %v", err)
			}
			manifest, failure := "/sites/"+siteID+"/artifacts/target-version/manifest", ""
			if status == "failed" {
				manifest, failure = "", "fixture execution failed"
			}
			if status == "completed" {
				if err := reportResult(managerURL, &job, status, manifest, failure); err == nil {
					t.Fatal("completion without storage commit accepted")
				}
				archive := filepath.Join(t.TempDir(), "fixture.tar.gz")
				if err := os.WriteFile(archive, []byte("transport fixture"), 0600); err != nil {
					t.Fatal(err)
				}
				client := storage.NewClient(os.Getenv("TEST_STORAGE_URL")).WithAttempt(&job)
				if err := client.UploadArtifact(job.SiteID, job.TargetVersion, archive); err != nil {
					t.Fatal(err)
				}
				if err := client.UploadLog(job.SiteID, job.TargetVersion, "fixture"); err != nil {
					t.Fatal(err)
				}
				if err := client.UploadManifest(job.SiteID, job.TargetVersion, types.Manifest{SiteID: job.SiteID, BuildID: job.TargetVersion, CreatedAt: time.Now().UTC(), FencingToken: job.FencingToken}); err != nil {
					t.Fatal(err)
				}
			}
			if err := reportResult(managerURL, &job, status, manifest, failure); err != nil {
				t.Fatal(err)
			}
			stored, err := client.Get(managerURL + "/jobs/" + job.JobID)
			if err != nil {
				t.Fatal(err)
			}
			defer stored.Body.Close()
			if stored.StatusCode != http.StatusOK {
				t.Fatalf("get status = %d", stored.StatusCode)
			}
			var got types.Job
			if err := json.NewDecoder(stored.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got.JobID != job.JobID || got.SiteID != siteID || got.OwnerID != request["owner_id"] || got.SourceVersion != request["source_version"] || got.TargetVersion != request["target_version"] || got.Prompt != request["prompt"] {
				t.Fatalf("associations changed: %+v", got)
			}
			if got.Status != status || got.ErrorMessage != failure || got.ManifestPath != manifest {
				t.Fatalf("wrong terminal snapshot: %+v", got)
			}
			if status == "completed" && got.Result != "Successfully processed site "+siteID {
				t.Fatalf("wrong result: %q", got.Result)
			}
			if status == "failed" && got.Result != "" {
				t.Fatalf("failure retained result: %q", got.Result)
			}
		})
	}
}
