//go:build integration

package main

import (
	"context"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
)

// Contract suites launch their own deterministic worker; never in production.
type manualSpawner struct{}

func (manualSpawner) Spawn(_ context.Context, j *types.Job, _ string) (string, error) {
	return "test-manual-" + j.JobID, nil
}
func (manualSpawner) Close() error { return nil }
func init()                        { testSpawners["test-manual"] = func() spawner.Spawner { return manualSpawner{} } }
