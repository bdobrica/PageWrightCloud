package queue

import (
	"context"
	"errors"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
)

var ErrJobNotFound = errors.New("job not found")
var ErrClaimLost = errors.New("dispatch claim lost")
var ErrFenced = errors.New("stale attempt or conflicting commit")

type CommitBackend interface {
	AuthorizeWrite(context.Context, *types.WriteCommit) error
}

type Claim struct {
	Job   *types.Job
	Token string
}

// DispatchBackend keeps recoverable claims separate from irrevocable launch intent.
type DispatchBackend interface {
	Backend
	InitializeDispatch(context.Context, int) error
	Claim(context.Context, int, time.Duration) (*Claim, error)
	BeginDispatch(context.Context, *Claim, string, int64) (*types.Job, error)
	ResolveDispatch(context.Context, *Claim, string, string) error
}

// Backend defines the interface for queue backends
type Backend interface {
	// CreateJob reserves and enqueues once; a duplicate returns its stored snapshot.
	CreateJob(ctx context.Context, job *types.Job) (*types.Job, bool, error)

	// SetWorkerID changes only spawn metadata, preserving concurrent outcomes.
	SetWorkerID(ctx context.Context, jobID, workerID string) (*types.Job, error)

	// GetJob retrieves a job by ID without removing it
	GetJob(ctx context.Context, jobID string) (*types.Job, error)

	// UpdateJob updates a job's status and metadata
	UpdateJob(ctx context.Context, job *types.Job) error

	// Close closes the backend connection
	Close() error
}
