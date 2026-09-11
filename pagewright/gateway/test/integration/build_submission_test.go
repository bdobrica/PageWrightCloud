//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type submissionProvider struct{ calls atomic.Int32 }

type emptyCompletedVersions struct{}

func (emptyCompletedVersions) ListVersionsContext(context.Context, string) ([]clients.StorageVersion, error) {
	return nil, nil
}

func (p *submissionProvider) EvaluateRequestContext(context.Context, string) (*clients.EvaluationResponse, error) {
	p.calls.Add(1)
	return &clients.EvaluationResponse{IsClear: true}, nil
}
func (p *submissionProvider) GenerateJobInstructionsContext(context.Context, string, string) (string, error) {
	p.calls.Add(1)
	return "Change the title", nil
}

type submissionStore interface {
	GetSiteByFQDNContext(context.Context, string) (*types.Site, error)
	FindBuildSubmission(context.Context, string, string, string) (*database.BuildSubmission, error)
	ReserveBuildSubmission(context.Context, *database.BuildSubmission) (*database.BuildSubmission, bool, error)
	ClaimBuildDispatch(context.Context, string) (bool, error)
	RecordBuildOutcome(context.Context, string, string, string, string, string, int) error
}
type failingSubmissionStore struct {
	*database.DB
	reserveFails bool
	outcomeFails atomic.Bool
}

func (s *failingSubmissionStore) ReserveBuildSubmission(ctx context.Context, p *database.BuildSubmission) (*database.BuildSubmission, bool, error) {
	if s.reserveFails {
		return nil, false, errors.New("forced reservation failure")
	}
	return s.DB.ReserveBuildSubmission(ctx, p)
}
func (s *failingSubmissionStore) RecordBuildOutcome(ctx context.Context, id, state, status, code, message string, responseStatus int) error {
	if s.outcomeFails.Load() {
		return errors.New("forced outcome write failure")
	}
	return s.DB.RecordBuildOutcome(ctx, id, state, status, code, message, responseStatus)
}

type submissionManager struct {
	t            *testing.T
	key          string
	posts        atomic.Int32
	getFails     atomic.Bool
	dropResponse bool
	reject       bool
	delay        time.Duration
	mu           sync.Mutex
	job          *types.Job
}

func (m *submissionManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.getFails.Load() {
			w.WriteHeader(503)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal_error", "message": "unavailable"})
			return
		}
		if m.job == nil {
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]string{"error": "job_not_found", "message": "not found"})
			return
		}
		json.NewEncoder(w).Encode(m.job)
		return
	}
	m.posts.Add(1)
	var req clients.ManagerJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		m.t.Error(err)
		w.WriteHeader(400)
		return
	}
	// Inspect the actual DB while the manager is receiving POST, not after it:
	// both IDs and their pending version must already be committed and visible.
	saved, err := testDB.FindBuildSubmission(context.Background(), req.OwnerID, req.SiteID, m.key)
	if err != nil || saved == nil || saved.JobID != req.JobID || saved.TargetVersion != req.TargetVersion || saved.SourceVersion != req.SourceVersion || saved.Prompt != req.Prompt || saved.DispatchState != "dispatching" || saved.JobID == saved.TargetVersion {
		m.t.Errorf("manager received POST without committed canonical mapping: %#v %v", saved, err)
		w.WriteHeader(500)
		return
	}
	var versionStatus string
	if err := testDB.Get(&versionStatus, "SELECT status FROM versions WHERE site_id=$1 AND build_id=$2", req.SiteID, req.TargetVersion); err != nil || versionStatus != "pending" {
		m.t.Errorf("version missing before dispatch: %q %v", versionStatus, err)
		w.WriteHeader(500)
		return
	}
	if m.reject {
		m.mu.Lock()
		m.job = &types.Job{JobAccepted: types.JobAccepted{JobID: req.JobID, SiteID: req.SiteID, OwnerID: req.OwnerID, SourceVersion: req.SourceVersion, TargetVersion: req.TargetVersion, Status: types.JobStatusFailed, ErrorMessage: "site locked"}, Prompt: req.Prompt, ErrorCode: "job_busy", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		m.mu.Unlock()
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]string{"error": "job_busy", "message": "site locked"})
		return
	}
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	m.mu.Lock()
	job := &types.Job{JobAccepted: types.JobAccepted{JobID: req.JobID, SiteID: req.SiteID, OwnerID: req.OwnerID, SourceVersion: req.SourceVersion, TargetVersion: req.TargetVersion, Status: types.JobStatusRunning}, Prompt: req.Prompt, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	m.job = job
	m.mu.Unlock()
	if m.dropResponse {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			m.t.Error(err)
			return
		}
		conn.Close()
		return
	}
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(job)
}

