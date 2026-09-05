package artifact

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackAndUnpack(t *testing.T) {
	// Create temporary directories
	srcDir, err := os.MkdirTemp("", "pack-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(srcDir)

	destDir, err := os.MkdirTemp("", "unpack-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(destDir)

	// Create test files in source
	testFiles := map[string]string{
		"content/site.json":      `{"site_name":"Test"}`,
		"content/home/index.md":  "# Hello World",
		"theme/styles.css":       "body { margin: 0; }",
		"public/index.html":      "<html></html>",
		".codex/instructions.md": "# Instructions",
		".env":                   "API_TOKEN=private",
		"execution.log":          "private log",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(srcDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644))
	}

	// Pack
	archivePath := filepath.Join(t.TempDir(), "test-archive.tar.gz")

	err = Pack(srcDir, archivePath)
	require.NoError(t, err)

	// Verify archive exists
	_, err = os.Stat(archivePath)
	require.NoError(t, err)

	// Unpack
	err = Unpack(archivePath, destDir)
	require.NoError(t, err)

	// Verify files
	for path, expectedContent := range testFiles {
		fullPath := filepath.Join(destDir, path)
		if path == "theme/styles.css" || path == ".codex/instructions.md" || path == ".env" || path == "execution.log" {
			_, err := os.Stat(fullPath)
			require.True(t, os.IsNotExist(err))
			continue
		}
		content, err := os.ReadFile(fullPath)
		require.NoError(t, err)
		assert.Equal(t, expectedContent, string(content))
	}
	// Repack a later edit without losing the existing editable source.
	require.NoError(t, os.WriteFile(filepath.Join(destDir, "content", "home", "index.md"), []byte("# Second edit"), 0644))
	second := filepath.Join(t.TempDir(), "second.tar.gz")
	require.NoError(t, Pack(destDir, second))
	next := filepath.Join(t.TempDir(), "next")
	require.NoError(t, Unpack(second, next))
	data, err := os.ReadFile(filepath.Join(next, "content", "home", "index.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Second edit", string(data))
	data, err = os.ReadFile(filepath.Join(next, "content", "site.json"))
	require.NoError(t, err)
	assert.Equal(t, testFiles["content/site.json"], string(data))
}

func TestPackRejectsUnsafeOutputWithoutReplacingArchive(t *testing.T) {
	for _, scenario := range []string{"secret", "symlink", "missing-home"} {
		t.Run(scenario, func(t *testing.T) {
			src := t.TempDir()
			seedSource(t, src)
			archive := filepath.Join(t.TempDir(), "existing.tar.gz")
			require.NoError(t, Pack(src, archive))
			original, err := os.ReadFile(archive)
			require.NoError(t, err)
			switch scenario {
			case "secret":
				require.NoError(t, os.WriteFile(filepath.Join(src, "content", ".env"), []byte("token"), 0644))
			case "symlink":
				require.NoError(t, os.Symlink("/etc/passwd", filepath.Join(src, "content", "linked")))
			case "missing-home":
				require.NoError(t, os.Remove(filepath.Join(src, "content", "home", "index.md")))
			}
			require.Error(t, Pack(src, archive))
			current, err := os.ReadFile(archive)
			require.NoError(t, err)
			assert.Equal(t, original, current)
		})
	}
}

func TestUnpackFailureLeavesWorkspaceUntouched(t *testing.T) {
	entries := append(validEntries(), testEntry{name: "public/.env", data: "secret"})
	archive := writeTestArchive(t, entries)
	dest := t.TempDir()
	require.Error(t, Unpack(archive, dest))
	files, err := os.ReadDir(dest)
	require.NoError(t, err)
	require.Empty(t, files)
	good := writeTestArchive(t, validEntries())
	require.NoError(t, os.WriteFile(filepath.Join(dest, "existing"), []byte("keep"), 0644))
	require.Error(t, Unpack(good, dest))
	data, err := os.ReadFile(filepath.Join(dest, "existing"))
	require.NoError(t, err)
	assert.Equal(t, "keep", string(data))
}

func TestUnpackRejectsDamagedGzipTrailer(t *testing.T) {
	src := t.TempDir()
	seedSource(t, src)
	archive := filepath.Join(t.TempDir(), "archive.tar.gz")
	require.NoError(t, Pack(src, archive))
	original, err := os.ReadFile(archive)
	require.NoError(t, err)
	for _, mode := range []string{"truncated", "checksum"} {
		t.Run(mode, func(t *testing.T) {
			broken := append([]byte(nil), original...)
			if mode == "truncated" {
				broken = broken[:len(broken)-4]
			} else {
				broken[len(broken)-8] ^= 0xff
			}
			path := filepath.Join(t.TempDir(), "broken.tar.gz")
			require.NoError(t, os.WriteFile(path, broken, 0600))
			require.Error(t, Unpack(path, t.TempDir()))
		})
	}
}

type footerFailureWriter struct{ writes int }

func (w *footerFailureWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes > 1 {
		return 0, errors.New("simulated footer write failure")
	}
	return len(p), nil
}

func TestPackReportsFooterWriteFailure(t *testing.T) {
	// Empty tar output is buffered in gzip until Close; only its header succeeds.
	src := t.TempDir()
	seedSource(t, src)
	err := packToWriter(src, &footerFailureWriter{})
	require.ErrorContains(t, err, "simulated footer write failure")
}

func TestPatchInstructions(t *testing.T) {
	// Create temporary directories
	siteDir, err := os.MkdirTemp("", "site-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(siteDir)

	instructionsFile, err := os.CreateTemp("", "instructions-*.md")
	require.NoError(t, err)
	defer os.Remove(instructionsFile.Name())

	newInstructions := "# New Instructions\nFollow these rules."
	_, err = instructionsFile.WriteString(newInstructions)
	require.NoError(t, err)
	instructionsFile.Close()

	// Patch instructions
	err = PatchInstructions(siteDir, instructionsFile.Name())
	require.NoError(t, err)

	// Verify patched file
	patchedPath := filepath.Join(siteDir, ".codex", "instructions.md")
	content, err := os.ReadFile(patchedPath)
	require.NoError(t, err)
	assert.Equal(t, newInstructions, string(content))
}

func TestGetFileCount(t *testing.T) {
	dir, err := os.MkdirTemp("", "count-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// Create some files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("test"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "subdir", "file2.txt"), []byte("test"), 0644))

	count, err := GetFileCount(dir)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestGetTotalSize(t *testing.T) {
	dir, err := os.MkdirTemp("", "size-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// Create files with known sizes
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("12345"), 0644))      // 5 bytes
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("1234567890"), 0644)) // 10 bytes

	size, err := GetTotalSize(dir)
	require.NoError(t, err)
	assert.Equal(t, int64(15), size)
}

func seedSource(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "content", "home"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "content", "site.json"), []byte(`{"site_name":"Test"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "content", "home", "index.md"), []byte("# Test"), 0644))
}
