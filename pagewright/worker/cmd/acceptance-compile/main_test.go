package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixedEditPreservesBodyAndAddsAsset(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "content/home")
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "index.md")
	if err := os.WriteFile(path, []byte("---\ntitle: Existing\n---\n# Version one\n\nKeep this paragraph.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := edit(root); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"title: Existing", "version 2 (operator test)", "Keep this paragraph.", "no AI call", "/assets/pages/home/m49-operator-version.txt"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %q", want)
		}
	}
	if err := edit(root); err == nil {
		t.Fatal("must not overwrite existing test asset")
	}
}

func TestRejectAmbiguousHeading(t *testing.T) {
	for _, body := range []string{"no heading", "# One\n# Two\n"} {
		root := t.TempDir()
		home := filepath.Join(root, "content/home")
		os.MkdirAll(home, 0755)
		path := filepath.Join(home, "index.md")
		os.WriteFile(path, []byte(body), 0644)
		if err := edit(root); err == nil {
			t.Fatal("accepted ambiguous input")
		}
		got, _ := os.ReadFile(path)
		if string(got) != body {
			t.Fatal("changed rejected input")
		}
	}
}

func TestInvalidIdentityAndArchiveNeverEmitReceipt(t *testing.T) {
	for _, target := range []string{"../../bad", "normal-version", "operator-m49-v2"} {
		root := t.TempDir()
		input := filepath.Join(root, "bad.tar.gz")
		os.WriteFile(input, []byte("not an archive"), 0600)
		out := filepath.Join(root, "bundle")
		if err := compile(input, out, "site", "base", target); err == nil {
			t.Fatal("invalid input compiled")
		}
		if _, err := os.Stat(filepath.Join(out, "manifest.json")); !os.IsNotExist(err) {
			t.Fatal("failed compilation emitted receipt")
		}
	}
}
