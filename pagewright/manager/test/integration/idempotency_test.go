//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/google/uuid"
)

func TestIntegrationConcurrentIdempotentSubmission(t *testing.T) {
	waitForService(t)
	req := types.JobRequest{JobID: uuid.NewString(), SiteID: "dedup-" + uuid.NewString(), OwnerID: "owner", Prompt: "edit", SourceVersion: "v1", TargetVersion: uuid.NewString()}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: timeout}
	var wg sync.WaitGroup
	codes := make(chan int, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Post(baseURL+"/jobs", "application/json", bytes.NewReader(data))
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close()
			codes <- resp.StatusCode
			var job types.Job
			if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
				t.Error(err)
				return
			}
			if job.JobID != req.JobID || job.TargetVersion != req.TargetVersion || job.OwnerID != req.OwnerID {
				t.Errorf("association mismatch: %+v", job)
			}
		}()
	}
	wg.Wait()
	close(codes)
	created, total := 0, 0
	for code := range codes {
		total++
		if code == 201 {
			created++
		} else if code != 200 {
			t.Errorf("duplicate returned %d", code)
		}
	}
	if created != 1 || total != 16 {
		t.Fatalf("created=%d responses=%d", created, total)
	}
	// A retry after a successful terminal callback returns the stored terminal job.
	result := types.JobStatusUpdate{JobID: req.JobID, SiteID: req.SiteID, OwnerID: req.OwnerID, SourceVersion: req.SourceVersion, TargetVersion: req.TargetVersion, Status: types.JobStatusCompleted, Result: "done"}
	resultData, _ := json.Marshal(result)
	resp, err := client.Post(baseURL+"/jobs/"+req.JobID+"/result", "application/json", bytes.NewReader(resultData))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("result: %d", resp.StatusCode)
	}
	resp, err = client.Post(baseURL+"/jobs", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var replay types.Job
	if err := json.NewDecoder(resp.Body).Decode(&replay); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || replay.Status != types.JobStatusCompleted || replay.Result != "done" {
		t.Fatalf("terminal replay %d %+v", resp.StatusCode, replay)
	}
	req.Prompt = "different"
	changed, _ := json.Marshal(req)
	resp, err = client.Post(baseURL+"/jobs", "application/json", bytes.NewReader(changed))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var envelope types.APIError
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 409 || envelope.Error != "job_conflict" {
		t.Fatalf("mismatch: %d %+v", resp.StatusCode, envelope)
	}
}
