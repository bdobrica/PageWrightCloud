package artifact

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Manager struct {
	wwwRoot            string
	maxVersionsPerSite int
}

func NewManager(wwwRoot string, maxVersions int) *Manager {
	return &Manager{
		wwwRoot:            wwwRoot,
		maxVersionsPerSite: maxVersions,
	}
}

// GetSitePath returns /var/www/{domain}/{fqdn}/
func (m *Manager) GetSitePath(fqdn string) string {
	domain := m.extractDomain(fqdn)
	return filepath.Join(m.wwwRoot, domain, fqdn)
}

// GetArtifactPath returns /var/www/{domain}/{fqdn}/artifacts/{version}/
func (m *Manager) GetArtifactPath(fqdn, version string) string {
	return filepath.Join(m.GetSitePath(fqdn), "artifacts", version)
}

// DeployArtifact unpacks an artifact to the version directory
func (m *Manager) DeployArtifact(fqdn, version, archivePath string) error {
	destDir := m.GetArtifactPath(fqdn, version)

	// Create destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create artifact directory: %w", err)
	}

	// Unpack archive
	if err := m.unpack(archivePath, destDir); err != nil {
		return fmt.Errorf("failed to unpack artifact: %w", err)
	}

	return nil
}

// ActivateVersion creates/updates symlink for public or preview
func (m *Manager) ActivateVersion(fqdn, version string, isPreview bool) error {
	sitePath := m.GetSitePath(fqdn)
	artifactPath := m.GetArtifactPath(fqdn, version)

	// Verify artifact exists
	publicPath := filepath.Join(artifactPath, "public")
	if _, err := os.Stat(publicPath); err != nil {
		return fmt.Errorf("artifact public directory not found: %w", err)
	}

	// Determine symlink name
	linkName := "public"
	if isPreview {
		linkName = "preview"
	}

	linkPath := filepath.Join(sitePath, linkName)

	// Remove existing symlink if any
	os.Remove(linkPath)

	// Create relative symlink path (artifacts/{version}/public)
	relTarget := filepath.Join("artifacts", version, "public")

	// Create new symlink
	if err := os.Symlink(relTarget, linkPath); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	return nil
}

// CleanupOldVersions removes old artifact versions, keeping max configured
func (m *Manager) CleanupOldVersions(fqdn string) error {
	sitePath := m.GetSitePath(fqdn)
	artifactsDir := filepath.Join(sitePath, "artifacts")

	// List all version directories
	entries, err := os.ReadDir(artifactsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No artifacts yet
		}
		return fmt.Errorf("failed to read artifacts directory: %w", err)
	}

	// Get current active versions
	publicVersion, _ := m.getSymlinkTarget(filepath.Join(sitePath, "public"))
	previewVersion, _ := m.getSymlinkTarget(filepath.Join(sitePath, "preview"))

	// Collect versions with access times
	type versionInfo struct {
		name       string
		accessTime time.Time
		protected  bool
	}

	var versions []versionInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		versionPath := filepath.Join(artifactsDir, entry.Name())
		info, err := os.Stat(versionPath)
		if err != nil {
			continue
		}

		protected := strings.Contains(publicVersion, entry.Name()) ||
			strings.Contains(previewVersion, entry.Name())

		versions = append(versions, versionInfo{
			name:       entry.Name(),
			accessTime: info.ModTime(),
			protected:  protected,
		})
	}

	// Sort by access time (newest first)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].accessTime.After(versions[j].accessTime)
	})

	// Count unprotected versions
	unprotectedCount := 0
	for _, v := range versions {
		if !v.protected {
			unprotectedCount++
		}
	}

	// Remove excess old versions
	if unprotectedCount > m.maxVersionsPerSite {
		toRemove := unprotectedCount - m.maxVersionsPerSite
		removed := 0

		// Start from oldest (end of sorted list)
		for i := len(versions) - 1; i >= 0 && removed < toRemove; i-- {
			if !versions[i].protected {
				versionPath := filepath.Join(artifactsDir, versions[i].name)
				if err := os.RemoveAll(versionPath); err != nil {
					fmt.Printf("Warning: failed to remove old version %s: %v\n", versions[i].name, err)
				} else {
					removed++
				}
			}
		}
	}

	return nil
}

// RemoveSite removes all site data
func (m *Manager) RemoveSite(fqdn string) error {
	sitePath := m.GetSitePath(fqdn)
	return os.RemoveAll(sitePath)
}

func (m *Manager) unpack(archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		target := filepath.Join(destDir, header.Name)

		// Prevent path traversal
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to copy file content: %w", err)
			}
			outFile.Close()
		}
	}

	return nil
}

func (m *Manager) extractDomain(fqdn string) string {
	// Extract domain from FQDN
	// blog.example.com -> example.com
	parts := strings.Split(fqdn, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return fqdn
}

func (m *Manager) getSymlinkTarget(linkPath string) (string, error) {
	target, err := os.Readlink(linkPath)
	if err != nil {
		return "", err
	}
	return target, nil
}
