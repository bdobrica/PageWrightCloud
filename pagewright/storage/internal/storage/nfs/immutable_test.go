package nfs

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage"
	"github.com/stretchr/testify/require"
)

type failedUpload struct{}

func (failedUpload) Read(p []byte) (int, error) { copy(p, "partial"); return 7, io.ErrUnexpectedEOF }

func TestImmutableFailureRetryAndCommit(t *testing.T) {
	n, root := setupTestBackend(t)
	require.Error(t, n.StoreArtifact("site", "v1", failedUpload{}))
	entries, err := os.ReadDir(filepath.Join(root, "sites", "site", "artifacts"))
	require.NoError(t, err)
	require.Empty(t, entries)
	require.NoError(t, n.StoreArtifact("site", "v1", strings.NewReader("archive")))
	require.NoError(t, n.StoreArtifact("site", "v1", strings.NewReader("archive")))
	require.ErrorIs(t, n.StoreArtifact("site", "v1", strings.NewReader("changed")), storage.ErrConflict)
	log := []byte(`{"content":"original"}`)
	require.NoError(t, n.StorePrivateLog("site", "v1", log))
	manifest := []byte(`{"site_id":"site","build_id":"v1","created_at":"2026-09-05T12:00:00Z"}`)
	require.NoError(t, n.CommitManifest("site", "v1", manifest))
	n, err = NewNFSBackend(root)
	require.NoError(t, err)
	require.NoError(t, n.CommitManifest("site", "v1", manifest))
	require.NoError(t, n.StorePrivateLog("site", "v1", log))
	require.ErrorIs(t, n.StorePrivateLog("site", "v1", []byte(`{"content":"replacement"}`)), storage.ErrConflict)
	require.ErrorIs(t, n.CommitManifest("site", "v1", append(manifest, ' ')), storage.ErrConflict)
	require.ErrorIs(t, n.StoreArtifact("site", "v1", strings.NewReader("replacement")), storage.ErrConflict)
	got, err := n.FetchManifest("site", "v1")
	require.NoError(t, err)
	require.Equal(t, manifest, []byte(got))
	require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		require.False(t, strings.HasPrefix(d.Name(), ".upload-"), path)
		return nil
	}))
}

func TestImmutableConcurrentWriters(t *testing.T) {
	for _, same := range []bool{true, false} {
		root := t.TempDir()
		var wg sync.WaitGroup
		results := make(chan error, 16)
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				n, err := NewNFSBackend(root)
				if err != nil {
					results <- err
					return
				}
				body := "same"
				if !same {
					body = strings.Repeat("x", i+1)
				}
				results <- n.StoreArtifact("site", "version", strings.NewReader(body))
			}(i)
		}
		wg.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			} else {
				require.ErrorIs(t, err, storage.ErrConflict)
			}
		}
		if same {
			require.Equal(t, 16, successes)
		} else {
			require.Equal(t, 1, successes)
		}
	}
}

func TestImmutableProcessHelper(t *testing.T) {
	root := os.Getenv("TEST_IMMUTABLE_ROOT")
	if root == "" {
		return
	}
	n, err := NewNFSBackend(root)
	if err != nil {
		os.Exit(2)
	}
	err = n.StoreArtifact("site", "version", strings.NewReader(os.Getenv("TEST_IMMUTABLE_BODY")))
	if errors.Is(err, storage.ErrConflict) {
		os.Exit(10)
	}
	if err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestImmutableAcrossProcesses(t *testing.T) {
	root := t.TempDir()
	var commands []*exec.Cmd
	for i := 0; i < 6; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestImmutableProcessHelper$")
		cmd.Env = append(os.Environ(), "TEST_IMMUTABLE_ROOT="+root, "TEST_IMMUTABLE_BODY="+strings.Repeat("x", i+1))
		require.NoError(t, cmd.Start())
		commands = append(commands, cmd)
	}
	successes := 0
	for _, cmd := range commands {
		err := cmd.Wait()
		if err == nil {
			successes++
		} else {
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit)
			require.Equal(t, 10, exit.ExitCode())
		}
	}
	require.Equal(t, 1, successes)
}
