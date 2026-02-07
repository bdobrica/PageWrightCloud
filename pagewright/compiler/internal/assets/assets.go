package assets

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/types"
)

// CopyThemeAssets copies theme assets to the output directory
func CopyThemeAssets(themeDir, outputDir string) error {
	srcAssetsDir := filepath.Join(themeDir, "src", "assets")
	destAssetsDir := filepath.Join(outputDir, "assets")

	// Check if source assets directory exists
	if _, err := os.Stat(srcAssetsDir); os.IsNotExist(err) {
		// No assets directory, skip
		return nil
	}

	// Copy recursively
	return copyDir(srcAssetsDir, destAssetsDir)
}

// CopyPageAssets copies per-page assets to the output directory
func CopyPageAssets(page *types.Page, outputDir string) error {
	if page.AssetsDir == "" {
		return nil // no assets for this page
	}

	// Create destination path: /assets/pages/{page-id}/
	destDir := filepath.Join(outputDir, "assets", "pages", page.ID)

	return copyDir(page.AssetsDir, destDir)
}

// copyDir recursively copies a directory
func copyDir(src, dest string) error {
	// Get source directory info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Create destination directory
	if err := os.MkdirAll(dest, srcInfo.Mode()); err != nil {
		return err
	}

	// Read source directory
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			// Recursively copy subdirectory
			if err := copyDir(srcPath, destPath); err != nil {
				return err
			}
		} else {
			// Copy file
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file
func copyFile(src, dest string) error {
	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Get source file info
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// Create destination file
	destFile, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Copy contents
	if _, err := io.Copy(destFile, srcFile); err != nil {
		return err
	}

	return nil
}

// WriteFile atomically writes a file (write to temp, then rename)
func WriteFile(path string, data []byte) error {
	// Create directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Write to temporary file
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Rename to final path
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath) // cleanup on error
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
