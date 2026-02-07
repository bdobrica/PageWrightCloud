package nfs

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage"
)

type NFSBackend struct {
	basePath string
}

func NewNFSBackend(basePath string) (*NFSBackend, error) {
	// Verify base path exists or create it
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base path: %w", err)
	}

	return &NFSBackend{
		basePath: basePath,
	}, nil
}

func (n *NFSBackend) StoreArtifact(siteID, buildID string, reader io.Reader) error {
	artifactDir := filepath.Join(n.basePath, "sites", siteID, "artifacts")
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return fmt.Errorf("failed to create artifact directory: %w", err)
	}

	artifactPath := filepath.Join(artifactDir, fmt.Sprintf("%s.tar.gz", buildID))
	return atomicWrite(artifactPath, reader)
}

func (n *NFSBackend) FetchArtifact(siteID, buildID string) (io.ReadCloser, error) {
	artifactPath := filepath.Join(n.basePath, "sites", siteID, "artifacts", fmt.Sprintf("%s.tar.gz", buildID))

	file, err := os.Open(artifactPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("artifact not found: %s/%s", siteID, buildID)
		}
		return nil, fmt.Errorf("failed to open artifact: %w", err)
	}

	return file, nil
}

func (n *NFSBackend) WriteLogEntry(siteID string, entry *storage.LogEntry) error {
	logDir := filepath.Join(n.basePath, "sites", siteID, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Use timestamp and build_id for unique, sortable filename
	timestamp := entry.Timestamp.UTC().Format("20060102-150405.000000")
	logPath := filepath.Join(logDir, fmt.Sprintf("%s-%s.json", timestamp, entry.BuildID))

	// Marshal to JSON
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	// Use atomic write for log entry
	return atomicWriteBytes(logPath, data)
}

func (n *NFSBackend) ListVersions(siteID string) ([]*storage.Version, error) {
	logDir := filepath.Join(n.basePath, "sites", siteID, "logs")

	// Check if directory exists
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		return []*storage.Version{}, nil
	}

	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read log directory: %w", err)
	}

	versions := make([]*storage.Version, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		logPath := filepath.Join(logDir, entry.Name())
		data, err := os.ReadFile(logPath)
		if err != nil {
			continue // Skip unreadable files
		}

		var logEntry storage.LogEntry
		if err := json.Unmarshal(data, &logEntry); err != nil {
			continue // Skip malformed files
		}

		versions = append(versions, &storage.Version{
			BuildID:   logEntry.BuildID,
			Timestamp: logEntry.Timestamp,
			Action:    logEntry.Action,
			Status:    logEntry.Status,
			Metadata:  logEntry.Metadata,
		})
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Timestamp.After(versions[j].Timestamp)
	})

	return versions, nil
}

// atomicWrite writes data from reader to path atomically
func atomicWrite(path string, reader io.Reader) error {
	tmpPath := path + ".tmp"

	// Create temporary file
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	// Copy data
	_, err = io.Copy(tmpFile, reader)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write data: %w", err)
	}

	// Sync to disk
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to sync file: %w", err)
	}

	tmpFile.Close()

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

// atomicWriteBytes writes byte data to path atomically
func atomicWriteBytes(path string, data []byte) error {
	tmpPath := path + ".tmp"

	// Write to temporary file
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Open for sync
	tmpFile, err := os.Open(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to open temp file: %w", err)
	}

	// Sync to disk
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to sync file: %w", err)
	}

	tmpFile.Close()

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}
