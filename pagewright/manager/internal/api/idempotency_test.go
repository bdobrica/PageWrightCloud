package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/google/uuid"
)

func explicitRequest() types.JobRequest {
	req := request()
	req.JobID = uuid.NewString()
	return req
}

func TestIdempotentSubmissionAndLostResponse(t *testing.T) {
	h, q, l, s := fixture()
	req := explicitRequest()
	first := call(t, h, "POST", "/jobs", req)
	if first.Code != 201 {
		t.Fatalf("%d %s", first.Code, first.Body)
	}
	// Treat the first response as lost. The same request retrieves its reservation.
	replay := call(t, h, "POST", "/jobs", req)
	if replay.Code != 200 || replay.Body.String() != first.Body.String() {
		t.Fatalf("replay %d %s", replay.Code, replay.Body)
	}
	if len(q.jobs) != 1 || l.acquired != 1 || len(s.jobs) != 1 {
		t.Fatal("retry duplicated side effects")
	}
}

func TestConcurrentDuplicateSubmissions(t *testing.T) {
	h, q, l, s := fixture()
	req := explicitRequest()
	var wg sync.WaitGroup
	codes := make(chan int, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- call(t, h, "POST", "/jobs", req).Code }()
	}
	wg.Wait()
	close(codes)
	created := 0
	for code := range codes {
		if code == 201 {
			created++
		} else if code != 200 {
			t.Fatalf("unexpected %d", code)
		}
	}
	if created != 1 || len(q.jobs) != 1 || l.acquired != 1 || len(s.jobs) != 1 {
		t.Fatalf("claims=%d locks=%d spawns=%d", created, l.acquired, len(s.jobs))
	}
}

func TestDuplicateIdentityConflicts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*types.JobRequest)
	}{
		{"site", func(r *types.JobRequest) { r.SiteID = "other" }},
		{"owner", func(r *types.JobRequest) { r.OwnerID = "other" }},
		{"prompt", func(r *types.JobRequest) { r.Prompt = "other" }},
		{"source", func(r *types.JobRequest) { r.SourceVersion = "other" }},
		{"target", func(r *types.JobRequest) { r.TargetVersion = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, q, l, s := fixture()
			req := explicitRequest()
			call(t, h, "POST", "/jobs", req)
			before := q.jobs[req.JobID]
			tc.change(&req)
			assertError(t, call(t, h, "POST", "/jobs", req), 409, "job_conflict")
			if q.jobs[req.JobID] != before || l.acquired != 1 || len(s.jobs) != 1 {
				t.Fatal("conflict changed reservation")
			}
		})
	}
}

func TestExplicitJobIDValidation(t *testing.T) {
	for _, change := range []func(*types.JobRequest){
		func(r *types.JobRequest) { r.JobID = "bad" },
		func(r *types.JobRequest) { r.JobID = uuid.Nil.String() },
		func(r *types.JobRequest) { r.JobID = "ABCDEF01-2345-4678-9ABC-DEF012345678" },
		func(r *types.JobRequest) { r.JobID = "urn:uuid:" + r.JobID },
		func(r *types.JobRequest) { r.JobID = "{" + r.JobID + "}" },
		func(r *types.JobRequest) { r.JobID = strings.ReplaceAll(r.JobID, "-", "") },
		func(r *types.JobRequest) { r.TargetVersion = "" },
		func(r *types.JobRequest) { r.TargetVersion = " " },
		func(r *types.JobRequest) { r.TargetVersion = r.JobID },
		func(r *types.JobRequest) { r.TargetVersion = strings.ToUpper(r.JobID) },
		func(r *types.JobRequest) { r.TargetVersion = "urn:uuid:" + r.JobID },
		func(r *types.JobRequest) { r.TargetVersion = "{" + r.JobID + "}" },
		func(r *types.JobRequest) { r.TargetVersion = strings.ReplaceAll(r.JobID, "-", "") },
	} {
		h, q, l, s := fixture()
		req := explicitRequest()
		change(&req)
		assertError(t, call(t, h, "POST", "/jobs", req), 400, "invalid_request")
		if q.writes != 0 || l.acquired != 0 || len(s.jobs) != 0 {
			t.Fatal("invalid identity caused side effects")
		}
	}
}

