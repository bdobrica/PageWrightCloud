package clients

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBootstrapConflictStopsWrites(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(409) }))
	defer server.Close()
	err := NewStorageClient(server.URL).InitializeSource("site", "initial", []byte("archive"), []byte("log"), []byte("manifest"))
	if !errors.Is(err, ErrBootstrapConflict) || calls != 1 {
		t.Fatalf("conflict: %v calls=%d", err, calls)
	}
}
