package nfs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommittedVersionsSortedAndPrivate(t *testing.T) {
	backend, root := setupTestBackend(t)
	for _, version := range []string{"v1", "v3", "v2"} {
		require.NoError(t, backend.StoreArtifact("site", version, strings.NewReader("archive")))
		require.NoError(t, backend.StorePrivateLog("site", version, []byte(`{"content":"private"}`)))
		data := json.RawMessage(fmt.Sprintf(`{"site_id":"site","build_id":%q,"created_at":"2026-09-05T12:00:0%sZ","prompt":"private"}`, version, version[1:]))
		require.NoError(t, backend.CommitManifest("site", version, data))
	}
	versions, err := backend.ListVersions("site")
	require.NoError(t, err)
	require.Len(t, versions, 3)
	for i, version := range versions {
		require.Equal(t, fmt.Sprintf("v%d", 3-i), version.BuildID)
		require.Equal(t, "completed", version.Status)
		require.Empty(t, version.Metadata)
	}
	// A corrupt manifest and a missing artifact cannot remain visible.
	require.NoError(t, os.WriteFile(filepath.Join(root, "sites", "site", "metadata", "v3", "manifest.json"), []byte(`broken`), 0600))
	require.NoError(t, os.Remove(filepath.Join(root, "sites", "site", "artifacts", "v2.tar.gz")))
	versions, err = backend.ListVersions("site")
	require.NoError(t, err)
	require.Len(t, versions, 1)
	require.Equal(t, "v1", versions[0].BuildID)
	for _, ids := range [][2]string{{"../outside", "v1"}, {"site", "../outside"}} {
		require.Error(t, backend.StorePrivateLog(ids[0], ids[1], nil))
		_, err := backend.FetchManifest(ids[0], ids[1])
		require.Error(t, err)
	}
}
