package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	lockKeyPrefix  = "lock:site:"
	fenceKeyPrefix = "fence:site:"
)

// Lua script to release lock only if token matches
const releaseLuaScript = `
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end
`

// Lua script to renew lock only if token matches
const renewLuaScript = `
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("pexpire", KEYS[1], ARGV[2])
else
    return 0
end
`

type RedisLockManager struct {
	client *redis.Client
}

func NewRedisLockManager(addr, password string, db int) (*RedisLockManager, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisLockManager{
		client: client,
	}, nil
}

func (r *RedisLockManager) Acquire(ctx context.Context, siteID string, ttl time.Duration) (string, int64, error) {
	lockKey := lockKeyPrefix + siteID
	fenceKey := fenceKeyPrefix + siteID
	token := uuid.New().String()

	// Try to acquire lock
	success, err := r.client.SetNX(ctx, lockKey, token, ttl).Result()
	if err != nil {
		return "", 0, fmt.Errorf("failed to acquire lock: %w", err)
	}

	if !success {
		return "", 0, fmt.Errorf("lock already held for site: %s", siteID)
	}

	// Increment fencing token
	fencingToken, err := r.client.Incr(ctx, fenceKey).Result()
	if err != nil {
		// Try to release the lock we just acquired
		r.Release(ctx, siteID, token)
		return "", 0, fmt.Errorf("failed to increment fencing token: %w", err)
	}

	return token, fencingToken, nil
}

func (r *RedisLockManager) Renew(ctx context.Context, siteID, token string, ttl time.Duration) error {
	lockKey := lockKeyPrefix + siteID
	ttlMs := ttl.Milliseconds()

	result, err := r.client.Eval(ctx, renewLuaScript, []string{lockKey}, token, ttlMs).Result()
	if err != nil {
		return fmt.Errorf("failed to renew lock: %w", err)
	}

	if result == int64(0) {
		return fmt.Errorf("lock token mismatch or lock not held")
	}

	return nil
}

func (r *RedisLockManager) Release(ctx context.Context, siteID, token string) error {
	lockKey := lockKeyPrefix + siteID

	result, err := r.client.Eval(ctx, releaseLuaScript, []string{lockKey}, token).Result()
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	if result == int64(0) {
		return fmt.Errorf("lock token mismatch or lock not held")
	}

	return nil
}

func (r *RedisLockManager) Close() error {
	return r.client.Close()
}
