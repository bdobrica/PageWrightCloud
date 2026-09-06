package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/types"
)

func contractJob() types.Job {
	return types.Job{LockToken: "attempt-123", FencingToken: 7, JobID: "job-123", SiteID: "site-456", OwnerID: "owner-789", Prompt: "Update heading", SourceVersion: "source-1", TargetVersion: "target-2", Status: "running", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
}

func TestReportResultWireContract(t *testing.T) {
	for _, status := range []string{"completed", "failed"} {
		t.Run(status, func(t *testing.T) {
			job := contractJob()
			manifest, failure := "/sites/site-456/artifacts/target-2/manifest", ""
			if status == "failed" {
				manifest, failure = "", "compiler failed"
			}
			called := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if r.Method != http.MethodPost || r.URL.Path != "/jobs/job-123/result" || r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("unexpected request: %s %s %s", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload["lock_token"] != job.LockToken || payload["fencing_token"] != float64(job.FencingToken) {
					t.Error("callback lost attempt identity")
				}
				for key, want := range map[string]string{"job_id": job.JobID, "site_id": job.SiteID, "owner_id": job.OwnerID, "source_version": job.SourceVersion, "target_version": job.TargetVersion, "status": status} {
					if payload[key] != want {
						t.Errorf("%s = %v, want %s", key, payload[key], want)
					}
				}
				if status == "completed" {
					if payload["manifest_path"] != manifest || payload["result"] != "Successfully processed site site-456" {
						t.Errorf("wrong completed payload: %v", payload)
					}
					if _, ok := payload["error_message"]; ok {
						t.Error("completed callback contains error_message")
					}
				} else {
					if payload["error_message"] != failure {
						t.Errorf("wrong failure payload: %v", payload)
					}
					for _, key := range []string{"manifest_path", "result"} {
						if _, ok := payload[key]; ok {
							t.Errorf("failure callback contains %s", key)
						}
					}
				}
				for _, old := range []string{"error", "version", "build_id"} {
					if _, ok := payload[old]; ok {
						t.Errorf("legacy field %s present", old)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(payload)
			}))
			defer server.Close()
			if err := reportResult(server.URL+"/", &job, status, manifest, failure); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("manager was not called")
			}
		})
	}
}

func TestReportResultHTTPError(t *testing.T) {
	for _, code := range []int{400, 404, 409, 500} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			w.Write([]byte(`{"error":"callback_rejected","message":"callback rejected"}`))
		}))
		job := contractJob()
		err := reportResult(server.URL, &job, "failed", "", "execution failed")
		server.Close()
		if !errors.Is(err, errDeliveryUncertain) {
			t.Errorf("HTTP %d error = %v", code, err)
		}
	}
}

func TestReportResultRejectsInvalidCallback(t *testing.T) {
	job := contractJob()
	for _, args := range [][3]string{{"pending", "", ""}, {"failed", "", " "}, {"failed", "manifest", "failure"}, {"completed", "", "failure"}} {
		if err := reportResult("http://invalid.invalid", &job, args[0], args[1], args[2]); err == nil || !strings.Contains(err.Error(), "invalid result") {
			t.Errorf("callback %v error = %v", args, err)
		}
	}
	job.OwnerID = ""
	if err := reportResult("http://invalid.invalid", &job, "completed", "", ""); err == nil || !strings.Contains(err.Error(), "owner_id") {
		t.Errorf("missing owner error = %v", err)
	}
	if err := reportResult("http://invalid.invalid", nil, "completed", "", ""); err == nil {
		t.Error("nil job accepted")
	}
}
