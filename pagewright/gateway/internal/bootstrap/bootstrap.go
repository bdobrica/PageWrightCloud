package bootstrap

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"io/fs"
	"strings"
	"time"
)

// Seed source for the bundled starter theme. Theme code remains trusted and
// outside the worker-editable archive; compilation/publication are later gates.
//
//go:embed starter/content
var seed embed.FS

const Version = "initial"

func Generate(siteID string, created time.Time) ([]byte, []byte, []byte, error) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	err := fs.WalkDir(seed, "starter/content", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := seed.ReadFile(path)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{Name: strings.TrimPrefix(path, "starter/"), Mode: 0644, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	if err != nil {
		return nil, nil, nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, nil, nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, nil, nil, err
	}
	manifest, err := json.Marshal(map[string]interface{}{
		"site_id": siteID, "build_id": Version, "created_at": created.UTC(),
		"template_id": "starter", "bootstrap_revision": 1, "kind": "source",
		"checks_passed": false, "compiled": false, "file_count": 2,
	})
	return archive.Bytes(), manifest, []byte(`{"content":"Initialized bundled starter source (revision 1); not compiled or published.\n"}`), err
}
