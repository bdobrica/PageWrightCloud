package main

import (
	"bytes"
	"encoding/json"
	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage/nfs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The importer trusts the operator compiler, not arbitrary caller-supplied
// provenance. These small byte fixtures exercise persistence, not compilation.
func setup(t *testing.T) (*nfs.NFSBackend, string, map[string]any) {
	t.Helper()
	backend, err := nfs.NewNFSBackend(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := []byte("immutable committed base")
	if err = backend.StoreArtifact("site", "base", bytes.NewReader(base)); err != nil {
		t.Fatal(err)
	}
	if err = backend.StorePrivateLog("site", "base", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if err = backend.CommitManifest("site", "base", []byte(`{"site_id":"site","build_id":"base","created_at":"2026-09-10T00:00:00Z"}`)); err != nil {
		t.Fatal(err)
	}
	bundle := t.TempDir()
	artifact := []byte("trusted compiler test bytes")
	os.WriteFile(filepath.Join(bundle, "artifact.tar.gz"), artifact, 0600)
	bh, _ := hash(bytes.NewReader(base))
	ah, _ := hash(bytes.NewReader(artifact))
	manifest := map[string]any{"site_id": "site", "base_build_id": "base", "build_id": "operator-m49-v2", "provenance": "operator-acceptance-v1", "provider_calls": 0, "artifact_sha256": ah, "base_archive_sha256": bh, "kind": "compiled", "checks_passed": true, "browser_checks_performed": false, "created_at": time.Now().UTC(), "validation_checks": []string{"allowed_source_changes", "trusted_compiler", "html_and_local_references", "archive_layout"}}
	return backend, bundle, manifest
}
func writeManifest(t *testing.T, bundle string, m map[string]any) {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(bundle, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestImmutableIdempotentImport(t *testing.T) {
	backend, bundle, m := setup(t)
	writeManifest(t, bundle, m)
	for i := 0; i < 2; i++ {
		if err := importArtifact(backend, bundle, "site", "base", "operator-m49-v2"); err != nil {
			t.Fatal(err)
		}
	}
	versions, err := backend.ListVersions("site")
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions=%v err=%v", versions, err)
	}
	original, _ := backend.FetchManifest("site", "operator-m49-v2")
	m["created_at"] = time.Now().Add(time.Hour).UTC()
	writeManifest(t, bundle, m)
	if err := importArtifact(backend, bundle, "site", "base", "operator-m49-v2"); err == nil {
		t.Fatal("overwrote committed receipt")
	}
	after, _ := backend.FetchManifest("site", "operator-m49-v2")
	if !bytes.Equal(original, after) {
		t.Fatal("receipt changed")
	}
}
func TestRejectInvalidReceiptBeforeAnyWrite(t *testing.T) {
	for field, value := range map[string]any{"site_id": "other", "base_build_id": "other", "build_id": "base", "provenance": "ai", "provider_calls": 1, "kind": "source", "checks_passed": false, "browser_checks_performed": true, "validation_checks": []string{}, "artifact_sha256": string(bytes.Repeat([]byte("0"), 64)), "base_archive_sha256": string(bytes.Repeat([]byte("0"), 64))} {
		t.Run(field, func(t *testing.T) {
			backend, bundle, m := setup(t)
			m[field] = value
			writeManifest(t, bundle, m)
			if err := importArtifact(backend, bundle, "site", "base", "operator-m49-v2"); err == nil {
				t.Fatal("accepted invalid receipt")
			}
			if _, err := backend.FetchArtifact("site", "operator-m49-v2"); err == nil {
				t.Fatal("wrote rejected artifact")
			}
		})
	}
}
func TestRejectSymlinkAndUnknownSite(t *testing.T) {
	backend, bundle, m := setup(t)
	writeManifest(t, bundle, m)
	if err := importArtifact(backend, bundle, "other", "base", "operator-m49-v2"); err == nil {
		t.Fatal("accepted other site")
	}
	path := filepath.Join(bundle, "artifact.tar.gz")
	saved := filepath.Join(bundle, "saved")
	os.Rename(path, saved)
	os.Symlink(saved, path)
	if err := importArtifact(backend, bundle, "site", "base", "operator-m49-v2"); err == nil {
		t.Fatal("accepted symlink")
	}
}
