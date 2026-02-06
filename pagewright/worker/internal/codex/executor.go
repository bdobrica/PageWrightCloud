package codex

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Executor wraps codex exec command execution
type Executor struct {
	binaryPath string
	workDir    string
	llmKey     string
	llmBaseURL string

	mu      sync.Mutex
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	running bool
	output  strings.Builder
}

// NewExecutor creates a new Codex executor
func NewExecutor(binaryPath, workDir, llmKey, llmBaseURL string) *Executor {
	return &Executor{
		binaryPath: binaryPath,
		workDir:    workDir,
		llmKey:     llmKey,
		llmBaseURL: llmBaseURL,
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
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.running = false
		e.cmd = nil
		e.cancel = nil
		e.mu.Unlock()
	}()

	// Create cancellable context
	cmdCtx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.cancel = cancel
	e.mu.Unlock()
	defer cancel()

	// Prepare command
	cmd := exec.CommandContext(cmdCtx, e.binaryPath, "exec", prompt)
	cmd.Dir = e.workDir

	// Set environment variables
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("OPENAI_API_KEY=%s", e.llmKey))
	if e.llmBaseURL != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("OPENAI_BASE_URL=%s", e.llmBaseURL))
	}

	e.mu.Lock()
	e.cmd = cmd
	e.mu.Unlock()

	// Capture stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start codex: %w", err)
	}

	// Read output in goroutines
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		e.readOutput(stdout, "STDOUT")
	}()

	go func() {
		defer wg.Done()
		e.readOutput(stderr, "STDERR")
	}()

	// Wait for command to complete
	err = cmd.Wait()
	wg.Wait()

	if err != nil {
		if cmdCtx.Err() == context.Canceled {
			return fmt.Errorf("codex execution was cancelled")
		}
		return fmt.Errorf("codex execution failed: %w", err)
	}

	return nil
}

func (e *Executor) readOutput(r io.Reader, prefix string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		e.mu.Lock()
		e.output.WriteString(fmt.Sprintf("[%s] %s\n", prefix, line))
		e.mu.Unlock()

		// Also log to console
		fmt.Printf("[CODEX %s] %s\n", prefix, line)
	}
}

// Kill terminates the running codex process
func (e *Executor) Kill() error {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return fmt.Errorf("codex is not running")
	}

	cancel := e.cancel
	cmd := e.cmd
	e.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if cmd != nil && cmd.Process != nil {
		if err := cmd.Process.Kill(); err != nil && !strings.Contains(err.Error(), "already finished") {
			return fmt.Errorf("failed to kill codex process: %w", err)
		}
	}

	return nil
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
	return e.output.String()
}

// ParseOutput extracts FILES_CHANGED and SUMMARY from codex output
func (e *Executor) ParseOutput() (filesChanged []string, summary string) {
	output := e.GetOutput()

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
		section := output[idx+8:]
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
