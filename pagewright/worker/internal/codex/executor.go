package codex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Executor wraps codex exec command execution
type Executor struct {
	binaryPath string
	workDir    string
	llmKey     string
	llmBaseURL string
	isolate    bool

	mu             sync.Mutex
	cmd            *exec.Cmd
	cancel         context.CancelFunc
	running        bool
	output         strings.Builder
	outputExceeded bool
}

// NewExecutor creates a new Codex executor
func NewExecutor(binaryPath, workDir, llmKey, llmBaseURL string) *Executor {
	return &Executor{
		binaryPath: binaryPath,
		workDir:    workDir,
		llmKey:     llmKey,
		llmBaseURL: llmBaseURL,
		isolate:    isolateCLI,
	}
}

// Execute runs codex exec with the given prompt
func (e *Executor) Execute(ctx context.Context, prompt string) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return fmt.Errorf("codex is already running")
	}
	e.running = true
	e.output.Reset()
	e.outputExceeded = false
	// Publish cancellation atomically with running, including preflight/startup.
	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	e.cancel = cancel
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.running = false
		e.cmd = nil
		e.cancel = nil
		e.mu.Unlock()
	}()

	defer cancel()

	if e.llmKey == "" {
		return fmt.Errorf("worker provider API key is required")
	}
	instructions, err := os.ReadFile(filepath.Join(e.workDir, ".codex", "instructions.md"))
	if err != nil {
		return fmt.Errorf("read trusted worker instructions: %w", err)
	}
	// Fresh runtime state outside the source archive: never reuse operator login,
	// project configuration or another job's session. No credentials are on argv.
	runtimeDir, err := os.MkdirTemp(filepath.Dir(e.workDir), "codex-runtime-")
	if err != nil {
		return fmt.Errorf("create CLI runtime: %w", err)
	}
	defer os.RemoveAll(runtimeDir)
	// Fail before contacting a provider if this Docker host cannot enforce the
	// required sandbox. No insecure fallback or approval escalation is offered.
	preflightCtx, stopPreflight := context.WithTimeout(cmdCtx, 10*time.Second)
	defer stopPreflight()
	preflight := exec.CommandContext(preflightCtx, e.binaryPath, "sandbox",
		"-c", "sandbox_mode=\"workspace-write\"",
		"-c", "sandbox_workspace_write.exclude_slash_tmp=true",
		"-c", "sandbox_workspace_write.exclude_tmpdir_env_var=true", "--", "/bin/true")
	preflight.Dir = e.workDir
	preflight.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + runtimeDir, "CODEX_HOME=" + runtimeDir}
	if e.isolate {
		// Fail before provider contact if PATH bypasses the tool-only filter.
		// Require the actual EPERM result, not merely a missing/failed binary.
		preflight.Args = append(preflight.Args[:len(preflight.Args)-1], "node", "-e", `const r=require('child_process').spawnSync('/usr/bin/unshare',['-U','/bin/true'],{encoding:'utf8'});process.exit(r.status===1 && r.stderr.includes('Operation not permitted') ? 0 : 1)`)
		wrapCLI(preflight, e.workDir, runtimeDir)
	}
	preflight.WaitDelay = 2 * time.Second
	confineProcess(preflight)
	if err := preflight.Run(); err != nil {
		return fmt.Errorf("worker sandbox unavailable; verify Docker host with make test-worker-cli: %w", err)
	}
	baseURL := e.llmBaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	cmd := exec.CommandContext(cmdCtx, e.binaryPath, "exec",
		"--ignore-user-config", "--ignore-rules", "--ephemeral", "--skip-git-repo-check",
		"--sandbox", "workspace-write", "--color", "never",
		"--disable", "shell_snapshot",
		"-c", "model_provider=\"pagewright\"",
		"-c", "model_providers.pagewright={name=\"PageWright\",base_url="+strconv.Quote(baseURL)+",env_key=\"CODEX_API_KEY\",wire_api=\"responses\",supports_websockets=false}",
		"-c", "approval_policy=\"never\"",
		"-c", "forced_login_method=\"api\"",
		"-c", "web_search=\"disabled\"",
		"-c", "sandbox_workspace_write.network_access=false",
		"-c", "sandbox_workspace_write.exclude_slash_tmp=true",
		"-c", "sandbox_workspace_write.exclude_tmpdir_env_var=true",
		"-c", "shell_environment_policy.inherit=\"none\"",
		"-c", "shell_environment_policy.ignore_default_excludes=false",
		"-c", "shell_environment_policy.filters={\"*KEY*\"=\"exclude\",\"*TOKEN*\"=\"exclude\",\"*SECRET*\"=\"exclude\"}",
		"-c", "shell_environment_policy.set={PATH=\"/usr/local/bin:/usr/bin:/bin\"}",
		"-c", "developer_instructions="+strconv.Quote(string(instructions)), "-")
	cmd.Dir = e.workDir
	cmd.WaitDelay = 2 * time.Second
	confineProcess(cmd)
	cmd.Stdin = strings.NewReader(prompt)

	// Deliberately exclude job JSON, callbacks, management credentials and host auth.
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C.UTF-8",
		"HOME=" + runtimeDir, "CODEX_HOME=" + runtimeDir, "CODEX_API_KEY=" + e.llmKey}
	if e.isolate {
		wrapCLI(cmd, e.workDir, runtimeDir)
	}

	e.mu.Lock()
	e.cmd = cmd
	e.mu.Unlock()

	cmd.Stdout = outputWriter{e}
	cmd.Stderr = outputWriter{e}

	// Start command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start codex: %w", err)
	}

	// Wait for command to complete
	err = cmd.Wait()
	// Normal exit also kills background children that retained no output pipes.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	e.mu.Lock()
	exceeded := e.outputExceeded
	e.mu.Unlock()
	if exceeded {
		return fmt.Errorf("codex output exceeded 1 MiB limit")
	}
	if cmdCtx.Err() != nil {
		return fmt.Errorf("codex execution stopped: %w", cmdCtx.Err())
	}

	if err != nil {
		if cmdCtx.Err() == context.Canceled {
			return fmt.Errorf("codex execution was cancelled")
		}
		return fmt.Errorf("codex execution failed: %w", err)
	}

	return nil
}

