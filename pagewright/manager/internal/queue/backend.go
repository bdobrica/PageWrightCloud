package queue

import (
	"context"
	"errors"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
)

var ErrJobNotFound = errors.New("job not found")

// Backend defines the interface for queue backends
type Backend interface {
	// CreateJob reserves and enqueues once; a duplicate returns its stored snapshot.
	CreateJob(ctx context.Context, job *types.Job) (*types.Job, bool, error)

	// SetWorkerID changes only spawn metadata, preserving concurrent outcomes.
	SetWorkerID(ctx context.Context, jobID, workerID string) (*types.Job, error)

	// Pop retrieves and removes a job from the queue (blocking with timeout)
	Pop(ctx context.Context) (*types.Job, error)

	// GetJob retrieves a job by ID without removing it
	GetJob(ctx context.Context, jobID string) (*types.Job, error)

	// UpdateJob updates a job's status and metadata
	UpdateJob(ctx context.Context, job *types.Job) error

	// Close closes the backend connection
	Close() error
}
