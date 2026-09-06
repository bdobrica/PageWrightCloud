package nfs

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupStagingPreservesLiveAndFinalFiles(t *testing.T) {
	root := t.TempDir()
	n, err := NewNFSBackend(root)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "sites", "site", "artifacts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, ".upload-abandoned")
	final := filepath.Join(dir, "v1.tar.gz")
	recent := filepath.Join(dir, ".upload-recent")
	outside := filepath.Join(t.TempDir(), ".upload-outside")
	for _, path := range []string{old, final, recent, outside} {
		if err := os.WriteFile(path, []byte("reserved bytes"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-8 * 24 * time.Hour)
	for _, path := range []string{old, final, outside} {
		if err := os.Chtimes(path, past, past); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, ".upload-symlink")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(root, "external")); err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- n.StoreArtifact("site", "inflight", reader) }()
	if _, err := writer.Write([]byte("first bytes")); err != nil {
		t.Fatal(err)
	}
	if removed, err := n.CleanupStaging(context.Background(), time.Now()); err != nil || removed != 0 {
		t.Fatalf("cleanup raced active upload: %d %v", removed, err)
	}
	writer.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	removed, err := n.CleanupStaging(context.Background(), time.Now())
	if err != nil || removed != 1 {
		t.Fatalf("cleanup %d %v", removed, err)
	}
	if _, err := os.Lstat(old); !os.IsNotExist(err) {
		t.Fatal("abandoned stage retained")
	}
	for _, path := range []string{final, recent, outside, link, filepath.Join(dir, "inflight.tar.gz")} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("protected file removed: %s %v", path, err)
		}
	}
}

func TestCleanupLocksAcrossBackendsAndCancellation(t *testing.T) {
	root := t.TempDir()
	n, _ := NewNFSBackend(root)
	lock, err := uploadLock(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if count, err := n.CleanupStaging(context.Background(), time.Now()); err != nil || count != 0 {
		t.Fatal("shared lock ignored")
	}
	lock.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := n.CleanupStaging(ctx, time.Now()); err == nil {
		t.Fatal("cancellation ignored")
	}
}
