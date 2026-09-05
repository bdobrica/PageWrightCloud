package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage/nfs"
	"github.com/stretchr/testify/require"
)

func TestMetadataCommitAndRestart(t *testing.T) {
	root := t.TempDir()
	backend, err := nfs.NewNFSBackend(root)
	require.NoError(t, err)
	server := httptest.NewServer(NewHandler(backend).SetupRoutes())
	defer server.Close()
	request := func(method, path, media string, data []byte, status int) []byte {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+path, bytes.NewReader(data))
		require.NoError(t, err)
		req.Header.Set("Content-Type", media)
		resp, err := server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, status, resp.StatusCode, string(body))
		if strings.HasSuffix(path, "/manifest") || strings.HasSuffix(path, "/logs") {
			require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
		}
		return body
	}
	base := "/sites/site/artifacts/version"
	manifest := []byte(`{"site_id":"site","build_id":"version","created_at":"2026-09-05T12:00:00Z","prompt":"private prompt","checks_passed":false}`)
	log := []byte(`{"content":"private stdout\nUTF-8 café\u0000"}`)
	assertHidden := func() {
		body := request("GET", "/sites/site/versions", "", nil, 200)
		require.NotContains(t, string(body), `"build_id"`)
		request("GET", base+"/manifest", "", nil, 404)
	}
	request("POST", base+"/manifest", "application/json", manifest, 409)
	assertHidden()
	request("PUT", base, "application/gzip", []byte("opaque archive"), 201)
	request("POST", base+"/manifest", "application/json", manifest, 409)
	assertHidden()
	// A private-log write failure must not allow the final manifest.
	logPath := filepath.Join(root, "sites", "site", "metadata", "version", "execution.json")
	require.NoError(t, os.MkdirAll(logPath, 0700))
	request("POST", base+"/logs", "application/json", log, 500)
	request("POST", base+"/manifest", "application/json", manifest, 409)
	assertHidden()
	require.NoError(t, os.Remove(logPath))
	request("POST", base+"/logs", "application/json", log, 201)
	require.Equal(t, log, request("GET", base+"/logs", "", nil, 200))
	assertHidden()
	// A failed final rename leaves no committed version; retry is safe.
	manifestPath := filepath.Join(root, "sites", "site", "metadata", "version", "manifest.json")
	require.NoError(t, os.Mkdir(manifestPath, 0700))
	request("POST", base+"/manifest", "application/json", manifest, 500)
	versions, err := backend.ListVersions("site")
	require.NoError(t, err)
	require.Empty(t, versions)
	require.NoError(t, os.Remove(manifestPath))
	request("POST", base+"/manifest", "application/json", manifest, 201)
	request("POST", base+"/manifest", "application/json", manifest, 201)
	require.Equal(t, manifest, request("GET", base+"/manifest", "", nil, 200))
	listing := request("GET", "/sites/site/versions", "", nil, 200)
	var result struct {
		Versions []json.RawMessage `json:"versions"`
	}
	require.NoError(t, json.Unmarshal(listing, &result))
	require.Len(t, result.Versions, 1)
	require.NotContains(t, string(listing), "private")
	require.Contains(t, string(listing), `"status":"completed"`)
	restarted, err := nfs.NewNFSBackend(root)
	require.NoError(t, err)
	saved, err := restarted.FetchManifest("site", "version")
	require.NoError(t, err)
	require.Equal(t, manifest, []byte(saved))
	savedLog, err := restarted.FetchPrivateLog("site", "version")
	require.NoError(t, err)
	require.Equal(t, log, savedLog)
	info, err := os.Stat(filepath.Join(root, "sites", "site", "metadata", "version", "execution.json"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	// Missing prerequisites fail closed even if a manifest remains on disk.
	require.NoError(t, os.Remove(filepath.Join(root, "sites", "site", "metadata", "version", "execution.json")))
	assertHidden()
}

func TestMetadataRejectsInvalidRequests(t *testing.T) {
	backend, err := nfs.NewNFSBackend(t.TempDir())
	require.NoError(t, err)
	router := NewHandler(backend).SetupRoutes()
	for _, tc := range []struct {
		endpoint, body, media string
		status                int
	}{
		{"logs", `{}`, "application/json", 400},
		{"logs", `{"content":null}`, "application/json", 400},
		{"logs", `{"content":"","unexpected":true}`, "application/json", 400},
		{"logs", `{"content":""} {}`, "application/json", 400},
		{"logs", `{"content":""}`, "text/plain", 415},
		{"logs", `{"content":"` + strings.Repeat("x", metadataLimit) + `"}`, "application/json", 413},
		{"manifest", `null`, "application/json", 400},
		{"manifest", `{"site_id":"other","build_id":"version","created_at":"2026-09-05T12:00:00Z"}`, "application/json", 400},
		{"manifest", `{"site_id":"site","build_id":"version"}`, "application/json", 400},
	} {
		t.Run(fmt.Sprintf("%s-%d-%d", tc.endpoint, tc.status, len(tc.body)), func(t *testing.T) {
			req := httptest.NewRequest("POST", "/sites/site/artifacts/version/"+tc.endpoint, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.media)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code)
		})
	}
	_, err = backend.FetchPrivateLog("site", "version")
	require.ErrorIs(t, err, os.ErrNotExist)
	for _, endpoint := range []string{"logs", "manifest"} {
		req := httptest.NewRequest("POST", "/sites/site/artifacts/version/"+endpoint, strings.NewReader(`{"content":""}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Add("Content-Encoding", "identity")
		req.Header.Add("Content-Encoding", "gzip")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, 415, w.Code)
	}
	req := httptest.NewRequest("POST", "/sites/site/artifacts/version/logs", strings.NewReader(`{"content":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, 201, w.Code, "empty execution output is valid")
	log, err := backend.FetchPrivateLog("site", "version")
	require.NoError(t, err)
	require.JSONEq(t, `{"content":""}`, string(log))
}
