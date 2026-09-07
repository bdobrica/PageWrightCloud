package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/serviceauth"
)

func TestInternalStorageRejectsBeforeBackend(t *testing.T) {
	key := strings.Repeat("k", 32)
	t.Setenv("PAGEWRIGHT_SERVICE_TOKEN", key)
	token := serviceauth.Sign(key, serviceauth.Scope{Job: "job", Site: "site", Source: "initial", Target: "v2", Lock: "lock", Fence: 1, Expires: time.Now().Add(time.Minute).Unix()})
	secured := serviceauth.Wrap("storage", NewHandler(nil).SetupRoutes())
	for _, tc := range []struct {
		method, path, token string
		status              int
	}{
		{"PUT", "/sites/site/artifacts/initial", "", 401}, {"GET", "/sites/site/artifacts/v2/logs", "", 401},
		{"PUT", "/sites/other/artifacts/v2", token, 403}, {"POST", "/sites/site/artifacts/v3/manifest", token, 403},
		{"POST", "/sites/site/logs", token, 403}, {"GET", "/sites/site/versions", token, 403},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader("private-body"))
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		secured.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("status %d expected %d", w.Code, tc.status)
		}
	}
}

func TestSignedStorageAttemptCannotSwapIdentity(t *testing.T) {
	key := strings.Repeat("k", 32)
	t.Setenv("PAGEWRIGHT_SERVICE_TOKEN", key)
	token := serviceauth.Sign(key, serviceauth.Scope{Job: "job", Site: "site", Source: "initial", Target: "v2", Lock: "lock", Fence: 1, Expires: time.Now().Add(time.Minute).Unix()})
	h := NewHandler(nil)
	h.commitURL = "http://authority.test"
	secured := serviceauth.Wrap("storage", h.SetupRoutes())
	for _, attempt := range []writeCommit{
		{JobID: "other-job", SiteID: "site", TargetVersion: "v2", LockToken: "lock", FencingToken: 1},
		{JobID: "job", SiteID: "site", TargetVersion: "v2", LockToken: "other-attempt", FencingToken: 1},
		{JobID: "job", SiteID: "site", TargetVersion: "v2", LockToken: "lock", FencingToken: 2},
	} {
		data, _ := json.Marshal(attempt)
		r := httptest.NewRequest("PUT", "/sites/site/artifacts/v2", strings.NewReader("private-body"))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Pagewright-Attempt", string(data))
		w := httptest.NewRecorder()
		secured.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("swapped attempt reached nil backend: %d", w.Code)
		}
	}
}
