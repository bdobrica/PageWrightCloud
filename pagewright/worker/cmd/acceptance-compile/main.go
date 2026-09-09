// Offline operator compilation. No provider client, callbacks or publication.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/artifact"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/build"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var identity = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$`)

func digest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func edit(site string) error {
	path := filepath.Join(site, "content/home/index.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			if found {
				return fmt.Errorf("expected exactly one homepage H1")
			}
			lines[i] = "# PageWright HTTPS acceptance — version 2 (operator test)"
			found = true
		}
	}
	if !found {
		return fmt.Errorf("homepage H1 missing")
	}
	data = []byte(strings.Join(lines, "\n") + "\n\nDeterministic operator acceptance version; no AI call was made.\n\n[Version-specific asset](/assets/pages/home/m49-operator-version.txt)\n")
	if err = os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	assets := filepath.Join(site, "content/home/assets")
	if err = os.MkdirAll(assets, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(assets, "m49-operator-version.txt"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, err = f.WriteString("PageWright operator acceptance version 2\n")
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func compile(input, out, siteID, base, target string) error {
	if !identity.MatchString(siteID) || !identity.MatchString(base) || !identity.MatchString(target) || !strings.HasPrefix(target, "operator-m49-") || target == base {
		return fmt.Errorf("invalid operator artifact identity")
	}
	baseHash, err := digest(input)
	if err != nil {
		return err
	}
	if err = os.Mkdir(out, 0700); err != nil {
		return err
	}
	workspace, err := os.MkdirTemp(filepath.Dir(out), "acceptance-source-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workspace)
	if err = artifact.Unpack(input, workspace); err != nil {
		return err
	}
	before, err := artifact.SnapshotWorkspace(workspace)
	if err != nil {
		return err
	}
	if err = edit(workspace); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	archive := filepath.Join(out, "artifact.tar.gz")
	result, err := build.Compile(ctx, workspace, build.DefaultCompiler, build.DefaultTheme, archive, before)
	if err != nil {
		return err
	}
	hash, err := digest(archive)
	if err != nil {
		return err
	}
	manifest := map[string]any{
		"site_id": siteID, "build_id": target, "base_build_id": base,
		"created_at": time.Now().UTC(), "provenance": "operator-acceptance-v1", "provider_calls": 0,
		"base_archive_sha256": baseHash, "artifact_sha256": hash,
		"archive_schema_version": result.Layout.SchemaVersion, "kind": result.Layout.Kind,
		"theme_id": result.Layout.ThemeID, "file_count": result.Layout.FileCount, "total_size": result.Layout.TotalSize,
		"compiler_version": build.CompilerVersion, "theme_version": build.ThemeVersion,
		"checks_passed": true, "validation_checks": result.Checks, "files_changed": result.FilesChanged,
		"browser_checks_performed": false, "entrypoints": []string{"index.html"},
		"prompt":          "Operator acceptance: deterministic version 2; no AI job",
		"changes_summary": "Fixed homepage heading and version-specific asset; trusted compilation and static checks only",
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "manifest.json"), data, 0600)
}

func main() {
	input := flag.String("input", "", "base archive")
	out := flag.String("out", "", "new output directory")
	site := flag.String("site", "", "operator-verified site ID")
	base := flag.String("base", "", "committed source version")
	target := flag.String("target", "", "new operator-m49- version")
	flag.Parse()
	if err := compile(*input, *out, *site, *base, *target); err != nil {
		fmt.Fprintln(os.Stderr, "Operator compilation failed:", err)
		os.Exit(1)
	}
	fmt.Println("Operator artifact compiled and validated; no provider calls or publication")
}
