package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CheckPath rejects symlinks in every existing path component. Inputs must stay
// quiescent during compilation; this is not protection against hostile mutation.
func CheckPath(name string, allowMissing bool) error {
	// Inspect before cleaning: link/../file must not hide a traversed symlink.
	absolute := name
	if !filepath.IsAbs(name) {
		cwd, err := Absolute(".")
		if err != nil {
			return err
		}
		absolute = cwd + string(filepath.Separator) + name
	}
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(absolute, current), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) && allowMissing {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink not allowed: %s", current)
		}
	}
	return nil
}

// Absolute resolves the inherited working-directory alias, but not symlinks
// supplied inside an explicit input/output path.
func Absolute(name string) (string, error) {
	if filepath.IsAbs(name) {
		return filepath.Clean(name), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, name), nil
}
func CheckTree(root string) error {
	if err := CheckPath(root, false); err != nil {
		return err
	}
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", root)
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular input: %s", path)
		}
		return nil
	})
}
func Within(root, name string) (string, error) {
	if name == "" || name == "." || !filepath.IsLocal(name) || strings.ContainsAny(name, "\\:") {
		return "", fmt.Errorf("invalid relative path %q", name)
	}
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		if part == ".." || part == "." || part == "" {
			return "", fmt.Errorf("invalid relative path %q", name)
		}
	}
	return filepath.Join(root, name), nil
}
func Overlap(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}
