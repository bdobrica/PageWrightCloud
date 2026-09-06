//go:build cli_acceptance

package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Runs against the installed production binary, never a real provider.
func TestInstalledCLI(t *testing.T) {
	version, err := exec.Command("/usr/local/bin/codex", "--version").Output()
	if err != nil || strings.TrimSpace(string(version)) != "codex-cli 0.153.4" {
		t.Fatalf("version: %s %v", version, err)
	}
	var mu sync.Mutex
	var requests []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("fixture request: %s %s", r.Method, r.URL.Path)
		if r.Header.Get("Upgrade") != "" {
			http.Error(w, "no websocket", 400)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(r.URL.Path, "responses") {
			http.Error(w, "fixture only supports responses", 404)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("missing explicit API authentication")
		}
		mu.Lock()
		requests = append(requests, string(body))
		first := len(requests) == 1
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		message := map[string]any{"id": "msg_fixture", "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": "SUMMARY: Installed CLI fixture completed"}}}
		if first {
			message = map[string]any{"id": "fc_fixture", "type": "function_call", "call_id": "call_fixture", "name": "exec_command", "arguments": "{\"cmd\":\"test -z \\\"$CODEX_API_KEY\\\" && touch CLI_WRITE_SENTINEL\",\"max_output_tokens\":1000}"}
		}
		for _, event := range []map[string]any{
			{"type": "response.created", "response": map[string]any{"id": "resp_fixture", "status": "in_progress", "output": []any{}}},
			{"type": "response.output_item.added", "output_index": 0, "item": message},
			{"type": "response.output_item.done", "output_index": 0, "item": message},
			{"type": "response.completed", "response": map[string]any{"id": "resp_fixture", "status": "completed", "output": []any{message}, "usage": map[string]int{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}}},
		} {
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event["type"], data)
		}
	}))
	defer api.Close()
	dir := t.TempDir()
	site := filepath.Join(dir, "site")
	if err := os.MkdirAll(filepath.Join(site, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(site, ".codex", "instructions.md"), []byte("PAGEWRIGHT_TRUSTED_INSTRUCTION_SENTINEL"), 0600); err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor("/usr/local/bin/codex", site, "fixture-key", api.URL+"/v1")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := executor.Execute(ctx, "PAGEWRIGHT_PROMPT_SENTINEL"); err != nil {
		t.Fatalf("exec: %v\n%s", err, executor.GetOutput())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) == 0 {
		t.Fatal("installed CLI did not contact fixture")
	}
	if !strings.Contains(requests[0], "PAGEWRIGHT_TRUSTED_INSTRUCTION_SENTINEL") || !strings.Contains(requests[0], "PAGEWRIGHT_PROMPT_SENTINEL") {
		t.Fatal("instruction/prompt missing from actual request")
	}
	if !strings.Contains(executor.GetOutput(), "Installed CLI fixture completed") {
		t.Fatalf("final message missing: %s", executor.GetOutput())
	}
	if _, err := os.Stat(filepath.Join(site, "CLI_WRITE_SENTINEL")); err != nil {
		t.Fatalf("real tool could not write workspace: %s", executor.GetOutput())
	}
	if entries, _ := filepath.Glob(filepath.Join(dir, "codex-runtime-*")); len(entries) != 0 {
		t.Fatal("CLI state not cleaned")
	}
}

func TestInstalledSandbox(t *testing.T) {
	if os.Getuid() != 1000 {
		t.Fatal("sandbox acceptance must run as worker UID 1000")
	}
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(status), "CapEff:\t0000000000000000") || !strings.Contains(string(status), "NoNewPrivs:\t1") || !strings.Contains(string(status), "Seccomp:\t2") {
		t.Fatal("outer container confinement not active")
	}
	dir := t.TempDir()
	inside := filepath.Join(dir, "site")
	if err := os.Mkdir(inside, 0700); err != nil {
		t.Fatal(err)
	}
	// Outside is writable by the container user, so denial must come from Codex.
	outside := filepath.Join(dir, "outside")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/local/bin/codex", "sandbox", "-c", "sandbox_mode=\"workspace-write\"", "-c", "sandbox_workspace_write.exclude_slash_tmp=true", "-c", "sandbox_workspace_write.exclude_tmpdir_env_var=true", "--", "sh", "-c", "touch allowed; touch "+outside)
	cmd.Dir = inside
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + dir, "CODEX_HOME=" + dir}
	output, err := cmd.CombinedOutput()
	if _, statErr := os.Stat(outside); statErr == nil {
		t.Fatal("sandbox allowed outside write")
	}
	if err == nil {
		t.Fatalf("sandbox did not reject outside write: %s", output)
	}
	if _, statErr := os.Stat(filepath.Join(inside, "allowed")); statErr != nil {
		t.Fatalf("sandbox could not execute inside workspace under worker restrictions: %s", output)
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer api.Close()
	response, err := http.Get(api.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	network := exec.CommandContext(ctx, "/usr/local/bin/codex", "sandbox", "-c", "sandbox_mode=\"workspace-write\"", "--", "node", "-e",
		"const u=new URL("+fmt.Sprintf("%q", api.URL)+");require('net').connect({host:u.hostname,port:u.port}).once('connect',()=>process.exit(7)).once('error',()=>process.exit(0));setTimeout(()=>process.exit(8),2000)")
	network.Dir = inside
	network.Env = cmd.Env
	if output, err := network.CombinedOutput(); err != nil {
		t.Fatalf("sandbox network denial failed: %v %s", err, output)
	}
	mount := exec.CommandContext(ctx, "mount", "-t", "tmpfs", "none", inside)
	if err := mount.Run(); err == nil {
		t.Fatal("outer container acquired mount authority")
	}
}
