package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJobCancellationInterruptsDownload(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer api.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := NewClient(api.URL).WithContext(ctx).FetchArtifact("site", "version", filepath.Join(t.TempDir(), "output")); err == nil {
		t.Fatal("canceled download succeeded")
	}
}

type zeroStream struct{}

func (zeroStream) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestOversizedDownloadPreservesDestination(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		io.CopyN(w, zeroStream{}, (64<<20)+1)
	}))
	defer api.Close()
	dest := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(dest, []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := NewClient(api.URL).FetchArtifact("site", "version", dest); err == nil {
		t.Fatal("oversized download accepted")
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "previous" {
		t.Fatal("failed download replaced destination")
	}
}
