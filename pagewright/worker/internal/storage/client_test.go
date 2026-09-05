package storage

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func archiveBytes(t *testing.T) []byte {
	t.Helper()
	var body bytes.Buffer
	w := gzip.NewWriter(&body)
	if _, err := w.Write([]byte("binary archive fixture\x00\xff")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func TestArtifactTransportWire(t *testing.T) {
	payload := archiveBytes(t)
	var put, get bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/sites/site-1/artifacts/build_2.3" {
			t.Errorf("wrong URL %s", r.URL)
		}
		switch r.Method {
		case http.MethodPut:
			put = true
			if r.Header.Get("Content-Type") != "application/gzip" || r.Header.Get("Content-Encoding") != "" {
				t.Errorf("wrong upload headers %v", r.Header)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil || !bytes.Equal(body, payload) {
				t.Errorf("upload bytes changed: %x %v", body, err)
			}
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			get = true
			if r.Header.Get("Accept-Encoding") != "identity" {
				t.Errorf("wrong Accept-Encoding %q", r.Header.Get("Accept-Encoding"))
			}
			w.Header().Set("Content-Type", "application/gzip")
			w.Header().Set("Content-Encoding", "identity")
			w.Write(payload)
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.tar.gz")
	dest := filepath.Join(dir, "nested", "download.tar.gz")
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	client := NewClient(server.URL + "/")
	if err := client.UploadArtifact("site-1", "build_2.3", source); err != nil {
		t.Fatal(err)
	}
	if err := client.FetchArtifact("site-1", "build_2.3", dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("download bytes changed: %x %v", got, err)
	}
	if !put || !get {
		t.Fatalf("requests missing: PUT=%v GET=%v", put, get)
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestArtifactUploadStreamsFile(t *testing.T) {
	payload := archiveBytes(t)
	source := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	client := NewClient("http://storage.invalid")
	client.httpClient.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		file, ok := r.Body.(*os.File)
		if !ok {
			t.Fatalf("upload must stream os.File, got %T", r.Body)
		}
		if file.Name() != source {
			t.Errorf("wrong file %s", file.Name())
		}
		if r.GetBody != nil {
			t.Error("file upload should not install an automatic replay body")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(body, payload) {
			t.Errorf("body=%x error=%v", body, err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})
	if err := client.UploadArtifact("site", "version", source); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactIdentifiersRejectBeforeHTTP(t *testing.T) {
	client := NewClient("http://storage.invalid")
	client.httpClient.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		t.Error("invalid identifier reached HTTP")
		return nil, fmt.Errorf("unexpected HTTP")
	})
	for _, id := range []string{"", ".", "..", "../site", "/site", "site/part", `site\part`, "site?x", "site#x", "site%2Fpart", "site space", "é", "-prefix", strings.Repeat("a", 256)} {
		for _, pair := range [][2]string{{id, "version"}, {"site", id}} {
			if err := client.UploadArtifact(pair[0], pair[1], "missing.tar.gz"); err == nil {
				t.Errorf("upload accepted %q", pair)
			}
			if err := client.FetchArtifact(pair[0], pair[1], filepath.Join(t.TempDir(), "download")); err == nil {
				t.Errorf("download accepted %q", pair)
			}
		}
	}
	for _, id := range []string{"a", "A0._-", strings.Repeat("a", 255)} {
		if _, err := client.artifactURL(id, id); err != nil {
			t.Errorf("valid ID rejected %q: %v", id, err)
		}
	}
}

func TestArtifactDownloadFailurePreservesDestination(t *testing.T) {
	for _, mode := range []string{"not_found", "server_error", "truncated", "gzip_encoding", "multiple_encodings", "multipart", "missing_type", "text_type"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/gzip")
				switch mode {
				case "not_found":
					w.WriteHeader(404)
					return
				case "server_error":
					w.WriteHeader(500)
					return
				case "truncated":
					w.Header().Set("Content-Length", "100")
				case "gzip_encoding":
					w.Header().Set("Content-Encoding", "gzip")
				case "multiple_encodings":
					w.Header().Add("Content-Encoding", "identity")
					w.Header().Add("Content-Encoding", "gzip")
				case "multipart":
					w.Header().Set("Content-Type", "multipart/form-data; boundary=fixture")
				case "missing_type":
					w.Header()["Content-Type"] = nil
				case "text_type":
					w.Header().Set("Content-Type", "text/plain")
				}
				w.Write([]byte("short"))
			}))
			defer server.Close()
			dir := t.TempDir()
			dest := filepath.Join(dir, "existing.tar.gz")
			if err := os.WriteFile(dest, []byte("preserve me"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := NewClient(server.URL).FetchArtifact("site", "version", dest); err == nil {
				t.Fatal("invalid response accepted")
			}
			got, err := os.ReadFile(dest)
			if err != nil || string(got) != "preserve me" {
				t.Fatalf("destination changed %q %v", got, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("temporary files remain: %v %v", entries, err)
			}
		})
	}
}

type closeErrorBody struct{ io.Reader }

func (closeErrorBody) Close() error { return fmt.Errorf("simulated response close failure") }

func TestArtifactResponseCloseFailurePreservesDestination(t *testing.T) {
	client := NewClient("http://storage.invalid")
	client.httpClient.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/gzip"}}, Body: closeErrorBody{bytes.NewReader(archiveBytes(t))}}, nil
	})
	dir := t.TempDir()
	destination := filepath.Join(dir, "archive.tar.gz")
	if err := os.WriteFile(destination, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := client.FetchArtifact("site", "version", destination); err == nil || !strings.Contains(err.Error(), "close failure") {
		t.Fatalf("error=%v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != "existing" {
		t.Fatalf("destination=%q error=%v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files remain: %v %v", entries, err)
	}
}

func TestArtifactRejectsRedirects(t *testing.T) {
	for _, method := range []string{"GET", "PUT"} {
		t.Run(method, func(t *testing.T) {
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("redirect was followed") }))
			defer destination.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
			}))
			defer server.Close()
			client := NewClient(server.URL)
			path := filepath.Join(t.TempDir(), "artifact")
			if err := os.WriteFile(path, archiveBytes(t), 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			if method == "GET" {
				err = client.FetchArtifact("site", "version", path)
			} else {
				err = client.UploadArtifact("site", "version", path)
			}
			if err == nil || !strings.Contains(err.Error(), "307") {
				t.Fatalf("redirect error=%v", err)
			}
		})
	}
}

func TestArtifactUploadHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500); w.Write([]byte("storage failure")) }))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(path, archiveBytes(t), 0600); err != nil {
		t.Fatal(err)
	}
	if err := NewClient(server.URL).UploadArtifact("site", "version", path); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("upload error=%v", err)
	}
}
