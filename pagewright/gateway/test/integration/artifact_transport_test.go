//go:build integration

package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

func TestGatewayArtifactTransport(t *testing.T) {
	env := map[string]string{}
	for _, key := range []string{"TEST_STORAGE_URL", "TEST_ARTIFACT_FIXTURE", "TEST_ARTIFACT_SITE_ID", "TEST_ARTIFACT_VERSION_ID", "TEST_ARTIFACT_PATH"} {
		env[key] = os.Getenv(key)
		if env[key] == "" {
			t.Fatalf("%s required; use make test-integration", key)
		}
	}
	original, err := os.ReadFile(env["TEST_ARTIFACT_PATH"])
	if err != nil {
		t.Fatal(err)
	}
	got, err := clients.NewStorageClient(env["TEST_STORAGE_URL"]).FetchArtifact(env["TEST_ARTIFACT_SITE_ID"], env["TEST_ARTIFACT_VERSION_ID"])
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("worker/gateway archive mismatch: %v", err)
	}
	versions, err := clients.NewStorageClient(env["TEST_STORAGE_URL"]).ListVersions(env["TEST_ARTIFACT_SITE_ID"])
	if err != nil || len(versions) != 1 || versions[0].BuildID != env["TEST_ARTIFACT_VERSION_ID"] {
		t.Fatalf("committed worker version missing from gateway listing: %v %v", versions, err)
	}
	fixtureBytes, err := os.ReadFile(env["TEST_ARTIFACT_FIXTURE"])
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Files []struct{ Path, Text, Base64 string }
	}
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatal(err)
	}
	expected := map[string][]byte{}
	for _, file := range fixture.Files {
		data := []byte(file.Text)
		if file.Base64 != "" {
			data, err = base64.StdEncoding.DecodeString(file.Base64)
			if err != nil {
				t.Fatal(err)
			}
		}
		expected[file.Path] = data
	}
	gz, err := gzip.NewReader(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		data, err := io.ReadAll(tr)
		want, exists := expected[header.Name]
		if err != nil || !exists || !bytes.Equal(data, want) {
			t.Fatalf("invalid archive entry %s: %v", header.Name, err)
		}
		delete(expected, header.Name)
	}
	if _, err := io.Copy(io.Discard, gz); err != nil {
		t.Fatal(err)
	}
	if len(expected) != 0 {
		t.Fatalf("missing files: %v", expected)
	}

	// Exercise authenticated gateway streaming independently of the shared site's
	// synthetic ID: upload the identical worker archive under a real DB site.
	user, err := testDB.CreateUser("artifact-"+uuid.NewString()+"@example.test", "test-only-hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	site, err := testDB.CreateSite(user.ID, "artifact-"+uuid.NewString()+".example.test", "starter")
	if err != nil {
		t.Fatal(err)
	}
	token, err := testJWTManager.GenerateToken(user.ID, user.Email)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("PUT", env["TEST_STORAGE_URL"]+"/sites/"+site.ID+"/artifacts/version", bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("upload status %d", resp.StatusCode)
	}
	for _, truncated := range []bool{false, true} {
		t.Run(strconv.FormatBool(truncated), func(t *testing.T) {
			storageURL := env["TEST_STORAGE_URL"]
			if truncated {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/gzip")
					w.Header().Set("Content-Length", strconv.Itoa(len(original)+1))
					w.Write(original)
				}))
				defer upstream.Close()
				storageURL = upstream.URL
			}
			handler := handlers.NewVersionsHandler(testDB, clients.NewStorageClient(storageURL), nil, 25)
			router := mux.NewRouter()
			router.Handle("/sites/{fqdn}/versions/{version_id}/download", middleware.AuthMiddleware(testJWTManager)(http.HandlerFunc(handler.DownloadVersion)))
			server := httptest.NewServer(router)
			defer server.Close()
			req, err := http.NewRequest("GET", server.URL+"/sites/"+site.FQDN+"/versions/version/download", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := server.Client().Do(req)
			if err != nil {
				if truncated {
					return
				}
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if truncated {
				if err == nil {
					t.Fatal("truncated upstream became successful download")
				}
				return
			}
			if err != nil || resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/gzip" || !bytes.Equal(body, original) {
				t.Fatalf("gateway download: status %d, error %v", resp.StatusCode, err)
			}
		})
	}
}
