package artifact

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCompressedWriterBoundary(t *testing.T) {
	var out bytes.Buffer
	w := &archiveWriter{writer: &out, remaining: 3}
	if _, err := w.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("d")); err == nil {
		t.Fatal("unbounded compressed output")
	}
	if out.String() != "abc" {
		t.Fatal("overflow bytes written")
	}
}

func TestArchiveSymlinkRejectedBeforeExtraction(t *testing.T) {
	archive := writeTestArchive(t, validEntries())
	link := filepath.Join(t.TempDir(), "archive")
	if err := os.Symlink(archive, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readArchive(link, t.TempDir(), false); err == nil {
		t.Fatal("symlink input accepted")
	}
}
