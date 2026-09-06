//go:build integration

package integration

import (
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
	"testing"
	"time"
)

func waitForDispatch(t *testing.T, manager *clients.ManagerClient, id string) *types.Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		job, err := manager.GetJobStatus(id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status == types.JobStatusRunning {
			return job
		}
		if job.Status != types.JobStatusPending || time.Now().After(deadline) {
			t.Fatalf("job did not dispatch: %+v", job)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
