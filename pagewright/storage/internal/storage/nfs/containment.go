package nfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The storage volume is owned exclusively by the service. Reject existing
// symlinks before any read, mkdir or write; external concurrent mutation of this
// private volume is outside the supported trust model.
func contained(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes storage root")
	}
	current := root
	parts := append([]string{""}, strings.Split(rel, string(os.PathSeparator))...)
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) {
			return fmt.Errorf("unsafe storage path")
		}
	}
	return nil
}
