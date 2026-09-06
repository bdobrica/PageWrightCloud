package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutorMock(t *testing.T) {

	// Create temporary work directory
	workDir, err := os.MkdirTemp("", "codex-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(workDir)

	// Create mock codex script
	mockCodex := filepath.Join(workDir, "mock-codex")
	mockScript := `#!/bin/sh
echo "Processing prompt"
echo "FILES_CHANGED:"
echo "- modified: content/page.md"
echo "- created: theme/new-style.css"
echo "SUMMARY:"
echo "Updated page content and added new stylesheet"
exit 0
`
	require.NoError(t, os.WriteFile(mockCodex, []byte(mockScript), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".codex"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, ".codex", "instructions.md"), []byte("trusted instructions"), 0600))

	executor := newTestExecutor(mockCodex, workDir, "test-key", "https://api.test.com")

	// Test execution
	ctx := context.Background()
	err = executor.Execute(ctx, "Update the homepage")
	require.NoError(t, err)

	// Test output capture
	output := executor.GetOutput()
	assert.Contains(t, output, "Processing prompt")

	// Test parsing
	filesChanged, summary := executor.ParseOutput()
	assert.Len(t, filesChanged, 2)
	assert.Contains(t, filesChanged, "content/page.md")
	assert.Contains(t, filesChanged, "theme/new-style.css")
	assert.Contains(t, summary, "Updated page content")
}

func TestExecutorKill(t *testing.T) {

	workDir, err := os.MkdirTemp("", "codex-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(workDir)

	// Create mock codex that sleeps
	mockCodex := filepath.Join(workDir, "mock-codex-slow")
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".codex"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, ".codex", "instructions.md"), []byte("trusted"), 0600))
	mockScript := `#!/bin/sh
if [ "$1" = sandbox ]; then exit 0; fi
(sleep 2; touch escaped-child) &
echo READY
wait
`
	require.NoError(t, os.WriteFile(mockCodex, []byte(mockScript), 0755))

	executor := newTestExecutor(mockCodex, workDir, "test-key", "https://api.test.com")

	// Start execution in background
	done := make(chan error, 1)
	go func() {
		ctx := context.Background()
		done <- executor.Execute(ctx, "Long running task")
	}()

	// Wait for it to start
	require.Eventually(t, func() bool { return strings.Contains(executor.GetOutput(), "READY") }, 3*time.Second, 10*time.Millisecond)
	assert.True(t, executor.IsRunning())

	// Kill it
	err = executor.Kill()
	require.NoError(t, err)

	// Wait a bit and verify it stopped
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not finish")
	}
	assert.False(t, executor.IsRunning())
	time.Sleep(2100 * time.Millisecond)
	_, err = os.Stat(filepath.Join(workDir, "escaped-child"))
	require.True(t, os.IsNotExist(err), "child survived cancellation")
}

func TestParseOutput(t *testing.T) {

	executor := newTestExecutor("", "", "", "")

	// Simulate output
	executor.mu.Lock()
	executor.output.WriteString(`[STDOUT] Making changes...
[STDOUT] FILES_CHANGED:
[STDOUT] - modified: content/index.md
[STDOUT] - created: assets/logo.png
[STDOUT] - deleted: old/deprecated.css
[STDOUT] 
[STDOUT] SUMMARY:
[STDOUT] Updated homepage content with new logo
[STDOUT] and removed deprecated styles.
`)
	executor.mu.Unlock()

	filesChanged, summary := executor.ParseOutput()

	assert.Len(t, filesChanged, 3)
	assert.Contains(t, filesChanged, "content/index.md")
	assert.Contains(t, filesChanged, "assets/logo.png")
	assert.Contains(t, filesChanged, "old/deprecated.css")

	assert.Contains(t, summary, "Updated homepage")
	assert.Contains(t, summary, "removed deprecated")
}
