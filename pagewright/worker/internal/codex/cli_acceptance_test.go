//go:build cli_acceptance

package codex

import (
	"bytes"
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
	private := filepath.Join(t.TempDir(), "management-secret")
	if err := os.WriteFile(private, []byte("management-canary"), 0600); err != nil {
		t.Fatal(err)
	}
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
			command := "test -z \"$CODEX_API_KEY\" && test ! -e " + private + " && test ! -e /var/run/docker.sock && test ! -e /opt/pagewright/themes && ! grep -aq fixture-key /proc/[0-9]*/environ 2>/dev/null && touch CLI_WRITE_SENTINEL"
			arguments, _ := json.Marshal(map[string]any{"cmd": command, "max_output_tokens": 1000})
			message = map[string]any{"id": "fc_fixture", "type": "function_call", "call_id": "call_fixture", "name": "exec_command", "arguments": string(arguments)}
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
	// A tool must not create a mount alias that bypasses path-based protections.
	namespace := exec.CommandContext(ctx, "/usr/local/bin/codex", "sandbox", "-c", "sandbox_mode=\"workspace-write\"", "--", "sh", "-c", "touch namespace-probe-started; unshare -U /bin/true")
	namespace.Dir, namespace.Env = inside, cmd.Env
	output, err = namespace.CombinedOutput()
	if _, startErr := os.Stat(filepath.Join(inside, "namespace-probe-started")); startErr != nil {
		t.Fatalf("namespace probe did not start: %v %s", startErr, output)
	}
	if err == nil || !strings.Contains(string(output), "Operation not permitted") {
		t.Fatalf("tool user namespace was not denied: %v %s", err, output)
	}
	mount := exec.CommandContext(ctx, "mount", "-t", "tmpfs", "none", inside)
	if err := mount.Run(); err == nil {
		t.Fatal("outer container acquired mount authority")
	}
}

func TestInstalledOuterNamespaceLifecycle(t *testing.T) {
	for _, mode := range []string{"exit", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			site, runtime := t.TempDir(), t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			script := "setsid sh -c 'touch started; sleep 2; touch escaped' >/dev/null 2>&1 & while [ ! -e started ]; do sleep 0.01; done; "
			if mode == "cancel" {
				script += "echo READY; sleep 20"
			} else {
				script += "exit 0"
			}
			cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script)
			cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin"}
			wrapCLI(cmd, site, runtime)
			confineProcess(cmd)
			var diagnostics bytes.Buffer
			cmd.Stdout, cmd.Stderr = &diagnostics, &diagnostics
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			started := false
			for ctx.Err() == nil {
				if _, err := os.Stat(filepath.Join(site, "started")); err == nil {
					started = true
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if mode == "cancel" || !started {
				cancel()
			}
			err := cmd.Wait()
			if !started {
				t.Fatalf("namespace never started child: %v %s", err, diagnostics.String())
			}
			if mode == "exit" && err != nil {
				t.Fatalf("namespace exit: %v %s", err, diagnostics.String())
			}
			if mode == "cancel" && err == nil {
				t.Fatal("cancellation succeeded unexpectedly")
			}
			time.Sleep(2200 * time.Millisecond)
			if _, err := os.Stat(filepath.Join(site, "escaped")); !os.IsNotExist(err) {
				t.Fatal("detached child survived namespace teardown")
			}
		})
	}
}

func TestInstalledResourceLimits(t *testing.T) {
	if expected := os.Getenv("PAGEWRIGHT_EXPECT_APPARMOR"); expected != "" {
		profile, err := os.ReadFile("/proc/self/attr/current")
		if err != nil || strings.TrimSpace(string(profile)) != expected+" (enforce)" {
			t.Fatalf("AppArmor not enforcing: %s %v", profile, err)
		}
		probe := "/tmp/pagewright-apparmor-probe"
		if err := os.WriteFile(probe, []byte("probe"), 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(probe)
		if _, err := os.ReadFile(probe); !os.IsPermission(err) {
			t.Fatalf("AppArmor read rule not enforced: %v", err)
		}
		if expected == "pagewright-worker-proc" {
			for _, path := range []string{"/proc/interrupts", "/proc/keys", "/proc/timer_list", "/proc/kcore"} {
				file, err := os.Open(path)
				if err == nil {
					file.Close()
					t.Errorf("protected proc read allowed: %s", path)
				} else if !os.IsPermission(err) {
					t.Errorf("proc denial not proven for %s: %v", path, err)
				}
			}
			// Opening without truncation or writing cannot change a sysctl even
			// if the protection under test is broken.
			file, err := os.OpenFile("/proc/sys/kernel/hostname", os.O_WRONLY, 0)
			if err == nil {
				file.Close()
				t.Fatal("protected proc write-open allowed")
			}
			if !os.IsPermission(err) {
				t.Fatalf("proc write denial not proven: %v", err)
			}
		}
	}
	for file, want := range map[string]string{"memory.max": "1073741824", "memory.swap.max": "0", "pids.max": "128", "cpu.max": "100000 100000"} {
		data, err := os.ReadFile(filepath.Join("/sys/fs/cgroup", file))
		if err != nil || strings.TrimSpace(string(data)) != want {
			t.Fatalf("cgroup %s: %q %v", file, data, err)
		}
	}
	if err := os.WriteFile("/etc/worker-write-test", []byte("denied"), 0600); err == nil {
		t.Fatal("root filesystem writable")
	}
}
