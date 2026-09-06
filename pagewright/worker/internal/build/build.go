package build

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/artifact"
)

const CompilerVersion = "0.1.0"
const ThemeVersion = "1.0.0"
const DefaultCompiler = "/usr/local/bin/pagewrightc"
const DefaultTheme = "/opt/pagewright/themes/starter-1.0.0"

type Result struct {
	Layout       artifact.LayoutManifest
	FilesChanged []string
	Checks       []string
}

// Compile publishes a validated immutable archive of frozen source plus fresh
// compiler output. It never consumes AI-produced public/ or bundled themes.
func Compile(ctx context.Context, site, compiler, theme, archive string, before artifact.Snapshot) (Result, error) {
	var result Result
	after, err := artifact.SnapshotWorkspace(site)
	if err != nil {
		return result, err
	}
	result.FilesChanged, err = artifact.ValidateChanges(before, after)
	if err != nil {
		return result, err
	}
	if compiler == "" {
		compiler = DefaultCompiler
	}
	if theme == "" {
		theme = DefaultTheme
	}
	theme, err = filepath.Abs(theme)
	if err != nil {
		return result, err
	}
	compiler, err = filepath.Abs(compiler)
	if err != nil {
		return result, err
	}
	// Never permit workspace-selected executables or theme directories.
	for _, trusted := range []string{theme, compiler} {
		rel, err := filepath.Rel(site, trusted)
		if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return result, fmt.Errorf("compiler/theme must be outside workspace")
		}
	}
	tokenData, err := os.ReadFile(filepath.Join(theme, "tokens.json"))
	if err != nil {
		return result, err
	}
	var tokens struct {
		Version string `json:"theme_version"`
	}
	if json.Unmarshal(tokenData, &tokens) != nil || tokens.Version != ThemeVersion {
		return result, fmt.Errorf("unsupported trusted theme version")
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	version := exec.CommandContext(bounded, compiler, "version")
	version.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin"}
	versionOutput, err := version.Output()
	if err != nil || strings.TrimSpace(string(versionOutput)) != "pagewrightc version "+CompilerVersion {
		return result, fmt.Errorf("unsupported compiler version")
	}
	stage, err := os.MkdirTemp(filepath.Dir(site), ".compile-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(stage)
	if err := artifact.FreezeContent(site, stage, after); err != nil {
		return result, err
	}
	output := filepath.Join(stage, "public")
	cmd := exec.CommandContext(bounded, compiler, "build", "--theme", theme, "--content", filepath.Join(stage, "content"), "--out", output)
	cmd.Dir = stage
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin"}
	cmd.WaitDelay = 2 * time.Second
	// Compiler diagnostics can contain source; do not forward them to public status.
	if err := cmd.Run(); err != nil {
		return result, fmt.Errorf("trusted compilation failed: %w", err)
	}
	if err := ValidateOutput(output); err != nil {
		return result, fmt.Errorf("compiled output: %w", err)
	}
	if err := artifact.Pack(stage, archive); err != nil {
		return result, err
	}
	result.Layout, err = artifact.Inspect(archive)
	if err != nil {
		return result, err
	}
	if result.Layout.Kind != "compiled" {
		return result, fmt.Errorf("compiler did not produce compiled artifact")
	}
	// Preserve old public until compilation, HTML/assets and archive checks pass.
	public := filepath.Join(site, "public")
	backup := filepath.Join(stage, "previous-public")
	hadOld := false
	if _, err := os.Lstat(public); err == nil {
		if err := os.Rename(public, backup); err != nil {
			return result, err
		}
		hadOld = true
	} else if !os.IsNotExist(err) {
		return result, err
	}
	if err := os.Rename(output, public); err != nil {
		if hadOld {
			if restore := os.Rename(backup, public); restore != nil {
				return result, fmt.Errorf("public replacement and rollback failed: %v; %w", err, restore)
			}
		}
		return result, err
	}
	result.Checks = []string{"allowed_source_changes", "trusted_compiler", "html_and_local_references", "archive_layout"}
	return result, nil
}
