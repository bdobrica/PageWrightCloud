package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
)

type memoryQueue struct {
	mu           sync.Mutex
	jobs         map[string]types.Job
	writes       int
	failUpdate   bool
	failCreate   bool
	failGet      bool
	failWorkerID bool
}

func (q *memoryQueue) CreateJob(_ context.Context, j *types.Job) (*types.Job, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.failCreate {
		return nil, false, errors.New("unavailable")
	}
	if existing, ok := q.jobs[j.JobID]; ok {
		return &existing, false, nil
	}
	q.jobs[j.JobID] = *j
	q.writes++
	copy := *j
	return &copy, true, nil
}
func (q *memoryQueue) GetJob(_ context.Context, id string) (*types.Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.failGet {
		return nil, errors.New("unavailable")
	}
	j, ok := q.jobs[id]
	if !ok {
		return nil, queue.ErrJobNotFound
	}
	return &j, nil
}
func (q *memoryQueue) UpdateJob(ctx context.Context, j *types.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.failUpdate {
		return errors.New("unavailable")
	}
	if _, ok := q.jobs[j.JobID]; !ok {
		return queue.ErrJobNotFound
	}
	q.jobs[j.JobID] = *j
	q.writes++
	return nil
}
func (q *memoryQueue) SetWorkerID(_ context.Context, id, workerID string) (*types.Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.failWorkerID {
		return nil, errors.New("unavailable")
	}
	j, ok := q.jobs[id]
	if !ok {
		return nil, queue.ErrJobNotFound
	}
	j.WorkerID = workerID
	q.jobs[id] = j
	q.writes++
	return &j, nil
}
func (q *memoryQueue) Close() error { return nil }

type fakeLock struct {
	acquired, released int
	busy               bool
}

func (l *fakeLock) Acquire(context.Context, string, time.Duration) (string, int64, error) {
	if l.busy {
		return "", 0, errors.New("locked")
	}
	l.acquired++
	return "lock", 1, nil
}
func (l *fakeLock) Renew(context.Context, string, string, time.Duration) error { return nil }
func (l *fakeLock) Release(context.Context, string, string) error              { l.released++; return nil }
func (l *fakeLock) Close() error                                               { return nil }

func fixture() (http.Handler, *memoryQueue, *fakeLock) {
	q := &memoryQueue{jobs: make(map[string]types.Job)}
	l := &fakeLock{}
	return NewHandler(q, l).SetupRoutes(), q, l
}
func call(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if raw, ok := body.(string); ok {
		data = []byte(raw)
	} else if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewReader(data)))
	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("non-JSON response: %s", w.Body.String())
	}
	return w
}
func request() types.JobRequest {
	return types.JobRequest{SiteID: "site", OwnerID: "owner", Prompt: "Build a page", SourceVersion: "v1", TargetVersion: "v2"}
}
func create(t *testing.T, h http.Handler) types.Job {
	t.Helper()
	w := call(t, h, "POST", "/jobs", request())
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var j types.Job
	if err := json.Unmarshal(w.Body.Bytes(), &j); err != nil {
		t.Fatal(err)
	}
	return j
}
func callback(j types.Job) types.JobStatusUpdate {
	return types.JobStatusUpdate{LockToken: j.LockToken, FencingToken: j.FencingToken, JobID: j.JobID, SiteID: j.SiteID, OwnerID: j.OwnerID, SourceVersion: j.SourceVersion, TargetVersion: j.TargetVersion, Status: types.JobStatusCompleted, Result: "Done", ManifestPath: "v2/manifest.json"}
}
func assertError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var e types.APIError
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if w.Code != status || e.Error != code || e.Message == "" {
		t.Fatalf("want %d/%s; got %d %s", status, code, w.Code, w.Body.String())
	}
}

