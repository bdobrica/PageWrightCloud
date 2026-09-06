//go:build integration

package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

func TestCompiledArtifactRoundTrip(t *testing.T) {
	for _, key := range []string{"TEST_MANAGER_URL", "TEST_STORAGE_URL", "TEST_SERVING_URL", "TEST_HOSTING_URL"} {
		if os.Getenv(key) == "" {
			t.Fatalf("%s required; run make test-integration", key)
		}
	}
	const prompt = "Set the homepage heading to Deterministic M1 round trip."
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
			http.Error(w, "invalid request", 400)
			return
		}
		content := prompt
		if strings.Contains(request.Messages[0].Content, "Append a second unpublished edit.") {
			content = "Append a second unpublished edit."
		}
		if strings.Contains(request.Messages[0].Content, "Determine if this request") {
			content = "CLEAR: Update homepage heading"
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": content}}}})
	}))
	defer provider.Close()
	storage := clients.NewStorageClient(os.Getenv("TEST_STORAGE_URL"))
	manager := clients.NewManagerClient(os.Getenv("TEST_MANAGER_URL"))
	serving := clients.NewServingClient(os.Getenv("TEST_SERVING_URL"))
	sites := handlers.NewSitesHandler(testDB, serving, storage, 25)
	builds := handlers.NewBuildHandler(testDB, clients.NewLLMClient("test-only", provider.URL), manager, storage)
	versions := handlers.NewVersionsHandler(testDB, storage, serving, 25)
	router := mux.NewRouter()
	auth := handlers.NewAuthHandler(testDB, testJWTManager, nil)
	router.HandleFunc("/auth/register", auth.Register).Methods("POST")
	protected := router.PathPrefix("/sites").Subrouter()
	protected.Use(middleware.AuthMiddleware(testJWTManager))
	protected.HandleFunc("", sites.CreateSite).Methods("POST")
	protected.HandleFunc("/{fqdn}/build", builds.Build).Methods("POST")
	protected.HandleFunc("/{fqdn}/versions", versions.ListVersions).Methods("GET")
	protected.HandleFunc("/{fqdn}/versions/{version_id}/download", versions.DownloadVersion).Methods("GET")
	protected.HandleFunc("/{fqdn}/versions/{version_id}/deploy", versions.DeployVersion).Methods("POST")
	gateway := httptest.NewServer(router)
	defer gateway.Close()
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	token := ""
	request := func(method, url, body string, status int) []byte {
		t.Helper()
		req, err := http.NewRequest(method, url, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if method == http.MethodPut {
			req.Header.Set("Content-Type", "application/gzip")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", uuid.NewString())
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != status {
			t.Fatalf("%s %s: status %d, want %d: %s", method, url, resp.StatusCode, status, data)
		}
		return data
	}
	decode := func(data []byte, value any) {
		t.Helper()
		if err := json.Unmarshal(data, value); err != nil {
			t.Fatal(err)
		}
	}
	var registered types.AuthResponse
	decode(request("POST", gateway.URL+"/auth/register", `{"email":"`+uuid.NewString()+`@roundtrip.test","password":"integration-only-password"}`, 200), &registered)
	token = registered.Token
	fqdn := "compiled-" + uuid.NewString() + ".example.test"
	var site types.Site
	decode(request("POST", gateway.URL+"/sites", `{"fqdn":"`+fqdn+`","template_id":"starter"}`, 201), &site)
	if site.ID == "" || site.UserID != registered.User.ID {
		t.Fatalf("wrong created site: %+v", site)
	}
	base := gateway.URL + "/sites/" + fqdn
	initial := request("GET", base+"/versions/initial/download", "", 200)
	// Source-only bootstrap must never be published as HTML.
	request("POST", base+"/versions/initial/deploy", `{"target":"live"}`, 500)
	var accepted types.BuildResponse
	decode(request("POST", base+"/build", `{"message":"`+prompt+`"}`, 200), &accepted)
	if accepted.JobAccepted == nil {
		t.Fatalf("not accepted: %+v", accepted)
	}
	job, err := manager.GetJobStatus(accepted.JobID)
	if err != nil {
		t.Fatal(err)
	}
	job = waitForDispatch(t, manager, accepted.JobID)
	if job.SiteID != site.ID || job.OwnerID != registered.User.ID || job.SourceVersion != "initial" || job.TargetVersion != accepted.TargetVersion || job.Prompt != prompt {
		t.Fatalf("wrong launch snapshot: %+v", job)
	}
	// Preserve internal lease fields omitted by the gateway's public Job type.
	payload := request("GET", os.Getenv("TEST_MANAGER_URL")+"/jobs/"+job.JobID, "", 200)
	// This contract suite uses the integration-only manual launch bridge.
	// Real Docker dispatch is tested separately by make test-docker-spawner.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	worker := exec.CommandContext(ctx, "/usr/local/bin/worker-contract.test", "-test.run=^TestDeterministicWorkerEntrypoint$", "-test.v")
	worker.Env = append(os.Environ(), "TEST_DETERMINISTIC_JOB="+string(payload))
	if output, err := worker.CombinedOutput(); err != nil {
		t.Fatalf("worker: %v\n%s", err, output)
	} else {
		t.Logf("worker:\n%s", output)
	}
	completed, err := manager.GetJobStatus(job.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != types.JobStatusCompleted || completed.TargetVersion != job.TargetVersion || completed.ManifestPath == "" {
		t.Fatalf("wrong completion: %+v", completed)
	}
	archive := request("GET", base+"/versions/"+job.TargetVersion+"/download", "", 200)
	if bytes.Equal(initial, archive) {
		t.Fatal("build did not change archive")
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.FileInfo().IsDir() {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		files[header.Name] = data
	}
	if _, err := io.Copy(io.Discard, gz); err != nil {
		t.Fatal(err)
	}
	html := files["public/index.html"]
	if !bytes.Contains(html, []byte("Deterministic M1 round trip")) || !bytes.Contains(html, []byte("<h1")) || !bytes.Contains(files["content/home/index.md"], []byte("# Deterministic M1 round trip")) || len(files["content/site.json"]) == 0 {
		t.Fatalf("missing compiled HTML or preserved source: %v", files)
	}
	var layout struct {
		Kind string `json:"kind"`
	}
	decode(files["manifest.json"], &layout)
	if layout.Kind != "compiled" {
		t.Fatalf("layout: %+v", layout)
	}
	listing := request("GET", base+"/versions", "", 200)
	if bytes.Contains(listing, []byte(job.TargetVersion)) {
		t.Fatal("unreconciled version advertised as completed")
	}
	if err := testDB.ObserveBuildJob(context.Background(), completed); err != nil {
		t.Fatal(err)
	}
	listing = request("GET", base+"/versions", "", 200)
	if !bytes.Contains(listing, []byte(job.TargetVersion)) {
		t.Fatalf("compiled version absent: %s", listing)
	}
	// Conflicting overwrite must fail and preserve both source and output.
	request("PUT", os.Getenv("TEST_STORAGE_URL")+"/sites/"+site.ID+"/artifacts/"+job.TargetVersion, string(initial), 409)
	if !bytes.Equal(archive, request("GET", base+"/versions/"+job.TargetVersion+"/download", "", 200)) || !bytes.Equal(initial, request("GET", base+"/versions/initial/download", "", 200)) {
		t.Fatal("immutable bytes changed")
	}
	// A second build without publishing inherits the confirmed first draft.
	var second types.BuildResponse
	decode(request("POST", base+"/build", `{"message":"Append a second unpublished edit."}`, 200), &second)
	if second.JobAccepted == nil || second.SourceVersion != job.TargetVersion {
		t.Fatalf("second edit lost first draft base: %+v", second)
	}
	secondJob := waitForDispatch(t, manager, second.JobID)
	secondPayload := request("GET", os.Getenv("TEST_MANAGER_URL")+"/jobs/"+secondJob.JobID, "", 200)
	secondWorker := exec.CommandContext(ctx, "/usr/local/bin/worker-contract.test", "-test.run=^TestDeterministicWorkerEntrypoint$", "-test.v")
	secondWorker.Env = append(os.Environ(), "TEST_DETERMINISTIC_JOB="+string(secondPayload))
	if output, err := secondWorker.CombinedOutput(); err != nil {
		t.Fatalf("second worker: %v\n%s", err, output)
	}
	secondArchive := request("GET", base+"/versions/"+second.TargetVersion+"/download", "", 200)
	secondGzip, err := gzip.NewReader(bytes.NewReader(secondArchive))
	if err != nil {
		t.Fatal(err)
	}
	defer secondGzip.Close()
	secondTar := tar.NewReader(secondGzip)
	found := 0
	for {
		header, err := secondTar.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name != "content/home/index.md" && header.Name != "public/index.html" {
			continue
		}
		data, err := io.ReadAll(secondTar)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(data, []byte("Deterministic M1 round trip")) || !bytes.Contains(data, []byte("Second unpublished edit preserved.")) {
			t.Fatalf("edits did not accumulate in %s", header.Name)
		}
		found++
	}
	if found != 2 {
		t.Fatal("second archive missing source/output")
	}
	unchanged, err := testDB.GetSiteByFQDN(fqdn)
	if err != nil || unchanged.LiveVersionID != nil {
		t.Fatalf("unpublished edits changed live: %+v, %v", unchanged, err)
	}
	if !bytes.Equal(archive, request("GET", base+"/versions/"+job.TargetVersion+"/download", "", 200)) {
		t.Fatal("second edit mutated first draft")
	}
	request("POST", base+"/versions/"+job.TargetVersion+"/deploy", `{"target":"live"}`, 200)
	hosted := func(path string) (int, []byte) {
		t.Helper()
		req, err := http.NewRequest("GET", os.Getenv("TEST_HOSTING_URL")+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = fqdn
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, data
	}
	// nginx reload acknowledges the signal before new workers accept requests.
	deadline := time.Now().Add(5 * time.Second)
	for {
		status, data := hosted("/index.html")
		if status == 200 && bytes.Equal(data, html) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("hosted HTML mismatch: %d %s", status, data)
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, path := range []string{"/content/site.json", "/content/home/index.md", "/manifest.json", "/.codex/instructions.md", "/execution.log", "/.env", "/.archive-sha256"} {
		if status, _ := hosted(path); status != 404 {
			t.Errorf("private path %s returned %d", path, status)
		}
	}
	for name, expected := range files {
		if strings.HasPrefix(name, "public/") && !strings.HasSuffix(name, "/") {
			if status, data := hosted("/" + strings.TrimPrefix(name, "public/")); status != 200 || !bytes.Equal(data, expected) {
				t.Errorf("hosted file differs from archive: %s (status %d)", name, status)
			}
		}
	}
	// An invalid subsequent deployment cannot replace the already hosted output.
	request("POST", base+"/versions/initial/deploy", `{"target":"live"}`, 500)
	if status, data := hosted("/index.html"); status != 200 || !bytes.Equal(data, html) {
		t.Fatal("failed deployment changed live HTML")
	}
}
