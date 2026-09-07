package artifact

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Manager struct {
	mu sync.Mutex
	// Optional filesystem seam for deterministic rename-failure tests.
	renameLink         func(string, string) error
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
	m.mu.Lock()
	defer m.mu.Unlock()
	if !safeIdentifier(fqdn) || !safeIdentifier(version) || len(version) > 200 {
		return fmt.Errorf("invalid site or version")
	}
	if err := m.checkSitePath(fqdn); err != nil {
		return err
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
	if info, err := os.Lstat(dest); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("artifact version is not a real directory")
		}
		prior, err := os.ReadFile(filepath.Join(dest, ".archive-sha256"))
		if err == nil && string(prior) == digest {
			return syncDirectory(filepath.Dir(dest))
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
	if err := syncTree(stage); err != nil {
		return err
	}
	if err := os.Rename(stage, dest); err != nil {
		// A concurrent identical deployment may have published while we validated.
		prior, readErr := os.ReadFile(filepath.Join(dest, ".archive-sha256"))
		if readErr == nil && string(prior) == digest {
			return syncDirectory(filepath.Dir(dest))
		}
		return err
	}
	return syncDirectory(filepath.Dir(dest))
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
	m.mu.Lock()
	defer m.mu.Unlock()
	if !safeIdentifier(fqdn) || !safeIdentifier(version) || len(version) > 200 {
		return fmt.Errorf("invalid site or version")
	}
	if err := m.checkSitePath(fqdn); err != nil {
		return err
	}
	if err := realDirectory(filepath.Join(m.GetSitePath(fqdn), "artifacts")); err != nil {
		return err
	}
	sitePath := m.GetSitePath(fqdn)
	artifactPath := m.GetArtifactPath(fqdn, version)

	// Verify artifact exists
	publicPath := filepath.Join(artifactPath, "public")
	if err := realDirectory(artifactPath); err != nil {
		return err
	}
	if err := realDirectory(publicPath); err != nil {
		return fmt.Errorf("artifact public directory not found: %w", err)
	}

	// Determine symlink name
	linkName := "public"
	if isPreview {
		linkName = "preview"
	}

	linkPath := filepath.Join(sitePath, linkName)

	// Validate without unlinking anything. Unknown regular files/directories or
	// noncanonical pointers require operator review, not destructive replacement.
	old, err := activeVersion(linkPath)
	if err != nil {
		return err
	}
	// Delay cache eviction of recently selected/retired output for in-flight reads.
	for _, v := range []string{old, version} {
		if v == "" {
			continue
		}
		path := m.GetArtifactPath(fqdn, v)
		if err := realDirectory(path); err != nil {
			return err
		}
		now := time.Now()
		if err := os.Chtimes(path, now, now); err != nil {
			return err
		}
	}
	relTarget := filepath.Join("artifacts", version, "public")
	stage, err := os.MkdirTemp(sitePath, ".activate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	tmpLink := filepath.Join(stage, "link")
	if err := os.Symlink(relTarget, tmpLink); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}
	rename := m.renameLink
	if rename == nil {
		rename = os.Rename
	}
	if err := rename(tmpLink, linkPath); err != nil {
		return fmt.Errorf("failed to replace symlink: %w", err)
	}
	// A sync error after rename is uncertain, not evidence that activation failed.
	return syncDirectory(sitePath)
}

// CleanupOldVersions removes old artifact versions, keeping max configured
func (m *Manager) CleanupOldVersions(fqdn string, protected ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cleanupOldVersions(fqdn, protected)
}

// RemoveSite never deletes active output or durable sequence evidence. The
// optional infrastructure step runs under the same guard before removing files.
func (m *Manager) RemoveSite(fqdn string, beforeRemove ...func() error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !safeIdentifier(fqdn) {
		return fmt.Errorf("invalid site")
	}
	if err := m.checkSitePath(fqdn); err != nil {
		return err
	}
	sitePath := m.GetSitePath(fqdn)
	for _, name := range []string{"public", "preview", ".deployment.json"} {
		if _, err := os.Lstat(filepath.Join(sitePath, name)); !os.IsNotExist(err) {
			return fmt.Errorf("site has active output or deployment evidence")
		}
	}
	for _, before := range beforeRemove {
		if err := before(); err != nil {
			return err
		}
	}
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
