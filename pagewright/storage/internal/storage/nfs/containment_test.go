package nfs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSymlinkedStorageNeverEscapes(t *testing.T) {
	for _, component := range []string{"sites", "sites/site", "sites/site/artifacts", "sites/site/metadata", "sites/site/metadata/v1"} {
		t.Run(component, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			n, err := NewNFSBackend(root)
			require.NoError(t, err)
			link := filepath.Join(root, component)
			require.NoError(t, os.MkdirAll(filepath.Dir(link), 0700))
			require.NoError(t, os.Symlink(outside, link))
			if strings.Contains(component, "metadata") {
				require.Error(t, n.StorePrivateLog("site", "v1", []byte("private")))
				_, err = n.FetchPrivateLog("site", "v1")
				require.Error(t, err)
			} else {
				require.Error(t, n.StoreArtifact("site", "v1", strings.NewReader("artifact")))
				_, err = n.FetchArtifact("site", "v1")
				require.Error(t, err)
			}
			entries, err := os.ReadDir(outside)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestFinalSymlinkAndTraversalRejected(t *testing.T) {
	root := t.TempDir()
	n, err := NewNFSBackend(root)
	require.NoError(t, err)
	out := filepath.Join(t.TempDir(), "sentinel")
	require.NoError(t, os.WriteFile(out, []byte("secret"), 0600))
	dir := filepath.Join(root, "sites/site/artifacts")
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, os.Symlink(out, filepath.Join(dir, "v1.tar.gz")))
	_, err = n.FetchArtifact("site", "v1")
	require.Error(t, err)
	require.Error(t, n.StoreArtifact("site", "v1", strings.NewReader("replace")))
	for _, id := range []string{"../outside", "/tmp/outside", ".", "..", "a/b", `a\b`, "a%2fb", "a\n"} {
		require.Error(t, n.StoreArtifact(id, "v1", strings.NewReader("bad")))
		require.Error(t, n.StoreArtifact("site", id, strings.NewReader("bad")))
	}
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Equal(t, "secret", string(data))
}
