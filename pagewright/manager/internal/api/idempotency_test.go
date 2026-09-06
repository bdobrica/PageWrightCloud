package api

import (
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
	h, q, l := fixture()
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
	if len(q.jobs) != 1 || l.acquired != 0 {
		t.Fatal("retry duplicated side effects")
	}
}

func TestConcurrentDuplicateSubmissions(t *testing.T) {
	h, q, l := fixture()
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
	if created != 1 || len(q.jobs) != 1 || l.acquired != 0 {
		t.Fatalf("claims=%d locks=%d", created, l.acquired)
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
			h, q, l := fixture()
			req := explicitRequest()
			call(t, h, "POST", "/jobs", req)
			before := q.jobs[req.JobID]
			tc.change(&req)
			assertError(t, call(t, h, "POST", "/jobs", req), 409, "job_conflict")
			if q.jobs[req.JobID] != before || l.acquired != 0 {
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
		h, q, l := fixture()
		req := explicitRequest()
		change(&req)
		assertError(t, call(t, h, "POST", "/jobs", req), 400, "invalid_request")
		if q.writes != 0 || l.acquired != 0 {
			t.Fatal("invalid identity caused side effects")
		}
	}
}

func TestBackendErrorsAreNotMissingJobs(t *testing.T) {
	h, q, l := fixture()
	q.failCreate = true
	assertError(t, call(t, h, "POST", "/jobs", explicitRequest()), 500, "internal_error")
	if l.acquired != 0 {
		t.Fatal("backend failure reached lock/spawn")
	}
	q.failGet = true
	assertError(t, call(t, h, "GET", "/jobs/missing", nil), 500, "internal_error")
	u := callback(types.Job{JobID: "missing", SiteID: "site", OwnerID: "owner", SourceVersion: "v1", TargetVersion: "v2"})
	assertError(t, call(t, h, "POST", "/jobs/missing/result", u), 500, "internal_error")
}
