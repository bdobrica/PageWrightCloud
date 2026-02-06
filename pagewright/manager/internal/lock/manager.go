package lock

import (
	"context"
	"time"
)

// Manager handles distributed locking
type Manager interface {
	// Acquire acquires a lock for a site and returns a token and fencing token
	Acquire(ctx context.Context, siteID string, ttl time.Duration) (token string, fencingToken int64, err error)

	// Renew extends the lock TTL
	Renew(ctx context.Context, siteID, token string, ttl time.Duration) error

	// Release releases the lock
	Release(ctx context.Context, siteID, token string) error

	// Close closes the lock manager
	Close() error
}
