package artifact

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAtomicActivationReadersFailureAndRollback(t *testing.T) {
	root := t.TempDir()
	m := NewManager(root, 0)
	fqdn := "atomic.example.test"
	first := validEntries()
	second := validEntries()
	second[3].data = "<h1>Second</h1>"
	a := writeTestArchive(t, first)
	b := writeTestArchive(t, second)
	require.NoError(t, m.DeployArtifact(fqdn, "v1", a))
	require.NoError(t, m.DeployArtifact(fqdn, "v2", b))
	require.NoError(t, m.ActivateVersion(fqdn, "v1", false))
	require.NoError(t, m.ActivateVersion(fqdn, "v1", true))
	public := filepath.Join(m.GetSitePath(fqdn), "public", "index.html")
	stop := make(chan struct{})
	ready := make(chan struct{})
	var reads atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		close(ready)
		for {
			select {
			case <-stop:
				return
			default:
			}
			data, err := os.ReadFile(public)
			if err != nil || (string(data) != first[3].data && string(data) != second[3].data) {
				t.Errorf("non-atomic read: %q %v", data, err)
				return
			}
			reads.Add(1)
		}
	}()
	<-ready
	for i := 0; i < 150; i++ {
		version := "v1"
		if i%2 == 1 {
			version = "v2"
		}
		if err := m.ActivateVersion(fqdn, version, false); err != nil {
			t.Error(err)
			break
		}
		if err := m.CleanupOldVersions(fqdn); err != nil {
			t.Error(err)
			break
		}
	}
	close(stop)
	wg.Wait()
	require.Greater(t, reads.Load(), int64(0))
	require.NoError(t, m.ActivateVersion(fqdn, "v1", false))
	// Deterministic rename failure must leave old output present.
	m.renameLink = func(string, string) error { return errors.New("injected rename failure") }
	require.Error(t, m.ActivateVersion(fqdn, "v2", false))
	data, err := os.ReadFile(public)
	require.NoError(t, err)
	require.Equal(t, first[3].data, string(data))
	leftovers, err := filepath.Glob(filepath.Join(m.GetSitePath(fqdn), ".activate-*"))
	require.NoError(t, err)
	require.Empty(t, leftovers)
	// Failure reported after rename is uncertain, with complete new bytes. A fresh
	// manager sees the new pointer; explicit rollback is another atomic selection.
	m.renameLink = func(from, to string) error {
		if err := os.Rename(from, to); err != nil {
			return err
		}
		return errors.New("injected post-rename failure")
	}
	require.Error(t, m.ActivateVersion(fqdn, "v2", false))
	data, err = os.ReadFile(public)
	require.NoError(t, err)
	require.Equal(t, second[3].data, string(data))
	m = NewManager(root, 0)
	require.NoError(t, m.ActivateVersion(fqdn, "v1", false))
	require.Error(t, m.ActivateVersion(fqdn, "missing", false))
	data, err = os.ReadFile(public)
	require.NoError(t, err)
	require.Equal(t, first[3].data, string(data))
	preview, err := os.Readlink(filepath.Join(m.GetSitePath(fqdn), "preview"))
	require.NoError(t, err)
	require.Equal(t, "artifacts/v1/public", preview)
	called := false
	require.Error(t, m.RemoveSite(fqdn, func() error { called = true; return nil }))
	require.False(t, called)
	require.FileExists(t, public)
}

func TestRetentionExactPinsReceiptAndFailClosed(t *testing.T) {
	m := NewManager(t.TempDir(), 0)
	fqdn := "retention.example.test"
	archive := writeTestArchive(t, validEntries())
	for _, v := range []string{"v1", "v10", "draft", "pending"} {
		require.NoError(t, m.DeployArtifact(fqdn, v, archive))
	}
	require.NoError(t, m.ActivateVersion(fqdn, "v10", false))
	require.NoError(t, m.ActivateVersion(fqdn, "draft", true))
	root := m.GetSitePath(fqdn)
	receipt := DeploymentReceipt{SiteID: "site", FQDN: fqdn, Sequence: 1, Version: "pending", Target: "live", Status: "activating"}
	data, err := json.Marshal(receipt)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".deployment.json"), data, 0600))
	stamp := time.Unix(100, 0)
	for _, v := range []string{"v1", "v10", "draft", "pending"} {
		require.NoError(t, os.Chtimes(m.GetArtifactPath(fqdn, v), stamp, stamp))
	}
	require.NoError(t, m.CleanupOldVersions(fqdn))
	require.NoDirExists(t, m.GetArtifactPath(fqdn, "v1")) // not a substring pin of v10
	for _, v := range []string{"v10", "draft", "pending"} {
		require.DirExists(t, m.GetArtifactPath(fqdn, v))
	}
	require.NoError(t, m.DeployArtifact(fqdn, "unused", archive))
	require.NoError(t, os.Chtimes(m.GetArtifactPath(fqdn, "unused"), stamp, stamp))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".deployment.json"), []byte("corrupt"), 0600))
	require.Error(t, m.CleanupOldVersions(fqdn))
	require.DirExists(t, m.GetArtifactPath(fqdn, "unused"))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".deployment.json"), data, 0600))
	require.NoError(t, os.Remove(filepath.Join(root, "public")))
	require.NoError(t, os.Symlink("../outside", filepath.Join(root, "public")))
	require.Error(t, m.CleanupOldVersions(fqdn))
	require.Error(t, m.ActivateVersion(fqdn, "draft", false))
	require.DirExists(t, m.GetArtifactPath(fqdn, "unused"))
	require.Error(t, m.RemoveSite(fqdn))
	require.Error(t, m.RemoveSite("../outside"))
}

