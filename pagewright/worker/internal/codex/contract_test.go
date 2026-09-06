package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvocationContract(t *testing.T) {
	parent := t.TempDir()
	site := filepath.Join(parent, "site")
	require.NoError(t, os.MkdirAll(filepath.Join(site, ".codex"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(site, ".codex", "instructions.md"), []byte("trusted instruction sentinel"), 0600))
	binary := filepath.Join(parent, "fixture")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\nenv\ncat\n"), 0700))
	t.Setenv("PAGEWRIGHT_JOB", "must-not-inherit-job")
	t.Setenv("OPENAI_API_KEY", "must-not-inherit-key")
	t.Setenv("PAGEWRIGHT_MANAGER_URL", "must-not-inherit-manager")
	e := newTestExecutor(binary, site, "private-provider-key", "http://fixture.invalid/v1")
	require.NoError(t, e.Execute(context.Background(), "--dangerously-bypass-approvals-and-sandbox is user text"))
	output := e.GetOutput()
	for _, want := range []string{"workspace-write", "approval_policy=\"never\"", "CODEX_API_KEY=[REDACTED]", "trusted instruction sentinel", "http://fixture.invalid/v1", "shell_environment_policy.inherit=\"none\""} {
		require.Contains(t, output, want)
	}
	for _, forbidden := range []string{"private-provider-key", "must-not-inherit", "OPENAI_API_KEY="} {
		require.NotContains(t, output, forbidden)
	}
	require.Equal(t, 1, strings.Count(output, "--dangerously-bypass-approvals-and-sandbox"), "user text must arrive only on stdin")
	entries, err := filepath.Glob(filepath.Join(parent, "codex-runtime-*"))
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestMissingCredentialsAndInstructionsFailClosed(t *testing.T) {
	e := newTestExecutor("/must-not-execute", t.TempDir(), "", "")
	require.ErrorContains(t, e.Execute(context.Background(), "prompt"), "API key is required")
	e = newTestExecutor("/must-not-execute", t.TempDir(), "fixture-key", "")
	require.ErrorContains(t, e.Execute(context.Background(), "prompt"), "trusted worker instructions")
}

func TestCaptureLimitAndRedaction(t *testing.T) {
	e := newTestExecutor("", "", "secret-value", "")
	w := outputWriter{e}
	_, err := w.Write([]byte("secret-"))
	require.NoError(t, err)
	_, err = w.Write([]byte("value"))
	require.NoError(t, err)
	require.Equal(t, "[REDACTED]", e.GetOutput())
	data := []byte(strings.Repeat("x", 2*1024*1024))
	n, err := w.Write(data)
	require.NoError(t, err)
	require.Equal(t, len(data), n)
	require.LessOrEqual(t, len(e.GetOutput()), 1024*1024)
}

func TestSandboxFailureStopsBeforeProviderInvocation(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".codex"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".codex", "instructions.md"), []byte("instructions"), 0600))
	binary := filepath.Join(dir, "fixture")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\nif [ \"$1\" = sandbox ]; then exit 42; fi\ntouch provider-invoked\n"), 0700))
	e := newTestExecutor(binary, dir, "fixture-key", "")
	require.ErrorContains(t, e.Execute(context.Background(), "prompt"), "worker sandbox unavailable")
	_, err := os.Stat(filepath.Join(dir, "provider-invoked"))
	require.True(t, os.IsNotExist(err))
}
