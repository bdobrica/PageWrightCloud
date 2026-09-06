package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/storage"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/types"
)

func TestPersistenceGatesCompletion(t *testing.T) {
	for _, failure := range []string{"", "artifact", "logs", "manifest", "callback"} {
		t.Run("failure="+failure, func(t *testing.T) {
			job := contractJob()
			manifest := types.Manifest{SiteID: job.SiteID, BuildID: job.TargetVersion, CreatedAt: time.Now().UTC()}
			archive := filepath.Join(t.TempDir(), "archive.gz")
			if err := os.WriteFile(archive, []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" && r.URL.Path == "/jobs/"+job.JobID {
					json.NewEncoder(w).Encode(job)
					return
				}
				stage := "artifact"
				base := "/sites/" + job.SiteID + "/artifacts/" + job.TargetVersion
				switch r.URL.Path {
				case base:
					if r.Method != "PUT" {
						t.Error("artifact method")
					}
				case base + "/logs":
					stage = "logs"
				case base + "/manifest":
					stage = "manifest"
				case "/jobs/" + job.JobID + "/result":
					stage = "callback"
				default:
					t.Errorf("unexpected URL %s", r.URL)
					w.WriteHeader(404)
					return
				}
				calls = append(calls, stage)
				body, _ := io.ReadAll(r.Body)
				if stage != "artifact" && (r.Method != "POST" || r.Header.Get("Content-Type") != "application/json") {
					t.Error("metadata/callback wire contract")
				}
				if stage == "logs" && (!strings.Contains(string(body), "executor output withheld") || strings.Contains(string(body), "private output")) {
					t.Error("missing execution log")
				}
				if stage == "manifest" {
					var got types.Manifest
					if json.Unmarshal(body, &got) != nil || got.BuildID != job.TargetVersion {
						t.Error("manifest differs")
					}
				}
				if stage == "callback" {
					var got types.JobResult
					if json.Unmarshal(body, &got) != nil || got.ManifestPath != base+"/manifest" || got.Status != "completed" {
						t.Error("wrong completion")
					}
				}
				if stage == failure {
					w.WriteHeader(500)
					return
				}
				if stage == "callback" {
					w.WriteHeader(200)
					w.Write(body)
				} else {
					w.WriteHeader(201)
				}
			}))
			defer server.Close()
			err := persistAndReport(storage.NewClient(server.URL+"/"), server.URL, &job, archive, manifest, "private output")
			if (err != nil) != (failure != "") {
				t.Fatalf("error=%v failure=%s", err, failure)
			}
			want := []string{"artifact", "logs", "manifest", "callback"}
			if failure == "callback" {
				want = append(want, "callback", "callback", "callback")
			}
			for i, stage := range want {
				if stage == failure && stage != "callback" {
					want = want[:i+1]
					break
				}
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("calls %v, want %v", calls, want)
			}
		})
	}
}
