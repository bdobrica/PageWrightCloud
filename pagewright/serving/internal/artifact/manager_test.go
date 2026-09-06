package artifact

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractDomain(t *testing.T) {
	mgr := &Manager{}

	tests := []struct {
		fqdn     string
		expected string
	}{
		{"blog.example.com", "example.com"},
		{"www.blog.example.com", "example.com"},
		{"api.v2.example.com", "example.com"},
		{"example.com", "example.com"},
		{"localhost", "localhost"},
	}

	for _, tt := range tests {
		t.Run(tt.fqdn, func(t *testing.T) {
			result := mgr.extractDomain(tt.fqdn)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetSitePath(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := &Manager{
		wwwRoot: tmpDir,
	}

	path := mgr.GetSitePath("blog.example.com")
	expected := filepath.Join(tmpDir, "example.com", "blog.example.com")
	assert.Equal(t, expected, path)
}

func TestDeployArtifact(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := &Manager{
		wwwRoot: tmpDir,
	}

	// Create a test artifact
	artifactPath := createTestArtifact(t, tmpDir)

	err := mgr.DeployArtifact("blog.example.com", "v1", artifactPath)
	require.NoError(t, err)

	// Verify artifact was extracted
	sitePath := mgr.GetSitePath("blog.example.com")
	versionPath := filepath.Join(sitePath, "artifacts", "v1", "public")

	assert.DirExists(t, versionPath)
	assert.FileExists(t, filepath.Join(versionPath, "index.html"))

	// Read and verify content
	content, err := os.ReadFile(filepath.Join(versionPath, "index.html"))
	require.NoError(t, err)
	assert.Equal(t, "<html>test</html>", string(content))
}

func TestActivateVersion(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := &Manager{
		wwwRoot: tmpDir,
	}

	// Deploy artifact first
	artifactPath := createTestArtifact(t, tmpDir)
	err := mgr.DeployArtifact("blog.example.com", "v1", artifactPath)
	require.NoError(t, err)

	// Activate as public
	err = mgr.ActivateVersion("blog.example.com", "v1", false)
	require.NoError(t, err)

	// Verify symlink
	sitePath := mgr.GetSitePath("blog.example.com")
	publicLink := filepath.Join(sitePath, "public")

	assert.FileExists(t, publicLink)

	// Verify symlink points to correct location
	target, err := os.Readlink(publicLink)
	require.NoError(t, err)
	assert.Equal(t, "artifacts/v1/public", target)

	// Activate as preview
	err = mgr.ActivateVersion("blog.example.com", "v1", true)
	require.NoError(t, err)

	previewLink := filepath.Join(sitePath, "preview")
	assert.FileExists(t, previewLink)

	target, err = os.Readlink(previewLink)
	require.NoError(t, err)
	assert.Equal(t, "artifacts/v1/public", target)
}

func TestCleanupOldVersions(t *testing.T) {
	m := NewManager(t.TempDir(), 3)
	archive := writeTestArchive(t, validEntries())
	for i, v := range []string{"v1", "v2", "v3", "v4", "v5"} {
		require.NoError(t, m.DeployArtifact("blog.example.com", v, archive))
		stamp := time.Unix(100+int64(i), 0)
		require.NoError(t, os.Chtimes(m.GetArtifactPath("blog.example.com", v), stamp, stamp))
	}
	require.NoError(t, m.ActivateVersion("blog.example.com", "v3", false))
	require.NoError(t, m.CleanupOldVersions("blog.example.com"))
	entries, err := os.ReadDir(filepath.Join(m.GetSitePath("blog.example.com"), "artifacts"))
	require.NoError(t, err)
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	require.Equal(t, []string{"v3", "v4", "v5"}, names)
	require.FileExists(t, filepath.Join(m.GetSitePath("blog.example.com"), "public", "index.html"))
}

func TestRemoveSite(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := &Manager{
		wwwRoot: tmpDir,
	}

	// Deploy artifact
	artifactPath := createTestArtifact(t, tmpDir)
	err := mgr.DeployArtifact("blog.example.com", "v1", artifactPath)
	require.NoError(t, err)

	// Verify site exists
	sitePath := mgr.GetSitePath("blog.example.com")
	assert.DirExists(t, sitePath)

	// Remove site
	err = mgr.RemoveSite("blog.example.com")
	require.NoError(t, err)

	// Verify site is gone
	_, err = os.Stat(sitePath)
	assert.True(t, os.IsNotExist(err))
}

// Helper function to create a test artifact (tar.gz with public/index.html)
func createTestArtifact(t *testing.T, baseDir string) string {
	t.Helper()

	// Create temp directory for artifact content
	contentDir := filepath.Join(baseDir, "artifact-content")
	publicDir := filepath.Join(contentDir, "public")
	err := os.MkdirAll(publicDir, 0755)
	require.NoError(t, err)

	// Create index.html
	require.NoError(t, os.MkdirAll(filepath.Join(contentDir, "content", "home"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(contentDir, "content", "site.json"), []byte(`{"site_name":"Test"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(contentDir, "content", "home", "index.md"), []byte("# Test"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(contentDir, "manifest.json"), []byte(`{"schema_version":1,"kind":"compiled","theme_id":"starter"}`), 0644))
	indexPath := filepath.Join(publicDir, "index.html")
	err = os.WriteFile(indexPath, []byte("<html>test</html>"), 0644)
	require.NoError(t, err)

	// Create tar.gz
	artifactPath := filepath.Join(baseDir, "artifact-"+time.Now().Format("20060102150405")+".tar.gz")
	file, err := os.Create(artifactPath)
	require.NoError(t, err)
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Add public directory and index.html to tar
	err = filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(contentDir, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// Write file content if it's a file
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return err
			}
		}

		return nil
	})
	require.NoError(t, err)

	return artifactPath
}
