//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/artifact"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/codex"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/config"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/server"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/storage"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/types"
)

// Only the integration test binary has this entrypoint and the source-only CLI
// fixture. Production main, including exit codes and failure reporting, is used.
func TestRunnerProcess(t *testing.T) {
	if os.Getenv("TEST_RUNNER_PROCESS") == "1" {
		main()
	}
}

func runnerFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0700); err != nil {
		t.Fatal(err)
	}
}

func TestActualRunnerFailures(t *testing.T) {
	for _, failure := range []string{"success", "invalid_edit", "invalid_output", "compiler", "artifact", "logs", "manifest", "callback", "lost_ack", "cancel", "crash_before_manifest", "crash_after_manifest"} {
		t.Run(failure, func(t *testing.T) {
			job := contractJob()
			root := t.TempDir()
			source := filepath.Join(root, "source")
			runnerFile(t, filepath.Join(source, "content/site.json"), `{"site_name":"Runner acceptance"}`)
			runnerFile(t, filepath.Join(source, "content/home/index.md"), "# Original\n")
			archive := filepath.Join(root, "source.tar.gz")
			if err := artifact.Pack(source, archive); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(archive)
			if err != nil {
				t.Fatal(err)
			}
			work := filepath.Join(root, "work")
			if err := os.Mkdir(work, 0700); err != nil {
				t.Fatal(err)
			}
			instructions := filepath.Join(root, "instructions.md")
			runnerFile(t, instructions, "Trusted test instructions")
			cli := filepath.Join(root, "executor")
			edit := "printf '# Changed\\n' > content/home/index.md\n"
			switch failure {
			case "invalid_edit":
				edit = "printf 'tampered' > .codex/instructions.md\n"
			case "compiler":
				edit = "printf ':::component MissingComponent\\n:::\\n' > content/home/index.md\n"
			case "invalid_output":
				edit = "printf '# Home\\n\\n![Missing](/assets/missing.png)\\n' > content/home/index.md\n"
			case "cancel":
				edit = "touch executing\nexec sleep 60\n"
			}
			runnerFile(t, cli, "#!/bin/sh\nif [ \"$1\" = sandbox ]; then exit 0; fi\n"+edit+"echo 'SUMMARY: secret-sentinel'\n")
			var mu sync.Mutex
			var stages, outcomes []string
			stored := job
			blocked := make(chan struct{}, 1)
			release := make(chan struct{})
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					if strings.HasPrefix(r.URL.Path, "/jobs/") {
						mu.Lock()
						defer mu.Unlock()
						json.NewEncoder(w).Encode(stored)
					} else {
						w.Header().Set("Content-Type", "application/gzip")
						w.Write(data)
					}
					return
				}
				body, _ := io.ReadAll(r.Body)
				stage := "artifact"
				for _, suffix := range []string{"logs", "manifest", "result"} {
					if strings.HasSuffix(r.URL.Path, "/"+suffix) {
						stage = suffix
					}
				}
				mu.Lock()
				stages = append(stages, stage)
				if stage == "result" {
					var result types.JobResult
					if err := json.Unmarshal(body, &result); err != nil {
						t.Error(err)
					}
					outcomes = append(outcomes, result.Status)
					if result.LockToken != job.LockToken || result.FencingToken != job.FencingToken {
						t.Error("lost fencing identity")
					}
					if failure != "callback" && failure != "crash_after_manifest" {
						stored.Status, stored.Result = result.Status, result.Result
						stored.ManifestPath, stored.ErrorMessage = result.ManifestPath, result.ErrorMessage
					}
				}
				mu.Unlock()
				if stage == "logs" && strings.Contains(string(body), "secret-sentinel") {
					t.Error("raw diagnostic leaked")
				}
				if stage == "result" && failure == "crash_after_manifest" || stage == "logs" && failure == "crash_before_manifest" {
					select {
					case blocked <- struct{}{}:
					default:
					}
					select {
					case <-release:
					case <-r.Context().Done():
					}
					return
				}
				if stage == failure || stage == "result" && (failure == "callback" || failure == "lost_ack") {
					w.WriteHeader(503)
					return
				}
				if stage == "result" {
					mu.Lock()
					defer mu.Unlock()
					json.NewEncoder(w).Encode(stored)
				} else {
					w.WriteHeader(201)
				}
			}))
			defer api.Close()
			defer close(release)
			payload, _ := json.Marshal(job)
			cmd := exec.Command(os.Args[0], "-test.run=^TestRunnerProcess$")
			cmd.WaitDelay = 2 * time.Second
			cmd.Env = append(os.Environ(), "TEST_RUNNER_PROCESS=1", "PAGEWRIGHT_JOB="+string(payload), "PAGEWRIGHT_WORK_DIR="+work,
				"PAGEWRIGHT_WORKER_PORT=0", "PAGEWRIGHT_LLM_KEY=test-only-key", "PAGEWRIGHT_CODEX_BINARY="+cli,
				"PAGEWRIGHT_COMPILER_BINARY=/usr/local/bin/pagewrightc", "PAGEWRIGHT_THEME_PATH=/workspace/pagewright/themes/starter",
				"PAGEWRIGHT_INSTRUCTIONS_PATH="+instructions, "PAGEWRIGHT_STORAGE_URL="+api.URL, "PAGEWRIGHT_MANAGER_URL="+api.URL)
			var output bytes.Buffer
			cmd.Stdout, cmd.Stderr = &output, &output
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			finished := false
			defer func() {
				if !finished {
					// Let the runner cancel its separate executor process group first.
					_ = cmd.Process.Signal(syscall.SIGTERM)
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						_ = cmd.Process.Kill()
						<-done
					}
				}
			}()
			if failure == "cancel" {
				deadline := time.Now().Add(10 * time.Second)
				for {
					if _, err := os.Stat(filepath.Join(work, "site/executing")); err == nil {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("executor never started")
					}
					time.Sleep(10 * time.Millisecond)
				}
				if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
					t.Fatal(err)
				}
			}
			if strings.HasPrefix(failure, "crash_") {
				select {
				case <-blocked:
				case <-time.After(10 * time.Second):
					t.Fatal("runner never reached crash boundary")
				}
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err = <-done:
				finished = true
			case <-time.After(20 * time.Second):
				t.Fatal("runner did not terminate")
			}
			wantSuccess := failure == "success" || failure == "lost_ack"
			if (err == nil) != wantSuccess {
				t.Fatalf("exit %v: %s", err, &output)
			}
			if !wantSuccess {
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Fatalf("missing process exit status: %v", err)
				}
				if strings.HasPrefix(failure, "crash_") {
					status, ok := exit.Sys().(syscall.WaitStatus)
					if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
						t.Fatalf("not killed at crash boundary: %v", err)
					}
				} else if exit.ExitCode() != 1 {
					t.Fatalf("unexpected failure exit (including race detector): %v; %s", err, &output)
				}
			}
			if strings.Contains(output.String(), "secret-sentinel") {
				t.Fatal("raw execution summary leaked")
			}
			mu.Lock()
			defer mu.Unlock()
			wantStages := []string{"artifact", "logs", "manifest", "result"}
			switch failure {
			case "invalid_edit", "invalid_output", "compiler", "cancel":
				wantStages = []string{"result"}
			case "crash_before_manifest":
				wantStages = []string{"artifact", "logs"}
			case "artifact":
				wantStages = []string{"artifact", "result"}
			case "logs":
				wantStages = []string{"artifact", "logs", "result"}
			case "manifest":
				wantStages = []string{"artifact", "logs", "manifest"}
			case "callback":
				wantStages = append(wantStages, "result", "result", "result")
			}
			if !reflect.DeepEqual(stages, wantStages) {
				t.Fatalf("pipeline stages %v, want %v; output: %s", stages, wantStages, &output)
			}
			want := "failed"
			if wantSuccess || failure == "callback" || failure == "crash_after_manifest" {
				want = "completed"
			}
			if failure == "manifest" || failure == "crash_before_manifest" {
				want = ""
			}
			if want == "" && len(outcomes) != 0 {
				t.Fatalf("uncertain manifest sent contradictory callback: %v", outcomes)
			}
			if want != "" && len(outcomes) == 0 {
				t.Fatal("missing callback")
			}
			for _, outcome := range outcomes {
				if outcome != want {
					t.Fatalf("outcomes: %v", outcomes)
				}
			}
			if failure == "callback" && len(outcomes) != 4 {
				t.Fatalf("unbounded retries: %v", outcomes)
			}
			if failure == "lost_ack" && len(outcomes) != 1 {
				t.Fatal("lookup failed to resolve lost acknowledgement")
			}
			if failure == "invalid_edit" || failure == "invalid_output" || failure == "compiler" || failure == "cancel" {
				for _, stage := range stages {
					if stage != "result" {
						t.Fatalf("invalid output uploaded: %v", stages)
					}
				}
			}
			if failure == "artifact" || failure == "logs" {
				for _, stage := range stages {
					if stage == "manifest" {
						t.Fatal("partial upload committed")
					}
				}
			}
			if strings.HasPrefix(failure, "crash_") && stored.Status != "running" {
				t.Fatal("crash invented terminal evidence")
			}
		})
	}
}

func TestRunnerDeadlineDuringFetch(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer api.Close()
	cfg := &config.Config{WorkDir: t.TempDir(), StorageURL: api.URL}
	job := contractJob()
	executor := codex.NewExecutor("unused", cfg.WorkDir, "test", "")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := runJobWithContext(ctx, cfg, &job, storage.NewClient(api.URL), executor, server.NewServer(0, executor))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline not propagated: %v", err)
	}
	if executor.IsRunning() {
		t.Fatal("executor started after deadline")
	}
}
