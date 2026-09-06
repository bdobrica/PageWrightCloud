package reconciler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
)

type fakeQueue struct {
	candidates      []Candidate
	statuses, codes []string
}

func (q *fakeQueue) Candidates(context.Context, time.Duration) ([]Candidate, error) {
	return q.candidates, nil
}
func (q *fakeQueue) Recover(_ context.Context, _ Candidate, status, code, message string) error {
	q.statuses = append(q.statuses, status)
	q.codes = append(q.codes, code)
	return nil
}

type fakeWorkers struct {
	state spawner.WorkerState
	err   error
	kills int
}

func (w *fakeWorkers) Inspect(context.Context, *types.Job) (spawner.WorkerState, error) {
	return w.state, w.err
}
func (w *fakeWorkers) Kill(context.Context, string) error { w.kills++; return nil }

func TestReconciliationEvidence(t *testing.T) {
	for _, scenario := range []string{"complete", "missing_manifest", "wrong_bytes", "partial_receipt", "empty_exit", "oom", "running", "missing_worker", "timeout", "daemon_down_timeout", "storage_down", "receipt_mismatch"} {
		t.Run(scenario, func(t *testing.T) {
			j := types.Job{JobID: "job", SiteID: "site", OwnerID: "owner", SourceVersion: "initial", TargetVersion: "version", LockToken: "lock", FencingToken: 3}
			fingerprint := fmt.Sprintf("%x:7", sha256.Sum256([]byte("content")))
			record := map[string]any{"job_id": j.JobID, "lock_token": j.LockToken, "fencing_token": j.FencingToken, "artifact": fingerprint, "logs": fingerprint, "manifest": fingerprint}
			if scenario == "partial_receipt" {
				delete(record, "manifest")
			}
			if scenario == "receipt_mismatch" {
				record["lock_token"] = "other"
			}
			data, _ := json.Marshal(record)
			c := Candidate{Job: j, Receipt: string(data)}
			workers := &fakeWorkers{state: spawner.WorkerState{ID: strings.Repeat("a", 64), Exists: true, Exited: true}}
			switch scenario {
			case "empty_exit", "oom":
				c.Receipt = ""
				workers.state.OOMKilled = scenario == "oom"
			case "running":
				workers.state.Exited = false
				workers.state.Running = true
			case "missing_worker":
				workers.state = spawner.WorkerState{}
			case "timeout":
				c.Expired = true
				c.Receipt = ""
				workers.state.Exited = false
				workers.state.Running = true
			case "daemon_down_timeout":
				c.Expired = true
				c.Receipt = ""
				workers.err = errors.New("daemon unavailable")
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if scenario == "missing_manifest" && strings.HasSuffix(r.URL.Path, "/manifest") {
					w.WriteHeader(404)
					return
				}
				if scenario == "storage_down" {
					w.WriteHeader(503)
					return
				}
				if scenario == "wrong_bytes" {
					w.Write([]byte("changed"))
					return
				}
				w.Write([]byte("content"))
			}))
			defer srv.Close()
			q := &fakeQueue{candidates: []Candidate{c}}
			r := Reconciler{Queue: q, Workers: workers, StorageURL: srv.URL, Lifetime: time.Minute}
			err := r.Once(context.Background())
			if scenario == "storage_down" || scenario == "receipt_mismatch" {
				if err == nil || len(q.statuses) != 0 {
					t.Fatal("unavailable evidence accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "running" || scenario == "missing_worker" {
				if len(q.statuses) != 0 {
					t.Fatal("premature terminal")
				}
				return
			}
			wantStatus, wantCode := "failed", "artifact_incomplete"
			switch scenario {
			case "complete":
				wantStatus = "completed"
				wantCode = ""
			case "empty_exit":
				wantCode = "worker_exit"
			case "oom":
				wantCode = "worker_oom"
			case "timeout", "daemon_down_timeout":
				wantCode = "worker_timeout"
			}
			if len(q.statuses) != 1 || q.statuses[0] != wantStatus || q.codes[0] != wantCode {
				t.Fatalf("outcome %v %v", q.statuses, q.codes)
			}
			if (workers.kills == 1) != (scenario == "timeout") {
				t.Fatal("unsafe kill")
			}
		})
	}
}