func TestSubmissionRejectionIsRemembered(t *testing.T) {
	for _, kind := range []string{"job_busy", "spawn_failed"} {
		t.Run(kind, func(t *testing.T) {
			h, q, l, s := fixture()
			req := explicitRequest()
			status := 409
			if kind == "job_busy" {
				l.busy = true
			} else {
				s.fail = true
				status = 502
			}
			assertError(t, call(t, h, "POST", "/jobs", req), status, kind)
			saved := q.jobs[req.JobID]
			if saved.Status != types.JobStatusFailed || saved.ErrorCode != kind || saved.ErrorMessage == "" {
				t.Fatalf("missing rejection snapshot: %+v", saved)
			}
			l.busy = false
			s.fail = false
			locks, spawns := l.acquired, len(s.jobs)
			assertError(t, call(t, h, "POST", "/jobs", req), status, kind)
			if l.acquired != locks || len(s.jobs) != spawns || q.jobs[req.JobID] != saved {
				t.Fatal("rejection retry dispatched")
			}
		})
	}
}

func TestPersistenceFailuresNeverRespawnReservation(t *testing.T) {
	for _, kind := range []string{"running update", "busy rejection", "spawn rejection", "worker identity"} {
		t.Run(kind, func(t *testing.T) {
			h, q, l, s := fixture()
			req := explicitRequest()
			switch kind {
			case "running update":
				q.failUpdate = true
			case "busy rejection":
				q.failUpdate = true
				l.busy = true
			case "spawn rejection":
				s.fail = true
				s.onSpawn = func(*types.Job) { q.failUpdate = true }
			case "worker identity":
				q.failWorkerID = true
			}
			assertError(t, call(t, h, "POST", "/jobs", req), 500, "internal_error")
			locks, spawns := l.acquired, len(s.jobs)
			q.failUpdate = false
			q.failWorkerID = false
			l.busy = false
			s.fail = false
			replay := call(t, h, "POST", "/jobs", req)
			if replay.Code != 200 {
				t.Fatalf("retry: %d %s", replay.Code, replay.Body)
			}
			if locks != l.acquired || spawns != len(s.jobs) {
				t.Fatal("retry dispatched ambiguous reservation")
			}
		})
	}
}

func TestBackendErrorsAreNotMissingJobs(t *testing.T) {
	h, q, l, s := fixture()
	q.failCreate = true
	assertError(t, call(t, h, "POST", "/jobs", explicitRequest()), 500, "internal_error")
	if l.acquired != 0 || len(s.jobs) != 0 {
		t.Fatal("backend failure reached lock/spawn")
	}
	q.failGet = true
	assertError(t, call(t, h, "GET", "/jobs/missing", nil), 500, "internal_error")
	u := callback(types.Job{JobID: "missing", SiteID: "site", OwnerID: "owner", SourceVersion: "v1", TargetVersion: "v2"})
	assertError(t, call(t, h, "POST", "/jobs/missing/result", u), 500, "internal_error")
}

func TestFastCallbackIsNotOverwrittenBySpawnMetadata(t *testing.T) {
	h, q, _, s := fixture()
	req := explicitRequest()
	s.onSpawn = func(j *types.Job) {
		response := call(t, h, "POST", "/jobs/"+j.JobID+"/result", callback(*j))
		if response.Code != http.StatusOK {
			t.Fatalf("callback %d %s", response.Code, response.Body)
		}
	}
	response := call(t, h, "POST", "/jobs", req)
	var got types.Job
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if response.Code != 201 || got.Status != types.JobStatusCompleted || got.ManifestPath == "" || got.WorkerID != "worker" || q.jobs[req.JobID] != got {
		t.Fatalf("callback outcome lost: %d %+v", response.Code, got)
	}
}

func TestCallbackClearsSubmissionErrorCode(t *testing.T) {
	h, q, l, _ := fixture()
	req := explicitRequest()
	l.busy = true
	call(t, h, "POST", "/jobs", req)
	j := q.jobs[req.JobID]
	resp := call(t, h, "POST", "/jobs/"+j.JobID+"/result", callback(j))
	if resp.Code != 200 || q.jobs[j.JobID].ErrorCode != "" {
		t.Fatalf("stale rejection code: %s", resp.Body)
	}
}
