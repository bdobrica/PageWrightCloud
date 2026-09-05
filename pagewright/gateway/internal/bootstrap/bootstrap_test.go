package bootstrap

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"testing"
	"time"
)

func TestDeterministicStarterSource(t *testing.T) {
	created := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	a, m, l, err := Generate("site", created)
	if err != nil {
		t.Fatal(err)
	}
	a2, m2, l2, err := Generate("site", created)
	if err != nil || !bytes.Equal(a, a2) || !bytes.Equal(m, m2) || !bytes.Equal(l, l2) {
		t.Fatal("bootstrap is not deterministic")
	}
	gz, err := gzip.NewReader(bytes.NewReader(a))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag != tar.TypeReg {
			t.Fatal("unexpected archive type")
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		files[header.Name] = data
	}
	if _, err := io.Copy(io.Discard, gz); err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || len(files["content/home/index.md"]) == 0 {
		t.Fatalf("unexpected file set: %v", files)
	}
	var config struct {
		SiteName string `json:"site_name"`
		Lang     string `json:"lang"`
	}
	if json.Unmarshal(files["content/site.json"], &config) != nil || config.SiteName == "" || config.Lang == "" {
		t.Fatal("invalid site config")
	}
	if bytes.Contains(a, []byte(".codex")) {
		t.Fatal("unexpected instructions")
	}
}
