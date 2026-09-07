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

var metadataID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$`)

func (n *NFSBackend) metadataPath(site, version, name string) (string, error) {
	if !metadataID.MatchString(site) || !metadataID.MatchString(version) {
		return "", fmt.Errorf("invalid metadata identity")
	}
	path := filepath.Join(n.basePath, "sites", site, "metadata", version, name)
	return path, contained(n.basePath, path)
}

func (n *NFSBackend) StorePrivateLog(site, version string, data []byte) error {
	path, err := n.metadataPath(site, version, "execution.json")
	if err != nil {
		return err
	}
	return immutableWrite(n.basePath, path, bytes.NewReader(data), n.writeGuard)
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
		if err := contained(n.basePath, path); err != nil {
			return err
		}
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
	// A concurrent upload may have linked its completed inode but not yet
	// flushed its directory. Flush prerequisites before publishing the commit.
	if err := syncDirectory(filepath.Join(n.basePath, "sites", site, "artifacts")); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	return immutableWrite(n.basePath, path, bytes.NewReader(data), n.writeGuard)
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
