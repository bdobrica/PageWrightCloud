package codex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOutputLimitCancelsExecution(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".codex"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".codex/instructions.md"), []byte("trusted"), 0600))
	binary := filepath.Join(dir, "fixture")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\nif [ \"$1\" = sandbox ]; then exit 0; fi\nyes output\n"), 0700))
	e := newTestExecutor(binary, dir, "fixture-key", "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.ErrorContains(t, e.Execute(ctx, "prompt"), "output exceeded")
	require.LessOrEqual(t, len(e.GetOutput()), 1<<20)
	require.False(t, e.IsRunning())
}
