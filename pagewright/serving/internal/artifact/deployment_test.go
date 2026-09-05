package artifact

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeploymentIsolationAndRetry(t *testing.T) {
	m := NewManager(t.TempDir(), 10)
	archive := writeTestArchive(t, validEntries())
	require.NoError(t, m.DeployArtifact("test.example.test", "v1", archive))
	require.NoError(t, m.ActivateVersion("test.example.test", "v1", false))
	require.NoError(t, m.DeployArtifact("test.example.test", "v1", archive))
	public := filepath.Join(m.GetSitePath("test.example.test"), "public")
	handler := http.FileServer(http.Dir(public))
	for _, url := range []string{"/", "/content/site.json", "/content/home/index.md", "/manifest.json", "/.archive-sha256", "/.codex/instructions.md", "/.env", "/execution.log"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, url, nil))
		if url == "/" {
			require.Equal(t, 200, response.Code)
			require.Contains(t, response.Body.String(), "Public")
		} else {
			require.Equal(t, 404, response.Code, url)
		}
	}
	changed := validEntries()
	changed[3].data = "replacement"
	require.Error(t, m.DeployArtifact("test.example.test", "v1", writeTestArchive(t, changed)))
	bad := append(validEntries(), testEntry{name: "public/.env", data: "private"})
	require.Error(t, m.DeployArtifact("test.example.test", "v2", writeTestArchive(t, bad)))
	_, err := os.Stat(m.GetArtifactPath("test.example.test", "v2"))
	require.True(t, os.IsNotExist(err))
	data, err := os.ReadFile(filepath.Join(public, "index.html"))
	require.NoError(t, err)
	require.Equal(t, "<h1>Public</h1>", string(data))
	versions, err := os.ReadDir(filepath.Dir(m.GetArtifactPath("test.example.test", "v1")))
	require.NoError(t, err)
	require.Len(t, versions, 1)
	require.Error(t, m.DeployArtifact("../escape", "v1", archive))
	require.Error(t, m.DeployArtifact("test.example.test", "../escape", archive))
}

func TestConcurrentDeploymentRetries(t *testing.T) {
	m := NewManager(t.TempDir(), 10)
	archive := writeTestArchive(t, validEntries())
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errors <- m.DeployArtifact("test.example.test", "v1", archive) }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}

func TestCleanupDoesNotRemoveDeploymentStages(t *testing.T) {
	m := NewManager(t.TempDir(), 0)
	base := filepath.Dir(m.GetArtifactPath("test.example.test", "v1"))
	require.NoError(t, os.MkdirAll(filepath.Join(base, ".deploy-in-progress"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(base, "v1"), 0755))
	require.NoError(t, m.CleanupOldVersions("test.example.test"))
	_, err := os.Stat(filepath.Join(base, ".deploy-in-progress"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(base, "v1"))
	require.True(t, os.IsNotExist(err))
}