func submissionSite(t *testing.T) (*types.Site, string) {
	t.Helper()
	user, err := testDB.CreateUser(uuid.NewString()+"@submission.test", "test-hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	site, err := testDB.CreateSite(user.ID, uuid.NewString()+".submission.test", "starter")
	if err != nil {
		t.Fatal(err)
	}
	token, err := testJWTManager.GenerateToken(user.ID, user.Email)
	if err != nil {
		t.Fatal(err)
	}
	return site, token
}
func submissionGateway(t *testing.T, store submissionStore, provider *submissionProvider, managerURL string) *httptest.Server {
	t.Helper()
	return submissionGatewayWithVersions(t, store, provider, managerURL, emptyCompletedVersions{})
}
func submissionGatewayWithVersions(t *testing.T, store submissionStore, provider *submissionProvider, managerURL string, versions interface {
	ListVersionsContext(context.Context, string) ([]clients.StorageVersion, error)
}) *httptest.Server {
	t.Helper()
	h := handlers.NewBuildHandler(store, provider, clients.NewManagerClient(managerURL), versions)
	r := mux.NewRouter()
	r.Handle("/sites/{fqdn}/build", middleware.AuthMiddleware(testJWTManager)(http.HandlerFunc(h.Build))).Methods("POST")
	server := httptest.NewServer(middleware.CORS(r))
	t.Cleanup(server.Close)
	return server
}
func submit(t *testing.T, server *httptest.Server, site *types.Site, token, key, message string) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(types.BuildRequest{Message: message})
	req, _ := http.NewRequest("POST", server.URL+"/sites/"+site.FQDN+"/build", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", key)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, payload
}
func assertSubmissionRows(t *testing.T, site *types.Site, want int, status string) {
	t.Helper()
	var jobs, versions int
	if err := testDB.Get(&jobs, "SELECT count(*) FROM build_submissions WHERE site_id=$1", site.ID); err != nil {
		t.Fatal(err)
	}
	if err := testDB.Get(&versions, "SELECT count(*) FROM versions WHERE site_id=$1", site.ID); err != nil {
		t.Fatal(err)
	}
	if jobs != want || versions != want {
		t.Fatalf("jobs=%d versions=%d want=%d", jobs, versions, want)
	}
	if want == 1 {
		var got string
		if err := testDB.Get(&got, "SELECT status FROM versions WHERE site_id=$1", site.ID); err != nil || got != status {
			t.Fatalf("version status=%q err=%v want=%s", got, err, status)
		}
	}
}

func TestBuildSubmissionPersistenceAndReplay(t *testing.T) {
	site, token := submissionSite(t)
	key := uuid.NewString()
	m := &submissionManager{t: t, key: key}
	manager := httptest.NewServer(m)
	defer manager.Close()
	p := &submissionProvider{}
	server := submissionGateway(t, testDB, p, manager.URL)
	status, first := submit(t, server, site, token, key, "change title")
	if status != 200 || first["job_id"] == first["target_version"] {
		t.Fatalf("first=%d %#v", status, first)
	}
	// A new handler simulates losing all in-memory gateway state.
	p2 := &submissionProvider{}
	restarted := submissionGateway(t, testDB, p2, manager.URL)
	status, replay := submit(t, restarted, site, token, key, "change title")
	if status != 200 || replay["job_id"] != first["job_id"] || replay["target_version"] != first["target_version"] || p2.calls.Load() != 0 || m.posts.Load() != 1 {
		t.Fatalf("replay=%d %#v calls=%d", status, replay, m.posts.Load())
	}
	status, _ = submit(t, restarted, site, token, key, "different input")
	if status != 409 || m.posts.Load() != 1 || p2.calls.Load() != 0 {
		t.Fatalf("key conflict=%d posts=%d", status, m.posts.Load())
	}
	assertSubmissionRows(t, site, 1, "running")
}

type fixedCompletedVersions struct {
	versions []clients.StorageVersion
	err      error
}

func (s fixedCompletedVersions) ListVersionsContext(context.Context, string) ([]clients.StorageVersion, error) {
	return s.versions, s.err
}

func TestBuildBaseIsPinnedAcrossRestartAndStorageFailure(t *testing.T) {
	site, token := submissionSite(t)
	key := uuid.NewString()
	m := &submissionManager{t: t, key: key}
	manager := httptest.NewServer(m)
	defer manager.Close()
	versions := fixedCompletedVersions{versions: []clients.StorageVersion{{BuildID: "draft-one", Status: "completed", Timestamp: time.Now().UTC()}}}
	server := submissionGatewayWithVersions(t, testDB, &submissionProvider{}, manager.URL, versions)
	status, first := submit(t, server, site, token, key, "change title")
	if status != 200 || first["source_version"] != "draft-one" {
		t.Fatalf("first=%d %+v", status, first)
	}
	// Even newer content cannot change the source of this same accepted request.
	versions.versions = []clients.StorageVersion{{BuildID: "draft-two", Status: "completed", Timestamp: time.Now().UTC()}}
	restarted := submissionGatewayWithVersions(t, testDB, &submissionProvider{}, manager.URL, versions)
	status, replay := submit(t, restarted, site, token, key, "change title")
	if status != 200 || replay["source_version"] != "draft-one" || replay["target_version"] != first["target_version"] {
		t.Fatalf("replay=%d %+v", status, replay)
	}
	offline := submissionGatewayWithVersions(t, testDB, &submissionProvider{}, manager.URL, fixedCompletedVersions{err: errors.New("offline")})
	status, replay = submit(t, offline, site, token, key, "change title")
	if status != 200 || replay["source_version"] != "draft-one" {
		t.Fatalf("offline replay=%d %+v", status, replay)
	}
	status, _ = submit(t, offline, site, token, uuid.NewString(), "new edit")
	if status != 502 || m.posts.Load() != 1 {
		t.Fatalf("unsafe storage failure fallback: %d posts=%d", status, m.posts.Load())
	}
	assertSubmissionRows(t, site, 1, "running")
}

func TestBuildSubmissionFailureHandling(t *testing.T) {
	for _, mode := range []string{"reservation failure", "manager rejection", "rejection outcome failure", "dial failure", "lost response", "outcome write failure", "missing uncertain job"} {
		t.Run(mode, func(t *testing.T) {
			site, token := submissionSite(t)
			key := uuid.NewString()
			store := &failingSubmissionStore{DB: testDB}
			m := &submissionManager{t: t, key: key}
			manager := httptest.NewServer(m)
			defer manager.Close()
			switch mode {
			case "reservation failure":
				store.reserveFails = true
			case "manager rejection":
				m.reject = true
			case "rejection outcome failure":
				m.reject = true
				store.outcomeFails.Store(true)
			case "dial failure":
				manager.Close()
			case "lost response":
				m.dropResponse = true
				m.getFails.Store(true)
			case "outcome write failure":
				store.outcomeFails.Store(true)
			case "missing uncertain job":
				m.dropResponse = true
				m.getFails.Store(true)
			}
			p := &submissionProvider{}
			server := submissionGateway(t, store, p, manager.URL)
			status, first := submit(t, server, site, token, key, "change title")
			if mode == "reservation failure" {
				if status != 500 || m.posts.Load() != 0 {
					t.Fatalf("unsafe dispatch=%d calls=%d", status, m.posts.Load())
				}
				assertSubmissionRows(t, site, 0, "")
				return
			}
			if mode == "manager rejection" {
				if status != 409 || first["submission_state"] != "rejected" {
					t.Fatalf("rejection=%d %#v", status, first)
				}
				status, replay := submit(t, server, site, token, key, "change title")
				if status != 409 || replay["job_id"] != first["job_id"] || m.posts.Load() != 1 {
					t.Fatalf("rejection replay=%d %#v", status, replay)
				}
				assertSubmissionRows(t, site, 1, "failed")
				return
			}
			if mode == "dial failure" {
				if status != 503 || first["submission_state"] != "rejected" || m.posts.Load() != 0 {
					t.Fatalf("dial failure=%d %#v", status, first)
				}
				status, replay := submit(t, server, site, token, key, "change title")
				if status != 503 || replay["job_id"] != first["job_id"] {
					t.Fatalf("dial replay=%d %#v", status, replay)
				}
				assertSubmissionRows(t, site, 1, "failed")
				return
			}
			if status != 503 || first["submission_state"] != "dispatching" {
				t.Fatalf("uncertain=%d %#v", status, first)
			}
			store.outcomeFails.Store(false)
			m.getFails.Store(false)
			if mode == "missing uncertain job" {
				m.mu.Lock()
				m.job = nil
				m.mu.Unlock()
			}
			restarted := submissionGateway(t, store, &submissionProvider{}, manager.URL)
			status, replay := submit(t, restarted, site, token, key, "change title")
			if replay["job_id"] != first["job_id"] || m.posts.Load() != 1 {
				t.Fatalf("duplicate execution: %d %#v posts=%d", status, replay, m.posts.Load())
			}
			if mode == "rejection outcome failure" {
				if status != 409 || replay["submission_state"] != "rejected" {
					t.Fatalf("rejection reconciliation=%d %#v", status, replay)
				}
				assertSubmissionRows(t, site, 1, "failed")
				return
			}
			if mode == "missing uncertain job" {
				if status != 503 {
					t.Fatalf("must not redispatch missing uncertain job: %d", status)
				}
				assertSubmissionRows(t, site, 1, "pending")
			} else {
				if status != 200 {
					t.Fatalf("reconciliation=%d %#v", status, replay)
				}
				assertSubmissionRows(t, site, 1, "running")
			}
		})
	}
}

func TestBuildSubmissionHeaderAndCORS(t *testing.T) {
	site, token := submissionSite(t)
	p := &submissionProvider{}
	server := submissionGateway(t, testDB, p, "http://unused.invalid")
	for _, key := range []string{"", "not-a-uuid", uuid.Nil.String()} {
		status, _ := submit(t, server, site, token, key, "change title")
		if status != 400 {
			t.Errorf("invalid key %q status=%d", key, status)
		}
	}
	if p.calls.Load() != 0 {
		t.Fatal("invalid key reached provider")
	}
	assertSubmissionRows(t, site, 0, "")
	req, _ := http.NewRequest("OPTIONS", server.URL+"/sites/"+site.FQDN+"/build", nil)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Access-Control-Allow-Headers"), "Idempotency-Key") {
		t.Fatal("browser retry header not allowed")
	}
}

func TestBuildSubmissionConcurrentDuplicateRequests(t *testing.T) {
	site, token := submissionSite(t)
	key := uuid.NewString()
	m := &submissionManager{t: t, key: key, delay: 100 * time.Millisecond}
	manager := httptest.NewServer(m)
	defer manager.Close()
	server := submissionGateway(t, testDB, &submissionProvider{}, manager.URL)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, _ := submit(t, server, site, token, key, "change title")
			if status != 200 && status != 503 {
				t.Errorf("concurrent response=%d", status)
			}
		}()
	}
	wg.Wait()
	status, _ := submit(t, server, site, token, key, "change title")
	if status != 200 || m.posts.Load() != 1 {
		t.Fatalf("duplicate dispatch status=%d posts=%d", status, m.posts.Load())
	}
	assertSubmissionRows(t, site, 1, "running")
}
