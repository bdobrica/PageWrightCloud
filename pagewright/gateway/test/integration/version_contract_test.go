//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/bootstrap"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

func TestVersionListHTTPContract(t *testing.T) {
	user, err := testDB.CreateUser("versions-"+uuid.NewString()+"@example.test", "test-hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	site, err := testDB.CreateSite(user.ID, "versions-"+uuid.NewString()+".example.test", "starter")
	if err != nil {
		t.Fatal(err)
	}
	token, err := testJWTManager.GenerateToken(user.ID, user.Email)
	if err != nil {
		t.Fatal(err)
	}
	other, err := testDB.CreateUser("other-"+uuid.NewString()+"@example.test", "test-hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	otherToken, err := testJWTManager.GenerateToken(other.ID, other.Email)
	if err != nil {
		t.Fatal(err)
	}
	storageURL := os.Getenv("TEST_STORAGE_URL")
	if storageURL == "" {
		t.Fatal("TEST_STORAGE_URL required")
	}
	archive, manifest, log, err := bootstrap.Generate(site.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := clients.NewStorageClient(storageURL).InitializeSource(site.ID, "initial", archive, log, manifest); err != nil {
		t.Fatal(err)
	}
	request := func(upstream, query, auth string) (int, map[string]interface{}) {
		t.Helper()
		h := handlers.NewVersionsHandler(testDB, clients.NewStorageClient(upstream), nil, 0)
		router := mux.NewRouter()
		router.Handle("/sites/{fqdn}/versions", middleware.AuthMiddleware(testJWTManager)(http.HandlerFunc(h.ListVersions)))
		req := httptest.NewRequest("GET", "/sites/"+site.FQDN+"/versions"+query, nil)
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return w.Code, body
	}
	code, body := request(storageURL, "", token)
	if code != 200 {
		t.Fatalf("%d %v", code, body)
	}
	rows, ok := body["data"].([]interface{})
	if !ok || len(rows) != 1 {
		t.Fatalf("invalid list: %v", body)
	}
	row := rows[0].(map[string]interface{})
	if len(row) != 5 || row["id"] != "initial" || row["build_id"] != "initial" || row["site_id"] != site.ID || row["status"] != "completed" {
		t.Fatalf("wrong version fields: %v", row)
	}
	if _, err := time.Parse(time.RFC3339Nano, row["created_at"].(string)); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"?page=2", "?page=9223372036854775807"} {
		code, body = request(storageURL, query, token)
		rows, ok = body["data"].([]interface{})
		if code != 200 || !ok || len(rows) != 0 {
			t.Fatalf("invalid empty page: %d %v", code, body)
		}
	}
	for _, auth := range []string{"", otherToken} {
		untouched := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("unauthorized storage call"); w.WriteHeader(500) }))
		code, _ = request(untouched.URL, "", auth)
		untouched.Close()
		if code != 401 && code != 403 {
			t.Fatalf("authorization status %d", code)
		}
	}
	for _, scenario := range []string{"empty", "invalid", "failure"} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if scenario == "failure" {
				w.WriteHeader(500)
				return
			}
			if scenario == "empty" {
				_, _ = w.Write([]byte(`{"versions":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"versions":[{"build_id":"bad","timestamp":"2026-09-06T00:00:00Z","status":"success"}]}`))
		}))
		code, body = request(upstream.URL, "?page_size=0", token)
		upstream.Close()
		if scenario == "empty" {
			if code != 200 || body["page_size"] != float64(25) || !strings.Contains(mustJSON(t, body), `"data":[]`) {
				t.Fatalf("%d %v", code, body)
			}
		} else if code < 500 {
			t.Fatalf("upstream error hidden: %d", code)
		}
	}
}
func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
