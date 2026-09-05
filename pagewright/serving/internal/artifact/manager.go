package artifact

import (
	"crypto/sha256"
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
	if !safeIdentifier(fqdn) || !safeIdentifier(version) {
		return fmt.Errorf("invalid site or version")
	}
	dest := m.GetArtifactPath(fqdn, version)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".deploy-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := m.unpack(archivePath, stage); err != nil {
		return err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, f)
	f.Close()
	if copyErr != nil {
		return copyErr
	}
	digest := fmt.Sprintf("%x", hash.Sum(nil))
	if _, err := os.Lstat(dest); err == nil {
		prior, err := os.ReadFile(filepath.Join(dest, ".archive-sha256"))
		if err == nil && string(prior) == digest {
			return nil
		}
		return fmt.Errorf("version already exists with different or unverified content")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, ".archive-sha256"), []byte(digest), 0644); err != nil {
		return err
	}
	if err := os.Chmod(stage, 0755); err != nil {
		return err
	}
	if err := os.Rename(stage, dest); err != nil {
		// A concurrent identical deployment may have published while we validated.
		prior, readErr := os.ReadFile(filepath.Join(dest, ".archive-sha256"))
		if readErr == nil && string(prior) == digest {
			return nil
		}
		return err
	}
	return nil
}

func safeIdentifier(value string) bool {
	if value == "" || len(value) > 255 {
		return false
	}
	first := value[0]
	if !(first >= 'a' && first <= 'z' || first >= 'A' && first <= 'Z' || first >= '0' && first <= '9') {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// ActivateVersion creates/updates symlink for public or preview
func (m *Manager) ActivateVersion(fqdn, version string, isPreview bool) error {
	if !safeIdentifier(fqdn) || !safeIdentifier(version) {
		return fmt.Errorf("invalid site or version")
	}
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
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".deploy-") {
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
	_, err := readArchive(archivePath, destDir, true)
	return err
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
