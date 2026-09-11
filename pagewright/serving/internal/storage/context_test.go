package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCancelledDownloadPreservesDestination(t *testing.T) {
	entered := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		close(entered)
		<-r.Context().Done()
	}))
	defer s.Close()
	dir := t.TempDir()
	dest := filepath.Join(dir, "artifact")
	if err := os.WriteFile(dest, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- NewClient(s.URL).FetchArtifactContext(ctx, "site", "v1", dest) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("no request")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancel succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("download hung")
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "original" {
		t.Fatal("destination replaced", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("staging leaked", err)
	}
}
