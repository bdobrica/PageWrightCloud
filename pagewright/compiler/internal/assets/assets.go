package assets

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/types"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/util"
)

func CopyThemeAssets(themeDir, outputDir string) error {
	if err := util.CheckPath(themeDir, false); err != nil {
		return err
	}
	src := filepath.Join(themeDir, "src", "assets")
	if _, err := os.Lstat(src); os.IsNotExist(err) {
		return nil
	}
	return copyDir(src, filepath.Join(outputDir, "assets"))
}
func CopyPageAssets(page *types.Page, outputDir string) error {
	if page.AssetsDir == "" {
		return nil
	}
	dest, err := util.Within(filepath.Join(outputDir, "assets", "pages"), page.ID)
	if err != nil {
		return err
	}
	return copyDir(page.AssetsDir, dest)
}
func copyDir(src, dest string) error {
	if err := util.CheckTree(src); err != nil {
		return err
	}
	if err := util.CheckPath(dest, true); err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		source, target := filepath.Join(src, entry.Name()), filepath.Join(dest, entry.Name())
		if entry.IsDir() {
			if err := copyDir(source, target); err != nil {
				return err
			}
		} else {
			if err := copyFile(source, target); err != nil {
				return err
			}
		}
	}
	return nil
}
func copyFile(src, dest string) error {
	if err := util.CheckPath(src, false); err != nil {
		return err
	}
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("non-regular asset: %s", src)
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeAtomic(dest, f)
}
func WriteFile(path string, data []byte) error {
	return writeAtomic(path, bytes.NewReader(data))
}
func writeAtomic(path string, reader io.Reader) error {
	if err := util.CheckPath(path, true); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".compiler-file-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if n, err := io.Copy(f, io.LimitReader(reader, (32<<20)+1)); err != nil {
		return err
	} else if n > 32<<20 {
		return fmt.Errorf("compiler output file too large")
	}
	if err := f.Chmod(0644); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
