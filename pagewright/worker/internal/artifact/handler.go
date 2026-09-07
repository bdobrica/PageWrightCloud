package artifact

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Unpack validates in isolation before publishing a fresh worker workspace.
func Unpack(archivePath, destDir string) error {
	if info, err := os.Lstat(destDir); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("destination must be a directory")
		}
		entries, err := os.ReadDir(destDir)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("destination must be empty")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destDir), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(destDir), ".unpack-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if _, err := readArchive(archivePath, stage, false); err != nil {
		return err
	}
	// os.Remove removes only an empty directory; never clear existing contents.
	if err := os.Remove(destDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(stage, destDir)
}

// Pack includes only editable source, public output and generated layout metadata.
func Pack(srcDir, archivePath string) error {
	out, err := os.CreateTemp(filepath.Dir(archivePath), ".archive-")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	defer out.Close()
	if err := packToWriter(srcDir, out); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if _, err := readArchive(out.Name(), "", false); err != nil {
		return err
	}
	return os.Rename(out.Name(), archivePath)
}

func packToWriter(srcDir string, out io.Writer) error {
	manifest := LayoutManifest{SchemaVersion: 1, Kind: "source", ThemeID: "starter"}
	if _, err := os.Lstat(filepath.Join(srcDir, "public")); err == nil {
		manifest.Kind = "compiled"
	} else if !os.IsNotExist(err) {
		return err
	}
	gzw := gzip.NewWriter(&archiveWriter{writer: out, remaining: maxArchiveBytes})
	defer gzw.Close()
	tw := tar.NewWriter(gzw)
	defer tw.Close()
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0644, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	if _, err := tw.Write(data); err != nil {
		return err
	}
	entries := 1
	var expanded int64 = 2048
	for _, root := range []string{"content", "public"} {
		base := filepath.Join(srcDir, root)
		if _, err := os.Lstat(base); os.IsNotExist(err) && root == "public" {
			continue
		}
		err := filepath.Walk(base, func(filePath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(srcDir, filePath)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if !allowedPath(rel) || (!info.IsDir() && !info.Mode().IsRegular()) {
				return fmt.Errorf("disallowed workspace entry %q", rel)
			}
			entries++
			expanded += 1024
			if !info.IsDir() {
				expanded += info.Size()
			}
			if entries > maxEntries || expanded > maxExpandedBytes || (!info.IsDir() && info.Size() > maxFileBytes) {
				return fmt.Errorf("workspace exceeds archive limits")
			}
			h := &tar.Header{Name: rel, Mode: 0644, Size: info.Size(), Typeflag: tar.TypeReg}
			if info.IsDir() {
				h.Mode = 0755
				h.Size = 0
				h.Typeflag = tar.TypeDir
			}
			if err := tw.WriteHeader(h); err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			f, err := os.Open(filePath)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(tw, f)
			closeErr := f.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		})
		if err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("failed to finish tar archive: %w", err)
	}
	if err := gzw.Close(); err != nil {
		return fmt.Errorf("failed to finish gzip archive: %w", err)
	}
	return nil
}

// Inspect validates an archive without extracting it and returns its layout/counts.
func Inspect(archivePath string) (LayoutManifest, error) { return readArchive(archivePath, "", false) }

// PatchInstructions replaces .codex/instructions.md in the unpacked site.
func PatchInstructions(siteDir, instructionsPath string) error {
	targetPath := filepath.Join(siteDir, ".codex", "instructions.md")

	// Read source instructions
	content, err := os.ReadFile(instructionsPath)
	if err != nil {
		return fmt.Errorf("failed to read instructions: %w", err)
	}

	// Ensure .codex directory exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("failed to create .codex directory: %w", err)
	}

	// Write instructions
	if err := os.WriteFile(targetPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write instructions: %w", err)
	}

	return nil
}

// GetFileCount returns the number of files in a directory
func GetFileCount(dir string) (int, error) {
	count := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}

// GetTotalSize returns the total size of all files in a directory
func GetTotalSize(dir string) (int64, error) {
	var size int64
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
