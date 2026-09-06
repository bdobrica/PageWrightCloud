package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage/nfs"
)

func TestFencedStorageChecksAfterUploadBeforePublication(t *testing.T) {
	var expired atomic.Bool
	var calls atomic.Int32
	payload := []byte("staged archive bytes")
	digest := sha256.Sum256(payload)
	manager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var c writeCommit
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			t.Error(err)
		}
		if c.SHA256 != hex.EncodeToString(digest[:]) || c.Size != int64(len(payload)) || c.Part != "artifact" || c.LockToken != "attempt" || c.FencingToken != 7 || r.URL.Path != "/jobs/job/write-commit" {
			t.Errorf("wrong commit: %+v %s", c, r.URL)
		}
		if expired.Load() {
			w.WriteHeader(409)
		} else {
			w.WriteHeader(204)
		}
	}))
	defer manager.Close()
	root := t.TempDir()
	backend, err := nfs.NewNFSBackend(root)
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewFencedHandler(backend, manager.URL)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h.SetupRoutes())
	defer server.Close()
	identity, _ := json.Marshal(writeCommit{JobID: "job", SiteID: "site", OwnerID: "owner", SourceVersion: "initial", TargetVersion: "build", LockToken: "attempt", FencingToken: 7})
	reader, writer := io.Pipe()
	req, _ := http.NewRequest("PUT", server.URL+"/sites/site/artifacts/build", reader)
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("X-Pagewright-Attempt", string(identity))
	done := make(chan *http.Response, 1)
	errs := make(chan error, 1)
	go func() { response, err := http.DefaultClient.Do(req); errs <- err; done <- response }()
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("authorized before upload finished")
	}
	expired.Store(true)
	writer.Close()
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	response := <-done
	response.Body.Close()
	if response.StatusCode != 409 || calls.Load() != 1 {
		t.Fatalf("expired upload: %d calls=%d", response.StatusCode, calls.Load())
	}
	if _, err := os.Stat(filepath.Join(root, "sites/site/artifacts/build.tar.gz")); !os.IsNotExist(err) {
		t.Fatalf("expired upload published: %v", err)
	}
	expired.Store(false)
	req, _ = http.NewRequest("PUT", server.URL+"/sites/site/artifacts/build", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("X-Pagewright-Attempt", string(identity))
	response, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatalf("valid attempt: %d", response.StatusCode)
	}
	stored, err := os.ReadFile(filepath.Join(root, "sites/site/artifacts/build.tar.gz"))
	if err != nil || !bytes.Equal(stored, payload) {
		t.Fatalf("wrong published bytes: %v", err)
	}
	// Missing identity is rejected without contacting the authority.
	req, _ = http.NewRequest("PUT", server.URL+"/sites/site/artifacts/other", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/gzip")
	response, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 409 || calls.Load() != 2 {
		t.Fatal("unguarded worker write accepted")
	}
}
