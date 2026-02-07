package queue

import (
	"context"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
)

// Backend defines the interface for queue backends
type Backend interface {
	// Push adds a job to the queue
	Push(ctx context.Context, job *types.Job) error

	// Pop retrieves and removes a job from the queue (blocking with timeout)
	Pop(ctx context.Context) (*types.Job, error)

	// GetJob retrieves a job by ID without removing it
	GetJob(ctx context.Context, jobID string) (*types.Job, error)

	// UpdateJob updates a job's status and metadata
	UpdateJob(ctx context.Context, job *types.Job) error

	// Close closes the backend connection
	Close() error
}