func TestSymlinkedSiteAndPublicDirectoryAreNotFollowed(t *testing.T) {
	root := t.TempDir()
	m := NewManager(root, 0)
	outside := t.TempDir()
	fqdn := "site.example.test"
	require.NoError(t, os.WriteFile(filepath.Join(outside, "sentinel"), []byte("keep"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "example.test")))
	require.Error(t, m.RemoveSite(fqdn))
	require.Error(t, m.CleanupOldVersions(fqdn))
	require.Error(t, m.DeployArtifact(fqdn, "v1", writeTestArchive(t, validEntries())))
	require.FileExists(t, filepath.Join(outside, "sentinel"))
	m = NewManager(t.TempDir(), 0)
	require.NoError(t, m.DeployArtifact(fqdn, "good", writeTestArchive(t, validEntries())))
	require.NoError(t, m.ActivateVersion(fqdn, "good", false))
	for _, kind := range []string{"symlink", "file"} {
		t.Run(kind, func(t *testing.T) {
			version := m.GetArtifactPath(fqdn, kind)
			require.NoError(t, os.MkdirAll(version, 0755))
			if kind == "symlink" {
				require.NoError(t, os.Symlink(outside, filepath.Join(version, "public")))
			} else {
				require.NoError(t, os.WriteFile(filepath.Join(version, "public"), []byte("not a directory"), 0600))
			}
			require.Error(t, m.ActivateVersion(fqdn, kind, false))
			target, err := os.Readlink(filepath.Join(m.GetSitePath(fqdn), "public"))
			require.NoError(t, err)
			require.Equal(t, "artifacts/good/public", target)
		})
	}
}

func TestDeletionGuardPrecedesInfrastructureRemoval(t *testing.T) {
	m := NewManager(t.TempDir(), 0)
	fqdn := "delete.example.test"
	require.NoError(t, m.DeployArtifact(fqdn, "v1", writeTestArchive(t, validEntries())))
	require.NoError(t, m.ActivateVersion(fqdn, "v1", true))
	called := false
	require.Error(t, m.RemoveSite(fqdn, func() error { called = true; return nil }))
	require.False(t, called)
	require.FileExists(t, filepath.Join(m.GetSitePath(fqdn), "preview", "index.html"))
}

func TestRetentionGraceAndStableTieBreak(t *testing.T) {
	m := NewManager(t.TempDir(), 1)
	fqdn := "grace.example.test"
	archive := writeTestArchive(t, validEntries())
	for _, v := range []string{"a", "b"} {
		require.NoError(t, m.DeployArtifact(fqdn, v, archive))
	}
	require.NoError(t, m.CleanupOldVersions(fqdn))
	for _, v := range []string{"a", "b"} {
		require.DirExists(t, m.GetArtifactPath(fqdn, v))
		stamp := time.Unix(100, 0)
		require.NoError(t, os.Chtimes(m.GetArtifactPath(fqdn, v), stamp, stamp))
	}
	require.NoError(t, m.CleanupOldVersions(fqdn))
	require.DirExists(t, m.GetArtifactPath(fqdn, "a"))
	require.NoDirExists(t, m.GetArtifactPath(fqdn, "b"))
}

func TestRetentionSerializesWithPointerReplacement(t *testing.T) {
	m := NewManager(t.TempDir(), 0)
	fqdn := "concurrent.example.test"
	archive := writeTestArchive(t, validEntries())
	for _, v := range []string{"v1", "v2"} {
		require.NoError(t, m.DeployArtifact(fqdn, v, archive))
	}
	require.NoError(t, m.ActivateVersion(fqdn, "v1", false))
	entered := make(chan struct{})
	release := make(chan struct{})
	m.renameLink = func(from, to string) error { close(entered); <-release; return os.Rename(from, to) }
	activated := make(chan error, 1)
	go func() { activated <- m.ActivateVersion(fqdn, "v2", false) }()
	<-entered
	// Artificially age the candidates to make protection, not grace, decisive.
	stamp := time.Unix(100, 0)
	for _, v := range []string{"v1", "v2"} {
		require.NoError(t, os.Chtimes(m.GetArtifactPath(fqdn, v), stamp, stamp))
	}
	cleaning := make(chan struct{})
	cleaned := make(chan error, 1)
	go func() { close(cleaning); cleaned <- m.CleanupOldVersions(fqdn) }()
	<-cleaning
	select {
	case err := <-cleaned:
		close(release)
		<-activated
		t.Fatalf("cleanup bypassed activation lock: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-activated)
	require.NoError(t, <-cleaned)
	require.FileExists(t, filepath.Join(m.GetSitePath(fqdn), "public", "index.html"))
	require.DirExists(t, m.GetArtifactPath(fqdn, "v2"))
	require.NoDirExists(t, m.GetArtifactPath(fqdn, "v1"))
}
