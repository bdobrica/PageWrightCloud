package nfs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage"
)

type NFSBackend struct {
	basePath   string
	writeGuard func(string, int64) error
}

func (n *NFSBackend) WithWriteGuard(guard func(string, int64) error) storage.Backend {
	copy := *n
	copy.writeGuard = guard
	return &copy
}

func NewNFSBackend(basePath string) (*NFSBackend, error) {
	basePath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, err
	}
	// Verify base path exists or create it
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base path: %w", err)
	}

	return &NFSBackend{
		basePath: basePath,
	}, nil
}

func (n *NFSBackend) StoreArtifact(siteID, buildID string, reader io.Reader) error {
	if !metadataID.MatchString(siteID) || !metadataID.MatchString(buildID) {
		return fmt.Errorf("invalid artifact identity")
	}
	artifactDir := filepath.Join(n.basePath, "sites", siteID, "artifacts")
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return fmt.Errorf("failed to create artifact directory: %w", err)
	}

	artifactPath := filepath.Join(artifactDir, fmt.Sprintf("%s.tar.gz", buildID))
	return immutableWrite(n.basePath, artifactPath, reader, n.writeGuard)
}

func (n *NFSBackend) FetchArtifact(siteID, buildID string) (io.ReadCloser, error) {
	if !metadataID.MatchString(siteID) || !metadataID.MatchString(buildID) {
		return nil, fmt.Errorf("invalid artifact identity")
	}
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
	if entry == nil || !metadataID.MatchString(siteID) || !metadataID.MatchString(entry.BuildID) {
		return fmt.Errorf("invalid event identity")
	}
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
	return immutableWrite(n.basePath, logPath, bytes.NewReader(data))
}

func (n *NFSBackend) ListVersions(siteID string) ([]*storage.Version, error) {
	if !metadataID.MatchString(siteID) {
		return nil, fmt.Errorf("invalid site identity")
	}
	metadataDir := filepath.Join(n.basePath, "sites", siteID, "metadata")

	// Check if directory exists
	if _, err := os.Stat(metadataDir); os.IsNotExist(err) {
		return []*storage.Version{}, nil
	}

	entries, err := os.ReadDir(metadataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata directory: %w", err)
	}

	versions := make([]*storage.Version, 0, len(entries))

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		data, err := n.FetchManifest(siteID, entry.Name())
		if err != nil {
			continue // Incomplete, missing or corrupt commit records stay hidden.
		}

		var manifest storage.ManifestIdentity
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue // Skip malformed files
		}

		versions = append(versions, &storage.Version{
			BuildID:   manifest.BuildID,
			Timestamp: manifest.CreatedAt,
			Action:    "build",
			Status:    "completed",
		})
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Timestamp.After(versions[j].Timestamp)
	})

	return versions, nil
}
