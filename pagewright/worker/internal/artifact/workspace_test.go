package artifact

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFreezeRejectsChangedSource(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "content"), 0755))
	source := filepath.Join(root, "content", "site.json")
	require.NoError(t, os.WriteFile(source, []byte(`{"site_name":"before"}`), 0644))
	snapshot, err := SnapshotWorkspace(root)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(source, []byte(`{"site_name":"after"}`), 0644))
	require.ErrorContains(t, FreezeContent(root, t.TempDir(), snapshot), "source changed during freeze")
}

func TestRejectProtectedDeletionsAndModeChanges(t *testing.T) {
	for _, name := range []string{"public/index.html", "manifest.json", ".codex/instructions.md", "content"} {
		before := Snapshot{name: {Mode: 0644}}
		_, err := ValidateChanges(before, Snapshot{})
		require.Error(t, err, name)
		_, err = ValidateChanges(before, Snapshot{name: {Mode: 0600}})
		require.Error(t, err, name)
	}
}