func TestCreateAndGetContract(t *testing.T) {
	for _, target := range []string{"v2", ""} {
		t.Run("target="+target, func(t *testing.T) {
			h, q, l := fixture()
			req := request()
			req.TargetVersion = target
			w := call(t, h, "POST", "/jobs", req)
			if w.Code != http.StatusCreated {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			var j types.Job
			if err := json.Unmarshal(w.Body.Bytes(), &j); err != nil {
				t.Fatal(err)
			}
			if j.JobID == "" || j.SiteID != req.SiteID || j.OwnerID != req.OwnerID || j.Prompt != req.Prompt || j.SourceVersion != req.SourceVersion || j.TargetVersion == "" || (target != "" && j.TargetVersion != target) || j.Status != types.JobStatusPending || j.WorkerID != "" || j.CreatedAt.IsZero() || j.UpdatedAt.IsZero() {
				t.Fatalf("incorrect snapshot: %+v", j)
			}
			if l.acquired != 0 || q.jobs[j.JobID] != j {
				t.Fatal("job identity not propagated")
			}
			got := call(t, h, "GET", "/jobs/"+j.JobID, nil)
			if got.Code != http.StatusOK || got.Body.String() != w.Body.String() {
				t.Fatalf("GET differs: %s", got.Body.String())
			}
		})
	}
}

func TestRejectInvalidRequestsBeforeSideEffects(t *testing.T) {
	bodies := []string{
		"", "{", "null", "[]", "{}",
		`{"site_id":"site","owner_id":"owner","prompt":"p"}`,
		`{"site_id":"site","prompt":"p","source_version":"v1"}`,
		`{"site_id":"site","owner_id":"owner","prompt":"  ","source_version":"v1"}`,
		`{"site_id":" ","owner_id":"owner","prompt":"p","source_version":"v1"}`,
		`{"site_id":"site","owner_id":" ","prompt":"p","source_version":"v1"}`,
		`{"site_id":"site","owner_id":"owner","prompt":"p","source_version":" "}`,
		`{"site_id":"site","owner_id":"owner","prompt":"p","source_version":"v1","target_version":" "}`,
		`{"site_id":"site","owner_id":"owner","prompt":"p","source_version":"v1","job_id":"caller"}`,
		`{"site_id":"site","build_id":"build","input":"p","user_id":"owner"}`,
		`{"site_id":"site","owner_id":"owner","prompt":"p","source_version":"v1"}{}`,
		`{"site_id":"site","owner_id":"owner","prompt":"p","source_version":"v1"} junk`,
	}
	for _, body := range bodies {
		t.Run(body, func(t *testing.T) {
			h, q, l := fixture()
			assertError(t, call(t, h, "POST", "/jobs", body), 400, "invalid_request")
			if q.writes != 0 || l.acquired != 0 {
				t.Fatal("invalid request caused side effects")
			}
		})
	}
}

func TestCallbackSuccessAndFailureSnapshots(t *testing.T) {
	for _, endpoint := range []string{"status", "result"} {
		for _, status := range []types.JobStatus{types.JobStatusCompleted, types.JobStatusFailed, types.JobStatusRunning} {
			if endpoint == "result" && status == types.JobStatusRunning {
				continue
			}
			t.Run(endpoint+"/"+string(status), func(t *testing.T) {
				h, q, l := fixture()
				j := create(t, h)
				j.Status, j.LockToken = types.JobStatusRunning, "test-lock"
				j.FencingToken = 1
				q.jobs[j.JobID] = j
				update := callback(j)
				update.Status = status
				if status != types.JobStatusCompleted {
					update.ManifestPath = ""
				}
				if status == types.JobStatusFailed {
					update.ErrorMessage = "Build failed"
				}
				w := call(t, h, "POST", "/jobs/"+j.JobID+"/"+endpoint, update)
				if w.Code != 200 {
					t.Fatalf("%d %s", w.Code, w.Body.String())
				}
				var got types.Job
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.Status != status || got.Result != update.Result || got.ErrorMessage != update.ErrorMessage || got.ManifestPath != update.ManifestPath || got.OwnerID != j.OwnerID || got.TargetVersion != j.TargetVersion || got.UpdatedAt.Before(j.UpdatedAt) || q.jobs[j.JobID] != got {
					t.Fatalf("incorrect callback snapshot: %+v", got)
				}
				// Release belongs to the atomic queue commit, not a later HTTP step.
				if l.released != 0 {
					t.Fatalf("lock releases: %d", l.released)
				}
			})
		}
	}
}

func TestCallbackValidationDoesNotMutateJob(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*types.JobStatusUpdate)
		code   int
	}{
		{"job mismatch", func(u *types.JobStatusUpdate) { u.JobID = "other" }, 409},
		{"site mismatch", func(u *types.JobStatusUpdate) { u.SiteID = "other" }, 409},
		{"owner mismatch", func(u *types.JobStatusUpdate) { u.OwnerID = "other" }, 409},
		{"source mismatch", func(u *types.JobStatusUpdate) { u.SourceVersion = "other" }, 409},
		{"target mismatch", func(u *types.JobStatusUpdate) { u.TargetVersion = "other" }, 409},
		{"missing job", func(u *types.JobStatusUpdate) { u.JobID = "" }, 400},
		{"missing site", func(u *types.JobStatusUpdate) { u.SiteID = "" }, 400},
		{"missing owner", func(u *types.JobStatusUpdate) { u.OwnerID = "" }, 400},
		{"missing source", func(u *types.JobStatusUpdate) { u.SourceVersion = "" }, 400},
		{"blank target", func(u *types.JobStatusUpdate) { u.TargetVersion = " " }, 400},
		{"missing status", func(u *types.JobStatusUpdate) { u.Status = "" }, 400},
		{"pending", func(u *types.JobStatusUpdate) { u.Status = types.JobStatusPending }, 400},
		{"unknown status", func(u *types.JobStatusUpdate) { u.Status = "success" }, 400},
		{"missing error", func(u *types.JobStatusUpdate) { u.Status = types.JobStatusFailed; u.ManifestPath = "" }, 400},
		{"blank error", func(u *types.JobStatusUpdate) {
			u.Status = types.JobStatusFailed
			u.ManifestPath = ""
			u.ErrorMessage = " "
		}, 400},
		{"error on success", func(u *types.JobStatusUpdate) { u.ErrorMessage = "oops" }, 400},
		{"manifest on failure", func(u *types.JobStatusUpdate) { u.Status = types.JobStatusFailed; u.ErrorMessage = "oops" }, 400},
	}
	for _, endpoint := range []string{"status", "result"} {
		for _, tc := range mutations {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				h, q, l := fixture()
				j := create(t, h)
				beforeWrites := q.writes
				update := callback(j)
				tc.mutate(&update)
				code := "invalid_request"
				if tc.code == 409 {
					code = "job_conflict"
				}
				assertError(t, call(t, h, "POST", "/jobs/"+j.JobID+"/"+endpoint, update), tc.code, code)
				if !reflect.DeepEqual(q.jobs[j.JobID], j) || q.writes != beforeWrites || l.released != 0 {
					t.Fatal("rejected callback mutated job or released lock")
				}
			})
		}
	}
}

