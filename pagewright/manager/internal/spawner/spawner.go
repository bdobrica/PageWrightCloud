package spawner

import (
	"context"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
)

// Spawner defines the interface for worker spawners
type Spawner interface {
	// Spawn creates and starts a worker container
	Spawn(ctx context.Context, job *types.Job, managerURL string) (workerID string, err error)

	// Close closes the spawner
	Close() error
}
