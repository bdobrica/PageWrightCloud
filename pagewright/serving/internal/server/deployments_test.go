package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/artifact"
	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/nginx"
	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestDurableFencedDeployment(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for _, file := range [][2]string{{"manifest.json", `{"schema_version":1,"kind":"compiled","theme_id":"starter"}`}, {"content/site.json", `{"site_name":"Test"}`}, {"content/home/index.md", "# Source"}, {"public/index.html", "<h1>Public</h1>"}} {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: file[0], Mode: 0644, Size: int64(len(file[1]))}))
		_, err := tw.Write([]byte(file[1]))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	var fail atomic.Bool
	var fetches atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		if fail.Load() {
			http.Error(w, "offline", 500)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Write(archive.Bytes())
	}))
	defer upstream.Close()
	am := artifact.NewManager(t.TempDir(), 0)
	configDir := t.TempDir()
	nm := nginx.NewManager(configDir, "true", "/tmp/503.html")
	sc := storage.NewClient(upstream.URL)
	h := NewHandler(am, nm, sc)
	// A legacy site without a receipt is still protected by its active pointer;
	// deletion must not remove nginx routing before discovering that guard.
	legacy := "legacy.example.test"
	legacyArchive := filepath.Join(t.TempDir(), "legacy.tar.gz")
	require.NoError(t, os.WriteFile(legacyArchive, archive.Bytes(), 0600))
	require.NoError(t, am.DeployArtifact(legacy, "v1", legacyArchive))
	require.NoError(t, am.ActivateVersion(legacy, "v1", false))
	require.NoError(t, nm.CreateSiteConfig(legacy, am.GetSitePath(legacy), nil, true))
	deletion := httptest.NewRecorder()
	h.SetupRoutes().ServeHTTP(deletion, httptest.NewRequest("DELETE", "/sites/"+legacy, nil))
	require.Equal(t, 500, deletion.Code)
	require.FileExists(t, filepath.Join(configDir, legacy))
	require.FileExists(t, filepath.Join(am.GetSitePath(legacy), "public", "index.html"))
	d := deploymentReceipt{SiteID: "site-id", FQDN: "site.example.test", Sequence: 1, Version: "v1", Target: "live", Status: "pending"}
	send := func(h *Handler, d deploymentReceipt) *httptest.ResponseRecorder {
		data, _ := json.Marshal(d)
		w := httptest.NewRecorder()
		h.SetupRoutes().ServeHTTP(w, httptest.NewRequest("POST", "/sites/"+d.FQDN+"/deployment", bytes.NewReader(data)))
		return w
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := send(h, d); w.Code != 200 {
				t.Errorf("retry: %d %s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), fetches.Load())
	link := filepath.Join(am.GetSitePath(d.FQDN), "public")
	target, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, "artifacts/v1/public", target)
	// Lost acknowledgement + serving restart: durable receipt deduplicates.
	h = NewHandler(am, nm, sc)
	require.Equal(t, 200, send(h, d).Code)
	require.Equal(t, int32(1), fetches.Load())
	newer := d
	newer.Sequence = 2
	newer.Target = "preview"
	newer.Version = "v2"
	require.Equal(t, 200, send(h, newer).Code)
	require.Equal(t, 409, send(h, d).Code)
	changed := newer
	changed.Version = "v3"
	require.Equal(t, 409, send(h, changed).Code)
	for _, path := range []string{"activate", "preview", "artifacts"} {
		w := httptest.NewRecorder()
		h.SetupRoutes().ServeHTTP(w, httptest.NewRequest("POST", "/sites/"+d.FQDN+"/"+path, bytes.NewBufferString(`{"version":"v1"}`)))
		require.Equal(t, 409, w.Code)
	}
	w := httptest.NewRecorder()
	h.SetupRoutes().ServeHTTP(w, httptest.NewRequest("DELETE", "/sites/"+d.FQDN, nil))
	require.Equal(t, 409, w.Code)
	// Model a crash after the symlink changed but before its completion receipt.
	newer.Status = "activating"
	require.NoError(t, saveReceipt(h.receiptPath(d.FQDN), &newer))
	newer.Status = "pending"
	fail.Store(true)
	require.Equal(t, 503, send(h, newer).Code)
	receipt, err := readReceipt(h.receiptPath(d.FQDN))
	require.NoError(t, err)
	require.Equal(t, "activating", receipt.Status)
	fail.Store(false)
	h = NewHandler(am, nm, sc)
	require.Equal(t, 200, send(h, newer).Code)
	// A genuine pre-activation failure is terminal and never mutates live.
	failed := d
	failed.Sequence = 3
	failed.Version = "v3"
	fail.Store(true)
	response := send(h, failed)
	require.Equal(t, 200, response.Code)
	require.Contains(t, response.Body.String(), `"status":"failed"`)
	fail.Store(false)
	require.Contains(t, send(h, failed).Body.String(), `"status":"failed"`)
	target, err = os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, "artifacts/v1/public", target)
	// Corrupt receipts cannot reset the sequence or overwrite serving state.
	// Evict an inactive cache entry, then roll back via a newer fenced operation.
	next := d
	next.Sequence = 4
	next.Version = "v3"
	require.Equal(t, 200, send(h, next).Code)
	stamp := time.Unix(100, 0)
	for _, v := range []string{"v1", "v2"} {
		require.NoError(t, os.Chtimes(am.GetArtifactPath(d.FQDN, v), stamp, stamp))
	}
	require.NoError(t, am.CleanupOldVersions(d.FQDN))
	require.NoDirExists(t, am.GetArtifactPath(d.FQDN, "v1"))
	require.DirExists(t, am.GetArtifactPath(d.FQDN, "v2"))
	before := fetches.Load()
	rollback := d
	rollback.Sequence = 5
	require.Equal(t, 200, send(h, rollback).Code)
	require.Equal(t, before+1, fetches.Load())
	target, err = os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, "artifacts/v1/public", target)
	preview, err := os.Readlink(filepath.Join(am.GetSitePath(d.FQDN), "preview"))
	require.NoError(t, err)
	require.Equal(t, "artifacts/v2/public", preview)
	require.Equal(t, 409, send(h, next).Code)
	require.NoError(t, os.WriteFile(h.receiptPath(d.FQDN), []byte("corrupt"), 0600))
	failed.Sequence = 6
	require.Equal(t, 503, send(h, failed).Code)
	target, err = os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, "artifacts/v1/public", target)
}
