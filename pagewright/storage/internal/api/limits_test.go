package api

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage/nfs"
	"github.com/gorilla/mux"
)

type zeroBody struct{}

func (zeroBody) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestOversizedBootstrapCannotPublish(t *testing.T) {
	root := t.TempDir()
	backend, err := nfs.NewNFSBackend(root)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(backend)
	r := httptest.NewRequest("PUT", "/sites/site/artifacts/initial", io.LimitReader(zeroBody{}, (64<<20)+1))
	r.Header.Set("Content-Type", "application/gzip")
	r = mux.SetURLVars(r, map[string]string{"site_id": "site", "build_id": "initial"})
	w := httptest.NewRecorder()
	h.StoreArtifact(w, r)
	if w.Code != 413 {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "sites/site/artifacts/initial.tar.gz")); !os.IsNotExist(err) {
		t.Fatal("oversized artifact published")
	}
	entries, err := os.ReadDir(filepath.Join(root, "sites/site/artifacts"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("staging not cleaned: %v", err)
	}
}