type outputWriter struct{ executor *Executor }

func (w outputWriter) Write(p []byte) (int, error) {
	w.executor.mu.Lock()
	defer w.executor.mu.Unlock()
	// Continue draining even after the capture limit, avoiding pipe deadlocks.
	remaining := 1024*1024 - w.executor.output.Len()
	if len(p) > remaining {
		w.executor.outputExceeded = true
		if w.executor.cancel != nil {
			w.executor.cancel()
		}
	}
	if remaining > len(p) {
		remaining = len(p)
	}
	if remaining > 0 {
		w.executor.output.Write(p[:remaining])
	}
	return len(p), nil
}

// Kill terminates the running codex process
func (e *Executor) Kill() error {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return fmt.Errorf("codex is not running")
	}

	cancel := e.cancel
	e.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	return nil
}

// Linux worker: cancellation terminates the process group, not just its leader.
func confineProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
}

// IsRunning returns whether codex is currently executing
func (e *Executor) IsRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// GetOutput returns the captured output
func (e *Executor) GetOutput() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	output := e.output.String()
	if e.llmKey != "" {
		output = strings.ReplaceAll(output, e.llmKey, "[REDACTED]")
	}
	return output
}

// ParseOutput extracts FILES_CHANGED and SUMMARY from codex output
func (e *Executor) ParseOutput() (filesChanged []string, summary string) {
	output := e.GetOutput()
	output = strings.ReplaceAll(output, "[STDOUT] ", "")

	// Look for FILES_CHANGED section
	if idx := strings.Index(output, "FILES_CHANGED:"); idx != -1 {
		section := output[idx:]
		lines := strings.Split(section, "\n")
		for i := 1; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			// Stop at empty line, code fence, or SUMMARY
			if line == "" || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "SUMMARY") {
				break
			}
			// Parse lines like "- modified: path" or just "- path"
			if strings.HasPrefix(line, "- ") {
				// Handle both "- modified: path" and "- path"
				rest := line[2:]
				if strings.Contains(rest, ":") {
					parts := strings.SplitN(rest, ":", 2)
					if len(parts) == 2 {
						filesChanged = append(filesChanged, strings.TrimSpace(parts[1]))
					}
				} else {
					filesChanged = append(filesChanged, strings.TrimSpace(rest))
				}
			}
		}
	}

	// Look for SUMMARY section
	if idx := strings.Index(output, "SUMMARY:"); idx != -1 {
		section := strings.TrimSpace(output[idx+8:])
		lines := strings.Split(section, "\n")
		for i := 0; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if line == "" || strings.HasPrefix(line, "```") {
				break
			}
			if summary != "" {
				summary += " "
			}
			summary += line
		}
	}

	return filesChanged, summary
}
