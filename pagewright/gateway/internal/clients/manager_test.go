package clients

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
)

func contractJob() types.Job {
	return types.Job{
		JobAccepted: types.JobAccepted{JobID: "job-1", SiteID: "site-1", OwnerID: "owner-1", SourceVersion: "source-1", TargetVersion: "target-2", Status: types.JobStatusRunning},
		Prompt:      "Change the title", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
}

func TestManagerClientHTTPContract(t *testing.T) {
	job := contractJob()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/jobs" {
			if r.Header.Get("Content-Type") != "application/json" {
				t.Error("missing JSON content type")
			}
			var wire map[string]string
			if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
				t.Error(err)
			}
			want := map[string]string{"job_id": job.JobID, "site_id": job.SiteID, "owner_id": job.OwnerID, "prompt": job.Prompt, "source_version": job.SourceVersion, "target_version": job.TargetVersion}
			if !reflect.DeepEqual(wire, want) {
				t.Errorf("wire request = %#v, want %#v", wire, want)
			}
			w.WriteHeader(http.StatusCreated)
		} else if r.Method != http.MethodGet || r.URL.Path != "/jobs/job-1" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(job)
	}))
	defer server.Close()
	client := NewManagerClient(server.URL)
	created, err := client.EnqueueJob(ManagerJobRequest{JobID: job.JobID, SiteID: job.SiteID, OwnerID: job.OwnerID, Prompt: job.Prompt, SourceVersion: job.SourceVersion, TargetVersion: job.TargetVersion})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*created, job) {
		t.Fatalf("created = %#v, want %#v", created, job)
	}
	got, err := client.GetJobStatus(job.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*got, job) {
		t.Fatalf("get = %#v, want %#v", got, job)
	}
}

func TestManagerClientRejectsInvalidSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*types.Job)
	}{
		{"legacy success", func(j *types.Job) { j.Status = "success" }},
		{"legacy queued", func(j *types.Job) { j.Status = "queued" }},
		{"missing owner", func(j *types.Job) { j.OwnerID = "" }},
		{"missing target", func(j *types.Job) { j.TargetVersion = "" }},
		{"missing source", func(j *types.Job) { j.SourceVersion = "" }},
		{"missing timestamps", func(j *types.Job) { j.CreatedAt = time.Time{} }},
		{"failed without error", func(j *types.Job) { j.Status = types.JobStatusFailed }},
		{"running with error", func(j *types.Job) { j.ErrorMessage = "broken" }},
		{"running with manifest", func(j *types.Job) { j.ManifestPath = "manifest" }},
		{"wrong owner", func(j *types.Job) { j.OwnerID = "someone-else" }},
		{"wrong execution", func(j *types.Job) { j.JobID = "another-job" }},
		{"wrong site", func(j *types.Job) { j.SiteID = "somewhere-else" }},
		{"wrong source", func(j *types.Job) { j.SourceVersion = "different-source" }},
		{"wrong target", func(j *types.Job) { j.TargetVersion = "different-target" }},
		{"wrong prompt", func(j *types.Job) { j.Prompt = "different-prompt" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job := contractJob()
			request := ManagerJobRequest{JobID: job.JobID, SiteID: job.SiteID, OwnerID: job.OwnerID, SourceVersion: job.SourceVersion, TargetVersion: job.TargetVersion, Prompt: job.Prompt}
			tc.change(&job)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(job) }))
			defer server.Close()
			if _, err := NewManagerClient(server.URL).EnqueueJob(request); err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
}

func TestManagerClientErrorContract(t *testing.T) {
	for _, status := range []int{400, 404, 409, 500} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(types.ErrorResponse{Error: "contract_error", Message: "Expected diagnostic"})
		}))
		client := NewManagerClient(server.URL)
		_, err := client.EnqueueJob(ManagerJobRequest{})
		var typed *ManagerError
		if !errors.As(err, &typed) || typed.StatusCode != status || typed.Code != "contract_error" || typed.Message != "Expected diagnostic" {
			t.Errorf("POST error = %v", err)
		}
		_, err = client.GetJobStatus("job-1")
		if !errors.As(err, &typed) || typed.StatusCode != status || typed.Code != "contract_error" {
			t.Errorf("GET error = %v", err)
		}
		server.Close()
	}
}

func TestManagerClientRejectsWrongRetrievedJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(contractJob()) }))
	defer server.Close()
	if _, err := NewManagerClient(server.URL).GetJobStatus("another-job"); err == nil {
		t.Fatal("wrong job accepted")
	}
}

func TestManagerClientDoesNotRedispatchRedirects(t *testing.T) {
	var redirected bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected = true
		json.NewEncoder(w).Encode(contractJob())
	}))
	defer target.Close()
	manager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer manager.Close()
	_, err := NewManagerClient(manager.URL).EnqueueJob(ManagerJobRequest{})
	var result *ManagerError
	if !errors.As(err, &result) || result.StatusCode != http.StatusTemporaryRedirect || redirected {
		t.Fatalf("redirect followed or hidden: %v followed=%v", err, redirected)
	}
}
