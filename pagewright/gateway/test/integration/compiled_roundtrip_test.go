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
	"net/url"
	"os"
	"os/exec"
	"regexp"
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
	if err := sites.SetSiteDomain("example.test"); err != nil {
		t.Fatal(err)
	}
	builds := handlers.NewBuildHandler(testDB, clients.NewLLMClient("test-only", provider.URL), manager, storage)
	versions := handlers.NewVersionsHandler(testDB, storage, serving, 25)
	router := mux.NewRouter()
	auth := handlers.NewAuthHandler(testDB, testJWTManager, nil)
	router.HandleFunc("/auth/register", auth.Register).Methods("POST")
	protected := router.PathPrefix("/sites").Subrouter()
	protected.Use(middleware.AuthMiddleware(testJWTManager))
	protected.HandleFunc("", sites.CreateSite).Methods("POST")
	protected.HandleFunc("", sites.ListSites).Methods("GET")
	protected.HandleFunc("/{fqdn}", sites.GetSite).Methods("GET")
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
	previewFQDN := strings.Replace(fqdn, ".", ".preview.", 1)
	var site types.Site
	decode(request("POST", gateway.URL+"/sites", `{"fqdn":"`+fqdn+`","template_id":"starter"}`, 201), &site)
	if site.ID == "" || site.UserID != registered.User.ID {
		t.Fatalf("wrong created site: %+v", site)
	}
	base := gateway.URL + "/sites/" + fqdn
	var siteWire map[string]any
	decode(request("GET", base, "", 200), &siteWire)
	if siteWire["live_url"] != "http://"+fqdn+":8084/" || siteWire["preview_url"] != "http://"+previewFQDN+":8084/" {
		t.Fatalf("wrong site hosting URLs: %+v", siteWire)
	}
	var page struct {
		Data []map[string]any `json:"data"`
	}
	decode(request("GET", gateway.URL+"/sites", "", 200), &page)
	if len(page.Data) != 1 || page.Data[0]["preview_url"] != siteWire["preview_url"] {
		t.Fatalf("wrong listing hosting URLs: %+v", page)
	}
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
	secondFiles := map[string][]byte{}
	found := 0
	for {
		header, err := secondTar.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(secondTar)
		if err != nil {
			t.Fatal(err)
		}
		if !header.FileInfo().IsDir() && strings.HasPrefix(header.Name, "public/") {
			secondFiles[header.Name] = data
		}
		if header.Name != "content/home/index.md" && header.Name != "public/index.html" {
			continue
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
	hosted := func(path string, previewHost ...bool) (int, []byte) {
		t.Helper()
		req, err := http.NewRequest("GET", os.Getenv("TEST_HOSTING_URL")+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = fqdn + ":8084"
		if len(previewHost) > 0 && previewHost[0] {
			req.Host = previewFQDN + ":8084"
		}
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
	// Old nginx workers may briefly drain after new-generation acknowledgment.
	// Bound convergence for expected public resources; missing/private paths still
	// assert their first response directly and never accept a successful fallback.
	hostedResource := func(path string, previewHost bool) (int, []byte) {
		deadline := time.Now().Add(5 * time.Second)
		for {
			status, data := hosted(path, previewHost)
			if status == 200 || time.Now().After(deadline) {
				return status, data
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	checkNavigation := func(previewHost bool) {
		t.Helper()
		for _, path := range []string{"/", "/guide/nested/"} {
			status, data := hostedResource(path, previewHost)
			if status != 200 {
				t.Fatalf("nested route %s preview=%v: %d", path, previewHost, status)
			}
			origin, _ := url.Parse("http://" + fqdn + ":8084" + path)
			if previewHost {
				origin.Host = previewFQDN + ":8084"
			}
			for _, match := range regexp.MustCompile(`(?:href|src)="([^"]+)"`).FindAllSubmatch(data, -1) {
				ref, err := url.Parse(string(match[1]))
				if err != nil {
					t.Fatal(err)
				}
				resolved := origin.ResolveReference(ref)
				if resolved.Host != origin.Host {
					t.Fatalf("cross-host generated reference %s", resolved)
				}
				path := resolved.Path
				// Compiler navigation uses slashless page URLs. Assert redirect
				// separately below, then compare the final response to its artifact.
				name := "public/" + strings.TrimPrefix(path, "/")
				if strings.HasSuffix(path, "/") {
					name += "index.html"
				} else if _, ok := files[name+"/index.html"]; ok {
					path += "/"
					name += "/index.html"
				}
				if status, body := hostedResource(path, previewHost); status != 200 || !bytes.Equal(body, files[name]) {
					t.Fatalf("linked resource %s preview=%v differs: %d", path, previewHost, status)
				}
			}
		}
		req, _ := http.NewRequest("GET", os.Getenv("TEST_HOSTING_URL")+"/guide/nested", nil)
		req.Host = fqdn + ":8084"
		if previewHost {
			req.Host = previewFQDN + ":8084"
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 301 || resp.Header.Get("Location") != "/guide/nested/" {
			t.Fatalf("redirect loses external origin: %d %s", resp.StatusCode, resp.Header.Get("Location"))
		}
		for _, path := range []string{"/missing", "/preview/build-id", "/content/site.json", "/manifest.json", "/.env", "/execution.log", "/.archive-sha256"} {
			if status, _ := hosted(path, previewHost); status != 404 {
				t.Fatalf("private/missing path %s preview=%v: %d", path, previewHost, status)
			}
		}
	}
	var preview struct {
		URL     string `json:"url"`
		Version string `json:"version_id"`
	}
	decode(request("POST", base+"/versions/"+job.TargetVersion+"/deploy", `{"target":"preview"}`, 200), &preview)
	if preview.URL != "http://"+previewFQDN+":8084/" || preview.Version != job.TargetVersion {
		t.Fatalf("preview response %+v", preview)
	}
	previewDeadline := time.Now().Add(5 * time.Second)
	for {
		status, data := hosted("/index.html", true)
		if status == 200 && bytes.Equal(data, html) {
			break
		}
		if time.Now().After(previewDeadline) {
			t.Fatalf("first preview before publish failed: %d %s", status, data)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if status, _ := hosted("/index.html"); status != 404 {
		t.Fatalf("preview unexpectedly published live: %d", status)
	}
	for name, expected := range files {
		if strings.HasPrefix(name, "public/") && !strings.HasSuffix(name, "/") {
			if status, data := hostedResource("/"+strings.TrimPrefix(name, "public/"), true); status != 200 || !bytes.Equal(data, expected) {
				t.Errorf("preview file differs from artifact: %s (status %d)", name, status)
			}
		}
	}
	previewSite, err := testDB.GetSiteByFQDN(fqdn)
	if err != nil || previewSite.LiveVersionID != nil || previewSite.PreviewVersionID == nil || *previewSite.PreviewVersionID != job.TargetVersion {
		t.Fatalf("first preview DB %+v %v", previewSite, err)
	}
	checkNavigation(true)
	request("POST", base+"/versions/"+job.TargetVersion+"/deploy", `{"target":"live"}`, 200)
	checkNavigation(false)
	request("POST", base+"/versions/"+second.TargetVersion+"/deploy", `{"target":"preview"}`, 200)
	previewSite, err = testDB.GetSiteByFQDN(fqdn)
	if err != nil || previewSite.LiveVersionID == nil || *previewSite.LiveVersionID != job.TargetVersion || previewSite.PreviewVersionID == nil || *previewSite.PreviewVersionID != second.TargetVersion {
		t.Fatalf("preview changed live DB %+v %v", previewSite, err)
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
			if status, data := hostedResource("/"+strings.TrimPrefix(name, "public/"), false); status != 200 || !bytes.Equal(data, expected) {
				t.Errorf("hosted file differs from archive: %s (status %d)", name, status)
			}
		}
	}
	// An invalid subsequent deployment cannot replace the already hosted output.
	request("POST", base+"/versions/initial/deploy", `{"target":"live"}`, 500)
	if status, data := hosted("/index.html"); status != 200 || !bytes.Equal(data, html) {
		t.Fatal("failed deployment changed live HTML")
	}
	// Promote the exact second preview artifact, without rebuilding or changing it.
	_, previewHTML := hosted("/index.html", true)
	if bytes.Equal(previewHTML, html) || !bytes.Contains(previewHTML, []byte("Second unpublished edit preserved.")) {
		t.Fatal("preview did not isolate the second draft")
	}
	request("POST", base+"/versions/"+second.TargetVersion+"/deploy", `{"target":"live"}`, 200)
	if status, data := hosted("/index.html"); status != 200 || !bytes.Equal(data, previewHTML) {
		t.Fatal("promotion changed preview bytes")
	}
	if !bytes.Equal(secondArchive, request("GET", base+"/versions/"+second.TargetVersion+"/download", "", 200)) {
		t.Fatal("promotion changed immutable archive")
	}
	for name, expected := range secondFiles {
		for _, previewHost := range []bool{false, true} {
			if status, data := hostedResource("/"+strings.TrimPrefix(name, "public/"), previewHost); status != 200 || !bytes.Equal(data, expected) {
				t.Fatalf("promoted resource %s preview=%v differs: %d", name, previewHost, status)
			}
		}
	}
	// Lose the actual serving acknowledgment after it switched live. A new
	// gateway recovery instance must reconcile the receipt, not invent a rollback.
	lostAck := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstream, err := http.NewRequest("POST", os.Getenv("TEST_SERVING_URL")+r.URL.Path, r.Body)
		if err != nil {
			t.Error(err)
			http.Error(w, "proxy", 502)
			return
		}
		upstream.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(upstream)
		if err != nil {
			t.Error(err)
			http.Error(w, "proxy", 502)
			return
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("real activation failed: %d", resp.StatusCode)
		}
		http.Error(w, "injected lost acknowledgment", 502)
	}))
	defer lostAck.Close()
	uncertain := handlers.NewVersionsHandler(testDB, storage, clients.NewServingClient(lostAck.URL), 25)
	faultRouter := mux.NewRouter()
	faultRouter.Handle("/sites/{fqdn}/versions/{version_id}/deploy", middleware.AuthMiddleware(testJWTManager)(http.HandlerFunc(uncertain.DeployVersion)))
	faultGateway := httptest.NewServer(faultRouter)
	defer faultGateway.Close()
	request("POST", faultGateway.URL+"/sites/"+fqdn+"/versions/"+job.TargetVersion+"/deploy", `{"target":"live"}`, 500)
	stale, err := testDB.GetSiteByFQDN(fqdn)
	if err != nil || stale.LiveVersionID == nil || *stale.LiveVersionID != second.TargetVersion {
		t.Fatalf("uncertain deployment wrote DB: %+v %v", stale, err)
	}
	if status, data := hosted("/index.html"); status != 200 || !bytes.Equal(data, html) {
		t.Fatal("lost ack fixture did not switch real hosting")
	}
	recoveryCtx, stopRecovery := context.WithCancel(context.Background())
	defer stopRecovery()
	recovered := make(chan struct{})
	go func() {
		defer close(recovered)
		handlers.NewVersionsHandler(testDB, storage, serving, 25).RunDeploymentRecovery(recoveryCtx)
	}()
	deadline = time.Now().Add(5 * time.Second)
	for {
		state, err := testDB.GetSiteByFQDN(fqdn)
		if err == nil && state.LiveVersionID != nil && *state.LiveVersionID == job.TargetVersion && state.PreviewVersionID != nil && *state.PreviewVersionID == second.TargetVersion {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("restart reconciliation did not restore DB/serving agreement")
		}
		time.Sleep(20 * time.Millisecond)
	}
	stopRecovery()
	<-recovered
}
