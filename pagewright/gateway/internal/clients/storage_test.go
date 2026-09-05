package clients

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestArtifactTransport(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	if _, err := gz.Write([]byte("archive bytes\x00\xff")); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sites/site-1/artifacts/version_1" || r.Method != "GET" || r.Header.Get("Accept-Encoding") != "identity" {
			t.Errorf("unexpected request: %s %s %v", r.Method, r.URL, r.Header)
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Write(archive.Bytes())
	}))
	defer server.Close()
	got, err := NewStorageClient(server.URL+"/").FetchArtifact("site-1", "version_1")
	if err != nil || !bytes.Equal(got, archive.Bytes()) {
		t.Fatalf("archive changed: %v", err)
	}
}

func TestArtifactTransportFailures(t *testing.T) {
	for _, tc := range []struct {
		name, media, encoding string
		status                int
		truncated             bool
	}{
		{"missing", "", "", 404, false},
		{"media", "application/json", "", 200, false},
		{"encoding", "application/gzip", "gzip", 200, false},
		{"repeated encoding", "application/gzip", "identity", 200, false},
		{"redirect", "application/gzip", "", 307, false},
		{"truncated", "application/gzip", "", 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", tc.media)
				w.Header().Set("Content-Encoding", tc.encoding)
				if tc.name == "repeated encoding" {
					w.Header().Add("Content-Encoding", "gzip")
				}
				w.Header().Set("Location", "/redirected")
				if tc.truncated {
					w.Header().Set("Content-Length", "100")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, "partial")
			}))
			defer server.Close()
			if _, err := NewStorageClient(server.URL).FetchArtifact("site", "version"); err == nil {
				t.Fatal("expected error")
			}
			if calls != 1 {
				t.Fatalf("followed redirect: %d calls", calls)
			}
		})
	}
}

func TestStorageInvalidIDs(t *testing.T) {
	client := NewStorageClient(":invalid-url")
	for _, id := range []string{"", ".", "..", "a/b", "a?b", "a#b", "a%2fb", "a b", strings.Repeat("a", 256)} {
		if _, err := client.FetchArtifact(id, "version"); err == nil || !strings.Contains(err.Error(), "invalid storage") {
			t.Errorf("site %q: %v", id, err)
		}
		if _, err := client.FetchArtifact("site", id); err == nil || !strings.Contains(err.Error(), "invalid storage") {
			t.Errorf("version %q: %v", id, err)
		}
		if _, err := client.ListVersions(id); err == nil || !strings.Contains(err.Error(), "invalid storage") {
			t.Errorf("list %q: %v", id, err)
		}
		if err := client.DeleteVersion("site", id); err == nil || !strings.Contains(err.Error(), "invalid storage") {
			t.Errorf("delete %q: %v", id, err)
		}
	}
}

func TestStorageListAndUnsupportedDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/sites/site/versions" {
			fmt.Fprint(w, `{"versions":[{"build_id":"version"}]}`)
			return
		}
		if r.Method != "DELETE" || r.URL.Path != "/sites/site/artifacts/version" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()
	client := NewStorageClient(server.URL)
	versions, err := client.ListVersions("site")
	if err != nil || len(versions) != 1 || versions[0].BuildID != "version" {
		t.Fatalf("versions: %v %v", versions, err)
	}
	if err := client.DeleteVersion("site", "version"); err == nil {
		t.Fatal("unsupported deletion reported success")
	}
}
