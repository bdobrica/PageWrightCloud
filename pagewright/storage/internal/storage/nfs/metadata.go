package nfs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/bdobrica/PageWrightCloud/pagewright/storage/internal/storage"
)

var metadataID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)

func (n *NFSBackend) metadataPath(site, version, name string) (string, error) {
	if !metadataID.MatchString(site) || !metadataID.MatchString(version) {
		return "", fmt.Errorf("invalid metadata identity")
	}
	return filepath.Join(n.basePath, "sites", site, "metadata", version, name), nil
}

func (n *NFSBackend) StorePrivateLog(site, version string, data []byte) error {
	path, err := n.metadataPath(site, version, "execution.json")
	if err != nil {
		return err
	}
	return writeMetadata(path, data)
}

func (n *NFSBackend) FetchPrivateLog(site, version string) ([]byte, error) {
	path, err := n.metadataPath(site, version, "execution.json")
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (n *NFSBackend) prerequisites(site, version string) error {
	for _, path := range []string{
		filepath.Join(n.basePath, "sites", site, "artifacts", version+".tar.gz"),
		filepath.Join(n.basePath, "sites", site, "metadata", version, "execution.json"),
	} {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			return storage.ErrIncomplete
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return storage.ErrIncomplete
		}
	}
	return nil
}

func validManifest(data []byte, site, version string) error {
	var identity storage.ManifestIdentity
	if err := json.Unmarshal(data, &identity); err != nil {
		return err
	}
	if identity.SiteID != site || identity.BuildID != version || identity.CreatedAt.IsZero() {
		return fmt.Errorf("manifest identity or created_at invalid")
	}
	return nil
}

// The manifest is the final commit record, not an event-log side effect.
func (n *NFSBackend) CommitManifest(site, version string, data json.RawMessage) error {
	path, err := n.metadataPath(site, version, "manifest.json")
	if err != nil {
		return err
	}
	if err := validManifest(data, site, version); err != nil {
		return err
	}
	if err := n.prerequisites(site, version); err != nil {
		return err
	}
	return writeMetadata(path, data)
}

func (n *NFSBackend) FetchManifest(site, version string) (json.RawMessage, error) {
	path, err := n.metadataPath(site, version, "manifest.json")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := validManifest(data, site, version); err != nil {
		return nil, err
	}
	if err := n.prerequisites(site, version); err != nil {
		return nil, err
	}
	return data, nil
}

// Separate private files, never packed into public/. Publish by rename only
// after checked write/sync/close; crash-durable directory fsync and immutable
// multi-writer version semantics remain M1.5.
func writeMetadata(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".metadata-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := bytes.NewReader(data).WriteTo(f); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
