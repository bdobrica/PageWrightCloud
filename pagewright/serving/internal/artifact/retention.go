package artifact

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const retentionGrace = time.Minute

// Shared with the serving protocol, so retention fails closed on unknown schema.
type DeploymentReceipt struct {
	SiteID   string `json:"site_id"`
	Sequence int64  `json:"sequence"`
	FQDN     string `json:"fqdn"`
	Version  string `json:"version"`
	Target   string `json:"target"`
	Status   string `json:"status"`
}

func realDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("expected real directory: %s", path)
	}
	return nil
}

// The configured root is trusted; descendants may not redirect writes/deletion
// through symlinks. The single-writer contract excludes external path mutation.
func (m *Manager) checkSitePath(fqdn string) error {
	root, err := filepath.Abs(m.wwwRoot)
	if err != nil {
		return err
	}
	if err = realDirectory(root); err != nil {
		return err
	}
	path, err := filepath.Abs(m.GetSitePath(fqdn))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("site escapes root")
	}
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		root = filepath.Join(root, part)
		if err := realDirectory(root); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return err
		}
	}
	artifacts := filepath.Join(path, "artifacts")
	if err := realDirectory(artifacts); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
func activeVersion(path string) (string, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return "", fmt.Errorf("unrecognized active pointer")
	}
	target, err := os.Readlink(path)
	if err != nil {
		return "", err
	}
	parts := strings.Split(target, "/")
	if len(parts) != 3 || parts[0] != "artifacts" || parts[2] != "public" || !safeIdentifier(parts[1]) {
		return "", fmt.Errorf("noncanonical active pointer")
	}
	return parts[1], nil
}
func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func syncTree(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected staged symlink")
		}
		if info.IsDir() {
			return syncDirectory(path)
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		return f.Sync()
	})
}

func (m *Manager) cleanupOldVersions(fqdn string, extra []string) error {
	if !safeIdentifier(fqdn) || m.maxVersionsPerSite < 0 {
		return fmt.Errorf("invalid retention parameters")
	}
	if err := m.checkSitePath(fqdn); err != nil {
		return err
	}
	root := m.GetSitePath(fqdn)
	dir := filepath.Join(root, "artifacts")
	protected := map[string]bool{}
	for _, v := range extra {
		if !safeIdentifier(v) {
			return fmt.Errorf("invalid protected version")
		}
		protected[v] = true
	}
	for _, name := range []string{"public", "preview"} {
		v, err := activeVersion(filepath.Join(root, name))
		if err != nil {
			return err
		}
		if v != "" {
			if err := realDirectory(m.GetArtifactPath(fqdn, v)); err != nil {
				return err
			}
			if err := realDirectory(filepath.Join(m.GetArtifactPath(fqdn, v), "public")); err != nil {
				return err
			}
			protected[v] = true
		}
	}
	path := filepath.Join(root, ".deployment.json")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("invalid receipt file")
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		decoder := json.NewDecoder(io.LimitReader(f, 4097))
		decoder.DisallowUnknownFields()
		var d DeploymentReceipt
		if decoder.Decode(&d) != nil || decoder.Decode(new(any)) != io.EOF || d.FQDN != fqdn || !safeIdentifier(d.SiteID) || !safeIdentifier(d.Version) || d.Sequence <= 0 || (d.Target != "live" && d.Target != "preview") || (d.Status != "pending" && d.Status != "activating" && d.Status != "completed" && d.Status != "failed") {
			return fmt.Errorf("unrecognized deployment receipt")
		}
		// Also retain terminal receipt versions until superseded; retries must find
		// the same evidence even when gateway confirmation was lost.
		protected[d.Version] = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := realDirectory(dir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	type version struct {
		name  string
		stamp time.Time
	}
	versions := []version{}
	protectedCount := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !safeIdentifier(entry.Name()) || !entry.IsDir() {
			return fmt.Errorf("unrecognized artifact entry")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if protected[entry.Name()] || time.Since(info.ModTime()) < retentionGrace {
			protectedCount++
			continue
		}
		versions = append(versions, version{entry.Name(), info.ModTime()})
	}
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].stamp.Equal(versions[j].stamp) {
			return versions[i].name < versions[j].name
		}
		return versions[i].stamp.After(versions[j].stamp)
	})
	keep := m.maxVersionsPerSite - protectedCount
	if keep < 0 {
		keep = 0
	}
	if keep > len(versions) {
		keep = len(versions)
	}
	for _, v := range versions[keep:] {
		if err := os.RemoveAll(filepath.Join(dir, v.name)); err != nil {
			return err
		}
	}
	return syncDirectory(dir)
}
