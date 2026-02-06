package storage

import (
	"io"
	"time"
)

// Backend defines the interface for storage backends
type Backend interface {
	// StoreArtifact stores an artifact tar.gz file
	StoreArtifact(siteID, buildID string, reader io.Reader) error

	// FetchArtifact retrieves an artifact and returns a reader
	FetchArtifact(siteID, buildID string) (io.ReadCloser, error)

	// WriteLogEntry writes a log entry for a site
	WriteLogEntry(siteID string, entry *LogEntry) error

	// ListVersions lists all versions for a site, sorted by timestamp
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
