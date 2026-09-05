//go:build integration

package storage

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/artifact"
)

// Uses the same fixture and stored bytes subsequently consumed by gateway and
// serving, including metadata commit visibility. No AI, worker orchestration
// or publishing is exercised.
func TestArtifactTransportIntegration(t *testing.T) {
	env := make(map[string]string)
	for _, key := range []string{"TEST_STORAGE_URL", "TEST_ARTIFACT_FIXTURE", "TEST_ARTIFACT_SITE_ID", "TEST_ARTIFACT_VERSION_ID", "TEST_ARTIFACT_PATH"} {
		env[key] = os.Getenv(key)
		if env[key] == "" {
			t.Fatalf("%s is required; run make test-integration from the repository root", key)
		}
	}
	var fixture struct {
		Files []struct {
			Path   string  `json:"path"`
			Text   *string `json:"text"`
			Base64 *string `json:"base64"`
		} `json:"files"`
	}
	data, err := os.ReadFile(env["TEST_ARTIFACT_FIXTURE"])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Files) == 0 {
		t.Fatal("artifact fixture has no files")
	}
	source := t.TempDir()
	expected := make(map[string][]byte)
	for _, file := range fixture.Files {
		if !filepath.IsLocal(file.Path) || file.Path == "." {
			t.Fatalf("invalid fixture path %q", file.Path)
		}
		if (file.Text == nil) == (file.Base64 == nil) {
			t.Fatalf("fixture %s needs exactly one of text/base64", file.Path)
		}
		var content []byte
		if file.Text != nil {
			content = []byte(*file.Text)
		} else {
			content, err = base64.StdEncoding.DecodeString(*file.Base64)
			if err != nil {
				t.Fatal(err)
			}
		}
		if _, exists := expected[file.Path]; exists {
			t.Fatalf("duplicate fixture path %s", file.Path)
		}
		expected[file.Path] = content
		path := filepath.Join(source, file.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	archive := filepath.Join(t.TempDir(), "fixture.tar.gz")
	if err := artifact.Pack(source, archive); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(env["TEST_STORAGE_URL"])
	if err := client.UploadArtifact(env["TEST_ARTIFACT_SITE_ID"], env["TEST_ARTIFACT_VERSION_ID"], archive); err != nil {
		t.Fatal(err)
	}
	download := filepath.Join(t.TempDir(), "download.tar.gz")
	if err := client.FetchArtifact(env["TEST_ARTIFACT_SITE_ID"], env["TEST_ARTIFACT_VERSION_ID"], download); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(download)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("stored archive differs from packed bytes: %v", err)
	}
	destination := t.TempDir()
	if err := artifact.Unpack(download, destination); err != nil {
		t.Fatal(err)
	}
	seen := 0
	if err := filepath.WalkDir(destination, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(destination, path)
		if err != nil {
			return err
		}
		want, exists := expected[filepath.ToSlash(rel)]
		if !exists {
			t.Errorf("unexpected unpacked file %s", rel)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(content, want) {
			t.Errorf("unpacked file bytes differ: %s", rel)
		}
		seen++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if seen != len(expected) {
		t.Fatalf("unpacked files=%d expected=%d", seen, len(expected))
	}
	// Commit metadata through the real worker client. Listing stays empty until
	// the final manifest; private log content never enters the packed archive.
	base := env["TEST_STORAGE_URL"] + "/sites/" + env["TEST_ARTIFACT_SITE_ID"]
	checkListing := func(count int) {
		t.Helper()
		resp, err := http.Get(base + "/versions")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var listing struct {
			Count int `json:"count"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil || resp.StatusCode != 200 || listing.Count != count {
			t.Fatalf("listing %+v: %v status %d", listing, err, resp.StatusCode)
		}
	}
	manifest := map[string]interface{}{"site_id": env["TEST_ARTIFACT_SITE_ID"], "build_id": env["TEST_ARTIFACT_VERSION_ID"], "created_at": time.Now().UTC(), "checks_passed": false, "prompt": "private fixture prompt"}
	if err := client.UploadManifest(env["TEST_ARTIFACT_SITE_ID"], env["TEST_ARTIFACT_VERSION_ID"], manifest); err == nil {
		t.Fatal("manifest accepted before log")
	}
	checkListing(0)
	privateOutput := "private fixture output\nUTF-8 café\x00"
	if err := client.UploadLog(env["TEST_ARTIFACT_SITE_ID"], env["TEST_ARTIFACT_VERSION_ID"], privateOutput); err != nil {
		t.Fatal(err)
	}
	checkListing(0)
	if err := client.UploadManifest(env["TEST_ARTIFACT_SITE_ID"], env["TEST_ARTIFACT_VERSION_ID"], manifest); err != nil {
		t.Fatal(err)
	}
	checkListing(1)
	for _, suffix := range []string{"manifest", "logs"} {
		resp, err := http.Get(base + "/artifacts/" + env["TEST_ARTIFACT_VERSION_ID"] + "/" + suffix)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("metadata fetch: %v status %d", err, resp.StatusCode)
		}
		if suffix == "logs" {
			var got struct{ Content string }
			if json.Unmarshal(body, &got) != nil || got.Content != privateOutput {
				t.Fatal("private log changed")
			}
		} else {
			expected, _ := json.Marshal(manifest)
			if !bytes.Equal(body, expected) {
				t.Fatal("manifest changed")
			}
		}
	}
	// The harness provides an isolated path shared only with subsequent suites.
	if err := os.WriteFile(env["TEST_ARTIFACT_PATH"], original, 0600); err != nil {
		t.Fatal(err)
	}
}