func TestCallbackStrictJSONAndResultStatus(t *testing.T) {
	for _, endpoint := range []string{"status", "result"} {
		for _, suffix := range []string{",\"unknown\":true}", "}{}", "} trailing"} {
			t.Run(endpoint+"/"+suffix, func(t *testing.T) {
				h, q, l := fixture()
				j := create(t, h)
				b, _ := json.Marshal(callback(j))
				body := string(b[:len(b)-1]) + suffix
				writes := q.writes
				assertError(t, call(t, h, "POST", "/jobs/"+j.JobID+"/"+endpoint, body), 400, "invalid_request")
				if q.writes != writes || l.released != 0 {
					t.Fatal("invalid JSON caused side effects")
				}
			})
		}
	}
	h, _, _ := fixture()
	j := create(t, h)
	u := callback(j)
	u.Status = types.JobStatusRunning
	u.ManifestPath = ""
	assertError(t, call(t, h, "POST", "/jobs/"+j.JobID+"/result", u), 400, "invalid_request")
}

func TestErrorEnvelopes(t *testing.T) {
	h, q, l := fixture()
	assertError(t, call(t, h, "GET", "/jobs/missing", nil), 404, "job_not_found")
	u := callback(types.Job{JobID: "missing", SiteID: "site", OwnerID: "owner", SourceVersion: "v1", TargetVersion: "v2"})
	assertError(t, call(t, h, "POST", "/jobs/missing/result", u), 404, "job_not_found")
	j := create(t, h)
	j.Status, j.LockToken = types.JobStatusRunning, "test-lock"
	j.FencingToken = 1
	q.jobs[j.JobID] = j
	q.failUpdate = true
	assertError(t, call(t, h, "POST", "/jobs/"+j.JobID+"/result", callback(j)), 500, "internal_error")
	if q.jobs[j.JobID] != j || l.released != 0 {
		t.Fatal("failed persistence mutated job or released lock")
	}
}

func TestPendingJobCannotReportExecutionOutcome(t *testing.T) {
	h, q, l := fixture()
	j := create(t, h)
	writes := q.writes
	assertError(t, call(t, h, "POST", "/jobs/"+j.JobID+"/result", callback(j)), 409, "job_conflict")
	if q.writes != writes || l.released != 0 {
		t.Fatal("pending callback changed reservation")
	}
}
