package serviceauth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServiceAndWorkerBoundary(t *testing.T) {
	key := strings.Repeat("k", 32)
	t.Setenv("PAGEWRIGHT_SERVICE_TOKEN", key)
	s := Scope{Job: "job-a", Site: "site-a", Source: "initial", Target: "v2", Lock: "attempt", Fence: 1, Expires: time.Now().Add(time.Minute).Unix()}
	token := Sign(key, s)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	for _, tc := range []struct {
		service, method, path, token string
		status                       int
	}{
		{"manager", "POST", "/jobs", "", 401}, {"storage", "PUT", "/sites/site-a/artifacts/initial", "", 401}, {"serving", "POST", "/sites/site-a/deployment", "", 401},
		{"manager", "GET", "/health", "", 204}, {"manager", "POST", "/jobs", key, 204},
		{"manager", "POST", "/jobs/job-a/result", token, 204}, {"manager", "POST", "/jobs/job-b/result", token, 403},
		{"manager", "GET", "/jobs/job-a", token, 204}, {"manager", "GET", "/jobs/job-b", token, 403},
		{"manager", "POST", "/jobs/job-a/write-commit", token, 403}, {"manager", "POST", "/jobs", token, 403},
		{"storage", "GET", "/sites/site-a/artifacts/initial", token, 204}, {"storage", "GET", "/sites/site-b/artifacts/initial", token, 403},
		{"storage", "GET", "/sites/site-a/artifacts/initial/logs", token, 403}, {"storage", "PUT", "/sites/site-a/artifacts/v2", token, 204},
		{"storage", "POST", "/sites/site-a/artifacts/v2/manifest", token, 204}, {"storage", "PUT", "/sites/site-a/artifacts/initial", token, 403},
		{"storage", "PUT", "/sites/site-a/artifacts/v3", token, 403}, {"serving", "POST", "/sites/site-a/deployment", token, 403},
		{"manager", "POST", "/jobs/job-a/result", token + "x", 401}, {"manager", "GET", "/jobs/job-a?extra=1", token, 403},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		Wrap(tc.service, next).ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s %s: %d want %d", tc.service, tc.method, tc.path, w.Code, tc.status)
		}
	}
	s.Expires = time.Now().Add(-time.Second).Unix()
	if _, ok := Verify(key, Sign(key, s)); ok {
		t.Fatal("expired token accepted")
	}
	if _, ok := Verify(strings.Repeat("x", 32), token); ok {
		t.Fatal("rotation failed")
	}
}
func TestCredentialDestinationAndValidation(t *testing.T) {
	t.Setenv("PAGEWRIGHT_SERVICE_TOKEN", "")
	if Validate() == nil {
		t.Fatal("missing key accepted")
	}
	key := strings.Repeat("k", 32)
	t.Setenv("PAGEWRIGHT_SERVICE_TOKEN", key)
	t.Setenv("PAGEWRIGHT_PROVIDER_TOKEN", key)
	if Validate() == nil {
		t.Fatal("worker-known key accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			t.Error("missing credential")
		}
		io.WriteString(w, "ok")
	}))
	defer server.Close()
	client := http.Client{Transport: Transport{Origin: server.URL, Token: key}}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if _, err := client.Get("http://unapproved.invalid"); err == nil {
		t.Fatal("credential destination not pinned")
	}
}
