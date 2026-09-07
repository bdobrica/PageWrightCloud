package artifact

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testEntry struct {
	name, data string
	kind       byte
	size       int64
}

func validEntries() []testEntry {
	return []testEntry{
		{name: "manifest.json", data: `{"schema_version":1,"kind":"compiled","theme_id":"starter"}`},
		{name: "content/site.json", data: `{"site_name":"Test"}`},
		{name: "content/home/index.md", data: "# Editable source"},
		{name: "public/index.html", data: "<h1>Public</h1>"},
	}
}
func writeTestArchive(t *testing.T, entries []testEntry) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "test.tar.gz")
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		kind := entry.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		size := int64(len(entry.data))
		if entry.size != 0 {
			size = entry.size
		}
		h := &tar.Header{Name: entry.name, Size: size, Mode: 07777, Typeflag: kind}
		if kind == tar.TypeSymlink || kind == tar.TypeLink {
			h.Linkname = "../../outside"
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := io.WriteString(tw, entry.data); err != nil {
				t.Fatal(err)
			}
		}
		if entry.size != 0 {
			break
		} // Deliberately truncated oversized entry.
	}
	_ = tw.Close()
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return name
}
func TestArchiveLayoutRejections(t *testing.T) {
	cases := map[string][]testEntry{}
	for _, name := range []string{"../escape", "/public/escape", "public/../escape", "public//asset", "public/./asset", "public\\escape", "public/a:b", "public/a\nb", "other/file", "content/.env", "public/.env", "public/.codex/instructions.md", "public/instructions.md", "public/server.log", "public/private.key", "content/secret.pem", "public/content/source.txt", "public/source/site.txt", "public/site.json", "public/page.md", "public/page.mdx", "public/manifest.json", "logs/output.txt", "public/secrets/token"} {
		cases[name] = append(validEntries(), testEntry{name: name, data: "secret"})
	}
	cases["symlink"] = append(validEntries(), testEntry{name: "public/link", kind: tar.TypeSymlink})
	cases["hardlink"] = append(validEntries(), testEntry{name: "public/link", kind: tar.TypeLink})
	cases["fifo"] = append(validEntries(), testEntry{name: "public/pipe", kind: tar.TypeFifo})
	cases["character-device"] = append(validEntries(), testEntry{name: "public/device", kind: tar.TypeChar})
	cases["block-device"] = append(validEntries(), testEntry{name: "public/device", kind: tar.TypeBlock})
	cases["duplicate"] = append(validEntries(), testEntry{name: "public/index.html", data: "overwrite"})
	cases["parent-file"] = append(validEntries(), testEntry{name: "public/assets", data: "file"}, testEntry{name: "public/assets/a", data: "child"})
	cases["child-first"] = append(validEntries(), testEntry{name: "public/assets/a", data: "child"}, testEntry{name: "public/assets", data: "file"})
	cases["oversized"] = append(validEntries(), testEntry{name: "public/large", size: maxFileBytes + 1})
	cases["missing-manifest"] = validEntries()[1:]
	cases["missing-source"] = []testEntry{validEntries()[0], validEntries()[3]}
	for _, bad := range []string{`{}`, `{"schema_version":2,"kind":"compiled","theme_id":"starter"}`, `{"schema_version":1,"kind":"source","theme_id":"starter"}`, `{"schema_version":1,"kind":"compiled","theme_id":"unknown"}`, `{"schema_version":1,"kind":"compiled","theme_id":"starter","prompt":"private"}`, `{} {}`} {
		entries := validEntries()
		entries[0].data = bad
		cases["manifest-"+bad] = entries
	}
	entries := validEntries()
	entries[1].data = `{"title":"wrong"}`
	cases["invalid-config"] = entries
	entries = validEntries()
	entries[2].data = ""
	cases["empty-home"] = entries
	entries = validEntries()
	entries[3].data = ""
	cases["empty-index"] = entries
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			archive := writeTestArchive(t, entries)
			for _, publicOnly := range []bool{false, true} {
				if _, err := readArchive(archive, t.TempDir(), publicOnly); err == nil {
					t.Fatal("unsafe archive accepted")
				}
			}
		})
	}
}
func TestArchiveSourceAndPublicSeparation(t *testing.T) {
	archive := writeTestArchive(t, validEntries())
	for _, publicOnly := range []bool{false, true} {
		stage := t.TempDir()
		layout, err := readArchive(archive, stage, publicOnly)
		if err != nil {
			t.Fatal(err)
		}
		if layout.Kind != "compiled" || layout.FileCount != 4 || layout.TotalSize == 0 {
			t.Fatalf("wrong metadata: %+v", layout)
		}
		for _, entry := range validEntries() {
			data, err := os.ReadFile(filepath.Join(stage, entry.name))
			if publicOnly && entry.name != "public/index.html" {
				if !os.IsNotExist(err) {
					t.Fatal("private file extracted")
				}
				continue
			}
			if err != nil || string(data) != entry.data {
				t.Fatalf("missing/changed %s: %v", entry.name, err)
			}
			info, err := os.Stat(filepath.Join(stage, entry.name))
			if err != nil || info.Mode().Perm() != 0644 {
				t.Fatal("unsafe permissions")
			}
		}
	}
}
func TestArchiveLimitsAndTrailingData(t *testing.T) {
	t.Run("expanded-padding", func(t *testing.T) {
		archive := writeTestArchive(t, validEntries())
		f, err := os.OpenFile(archive, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		gz, err := gzip.NewWriterLevel(f, gzip.BestSpeed)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.CopyN(gz, zeroReader{}, maxExpandedBytes); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := readArchive(archive, "", false); err == nil || !strings.Contains(err.Error(), "expanded archive too large") {
			t.Fatalf("expansion limit not enforced: %v", err)
		}
	})
	t.Run("compressed", func(t *testing.T) {
		archive := writeTestArchive(t, validEntries())
		if err := os.Truncate(archive, maxArchiveBytes+1); err != nil {
			t.Fatal(err)
		}
		if _, err := readArchive(archive, "", false); err == nil {
			t.Fatal("oversized compressed archive accepted")
		}
	})
	t.Run("entries", func(t *testing.T) {
		entries := validEntries()
		for i := 0; i < maxEntries; i++ {
			entries = append(entries, testEntry{name: fmt.Sprintf("public/assets/%d", i)})
		}
		if _, err := readArchive(writeTestArchive(t, entries), "", false); err == nil {
			t.Fatal("too many entries accepted")
		}
	})
	t.Run("metadata", func(t *testing.T) {
		entries := validEntries()
		entries[1].data = strings.Repeat(" ", (64<<10)+1)
		if _, err := readArchive(writeTestArchive(t, entries), "", false); err == nil {
			t.Fatal("oversized config accepted")
		}
	})
	t.Run("trailing-payload", func(t *testing.T) {
		archive := writeTestArchive(t, validEntries())
		f, err := os.OpenFile(archive, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		gz := gzip.NewWriter(f)
		if _, err := io.WriteString(gz, "hidden payload"); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := readArchive(archive, "", false); err == nil {
			t.Fatal("hidden gzip payload accepted")
		}
	})
}
func TestSourceBootstrapCompatibility(t *testing.T) {
	source := validEntries()[:3]
	source[0].data = `{"schema_version":1,"kind":"source","theme_id":"starter"}`
	for _, entries := range [][]testEntry{source, source[1:]} {
		archive := writeTestArchive(t, entries)
		layout, err := readArchive(archive, t.TempDir(), false)
		if err != nil || layout.Kind != "source" {
			t.Fatalf("bootstrap rejected: %v", err)
		}
		if _, err := readArchive(archive, t.TempDir(), true); err == nil {
			t.Fatal("source bootstrap deployed")
		}
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }
