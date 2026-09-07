package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage"
	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage/nfs"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func transportArchive(t *testing.T) ([]byte, map[string]string) {
	t.Helper()
	files := map[string]string{"content/index.md": "# Transport fixture\n", "public/index.html": "<!doctype html><title>Transport</title>\n", "public/assets/site.css": "body{color:#123}\n"}
	var output bytes.Buffer
	compressed := gzip.NewWriter(&output)
	archive := tar.NewWriter(compressed)
	// Stable order allows exact byte comparisons; no external fixture generator.
	for _, name := range []string{"content/index.md", "public/index.html", "public/assets/site.css"} {
		body := files[name]
		require.NoError(t, archive.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(body)), Typeflag: tar.TypeReg}))
		_, err := archive.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, archive.Close())
	require.NoError(t, compressed.Close())
	return output.Bytes(), files
}

func requireTransportArchive(t *testing.T, data []byte, expected map[string]string) {
	t.Helper()
	compressed, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	archive := tar.NewReader(compressed)
	actual := make(map[string]string)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		require.Equal(t, byte(tar.TypeReg), header.Typeflag)
		contents, err := io.ReadAll(archive)
		require.NoError(t, err)
		actual[header.Name] = string(contents)
	}
	// Tar EOF alone does not validate the enclosing gzip checksum/footer.
	_, err = io.Copy(io.Discard, compressed)
	require.NoError(t, err)
	require.NoError(t, compressed.Close())
	require.Equal(t, expected, actual)
}

func TestArtifactTransportRawGzipRoundTrip(t *testing.T) {
	base := t.TempDir()
	backend, err := nfs.NewNFSBackend(base)
	require.NoError(t, err)
	server := httptest.NewServer(NewHandler(backend).SetupRoutes())
	defer server.Close()
	original, files := transportArchive(t)
	for _, encoding := range []string{"", "identity"} {
		t.Run("encoding="+encoding, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPut, server.URL+"/sites/fixture-site/artifacts/build-v1", bytes.NewReader(original))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/gzip")
			if encoding != "" {
				req.Header.Set("Content-Encoding", encoding)
			}
			response, err := server.Client().Do(req)
			require.NoError(t, err)
			response.Body.Close()
			require.Equal(t, http.StatusCreated, response.StatusCode)
			stored, err := os.ReadFile(filepath.Join(base, "sites", "fixture-site", "artifacts", "build-v1.tar.gz"))
			require.NoError(t, err)
			require.Equal(t, original, stored, "storage altered uploaded bytes")
			// A normal Go HTTP transport advertises gzip and may transparently decode
			// Content-Encoding:gzip. Check the public headers and bytes using that client.
			response, err = server.Client().Get(server.URL + "/sites/fixture-site/artifacts/build-v1")
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusOK, response.StatusCode)
			require.Equal(t, "application/gzip", response.Header.Get("Content-Type"))
			require.Empty(t, response.Header.Get("Content-Encoding"))
			require.False(t, response.Uncompressed, "HTTP transport decoded the artifact")
			downloaded, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, original, downloaded, "HTTP transfer altered compressed bytes")
			requireTransportArchive(t, downloaded, files)
		})
	}
}

type transportCountingBackend struct {
	storage.Backend
	stores, fetches int
}

func (b *transportCountingBackend) StoreArtifact(site, version string, r io.Reader) error {
	b.stores++
	return b.Backend.StoreArtifact(site, version, r)
}
func (b *transportCountingBackend) FetchArtifact(site, version string) (io.ReadCloser, error) {
	b.fetches++
	return b.Backend.FetchArtifact(site, version)
}

