package storage

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFetchArtifact(t *testing.T) {
	var body bytes.Buffer
	gz := gzip.NewWriter(&body)
	_, _ = gz.Write([]byte("unchanged gzip payload"))
	_ = gz.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/sites/site-1/artifacts/v1" || r.Header.Get("Accept-Encoding") != "identity" {
			t.Errorf("unexpected request: %s %s %s", r.Method, r.URL.Path, r.Header.Get("Accept-Encoding"))
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Encoding", "identity")
		_, _ = w.Write(body.Bytes())
	}))
	defer server.Close()
	dest := filepath.Join(t.TempDir(), "nested", "archive.tar.gz")
	if err := NewClient(server.URL+"/").FetchArtifact("site-1", "v1", dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(got, body.Bytes()) {
		t.Fatalf("bytes changed: %v", err)
	}
}

func TestFetchFailurePreservesDestination(t *testing.T) {
	for _, scenario := range []string{"missing", "truncated", "media", "encoding", "repeated encoding", "redirect"} {
		t.Run(scenario, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/gzip")
				switch scenario {
				case "missing":
					w.WriteHeader(404)
					return
				case "truncated":
					w.Header().Set("Content-Length", "100")
				case "media":
					w.Header().Set("Content-Type", "text/html")
				case "encoding":
					w.Header().Set("Content-Encoding", "gzip")
				case "repeated encoding":
					w.Header().Set("Content-Encoding", "identity")
					w.Header().Add("Content-Encoding", "gzip")
				case "redirect":
					w.Header().Set("Location", "/unexpected")
					w.WriteHeader(302)
					return
				}
				_, _ = w.Write([]byte("short"))
			}))
			defer server.Close()
			dir := t.TempDir()
			dest := filepath.Join(dir, "artifact")
			if err := os.WriteFile(dest, []byte("existing"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := NewClient(server.URL).FetchArtifact("site", "v1", dest); err == nil {
				t.Fatal("expected failure")
			}
			got, err := os.ReadFile(dest)
			if err != nil || string(got) != "existing" {
				t.Fatalf("destination changed: %q %v", got, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("temporary file leaked: %v %v", entries, err)
			}
			if requests.Load() != 1 {
				t.Fatalf("redirect followed: %d requests", requests.Load())
			}
		})
	}
}

func TestFetchInvalidIdentifiersNeverRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid identifier sent over HTTP") }))
	defer server.Close()
	for _, id := range []string{"", ".", "..", "../x", "a/b", "a?b", "a#b", "%2f", "é", "_x", "-x", strings.Repeat("a", 256)} {
		for _, ids := range [][2]string{{id, "v1"}, {"site", id}} {
			if err := NewClient(server.URL).FetchArtifact(ids[0], ids[1], filepath.Join(t.TempDir(), "artifact")); err == nil {
				t.Errorf("accepted %q", id)
			}
		}
	}
}
