//go:build integration

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	cfg := &config.Config{WorkDir: t.TempDir(), ManagerURL: os.Getenv("TEST_MANAGER_URL"), StorageURL: os.Getenv("TEST_STORAGE_URL")}
	cfg.InstructionsPath = filepath.Join(cfg.WorkDir, "instructions.md")
	if err := os.WriteFile(cfg.InstructionsPath, []byte("Test-only deterministic executor.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	executor := codex.NewExecutor("/usr/local/bin/deterministic-executor", filepath.Join(cfg.WorkDir, "site"), "", "")
	if err := runJob(cfg, &job, storage.NewClient(cfg.StorageURL), executor, server.NewServer(0, executor)); err != nil {
		t.Fatal(err)
	}
}