func TestArtifactTransportRejectsMultipartWithoutOverwriting(t *testing.T) {
	backend, err := nfs.NewNFSBackend(t.TempDir())
	require.NoError(t, err)
	original, _ := transportArchive(t)
	require.NoError(t, backend.StoreArtifact("site", "version", bytes.NewReader(original)))
	counted := &transportCountingBackend{Backend: backend}
	server := httptest.NewServer(NewHandler(counted).SetupRoutes())
	defer server.Close()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("artifact", "archive.tar.gz")
	require.NoError(t, err)
	_, err = part.Write(original)
	require.NoError(t, err)
	require.NoError(t, form.Close())
	req, err := http.NewRequest(http.MethodPut, server.URL+"/sites/site/artifacts/version", &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", form.FormDataContentType())
	response, err := server.Client().Do(req)
	require.NoError(t, err)
	response.Body.Close()
	require.Equal(t, http.StatusUnsupportedMediaType, response.StatusCode)
	require.Zero(t, counted.stores)
	saved, err := backend.FetchArtifact("site", "version")
	require.NoError(t, err)
	defer saved.Close()
	savedBytes, err := io.ReadAll(saved)
	require.NoError(t, err)
	require.Equal(t, original, savedBytes)
}

func TestArtifactTransportRejectsMediaAndEncodingBeforeBackend(t *testing.T) {
	for _, tc := range []struct{ media, encoding string }{
		{"", ""}, {"text/plain", ""}, {"application/octet-stream", ""}, {"application/gzip; broken", ""},
		{"multipart/form-data; boundary=fixture", ""}, {"application/gzip", "gzip"},
		{"application/gzip", "br"}, {"application/gzip", "identity, gzip"}, {"application/gzip", "gzip, identity"},
		{"application/gzip", "repeated"},
	} {
		t.Run(tc.media+"/"+tc.encoding, func(t *testing.T) {
			// A backend call would panic through the nil embedded backend; invalid
			// input must be rejected before any storage operation.
			backend := &transportCountingBackend{}
			req := httptest.NewRequest(http.MethodPut, "/sites/site/artifacts/version", strings.NewReader("opaque bytes"))
			if tc.media != "" {
				req.Header.Set("Content-Type", tc.media)
			}
			if tc.encoding != "" {
				req.Header.Set("Content-Encoding", tc.encoding)
			}
			if tc.encoding == "repeated" {
				req.Header.Set("Content-Encoding", "identity")
				req.Header.Add("Content-Encoding", "gzip")
			}
			response := httptest.NewRecorder()
			NewHandler(backend).SetupRoutes().ServeHTTP(response, req)
			require.Equal(t, http.StatusUnsupportedMediaType, response.Code)
			require.Zero(t, backend.stores)
			require.Zero(t, backend.fetches)
		})
	}
}

func TestArtifactTransportRejectsInvalidIdentifiersBeforeBackend(t *testing.T) {
	for _, id := range []string{"", ".", "..", ".hidden", "-prefix", "_prefix", "a/b", "a\\b", "a b", "a?b", "a%b", "你好", strings.Repeat("a", 201)} {
		for _, field := range []string{"site_id", "build_id"} {
			for _, method := range []string{http.MethodGet, http.MethodPut} {
				t.Run(method+"/"+field+"/"+id, func(t *testing.T) {
					backend := &transportCountingBackend{}
					handler := NewHandler(backend)
					req := httptest.NewRequest(method, "/sites/site/artifacts/version", strings.NewReader("opaque bytes"))
					req.Header.Set("Content-Type", "application/gzip")
					vars := map[string]string{"site_id": "site", "build_id": "version"}
					vars[field] = id
					req = mux.SetURLVars(req, vars)
					response := httptest.NewRecorder()
					if method == http.MethodGet {
						handler.FetchArtifact(response, req)
					} else {
						handler.StoreArtifact(response, req)
					}
					require.Equal(t, http.StatusBadRequest, response.Code)
					require.Zero(t, backend.stores)
					require.Zero(t, backend.fetches)
				})
			}
		}
	}
}

func TestArtifactTransportStoresOpaqueBytesWithoutArchiveValidation(t *testing.T) {
	backend, err := nfs.NewNFSBackend(t.TempDir())
	require.NoError(t, err)
	body := []byte("not a valid gzip archive; validation is a later boundary")
	req := httptest.NewRequest(http.MethodPut, "/sites/site/artifacts/version", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/gzip")
	response := httptest.NewRecorder()
	NewHandler(backend).SetupRoutes().ServeHTTP(response, req)
	require.Equal(t, http.StatusCreated, response.Code)
	stored, err := backend.FetchArtifact("site", "version")
	require.NoError(t, err)
	defer stored.Close()
	actual, err := io.ReadAll(stored)
	require.NoError(t, err)
	require.Equal(t, body, actual)
}

type transportErrorReader struct{}

func (transportErrorReader) Read([]byte) (int, error) {
	return 0, errors.New("injected artifact read failure")
}

type transportFailingBackend struct{ storage.Backend }

func (transportFailingBackend) FetchArtifact(string, string) (io.ReadCloser, error) {
	// Large enough to commit response headers and stream data before failure.
	return io.NopCloser(io.MultiReader(bytes.NewReader(bytes.Repeat([]byte("x"), 32<<10)), transportErrorReader{})), nil
}

func TestArtifactTransportAbortsResponseAfterBackendReadFailure(t *testing.T) {
	server := httptest.NewServer(NewHandler(transportFailingBackend{}).SetupRoutes())
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/sites/site/artifacts/version")
	if err != nil {
		return
	} // Abort before client receives headers also fails the transfer.
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	_, err = io.ReadAll(response.Body)
	require.Error(t, err, "backend read failure must not look like successful HTTP EOF")
}
