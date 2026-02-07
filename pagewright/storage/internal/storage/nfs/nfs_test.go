package nfs

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestBackend(t *testing.T) (*NFSBackend, string) {
	tmpDir := t.TempDir()
	backend, err := NewNFSBackend(tmpDir)
	require.NoError(t, err)
	return backend, tmpDir
}

func TestNewNFSBackend(t *testing.T) {
	tmpDir := t.TempDir()
	backend, err := NewNFSBackend(tmpDir)

	assert.NoError(t, err)
	assert.NotNil(t, backend)
	assert.Equal(t, tmpDir, backend.basePath)
}

func TestStoreArtifact(t *testing.T) {
	backend, _ := setupTestBackend(t)

	siteID := "test-site"
	buildID := "build-123"
	content := []byte("test artifact content")

	err := backend.StoreArtifact(siteID, buildID, bytes.NewReader(content))
	assert.NoError(t, err)

	// Verify file exists
	artifactPath := filepath.Join(backend.basePath, "sites", siteID, "artifacts", buildID+".tar.gz")
	assert.FileExists(t, artifactPath)

	// Verify content
	storedContent, err := os.ReadFile(artifactPath)
	assert.NoError(t, err)
	assert.Equal(t, content, storedContent)
}

func TestStoreArtifactAtomic(t *testing.T) {
	backend, tmpDir := setupTestBackend(t)

	siteID := "test-site"
	buildID := "build-123"
	content := []byte("test artifact content")

	err := backend.StoreArtifact(siteID, buildID, bytes.NewReader(content))
	assert.NoError(t, err)

	// Verify no temp files remain
	artifactDir := filepath.Join(tmpDir, "sites", siteID, "artifacts")
	entries, err := os.ReadDir(artifactDir)
	assert.NoError(t, err)

	for _, entry := range entries {
		assert.NotContains(t, entry.Name(), ".tmp", "Temporary file should not exist after atomic write")
	}
}

func TestFetchArtifact(t *testing.T) {
	backend, _ := setupTestBackend(t)

	siteID := "test-site"
	buildID := "build-123"
	content := []byte("test artifact content")

	// Store artifact first
	err := backend.StoreArtifact(siteID, buildID, bytes.NewReader(content))
	require.NoError(t, err)

	// Fetch artifact
	reader, err := backend.FetchArtifact(siteID, buildID)
	assert.NoError(t, err)
	require.NotNil(t, reader)
	defer reader.Close()

	// Verify content
	fetchedContent, err := io.ReadAll(reader)
	assert.NoError(t, err)
	assert.Equal(t, content, fetchedContent)
}

func TestFetchArtifactNotFound(t *testing.T) {
	backend, _ := setupTestBackend(t)

	reader, err := backend.FetchArtifact("non-existent-site", "non-existent-build")
	assert.Error(t, err)
	assert.Nil(t, reader)
	assert.Contains(t, err.Error(), "artifact not found")
}

func TestWriteLogEntry(t *testing.T) {
	backend, _ := setupTestBackend(t)

	siteID := "test-site"
	entry := &storage.LogEntry{
		Timestamp: time.Now().UTC(),
		BuildID:   "build-123",
		SiteID:    siteID,
		Action:    "build",
		Status:    "success",
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	err := backend.WriteLogEntry(siteID, entry)
	assert.NoError(t, err)

	// Verify log file exists
	logDir := filepath.Join(backend.basePath, "sites", siteID, "logs")
	entries, err := os.ReadDir(logDir)
	assert.NoError(t, err)
	assert.Greater(t, len(entries), 0)
}

func TestWriteLogEntryAtomic(t *testing.T) {
	backend, tmpDir := setupTestBackend(t)

	siteID := "test-site"
	entry := &storage.LogEntry{
		Timestamp: time.Now().UTC(),
		BuildID:   "build-123",
		SiteID:    siteID,
		Action:    "build",
		Status:    "success",
		Metadata:  map[string]string{},
	}

	err := backend.WriteLogEntry(siteID, entry)
	assert.NoError(t, err)

	// Verify no temp files remain
	logDir := filepath.Join(tmpDir, "sites", siteID, "logs")
	entries, err := os.ReadDir(logDir)
	assert.NoError(t, err)

	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp", "Temporary file should not exist after atomic write")
	}
}

func TestListVersions(t *testing.T) {
	backend, _ := setupTestBackend(t)

	siteID := "test-site"

	// Write multiple log entries
	entries := []*storage.LogEntry{
		{
			Timestamp: time.Now().UTC().Add(-2 * time.Hour),
			BuildID:   "build-1",
			SiteID:    siteID,
			Action:    "build",
			Status:    "success",
		},
		{
			Timestamp: time.Now().UTC().Add(-1 * time.Hour),
			BuildID:   "build-2",
			SiteID:    siteID,
			Action:    "build",
			Status:    "success",
		},
		{
			Timestamp: time.Now().UTC(),
			BuildID:   "build-3",
			SiteID:    siteID,
			Action:    "deploy",
			Status:    "success",
		},
	}

	for _, entry := range entries {
		err := backend.WriteLogEntry(siteID, entry)
		require.NoError(t, err)
	}

	// List versions
	versions, err := backend.ListVersions(siteID)
	assert.NoError(t, err)
	assert.Len(t, versions, 3)

	// Verify sorting (newest first)
	assert.Equal(t, "build-3", versions[0].BuildID)
	assert.Equal(t, "build-2", versions[1].BuildID)
	assert.Equal(t, "build-1", versions[2].BuildID)
}

func TestListVersionsEmpty(t *testing.T) {
	backend, _ := setupTestBackend(t)

	versions, err := backend.ListVersions("non-existent-site")
	assert.NoError(t, err)
	assert.Empty(t, versions)
}

func TestListVersionsWithMetadata(t *testing.T) {
	backend, _ := setupTestBackend(t)

	siteID := "test-site"
	metadata := map[string]string{
		"user":   "john",
		"branch": "main",
	}

	entry := &storage.LogEntry{
		Timestamp: time.Now().UTC(),
		BuildID:   "build-123",
		SiteID:    siteID,
		Action:    "build",
		Status:    "success",
		Metadata:  metadata,
	}

	err := backend.WriteLogEntry(siteID, entry)
	require.NoError(t, err)

	versions, err := backend.ListVersions(siteID)
	assert.NoError(t, err)
	require.Len(t, versions, 1)

	assert.Equal(t, metadata, versions[0].Metadata)
}
