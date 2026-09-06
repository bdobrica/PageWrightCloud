//go:build integration

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/codex"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/config"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/server"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/storage"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/types"
)

// Subprocess entrypoint called by the gateway round-trip test with the actual
// manager launch snapshot. No production flag or alternative worker pipeline.
func TestDeterministicWorkerEntrypoint(t *testing.T) {
	payload := os.Getenv("TEST_DETERMINISTIC_JOB")
	if payload == "" {
		return
	}
	var job types.Job
	if err := json.Unmarshal([]byte(payload), &job); err != nil {
		t.Fatal(err)
	}
	if err := job.ValidateLaunch(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{WorkDir: t.TempDir(), ManagerURL: os.Getenv("TEST_MANAGER_URL"), StorageURL: os.Getenv("TEST_STORAGE_URL"), ThemePath: "/workspace/pagewright/themes/starter"}
	cfg.InstructionsPath = filepath.Join(cfg.WorkDir, "instructions.md")
	if err := os.WriteFile(cfg.InstructionsPath, []byte("Test-only deterministic executor.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	executor := codex.NewExecutor("/usr/local/bin/deterministic-executor", filepath.Join(cfg.WorkDir, "site"), "test-only-key", "")
	if err := runJob(cfg, &job, storage.NewClient(cfg.StorageURL), executor, server.NewServer(0, executor)); err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(cfg.StorageURL + "/sites/" + job.SiteID + "/artifacts/" + job.TargetVersion + "/manifest")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("manifest status: %d", response.StatusCode)
	}
	var manifest types.Manifest
	if err := json.NewDecoder(response.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	wantChanged := []string{"content/home/index.md"}
	if job.Prompt == "Set the homepage heading to Deterministic M1 round trip." {
		wantChanged = []string{"content/guide/nested/assets/pixel.svg", "content/guide/nested/index.md", "content/home/index.md"}
	}
	if !manifest.ChecksPassed || manifest.BrowserChecksPerformed || len(manifest.ValidationChecks) != 4 || manifest.CompilerVersion != "0.1.0" || manifest.ThemeVersion != "1.0.0" || !reflect.DeepEqual(manifest.FilesChanged, wantChanged) {
		t.Fatalf("untruthful compilation manifest: %+v", manifest)
	}
}
