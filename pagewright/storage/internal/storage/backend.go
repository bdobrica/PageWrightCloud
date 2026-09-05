package storage

import (
	"encoding/json"
	"errors"
	"io"
	"time"
)

var ErrIncomplete = errors.New("artifact and private log must be persisted before manifest")

// VersionMetadata is separate from event logs and from the downloadable archive.
type VersionMetadata interface {
	StorePrivateLog(siteID, buildID string, data []byte) error
	FetchPrivateLog(siteID, buildID string) ([]byte, error)
	CommitManifest(siteID, buildID string, data json.RawMessage) error
	FetchManifest(siteID, buildID string) (json.RawMessage, error)
}

// ManifestIdentity is the persistence envelope. Compiler-specific fields remain
// in the stored JSON; their truthfulness/layout are later validation gates.
type ManifestIdentity struct {
	SiteID    string    `json:"site_id"`
	BuildID   string    `json:"build_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Backend defines the interface for storage backends
type Backend interface {
	// StoreArtifact stores an artifact tar.gz file
	StoreArtifact(siteID, buildID string, reader io.Reader) error

	// FetchArtifact retrieves an artifact and returns a reader
	FetchArtifact(siteID, buildID string) (io.ReadCloser, error)

	// WriteLogEntry writes a log entry for a site
	WriteLogEntry(siteID string, entry *LogEntry) error

	// ListVersions lists committed versions only, sorted by timestamp.
	ListVersions(siteID string) ([]*Version, error)
}

// LogEntry represents a log entry
type LogEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	BuildID   string            `json:"build_id"`
	SiteID    string            `json:"site_id"`
	Action    string            `json:"action"`
	Status    string            `json:"status"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// Version represents a site version
type Version struct {
	BuildID   string            `json:"build_id"`
	Timestamp time.Time         `json:"timestamp"`
	Action    string            `json:"action"`
	Status    string            `json:"status"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}
