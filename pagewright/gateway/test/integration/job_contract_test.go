//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// This exercises the real gateway Build handler, PostgreSQL and manager HTTP
// API. Only the paid instruction provider is faked; no worker/AI is executed.
func TestGatewayManagerJobContract(t *testing.T) {
	managerURL := os.Getenv("TEST_MANAGER_URL")
	if managerURL == "" {
		t.Fatal("TEST_MANAGER_URL required; use make test-integration")
	}
	user, err := testDB.CreateUser("job-"+uuid.NewString()+"@example.test", "test-only-hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	site, err := testDB.CreateSite(user.ID, "job-"+uuid.NewString()+".example.test", "starter")
	if err != nil {
		t.Fatal(err)
	}
	token, err := testJWTManager.GenerateToken(user.ID, user.Email)
	if err != nil {
		t.Fatal(err)
	}

	var providerCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
			t.Error("invalid provider request")
			http.Error(w, "invalid", 400)
			return
		}
		content := "Change the homepage title to Contract Test."
		if strings.Contains(request.Messages[0].Content, "Determine if this request") {
			content = "CLEAR: Update homepage title"
			if strings.Contains(request.Messages[0].Content, "User request: Make it better") {
				content = "UNCLEAR: Which title?"
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": map[string]string{"role": "assistant", "content": content}}}})
	}))
	defer provider.Close()
	manager := clients.NewManagerClient(managerURL)
	build := handlers.NewBuildHandler(testDB, clients.NewLLMClient("test-only", provider.URL), manager, emptyCompletedVersions{})
	router := mux.NewRouter()
	router.Handle("/sites/{fqdn}/build", middleware.AuthMiddleware(testJWTManager)(http.HandlerFunc(build.Build))).Methods("POST")
	server := httptest.NewServer(router)
	defer server.Close()
	post := func(body, bearer string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, server.URL+"/sites/"+site.FQDN+"/build", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", uuid.NewString())
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// The browser cannot supply a different owner, execution ID or version base.
	for _, body := range []string{`{"message":"edit","owner_id":"other"}`, `{"prompt":"edit"}`, `{"message":"edit","source_version":"other"}`, `{"message":"edit"} {}`, `{"message":" "}`} {
		resp := post(body, token)
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Errorf("invalid request %s: status %d", body, resp.StatusCode)
		}
	}
	resp := post(`{"message":"edit"}`, "")
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("unauthenticated status %d", resp.StatusCode)
	}
	otherToken, err := testJWTManager.GenerateToken(uuid.NewString(), "other@example.test")
	if err != nil {
		t.Fatal(err)
	}
	resp = post(`{"message":"edit"}`, otherToken)
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("other owner status %d", resp.StatusCode)
	}
	if providerCalls.Load() != 0 {
		t.Fatal("invalid/unauthorized request reached provider")
	}

	resp = post(`{"message":"Change the homepage title"}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("build response status %d", resp.StatusCode)
	}
	var accepted types.BuildResponse
	acceptedDecoder := json.NewDecoder(resp.Body)
	acceptedDecoder.DisallowUnknownFields() // No prompt or internal lease fields in the public response.
	if err := acceptedDecoder.Decode(&accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.JobAccepted == nil || accepted.JobID == "" || accepted.TargetVersion == "" || accepted.JobID == accepted.TargetVersion || accepted.SiteID != site.ID || accepted.OwnerID != user.ID || accepted.SourceVersion != "initial" || accepted.Status != types.JobStatusPending || accepted.Question != nil {
		t.Fatalf("invalid accepted response: %#v", accepted)
	}
	job, err := manager.GetJobStatus(accepted.JobID)
	if err != nil {
		t.Fatal(err)
	}
	job = waitForDispatch(t, manager, accepted.JobID)
	if job.JobID != accepted.JobID || job.SiteID != accepted.SiteID || job.OwnerID != accepted.OwnerID || job.SourceVersion != accepted.SourceVersion || job.TargetVersion != accepted.TargetVersion || job.Prompt != "Change the homepage title to Contract Test." {
		t.Fatalf("manager lost contract: %#v", job)
	}

	// A live manager lock conflict is returned as HTTP 409 through the gateway.
	resp = post(`{"message":"Change the title again"}`, token)
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatalf("busy build status %d", resp.StatusCode)
	}

	// Canonical result round-trips without confusing job_id and target_version.
	// Fetch private attempt fields directly: the gateway's public job DTO must
	// continue to omit them.
	private, err := http.Get(managerURL + "/jobs/" + job.JobID)
	if err != nil {
		t.Fatal(err)
	}
	var attempt struct {
		LockToken    string `json:"lock_token"`
		FencingToken int64  `json:"fencing_token"`
	}
	err = json.NewDecoder(private.Body).Decode(&attempt)
	private.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"lock_token": attempt.LockToken, "fencing_token": attempt.FencingToken, "job_id": job.JobID, "site_id": job.SiteID, "owner_id": job.OwnerID, "source_version": job.SourceVersion, "target_version": job.TargetVersion, "status": "failed", "error_message": "deterministic test failure"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.Post(managerURL+"/jobs/"+job.JobID+"/result", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("callback status %d", resp.StatusCode)
	}
	job, err = manager.GetJobStatus(job.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != types.JobStatusFailed || job.ErrorMessage != "deterministic test failure" || job.TargetVersion != accepted.TargetVersion {
		t.Fatalf("result lost: %#v", job)
	}

	// The existing clarification flow retains its distinct public response shape.
	resp = post(`{"message":"Make it better"}`, token)
	var clarification types.BuildResponse
	clarificationDecoder := json.NewDecoder(resp.Body)
	clarificationDecoder.DisallowUnknownFields()
	err = clarificationDecoder.Decode(&clarification)
	resp.Body.Close()
	if err != nil || resp.StatusCode != 200 || clarification.JobAccepted != nil || clarification.Question == nil || *clarification.Question != "Which title?" || clarification.ConversationID == nil {
		t.Fatalf("invalid clarification response: %#v, %v", clarification, err)
	}
	followUp, err := json.Marshal(types.BuildRequest{Message: "The homepage title", ConversationID: clarification.ConversationID})
	if err != nil {
		t.Fatal(err)
	}
	resp = post(string(followUp), token)
	var clarified types.BuildResponse
	err = json.NewDecoder(resp.Body).Decode(&clarified)
	resp.Body.Close()
	if err != nil || resp.StatusCode != 200 || clarified.JobAccepted == nil || clarified.OwnerID != user.ID || clarified.SiteID != site.ID || clarified.JobID == job.JobID || clarified.Question != nil {
		t.Fatalf("invalid clarified acceptance: %#v, %v", clarified, err)
	}
}
