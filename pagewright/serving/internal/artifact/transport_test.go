package artifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestDeployRejectsDamagedGzipTrailer(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "public/index.html", Mode: 0600, Size: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"valid", "corrupt", "truncated"} {
		t.Run(scenario, func(t *testing.T) {
			data := bytes.Clone(archive.Bytes())
			if scenario == "corrupt" {
				data[len(data)-8] ^= 0xff
			}
			if scenario == "truncated" {
				data = data[:len(data)-4]
			}
			path := filepath.Join(t.TempDir(), "archive.tar.gz")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			err := NewManager(t.TempDir(), 10).DeployArtifact("test.example.test", "v1", path)
			if scenario == "valid" && err != nil {
				t.Fatal(err)
			}
			if scenario != "valid" && err == nil {
				t.Fatal("damaged gzip accepted")
			}
		})
	}
}
