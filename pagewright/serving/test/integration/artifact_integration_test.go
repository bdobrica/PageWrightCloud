//go:build integration

package integration

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/artifact"
	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/storage"
)

func TestStorageArtifactRoundTripAndDeployment(t *testing.T) {
	required := func(key string) string {
		t.Helper()
		v := os.Getenv(key)
		if v == "" {
			t.Fatalf("%s is required", key)
		}
		return v
	}
	reference, err := os.ReadFile(required("TEST_ARTIFACT_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	fixtureBytes, err := os.ReadFile(required("TEST_ARTIFACT_FIXTURE"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Files []struct {
			Path   string  `json:"path"`
			Text   *string `json:"text"`
			Base64 *string `json:"base64"`
		} `json:"files"`
	}
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Files) == 0 {
		t.Fatal("empty fixture")
	}
	site, version := required("TEST_ARTIFACT_SITE_ID"), required("TEST_ARTIFACT_VERSION_ID")
	dest := filepath.Join(t.TempDir(), "download.tar.gz")
	if err := storage.NewClient(required("TEST_STORAGE_URL")).FetchArtifact(site, version, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(reference, got) {
		t.Fatalf("download differs from worker archive: %v", err)
	}
	manager := artifact.NewManager(t.TempDir(), 10)
	fqdn := "artifact.example.test"
	if err := manager.DeployArtifact(fqdn, version, dest); err != nil {
		t.Fatal(err)
	}
	root := manager.GetArtifactPath(fqdn, version)
	expected := make(map[string]bool)
	expectedDirs := map[string]bool{".": true}
	for _, file := range fixture.Files {
		if !fs.ValidPath(file.Path) || (file.Text == nil) == (file.Base64 == nil) || expected[file.Path] {
			t.Fatalf("invalid fixture entry: %s", file.Path)
		}
		expected[file.Path] = true
		for dir := filepath.Dir(filepath.FromSlash(file.Path)); dir != "."; dir = filepath.Dir(dir) {
			expectedDirs[filepath.ToSlash(dir)] = true
		}
		var want []byte
		if file.Text != nil {
			want = []byte(*file.Text)
		} else {
			want, err = base64.StdEncoding.DecodeString(*file.Base64)
			if err != nil {
				t.Fatal(err)
			}
		}
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file.Path)))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("deployed file %s differs: %v", file.Path, err)
		}
	}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if !expectedDirs[filepath.ToSlash(rel)] {
				t.Errorf("unexpected deployed directory: %s", rel)
			}
			return nil
		}
		if !entry.Type().IsRegular() || !expected[filepath.ToSlash(rel)] {
			t.Errorf("unexpected deployed entry: %s", rel)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
