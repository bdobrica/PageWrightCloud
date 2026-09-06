package spawner

import (
	"context"
	"errors"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
)

// ErrNotStarted is positive evidence that this attempt did not start execution.
// Other errors are ambiguous: retain the reservation and lock.
var ErrNotStarted = errors.New("worker definitely not started")

// Spawner defines the interface for worker spawners
type Spawner interface {
	// Spawn creates and starts a worker container
	Spawn(ctx context.Context, job *types.Job, managerURL string) (workerID string, err error)

	// Close closes the spawner
	Close() error
}
